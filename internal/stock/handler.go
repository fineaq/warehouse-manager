package stock

import (
	"context"
	"errors"
	"net/http"
	"warehouse-manager/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/stock/receipts", h.Receive)
	rg.GET("/stock/on-hand", h.OnHand)
}

func (h *Handler) Receive(c *gin.Context) {

	var recJSON receiveJSON
	if err := c.ShouldBindJSON(&recJSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	userID, ok := middleware.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	tenantID, ok := middleware.TenantID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	req := ReceiveRequest{
		TenantID:   tenantID,
		UserID:     userID,
		LocationID: recJSON.LocationID,
		ProductID:  recJSON.ProductID,
		Quantity:   recJSON.Quantity,
		Note:       recJSON.Note,
	}

	err := h.service.Receive(c.Request.Context(), req)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) OnHand(c *gin.Context) {
	tenantID, ok := middleware.TenantID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	productID, err := uuid.Parse(c.Query("product_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product_id"})
		return
	}

	locationID, err := uuid.Parse(c.Query("location_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid location_id"})
		return
	}

	req := OnHandRequest{
		TenantID:   tenantID,
		ProductID:  productID,
		LocationID: locationID,
	}

	qty, err := h.service.OnHand(c.Request.Context(), req)

	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, OnHandResponse{Quantity: qty})
}

// respondError maps a service error to a status and a fixed message. Service
// errors are wrapped and may carry SQL detail, so err is never sent as-is.
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidQuantity):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "quantity must be greater than zero"})

	case errors.Is(err, ErrProductNotFound):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "unknown product"})

	case errors.Is(err, ErrLocationNotFound):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "unknown location"})

	// The client gave up; nothing will read this response.
	case errors.Is(err, context.Canceled):
		c.AbortWithStatus(499)

	case errors.Is(err, context.DeadlineExceeded):
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "request timed out"})

	// ErrUserNotFound lands here on purpose: a valid token naming a missing
	// user is a server-side problem, not something the caller can fix.
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
