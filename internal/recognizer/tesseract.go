package recognizer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

type tesseractResponse struct {
	Text       string           `json:"text"`
	Confidence float64          `json:"confidence"`
	Boxes      []tesseractBox   `json:"boxes"`
}

type tesseractBox struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
}

// TesseractRecognizer calls a Tesseract HTTP API for text recognition.
type TesseractRecognizer struct {
	baseURL    string
	httpClient *http.Client
}

func NewTesseractRecognizer(baseURL string, timeout time.Duration) *TesseractRecognizer {
	return &TesseractRecognizer{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (r *TesseractRecognizer) Recognize(ctx context.Context, imageData []byte, pageWidth, pageHeight float64, dpi int) ([]domain.TextBlock, error) {
	buf, contentType, err := buildMultipartImage(imageData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/ocr", buf)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Tesseract: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimitedBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Tesseract returned status %d: %s", resp.StatusCode, domain.TruncateString(string(body), 512))
	}

	var result tesseractResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing Tesseract response: %w", err)
	}

	return convertTesseractResults(result.Boxes, dpi), nil
}

func convertTesseractResults(boxes []tesseractBox, dpi int) []domain.TextBlock {
	blocks := make([]domain.TextBlock, 0, len(boxes))

	for _, b := range boxes {
		if b.Text == "" {
			continue
		}

		pdfX := PixelToPDFPoint(b.X, dpi)
		pdfY := PixelToPDFPoint(b.Y, dpi)
		pdfW := PixelToPDFPoint(b.Width, dpi)
		pdfH := PixelToPDFPoint(b.Height, dpi)

		estimatedFontSize, blockType := ClassifyBlock(pdfH)

		blocks = append(blocks, domain.TextBlock{
			BBox: domain.BoundingBox{
				X:      pdfX,
				Y:      pdfY,
				Width:  pdfW,
				Height: pdfH,
			},
			Text:      b.Text,
			FontSize:  estimatedFontSize,
			BlockType: blockType,
		})
	}

	return blocks
}

func (r *TesseractRecognizer) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrOCRServiceDown, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", domain.ErrOCRServiceDown, resp.StatusCode)
	}
	return nil
}
