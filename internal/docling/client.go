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

type ConvertResponse struct {
	Markdown string `json:"markdown"`
	Pages    int    `json:"pages"`
}

func (c *Client) Convert(ctx context.Context, pdfPath string, req ConvertRequest) (*ConvertResponse, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("opening PDF: %w", err)
	}
	defer f.Close()

	fw, err := w.CreateFormFile("file", "document.pdf")
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return nil, fmt.Errorf("copying PDF: %w", err)
	}

	settingsJSON, _ := json.Marshal(req)
	if err := w.WriteField("settings", string(settingsJSON)); err != nil {
		return nil, fmt.Errorf("writing settings: %w", err)
	}
	w.Close()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/convert", &body)
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

	var result ConvertResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
