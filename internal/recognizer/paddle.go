package recognizer

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

type paddleResponse struct {
	Success    bool            `json:"success"`
	Text       string          `json:"text"`
	Confidence float64         `json:"confidence"`
	Details    []paddleDetail  `json:"details"`
}

type paddleDetail struct {
	Text       string      `json:"text"`
	Confidence float64     `json:"confidence"`
	BBox       [][]float64 `json:"bbox"` // [[x1,y1],[x2,y2],[x3,y3],[x4,y4]]
}

// PaddleRecognizer calls the PaddleOCR HTTP API for text recognition.
type PaddleRecognizer struct {
	baseURL    string
	httpClient *http.Client
}

func NewPaddleRecognizer(baseURL string, timeout time.Duration) *PaddleRecognizer {
	return &PaddleRecognizer{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (r *PaddleRecognizer) Recognize(ctx context.Context, imageData []byte, pageWidth, pageHeight float64, dpi int) ([]domain.TextBlock, error) {
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
		return nil, fmt.Errorf("calling PaddleOCR: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimitedBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PaddleOCR returned status %d: %s", resp.StatusCode, domain.TruncateString(string(body), 512))
	}

	var result paddleResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing PaddleOCR response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("PaddleOCR reported failure")
	}

	return convertPaddleResults(result.Details, dpi), nil
}

func convertPaddleResults(details []paddleDetail, dpi int) []domain.TextBlock {
	blocks := make([]domain.TextBlock, 0, len(details))

	for _, d := range details {
		if len(d.BBox) < 4 || d.Text == "" {
			continue
		}
		if len(d.BBox[0]) < 2 {
			continue
		}

		minX, minY := d.BBox[0][0], d.BBox[0][1]
		maxX, maxY := d.BBox[0][0], d.BBox[0][1]
		for _, pt := range d.BBox {
			if len(pt) < 2 {
				continue
			}
			minX = math.Min(minX, pt[0])
			minY = math.Min(minY, pt[1])
			maxX = math.Max(maxX, pt[0])
			maxY = math.Max(maxY, pt[1])
		}

		pdfX := PixelToPDFPoint(minX, dpi)
		pdfY := PixelToPDFPoint(minY, dpi)
		pdfW := PixelToPDFPoint(maxX-minX, dpi)
		pdfH := PixelToPDFPoint(maxY-minY, dpi)

		estimatedFontSize, blockType := ClassifyBlock(pdfH)

		blocks = append(blocks, domain.TextBlock{
			BBox: domain.BoundingBox{
				X:      pdfX,
				Y:      pdfY,
				Width:  pdfW,
				Height: pdfH,
			},
			Text:      d.Text,
			FontSize:  estimatedFontSize,
			BlockType: blockType,
		})
	}

	return blocks
}

// HealthCheck verifies the PaddleOCR service is reachable.
func (r *PaddleRecognizer) HealthCheck(ctx context.Context) error {
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
