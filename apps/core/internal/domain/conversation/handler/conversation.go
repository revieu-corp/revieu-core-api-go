package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/conversation/service"
)

type ConversationHandler struct {
	svc *service.ConversationService
}

func NewConversationHandler(svc *service.ConversationService) *ConversationHandler {
	if svc == nil {
		svc = service.NewConversationService(nil)
	}
	return &ConversationHandler{svc: svc}
}

// ListConversations godoc
// @Summary List conversations
// @Description Returns conversations for the authenticated user
// @Tags conversation
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Router /conversations [get]
func (h *ConversationHandler) List(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	conversations, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list conversations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": conversations})
}

// CreateConversation godoc
// @Summary Create conversation
// @Description Creates a new conversation
// @Tags conversation
// @Accept json
// @Produce json
// @Success 201 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Router /conversations [post]
func (h *ConversationHandler) Create(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req service.CreateConversationInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	conversation, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		if errors.Is(err, service.ErrConversationInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation input"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create conversation"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": conversation})
}

// ConversationMessages godoc
// @Summary Get conversation messages
// @Description Returns messages for a conversation
// @Tags conversation
// @Produce json
// @Param id path int true "Conversation ID"
// @Param cursor query int false "Cursor (message id) for older messages"
// @Param limit query int false "Page size (max 100)"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /conversations/{id}/messages [get]
func (h *ConversationHandler) Messages(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	query, err := parseConversationMessageListQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	messages, cursor, err := h.svc.Messages(c.Request.Context(), userID, id, query)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrConversationForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, service.ErrConversationNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load messages"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": messages, "cursor": cursor})
}

func parseConversationMessageListQuery(c *gin.Context) (service.ConversationMessageListQuery, error) {
	query := service.ConversationMessageListQuery{Limit: service.DefaultConversationMessageLimit}
	if rawLimit := c.Query("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			return query, errors.New("limit must be a positive integer")
		}
		if limit > service.MaxConversationMessageLimit {
			limit = service.MaxConversationMessageLimit
		}
		query.Limit = limit
	}
	if rawCursor := c.Query("cursor"); rawCursor != "" {
		cursor, err := strconv.ParseInt(rawCursor, 10, 64)
		if err != nil || cursor <= 0 {
			return query, errors.New("cursor must be a positive integer")
		}
		query.Cursor = &cursor
	}
	return query, nil
}

// SendMessage godoc
// @Summary Send message
// @Description Sends a message in a conversation
// @Tags conversation
// @Accept json
// @Produce json
// @Param id path int true "Conversation ID"
// @Success 201 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Router /conversations/{id}/messages [post]
func (h *ConversationHandler) SendMessage(c *gin.Context) {
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

	var req service.SendMessageInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	message, err := h.svc.SendMessage(c.Request.Context(), userID, id, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrConversationForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, service.ErrConversationInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send message"})
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": message})
}

// UpdateConversationSettings godoc
// @Summary Update conversation settings
// @Description Updates settings for a conversation (e.g. mute)
// @Tags conversation
// @Accept json
// @Produce json
// @Param id path int true "Conversation ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Router /conversations/{id}/settings [patch]
func (h *ConversationHandler) UpdateSettings(c *gin.Context) {
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

	var req service.UpdateConversationSettingsInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	conversation, err := h.svc.UpdateSettings(c.Request.Context(), userID, id, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrConversationForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update settings"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": conversation})
}
