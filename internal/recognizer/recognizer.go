package recognizer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

// maxOCRResponseBytes caps the OCR response body to 50 MiB to prevent OOM.
const maxOCRResponseBytes = 50 << 20

// TextRecognizer extracts text blocks from a page image.
type TextRecognizer interface {
	Recognize(ctx context.Context, imageData []byte, pageWidth, pageHeight float64, dpi int) ([]domain.TextBlock, error)
}

// ClassifyBlock estimates font size from the bounding-box height and picks
// a block type (title vs text). Shared by all OCR recogniser backends.
func ClassifyBlock(pdfHeight float64) (fontSize float64, blockType string) {
	fontSize = pdfHeight * 0.75
	blockType = domain.BlockTypeText
	if fontSize > titleFontSizeThreshold {
		blockType = domain.BlockTypeTitle
	}
	return fontSize, blockType
}

const titleFontSizeThreshold = 16

// PixelToPDFPoint converts pixel coordinates from an image rendered at the
// given DPI to PDF points (1pt = 1/72 inch).
func PixelToPDFPoint(px float64, dpi int) float64 {
	if dpi <= 0 {
		dpi = 72
	}
	return px * 72.0 / float64(dpi)
}

// buildMultipartImage creates a multipart form body with the image data
// attached as a form file field named "image".
func buildMultipartImage(imageData []byte) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("image", "page.png")
	if err != nil {
		return nil, "", fmt.Errorf("creating multipart form: %w", err)
	}
	if _, err := part.Write(imageData); err != nil {
		return nil, "", fmt.Errorf("writing image data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("closing multipart writer: %w", err)
	}
	return &buf, writer.FormDataContentType(), nil
}

// readLimitedBody reads a response body with a size limit.
func readLimitedBody(body io.ReadCloser) ([]byte, error) {
	return io.ReadAll(io.LimitReader(body, maxOCRResponseBytes))
}
