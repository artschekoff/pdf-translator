package config

import (
	"fmt"
	"os"
	"time"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/spf13/viper"
)

type Config struct {
	OpenAIAPIKey   string        `mapstructure:"OPENAI_API_KEY"`
	PaddleOCRURL   string        `mapstructure:"PADDLE_OCR_URL"`
	TesseractURL   string        `mapstructure:"TESSERACT_URL"`
	OCREngine      string        `mapstructure:"OCR_ENGINE"`
	MaxPageWorkers int           `mapstructure:"MAX_PAGE_WORKERS"`
	DPI            int           `mapstructure:"DPI"`
	DataDir        string        `mapstructure:"DATA_DIR"`
	HTTPTimeout    time.Duration `mapstructure:"HTTP_TIMEOUT"`

	// Per-script TTF font paths. When set, the renderer uses these files
	// directly instead of downloading fonts. A single font file that covers
	// multiple scripts (e.g. NotoSans covers Latin + Cyrillic) can be reused
	// across several entries.
	FontLatin      string `mapstructure:"FONT_LATIN"`
	FontCyrillic   string `mapstructure:"FONT_CYRILLIC"`
	FontArabic     string `mapstructure:"FONT_ARABIC"`
	FontHebrew     string `mapstructure:"FONT_HEBREW"`
	FontDevanagari string `mapstructure:"FONT_DEVANAGARI"`
	FontThai       string `mapstructure:"FONT_THAI"`
	FontKorean     string `mapstructure:"FONT_KOREAN"`
	FontCJK        string `mapstructure:"FONT_CJK"`
}

// ScriptFontPaths returns a map of script name → font file path built from
// the individual FONT_* config entries. Only populated entries are included.
func (c *Config) ScriptFontPaths() map[string]string {
	m := make(map[string]string)
	add := func(script, path string) {
		if path != "" {
			m[script] = path
		}
	}
	add("latin", c.FontLatin)
	add("cyrillic", c.FontCyrillic)
	add("arabic", c.FontArabic)
	add("hebrew", c.FontHebrew)
	add("devanagari", c.FontDevanagari)
	add("thai", c.FontThai)
	add("korean", c.FontKorean)
	add("cjk", c.FontCJK)
	return m
}

func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	viper.SetDefault("PADDLE_OCR_URL", "http://localhost:8051")
	viper.SetDefault("TESSERACT_URL", "http://localhost:8052")
	viper.SetDefault("OCR_ENGINE", domain.OCREnginePaddleOCR)
	viper.SetDefault("MAX_PAGE_WORKERS", 4)
	viper.SetDefault("DPI", 300)
	viper.SetDefault("DATA_DIR", "")
	viper.SetDefault("HTTP_TIMEOUT", 120*time.Second)

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("reading config: %w", err)
			}
		}
	}

	cfg := &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDir()
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.OpenAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required; set it in .env or as an environment variable")
	}
	if c.OCREngine != domain.OCREnginePaddleOCR && c.OCREngine != domain.OCREngineTesseract {
		return fmt.Errorf("OCR_ENGINE must be '%s' or '%s', got %q", domain.OCREnginePaddleOCR, domain.OCREngineTesseract, c.OCREngine)
	}
	if c.MaxPageWorkers < 1 {
		return fmt.Errorf("MAX_PAGE_WORKERS must be >= 1, got %d", c.MaxPageWorkers)
	}
	if c.DPI < 72 || c.DPI > 600 {
		return fmt.Errorf("DPI must be between 72 and 600, got %d", c.DPI)
	}
	return nil
}
