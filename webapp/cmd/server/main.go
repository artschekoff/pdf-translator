package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/yourorg/pdf-translator-webapp/config"
	"github.com/yourorg/pdf-translator-webapp/internal/ai"
	"github.com/yourorg/pdf-translator-webapp/internal/auth"
	"github.com/yourorg/pdf-translator-webapp/internal/billing"
	"github.com/yourorg/pdf-translator-webapp/internal/blocks"
	"github.com/yourorg/pdf-translator-webapp/internal/documents"
	"github.com/yourorg/pdf-translator-webapp/internal/export"
	"github.com/yourorg/pdf-translator-webapp/internal/ocr"
	"github.com/yourorg/pdf-translator-webapp/internal/queue"
	"github.com/yourorg/pdf-translator-webapp/internal/quota"
	"github.com/yourorg/pdf-translator-webapp/internal/ws"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	config.Load()

	os.MkdirAll(config.C.UploadsDir, 0755)
	os.MkdirAll(config.C.ExportsDir, 0755)

	db, err := gorm.Open(postgres.Open(config.C.DatabaseURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	db.AutoMigrate(
		&documents.Document{}, &documents.Page{}, &documents.Block{},
		&billing.Plan{}, &billing.UserBalance{}, &billing.CreditedPurchase{},
	)

	qClient := queue.NewClient(config.C.RedisURL)
	defer qClient.Close()

	hub := ws.NewHub()

	jwks := auth.NewJWKS(config.C.AuthServiceURL + "/.well-known/jwks.json")

	quotaClient, err := quota.New(config.C.RedisURL, map[string]int{
		quota.ScopeOCR:    config.C.QuotaOCRDailyLimit,
		quota.ScopeAI:     config.C.QuotaAIDailyLimit,
		quota.ScopeExport: config.C.QuotaExportDailyLimit,
	})
	if err != nil {
		log.Fatalf("quota: %v", err)
	}

	billingRepo := billing.NewRepository(db)
	billingSvc := billing.NewService(billingRepo, config.C.PaymentsServiceURL)

	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "http://localhost:3000")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	r.GET("/api/ws/:id/progress", hub.Handler)

	api := r.Group("/api", auth.Middleware(jwks))

	billing.NewHandler(billingSvc).Register(api)

	docHandler := documents.NewHandler(
		documents.NewService(documents.NewRepository(db)),
		qClient,
		billingRepo,
	)
	docHandler.Register(api)
	docHandler.RegisterOCR(r.Group("/api",
		auth.Middleware(jwks),
		quota.Middleware(quotaClient, quota.ScopeOCR),
	))

	blocks.NewHandler(db).Register(api)
	ai.NewHandler().Register(r.Group("/api",
		auth.Middleware(jwks),
		quota.Middleware(quotaClient, quota.ScopeAI),
	))
	export.NewHandler(db).Register(r.Group("/api",
		auth.Middleware(jwks),
		quota.Middleware(quotaClient, quota.ScopeExport),
	))

	go startWorker(db, hub, billingRepo)

	log.Printf("server listening on :%s", config.C.Port)
	r.Run(":" + config.C.Port)
}

func startWorker(db *gorm.DB, hub *ws.Hub, billingRepo *billing.Repository) {
	srv := queue.NewServer(config.C.RedisURL, config.C.WorkerConcurrency)
	mux := queue.NewMux()

	ocrWorker := ocr.NewWorker(db, hub, billingRepo)
	mux.HandleFunc(queue.TaskOCRProcess, ocrWorker.HandleOCR)

	if err := srv.Run(mux); err != nil {
		log.Printf("worker error: %v", err)
	}
}
