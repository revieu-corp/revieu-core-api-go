package handler

import (
	"net/http"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/admin/service"
	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	svc *service.AdminService
}

func NewAdminHandler(svc *service.AdminService) *AdminHandler {
	if svc == nil {
		svc = service.NewAdminService(nil)
	}
	return &AdminHandler{svc: svc}
}

// ListReports godoc
// @Summary List reports
// @Description Returns a list of user reports for admin review
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /admin/reports [get]
func (h *AdminHandler) ListReports(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "not implemented"})
}

// UpdateReport godoc
// @Summary Update report
// @Description Updates a report status (approve/reject)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "Report ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /admin/reports/{id} [patch]
func (h *AdminHandler) UpdateReport(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "not implemented"})
}

// ListMerchants godoc
// @Summary List merchants for admin
// @Description Returns a list of merchants for admin management
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /admin/merchants [get]
func (h *AdminHandler) ListMerchants(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "not implemented"})
}

// UpdateMerchant godoc
// @Summary Update merchant status
// @Description Updates a merchant's status or verification
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "Merchant ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /admin/merchants/{id} [patch]
func (h *AdminHandler) UpdateMerchant(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "not implemented"})
}
