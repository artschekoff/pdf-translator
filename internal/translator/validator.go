package translator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// parseAndValidate parses the OpenAI response as a JSON array of strings
// and validates it has the expected length.
//
// GPT-4o-mini sometimes returns objects instead of bare strings, so we
// accept both formats:
//   - ["string1", "string2"]
//   - [{"text":"string1"}, {"text":"string2"}]
func parseAndValidate(content string, expectedCount int) ([]string, error) {
	cleaned := extractJSONArray(content)

	// Try plain string array first.
	var translations []string
	if err := json.Unmarshal([]byte(cleaned), &translations); err == nil {
		return validateTranslations(translations, expectedCount)
	}

	// Try object array: [{"text":"..."}, ...]
	type textObj struct {
		Text string `json:"text"`
	}
	var objects []textObj
	if err := json.Unmarshal([]byte(cleaned), &objects); err == nil {
		translations = make([]string, len(objects))
		for i, o := range objects {
			translations[i] = o.Text
		}
		return validateTranslations(translations, expectedCount)
	}

	// Try object array with id: [{"id":0,"text":"..."}, ...]
	type idTextObj struct {
		ID   int    `json:"id"`
		Text string `json:"text"`
	}
	var idObjects []idTextObj
	if err := json.Unmarshal([]byte(cleaned), &idObjects); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	sort.Slice(idObjects, func(i, j int) bool {
		return idObjects[i].ID < idObjects[j].ID
	})

	translations = make([]string, len(idObjects))
	for i, o := range idObjects {
		translations[i] = o.Text
	}
	return validateTranslations(translations, expectedCount)
}

func validateTranslations(translations []string, expectedCount int) ([]string, error) {
	if len(translations) != expectedCount {
		return nil, fmt.Errorf("expected %d translations, got %d", expectedCount, len(translations))
	}
	return translations, nil
}

// extractJSONArray extracts the JSON array from the response, stripping
// markdown code fences and other wrapping the model might add.
func extractJSONArray(s string) string {
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}

	s = strings.TrimSpace(s)

	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start >= 0 && end > start {
		s = s[start : end+1]
	}

	return s
}

