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
	Engine                string   `json:"engine"`
	Lang                  []string `json:"lang"`
	DoOcr                 bool     `json:"do_ocr"`
	DoTableStructure      bool     `json:"do_table_structure"`
	TableMode             string   `json:"table_mode"`
	GeneratePictureImages bool     `json:"generate_picture_images"`
	ImagesScale           float64  `json:"images_scale"`
}

type ConvertResponse struct {
	Markdown string            `json:"markdown"`
	Images   map[string]string `json:"images"`
	Pages    int               `json:"pages"`
}

func (c *Client) Convert(ctx context.Context, pdfPath string, req ConvertRequest) (*ConvertResponse, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fw, _ := w.CreateFormFile("file", "document.pdf")
	if _, err := io.Copy(fw, f); err != nil {
		return nil, err
	}

	settingsJSON, _ := json.Marshal(req)
	w.WriteField("settings", string(settingsJSON))
	w.Close()

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/convert", &body)
	httpReq.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("docling: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docling: status %d: %s", resp.StatusCode, b)
	}

	var result ConvertResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
