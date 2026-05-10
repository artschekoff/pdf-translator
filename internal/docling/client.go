package docling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 600 * time.Second},
	}
}

type ConvertRequest struct {
	DoOcr            bool    `json:"do_ocr"`
	DoTableStructure bool    `json:"do_table_structure"`
	ImagesScale      float64 `json:"images_scale"`
}

type convertAPIResponse struct {
	Document struct {
		MdContent *string `json:"md_content"`
	} `json:"document"`
	Status string `json:"status"`
	Errors []struct {
		Message string `json:"error_message"`
	} `json:"errors"`
}

type ConvertResponse struct {
	Markdown string
}

func (c *Client) Convert(ctx context.Context, pdfPath string, req ConvertRequest) (*ConvertResponse, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("opening PDF: %w", err)
	}
	defer f.Close()

	fw, err := w.CreateFormFile("files", "document.pdf")
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return nil, fmt.Errorf("copying PDF: %w", err)
	}

	// Individual flat form fields (not JSON-encoded settings)
	if err := w.WriteField("do_ocr", boolStr(req.DoOcr)); err != nil {
		return nil, fmt.Errorf("writing do_ocr: %w", err)
	}
	if err := w.WriteField("do_table_structure", boolStr(req.DoTableStructure)); err != nil {
		return nil, fmt.Errorf("writing do_table_structure: %w", err)
	}
	// Use placeholder mode so base64 image data is not embedded in markdown
	if err := w.WriteField("image_export_mode", "placeholder"); err != nil {
		return nil, fmt.Errorf("writing image_export_mode: %w", err)
	}
	if err := w.WriteField("to_formats", "md"); err != nil {
		return nil, fmt.Errorf("writing to_formats: %w", err)
	}

	w.Close()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/convert/file", &body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("docling request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docling: status %d: %s", resp.StatusCode, b)
	}

	var apiResp convertAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if apiResp.Status == "failure" {
		msg := "conversion failed"
		if len(apiResp.Errors) > 0 {
			msg = apiResp.Errors[0].Message
		}
		return nil, fmt.Errorf("docling: %s", msg)
	}

	if apiResp.Document.MdContent == nil {
		return nil, fmt.Errorf("docling: no markdown content in response")
	}

	return &ConvertResponse{Markdown: *apiResp.Document.MdContent}, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
