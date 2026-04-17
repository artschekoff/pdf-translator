package extractor

import (
	"context"
	"fmt"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/razvandimescu/gopdf/pdf"
)

type NativeExtractor struct{}

func NewNativeExtractor() *NativeExtractor {
	return &NativeExtractor{}
}

func (e *NativeExtractor) ExtractPage(ctx context.Context, inputPath string, password string, pageNum int, _ int) ([]domain.TextBlock, error) {
	doc, err := pdf.OpenFile(inputPath)
	if err != nil {
		if domain.IsPDFEncryptedError(err) {
			return nil, domain.ErrPDFEncrypted
		}
		return nil, fmt.Errorf("opening PDF: %w", err)
	}

	pageIndex := pageNum - 1
	if pageIndex < 0 || pageIndex >= doc.NumPages() {
		return nil, domain.ErrPageOutOfRange
	}

	page := doc.Page(pageIndex)
	lines, err := page.TextLines()
	if err != nil {
		return nil, fmt.Errorf("extracting text lines from page %d: %w", pageNum, err)
	}

	converted := make([]textLine, 0, len(lines))
	for _, l := range lines {
		if len(l.Spans) == 0 {
			continue
		}

		minX := l.Spans[0].X
		maxX := l.Spans[0].EndX
		fontName := l.Spans[0].Font

		for _, s := range l.Spans {
			if s.X < minX {
				minX = s.X
			}
			if s.EndX > maxX {
				maxX = s.EndX
			}
		}

		// gopdf reports the raw Tf font size, which ignores CTM and Tm scaling.
		// PDFs often use transforms like `0.24 0 0 0.24 0 640 cm` + `46 0 0 46 x y Tm`
		// giving a raw Tf of 1 but a visual size of ~11pt. We estimate the real
		// effective size from the actual rendered span width.
		fontSize := effectiveFontSize(l.Spans)

		converted = append(converted, textLine{
			Text:     l.Text,
			X:        minX,
			Y:        l.Y,
			Width:    maxX - minX,
			Height:   fontSize * 1.2,
			FontSize: fontSize,
			FontName: fontName,
		})
	}

	blocks := groupLines(converted, pageNum)
	return blocks, nil
}

// effectiveFontSize estimates the visual font size from span dimensions.
// The library's TextSpan.FontSize only reflects the raw Tf value, ignoring
// CTM and text matrix scaling. We compute the actual size from the rendered
// character widths: avgCharWidth ≈ 0.5 * fontSize for typical fonts.
func effectiveFontSize(spans []pdf.TextSpan) float64 {
	if len(spans) == 0 {
		return 12
	}

	var totalWidth float64
	var totalChars int

	for _, s := range spans {
		n := len([]rune(s.Text))
		if s.EndX > s.X && n > 0 {
			totalWidth += s.EndX - s.X
			totalChars += n
		}
	}

	rawSize := spans[0].FontSize
	if totalChars == 0 || totalWidth <= 0 {
		if rawSize > 0 {
			return rawSize
		}
		return 12
	}

	avgCharWidth := totalWidth / float64(totalChars)
	estimated := avgCharWidth / 0.5

	if estimated > rawSize {
		return estimated
	}
	return rawSize
}
