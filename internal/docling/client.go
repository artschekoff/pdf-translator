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

const pollInterval = 5 * time.Second

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

type ConvertRequest struct {
	DoOcr            bool    `json:"do_ocr"`
	DoTableStructure bool    `json:"do_table_structure"`
	ImagesScale      float64 `json:"images_scale"`
}

type taskStatusResponse struct {
	TaskID     string  `json:"task_id"`
	TaskStatus string  `json:"task_status"`
	ErrorMsg   *string `json:"error_message"`
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

// Convert submits a PDF for async conversion and polls until complete.
func (c *Client) Convert(ctx context.Context, pdfPath string, req ConvertRequest) (*ConvertResponse, error) {
	body, contentType, err := c.buildMultipart(pdfPath, req)
	if err != nil {
		return nil, err
	}

	// Submit async job
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/convert/file/async", body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("docling async submit: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docling: status %d: %s", resp.StatusCode, b)
	}

	var taskResp taskStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		return nil, fmt.Errorf("decoding task response: %w", err)
	}

	// Poll until done
	taskID := taskResp.TaskID
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}

		status, err := c.pollStatus(ctx, taskID)
		if err != nil {
			return nil, err
		}
		switch status.TaskStatus {
		case "success", "partial_success":
			return c.fetchResult(ctx, taskID)
		case "failure":
			msg := "conversion failed"
			if status.ErrorMsg != nil {
				msg = *status.ErrorMsg
			}
			return nil, fmt.Errorf("docling: %s", msg)
		}
		// pending / started — keep polling
	}
}

func (c *Client) pollStatus(ctx context.Context, taskID string) (*taskStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/status/poll/"+taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("building poll request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("polling status: %w", err)
	}
	defer resp.Body.Close()

	var status taskStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decoding poll response: %w", err)
	}
	return &status, nil
}

func (c *Client) fetchResult(ctx context.Context, taskID string) (*ConvertResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/result/"+taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("building result request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching result: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docling result: status %d: %s", resp.StatusCode, b)
	}

	var apiResp convertAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decoding result: %w", err)
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

func (c *Client) buildMultipart(pdfPath string, req ConvertRequest) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, "", fmt.Errorf("opening PDF: %w", err)
	}
	defer f.Close()

	fw, err := w.CreateFormFile("files", "document.pdf")
	if err != nil {
		return nil, "", fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return nil, "", fmt.Errorf("copying PDF: %w", err)
	}

	for field, val := range map[string]string{
		"do_ocr":             boolStr(req.DoOcr),
		"do_table_structure": boolStr(req.DoTableStructure),
		"image_export_mode":  "placeholder",
		"to_formats":         "md",
	} {
		if err := w.WriteField(field, val); err != nil {
			return nil, "", fmt.Errorf("writing field %s: %w", field, err)
		}
	}

	w.Close()
	return &body, w.FormDataContentType(), nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
