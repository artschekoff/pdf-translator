package main

import (
	"log"

	"github.com/yourorg/pdf-translator-webapp/config"
	"github.com/yourorg/pdf-translator-webapp/internal/billing"
	"github.com/yourorg/pdf-translator-webapp/internal/documents"
	"github.com/yourorg/pdf-translator-webapp/internal/ocr"
	"github.com/yourorg/pdf-translator-webapp/internal/queue"
	"github.com/yourorg/pdf-translator-webapp/internal/ws"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	config.Load()

	db, err := gorm.Open(postgres.Open(config.C.DatabaseURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	db.AutoMigrate(
		&documents.Document{}, &documents.Page{}, &documents.Block{},
		&billing.Plan{}, &billing.UserBalance{},
	)

	hub := ws.NewHub()
	billingRepo := billing.NewRepository(db)

	srv := queue.NewServer(config.C.RedisURL, config.C.WorkerConcurrency)
	mux := queue.NewMux()

	ocrWorker := ocr.NewWorker(db, hub, billingRepo)
	mux.HandleFunc(queue.TaskOCRProcess, ocrWorker.HandleOCR)

	log.Println("worker: starting — queues: ocr, ai, export")
	if err := srv.Run(mux); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
