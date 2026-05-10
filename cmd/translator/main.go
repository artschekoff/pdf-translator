package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/pdf-translator/pdf-translator/internal/config"
	"github.com/pdf-translator/pdf-translator/internal/detector"
	"github.com/pdf-translator/pdf-translator/internal/docling"
	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/pdf-translator/pdf-translator/internal/extractor"
	"github.com/pdf-translator/pdf-translator/internal/pipeline"
	"github.com/pdf-translator/pdf-translator/internal/queue"
	"github.com/pdf-translator/pdf-translator/internal/recognizer"
	"github.com/pdf-translator/pdf-translator/internal/renderer"
	"github.com/pdf-translator/pdf-translator/internal/translator"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "pdf-translator",
		Short: "Translate PDF files while preserving document structure",
	}

	rootCmd.AddCommand(
		newTranslateCmd(),
		newCleanupCmd(),
		newVersionCmd(),
		newDownloadFontsCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newTranslateCmd() *cobra.Command {
	var req domain.TranslateRequest
	var verbose bool

	cmd := &cobra.Command{
		Use:   "translate [input.pdf]",
		Short: "Translate a PDF file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.InputPath = args[0]

			if req.OutputPath == "" {
				ext := filepath.Ext(req.InputPath)
				base := req.InputPath[:len(req.InputPath)-len(ext)]
				req.OutputPath = base + "_translated" + ext
			}

			logger := buildLogger(verbose)
			defer func() { _ = logger.Sync() }()

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			applyFlagOverrides(cfg, &req)

			if !req.DryRun {
				if err := cfg.Validate(); err != nil {
					return err
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			db, err := openDatabase(cfg.DataDir)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}

			logger.Info("starting translation",
				zap.String("input", req.InputPath),
				zap.String("output", req.OutputPath),
				zap.String("targetLang", req.TargetLang),
				zap.String("sourceLang", req.SourceLang),
				zap.Int("workers", req.Workers),
				zap.Int("dpi", req.DPI),
				zap.Bool("dryRun", req.DryRun),
			)

			// Wire all components
			if req.OCREngine == domain.OCREngineDocling {
				doclingClient := docling.New(cfg.DoclingURL)
				trans := translator.New(cfg.OpenAIAPIKey, logger)
				rend := renderer.NewMarkdownRenderer(
					renderer.NewFontManagerWithOverrides(cfg.DataDir, cfg.ScriptFontPaths()),
					logger,
				)
				pipe := pipeline.NewDoclingPipeline(doclingClient, trans, rend, logger)
				return pipe.Run(ctx, &req)
			}

			det := detector.New()
			nativeExt := extractor.NewNativeExtractor()

			var rec recognizer.TextRecognizer
			if req.OCREngine == domain.OCREngineTesseract {
				rec = recognizer.NewTesseractRecognizer(cfg.TesseractURL, cfg.HTTPTimeout)
			} else {
				rec = recognizer.NewPaddleRecognizer(cfg.PaddleOCRURL, cfg.HTTPTimeout)
			}
			ocrExt := extractor.NewOCRExtractor(rec)
			layoutExt := extractor.NewLayoutExtractor(recognizer.NewPaddleLayoutAnalyzer(cfg.PaddleOCRURL, cfg.HTTPTimeout))

			trans := translator.New(cfg.OpenAIAPIKey, logger)
			rend := renderer.NewRendererWithFontPaths(cfg.DataDir, cfg.ScriptFontPaths(), logger)

			pipe := pipeline.New(nativeExt, ocrExt, layoutExt, trans, rend, logger)

			dispatcher := queue.NewDispatcher(db, cfg, logger)
			dispatcher.SetProcessor(pipe)
			dispatcher.SetCounter(det)

			return dispatcher.Run(ctx, &req)
		},
	}

	cmd.Flags().StringVarP(&req.OutputPath, "output", "o", "", "Output file path (default: input_translated.pdf)")
	cmd.Flags().StringVar(&req.SourceLang, "from", "auto", "Source language (auto-detect by default)")
	cmd.Flags().StringVar(&req.TargetLang, "to", "", "Target language (required)")
	// TODO: --pages is not yet implemented; the flag is reserved for future use.
	cmd.Flags().StringVar(&req.Pages, "pages", "", "Page range (e.g. '1-5'); default: all [NOT YET IMPLEMENTED]")
	cmd.Flags().StringVar(&req.OCREngine, "ocr-engine", "", "OCR backend: paddleocr (default), tesseract, or docling")
	cmd.Flags().IntVar(&req.Workers, "workers", 0, "Max parallel page workers (default: from config)")
	cmd.Flags().IntVar(&req.DPI, "dpi", 0, "Render DPI for scanned page OCR (default: from config)")
	cmd.Flags().StringVar(&req.Password, "password", "", "Decryption password for protected PDFs")
	cmd.Flags().BoolVar(&req.DryRun, "dry-run", false, "Extract and estimate cost without translating")
	cmd.Flags().BoolVar(&req.KeepOriginal, "keep-original", false, "Preserve source text above each translation in the output PDF")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable debug logging")

	_ = cmd.MarkFlagRequired("to")

	return cmd
}

func newCleanupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup",
		Short: "Remove orphaned temp files from failed or abandoned jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			db, err := openDatabase(cfg.DataDir)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}

			logger := buildLogger(false)
			defer func() { _ = logger.Sync() }()

			return queue.Cleanup(db, cfg.DataDir, logger)
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version info",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("pdf-translator %s\n", version)
		},
	}
}

func newDownloadFontsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "download-fonts",
		Short: "Pre-download all font variants for offline use",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			logger := buildLogger(false)
			defer func() { _ = logger.Sync() }()

			fm := renderer.NewFontManagerWithOverrides(cfg.DataDir, cfg.ScriptFontPaths())
			return fm.DownloadAllFonts(cmd.Context(), logger)
		},
	}
}

func openDatabase(dataDir string) (*gorm.DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data directory %s: %w", dataDir, err)
	}

	dbPath := filepath.Join(dataDir, "jobs.db")
	db, err := gorm.Open(sqlite.Open(dbPath+"?_journal_mode=WAL&_busy_timeout=5000"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("opening SQLite at %s: %w", dbPath, err)
	}

	if err := db.AutoMigrate(&queue.DocumentJob{}, &queue.PageJob{}); err != nil {
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	return db, nil
}

func applyFlagOverrides(cfg *config.Config, req *domain.TranslateRequest) {
	if req.OCREngine == "" {
		req.OCREngine = cfg.OCREngine
	}
	if req.Workers == 0 {
		req.Workers = cfg.MaxPageWorkers
	}
	if req.DPI == 0 {
		req.DPI = cfg.DPI
	}
}

func buildLogger(verbose bool) *zap.Logger {
	var cfg zap.Config
	if verbose {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
		cfg.DisableStacktrace = true
	}
	logger, err := cfg.Build()
	if err != nil {
		// zap.NewProduction/NewDevelopment only fail on misconfigured
		// encoders; fallback to nop rather than crashing the user's CLI.
		return zap.NewNop()
	}
	return logger
}
