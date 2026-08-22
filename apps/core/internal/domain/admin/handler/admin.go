package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/admin/service"
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
// @Description Returns a paginated list of user reports for admin review
// @Tags admin
// @Produce json
// @Param status query string false "Report status filter"
// @Param limit query int false "Page size (1-100)" default(20)
// @Param cursor query int false "ID cursor for the next page"
// @Success 200 {object} service.ReportPage
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Router /admin/reports [get]
func (h *AdminHandler) ListReports(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}

	query, err := parseReportQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report query"})
		return
	}
	page, err := h.svc.ListReports(c.Request.Context(), query)
	if err != nil {
		if errors.Is(err, service.ErrAdminInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report query"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list reports"})
		return
	}
	c.JSON(http.StatusOK, page)
}

// UpdateReport godoc
// @Summary Update report
// @Description Resolves or dismisses a report and records an admin audit log
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "Report ID"
// @Param body body service.UpdateReportInput true "Report resolution"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Router /admin/reports/{id} [patch]
func (h *AdminHandler) UpdateReport(c *gin.Context) {
	adminID, ok := requireAdmin(c)
	if !ok {
		return
	}

	reportID, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report id"})
		return
	}
	var input service.UpdateReportInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report payload"})
		return
	}

	report, err := h.svc.UpdateReport(c.Request.Context(), adminID, reportID, input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report update"})
		case errors.Is(err, service.ErrAdminNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "report not found"})
		case errors.Is(err, service.ErrAdminInvalidState):
			c.JSON(http.StatusConflict, gin.H{"error": "report is already resolved"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update report"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": report})
}

// ListMerchants godoc
// @Summary List merchants for admin
// @Description Returns a paginated list of merchants for admin management
// @Tags admin
// @Produce json
// @Param status query int false "Merchant status (0 active, 1 banned, 2 pending)"
// @Param verification_status query string false "Verification status filter"
// @Param limit query int false "Page size (1-100)" default(20)
// @Param cursor query int false "ID cursor for the next page"
// @Success 200 {object} service.MerchantPage
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Router /admin/merchants [get]
func (h *AdminHandler) ListMerchants(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}

	query, err := parseMerchantQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant query"})
		return
	}
	page, err := h.svc.ListMerchants(c.Request.Context(), query)
	if err != nil {
		if errors.Is(err, service.ErrAdminInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant query"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list merchants"})
		return
	}
	c.JSON(http.StatusOK, page)
}

// UpdateMerchant godoc
// @Summary Update merchant status
// @Description Updates a merchant status or verification status and records an admin audit log
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "Merchant ID"
// @Param body body service.UpdateMerchantInput true "Merchant update"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /admin/merchants/{id} [patch]
func (h *AdminHandler) UpdateMerchant(c *gin.Context) {
	adminID, ok := requireAdmin(c)
	if !ok {
		return
	}

	merchantID, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant id"})
		return
	}
	var input service.UpdateMerchantInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant payload"})
		return
	}

	merchant, err := h.svc.UpdateMerchant(c.Request.Context(), adminID, merchantID, input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant update"})
		case errors.Is(err, service.ErrAdminNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "merchant not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update merchant"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": merchant})
}

func requireAdmin(c *gin.Context) (int64, bool) {
	userID := c.GetInt64(authorization.UserIDKey)
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return 0, false
	}
	if !strings.EqualFold(strings.TrimSpace(c.GetString(authorization.UserRoleKey)), "admin") {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return 0, false
	}
	return userID, true
}

func parseReportQuery(c *gin.Context) (service.ListReportsQuery, error) {
	limit, cursor, err := parsePageQuery(c)
	if err != nil {
		return service.ListReportsQuery{}, err
	}
	return service.ListReportsQuery{
		Status: c.Query("status"),
		Limit:  limit,
		Cursor: cursor,
	}, nil
}

func parseMerchantQuery(c *gin.Context) (service.ListMerchantsQuery, error) {
	limit, cursor, err := parsePageQuery(c)
	if err != nil {
		return service.ListMerchantsQuery{}, err
	}

	query := service.ListMerchantsQuery{
		VerificationStatus: c.Query("verification_status"),
		Limit:              limit,
		Cursor:             cursor,
	}
	if rawStatus := c.Query("status"); rawStatus != "" {
		value, err := strconv.ParseInt(rawStatus, 10, 16)
		if err != nil {
			return service.ListMerchantsQuery{}, err
		}
		status := int16(value)
		query.Status = &status
	}
	return query, nil
}

func parsePageQuery(c *gin.Context) (int, int64, error) {
	limit := 0
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			return 0, 0, err
		}
		limit = parsed
	}

	cursor := int64(0)
	if rawCursor := c.Query("cursor"); rawCursor != "" {
		parsed, err := strconv.ParseInt(rawCursor, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		cursor = parsed
	}
	return limit, cursor, nil
}

func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
