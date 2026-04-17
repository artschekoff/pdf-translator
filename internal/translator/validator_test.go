package translator

import (
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAndValidate_ValidJSON(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		count    int
		expected []string
	}{
		{
			name:     "simple array",
			content:  `["Hola mundo", "Capítulo 1"]`,
			count:    2,
			expected: []string{"Hola mundo", "Capítulo 1"},
		},
		{
			name:     "with markdown code fence",
			content:  "```json\n[\"Hello\", \"World\"]\n```",
			count:    2,
			expected: []string{"Hello", "World"},
		},
		{
			name:     "with extra text around JSON",
			content:  "Here is the translation:\n[\"Bonjour\"]\nDone.",
			count:    1,
			expected: []string{"Bonjour"},
		},
		{
			name:     "single item",
			content:  `["Translated text"]`,
			count:    1,
			expected: []string{"Translated text"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseAndValidate(tt.content, tt.count)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseAndValidate_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		content string
		count   int
	}{
		{
			name:    "wrong count",
			content: `["one", "two"]`,
			count:   3,
		},
		{
			name:    "not JSON",
			content: "just plain text",
			count:   1,
		},
		{
			name:    "empty array with expected count",
			content: `[]`,
			count:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAndValidate(tt.content, tt.count)
			assert.Error(t, err)
		})
	}
}

func TestParseAndValidate_ObjectArray(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		count    int
		expected []string
	}{
		{
			name:     "objects with text field",
			content:  `[{"text":"Hello"},{"text":"World"}]`,
			count:    2,
			expected: []string{"Hello", "World"},
		},
		{
			name:     "objects with id and text",
			content:  `[{"id":0,"text":"Hello"},{"id":1,"text":"World"}]`,
			count:    2,
			expected: []string{"Hello", "World"},
		},
		{
			name:     "allows empty strings",
			content:  `["hello", ""]`,
			count:    2,
			expected: []string{"hello", ""},
		},
		{
			name:     "allows whitespace strings",
			content:  `[" ", "hello"]`,
			count:    2,
			expected: []string{" ", "hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseAndValidate(tt.content, tt.count)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractJSONArray(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`["a","b"]`, `["a","b"]`},
		{"```json\n[\"a\"]\n```", `["a"]`},
		{"```\n[\"a\"]\n```", `["a"]`},
		{"prefix [\"a\"] suffix", `["a"]`},
	}

	for _, tt := range tests {
		result := extractJSONArray(tt.input)
		assert.Equal(t, tt.expected, result, "input: %q", tt.input)
	}
}

func TestEstimateCost(t *testing.T) {
	t.Run("nil blocks", func(t *testing.T) {
		inputTokens, outputTokens, cost := EstimateCost(nil)
		assert.Equal(t, 0, inputTokens)
		assert.Equal(t, 0, outputTokens)
		assert.Equal(t, 0.0, cost)
	})

	t.Run("with blocks", func(t *testing.T) {
		blocks := []domain.TextBlock{
			{Text: "Hello world this is a test"},
			{Text: "Another block of text here"},
		}
		inputTokens, outputTokens, cost := EstimateCost(blocks)
		assert.Greater(t, inputTokens, 0)
		assert.Greater(t, outputTokens, 0)
		assert.Greater(t, cost, 0.0)
	})

	t.Run("empty text blocks", func(t *testing.T) {
		blocks := []domain.TextBlock{{Text: ""}, {Text: ""}}
		inputTokens, outputTokens, cost := EstimateCost(blocks)
		assert.Equal(t, 0, inputTokens)
		assert.Equal(t, 0, outputTokens)
		assert.Equal(t, 0.0, cost)
	})
}
