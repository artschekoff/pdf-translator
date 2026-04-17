package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const orphanThreshold = 24 * time.Hour

// Cleanup removes temp directories from failed or abandoned jobs older than 24h.
func Cleanup(db *gorm.DB, dataDir string, logger *zap.Logger) error {
	return CleanupWithContext(context.Background(), db, dataDir, logger)
}

// CleanupWithContext is the context-aware variant of Cleanup.
func CleanupWithContext(ctx context.Context, db *gorm.DB, dataDir string, logger *zap.Logger) error {
	var jobs []DocumentJob
	cutoff := time.Now().Add(-orphanThreshold)

	result := db.WithContext(ctx).Where("status IN ? AND updated_at < ?",
		[]JobStatus{JobStatusFailed, JobStatusPending, JobStatusRunning}, cutoff,
	).Find(&jobs)
	if result.Error != nil {
		return fmt.Errorf("querying stale jobs: %w", result.Error)
	}

	if len(jobs) == 0 {
		logger.Info("no orphaned jobs found")
		return nil
	}

	removed := 0
	for _, job := range jobs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if job.TempDir == "" {
			continue
		}

		absPath := job.TempDir
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(dataDir, absPath)
		}

		absPath = filepath.Clean(absPath)
		if !isSafeCleanupPath(absPath, dataDir) {
			logger.Warn("skipping suspicious temp dir path",
				zap.String("path", absPath),
			)
			continue
		}

		if err := os.RemoveAll(absPath); err != nil {
			logger.Warn("failed to remove temp dir",
				zap.String("path", absPath),
				zap.Error(err),
			)
			continue
		}

		if err := db.Delete(&PageJob{}, "document_id = ?", job.ID).Error; err != nil {
			logger.Warn("failed to delete page jobs", zap.String("jobID", job.ID), zap.Error(err))
		}
		if err := db.Delete(&job).Error; err != nil {
			logger.Warn("failed to delete document job", zap.String("jobID", job.ID), zap.Error(err))
		}
		removed++

		logger.Info("removed orphaned job",
			zap.String("jobID", job.ID),
			zap.String("tempDir", absPath),
		)
	}

	logger.Info("cleanup complete",
		zap.Int("removed", removed),
		zap.Int("total", len(jobs)),
	)
	return nil
}

// isSafeCleanupPath validates the path is within the OS temp directory or the
// application data directory to prevent accidental deletion of arbitrary paths.
func isSafeCleanupPath(absPath, dataDir string) bool {
	return isChildOf(absPath, os.TempDir()) ||
		(dataDir != "" && isChildOf(absPath, dataDir))
}

// isChildOf reports whether child is strictly inside parent. Both paths are
// cleaned before comparison, and a path-separator is appended to parent so
// "/tmp" does not match "/tmp_evil". On Windows the check is
// case-insensitive to match the filesystem semantics.
func isChildOf(child, parent string) bool {
	parent = filepath.Clean(parent) + string(filepath.Separator)
	child = filepath.Clean(child) + string(filepath.Separator)
	if runtime.GOOS == "windows" {
		return strings.HasPrefix(strings.ToLower(child), strings.ToLower(parent))
	}
	return strings.HasPrefix(child, parent)
}
