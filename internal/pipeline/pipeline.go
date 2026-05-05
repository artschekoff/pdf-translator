package pipeline

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/pdf-translator/pdf-translator/internal/extractor"
	"github.com/pdf-translator/pdf-translator/internal/queue"
	"go.uber.org/zap"
)

// BlockTranslator translates a batch of text blocks between languages.
type BlockTranslator interface {
	TranslateBlocks(ctx context.Context, blocks []domain.TextBlock, sourceLang, targetLang string) ([]domain.TextBlock, error)
}

// PageRenderer renders a translated page to a single-page PDF.
type PageRenderer interface {
	RenderPage(ctx context.Context, inputPath string, password string, pageNum int, blocks []domain.TextBlock, targetLang string, outputPath string) error
}

// Pipeline implements queue.PageProcessor by wiring extract -> translate -> render.
type Pipeline struct {
	nativeExtractor extractor.Extractor
	ocrExtractor    extractor.Extractor
	layoutExtractor extractor.Extractor
	translator      BlockTranslator
	renderer        PageRenderer
	logger          *zap.Logger
}

func New(
	nativeExt extractor.Extractor,
	ocrExt extractor.Extractor,
	layoutExt extractor.Extractor,
	trans BlockTranslator,
	rend PageRenderer,
	logger *zap.Logger,
) *Pipeline {
	return &Pipeline{
		nativeExtractor: nativeExt,
		ocrExtractor:    ocrExt,
		layoutExtractor: layoutExt,
		translator:      trans,
		renderer:        rend,
		logger:          logger,
	}
}

func (p *Pipeline) ProcessPage(ctx context.Context, pageJob *queue.PageJob, docJob *queue.DocumentJob) error {
	p.logger.Info("processing page",
		zap.Int("page", pageJob.PageNum),
		zap.String("type", pageJob.PageType),
	)

	if p.layoutExtractor == nil {
		return fmt.Errorf("layout analysis page %d: no layout extractor configured", pageJob.PageNum)
	}

	layoutBlocks, err := p.layoutExtractor.ExtractPage(ctx, docJob.InputPath, docJob.Password, pageJob.PageNum, docJob.DPI)
	if err != nil {
		return fmt.Errorf("layout analysis page %d: %w", pageJob.PageNum, err)
	}

	var textBlocks []domain.TextBlock
	if pageJob.PageType == string(domain.PageTypeNative) {
		textBlocks, err = p.nativeExtractor.ExtractPage(ctx, docJob.InputPath, docJob.Password, pageJob.PageNum, docJob.DPI)
		if err != nil {
			return fmt.Errorf("native extraction page %d: %w", pageJob.PageNum, err)
		}
	} else {
		if p.ocrExtractor == nil {
			return fmt.Errorf("OCR extraction page %d: no OCR extractor configured for scanned page", pageJob.PageNum)
		}
		textBlocks, err = p.ocrExtractor.ExtractPage(ctx, docJob.InputPath, docJob.Password, pageJob.PageNum, docJob.DPI)
		if err != nil {
			return fmt.Errorf("OCR extraction page %d: %w", pageJob.PageNum, err)
		}
	}

	blocks, unmatched := mergeLayoutBlocks(layoutBlocks, textBlocks)
	if unmatched > 0 {
		p.logger.Warn("layout analysis did not classify all text blocks",
			zap.Int("page", pageJob.PageNum),
			zap.Int("unmatched", unmatched),
		)
	}

	if len(blocks) == 0 {
		p.logger.Info("no text blocks found, copying page as-is", zap.Int("page", pageJob.PageNum))
		outputPath := filepath.Join(docJob.TempDir, fmt.Sprintf("page_%04d.pdf", pageJob.PageNum))
		return p.renderer.RenderPage(ctx, docJob.InputPath, docJob.Password, pageJob.PageNum, nil, docJob.TargetLang, outputPath)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Translate
	translated, err := p.translator.TranslateBlocks(ctx, blocks, docJob.SourceLang, docJob.TargetLang)
	if err != nil {
		return fmt.Errorf("translating page %d: %w", pageJob.PageNum, err)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Render
	outputPath := filepath.Join(docJob.TempDir, fmt.Sprintf("page_%04d.pdf", pageJob.PageNum))
	if err := p.renderer.RenderPage(ctx, docJob.InputPath, docJob.Password, pageJob.PageNum, translated, docJob.TargetLang, outputPath); err != nil {
		return fmt.Errorf("rendering page %d: %w", pageJob.PageNum, err)
	}

	p.logger.Info("page completed",
		zap.Int("page", pageJob.PageNum),
		zap.Int("blocks", len(translated)),
	)

	return nil
}

