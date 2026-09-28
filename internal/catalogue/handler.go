package catalogue

import (
	"context"
	"errors"
	"net/http"
	"warehouse-manager/internal/middleware"

	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/products", h.HandleListProducts)
	rg.GET("/locations", h.HandleListLocations)
	rg.POST("/locations", h.HandleCreateLocation)
	rg.POST("/products", h.HandleCreateProduct)
	rg.PUT("/products/:id", h.HandleUpdateProduct)
	rg.PUT("/locations/:id", h.HandleUpdateLocation)

}

func (h *Handler) HandleCreateProduct(c *gin.Context) {
	var prodJSON createProductJSON

	if err := c.ShouldBindJSON(&prodJSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	tenantID, ok := middleware.TenantID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	req := CreateProductRequest{
		TenantID: tenantID,
		Name:     prodJSON.Name,
		Code:     prodJSON.Code,
		Unit:     prodJSON.Unit,
		Note:     prodJSON.Note,
	}

	if err := h.service.CreateProduct(c.Request.Context(), req); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "ok"})
}

func (h *Handler) HandleCreateLocation(c *gin.Context) {
	var locJSON createLocationJSON

	if err := c.ShouldBindJSON(&locJSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	tenantID, ok := middleware.TenantID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	req := CreateLocationRequest{
		TenantID: tenantID,
		Name:     locJSON.Name,
		Code:     locJSON.Code,
		Address:  locJSON.Address,
		Note:     locJSON.Note,
	}

	if err := h.service.CreateLocation(c.Request.Context(), req); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "ok"})
}

func (h *Handler) HandleUpdateProduct(c *gin.Context) {
	var prodJSON productsJSON

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := c.ShouldBindJSON(&prodJSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	tenantID, ok := middleware.TenantID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	req := UpdateProductRequest{
		Id:       id,
		TenantID: tenantID,
		Name:     prodJSON.Name,
		Code:     prodJSON.Code,
		Unit:     prodJSON.Unit,
		Note:     prodJSON.Note,
	}

	if err := h.service.UpdateProduct(c.Request.Context(), req); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) HandleUpdateLocation(c *gin.Context) {
	var locJSON locationsJSON

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := c.ShouldBindJSON(&locJSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	tenantID, ok := middleware.TenantID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	req := UpdateLocationRequest{
		Id:       id,
		TenantID: tenantID,
		Name:     locJSON.Name,
		Code:     locJSON.Code,
		Address:  locJSON.Address,
		Note:     locJSON.Note,
	}

	if err := h.service.UpdateLocation(c.Request.Context(), req); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) HandleListProducts(c *gin.Context) {
	tenantID, ok := middleware.TenantID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	products, err := h.service.ListProduct(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	out := make([]productsJSON, len(products))
	for i, p := range products {
		out[i] = productsJSON(p)
	}

	c.JSON(http.StatusOK, gin.H{"products": out})
}

func (h *Handler) HandleListLocations(c *gin.Context) {
	tenantID, ok := middleware.TenantID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	locations, err := h.service.ListLocation(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	out := make([]locationsJSON, len(locations))
	for i, p := range locations {
		out[i] = locationsJSON(p)
	}

	c.JSON(http.StatusOK, gin.H{"locations": out})
}

// respondError maps a service error to a status and a fixed message. Service
// errors are wrapped and may carry SQL detail, so err is never sent as-is.
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDuplicateCode):
		c.JSON(http.StatusConflict, gin.H{"error": "duplicate code"})

	case errors.Is(err, ErrProductNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})

	case errors.Is(err, ErrLocationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "location not found"})

	// The client gave up; nothing will read this response.
	case errors.Is(err, context.Canceled):
		c.AbortWithStatus(499)

	case errors.Is(err, context.DeadlineExceeded):
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "request timed out"})

	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
