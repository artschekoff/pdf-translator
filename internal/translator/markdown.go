package translator

import (
	"context"
	"fmt"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const (
	markdownSep        = "\n\n<<<SEP>>>\n\n"
	defaultMDBatchSize = 4000
)

// TranslateMarkdownSections translates a slice of Markdown sections to targetLang.
// Sections are sent in batches joined by <<<SEP>>>, preserving Markdown syntax.
// Pass batchSize=0 to use the default (4000 chars per batch).
func (t *Translator) TranslateMarkdownSections(ctx context.Context, sections []string, sourceLang, targetLang string, batchSize int) ([]string, error) {
	if len(sections) == 0 {
		return nil, nil
	}
	if batchSize <= 0 {
		batchSize = defaultMDBatchSize
	}

	batches := mdBatchByChars(sections, batchSize)
	result := make([]string, 0, len(sections))

	for _, batch := range batches {
		translated, err := t.translateMarkdownBatch(ctx, batch, sourceLang, targetLang)
		if err != nil {
			return nil, fmt.Errorf("translating markdown batch: %w", err)
		}
		result = append(result, translated...)
	}
	return result, nil
}

func mdBatchByChars(sections []string, maxChars int) [][]string {
	var batches [][]string
	var current []string
	size := 0
	for _, s := range sections {
		if len(current) > 0 && size+len(s) > maxChars {
			batches = append(batches, current)
			current = nil
			size = 0
		}
		current = append(current, s)
		size += len(s)
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

func (t *Translator) translateMarkdownBatch(ctx context.Context, sections []string, sourceLang, targetLang string) ([]string, error) {
	combined := strings.Join(sections, markdownSep)

	from := sourceLang
	if from == "auto" {
		from = "the source language (auto-detect)"
	}
	sysPrompt := fmt.Sprintf(
		`You are a professional translator. Translate the following Markdown text from %s to %s.
Rules:
- The text contains multiple segments separated by <<<SEP>>>. Preserve ALL <<<SEP>>> separators exactly as-is.
- Preserve Markdown syntax: #, ##, **, *, -, |, backticks.
- Translate heading text but keep the # prefix.
- Translate table cell content but keep the | structure.
- Do not translate code blocks (backtick fences).
- Do not modify image placeholders like <!-- IMG_0 --> or <!-- image -->.
- Output only the translated Markdown with separators intact.`, from, targetLang)

	var lastErr error
	backoff := initialBackoff
	for attempt := 0; attempt <= maxBatchRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
			}
		}

		resp, err := t.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: modelName,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: sysPrompt},
				{Role: openai.ChatMessageRoleUser, Content: combined},
			},
			Temperature: 0.3,
		})
		if err != nil {
			lastErr = err
			continue
		}
		if len(resp.Choices) == 0 {
			lastErr = fmt.Errorf("empty response from OpenAI")
			continue
		}

		content := resp.Choices[0].Message.Content
		parts := strings.Split(content, "<<<SEP>>>")
		result := make([]string, len(sections))
		for i := range sections {
			if i < len(parts) {
				result[i] = strings.TrimSpace(parts[i])
			}
		}
		return result, nil
	}
	return nil, fmt.Errorf("markdown batch translation failed: %w", lastErr)
}
