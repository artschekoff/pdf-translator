package ocr

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	"github.com/hibiken/asynq"
	"github.com/yourorg/pdf-translator-webapp/config"
	"github.com/yourorg/pdf-translator-webapp/internal/documents"
	"github.com/yourorg/pdf-translator-webapp/internal/queue"
	"github.com/yourorg/pdf-translator-webapp/internal/shared"
	"github.com/yourorg/pdf-translator-webapp/internal/ws"
	"github.com/yourorg/pdf-translator-webapp/pkg/docling"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PageDeductor is satisfied by billing.Repository.
type PageDeductor interface {
	DeductPages(userID string, pages int) error
}

type Worker struct {
	db      *gorm.DB
	hub     *ws.Hub
	docl    *docling.Client
	billing PageDeductor
}

func NewWorker(db *gorm.DB, hub *ws.Hub, billing PageDeductor) *Worker {
	return &Worker{
		db:      db,
		hub:     hub,
		docl:    docling.New(config.C.DockingServiceURL),
		billing: billing,
	}
}

func (w *Worker) HandleOCR(ctx context.Context, task *asynq.Task) error {
	var p queue.OcrPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}

	w.broadcast(p.DocumentID, "loading", 5)

	var doc documents.Document
	if err := w.db.First(&doc, "id = ?", p.DocumentID).Error; err != nil {
		return err
	}

	var settings shared.OcrSettings
	json.Unmarshal(doc.OcrSettings, &settings)

	w.db.Model(&doc).Update("status", documents.StatusProcessing)
	w.broadcast(p.DocumentID, "converting", 20)

	pdfPath := filepath.Join(config.C.UploadsDir, p.DocumentID+".pdf")
	result, err := w.docl.Convert(ctx, pdfPath, docling.ConvertRequest{
		Engine:                settings.Engine,
		Lang:                  settings.Lang,
		DoOcr:                 settings.DoOcr,
		DoTableStructure:      settings.DoTableStructure,
		TableMode:             settings.TableMode,
		GeneratePictureImages: settings.GeneratePictureImages,
		ImagesScale:           settings.ImagesScale,
	})
	if err != nil {
		errMsg := err.Error()
		w.db.Model(&doc).Updates(map[string]any{"status": documents.StatusError, "error_message": errMsg})
		w.hub.Broadcast(p.DocumentID, ws.Message{Stage: "error", Error: errMsg})
		return err
	}

	w.broadcast(p.DocumentID, "extracting", 70)

	blocks := shared.MarkdownToBlocks(result.Markdown)
	rawMd := result.Markdown
	page := documents.Page{DocumentID: p.DocumentID, PageNumber: 1, RawMarkdown: &rawMd}
	w.db.Create(&page)

	for order, b := range blocks {
		dataJSON, _ := json.Marshal(b.Data)
		w.db.Create(&documents.Block{PageID: page.ID, Type: b.Type, Data: datatypes.JSON(dataJSON), Order: order})
	}

	w.db.Model(&doc).Updates(map[string]any{"status": documents.StatusReady, "page_count": result.Pages})
	w.hub.Broadcast(p.DocumentID, ws.Message{Stage: "complete", Percent: 100})
	log.Printf("ocr: document %s processed, %d blocks", p.DocumentID, len(blocks))

	if w.billing != nil && p.UserID != "" && result.Pages > 0 {
		if err := w.billing.DeductPages(p.UserID, result.Pages); err != nil {
			log.Printf("billing: failed to deduct %d pages for user %s: %v", result.Pages, p.UserID, err)
		}
	}
	return nil
}

func (w *Worker) broadcast(docID, stage string, pct int) {
	w.hub.Broadcast(docID, ws.Message{Stage: stage, Percent: pct})
}
