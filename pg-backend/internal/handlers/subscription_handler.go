package handlers

import (
	"net/http"

	"pg-backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type SubscriptionHandler struct {
	subscriptionService services.SubscriptionService
}

func NewSubscriptionHandler(subscriptionService services.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{
		subscriptionService: subscriptionService,
	}
}

// CreateSubscriptionRequest represents subscription creation request
type CreateSubscriptionRequest struct {
	UserID   string            `json:"user_id" binding:"required,uuid4"`
	PlanID   string            `json:"plan_id" binding:"required,uuid4"`
	CardID   string            `json:"card_id" binding:"required,uuid4"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// CreateSubscription creates a new subscription
func (h *SubscriptionHandler) CreateSubscription(c *gin.Context) {
	var req CreateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Parse UUIDs
	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	planID, err := uuid.Parse(req.PlanID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan ID"})
		return
	}

	cardID, err := uuid.Parse(req.CardID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid card ID"})
		return
	}

	subscription, err := h.subscriptionService.CreateSubscription(c.Request.Context(), userID, planID, cardID, req.Metadata)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case err.Error() == "user already has active subscription for this plan":
			status = http.StatusConflict
		case err.Error() == "card does not belong to user":
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, subscription)
}

// GetSubscription gets a subscription by ID
func (h *SubscriptionHandler) GetSubscription(c *gin.Context) {
	subscriptionID := c.Param("id")

	id, err := uuid.Parse(subscriptionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subscription ID"})
		return
	}

	subscription, err := h.subscriptionService.GetSubscription(c.Request.Context(), id)
	if err != nil {
		if _, ok := err.(*services.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, subscription)
}

// GetUserSubscriptions gets all subscriptions for a user
func (h *SubscriptionHandler) GetUserSubscriptions(c *gin.Context) {
	userID := c.Param("user_id")
	status := c.DefaultQuery("status", "")

	uid, err := uuid.Parse(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	subscriptions, err := h.subscriptionService.GetUserSubscriptions(c.Request.Context(), uid, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, subscriptions)
}

// CancelSubscriptionRequest represents subscription cancellation request
type CancelSubscriptionRequest struct {
	CancelAtPeriodEnd bool `json:"cancel_at_period_end"`
}

// CancelSubscription cancels a subscription
func (h *SubscriptionHandler) CancelSubscription(c *gin.Context) {
	subscriptionID := c.Param("id")

	id, err := uuid.Parse(subscriptionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subscription ID"})
		return
	}

	var req CancelSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.subscriptionService.CancelSubscription(c.Request.Context(), id, req.CancelAtPeriodEnd); err != nil {
		if _, ok := err.(*services.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	message := "Subscription cancelled"
	if req.CancelAtPeriodEnd {
		message = "Subscription will be cancelled at the end of the billing period"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
	})
}

// UpdateSubscriptionCardRequest represents subscription card update request
type UpdateSubscriptionCardRequest struct {
	CardID string `json:"card_id" binding:"required,uuid4"`
}

// UpdateSubscriptionCard updates the card for a subscription
func (h *SubscriptionHandler) UpdateSubscriptionCard(c *gin.Context) {
	subscriptionID := c.Param("id")

	subID, err := uuid.Parse(subscriptionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subscription ID"})
		return
	}

	var req UpdateSubscriptionCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cardID, err := uuid.Parse(req.CardID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid card ID"})
		return
	}

	if err := h.subscriptionService.UpdateSubscriptionCard(c.Request.Context(), subID, cardID); err != nil {
		if _, ok := err.(*services.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Subscription card updated successfully",
	})
}
