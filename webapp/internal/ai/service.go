package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/yourorg/pdf-translator-webapp/config"
)

const systemEdit = "You are a professional document editor. Edit the text per the instruction. Output only the edited text, preserving markdown."
const systemAsk = "You are a helpful assistant. Answer concisely based on the document."

func StreamBlock(ctx context.Context, blockText, instruction, mode, provider, model string, w io.Writer) error {
	system := systemAsk
	if mode == "edit" {
		system = systemEdit
	}
	userMsg := fmt.Sprintf("Text:\n%s\n\nInstruction: %s", blockText, instruction)

	if provider == "openai" {
		return streamOpenAI(ctx, system, userMsg, model, w)
	}
	return streamClaude(ctx, system, userMsg, model, w)
}

func streamClaude(ctx context.Context, system, userMsg, model string, w io.Writer) error {
	body := map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"stream":     true,
		"system": []map[string]any{
			{"type": "text", "text": system, "cache_control": map[string]string{"type": "ephemeral"}},
		},
		"messages": []map[string]any{{"role": "user", "content": userMsg}},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(string(b)))
	req.Header.Set("x-api-key", config.C.AnthropicAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			for line := range strings.SplitSeq(string(buf[:n]), "\n") {
				if strings.HasPrefix(line, "data: ") {
					var ev map[string]any
					if json.Unmarshal([]byte(line[6:]), &ev) == nil {
						if delta, ok := ev["delta"].(map[string]any); ok {
							if text, ok := delta["text"].(string); ok {
								fmt.Fprintf(w, "data: %s\n\n", text)
							}
						}
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	return nil
}

func streamOpenAI(ctx context.Context, system, userMsg, model string, w io.Writer) error {
	body := map[string]any{
		"model":  model,
		"stream": true,
		"messages": []map[string]any{
			{"role": "system", "content": system},
			{"role": "user", "content": userMsg},
		},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", strings.NewReader(string(b)))
	req.Header.Set("Authorization", "Bearer "+config.C.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(w, resp.Body)
	return nil
}
