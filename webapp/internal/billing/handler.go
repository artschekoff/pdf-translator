package billing

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourorg/pdf-translator-webapp/internal/auth"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(protected *gin.RouterGroup) {
	protected.GET("/billing/plans", h.listPlans)
	protected.GET("/billing/balance", h.balance)
	protected.POST("/billing/checkout", h.checkout)
	protected.POST("/billing/sync", h.sync)
}

func (h *Handler) listPlans(c *gin.Context) {
	plans, err := h.svc.ListPlans()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load plans"})
		return
	}
	c.JSON(http.StatusOK, plans)
}

func (h *Handler) balance(c *gin.Context) {
	pages, err := h.svc.GetBalance(auth.UserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load balance"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"pages": pages})
}

type checkoutRequest struct {
	PlanID string `json:"plan_id" binding:"required"`
}

func (h *Handler) checkout(c *gin.Context) {
	var req checkoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bearerToken := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	url, err := h.svc.CreateCheckoutSession(c.Request.Context(), req.PlanID, bearerToken)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url})
}

func (h *Handler) sync(c *gin.Context) {
	bearerToken := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	credited, err := h.svc.SyncPurchases(c.Request.Context(), auth.UserID(c), bearerToken)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not sync with payments service"})
		return
	}

	balance, _ := h.svc.GetBalance(auth.UserID(c))
	c.JSON(http.StatusOK, gin.H{"credited": credited, "balance": balance})
}
