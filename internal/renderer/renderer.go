package renderer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/razvandimescu/gopdf/pdf"
	"go.uber.org/zap"
)

type Renderer struct {
	fontManager *FontManager
	logger      *zap.Logger

	fontCacheMu sync.RWMutex
	fontCache   map[string]*TTFFont // script → loaded font (nil = unavailable)
}

func NewRenderer(dataDir string, logger *zap.Logger) *Renderer {
	return NewRendererWithFontPaths(dataDir, nil, logger)
}

func NewRendererWithFontPaths(dataDir string, fontPaths map[string]string, logger *zap.Logger) *Renderer {
	return &Renderer{
		fontManager: NewFontManagerWithOverrides(dataDir, fontPaths),
		logger:      logger,
		fontCache:   make(map[string]*TTFFont),
	}
}

// RenderPage re-flows the page content into a new single-page PDF that
// renders translated text in place of the originals while preserving
// graphical regions (diagrams, figures) intact.
//
// Layout within the output page (top → bottom):
//   - Text segments: translated text only, original source text hidden.
//   - Diagram segments (large vertical gaps between text groups):
//     shown twice — first the original (source language preserved),
//     then a translated copy (original vector/image content + translated
//     text labels overlaid).
//
// Font: Type0/CIDFont (Identity-H) when a TTF is available; Helvetica
// (WinAnsiEncoding, Latin-only) as a fallback.
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

	pageW, pageH := pageMediaBox(srcPage)

	script := dominantScript(blocks)
	ttfFont := r.getFontForScript(ctx, script)

	fontOverlayRegular := pdf.Name("F_overlay")
	fontOverlayBold := pdf.Name("F_overlay_b")

	// Partition the page into text and diagram segments.
	segments := buildSegments(blocks, pageH, pageW)

	// Preserve the original page's top and bottom margins so the output
	// looks like the source document (not wall-to-wall text).
	topMargin := 0.0
	bottomMargin := 0.0
	if len(segments) > 0 {
		if m := pageH - segments[0].renderHigh; m > 0 {
			topMargin = m
		}
		if m := segments[len(segments)-1].renderLow; m > 0 {
			bottomMargin = m
		}
	}

	// Calculate total output page height including preserved margins.
	totalH := topMargin + bottomMargin
	for _, seg := range segments {
		totalH += seg.outputHeight()
	}
	if totalH == 0 {
		totalH = pageH
	}

	// Build combined content stream working top → bottom.
	// outY starts just below the top margin so the first segment lands correctly.
	outY := totalH - topMargin
	var combined []byte

	for _, seg := range segments {
		h := seg.height()
		if seg.isDiagram {
			// ── Original view of diagram ──────────────────────────────────────
			outYLow := outY - h
			deltaY := outYLow - seg.renderLow // shift: original coords → output coords

			if len(existingContent) > 0 {
				combined = append(combined, clipAndDrawOriginal(existingContent, seg.renderLow, seg.height(), pageW, deltaY)...)
			}
			outY = outYLow - diagramSectionGap

			// ── Translated view of diagram ────────────────────────────────────
			outYLow = outY - h
			deltaY = outYLow - seg.renderLow

			if len(existingContent) > 0 {
				combined = append(combined, clipAndDrawOriginal(existingContent, seg.renderLow, seg.height(), pageW, deltaY)...)
			}
			// Overlay: white rects + translated text for blocks in this segment.
			combined = append(combined, r.renderBlockOverlay(seg.blocks, deltaY, ttfFont, fontOverlayRegular, fontOverlayBold, pageNum)...)
			outY = outYLow

		} else {
			// ── Text segment ──────────────────────────────────────────────────
			// 1. White background for the whole strip (prevents original text
			//    bleed-through and avoids graphics-state leakage from the
			//    original content stream).
			// 2. Re-render original content only for formula blocks (math/symbol
			//    fonts that cannot be reproduced with a standard TTF).
			// 3. Translated-text overlay for non-formula blocks.
			outYLow := outY - h
			deltaY := outYLow - seg.yLow
			combined = append(combined, fmt.Sprintf("q 1 1 1 rg 0 %.4f %.4f %.4f re f Q\n", outYLow, pageW, h)...)
			if len(existingContent) > 0 {
				for _, b := range seg.blocks {
					if isFormulaBlock(b.Text) {
						combined = append(combined, clipAndDrawOriginal(existingContent, b.BBox.Y, b.BBox.Height, pageW, deltaY)...)
						combined = append(combined, "/GS_overlay gs\n"...)
					}
				}
			}
			combined = append(combined, r.renderBlockOverlay(seg.blocks, deltaY, ttfFont, fontOverlayRegular, fontOverlayBold, pageNum)...)
			outY = outYLow
		}
	}

	w := pdf.NewWriter()
	pagesRef := w.AllocRef()
	catalogRef := w.AllocRef()
	copier := newPageCopier(reader, w)

	var ttfFontRef pdf.Ref
	if ttfFont != nil {
		ref, embedErr := ttfFont.EmbedInPDF(w)
		if embedErr != nil {
			r.logger.Warn("TTF embed failed, falling back to Helvetica",
				zap.Int("page", pageNum),
				zap.Error(embedErr),
			)
			ttfFont = nil
		} else {
			ttfFontRef = ref
		}
	}

	copiedPage := make(pdf.Dict)
	for k, v := range srcPage {
		if k == "Parent" || k == "Contents" || k == "MediaBox" || k == "Resources" {
			continue
		}
		copiedPage[k] = copier.copyObject(v)
	}
	copiedPage["Parent"] = pagesRef
	copiedPage["MediaBox"] = pdf.Array{0, 0, pageW, totalH}

	overlayFonts := buildOverlayFonts(fontOverlayRegular, fontOverlayBold, ttfFont, ttfFontRef)
	copiedPage["Resources"] = buildResources(reader, copier, srcPage, overlayFonts)

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

// clipAndDrawOriginal renders existingContent shifted by deltaY, clipping to the
// strip [yLow, yLow+height] in the ORIGINAL coordinate space.
//
// Order: shift first (cm), then clip in the now-shifted space (which equals
// original page coords). This ensures the clip rectangle aligns with the content.
func clipAndDrawOriginal(existingContent []byte, yLow, height, pageW, deltaY float64) []byte {
	var b []byte
	b = append(b, fmt.Sprintf("q 1 0 0 1 0 %.4f cm\n", deltaY)...)
	b = append(b, fmt.Sprintf("q 0 %.4f %.4f %.4f re W n\n", yLow, pageW, height)...)
	b = append(b, existingContent...)
	// Balance any unmatched q's from existingContent so our two Q's correctly
	// close only our own q's, regardless of the original stream structure.
	if extra := countNetQDepth(existingContent); extra > 0 {
		for range extra {
			b = append(b, "Q\n"...)
		}
	}
	b = append(b, "\nQ\nQ\n"...)
	return b
}

// renderBlockOverlay returns a PDF content stream fragment that covers each
// block's bounding box with a white rectangle and writes the translated text.
// deltaY is added to each block's Y coordinate to shift it to its output position.
//
// Structure: single q/Q scope with /GS_overlay gs to reset blend mode and
// opacity; all white rects in one pass; each block's text in a single BT…ET
// with relative Td between lines so PDF viewers can do character-level selection.
func (r *Renderer) renderBlockOverlay(blocks []domain.TextBlock, deltaY float64, ttfFont *TTFFont, fontOverlayRegular, fontOverlayBold pdf.Name, pageNum int) []byte {
	var out strings.Builder

	out.WriteString("q /GS_overlay gs\n")

	// Pass 1: white cover rectangles (skip formula and empty blocks).
	out.WriteString("1 1 1 rg\n")
	for _, block := range blocks {
		if strings.TrimSpace(block.Translated) == "" || isFormulaBlock(block.Text) {
			continue
		}
		layout := FitText(block.Translated, block.BBox, block.FontSize)
		boxHeight := block.BBox.Height
		if layout.BoxHeight > boxHeight {
			boxHeight = layout.BoxHeight
		}
		padX := block.FontSize * 0.3
		padY := block.FontSize * 0.5
		fmt.Fprintf(&out, "%.2f %.2f %.2f %.2f re f\n",
			block.BBox.X-padX,
			block.BBox.Y+deltaY-padY,
			block.BBox.Width+2*padX,
			boxHeight+2*padY)
	}

	// Pass 2: translated text — one BT…ET per block so the viewer can track
	// character positions for copy-paste and selection within the block.
	for _, block := range blocks {
		if strings.TrimSpace(block.Translated) == "" || isFormulaBlock(block.Text) {
			continue
		}

		layout := FitText(block.Translated, block.BBox, block.FontSize)
		isRTL := IsRTL(block.Translated)

		fontName := fontOverlayRegular
		if block.BlockType == domain.BlockTypeTitle {
			fontName = fontOverlayBold
		}

		// Collect lines within the block's vertical bounds.
		type rendLine struct {
			text string
			y    float64
		}
		var lines []rendLine
		boxHeight := block.BBox.Height
		if layout.BoxHeight > boxHeight {
			boxHeight = layout.BoxHeight
		}
		for i, line := range layout.Lines {
			y := block.BBox.Y + deltaY + boxHeight - float64(i+1)*layout.LineHeight
			if y < block.BBox.Y+deltaY-block.FontSize*0.5 {
				r.logger.Warn("text overflow",
					zap.Int("page", pageNum),
					zap.Int("textLen", len([]rune(block.Translated))),
				)
				break
			}
			lines = append(lines, rendLine{line, y})
		}
		if len(lines) == 0 {
			continue
		}

		x := block.BBox.X
		if isRTL {
			x = block.BBox.X + block.BBox.Width
		}

		// Open a single text object for all lines of this block.
		fmt.Fprintf(&out, "BT 0 0 0 rg /%s %.1f Tf %.2f %.2f Td\n",
			fontName, layout.FontSize, x, lines[0].y)

		for i, ln := range lines {
			var pdfStr string
			if ttfFont != nil {
				pdfStr = fmt.Sprintf("<%s>", ttfFont.EncodeTextHex(ln.text))
			} else {
				pdfStr = fmt.Sprintf("(%s)", escapeStringPDF(ln.text))
			}
			if i == 0 {
				fmt.Fprintf(&out, "%s Tj\n", pdfStr)
			} else {
				// Relative move: same x, step down by one line height.
				fmt.Fprintf(&out, "0 %.2f Td %s Tj\n", -layout.LineHeight, pdfStr)
			}
		}
		out.WriteString("ET\n")
	}

	out.WriteString("Q\n")
	return []byte(out.String())
}

// pageMediaBox returns the width and height of a page's MediaBox.
// Falls back to US letter (612×792) if the entry is missing or malformed.
func pageMediaBox(page pdf.Dict) (w, h float64) {
	arr, ok := page.Array("MediaBox")
	if !ok || len(arr) < 4 {
		return 612, 792
	}
	x1 := pdfFloat(arr[0])
	y1 := pdfFloat(arr[1])
	x2 := pdfFloat(arr[2])
	y2 := pdfFloat(arr[3])
	return x2 - x1, y2 - y1
}

// isFormulaBlock returns true when the block's source text contains box/square
// placeholder characters (U+25A1, U+25A0 …) that appear when a PDF glyph has
// no Unicode mapping — typical of mathematical / symbol fonts.  Such blocks
// cannot be re-rendered with a standard TTF; the original content must show.
func isFormulaBlock(text string) bool {
	for _, r := range text {
		if r == '□' || r == '■' || r == '◻' || r == '◼' || r == '�' {
			return true
		}
	}
	return false
}

// countNetQDepth counts (standalone "q" lines) − (standalone "Q" lines) in a
// PDF content stream.  Used to balance the graphics-state stack when wrapping
// existingContent inside our own q/Q pairs.
func countNetQDepth(content []byte) int {
	depth := 0
	for _, line := range bytes.Split(content, []byte("\n")) {
		t := bytes.TrimSpace(line)
		if len(t) == 1 {
			if t[0] == 'q' {
				depth++
			} else if t[0] == 'Q' {
				depth--
			}
		}
	}
	return depth
}

func pdfFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

// dominantScript returns the most common non-Latin script in the translated
// blocks, or "latin" if all text is Latin.
func dominantScript(blocks []domain.TextBlock) string {
	counts := make(map[string]int)
	for _, b := range blocks {
		s := DetectScript(b.Translated)
		counts[s]++
	}
	best := "latin"
	bestN := 0
	for _, s := range scriptPriority {
		if n := counts[s]; n > bestN {
			bestN = n
			best = s
		}
	}
	return best
}

// getFontForScript loads (and caches) a TTFFont for the given script.
// Returns nil if no font is available, triggering Helvetica fallback.
func (r *Renderer) getFontForScript(ctx context.Context, script string) *TTFFont {
	r.fontCacheMu.RLock()
	f, seen := r.fontCache[script]
	r.fontCacheMu.RUnlock()
	if seen {
		return f // may be nil (cached miss)
	}

	path, err := r.fontManager.FontPath(ctx, script)
	if err != nil {
		r.logger.Warn("font unavailable, using Helvetica fallback",
			zap.String("script", script),
			zap.Error(err),
		)
		r.storeFontCache(script, nil)
		return nil
	}

	loaded, err := LoadTTFFont(path)
	if err != nil {
		r.logger.Warn("failed to parse TTF font",
			zap.String("path", path),
			zap.Error(err),
		)
		r.storeFontCache(script, nil)
		return nil
	}

	r.logger.Info("loaded TTF font",
		zap.String("script", script),
		zap.String("path", path),
		zap.String("name", loaded.name),
	)
	r.storeFontCache(script, loaded)
	return loaded
}

func (r *Renderer) storeFontCache(script string, f *TTFFont) {
	r.fontCacheMu.Lock()
	r.fontCache[script] = f
	r.fontCacheMu.Unlock()
}

// buildOverlayFonts returns a pdf.Dict mapping overlay font names to their
// font dictionaries. Uses embedded TTF when available, Helvetica otherwise.
func buildOverlayFonts(regular, bold pdf.Name, ttfFont *TTFFont, ttfRef pdf.Ref) pdf.Dict {
	if ttfFont != nil {
		// Use the same embedded TTF for both regular and bold slots.
		return pdf.Dict{
			regular: ttfRef,
			bold:    ttfRef,
		}
	}
	return pdf.Dict{
		regular: type1Font("Helvetica"),
		bold:    type1Font("Helvetica-Bold"),
	}
}

// buildResources deep-copies the source page's Resources dict and injects the
// overlay fonts and a GS_overlay ExtGState (Normal blend, full opacity) into
// the appropriate sub-dicts.
func buildResources(reader *pdf.Reader, copier *pageCopier, srcPage pdf.Dict, overlayFonts pdf.Dict) pdf.Dict {
	// GS_overlay resets blend mode and opacity before our overlay draws so that
	// graphics-state leakage from the original content stream does not cause
	// white background rectangles to render as black in PDF viewers.
	overlayGS := pdf.Dict{
		"Type": pdf.Name("ExtGState"),
		"BM":   pdf.Name("Normal"),
		"ca":   float64(1.0),
		"CA":   float64(1.0),
	}

	srcRes, ok := reader.ResolveDict(srcPage["Resources"])
	if !ok || srcRes == nil {
		return pdf.Dict{
			"Font":      overlayFonts,
			"ExtGState": pdf.Dict{"GS_overlay": overlayGS},
		}
	}

	resCopy := make(pdf.Dict, len(srcRes))
	for k, v := range srcRes {
		switch k {
		case "Font":
			srcFontDict, ok := reader.ResolveDict(v)
			if ok && srcFontDict != nil {
				fontCopy := make(pdf.Dict, len(srcFontDict)+len(overlayFonts))
				for fk, fv := range srcFontDict {
					fontCopy[fk] = copier.copyObject(fv)
				}
				for fk, fv := range overlayFonts {
					fontCopy[fk] = fv
				}
				resCopy[k] = fontCopy
			} else {
				resCopy[k] = overlayFonts
			}
		case "ExtGState":
			srcGS, ok := reader.ResolveDict(v)
			gsCopy := make(pdf.Dict)
			if ok && srcGS != nil {
				for gk, gv := range srcGS {
					gsCopy[gk] = copier.copyObject(gv)
				}
			}
			gsCopy["GS_overlay"] = overlayGS
			resCopy[k] = gsCopy
		default:
			resCopy[k] = copier.copyObject(v)
		}
	}
	if _, exists := resCopy["Font"]; !exists {
		resCopy["Font"] = overlayFonts
	}
	if _, exists := resCopy["ExtGState"]; !exists {
		resCopy["ExtGState"] = pdf.Dict{"GS_overlay": overlayGS}
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
