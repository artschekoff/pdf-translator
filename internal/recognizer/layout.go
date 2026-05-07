package recognizer

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

type paddleLayoutResponse struct {
	Success bool                `json:"success"`
	Regions []paddleLayoutBlock `json:"regions"`
}

type paddleLayoutBlock struct {
	Label      string    `json:"label"`
	Score      float64   `json:"score"`
	Coordinate []float64 `json:"coordinate"`
}

// PaddleLayoutAnalyzer calls the Paddle layout-analysis HTTP API.
type PaddleLayoutAnalyzer struct {
	baseURL    string
	httpClient *http.Client
}

func NewPaddleLayoutAnalyzer(baseURL string, timeout time.Duration) *PaddleLayoutAnalyzer {
	return &PaddleLayoutAnalyzer{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (r *PaddleLayoutAnalyzer) Analyze(ctx context.Context, imageData []byte, pageWidth, pageHeight float64, dpi int) ([]domain.TextBlock, error) {
	buf, contentType, err := buildMultipartImage(imageData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/layout", buf)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Paddle layout analysis: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimitedBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Paddle layout analysis returned status %d: %s", resp.StatusCode, domain.TruncateString(string(body), 512))
	}

	var result paddleLayoutResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing Paddle layout response: %w", err)
	}
	if !result.Success {
		return nil, fmt.Errorf("Paddle layout analysis reported failure")
	}

	return convertPaddleLayoutResults(result.Regions, pageHeight, dpi), nil
}

func convertPaddleLayoutResults(regions []paddleLayoutBlock, pageHeight float64, dpi int) []domain.TextBlock {
	blocks := make([]domain.TextBlock, 0, len(regions))

	for _, region := range regions {
		if len(region.Coordinate) < 4 {
			continue
		}

		minX := region.Coordinate[0]
		minY := region.Coordinate[1]
		maxX := region.Coordinate[2]
		maxY := region.Coordinate[3]
		if maxX <= minX || maxY <= minY {
			continue
		}

		pdfX := PixelToPDFPoint(minX, dpi)
		pdfW := PixelToPDFPoint(maxX-minX, dpi)
		pdfH := PixelToPDFPoint(maxY-minY, dpi)
		pdfY := pageHeight - PixelToPDFPoint(maxY, dpi)

		blocks = append(blocks, domain.TextBlock{
			BBox: domain.BoundingBox{
				X:      pdfX,
				Y:      pdfY,
				Width:  pdfW,
				Height: pdfH,
			},
			FontSize:  math.Max(pdfH*0.75, 1),
			BlockType: MapPaddleLayoutLabel(region.Label),
		})
	}

	return blocks
}

func MapPaddleLayoutLabel(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "doc_title", "paragraph_title", "title":
		return domain.BlockTypeTitle
	case "header", "page_header":
		return domain.BlockTypeHeader
	case "footer", "page_footer", "page_number", "footnote", "vision_footnote":
		return domain.BlockTypeFooter
	case "figure_title", "table_title", "chart_title", "caption":
		return domain.BlockTypeCaption
	case "table":
		return domain.BlockTypeTable
	case "image", "figure", "chart", "seal", "formula", "equation":
		return domain.BlockTypeImage
	default:
		return domain.BlockTypeText
	}
}
