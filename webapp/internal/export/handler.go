package export

import (
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/yourorg/pdf-translator-webapp/internal/auth"
	"github.com/yourorg/pdf-translator-webapp/internal/documents"
	"gorm.io/gorm"
)

type Handler struct{ db *gorm.DB }

func NewHandler(db *gorm.DB) *Handler { return &Handler{db: db} }

func (h *Handler) Register(r *gin.RouterGroup) {
	r.POST("/documents/:id/export", h.export)
}

type exportRequest struct {
	Format string `json:"format" binding:"required"`
	Theme  string `json:"theme"`
}

func (h *Handler) export(c *gin.Context) {
	var req exportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Theme == "" {
		req.Theme = "light"
	}

	var doc documents.Document
	if err := h.db.Preload("Pages.Blocks").
		Where("id = ? AND user_id = ?", c.Param("id"), auth.UserID(c)).
		First(&doc).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	path, err := Export(h.db, &doc, req.Format, req.Theme)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.FileAttachment(path, filepath.Base(path))
}
