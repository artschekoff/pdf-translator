package renderer

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/razvandimescu/gopdf/pdf"
	"go.uber.org/zap"
)

const (
	mdPageW       = 595.0
	mdPageH       = 842.0
	mdMarginL     = 56.0
	mdMarginR     = 56.0
	mdMarginT     = 56.0
	mdMarginB     = 56.0
	mdBodySize    = 11.0
	mdH1Size      = 20.0
	mdH2Size      = 16.0
	mdH3Size      = 13.0
	mdLineSpacing = 1.4
)

// MarkdownRenderer renders translated Markdown text to a multi-page A4 PDF.
type MarkdownRenderer struct {
	fontManager *FontManager
	logger      *zap.Logger
}

func NewMarkdownRenderer(fontManager *FontManager, logger *zap.Logger) *MarkdownRenderer {
	return &MarkdownRenderer{fontManager: fontManager, logger: logger}
}

// RenderMarkdown renders markdown to a PDF file at outputPath.
func (r *MarkdownRenderer) RenderMarkdown(ctx context.Context, markdown, targetLang, outputPath string) error {
	script := mdScriptForText(markdown)
	ttfFont := r.mdLoadFont(ctx, script)

	usedRunes := mdRunesFromString(markdown)

	w := pdf.NewWriter()
	catalogRef := w.AllocRef()
	pagesRef := w.AllocRef()

	var (
		ttfRef        pdf.Ref
		hasTTF        bool
		fontResources pdf.Dict
	)
	if ttfFont != nil {
		ref, err := ttfFont.EmbedInPDF(w, usedRunes)
		if err != nil {
			r.logger.Warn("TTF embed failed, falling back to Helvetica", zap.Error(err))
		} else {
			ttfRef = ref
			hasTTF = true
		}
	}
	if hasTTF {
		fontResources = pdf.Dict{"F1": ttfRef}
	} else {
		fontResources = pdf.Dict{"F1": type1Font("Helvetica")}
	}

	elements := parseMDElements(markdown)
	pages := layoutMDPages(elements, ttfFont, hasTTF, mdPageW, mdPageH, mdMarginL, mdMarginR, mdMarginT, mdMarginB)

	var pageRefs []pdf.Ref
	for _, pg := range pages {
		contentRef := w.AllocRef()
		if err := w.WriteStream(contentRef, pdf.Dict{}, pg); err != nil {
			return fmt.Errorf("writing page stream: %w", err)
		}
		pageRef := w.AllocRef()
		if err := w.WriteObject(pageRef, pdf.Dict{
			"Type":      pdf.Name("Page"),
			"Parent":    pagesRef,
			"MediaBox":  pdf.Array{0, 0, mdPageW, mdPageH},
			"Contents":  contentRef,
			"Resources": pdf.Dict{"Font": fontResources},
		}); err != nil {
			return fmt.Errorf("writing page dict: %w", err)
		}
		pageRefs = append(pageRefs, pageRef)
	}

	if len(pageRefs) == 0 {
		contentRef := w.AllocRef()
		_ = w.WriteStream(contentRef, pdf.Dict{}, []byte(""))
		pageRef := w.AllocRef()
		_ = w.WriteObject(pageRef, pdf.Dict{
			"Type":     pdf.Name("Page"),
			"Parent":   pagesRef,
			"MediaBox": pdf.Array{0, 0, mdPageW, mdPageH},
			"Contents": contentRef,
		})
		pageRefs = append(pageRefs, pageRef)
	}

	kids := make(pdf.Array, len(pageRefs))
	for i, ref := range pageRefs {
		kids[i] = ref
	}
	if err := w.WriteObject(pagesRef, pdf.Dict{
		"Type":  pdf.Name("Pages"),
		"Kids":  kids,
		"Count": len(pageRefs),
	}); err != nil {
		return fmt.Errorf("writing pages: %w", err)
	}
	if err := w.WriteObject(catalogRef, pdf.Dict{
		"Type":  pdf.Name("Catalog"),
		"Pages": pagesRef,
	}); err != nil {
		return fmt.Errorf("writing catalog: %w", err)
	}

	data, err := w.Finish(catalogRef)
	if err != nil {
		return fmt.Errorf("building PDF: %w", err)
	}
	return os.WriteFile(outputPath, data, 0o600)
}

type mdElement struct {
	text     string
	fontSize float64
	isHR     bool
	isEmpty  bool
}

var (
	mdHeadingRE = regexp.MustCompile(`^(#{1,6})\s+(.*)`)
	mdHRRE      = regexp.MustCompile(`^(-{3,}|\*{3,}|_{3,})$`)
	mdImgRE     = regexp.MustCompile(`<!--\s*IMG_\d+\s*-->|!\[[^\]]*\]\([^)]*\)`)
)

func parseMDElements(md string) []mdElement {
	var elems []mdElement
	scanner := bufio.NewScanner(strings.NewReader(md))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var para strings.Builder

	stripInline := func(s string) string {
		s = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(s, "$1")
		s = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(s, "$1")
		s = regexp.MustCompile("`([^`]+)`").ReplaceAllString(s, "$1")
		return s
	}

	flush := func() {
		t := strings.TrimSpace(para.String())
		t = mdImgRE.ReplaceAllString(t, "")
		t = stripInline(strings.TrimSpace(t))
		if t != "" {
			elems = append(elems, mdElement{text: t, fontSize: mdBodySize})
		}
		para.Reset()
	}

	for scanner.Scan() {
		line := scanner.Text()

		if m := mdHeadingRE.FindStringSubmatch(line); m != nil {
			flush()
			level := len(m[1])
			size := mdH3Size
			if level == 1 {
				size = mdH1Size
			} else if level == 2 {
				size = mdH2Size
			}
			text := strings.TrimSpace(mdImgRE.ReplaceAllString(m[2], ""))
			if text != "" {
				elems = append(elems, mdElement{text: text, fontSize: size})
			}
			continue
		}

		if mdHRRE.MatchString(strings.TrimSpace(line)) {
			flush()
			elems = append(elems, mdElement{isHR: true})
			continue
		}

		if strings.TrimSpace(line) == "" {
			flush()
			elems = append(elems, mdElement{isEmpty: true})
			continue
		}

		if para.Len() > 0 {
			para.WriteByte(' ')
		}
		para.WriteString(line)
	}
	flush()
	return elems
}

func layoutMDPages(elems []mdElement, ttf *TTFFont, hasTTF bool, pageW, pageH, mL, mR, mT, mB float64) [][]byte {
	textW := pageW - mL - mR
	var pages [][]byte
	var buf strings.Builder
	curY := mT

	newPage := func() {
		if buf.Len() > 0 {
			pages = append(pages, []byte(buf.String()))
		}
		buf.Reset()
		curY = mT
	}

	drawText := func(line string, x, y, size float64) {
		pdfY := pageH - y - size
		if hasTTF && ttf != nil {
			hex := ttf.EncodeTextHex(line)
			fmt.Fprintf(&buf, "BT /F1 %.1f Tf %.2f %.2f Td <%s> Tj ET\n", size, x, pdfY, hex)
		} else {
			fmt.Fprintf(&buf, "BT /F1 %.1f Tf %.2f %.2f Td (%s) Tj ET\n", size, x, pdfY, escapeStringPDF(line))
		}
	}

	drawHR := func(y float64) {
		pdfY := pageH - y - 1
		fmt.Fprintf(&buf, "0.5 w %.2f %.2f m %.2f %.2f l S\n", mL, pdfY, pageW-mR, pdfY)
	}

	for _, el := range elems {
		if el.isEmpty {
			curY += mdBodySize * mdLineSpacing * 0.5
			if curY > pageH-mB {
				newPage()
			}
			continue
		}

		if el.isHR {
			if curY+mdBodySize > pageH-mB {
				newPage()
			}
			drawHR(curY)
			curY += mdBodySize * mdLineSpacing
			continue
		}

		lines := mdWrapText(el.text, textW, ttf, el.fontSize)
		lineH := el.fontSize * mdLineSpacing
		blockH := float64(len(lines)) * lineH

		if curY+blockH > pageH-mB && curY > mT {
			newPage()
		}

		for _, line := range lines {
			if curY+el.fontSize > pageH-mB {
				newPage()
			}
			drawText(line, mL, curY, el.fontSize)
			curY += lineH
		}
		curY += lineH * 0.3
	}

	if buf.Len() > 0 {
		pages = append(pages, []byte(buf.String()))
	}
	return pages
}

func mdWrapText(text string, maxWidth float64, ttf *TTFFont, fontSize float64) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	spaceW := mdMeasure(" ", ttf, fontSize)
	var lines []string
	var current strings.Builder
	var currentW float64

	for _, word := range words {
		ww := mdMeasure(word, ttf, fontSize)
		if current.Len() == 0 {
			current.WriteString(word)
			currentW = ww
		} else if currentW+spaceW+ww <= maxWidth {
			current.WriteByte(' ')
			current.WriteString(word)
			currentW += spaceW + ww
		} else {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(word)
			currentW = ww
		}
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func mdMeasure(text string, ttf *TTFFont, fontSize float64) float64 {
	if ttf != nil {
		return ttf.TextWidth(text, fontSize)
	}
	return float64(len([]rune(text))) * fontSize * 0.5
}

func mdScriptForText(text string) string {
	cyrillic, arabic, cjk := 0, 0, 0
	count := 0
	for _, r := range text {
		if count > 200 {
			break
		}
		switch {
		case r >= 0x0400 && r <= 0x04FF:
			cyrillic++
		case (r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F):
			arabic++
		case r >= 0x4E00 && r <= 0x9FFF:
			cjk++
		}
		count++
	}
	switch {
	case cyrillic > 0:
		return domain.ScriptCyrillic
	case arabic > 0:
		return domain.ScriptArabic
	case cjk > 0:
		return domain.ScriptCJK
	default:
		return domain.ScriptLatin
	}
}

func (r *MarkdownRenderer) mdLoadFont(ctx context.Context, script string) *TTFFont {
	path, err := r.fontManager.FontPath(ctx, script)
	if err != nil || path == "" {
		return nil
	}
	f, err := LoadTTFFont(path)
	if err != nil {
		r.logger.Warn("failed to load TTF for markdown renderer", zap.String("script", script), zap.Error(err))
		return nil
	}
	return f
}

func mdRunesFromString(s string) []rune {
	seen := make(map[rune]struct{})
	for _, r := range s {
		seen[r] = struct{}{}
	}
	runes := make([]rune, 0, len(seen))
	for r := range seen {
		runes = append(runes, r)
	}
	return runes
}
