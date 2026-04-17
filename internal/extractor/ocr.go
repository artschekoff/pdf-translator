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

// OCRExtractor renders a PDF page to an image using MuPDF and sends it
// to an OCR service for text recognition. Used for scanned (image-based) pages.
type OCRExtractor struct {
	recognizer recognizer.TextRecognizer
}

func NewOCRExtractor(rec recognizer.TextRecognizer) *OCRExtractor {
	return &OCRExtractor{recognizer: rec}
}

func (e *OCRExtractor) ExtractPage(ctx context.Context, inputPath string, password string, pageNum int, dpi int) ([]domain.TextBlock, error) {
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

	blocks, err := e.recognizer.Recognize(ctx, buf.Bytes(), pageWidthPt, pageHeightPt, dpi)
	if err != nil {
		return nil, fmt.Errorf("OCR on page %d: %w", pageNum, err)
	}

	for i := range blocks {
		blocks[i].PageNum = pageNum
	}

	if len(blocks) == 0 {
		return nil, domain.ErrNoTextFound
	}

	return blocks, nil
}
