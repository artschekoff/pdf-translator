package blocks

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourorg/pdf-translator-webapp/internal/documents"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Handler struct{ db *gorm.DB }

func NewHandler(db *gorm.DB) *Handler { return &Handler{db: db} }

func (h *Handler) Register(r *gin.RouterGroup) {
	r.PUT("/documents/:id/blocks", h.save)
}

type saveRequest struct {
	PageID string `json:"page_id" binding:"required"`
	Blocks []struct {
		ID    string         `json:"id"`
		Type  string         `json:"type"`
		Data  map[string]any `json:"data"`
		Order int            `json:"order"`
	} `json:"blocks"`
}

func (h *Handler) save(c *gin.Context) {
	var req saveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.db.Where("page_id = ?", req.PageID).Delete(&documents.Block{})
	for _, b := range req.Blocks {
		dataJSON, _ := json.Marshal(b.Data)
		h.db.Create(&documents.Block{
			ID: b.ID, PageID: req.PageID, Type: b.Type,
			Data: datatypes.JSON(dataJSON), Order: b.Order,
		})
	}
	c.JSON(http.StatusOK, gin.H{"saved": len(req.Blocks)})
}
