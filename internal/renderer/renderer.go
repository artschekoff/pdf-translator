package renderer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/razvandimescu/gopdf/pdf"
	"go.uber.org/zap"
)

type Renderer struct {
	fontManager *FontManager
	logger      *zap.Logger
}

func NewRenderer(dataDir string, logger *zap.Logger) *Renderer {
	return &Renderer{
		fontManager: NewFontManager(dataDir),
		logger:      logger,
	}
}

// RenderPage creates a single-page translated PDF. It covers original text
// regions with white rectangles and overlays the translated text.
//
// The key technique: the existing content stream is wrapped in q...Q to
// isolate its graphics state (CTM). Our redaction rectangles and text
// overlays are appended in the page's default coordinate space.
func (r *Renderer) RenderPage(ctx context.Context, inputPath string, password string, pageNum int, blocks []domain.TextBlock, targetLang string, outputPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	origData, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading input PDF: %w", err)
	}

	reader, err := pdf.Open(origData)
	if err != nil {
		return fmt.Errorf("opening PDF: %w", err)
	}

	allPages, err := reader.Pages()
	if err != nil {
		return fmt.Errorf("reading pages: %w", err)
	}

	pageIndex := pageNum - 1
	if pageIndex < 0 || pageIndex >= len(allPages) {
		return domain.ErrPageOutOfRange
	}

	srcPage := allPages[pageIndex]
	existingContent, err := reader.PageContent(srcPage)
	if err != nil {
		return fmt.Errorf("reading page content: %w", err)
	}

	fontRegular := pdf.Name("F_gopdf_overlay")
	fontBold := pdf.Name("F_gopdf_overlay_b")

	// Type1 Helvetica only supports WinAnsiEncoding (Latin-1 + small
	// extensions). Warn when the translated text uses a script that will
	// not render correctly until TrueType font embedding is implemented.
	for _, block := range blocks {
		if script := DetectScript(block.Translated); script != "latin" && script != "cyrillic" {
			r.logger.Warn("non-Latin script detected; Helvetica Type1 may not render correctly — TrueType embedding not yet implemented",
				zap.Int("page", pageNum),
				zap.String("script", script),
			)
			break
		}
	}

	var extra strings.Builder

	// Pass 1: draw white rectangles to cover original text.
	for _, block := range blocks {
		if strings.TrimSpace(block.Translated) == "" {
			continue
		}

		padX := block.FontSize * 0.3
		padY := block.FontSize * 0.5
		extra.WriteString(fmt.Sprintf("q 1 1 1 rg %.2f %.2f %.2f %.2f re f Q\n",
			block.BBox.X-padX,
			block.BBox.Y-padY,
			block.BBox.Width+2*padX,
			block.BBox.Height+2*padY))
	}

	// Pass 2: overlay translated text.
	for _, block := range blocks {
		if strings.TrimSpace(block.Translated) == "" {
			continue
		}

		layout := FitText(block.Translated, block.BBox, block.FontSize)
		isRTL := IsRTL(block.Translated)

		fontName := fontRegular
		if block.BlockType == domain.BlockTypeTitle {
			fontName = fontBold
		}

		for i, line := range layout.Lines {
			x := block.BBox.X
			y := block.BBox.Y + block.BBox.Height - float64(i+1)*layout.LineHeight

			if isRTL {
				x = block.BBox.X + block.BBox.Width
			}

			if y < block.BBox.Y-block.FontSize*0.5 {
				r.logger.Warn("text overflow",
					zap.Int("page", pageNum),
					zap.Int("textLen", len([]rune(block.Translated))),
				)
				break
			}

			extra.WriteString(fmt.Sprintf(
				"q BT 0 0 0 rg /%s %.1f Tf %.2f %.2f Td (%s) Tj ET Q\n",
				fontName, layout.FontSize, x, y, escapeStringPDF(line)))
		}
	}

	// Wrap existing content in q...Q so the original page's CTM is isolated,
	// then append our modifications in the default coordinate space.
	var combined []byte
	if len(existingContent) > 0 {
		combined = append(combined, []byte("q\n")...)
		combined = append(combined, existingContent...)
		combined = append(combined, []byte("\nQ\n")...)
	}
	combined = append(combined, []byte(extra.String())...)

	w := pdf.NewWriter()
	pagesRef := w.AllocRef()
	catalogRef := w.AllocRef()

	copier := newPageCopier(reader, w)

	// Copy the page, skipping Contents (we replace it) and Resources (we inline & modify).
	copiedPage := make(pdf.Dict)
	for k, v := range srcPage {
		if k == "Parent" || k == "Contents" || k == "Resources" {
			continue
		}
		copiedPage[k] = copier.copyObject(v)
	}
	copiedPage["Parent"] = pagesRef

	resCopy := buildResources(reader, copier, srcPage, fontRegular, fontBold)
	copiedPage["Resources"] = resCopy

	contentRef := w.AllocRef()
	w.WriteStream(contentRef, pdf.Dict{}, combined)
	copiedPage["Contents"] = contentRef

	pageRef := w.AllocRef()
	w.WriteObject(pageRef, copiedPage)

	w.WriteObject(pagesRef, pdf.Dict{
		"Type":  pdf.Name("Pages"),
		"Kids":  pdf.Array{pageRef},
		"Count": 1,
	})
	w.WriteObject(catalogRef, pdf.Dict{
		"Type":  pdf.Name("Catalog"),
		"Pages": pagesRef,
	})

	var origID pdf.Array
	if trailer := reader.Trailer(); trailer != nil {
		if id, ok := trailer.Array("ID"); ok {
			origID = id
		}
	}

	result, err := w.FinishWithID(catalogRef, origID)
	if err != nil {
		return fmt.Errorf("building output PDF: %w", err)
	}

	return os.WriteFile(outputPath, result, 0o600)
}

// buildResources deep-copies the source page's Resources dict, inlining
// the Font sub-dict so we can add our overlay fonts (regular + bold).
func buildResources(reader *pdf.Reader, copier *pageCopier, srcPage pdf.Dict, regular, bold pdf.Name) pdf.Dict {
	overlayFonts := pdf.Dict{
		regular: type1Font("Helvetica"),
		bold:    type1Font("Helvetica-Bold"),
	}

	srcRes, ok := reader.ResolveDict(srcPage["Resources"])
	if !ok || srcRes == nil {
		return pdf.Dict{"Font": overlayFonts}
	}

	resCopy := make(pdf.Dict, len(srcRes))
	for k, v := range srcRes {
		if k == "Font" {
			srcFontDict, ok := reader.ResolveDict(v)
			if ok && srcFontDict != nil {
				fontCopy := make(pdf.Dict, len(srcFontDict)+2)
				for fk, fv := range srcFontDict {
					fontCopy[fk] = copier.copyObject(fv)
				}
				fontCopy[regular] = type1Font("Helvetica")
				fontCopy[bold] = type1Font("Helvetica-Bold")
				resCopy[k] = fontCopy
			} else {
				resCopy[k] = overlayFonts
			}
			continue
		}
		resCopy[k] = copier.copyObject(v)
	}

	if _, exists := resCopy["Font"]; !exists {
		resCopy["Font"] = overlayFonts
	}

	return resCopy
}

func type1Font(baseFont string) pdf.Dict {
	return pdf.Dict{
		"Type":     pdf.Name("Font"),
		"Subtype":  pdf.Name("Type1"),
		"BaseFont": pdf.Name(baseFont),
		"Encoding": pdf.Name("WinAnsiEncoding"),
	}
}

// MergePages merges multiple single-page PDFs into one output file.
func MergePages(pagePaths []string, outputPath string) error {
	if len(pagePaths) == 0 {
		return fmt.Errorf("no pages to merge")
	}

	if len(pagePaths) == 1 {
		data, err := os.ReadFile(pagePaths[0])
		if err != nil {
			return fmt.Errorf("reading single page: %w", err)
		}
		return os.WriteFile(outputPath, data, 0o600)
	}

	m := pdf.NewMerger()
	for i, p := range pagePaths {
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("reading page %d (%s): %w", i+1, p, err)
		}
		if err := m.Add(data); err != nil {
			return fmt.Errorf("adding page %d to merger: %w", i+1, err)
		}
	}

	result, err := m.Merge()
	if err != nil {
		return fmt.Errorf("merging pages: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	return os.WriteFile(outputPath, result, 0o600)
}

// RenderPageFromImage creates a single-page PDF from a scanned page image
// with translated text overlaid.
func (r *Renderer) RenderPageFromImage(imgData []byte, pageWidth, pageHeight float64, blocks []domain.TextBlock, targetLang string, outputPath string) error {
	c := pdf.NewCreator()
	page := c.NewPage(pageWidth, pageHeight)

	for _, block := range blocks {
		if block.Translated == "" {
			continue
		}

		layout := FitText(block.Translated, block.BBox, block.FontSize)
		isRTL := IsRTL(block.Translated)

		page.SetFont("Helvetica", layout.FontSize)
		page.SetColor(0, 0, 0)

		for i, line := range layout.Lines {
			x := block.BBox.X
			y := block.BBox.Y + block.BBox.Height - float64(i+1)*layout.LineHeight

			if isRTL {
				x = block.BBox.X + block.BBox.Width
			}
			if y < block.BBox.Y {
				break
			}
			page.DrawText(x, y, line)
		}
	}

	data, err := c.Build()
	if err != nil {
		return fmt.Errorf("building page PDF: %w", err)
	}

	return os.WriteFile(outputPath, data, 0o600)
}
