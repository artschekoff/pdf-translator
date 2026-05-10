package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	openai "github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

const (
	modelName          = "gpt-4o-mini"
	maxBatchRetries    = 2
	initialBackoff     = 2 * time.Second
	tokensPerWord      = 1.33 // rough approximation
	inputPricePerMTok  = 0.15  // $0.15 per 1M input tokens
	outputPricePerMTok = 0.60  // $0.60 per 1M output tokens
)

// Translator handles text translation via OpenAI API.
type Translator struct {
	client *openai.Client
	logger *zap.Logger
}

func New(apiKey string, logger *zap.Logger) *Translator {
	client := openai.NewClient(apiKey)
	return &Translator{
		client: client,
		logger: logger,
	}
}

// TranslateBlocks translates a batch of text blocks from sourceLang to targetLang.
// Returns blocks with the Translated field populated.
// Whitespace-only blocks are passed through without calling the API.
func (t *Translator) TranslateBlocks(ctx context.Context, blocks []domain.TextBlock, sourceLang, targetLang string) ([]domain.TextBlock, error) {
	if len(blocks) == 0 {
		return blocks, nil
	}

	result := make([]domain.TextBlock, len(blocks))
	copy(result, blocks)

	// Separate substantive blocks from whitespace-only blocks.
	var substantive []int
	for i, b := range result {
		if strings.TrimSpace(b.Text) == "" {
			result[i].Translated = b.Text
		} else {
			substantive = append(substantive, i)
		}
	}

	if len(substantive) == 0 {
		return result, nil
	}

	subBlocks := make([]domain.TextBlock, len(substantive))
	for i, idx := range substantive {
		subBlocks[i] = result[idx]
	}

	translated, err := t.batchTranslate(ctx, subBlocks, sourceLang, targetLang)
	if err != nil {
		t.logger.Warn("batch translation failed, falling back to per-block",
			zap.Error(err),
			zap.Int("blocks", len(subBlocks)),
		)
		translated, err = t.perBlockFallback(ctx, subBlocks, sourceLang, targetLang)
		if err != nil {
			return nil, err
		}
	}

	for i, idx := range substantive {
		result[idx].Translated = translated[i].Translated
	}

	return result, nil
}

func (t *Translator) batchTranslate(ctx context.Context, blocks []domain.TextBlock, sourceLang, targetLang string) ([]domain.TextBlock, error) {
	prompt, err := buildBatchPrompt(blocks)
	if err != nil {
		return nil, fmt.Errorf("building batch prompt: %w", err)
	}

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
				{Role: openai.ChatMessageRoleSystem, Content: systemPrompt(sourceLang, targetLang)},
				{Role: openai.ChatMessageRoleUser, Content: prompt},
			},
			Temperature: 0.3,
		})
		if err != nil {
			lastErr = fmt.Errorf("OpenAI API call: %w", err)
			continue
		}

		if len(resp.Choices) == 0 {
			lastErr = fmt.Errorf("empty response from OpenAI")
			continue
		}

		content := resp.Choices[0].Message.Content
		translations, err := parseAndValidate(content, len(blocks))
		if err != nil {
			lastErr = fmt.Errorf("response validation: %w", err)
			t.logger.Warn("batch response invalid, retrying",
				zap.Error(err),
				zap.Int("attempt", attempt+1),
			)
			continue
		}

		result := make([]domain.TextBlock, len(blocks))
		copy(result, blocks)
		for i := range result {
			result[i].Translated = translations[i]
		}
		return result, nil
	}

	return nil, fmt.Errorf("batch translation failed after %d attempts: %w", maxBatchRetries+1, lastErr)
}

func (t *Translator) perBlockFallback(ctx context.Context, blocks []domain.TextBlock, sourceLang, targetLang string) ([]domain.TextBlock, error) {
	result := make([]domain.TextBlock, len(blocks))
	copy(result, blocks)

	for i, block := range result {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		translated, err := t.translateSingle(ctx, block.Text, sourceLang, targetLang, block.BlockType)
		if err != nil {
			return nil, fmt.Errorf("translating block %d: %w", i, err)
		}
		result[i].Translated = translated
	}

	return result, nil
}

func (t *Translator) translateSingle(ctx context.Context, text, sourceLang, targetLang, blockType string) (string, error) {
	userMsg := text
	if blockType == domain.BlockTypeTable {
		userMsg = "This is a table cell. Preserve row/line structure exactly.\n\n" + text
	}

	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt <= maxBatchRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
			}
		}

		resp, err := t.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: modelName,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: singleBlockSystemPrompt(sourceLang, targetLang)},
				{Role: openai.ChatMessageRoleUser, Content: userMsg},
			},
			Temperature: 0.3,
		})
		if err != nil {
			lastErr = err
			continue
		}

		if len(resp.Choices) == 0 {
			lastErr = fmt.Errorf("empty response")
			continue
		}

		return strings.TrimSpace(resp.Choices[0].Message.Content), nil
	}

	return "", fmt.Errorf("single block translation failed: %w", lastErr)
}

func systemPrompt(sourceLang, targetLang string) string {
	from := sourceLang
	if from == "auto" {
		from = "the source language (auto-detect)"
	}
	return fmt.Sprintf(`You are a professional document translator. Translate text from %s to %s.

Rules:
- Return ONLY a JSON array of translated strings, one per input block, in the same order.
- Preserve formatting: numbering, bullet points. For table blocks, preserve the row/line structure exactly.
- Do NOT translate proper nouns, code, URLs, or email addresses.
- Do NOT add explanations, notes, or markdown formatting.
- Output each block as a single continuous paragraph — do NOT insert line breaks.
- You MUST return exactly the same number of items as the input array.`, from, targetLang)
}

func singleBlockSystemPrompt(sourceLang, targetLang string) string {
	from := sourceLang
	if from == "auto" {
		from = "the source language (auto-detect)"
	}
	return fmt.Sprintf(`You are a professional document translator. Translate the following text from %s to %s.

Rules:
- Return ONLY the translated text, nothing else.
- Preserve formatting: numbering, bullet points.
- Output as a single continuous paragraph — do NOT insert line breaks.
- Do NOT translate proper nouns, code, URLs, or email addresses.`, from, targetLang)
}

func buildBatchPrompt(blocks []domain.TextBlock) (string, error) {
	type inputBlock struct {
		ID   int    `json:"id"`
		Text string `json:"text"`
		Type string `json:"type,omitempty"`
	}

	items := make([]inputBlock, len(blocks))
	for i, b := range blocks {
		text := b.Text
		// Collapse PDF line-boundary \n to spaces for non-table blocks.
		// Table blocks intentionally preserve row structure via \n.
		if b.BlockType != domain.BlockTypeTable {
			text = strings.Join(strings.Fields(text), " ")
		}
		items[i] = inputBlock{
			ID:   i,
			Text: text,
			Type: b.BlockType,
		}
	}

	data, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("serializing blocks: %w", err)
	}
	return fmt.Sprintf("Translate these %d blocks. Return a JSON array with exactly %d translated strings:\n\n%s",
		len(blocks), len(blocks), string(data)), nil
}

// EstimateCost returns the estimated token count and USD cost for a set of blocks.
func EstimateCost(blocks []domain.TextBlock) (inputTokens int, outputTokens int, costUSD float64) {
	var totalWords int
	for _, b := range blocks {
		totalWords += len(strings.Fields(b.Text))
	}

	inputTokens = int(float64(totalWords) * tokensPerWord)
	// Assume output is ~1.2x input for translation (different languages expand/contract)
	outputTokens = int(float64(inputTokens) * 1.2)

	inputCost := float64(inputTokens) / 1_000_000 * inputPricePerMTok
	outputCost := float64(outputTokens) / 1_000_000 * outputPricePerMTok
	costUSD = inputCost + outputCost

	return inputTokens, outputTokens, costUSD
}
