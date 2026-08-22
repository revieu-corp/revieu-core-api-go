package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/ai/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/ai/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/logger"
)

// Multipart and content limits applied before the request reaches the AI service.
// These mirror the constraints in the media domain and protect both the model
// (token budget) and the process (memory).
const (
	maxRequestBytes      = 25 << 20 // 25 MiB total payload cap
	maxImageCount        = 6
	maxPerImageBytes     = 5 << 20 // 5 MiB per image
	minTextLength        = 10
	maxTextLength        = 4000
	defaultTimeoutSecond = 30
)

// allowedImageContentTypes mirrors the allow-list used by internal/domain/media/service.
var allowedImageContentTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/gif":  {},
	"image/webp": {},
}

type AIHandler struct {
	svc   *service.AIService
	cfg   config.GeminiConfig
	quota *quotaStore
}

func NewAIHandler(svc *service.AIService, cfg config.GeminiConfig) *AIHandler {
	return &AIHandler{svc: svc, cfg: cfg, quota: newQuotaStore(database.DB, cfg.Quota)}
}

// Suggestions godoc
// @Summary Polish a draft review with AI
// @Description Sends a user-written draft review (text and optional images) to Gemini and returns three polished candidates. The response contains text only; images are processed for context but never returned.
// @Tags ai
// @Accept multipart/form-data
// @Produce json
// @Param text formData string true "Draft review text (10-4000 chars after trim)"
// @Param images formData file false "Image attachment (repeat for multiple, up to 6, 5 MiB each, jpeg/png/gif/webp)"
// @Param merchantName formData string false "Optional merchant name for context"
// @Param storeName formData string false "Optional store name for context"
// @Param businessCategory formData string false "Optional business category, e.g. restaurant, cafe"
// @Param language formData string false "Optional output language hint, e.g. en, zh"
// @Param ratingOverall formData number false "Optional overall rating 0-5"
// @Param ratingService formData number false "Optional service rating 0-5"
// @Param ratingEnvironment formData number false "Optional environment rating 0-5"
// @Param ratingValue formData number false "Optional value rating 0-5"
// @Param ratingFood formData number false "Optional food rating 0-5"
// @Success 200 {object} dto.PolishResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 422 {object} map[string]string
// @Failure 429 {object} map[string]string
// @Failure 502 {object} map[string]string
// @Router /ai/reviews/suggestions [post]
func (h *AIHandler) Suggestions(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if h.quota != nil {
		if err := h.quota.allow(c.Request.Context(), userID, c.ClientIP(), time.Now().UTC()); err != nil {
			logger.Warn(c.Request.Context(), "ai.review.limit_denied", "error", err.Error(), "user_id", userID)
			writePolishError(c, err)
			return
		}
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)

	if err := c.Request.ParseMultipartForm(maxRequestBytes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart payload: " + err.Error()})
		return
	}

	req, err := parsePolishRequest(c.Request.MultipartForm)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	timeout := time.Duration(h.cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeoutSecond * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	start := time.Now()
	resp, err := h.svc.PolishReview(ctx, req)
	latency := time.Since(start)

	logger.Info(c.Request.Context(), "ai.review.polish",
		"model", h.cfg.Model,
		"image_count", len(req.Images),
		"text_length", len(req.Text),
		"latency_ms", latency.Milliseconds(),
		"ok", err == nil,
	)

	if err != nil {
		writePolishError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// parsePolishRequest validates the multipart form and converts it into the service-layer DTO.
// It returns a single error message describing the first validation failure encountered.
func parsePolishRequest(form *multipart.Form) (dto.PolishRequest, error) {
	if form == nil {
		return dto.PolishRequest{}, errors.New("missing multipart form")
	}

	text := strings.TrimSpace(formValue(form, "text"))
	if l := len(text); l < minTextLength || l > maxTextLength {
		return dto.PolishRequest{}, fmt.Errorf("text length must be %d-%d characters (got %d)", minTextLength, maxTextLength, l)
	}

	rating, err := parseRating(form)
	if err != nil {
		return dto.PolishRequest{}, err
	}

	images, err := readImages(form.File["images"])
	if err != nil {
		return dto.PolishRequest{}, err
	}

	return dto.PolishRequest{
		Text:             text,
		Images:           images,
		MerchantName:     strings.TrimSpace(formValue(form, "merchantName")),
		StoreName:        strings.TrimSpace(formValue(form, "storeName")),
		BusinessCategory: strings.TrimSpace(formValue(form, "businessCategory")),
		Language:         strings.TrimSpace(formValue(form, "language")),
		Rating:           rating,
	}, nil
}

func formValue(form *multipart.Form, key string) string {
	if vs := form.Value[key]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

func parseRating(form *multipart.Form) (dto.PolishRating, error) {
	parse := func(key string) (*float64, error) {
		raw := strings.TrimSpace(formValue(form, key))
		if raw == "" {
			return nil, nil
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("%s must be a number", key)
		}
		if v < 0 || v > 5 {
			return nil, fmt.Errorf("%s must be between 0 and 5", key)
		}
		return &v, nil
	}

	var (
		r   dto.PolishRating
		err error
	)
	if r.Overall, err = parse("ratingOverall"); err != nil {
		return r, err
	}
	if r.Service, err = parse("ratingService"); err != nil {
		return r, err
	}
	if r.Environment, err = parse("ratingEnvironment"); err != nil {
		return r, err
	}
	if r.Value, err = parse("ratingValue"); err != nil {
		return r, err
	}
	if r.Food, err = parse("ratingFood"); err != nil {
		return r, err
	}
	return r, nil
}

func readImages(headers []*multipart.FileHeader) ([]dto.PolishImage, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	if len(headers) > maxImageCount {
		return nil, fmt.Errorf("too many images: max %d, got %d", maxImageCount, len(headers))
	}

	images := make([]dto.PolishImage, 0, len(headers))
	for _, h := range headers {
		mime := strings.ToLower(h.Header.Get("Content-Type"))
		if _, ok := allowedImageContentTypes[mime]; !ok {
			return nil, fmt.Errorf("unsupported image type %q (allowed: image/jpeg, image/png, image/gif, image/webp)", mime)
		}
		if h.Size > maxPerImageBytes {
			return nil, fmt.Errorf("image %q exceeds %d bytes", h.Filename, maxPerImageBytes)
		}
		data, err := readMultipartFile(h)
		if err != nil {
			return nil, fmt.Errorf("failed to read image %q: %w", h.Filename, err)
		}
		images = append(images, dto.PolishImage{MIMEType: mime, Data: data})
	}
	return images, nil
}

func readMultipartFile(h *multipart.FileHeader) ([]byte, error) {
	f, err := h.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxPerImageBytes+1))
}

// writePolishError maps service/client error sentinels to HTTP statuses.
func writePolishError(c *gin.Context, err error) {
	var safety *service.SafetyBlockError
	var quotaErr *quotaLimitError
	switch {
	case errors.As(err, &quotaErr):
		retryAfter := quotaErr.RetryAfterSeconds()
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":               quotaErr.Error(),
			"code":                quotaErrorCode(quotaErr),
			"retry_after_seconds": retryAfter,
		})
	case errors.Is(err, ErrAIQuotaStoreUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI quota service unavailable", "code": "AI_QUOTA_UNAVAILABLE"})
	case errors.As(err, &safety):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": safety.Error()})
	case errors.Is(err, service.ErrGeminiRateLimited):
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrEmptyText):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrGeminiInsufficientOutput),
		errors.Is(err, service.ErrInsufficientCandidates),
		errors.Is(err, service.ErrGeminiInvalidResponse),
		errors.Is(err, service.ErrGeminiUpstream),
		errors.Is(err, context.DeadlineExceeded):
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	}
}

func quotaErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrAIUserRateLimited):
		return "AI_USER_RATE_LIMITED"
	case errors.Is(err, ErrAIIPRateLimited):
		return "AI_IP_RATE_LIMITED"
	case errors.Is(err, ErrAIGlobalLimited):
		return "AI_GLOBAL_RATE_LIMITED"
	case errors.Is(err, ErrAIDailyQuota):
		return "AI_DAILY_QUOTA_EXCEEDED"
	case errors.Is(err, ErrAIMonthlyQuota):
		return "AI_MONTHLY_QUOTA_EXCEEDED"
	default:
		return "AI_RATE_LIMITED"
	}
}
