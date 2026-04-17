package pipeline

import (
	"context"
	"fmt"
	"math"
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
	translator      BlockTranslator
	renderer        PageRenderer
	logger          *zap.Logger
}

func New(
	nativeExt extractor.Extractor,
	ocrExt extractor.Extractor,
	trans BlockTranslator,
	rend PageRenderer,
	logger *zap.Logger,
) *Pipeline {
	return &Pipeline{
		nativeExtractor: nativeExt,
		ocrExtractor:    ocrExt,
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

	var blocks []domain.TextBlock

	if pageJob.PageType == string(domain.PageTypeNative) {
		nativeBlocks, err := p.nativeExtractor.ExtractPage(ctx, docJob.InputPath, docJob.Password, pageJob.PageNum, docJob.DPI)
		if err != nil {
			return fmt.Errorf("native extraction page %d: %w", pageJob.PageNum, err)
		}

		// Supplement with OCR to catch path-based text (vector banners, etc.)
		// that native extraction can't see.
		if p.ocrExtractor != nil {
			ocrBlocks, err := p.ocrExtractor.ExtractPage(ctx, docJob.InputPath, docJob.Password, pageJob.PageNum, docJob.DPI)
			if err != nil {
				p.logger.Debug("supplemental OCR unavailable, using native only",
					zap.Int("page", pageJob.PageNum),
					zap.Error(err),
				)
			} else {
				extra := mergeBlocks(nativeBlocks, ocrBlocks)
				if len(extra) > 0 {
					p.logger.Info("OCR found additional text blocks",
						zap.Int("page", pageJob.PageNum),
						zap.Int("extra", len(extra)),
					)
					nativeBlocks = append(nativeBlocks, extra...)
				}
			}
		}

		blocks = nativeBlocks
	} else {
		if p.ocrExtractor == nil {
			return fmt.Errorf("OCR extraction page %d: no OCR extractor configured for scanned page", pageJob.PageNum)
		}
		var err error
		blocks, err = p.ocrExtractor.ExtractPage(ctx, docJob.InputPath, docJob.Password, pageJob.PageNum, docJob.DPI)
		if err != nil {
			return fmt.Errorf("OCR extraction page %d: %w", pageJob.PageNum, err)
		}
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

// overlapThreshold is the minimum fraction of overlap (by area) for two
// bounding boxes to be considered duplicates during native+OCR merge.
const overlapThreshold = 0.3

// mergeBlocks returns OCR blocks that don't significantly overlap with any
// native block. This captures path-based text (vector banners, decorative
// headers) that native extraction misses.
func mergeBlocks(native, ocr []domain.TextBlock) []domain.TextBlock {
	var extra []domain.TextBlock
	for _, ob := range ocr {
		if !overlapsAny(ob, native) {
			extra = append(extra, ob)
		}
	}
	return extra
}

func overlapsAny(block domain.TextBlock, others []domain.TextBlock) bool {
	for _, o := range others {
		if bboxOverlap(block.BBox, o.BBox) > overlapThreshold {
			return true
		}
	}
	return false
}

// bboxOverlap returns the fraction of a's area that overlaps with b.
func bboxOverlap(a, b domain.BoundingBox) float64 {
	ax1, ay1 := a.X, a.Y
	ax2, ay2 := a.X+a.Width, a.Y+a.Height
	bx1, by1 := b.X, b.Y
	bx2, by2 := b.X+b.Width, b.Y+b.Height

	ox := math.Max(0, math.Min(ax2, bx2)-math.Max(ax1, bx1))
	oy := math.Max(0, math.Min(ay2, by2)-math.Max(ay1, by1))
	overlap := ox * oy

	aArea := a.Width * a.Height
	if aArea <= 0 {
		return 0
	}
	return overlap / aArea
}
