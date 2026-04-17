package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRTL(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{"english", "Hello world", false},
		{"arabic", "مرحبا بالعالم", true},
		{"hebrew", "שלום עולם", true},
		{"mixed mostly english", "Hello مرحبا world test", false},
		{"mixed mostly arabic", "مرحبا hello بالعالم test العربي", true},
		{"empty", "", false},
		{"numbers only", "12345", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsRTL(tt.text))
		})
	}
}

func TestDetectScript(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{"english", "Hello world", "latin"},
		{"arabic", "مرحبا بالعالم", "arabic"},
		{"hebrew", "שלום עולם", "hebrew"},
		{"cyrillic", "Привет мир", "cyrillic"},
		{"chinese", "你好世界", "cjk"},
		{"korean", "안녕하세요", "korean"},
		{"thai", "สวัสดีชาวโลก", "thai"},
		{"devanagari", "नमस्ते दुनिया", "devanagari"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, DetectScript(tt.text))
		})
	}
}
