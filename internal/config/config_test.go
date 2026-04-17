package config

import (
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestConfig_Validate_MissingAPIKey(t *testing.T) {
	cfg := &Config{
		OCREngine:      domain.OCREnginePaddleOCR,
		MaxPageWorkers: 4,
		DPI:            300,
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
}

func TestConfig_Validate_InvalidOCREngine(t *testing.T) {
	cfg := &Config{
		OpenAIAPIKey:   "sk-test",
		OCREngine:      "invalid",
		MaxPageWorkers: 4,
		DPI:            300,
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OCR_ENGINE")
}

func TestConfig_Validate_InvalidWorkers(t *testing.T) {
	cfg := &Config{
		OpenAIAPIKey:   "sk-test",
		OCREngine:      domain.OCREnginePaddleOCR,
		MaxPageWorkers: 0,
		DPI:            300,
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MAX_PAGE_WORKERS")
}

func TestConfig_Validate_InvalidDPI(t *testing.T) {
	cfg := &Config{
		OpenAIAPIKey:   "sk-test",
		OCREngine:      domain.OCREnginePaddleOCR,
		MaxPageWorkers: 4,
		DPI:            50,
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DPI")
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := &Config{
		OpenAIAPIKey:   "sk-test",
		OCREngine:      domain.OCREnginePaddleOCR,
		MaxPageWorkers: 4,
		DPI:            300,
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfig_Validate_Tesseract(t *testing.T) {
	cfg := &Config{
		OpenAIAPIKey:   "sk-test",
		OCREngine:      domain.OCREngineTesseract,
		MaxPageWorkers: 1,
		DPI:            150,
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestDefaultDataDir(t *testing.T) {
	dir := defaultDataDir()
	assert.NotEmpty(t, dir)
	assert.Contains(t, dir, ".pdf-translator")
}
