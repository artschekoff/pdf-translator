package queue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pdf-translator/pdf-translator/internal/config"
	"github.com/pdf-translator/pdf-translator/internal/domain"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

// PageProcessor is implemented by the pipeline integration layer (Phase F)
// to process a single page: extract -> translate -> render.
type PageProcessor interface {
	ProcessPage(ctx context.Context, job *PageJob, docJob *DocumentJob) error
}

// PageCounter returns total page count and per-page types for the input PDF.
type PageCounter interface {
	CountPages(ctx context.Context, inputPath string, password string) (int, error)
	DetectPageType(ctx context.Context, inputPath string, password string, pageNum int) (domain.PageType, error)
	CountAndClassifyAll(ctx context.Context, inputPath string, password string) (int, []domain.PageType, error)
}

type Dispatcher struct {
	db         *gorm.DB
	cfg        *config.Config
	logger     *zap.Logger
	maxRetries int
	processor  PageProcessor
	counter    PageCounter
	claimMu    sync.Mutex
}

func NewDispatcher(db *gorm.DB, cfg *config.Config, logger *zap.Logger) *Dispatcher {
	return &Dispatcher{
		db:         db,
		cfg:        cfg,
		logger:     logger,
		maxRetries: 3,
	}
}

func (d *Dispatcher) SetProcessor(p PageProcessor) {
	d.processor = p
}

func (d *Dispatcher) SetCounter(c PageCounter) {
	d.counter = c
}

func (d *Dispatcher) Run(ctx context.Context, req *domain.TranslateRequest) error {
	if d.processor == nil || d.counter == nil {
		return fmt.Errorf("dispatcher not fully wired: processor and counter must be set")
	}

	docJob, resumed, err := d.findOrCreateDocumentJob(ctx, req)
	if err != nil {
		return fmt.Errorf("creating document job: %w", err)
	}

	if resumed {
		d.logger.Info("resuming previous job",
			zap.String("jobID", docJob.ID),
			zap.Int("totalPages", docJob.TotalPages),
		)
		if err := d.ensureTempDir(ctx, docJob); err != nil {
			return fmt.Errorf("ensuring temp dir: %w", err)
		}
		if err := d.resetRunningPages(ctx, docJob.ID); err != nil {
			return fmt.Errorf("resetting stale running pages: %w", err)
		}
	}

	if req.DryRun {
		err := d.printDryRun(ctx, docJob)
		if err != nil {
			d.updateDocStatus(ctx, docJob, JobStatusFailed, err.Error())
		} else {
			d.updateDocStatus(ctx, docJob, JobStatusCompleted, "")
		}
		if cleanErr := os.RemoveAll(docJob.TempDir); cleanErr != nil {
			d.logger.Warn("failed to clean up dry-run temp dir", zap.String("path", docJob.TempDir), zap.Error(cleanErr))
		}
		return err
	}

	for retry := 0; retry <= d.maxRetries; retry++ {
		pendingCount, err := d.countByStatus(ctx, docJob.ID, JobStatusPending)
		if err != nil {
			return fmt.Errorf("checking pending pages: %w", err)
		}
		if pendingCount == 0 {
			break
		}

		if retry > 0 {
			d.logger.Info("retrying failed pages",
				zap.Int("attempt", retry),
				zap.Int64("pending", pendingCount),
			)
		}

		if err := d.runWorkerPool(ctx, docJob, req.Workers); err != nil {
			return fmt.Errorf("worker pool: %w", err)
		}

		failedCount, err := d.countByStatus(ctx, docJob.ID, JobStatusFailed)
		if err != nil {
			return fmt.Errorf("checking failed pages: %w", err)
		}
		if failedCount == 0 {
			break
		}

		if retry < d.maxRetries {
			d.markFailedAsPending(ctx, docJob.ID)
		}
	}

	failedCount, err := d.countByStatus(ctx, docJob.ID, JobStatusFailed)
	if err != nil {
		return fmt.Errorf("checking final failed count: %w", err)
	}
	if failedCount > 0 {
		d.updateDocStatus(ctx, docJob, JobStatusFailed, fmt.Sprintf("%d pages failed", failedCount))
		return fmt.Errorf("%d pages failed after %d attempts", failedCount, d.maxRetries+1)
	}

	if err := Assemble(ctx, d.db, docJob, d.logger); err != nil {
		d.updateDocStatus(ctx, docJob, JobStatusFailed, err.Error())
		return fmt.Errorf("assembling output: %w", err)
	}

	d.updateDocStatus(ctx, docJob, JobStatusCompleted, "")

	if err := os.RemoveAll(docJob.TempDir); err != nil {
		d.logger.Warn("failed to clean up temp dir", zap.String("path", docJob.TempDir), zap.Error(err))
	}

	d.logger.Info("translation complete",
		zap.String("output", docJob.OutputPath),
		zap.Int("pages", docJob.TotalPages),
	)
	return nil
}

func (d *Dispatcher) ensureTempDir(ctx context.Context, docJob *DocumentJob) error {
	if _, err := os.Stat(docJob.TempDir); err == nil {
		return nil
	}
	newDir, err := os.MkdirTemp("", "pdf-translator-*")
	if err != nil {
		return fmt.Errorf("recreating temp dir: %w", err)
	}
	d.logger.Info("recreated missing temp directory",
		zap.String("old", docJob.TempDir),
		zap.String("new", newDir),
	)
	docJob.TempDir = newDir
	if dbErr := d.db.WithContext(ctx).Model(docJob).Update("temp_dir", newDir).Error; dbErr != nil {
		return fmt.Errorf("updating temp dir in DB: %w", dbErr)
	}
	return nil
}

func (d *Dispatcher) findOrCreateDocumentJob(ctx context.Context, req *domain.TranslateRequest) (*DocumentJob, bool, error) {
	absInput, err := filepath.Abs(req.InputPath)
	if err != nil {
		return nil, false, fmt.Errorf("resolving input path: %w", err)
	}
	absOutput, err := filepath.Abs(req.OutputPath)
	if err != nil {
		return nil, false, fmt.Errorf("resolving output path: %w", err)
	}

	var existing DocumentJob
	result := d.db.WithContext(ctx).Where("input_path = ? AND output_path = ? AND status IN ?",
		absInput, absOutput,
		[]JobStatus{JobStatusPending, JobStatusRunning, JobStatusFailed},
	).First(&existing)

	if result.Error == nil {
		existing.Password = req.Password
		return &existing, true, nil
	}
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("querying existing job: %w", result.Error)
	}

	totalPages, pageTypes, err := d.counter.CountAndClassifyAll(ctx, req.InputPath, req.Password)
	if err != nil {
		return nil, false, fmt.Errorf("counting and classifying pages: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "pdf-translator-*")
	if err != nil {
		return nil, false, fmt.Errorf("creating temp dir: %w", err)
	}

	docJob := &DocumentJob{
		ID:         uuid.New().String(),
		InputPath:  absInput,
		OutputPath: absOutput,
		SourceLang: req.SourceLang,
		TargetLang: req.TargetLang,
		OCREngine:  req.OCREngine,
		TotalPages: totalPages,
		TempDir:    tempDir,
		DPI:        req.DPI,
		Password:   req.Password,
		Status:     JobStatusRunning,
	}

	if err := d.db.WithContext(ctx).Create(docJob).Error; err != nil {
		return nil, false, fmt.Errorf("inserting document job: %w", err)
	}

	if err := d.createPageJobs(ctx, docJob, pageTypes); err != nil {
		return nil, false, fmt.Errorf("creating page jobs: %w", err)
	}

	d.logger.Info("created document job",
		zap.String("jobID", docJob.ID),
		zap.Int("totalPages", totalPages),
		zap.String("tempDir", tempDir),
	)

	return docJob, false, nil
}

func (d *Dispatcher) createPageJobs(ctx context.Context, docJob *DocumentJob, pageTypes []domain.PageType) error {
	for i := 1; i <= docJob.TotalPages; i++ {
		pageType := domain.PageTypeScanned
		if i-1 < len(pageTypes) {
			pageType = pageTypes[i-1]
		}

		job := &PageJob{
			DocumentID: docJob.ID,
			PageNum:    i,
			PageType:   string(pageType),
			Status:     JobStatusPending,
		}
		if err := d.db.WithContext(ctx).Create(job).Error; err != nil {
			return fmt.Errorf("inserting page job %d: %w", i, err)
		}
	}
	return nil
}

func (d *Dispatcher) runWorkerPool(ctx context.Context, docJob *DocumentJob, workers int) error {
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	var claimErr error
	for {
		if gctx.Err() != nil {
			break
		}

		pageJob, err := d.claimNextPage(gctx, docJob.ID)
		if err != nil {
			claimErr = err
			break
		}
		if pageJob == nil {
			break
		}

		pj := pageJob
		g.Go(func() error {
			// Use a detached context for DB status updates so they
			// succeed even when the parent context is cancelled.
			dbCtx := context.Background()

			if err := d.processor.ProcessPage(gctx, pj, docJob); err != nil {
				d.logger.Error("page processing failed",
					zap.Int("page", pj.PageNum),
					zap.Int("retries", pj.Retries),
					zap.Error(err),
				)
				now := time.Now()
				if dbErr := d.db.WithContext(dbCtx).Model(pj).Updates(map[string]interface{}{
					"status":       JobStatusFailed,
					"error":        err.Error(),
					"retries":      pj.Retries + 1,
					"completed_at": &now,
				}).Error; dbErr != nil {
					d.logger.Error("failed to update page status", zap.Int("page", pj.PageNum), zap.Error(dbErr))
				}
				return nil
			}

			now := time.Now()
			if dbErr := d.db.WithContext(dbCtx).Model(pj).Updates(map[string]interface{}{
				"status":       JobStatusCompleted,
				"output_file":  filepath.Join(docJob.TempDir, fmt.Sprintf("page_%04d.pdf", pj.PageNum)),
				"completed_at": &now,
			}).Error; dbErr != nil {
				d.logger.Error("failed to update page status", zap.Int("page", pj.PageNum), zap.Error(dbErr))
			}

			completed, countErr := d.countByStatus(dbCtx, docJob.ID, JobStatusCompleted)
			if countErr != nil {
				d.logger.Warn("failed to count completed pages", zap.Error(countErr))
			}
			d.logger.Info("page completed",
				zap.Int("page", pj.PageNum),
				zap.Int64("completed", completed),
				zap.Int("total", docJob.TotalPages),
			)
			return nil
		})
	}

	waitErr := g.Wait()
	if claimErr != nil {
		return fmt.Errorf("claiming page: %w", claimErr)
	}
	if waitErr != nil {
		return fmt.Errorf("worker pool: %w", waitErr)
	}
	return nil
}

func (d *Dispatcher) claimNextPage(ctx context.Context, docID string) (*PageJob, error) {
	d.claimMu.Lock()
	defer d.claimMu.Unlock()

	var job PageJob
	tx := d.db.WithContext(ctx).Begin()
	defer tx.Rollback()

	result := tx.Where("document_id = ? AND status = ?", docID, JobStatusPending).
		Order("page_num ASC").
		Limit(1).
		First(&job)

	if result.Error != nil {
		tx.Rollback()
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying pending page: %w", result.Error)
	}

	now := time.Now()
	if err := tx.Model(&job).Updates(map[string]interface{}{
		"status":     JobStatusRunning,
		"started_at": &now,
	}).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("claiming page %d: %w", job.PageNum, err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("committing page claim: %w", err)
	}
	return &job, nil
}

func (d *Dispatcher) countByStatus(ctx context.Context, docID string, status JobStatus) (int64, error) {
	var count int64
	if err := d.db.WithContext(ctx).Model(&PageJob{}).Where("document_id = ? AND status = ?", docID, status).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("counting pages by status %s: %w", status, err)
	}
	return count, nil
}

func (d *Dispatcher) resetRunningPages(ctx context.Context, docID string) error {
	return d.db.WithContext(ctx).Model(&PageJob{}).
		Where("document_id = ? AND status = ?", docID, JobStatusRunning).
		Update("status", JobStatusPending).Error
}

func (d *Dispatcher) markFailedAsPending(ctx context.Context, docID string) {
	if err := d.db.WithContext(ctx).Model(&PageJob{}).
		Where("document_id = ? AND status = ?", docID, JobStatusFailed).
		Updates(map[string]interface{}{
			"status": JobStatusPending,
			"error":  "",
		}).Error; err != nil {
		d.logger.Error("marking failed pages as pending", zap.Error(err))
	}
}

func (d *Dispatcher) updateDocStatus(ctx context.Context, doc *DocumentJob, status JobStatus, errMsg string) {
	if err := d.db.WithContext(ctx).Model(doc).Updates(map[string]interface{}{
		"status": status,
		"error":  errMsg,
	}).Error; err != nil {
		d.logger.Error("updating document status", zap.String("status", string(status)), zap.Error(err))
	}
}

func (d *Dispatcher) printDryRun(ctx context.Context, docJob *DocumentJob) error {
	var nativeCount, scannedCount int64
	if err := d.db.WithContext(ctx).Model(&PageJob{}).Where("document_id = ? AND page_type = ?", docJob.ID, "native").Count(&nativeCount).Error; err != nil {
		return fmt.Errorf("counting native pages: %w", err)
	}
	if err := d.db.WithContext(ctx).Model(&PageJob{}).Where("document_id = ? AND page_type = ?", docJob.ID, "scanned").Count(&scannedCount).Error; err != nil {
		return fmt.Errorf("counting scanned pages: %w", err)
	}

	fmt.Printf("\nDry Run Summary\n")
	fmt.Printf("───────────────\n")
	fmt.Printf("Document:      %s (%d pages)\n", docJob.InputPath, docJob.TotalPages)
	fmt.Printf("Type:          native (%d pages), scanned (%d pages)\n", nativeCount, scannedCount)
	fmt.Printf("Target:        %s -> %s\n", docJob.SourceLang, docJob.TargetLang)
	fmt.Printf("OCR engine:    %s\n", docJob.OCREngine)
	fmt.Printf("OCR required:  %d pages\n", scannedCount)
	fmt.Printf("\nRun without --dry-run to translate.\n")
	return nil
}
