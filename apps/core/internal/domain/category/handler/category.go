package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/category/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
)

type CategoryHandler struct {
	svc *service.CategoryService
}

func NewCategoryHandler(svc *service.CategoryService) *CategoryHandler {
	if svc == nil {
		svc = service.NewCategoryService(nil)
	}
	return &CategoryHandler{svc: svc}
}

// ListCategories godoc
// @Summary List categories
// @Description Returns all categories as a parent-to-child hierarchy
// @Tags category
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /categories [get]
func (h *CategoryHandler) List(c *gin.Context) {
	categories, err := h.svc.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list categories"})
		return
	}
	if categories == nil {
		categories = []model.Category{}
	}
	c.JSON(http.StatusOK, gin.H{"data": categories})
}
