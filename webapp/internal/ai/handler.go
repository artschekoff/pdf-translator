package ai

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) Register(r *gin.RouterGroup) {
	r.POST("/ai/block", h.block)
	r.POST("/ai/document/:id", h.document)
}

type blockRequest struct {
	BlockText   string `json:"block_text" binding:"required"`
	Instruction string `json:"instruction" binding:"required"`
	Mode        string `json:"mode"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
}

func (h *Handler) block(c *gin.Context) {
	var req blockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Mode == "" {
		req.Mode = "ask"
	}
	if req.Provider == "" {
		req.Provider = "claude"
	}
	if req.Model == "" {
		req.Model = "claude-sonnet-4-6"
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	StreamBlock(c.Request.Context(), req.BlockText, req.Instruction, req.Mode, req.Provider, req.Model, c.Writer)
	c.Writer.Flush()
}

func (h *Handler) document(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
