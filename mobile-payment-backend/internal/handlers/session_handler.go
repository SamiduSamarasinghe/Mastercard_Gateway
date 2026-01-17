package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"mobile-payment-backend/internal/models"
	"mobile-payment-backend/internal/repositories"
	"mobile-payment-backend/internal/services"
)

type SessionHandler struct {
	gatewayService services.GatewayService
	orderRepo      repositories.OrderRepository
	sessionRepo    repositories.SessionRepository
	cfg            *models.MobileSDKConfig
}

func NewSessionHandler(
	gatewayService services.GatewayService,
	orderRepo repositories.OrderRepository,
	sessionRepo repositories.SessionRepository,
	cfg *models.MobileSDKConfig,
) *SessionHandler {
	return &SessionHandler{
		gatewayService: gatewayService,
		orderRepo:      orderRepo,
		sessionRepo:    sessionRepo,
		cfg:            cfg,
	}
}

// CreateSessionRequest from mobile app
type CreateSessionRequest struct {
	OrderReferenceID string `json:"order_reference_id" binding:"required"`
	// Amount           float64 `json:"amount" binding:"required,min=0.01"`
	// Currency         string  `json:"currency" binding:"required,len=3"`
	UserID string `json:"user_id,omitempty"`
}

// CreateSession creates a new payment session
func (h *SessionHandler) CreateSession(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 1. Get order from database using reference_id
	order, err := h.orderRepo.GetByReferenceID(c.Request.Context(), req.OrderReferenceID)
	if err != nil {
		if _, ok := err.(*repositories.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 2. Verify user owns this order (if user_id provided)
	if req.UserID != "" {
		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
			return
		}

		if order.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "order does not belong to user"})
			return
		}
	}

	// 3. Create session in gateway
	session, err := h.gatewayService.CreateSession(order, 25)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to create payment session",
			"details": err.Error(),
		})
		return
	}

	// ✅ ADD THIS - Update session with order details in Gateway
	amountStr := fmt.Sprintf("%.2f", order.Amount)
	err = h.gatewayService.UpdateSession(
		session.GatewayID,
		order.ReferenceID, // Use ReferenceID as Gateway Order ID
		amountStr,
		order.Currency,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to update session with order details",
			"details": err.Error(),
		})
		return
	}

	// 4. Save session to database (link to order)
	session.OrderDBID = order.ID // Link to order UUID
	if err := h.sessionRepo.Create(c.Request.Context(), session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to save session",
			"details": err.Error(),
		})
		return
	}

	// 5. Prepare response
	response := models.SessionResponse{
		SessionID:  session.GatewayID,
		OrderID:    order.ReferenceID, // Return human-readable ID
		Amount:     formatAmount(session.Amount),
		Currency:   session.Currency,
		APIVersion: session.APIVersion,
		SDKConfig:  *h.cfg,
	}

	c.JSON(http.StatusCreated, gin.H{
		"success":    true,
		"session":    response,
		"expires_at": session.ExpiresAt.Format(time.RFC3339),
	})
}

// GetSDKConfig returns SDK configuration for mobile app initialization
func (h *SessionHandler) GetSDKConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"config":  h.cfg,
	})
}

// VerifySession verifies if session is still valid
func (h *SessionHandler) VerifySession(c *gin.Context) {
	sessionID := c.Param("session_id")

	// In real implementation, check in database
	// session, err := sessionRepo.GetByGatewayID(sessionID)

	// For now, return mock
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"valid":      true,
		"session_id": sessionID,
	})
}

func formatAmount(amount float64) string {
	// Format amount as string with 2 decimal places
	return fmt.Sprintf("%.2f", amount)
}
