package domain

import (
	"errors"
	"strings"
)

var (
	ErrPDFEncrypted       = errors.New("PDF is password-protected; use --password to provide the decryption password")
	ErrPDFInvalid         = errors.New("file is not a valid PDF")
	ErrPageOutOfRange     = errors.New("page number out of range")
	ErrNoTextFound        = errors.New("no extractable text found on page")
	ErrOCRServiceDown     = errors.New("OCR service is not reachable")
	ErrTranslationFailed  = errors.New("translation failed after all retries")
	ErrInvalidPageRange   = errors.New("invalid page range format; expected e.g. '1-5' or '3'")
	ErrOutputExists       = errors.New("output file already exists; use a different name or remove it")
	ErrFontNotFound       = errors.New("required font file not found and download failed")
	ErrMaxRetriesExceeded = errors.New("maximum retries exceeded for page processing")
)

// IsPDFEncryptedError heuristically checks whether the error from a PDF
// library indicates the file is password-protected. Centralised here so
// callers don't duplicate fragile string matching.
func IsPDFEncryptedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "password") || strings.Contains(msg, "encrypt")
}

// TruncateString truncates s to at most n runes, appending "..." if truncated.
func TruncateString(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
