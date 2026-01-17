package handlers

import (
	"fmt"
	"net/http"

	"pg-backend/internal/models"
	"pg-backend/internal/repositories"
	"pg-backend/internal/services"
	"pg-backend/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type PaymentHandler struct {
	mastercardService services.MastercardService
	userRepo          repositories.UserRepository
	cardRepo          repositories.CardRepository
	transactionRepo   repositories.TransactionRepository
}

func NewPaymentHandler(
	mastercardService services.MastercardService,
	userRepo repositories.UserRepository,
	cardRepo repositories.CardRepository,
	transactionRepo repositories.TransactionRepository,
) *PaymentHandler {
	return &PaymentHandler{
		mastercardService: mastercardService,
		userRepo:          userRepo,
		cardRepo:          cardRepo,
		transactionRepo:   transactionRepo,
	}
}

// CreateUserRequest represents user creation request
type CreateUserRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// CreateUserResponse represents user creation response
type CreateUserResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt string    `json:"created_at"`
}

// PayRequest represents direct payment request
type PayRequest struct {
	UserID      string `json:"user_id" binding:"required,uuid4"`
	CardID      string `json:"card_id,omitempty"`     // Optional if using saved card
	CardNumber  string `json:"card_number,omitempty"` // Optional if using new card
	ExpiryMonth string `json:"expiry_month,omitempty"`
	ExpiryYear  string `json:"expiry_year,omitempty"`
	CVV         string `json:"cvv,omitempty"`
	Amount      string `json:"amount" binding:"required"`
	Currency    string `json:"currency" binding:"required"`
	Description string `json:"description,omitempty"`
}

// PayResponse represents payment response
type PayResponse struct {
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	TransactionID string `json:"transaction_id,omitempty"`
	OrderID       string `json:"order_id,omitempty"`
	Amount        string `json:"amount,omitempty"`
	Currency      string `json:"currency,omitempty"`
	Status        string `json:"status,omitempty"`
}

// CreateUser creates a new user
func (h *PaymentHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userRepo.CreateUser(c.Request.Context(), req.Email)
	if err != nil {
		status := http.StatusInternalServerError
		if _, ok := err.(*repositories.DuplicateError); ok {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	response := CreateUserResponse{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	c.JSON(http.StatusCreated, response)
}

// Pay processes a payment
func (h *PaymentHandler) Pay(c *gin.Context) {
	var req PayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate user exists
	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	_, err = h.userRepo.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if _, ok := err.(*repositories.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var paymentResp *services.PaymentResponse
	var cardID uuid.UUID
	var card *models.Card

	// Check if using saved card or new card
	if req.CardID != "" {
		// Pay with saved card (using token)
		cardID, err = uuid.Parse(req.CardID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid card ID"})
			return
		}

		// Get card from database
		card, err = h.cardRepo.GetCardByID(c.Request.Context(), cardID)
		if err != nil {
			if _, ok := err.(*repositories.NotFoundError); ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Verify card belongs to user
		if card.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "card does not belong to user"})
			return
		}

		// Pay with token
		paymentResp, err = h.mastercardService.PayWithToken(
			card.GatewayToken,
			req.Amount,
			req.Currency,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "payment failed",
				"details": err.Error(),
			})
			return
		}

	} else {
		// Pay with new card details
		if req.CardNumber == "" || req.ExpiryMonth == "" || req.ExpiryYear == "" || req.CVV == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "card details required when not using saved card"})
			return
		}

		paymentResp, err = h.mastercardService.PayWithCard(
			req.CardNumber,
			req.ExpiryMonth,
			req.ExpiryYear,
			req.CVV,
			req.Amount,
			req.Currency,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "payment failed",
				"details": err.Error(),
			})
			return
		}
	}

	// Validate payment response
	if paymentResp.Result != "SUCCESS" && paymentResp.GatewayCode != "APPROVED" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "payment declined",
			"code":   paymentResp.GatewayCode,
			"result": paymentResp.Result,
		})
		return
	}

	// Save transaction to database
	transaction := &models.Transaction{
		UserID:               userID,
		Amount:               utils.MustParseFloat(req.Amount),
		Currency:             req.Currency,
		Status:               paymentResp.Transaction.Status,
		GatewayTransactionID: paymentResp.Transaction.ID,
		Type:                 "manual",
	}

	// If using saved card, set card ID
	if req.CardID != "" {
		transaction.CardID = cardID
	}

	// Save transaction to database
	err = h.transactionRepo.CreateTransaction(c.Request.Context(), transaction)
	if err != nil {
		// Log error but still return payment success
		fmt.Printf("Warning: Failed to save transaction to database: %v\n", err)
		// Continue - payment was successful even if DB save failed
	}

	response := PayResponse{
		Success:       paymentResp.Result == "SUCCESS",
		Message:       "Payment processed successfully",
		TransactionID: paymentResp.Transaction.ID,
		OrderID:       paymentResp.Order.ID,
		Amount:        utils.ConvertToString(paymentResp.Order.Amount),
		Currency:      paymentResp.Order.Currency,
		Status:        paymentResp.Transaction.Status,
	}

	c.JSON(http.StatusOK, response)
}

// RefundRequest represents refund request
type RefundRequest struct {
	OrderID  string `json:"order_id" binding:"required"`
	Amount   string `json:"amount" binding:"required"`
	Currency string `json:"currency" binding:"required"`
}

// Refund processes a refund
func (h *PaymentHandler) Refund(c *gin.Context) {
	var req RefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	refundResp, err := h.mastercardService.RefundPayment(
		req.OrderID,
		req.Amount,
		req.Currency,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "refund failed",
			"details": err.Error(),
		})
		return
	}

	// Save refund transaction
	refundTransaction := &models.Transaction{
		Amount:               utils.MustParseFloat(req.Amount),
		Currency:             req.Currency,
		Status:               refundResp.Transaction.Status,
		GatewayTransactionID: refundResp.Transaction.ID,
		Type:                 "refund",
		// Note: We don't have userID or cardID for refunds without additional logic
	}

	// Try to save refund transaction
	if h.transactionRepo != nil {
		_ = h.transactionRepo.CreateTransaction(c.Request.Context(), refundTransaction)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        refundResp.Result == "SUCCESS",
		"message":        "Refund processed",
		"transaction_id": refundResp.Transaction.ID,
		"amount":         refundResp.Transaction.Amount,
		"currency":       refundResp.Transaction.Currency,
	})
}

// GetTransactionsRequest for getting user's transactions
type GetTransactionsRequest struct {
	UserID string `json:"user_id" binding:"required,uuid4"`
}

// GetTransactions gets all transactions for a user
func (h *PaymentHandler) GetTransactions(c *gin.Context) {
	userID := c.Param("user_id")

	uid, err := uuid.Parse(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	// Validate user exists
	_, err = h.userRepo.GetUserByID(c.Request.Context(), uid)
	if err != nil {
		if _, ok := err.(*repositories.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get user's transactions
	transactions, err := h.transactionRepo.GetTransactionsByUserID(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, transactions)
}

// GetTransactionByID gets a specific transaction
func (h *PaymentHandler) GetTransactionByID(c *gin.Context) {
	transactionID := c.Param("transaction_id")

	tid, err := uuid.Parse(transactionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transaction ID"})
		return
	}

	transaction, err := h.transactionRepo.GetTransactionByID(c.Request.Context(), tid)
	if err != nil {
		if _, ok := err.(*repositories.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "transaction not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, transaction)
}

