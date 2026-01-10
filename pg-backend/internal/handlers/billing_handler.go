package handlers

import (
	"net/http"
	"strconv"

	"pg-backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type BillingHandler struct {
	billingService services.BillingService
}

func NewBillingHandler(billingService services.BillingService) *BillingHandler {
	return &BillingHandler{
		billingService: billingService,
	}
}

// CreateManualPaymentRequest represents manual payment request
type CreateManualPaymentRequest struct {
	UserID      string  `json:"user_id" binding:"required,uuid4"`
	CardID      string  `json:"card_id" binding:"required,uuid4"`
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	Currency    string  `json:"currency" binding:"required,iso4217"`
	Description string  `json:"description,omitempty"`
}

// CreateManualPayment creates a manual payment
func (h *BillingHandler) CreateManualPayment(c *gin.Context) {
	var req CreateManualPaymentRequest
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

	cardID, err := uuid.Parse(req.CardID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid card ID"})
		return
	}

	// Default currency to LKR if not specified or invalid
	if req.Currency == "" {
		req.Currency = "LKR"
	}

	transaction, err := h.billingService.CreateManualPayment(
		c.Request.Context(),
		userID,
		cardID,
		req.Amount,
		req.Currency,
		req.Description,
	)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case err.Error() == "card does not belong to user":
			status = http.StatusForbidden
		case err.Error() == "user not found":
			status = http.StatusNotFound
		case err.Error() == "payment declined":
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, transaction)
}

// GetBillingHistory gets billing history for a user
func (h *BillingHandler) GetBillingHistory(c *gin.Context) {
	userID := c.Param("user_id")

	uid, err := uuid.Parse(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	// Parse pagination parameters
	limit := 50
	offset := 0

	if limitStr := c.DefaultQuery("limit", "50"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 100 {
				l = 100 // Max 100 records per request
			}
			limit = l
		}
	}

	if offsetStr := c.DefaultQuery("offset", "0"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	transactions, err := h.billingService.GetBillingHistory(c.Request.Context(), uid, limit, offset)
	if err != nil {
		if err.Error() == "user not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := gin.H{
		"transactions": transactions,
		"pagination": gin.H{
			"limit":  limit,
			"offset": offset,
			"count":  len(transactions),
		},
	}

	c.JSON(http.StatusOK, response)
}

// GetSubscriptionBillingHistory gets billing history for a subscription
func (h *BillingHandler) GetSubscriptionBillingHistory(c *gin.Context) {
	subscriptionID := c.Param("id")

	id, err := uuid.Parse(subscriptionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subscription ID"})
		return
	}

	attempts, err := h.billingService.GetSubscriptionBillingHistory(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, attempts)
}

// ProcessBillingAttempts processes pending billing attempts (admin endpoint)
func (h *BillingHandler) ProcessBillingAttempts(c *gin.Context) {
	limit := 50
	if limitStr := c.DefaultQuery("limit", "50"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 100 {
				l = 100
			}
			limit = l
		}
	}

	processed, err := h.billingService.ProcessPendingBillingAttempts(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "Billing attempts processed",
		"processed_count": processed,
		"limit":           limit,
	})
}
