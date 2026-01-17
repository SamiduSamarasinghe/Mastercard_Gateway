package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"mobile-payment-backend/internal/models"
	"mobile-payment-backend/internal/services"
)

type PaymentHandler struct {
	gatewayService services.GatewayService
}

func NewPaymentHandler(gatewayService services.GatewayService) *PaymentHandler {
	return &PaymentHandler{
		gatewayService: gatewayService,
	}
}

// ProcessPaymentRequest from mobile app
type ProcessPaymentRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	// OrderID   string `json:"order_id" binding:"required"`
	Operation string `json:"operation" binding:"required,oneof=PAY AUTHORIZE"`
	Amount    string `json:"amount,omitempty"`
	Currency  string `json:"currency,omitempty"`
}

// ProcessPayment processes the final payment
func (h *PaymentHandler) ProcessPayment(c *gin.Context) {
	var req ProcessPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	paymentReq := &models.PaymentRequest{
		SessionID: req.SessionID,
		// OrderID:   req.OrderID,
		Operation: req.Operation,
		Amount:    req.Amount,
		Currency:  req.Currency,
	}

	// Process payment through gateway
	paymentResp, err := h.gatewayService.ProcessPayment(paymentReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "payment processing failed",
			"details": err.Error(),
		})
		return
	}

	// Save transaction to database
	// transactionRepo.Save(paymentResp)

	c.JSON(http.StatusOK, gin.H{
		"success":   paymentResp.Success,
		"payment":   paymentResp,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// RefundPayment handles refunds
func (h *PaymentHandler) RefundPayment(c *gin.Context) {
	var req struct {
		OrderID       string `json:"order_id" binding:"required"`
		TransactionID string `json:"transaction_id" binding:"required"`
		Amount        string `json:"amount" binding:"required"`
		Currency      string `json:"currency" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Refund logic here
	// Similar to ProcessPayment but with REFUND operation

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "Refund initiated",
		"refund_id": uuid.New().String(),
	})
}
