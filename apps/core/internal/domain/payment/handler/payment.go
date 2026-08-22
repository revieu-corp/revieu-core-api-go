package handler

import (
	"net/http"
	"strconv"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/payment/service"
	"github.com/gin-gonic/gin"
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
// @Security BearerAuth
// @Router /payments [post]
func (h *PaymentHandler) Create(c *gin.Context) {
	var req service.CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
// @Security BearerAuth
// @Router /payments/{id} [get]
func (h *PaymentHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	p, err := h.svc.Detail(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, p)
}
