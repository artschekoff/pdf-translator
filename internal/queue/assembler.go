package queue

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/pdf-translator/pdf-translator/internal/renderer"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Assemble merges all completed single-page PDFs into the final output file.
func Assemble(ctx context.Context, db *gorm.DB, docJob *DocumentJob, logger *zap.Logger) error {
	var pages []PageJob
	if err := db.WithContext(ctx).Where("document_id = ? AND status = ?", docJob.ID, JobStatusCompleted).
		Order("page_num ASC").
		Find(&pages).Error; err != nil {
		return fmt.Errorf("querying completed pages: %w", err)
	}

	if len(pages) != docJob.TotalPages {
		return fmt.Errorf("expected %d completed pages, got %d", docJob.TotalPages, len(pages))
	}

	sort.Slice(pages, func(i, j int) bool {
		return pages[i].PageNum < pages[j].PageNum
	})

	pagePaths := make([]string, 0, len(pages))
	for _, p := range pages {
		path := p.OutputFile
		if path == "" {
			path = filepath.Join(docJob.TempDir, fmt.Sprintf("page_%04d.pdf", p.PageNum))
		}
		pagePaths = append(pagePaths, path)
	}

	if err := renderer.MergePages(pagePaths, docJob.OutputPath); err != nil {
		return fmt.Errorf("merging pages: %w", err)
	}

	logger.Info("assembled output PDF",
		zap.String("output", docJob.OutputPath),
		zap.Int("pages", len(pages)),
	)
	return nil
}
