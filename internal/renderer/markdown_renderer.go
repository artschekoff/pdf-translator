package renderer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"os"
	"regexp"
	"strconv"
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
// translateColor sets the text color for paragraphs marked with <!-- TRANS --> (keep-original mode).
// Pass "" or "black" to render all text in black.
func (r *MarkdownRenderer) RenderMarkdown(ctx context.Context, markdown, targetLang, outputPath, translateColor string) error {
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

	cr, cg, cb := parsePDFColor(translateColor)
	elements := parseMDElements(markdown)
	pages := layoutMDPages(elements, ttfFont, hasTTF, mdPageW, mdPageH, mdMarginL, mdMarginR, mdMarginT, mdMarginB, cr, cg, cb)

	var pageRefs []pdf.Ref
	for _, pg := range pages {
		contentRef := w.AllocRef()
		if err := w.WriteStream(contentRef, pdf.Dict{}, pg.stream); err != nil {
			return fmt.Errorf("writing page stream: %w", err)
		}

		resources := pdf.Dict{"Font": fontResources}
		if len(pg.images) > 0 {
			xObjects := pdf.Dict{}
			for _, img := range pg.images {
				imgRef := w.AllocRef()
				if err := w.WriteObject(imgRef, &pdf.Stream{
					Dict: pdf.Dict{
						"Type":             pdf.Name("XObject"),
						"Subtype":          pdf.Name("Image"),
						"Width":            img.width,
						"Height":           img.height,
						"ColorSpace":       pdf.Name("DeviceRGB"),
						"BitsPerComponent": 8,
						"Filter":           pdf.Name("DCTDecode"),
						"Length":           len(img.jpegData),
					},
					Data: img.jpegData,
				}); err != nil {
					return fmt.Errorf("writing image stream: %w", err)
				}
				xObjects[pdf.Name(img.name)] = imgRef
			}
			resources["XObject"] = xObjects
		}

		pageRef := w.AllocRef()
		if err := w.WriteObject(pageRef, pdf.Dict{
			"Type":      pdf.Name("Page"),
			"Parent":    pagesRef,
			"MediaBox":  pdf.Array{0, 0, mdPageW, mdPageH},
			"Contents":  contentRef,
			"Resources": resources,
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
	text          string
	fontSize      float64
	isHR          bool
	isEmpty       bool
	isTranslation bool
	isTable       bool
	tableRows     [][]string
	isImage       bool
	imageJPEG     []byte
	imageW        int
	imageH        int
}

type mdImageResource struct {
	name     string // "Im0", "Im1", etc. — unique within a page
	width    int
	height   int
	jpegData []byte
}

type mdPageContent struct {
	stream []byte
	images []mdImageResource
}

var (
	mdHeadingRE   = regexp.MustCompile(`^(#{1,6})\s+(.*)`)
	mdHRRE        = regexp.MustCompile(`^(-{3,}|\*{3,}|_{3,})$`)
	mdImgRE       = regexp.MustCompile(`<!--\s*(?:IMG_\d+|image)\s*-->|!\[[^\]]*\]\([^)]*\)`)
	mdDataImageRE = regexp.MustCompile(`^!\[[^\]]*\]\(data:image/(jpeg|png);base64,([A-Za-z0-9+/=\r\n]+)\)$`)
)

var mdTransMarkerRE = regexp.MustCompile(`^<!--\s*TRANS\s*-->$`)

func isTableSepRow(line string) bool {
	line = strings.Trim(strings.TrimSpace(line), "|")
	cells := strings.Split(line, "|")
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		for _, r := range strings.TrimSpace(c) {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

func parseTableRow(line string) []string {
	line = strings.Trim(strings.TrimSpace(line), "|")
	cells := strings.Split(line, "|")
	result := make([]string, len(cells))
	for i, c := range cells {
		result[i] = strings.TrimSpace(c)
	}
	return result
}

func parseMDElements(md string) []mdElement {
	var elems []mdElement
	scanner := bufio.NewScanner(strings.NewReader(md))
	scanner.Buffer(make([]byte, 20*1024*1024), 20*1024*1024)
	var para strings.Builder
	var tableLines []string
	nextIsTranslation := false

	stripInline := func(s string) string {
		s = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(s, "$1")
		s = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(s, "$1")
		s = regexp.MustCompile("`([^`]+)`").ReplaceAllString(s, "$1")
		return s
	}

	flushTable := func() {
		if len(tableLines) == 0 {
			return
		}
		var rows [][]string
		for _, tl := range tableLines {
			if isTableSepRow(tl) {
				continue
			}
			row := parseTableRow(tl)
			if len(row) > 0 {
				rows = append(rows, row)
			}
		}
		tableLines = nil
		if len(rows) == 0 {
			return
		}
		isTransl := nextIsTranslation
		nextIsTranslation = false
		elems = append(elems, mdElement{isTable: true, tableRows: rows, isTranslation: isTransl})
	}

	flush := func() {
		t := strings.TrimSpace(para.String())
		para.Reset()
		if mdTransMarkerRE.MatchString(t) {
			nextIsTranslation = true
			return
		}
		t = mdImgRE.ReplaceAllString(t, "")
		t = stripInline(strings.TrimSpace(t))
		if t != "" {
			isTransl := nextIsTranslation
			nextIsTranslation = false
			elems = append(elems, mdElement{text: t, fontSize: mdBodySize, isTranslation: isTransl})
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Table row detection
		if strings.HasPrefix(trimmed, "|") {
			if para.Len() > 0 {
				flush()
			}
			tableLines = append(tableLines, trimmed)
			continue
		} else if len(tableLines) > 0 {
			flushTable()
		}

		// Embedded image: ![alt](data:image/jpeg;base64,...) or .../png;base64,...
		if m := mdDataImageRE.FindStringSubmatch(trimmed); m != nil {
			flush()
			format := m[1]
			b64 := strings.TrimSpace(m[2])
			rawData, err := base64.StdEncoding.DecodeString(b64)
			if err == nil {
				jpegData, iw, ih, err := toJPEGImage(rawData, format)
				if err == nil && iw > 0 && ih > 0 {
					elems = append(elems, mdElement{isImage: true, imageJPEG: jpegData, imageW: iw, imageH: ih})
				}
			}
			continue
		}

		if m := mdHeadingRE.FindStringSubmatch(line); m != nil {
			flush()
			isTransl := nextIsTranslation
			nextIsTranslation = false
			level := len(m[1])
			size := mdH3Size
			if level == 1 {
				size = mdH1Size
			} else if level == 2 {
				size = mdH2Size
			}
			text := strings.TrimSpace(mdImgRE.ReplaceAllString(m[2], ""))
			if text != "" {
				elems = append(elems, mdElement{text: text, fontSize: size, isTranslation: isTransl})
			}
			continue
		}

		if mdHRRE.MatchString(trimmed) {
			flush()
			elems = append(elems, mdElement{isHR: true})
			continue
		}

		if trimmed == "" {
			flush()
			elems = append(elems, mdElement{isEmpty: true})
			continue
		}

		if para.Len() > 0 {
			para.WriteByte(' ')
		}
		para.WriteString(line)
	}

	if len(tableLines) > 0 {
		flushTable()
	} else {
		flush()
	}
	return elems
}

// toJPEGImage decodes rawData (JPEG or PNG) and returns JPEG bytes with dimensions.
// PNG images are converted to JPEG; JPEG images are returned as-is.
func toJPEGImage(rawData []byte, format string) (jpegData []byte, w, h int, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(rawData))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decoding image config: %w", err)
	}
	if format == "jpeg" {
		return rawData, cfg.Width, cfg.Height, nil
	}
	// PNG → decode and re-encode as JPEG (JPEG doesn't support transparency;
	// transparent pixels will appear black, which is acceptable for documents).
	img, err := png.Decode(bytes.NewReader(rawData))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decoding PNG: %w", err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, 0, 0, fmt.Errorf("encoding JPEG: %w", err)
	}
	return buf.Bytes(), cfg.Width, cfg.Height, nil
}

func layoutMDPages(elems []mdElement, ttf *TTFFont, hasTTF bool, pageW, pageH, mL, mR, mT, mB float64, cr, cg, cb float64) []mdPageContent {
	textW := pageW - mL - mR
	var pages []mdPageContent
	var buf strings.Builder
	var curImages []mdImageResource
	curY := mT

	newPage := func() {
		if buf.Len() > 0 {
			pages = append(pages, mdPageContent{stream: []byte(buf.String()), images: curImages})
			curImages = nil
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

		if el.isImage && len(el.imageJPEG) > 0 {
			dispW := float64(el.imageW)
			dispH := float64(el.imageH)
			if dispW > textW {
				scale := textW / dispW
				dispW = textW
				dispH *= scale
			}
			maxH := pageH - mT - mB
			if dispH > maxH {
				scale := maxH / dispH
				dispH = maxH
				dispW *= scale
			}
			if curY+dispH > pageH-mB && curY > mT {
				newPage()
			}
			imgName := fmt.Sprintf("Im%d", len(curImages))
			curImages = append(curImages, mdImageResource{
				name:     imgName,
				width:    el.imageW,
				height:   el.imageH,
				jpegData: el.imageJPEG,
			})
			pdfY := pageH - curY - dispH
			fmt.Fprintf(&buf, "q %.3f 0 0 %.3f %.3f %.3f cm /%s Do Q\n",
				dispW, dispH, mL, pdfY, imgName)
			curY += dispH + mdBodySize*mdLineSpacing*0.5
			continue
		}

		lines := mdWrapText(el.text, textW, ttf, el.fontSize)
		lineH := el.fontSize * mdLineSpacing
		blockH := float64(len(lines)) * lineH

		if curY+blockH > pageH-mB && curY > mT {
			newPage()
		}

		if el.isTable {
			numCols := 0
			for _, row := range el.tableRows {
				if len(row) > numCols {
					numCols = len(row)
				}
			}
			if numCols == 0 {
				continue
			}
			tableFS := mdBodySize * 0.9
			lineH := tableFS * mdLineSpacing
			colW := textW / float64(numCols)

			if el.isTranslation {
				fmt.Fprintf(&buf, "%.3f %.3f %.3f rg\n", cr, cg, cb)
			}
			for rowIdx, row := range el.tableRows {
				if curY+tableFS > pageH-mB {
					newPage()
					if el.isTranslation {
						fmt.Fprintf(&buf, "%.3f %.3f %.3f rg\n", cr, cg, cb)
					}
				}
				for colIdx := 0; colIdx < numCols; colIdx++ {
					cell := ""
					if colIdx < len(row) {
						cell = row[colIdx]
					}
					cell = truncateCellText(cell, colW-4, ttf, tableFS)
					drawText(cell, mL+float64(colIdx)*colW+2, curY, tableFS)
				}
				// Separator line under header row
				if rowIdx == 0 && len(el.tableRows) > 1 {
					sepY := pageH - curY - tableFS - 1
					fmt.Fprintf(&buf, "0.3 w %.2f %.2f m %.2f %.2f l S\n", mL, sepY, mL+textW, sepY)
				}
				curY += lineH
			}
			if el.isTranslation {
				fmt.Fprintf(&buf, "0 0 0 rg\n")
			}
			curY += lineH * 0.3
			continue
		}

		if el.isTranslation {
			fmt.Fprintf(&buf, "%.3f %.3f %.3f rg\n", cr, cg, cb)
		}
		for _, line := range lines {
			if curY+el.fontSize > pageH-mB {
				newPage()
				if el.isTranslation {
					fmt.Fprintf(&buf, "%.3f %.3f %.3f rg\n", cr, cg, cb)
				}
			}
			drawText(line, mL, curY, el.fontSize)
			curY += lineH
		}
		if el.isTranslation {
			fmt.Fprintf(&buf, "0 0 0 rg\n")
		}
		curY += lineH * 0.3
	}

	if buf.Len() > 0 {
		pages = append(pages, mdPageContent{stream: []byte(buf.String()), images: curImages})
	}
	return pages
}

// parsePDFColor parses a color name or #RRGGBB hex string into PDF RGB values (0.0–1.0).
func parsePDFColor(color string) (r, g, b float64) {
	switch strings.ToLower(strings.TrimSpace(color)) {
	case "green", "":
		return 0, 0.50, 0
	case "red":
		return 0.75, 0, 0
	case "blue":
		return 0, 0, 0.70
	case "purple":
		return 0.50, 0, 0.50
	case "orange":
		return 0.85, 0.45, 0
	case "black":
		return 0, 0, 0
	}
	// #RRGGBB
	hex := strings.TrimPrefix(strings.TrimSpace(color), "#")
	if len(hex) == 6 {
		ri, e1 := strconv.ParseInt(hex[0:2], 16, 64)
		gi, e2 := strconv.ParseInt(hex[2:4], 16, 64)
		bi, e3 := strconv.ParseInt(hex[4:6], 16, 64)
		if e1 == nil && e2 == nil && e3 == nil {
			return float64(ri) / 255, float64(gi) / 255, float64(bi) / 255
		}
	}
	return 0, 0.50, 0 // fallback: green
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

func truncateCellText(text string, maxW float64, ttf *TTFFont, fontSize float64) string {
	if mdMeasure(text, ttf, fontSize) <= maxW {
		return text
	}
	runes := []rune(text)
	for i := len(runes) - 1; i > 0; i-- {
		t := string(runes[:i]) + "…"
		if mdMeasure(t, ttf, fontSize) <= maxW {
			return t
		}
	}
	return ""
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
