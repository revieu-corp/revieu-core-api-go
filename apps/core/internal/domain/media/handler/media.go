package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/media/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/media/service"
)

type MediaHandler struct {
	svc *service.MediaService
}

func NewMediaHandler(svc *service.MediaService) *MediaHandler {
	if svc == nil {
		svc = service.NewMediaService(nil, nil)
	}
	return &MediaHandler{svc: svc}
}

// CreateMediaUpload godoc
// @Summary Create media upload
// @Description Creates a media upload and returns upload URLs
// @Tags media
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /media/uploads [post]
func (h *MediaHandler) CreateUpload(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	upload, err := h.svc.CreateUpload(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrUnauthorized) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, upload)
}

// AnalyzeMedia godoc
// @Summary Analyze media upload
// @Description Triggers analysis for a media upload
// @Tags media
// @Produce json
// @Param id path int true "Upload ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /media/{id}/analysis [post]
func (h *MediaHandler) Analyze(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := h.svc.Analyze(c.Request.Context(), userID, id); err != nil {
		if errors.Is(err, service.ErrUnauthorized) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// CreatePresignedURLs godoc
// @Summary Create presigned URLs for media upload
// @Description Generates presigned URLs for uploading files directly to R2 storage
// @Tags media
// @Accept json
// @Produce json
// @Param request body dto.PresignedURLRequest true "Files to upload"
// @Success 200 {object} dto.PresignedURLResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /media/presigned-urls [post]
func (h *MediaHandler) CreatePresignedURLs(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req dto.PresignedURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	response, err := h.svc.CreatePresignedURLs(c.Request.Context(), userID, &req)
	if err != nil {
		if errors.Is(err, service.ErrUnauthorized) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		if errors.Is(err, service.ErrTooManyFiles) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrInvalidContentType) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}
