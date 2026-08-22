package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/order/service"
)

type OrderHandler struct {
	svc *service.OrderService
}

func NewOrderHandler(svc *service.OrderService) *OrderHandler {
	if svc == nil {
		svc = service.NewOrderService(nil)
	}
	return &OrderHandler{svc: svc}
}

// CreateOrder godoc
// @Summary Create order
// @Description Creates a new order for the authenticated user
// @Tags order
// @Accept json
// @Produce json
// @Param request body service.CreateOrderInput true "Create order request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /orders [post]
func (h *OrderHandler) Create(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req service.CreateOrderInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		status, msg := orderErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": order})
}

// ListOrders godoc
// @Summary List orders
// @Description Returns orders for the authenticated user
// @Tags order
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /orders [get]
func (h *OrderHandler) List(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	orders, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list orders"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": orders})
}

// OrderDetail godoc
// @Summary Get order detail
// @Description Returns an order by ID
// @Tags order
// @Produce json
// @Param id path int true "Order ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /orders/{id} [get]
func (h *OrderHandler) Detail(c *gin.Context) {
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
	detail, err := h.svc.Detail(c.Request.Context(), userID, id)
	if err != nil {
		status, msg := orderErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": detail})
}

// PayOrder godoc
// @Summary Pay order
// @Description Completes the configured development payment flow for an order and issues vouchers
// @Tags order
// @Produce json
// @Param id path int true "Order ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /orders/{id}/pay [post]
func (h *OrderHandler) Pay(c *gin.Context) {
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
	result, err := h.svc.Pay(c.Request.Context(), userID, id)
	if err != nil {
		status, msg := orderErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func orderErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrOrderNotFound), errors.Is(err, service.ErrCouponNotFound), errors.Is(err, service.ErrStoreNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, service.ErrOrderForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, service.ErrOrderInvalidInput):
		return http.StatusBadRequest, "invalid order input"
	case errors.Is(err, service.ErrOrderInvalidState):
		return http.StatusBadRequest, "invalid order state"
	case errors.Is(err, service.ErrStoreNotPublished):
		return http.StatusBadRequest, "store not published"
	case errors.Is(err, service.ErrCouponInactive):
		return http.StatusBadRequest, "coupon inactive"
	case errors.Is(err, service.ErrCouponNotStarted):
		return http.StatusBadRequest, "coupon not started"
	case errors.Is(err, service.ErrCouponExpired):
		return http.StatusBadRequest, "coupon expired"
	case errors.Is(err, service.ErrCouponSoldOut):
		return http.StatusBadRequest, "coupon sold out"
	case errors.Is(err, service.ErrCouponNotStoreScope):
		return http.StatusBadRequest, "coupon must be store scoped"
	case errors.Is(err, service.ErrCouponStoreMismatch):
		return http.StatusBadRequest, "coupon store mismatch"
	case errors.Is(err, service.ErrCouponPerUserLimit):
		return http.StatusBadRequest, "coupon per-user limit exceeded"
	case errors.Is(err, service.ErrPaymentProviderUnavailable):
		return http.StatusServiceUnavailable, "payment provider unavailable"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
