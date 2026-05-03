package extractor

import (
	"bytes"
	"context"
	"fmt"
	"image/png"

	"github.com/gen2brain/go-fitz"
	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/pdf-translator/pdf-translator/internal/recognizer"
)

// LayoutExtractor renders a PDF page to an image and sends it to a layout-analysis service.
type LayoutExtractor struct {
	analyzer recognizer.LayoutAnalyzer
}

func NewLayoutExtractor(analyzer recognizer.LayoutAnalyzer) *LayoutExtractor {
	return &LayoutExtractor{analyzer: analyzer}
}

func (e *LayoutExtractor) ExtractPage(ctx context.Context, inputPath string, password string, pageNum int, dpi int) ([]domain.TextBlock, error) {
	doc, err := fitz.New(inputPath)
	if err != nil {
		return nil, fmt.Errorf("opening PDF with MuPDF: %w", err)
	}
	defer doc.Close()

	pageIndex := pageNum - 1
	if pageIndex < 0 || pageIndex >= doc.NumPage() {
		return nil, domain.ErrPageOutOfRange
	}

	img, err := doc.ImageDPI(pageIndex, float64(dpi))
	if err != nil {
		return nil, fmt.Errorf("rendering page %d at %d DPI: %w", pageNum, dpi, err)
	}

	imgBounds := img.Bounds()
	pageWidthPt := recognizer.PixelToPDFPoint(float64(imgBounds.Dx()), dpi)
	pageHeightPt := recognizer.PixelToPDFPoint(float64(imgBounds.Dy()), dpi)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encoding page %d as PNG: %w", pageNum, err)
	}

	blocks, err := e.analyzer.Analyze(ctx, buf.Bytes(), pageWidthPt, pageHeightPt, dpi)
	if err != nil {
		return nil, fmt.Errorf("layout analysis on page %d: %w", pageNum, err)
	}

	for i := range blocks {
		blocks[i].PageNum = pageNum
	}

	return blocks, nil
}
