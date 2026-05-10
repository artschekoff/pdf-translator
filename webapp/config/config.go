package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	Port string `mapstructure:"PORT"`

	DatabaseURL string `mapstructure:"DATABASE_URL"`

	RedisURL string `mapstructure:"REDIS_URL"`

	AuthServiceURL string `mapstructure:"AUTH_SERVICE_URL"`

	QuotaOCRDailyLimit    int `mapstructure:"QUOTA_OCR_DAILY_LIMIT"`
	QuotaAIDailyLimit     int `mapstructure:"QUOTA_AI_DAILY_LIMIT"`
	QuotaExportDailyLimit int `mapstructure:"QUOTA_EXPORT_DAILY_LIMIT"`

	UploadsDir string `mapstructure:"UPLOADS_DIR"`
	ExportsDir string `mapstructure:"EXPORTS_DIR"`

	DockingServiceURL string `mapstructure:"DOCLING_SERVICE_URL"`
	AnthropicAPIKey   string `mapstructure:"ANTHROPIC_API_KEY"`
	OpenAIAPIKey      string `mapstructure:"OPENAI_API_KEY"`

	PaymentsServiceURL string `mapstructure:"PAYMENTS_SERVICE_URL"`

	MdtoPdfScript string `mapstructure:"MDTOPDF_SCRIPT"`

	WorkerConcurrency int `mapstructure:"WORKER_CONCURRENCY"`
}

var C Config

func Load() {
	viper.AutomaticEnv()
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("UPLOADS_DIR", "./uploads")
	viper.SetDefault("EXPORTS_DIR", "./exports")
	viper.SetDefault("WORKER_CONCURRENCY", 5)
	viper.SetDefault("QUOTA_OCR_DAILY_LIMIT", 0)
	viper.SetDefault("QUOTA_AI_DAILY_LIMIT", 0)
	viper.SetDefault("QUOTA_EXPORT_DAILY_LIMIT", 0)
	viper.SetDefault("PAYMENTS_SERVICE_URL", "http://localhost:8003")
	viper.SetDefault("MDTOPDF_SCRIPT", "/Users/riskyworks/.scripts/pdf/mdtopdf.sh")

	if err := viper.Unmarshal(&C); err != nil {
		log.Fatalf("config: %v", err)
	}
}
