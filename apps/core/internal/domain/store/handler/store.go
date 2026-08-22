package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/store/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/store/service"
)

type StoreHandler struct {
	svc *service.StoreService
}

func NewStoreHandler(svc *service.StoreService) *StoreHandler {
	if svc == nil {
		svc = service.NewStoreService(nil)
	}
	return &StoreHandler{svc: svc}
}

// ListStores godoc
// @Summary List stores
// @Description Returns a list of stores
// @Tags store
// @Produce json
// @Param category query string false "Category name or ID"
// @Param lat query number false "Latitude"
// @Param lng query number false "Longitude"
// @Param rating query number false "Minimum average rating"
// @Param radius_km query number false "Search radius in KM (default 20)"
// @Param cursor query int false "Cursor for pagination (store id)"
// @Param limit query int false "Page size (max 100)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /stores [get]
func (h *StoreHandler) List(c *gin.Context) {
	query, err := parseStoreListQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	stores, cursor, err := h.svc.ListPublishedFiltered(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list stores"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stores, "cursor": cursor})
}

// StoreDetail godoc
// @Summary Get store detail
// @Description Returns a store by ID
// @Tags store
// @Produce json
// @Param id path int true "Store ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /stores/{id} [get]
func (h *StoreHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}
	store, err := h.svc.DetailPublished(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrStoreNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load store"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": store})
}

// StoreReviews godoc
// @Summary Get store reviews
// @Description Returns reviews for a store
// @Tags store
// @Produce json
// @Param id path int true "Store ID"
// @Param cursor query int false "Cursor for pagination (review id)"
// @Param limit query int false "Page size (max 100)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /stores/{id}/reviews [get]
func (h *StoreHandler) Reviews(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}
	query, err := parseStoreReviewListQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	reviews, cursor, err := h.svc.ReviewsPublishedPaginatedForViewer(c.Request.Context(), id, query, c.GetInt64("user_id"))
	if err != nil {
		if errors.Is(err, service.ErrStoreNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load reviews"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reviews, "cursor": cursor})
}

// StoreHours godoc
// @Summary Get store hours
// @Description Returns operating hours for a store
// @Tags store
// @Produce json
// @Param id path int true "Store ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /stores/{id}/hours [get]
func (h *StoreHandler) Hours(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}
	hours, err := h.svc.HoursPublished(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrStoreNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load hours"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": hours})
}

// ListMerchantStores godoc
// @Summary List current merchant stores
// @Description Returns stores owned by the authenticated merchant user
// @Tags store
// @Produce json
// @Param limit query int false "Page size (max 100)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/stores [get]
func (h *StoreHandler) ListMine(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	limit, hasLimit, err := parseIntQuery(c, "limit")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var limitPtr *int
	if hasLimit {
		limitPtr = &limit
	}

	stores, err := h.svc.ListMine(c.Request.Context(), userID, limitPtr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list stores"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stores})
}

// CreateStore godoc
// @Summary Create a store
// @Description Creates a new store for the authenticated merchant
// @Tags store
// @Accept json
// @Produce json
// @Param request body dto.CreateStoreRequest false "Create store request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/stores [post]
func (h *StoreHandler) Create(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req dto.CreateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	store, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		if errors.Is(err, service.ErrCategoryNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category ids"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create store"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": store})
}

// UpdateStore godoc
// @Summary Update a store
// @Description Updates a store for the authenticated merchant
// @Tags store
// @Accept json
// @Produce json
// @Param id path int true "Store ID"
// @Param request body dto.UpdateStoreRequest false "Update store request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/stores/{id} [patch]
func (h *StoreHandler) Update(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	storeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}

	var req dto.UpdateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	store, err := h.svc.Update(c.Request.Context(), userID, storeID, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrStoreNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.Is(err, service.ErrStoreForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, service.ErrCategoryNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category ids"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update store"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": store})
}

// ActivateStore godoc
// @Summary Activate a store
// @Description Marks a merchant-owned store as published
// @Tags store
// @Produce json
// @Param id path int true "Store ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/stores/{id}/activate [post]
func (h *StoreHandler) Activate(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	storeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}
	if err := h.svc.Activate(c.Request.Context(), userID, storeID); err != nil {
		switch {
		case errors.Is(err, service.ErrStoreNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.Is(err, service.ErrStoreForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate store"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// DeactivateStore godoc
// @Summary Deactivate a store
// @Description Marks a merchant-owned store as hidden
// @Tags store
// @Produce json
// @Param id path int true "Store ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/stores/{id}/deactivate [post]
func (h *StoreHandler) Deactivate(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	storeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}
	if err := h.svc.Deactivate(c.Request.Context(), userID, storeID); err != nil {
		switch {
		case errors.Is(err, service.ErrStoreNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.Is(err, service.ErrStoreForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to deactivate store"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// DeleteStore godoc
// @Summary Delete a store
// @Description Soft-deletes a merchant-owned store and its bound coupons
// @Tags store
// @Produce json
// @Param id path int true "Store ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/stores/{id} [delete]
func (h *StoreHandler) Delete(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	storeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}

	if err := h.svc.Delete(c.Request.Context(), userID, storeID); err != nil {
		switch {
		case errors.Is(err, service.ErrStoreNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.Is(err, service.ErrStoreForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete store"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func parseStoreListQuery(c *gin.Context) (dto.StoreListQuery, error) {
	var q dto.StoreListQuery

	if category := c.Query("category"); category != "" {
		q.Category = &category
	}

	lat, hasLat, err := parseFloat64Query(c, "lat")
	if err != nil {
		return q, err
	}
	lng, hasLng, err := parseFloat64Query(c, "lng")
	if err != nil {
		return q, err
	}
	if hasLat != hasLng {
		return q, errors.New("lat and lng must be provided together")
	}
	if hasLat {
		q.Lat = &lat
		q.Lng = &lng
	}

	rating, hasRating, err := parseFloat32Query(c, "rating")
	if err != nil {
		return q, err
	}
	if hasRating {
		q.Rating = &rating
	}

	radiusKM, hasRadiusKM, err := parseFloat64Query(c, "radius_km")
	if err != nil {
		return q, err
	}
	if hasRadiusKM {
		if radiusKM <= 0 {
			return q, errors.New("radius_km must be greater than 0")
		}
		q.RadiusKM = &radiusKM
	}

	cursor, hasCursor, err := parseInt64Query(c, "cursor")
	if err != nil {
		return q, err
	}
	if hasCursor {
		q.Cursor = &cursor
	}

	limit, hasLimit, err := parseIntQuery(c, "limit")
	if err != nil {
		return q, err
	}
	if hasLimit {
		q.Limit = &limit
	}

	return q, nil
}

func parseStoreReviewListQuery(c *gin.Context) (dto.StoreReviewListQuery, error) {
	var q dto.StoreReviewListQuery

	cursor, hasCursor, err := parseInt64Query(c, "cursor")
	if err != nil {
		return q, err
	}
	if hasCursor {
		q.Cursor = &cursor
	}

	limit, hasLimit, err := parseIntQuery(c, "limit")
	if err != nil {
		return q, err
	}
	if hasLimit {
		q.Limit = &limit
	}
	return q, nil
}

func parseIntQuery(c *gin.Context, key string) (int, bool, error) {
	raw := c.Query(key)
	if raw == "" {
		return 0, false, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false, errors.New("invalid " + key)
	}
	return parsed, true, nil
}

func parseInt64Query(c *gin.Context, key string) (int64, bool, error) {
	raw := c.Query(key)
	if raw == "" {
		return 0, false, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false, errors.New("invalid " + key)
	}
	return parsed, true, nil
}

func parseFloat64Query(c *gin.Context, key string) (float64, bool, error) {
	raw := c.Query(key)
	if raw == "" {
		return 0, false, nil
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false, errors.New("invalid " + key)
	}
	return parsed, true, nil
}

func parseFloat32Query(c *gin.Context, key string) (float32, bool, error) {
	raw := c.Query(key)
	if raw == "" {
		return 0, false, nil
	}
	parsed, err := strconv.ParseFloat(raw, 32)
	if err != nil {
		return 0, false, errors.New("invalid " + key)
	}
	return float32(parsed), true, nil
}
