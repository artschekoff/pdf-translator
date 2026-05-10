package documents

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourorg/pdf-translator-webapp/internal/auth"
	"github.com/yourorg/pdf-translator-webapp/internal/queue"
	"github.com/yourorg/pdf-translator-webapp/internal/shared"
)

// BalanceChecker is satisfied by billing.Repository.
type BalanceChecker interface {
	GetBalance(userID string) (int, error)
}

type Handler struct {
	svc     *Service
	queues  *queue.Client
	billing BalanceChecker
}

func NewHandler(svc *Service, q *queue.Client, billing BalanceChecker) *Handler {
	return &Handler{svc: svc, queues: q, billing: billing}
}

func (h *Handler) Register(r *gin.RouterGroup) {
	r.GET("/documents", h.list)
	r.POST("/documents", h.upload)
	r.GET("/documents/:id", h.get)
	r.DELETE("/documents/:id", h.delete)
}

// RegisterOCR registers the OCR trigger route on a separate group so callers
// can attach quota middleware independently of the CRUD routes.
func (h *Handler) RegisterOCR(r *gin.RouterGroup) {
	r.POST("/documents/:id/ocr", h.startOCR)
}

func (h *Handler) list(c *gin.Context) {
	docs, err := h.svc.List(auth.UserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, docs)
}

func (h *Handler) upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file"})
		return
	}
	defer file.Close()

	var settings shared.OcrSettings
	if raw := c.PostForm("ocr_settings"); raw != "" {
		json.Unmarshal([]byte(raw), &settings)
	}
	if settings.Engine == "" {
		settings = shared.OcrSettings{
			Engine: "rapidocr", Lang: []string{"en"}, DoOcr: true,
			DoTableStructure: true, TableMode: "fast",
			GeneratePictureImages: true, ImagesScale: 2.0, DocumentTimeout: 300,
		}
	}

	doc, err := h.svc.Upload(auth.UserID(c), header.Filename, file, settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, doc)
}

func (h *Handler) get(c *gin.Context) {
	doc, err := h.svc.Get(c.Param("id"), auth.UserID(c))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, doc)
}

func (h *Handler) delete(c *gin.Context) {
	if err := h.svc.Delete(c.Param("id"), auth.UserID(c)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) startOCR(c *gin.Context) {
	userID := auth.UserID(c)
	if h.billing != nil {
		pages, err := h.billing.GetBalance(userID)
		if err == nil && pages <= 0 {
			c.JSON(http.StatusPaymentRequired, gin.H{"error": "insufficient page balance", "code": "balance_empty"})
			return
		}
	}

	docID := c.Param("id")
	if err := h.queues.EnqueueOCR(c.Request.Context(), queue.OcrPayload{
		DocumentID: docID,
		UserID:     userID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}
