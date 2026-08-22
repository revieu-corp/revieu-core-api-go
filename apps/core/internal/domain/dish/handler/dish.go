package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/dish/service"
)

type DishHandler struct {
	svc *service.DishService
}

type UpsertDishRequest struct {
	Name          string  `json:"name" binding:"required"`
	ImageURL      string  `json:"image_url"`
	Description   string  `json:"description"`
	OriginalPrice float64 `json:"original_price"`
	Category      string  `json:"category"`
}

type UpdateDishRequest struct {
	Name          *string  `json:"name"`
	ImageURL      *string  `json:"image_url"`
	Description   *string  `json:"description"`
	OriginalPrice *float64 `json:"original_price"`
	Category      *string  `json:"category"`
}

func NewDishHandler(svc *service.DishService) *DishHandler {
	if svc == nil {
		svc = service.NewDishService(nil)
	}
	return &DishHandler{svc: svc}
}

func (h *DishHandler) List(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishes, err := h.svc.ListMine(c.Request.Context(), userID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dishes})
}

func (h *DishHandler) Create(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req UpsertDishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dish, err := h.svc.Create(c.Request.Context(), userID, service.UpsertDishInput{
		Name: req.Name, ImageURL: req.ImageURL, Description: req.Description,
		OriginalPrice: req.OriginalPrice, Category: req.Category,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": dish})
}

func (h *DishHandler) Update(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishID, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dish id"})
		return
	}
	var req UpdateDishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dish, err := h.svc.Update(c.Request.Context(), userID, dishID, service.UpdateDishInput{
		Name: req.Name, ImageURL: req.ImageURL, Description: req.Description,
		OriginalPrice: req.OriginalPrice, Category: req.Category,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dish})
}

func (h *DishHandler) Delete(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishID, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dish id"})
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, dishID); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *DishHandler) Enable(c *gin.Context) { h.setStatus(c, service.DishStatusActive) }

func (h *DishHandler) Disable(c *gin.Context) { h.setStatus(c, service.DishStatusDisabled) }

func (h *DishHandler) setStatus(c *gin.Context, status string) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishID, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dish id"})
		return
	}
	dish, err := h.svc.SetStatus(c.Request.Context(), userID, dishID, status)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dish})
}

func parseID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

func (h *DishHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrMerchantNotFound), errors.Is(err, service.ErrDishForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	case errors.Is(err, service.ErrDishNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	case errors.Is(err, service.ErrInvalidDishInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dish input"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
