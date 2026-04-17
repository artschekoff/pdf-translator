package detector

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/razvandimescu/gopdf/pdf"
)

const nativeTextThreshold = 20

// Detector checks PDF page types and counts pages.
type Detector struct{}

func New() *Detector {
	return &Detector{}
}

// NOTE: the password parameter is accepted for interface compatibility but the
// underlying gopdf library does not currently support decrypting protected
// PDFs. Encrypted files will surface ErrPDFEncrypted.
func (d *Detector) CountPages(ctx context.Context, inputPath string, password string) (int, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	doc, err := pdf.OpenFile(inputPath)
	if err != nil {
		if domain.IsPDFEncryptedError(err) {
			return 0, domain.ErrPDFEncrypted
		}
		return 0, fmt.Errorf("opening PDF: %w", err)
	}

	return doc.NumPages(), nil
}

func (d *Detector) DetectPageType(ctx context.Context, inputPath string, password string, pageNum int) (domain.PageType, error) {
	if ctx.Err() != nil {
		return domain.PageTypeScanned, ctx.Err()
	}

	doc, err := pdf.OpenFile(inputPath)
	if err != nil {
		if domain.IsPDFEncryptedError(err) {
			return domain.PageTypeScanned, domain.ErrPDFEncrypted
		}
		return domain.PageTypeScanned, fmt.Errorf("opening PDF: %w", err)
	}

	return classifyPage(doc, pageNum)
}

// CountAndClassifyAll opens the PDF once and returns the total page count
// plus per-page type classification. This avoids reopening the file for
// every page when both pieces of information are needed.
func (d *Detector) CountAndClassifyAll(ctx context.Context, inputPath string, password string) (int, []domain.PageType, error) {
	if ctx.Err() != nil {
		return 0, nil, ctx.Err()
	}

	doc, err := pdf.OpenFile(inputPath)
	if err != nil {
		if domain.IsPDFEncryptedError(err) {
			return 0, nil, domain.ErrPDFEncrypted
		}
		return 0, nil, fmt.Errorf("opening PDF: %w", err)
	}

	n := doc.NumPages()
	types := make([]domain.PageType, n)
	for i := range n {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		pt, err := classifyPage(doc, i+1)
		if err != nil {
			return 0, nil, fmt.Errorf("classifying page %d: %w", i+1, err)
		}
		types[i] = pt
	}

	return n, types, nil
}

func classifyPage(doc *pdf.Document, pageNum int) (domain.PageType, error) {
	pageIndex := pageNum - 1
	if pageIndex < 0 || pageIndex >= doc.NumPages() {
		return domain.PageTypeScanned, domain.ErrPageOutOfRange
	}

	page := doc.Page(pageIndex)
	text, err := page.Text()
	if err != nil {
		return domain.PageTypeScanned, nil
	}

	trimmed := strings.TrimSpace(text)
	if utf8.RuneCountInString(trimmed) >= nativeTextThreshold {
		return domain.PageTypeNative, nil
	}

	return domain.PageTypeScanned, nil
}
