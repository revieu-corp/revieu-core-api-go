package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/payment/service"
)

type PaymentHandler struct {
	svc *service.PaymentService
}

func NewPaymentHandler(svc *service.PaymentService) *PaymentHandler {
	if svc == nil {
		svc = service.NewPaymentService(nil)
	}
	return &PaymentHandler{svc: svc}
}

// CreatePayment godoc
// @Summary Create payment
// @Description Creates a payment record
// @Tags payment
// @Accept json
// @Produce json
// @Param request body service.CreatePaymentRequest true "Create payment request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /payments [post]
func (h *PaymentHandler) Create(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req service.CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		status, message := paymentErrorStatus(err)
		c.JSON(status, gin.H{"error": message})
		return
	}
	c.JSON(http.StatusCreated, p)
}

// PaymentDetail godoc
// @Summary Get payment detail
// @Description Returns a payment by ID
// @Tags payment
// @Produce json
// @Param id path int true "Payment ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /payments/{id} [get]
func (h *PaymentHandler) Detail(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	p, err := h.svc.Detail(c.Request.Context(), userID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, p)
}

func paymentErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrPaymentInvalidInput):
		return http.StatusBadRequest, "invalid payment input"
	case errors.Is(err, service.ErrPaymentOrderNotFound):
		return http.StatusNotFound, "order not found"
	case errors.Is(err, service.ErrPaymentForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, service.ErrPaymentOrderAlreadyPaid):
		return http.StatusConflict, "order already paid"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
