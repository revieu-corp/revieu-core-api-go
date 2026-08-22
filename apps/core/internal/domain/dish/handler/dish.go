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

func NewDishHandler(svc *service.DishService) *DishHandler {
	if svc == nil {
		svc = service.NewDishService(nil)
	}
	return &DishHandler{svc: svc}
}

type UpsertDishRequest struct {
	Name          string  `json:"name"`
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

func dishErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrDishNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, service.ErrDishForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, service.ErrInvalidDishInput):
		return http.StatusBadRequest, "invalid dish input"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

// CreateDish godoc
// @Summary Create dish
// @Tags dish
// @Accept json
// @Produce json
// @Param request body UpsertDishRequest true "Create dish request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/dishes [post]
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
	dish, err := h.svc.Create(c.Request.Context(), userID, service.CreateDishInput{
		Name: req.Name, ImageURL: req.ImageURL, Description: req.Description,
		OriginalPrice: req.OriginalPrice, Category: req.Category,
	})
	if err != nil {
		status, msg := dishErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": dish})
}

// ListMine godoc
// @Summary List my dishes
// @Tags dish
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/dishes [get]
func (h *DishHandler) ListMine(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishes, err := h.svc.ListMine(c.Request.Context(), userID)
	if err != nil {
		status, msg := dishErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dishes})
}

func parseDishID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dish id"})
		return 0, false
	}
	return id, true
}

// Update godoc
// @Summary Update dish
// @Tags dish
// @Accept json
// @Produce json
// @Param id path int true "Dish ID"
// @Param request body UpdateDishRequest true "Update dish request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/dishes/{id} [patch]
func (h *DishHandler) Update(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishID, ok := parseDishID(c)
	if !ok {
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
		status, msg := dishErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dish})
}

// Delete godoc
// @Summary Delete dish
// @Tags dish
// @Produce json
// @Param id path int true "Dish ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/dishes/{id} [delete]
func (h *DishHandler) Delete(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishID, ok := parseDishID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, dishID); err != nil {
		status, msg := dishErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *DishHandler) setStatus(c *gin.Context, targetStatus string) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishID, ok := parseDishID(c)
	if !ok {
		return
	}
	dish, err := h.svc.SetStatus(c.Request.Context(), userID, dishID, targetStatus)
	if err != nil {
		status, msg := dishErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dish})
}

// Enable godoc
// @Summary Enable dish
// @Tags dish
// @Produce json
// @Param id path int true "Dish ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/dishes/{id}/enable [post]
func (h *DishHandler) Enable(c *gin.Context) { h.setStatus(c, service.DishStatusActive) }

// Disable godoc
// @Summary Disable dish
// @Tags dish
// @Produce json
// @Param id path int true "Dish ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/dishes/{id}/disable [post]
func (h *DishHandler) Disable(c *gin.Context) { h.setStatus(c, service.DishStatusDisabled) }
