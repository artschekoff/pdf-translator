package extractor

import (
	"context"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

// Extractor extracts text blocks with positions from a single PDF page.
type Extractor interface {
	ExtractPage(ctx context.Context, inputPath string, password string, pageNum int, dpi int) ([]domain.TextBlock, error)
}
