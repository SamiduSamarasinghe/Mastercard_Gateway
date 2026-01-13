package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"pg-backend/internal/models"
	"pg-backend/internal/repositories"
	"pg-backend/internal/services"
	"pg-backend/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type GooglePayHandler struct {
	mastercardService services.MastercardService
	userRepo          repositories.UserRepository
	cardRepo          repositories.CardRepository
	transactionRepo   repositories.TransactionRepository
}

func NewGooglePayHandler(
	mastercardService services.MastercardService,
	userRepo repositories.UserRepository,
	cardRepo repositories.CardRepository,
	transactionRepo repositories.TransactionRepository,
) *GooglePayHandler {
	return &GooglePayHandler{
		mastercardService: mastercardService,
		userRepo:          userRepo,
		cardRepo:          cardRepo,
		transactionRepo:   transactionRepo,
	}
}

// GooglePayRequest represents Google Pay payment request (merchant-decrypted flow)
type GooglePayRequest struct {
	UserID       string `json:"user_id" binding:"required,uuid4"`
	CardID       string `json:"card_id,omitempty"`     // Optional: Use saved Google Pay card
	CardNumber   string `json:"card_number,omitempty"` // Required if not using saved card
	ExpiryMonth  string `json:"expiry_month,omitempty"`
	ExpiryYear   string `json:"expiry_year,omitempty"`
	Cryptogram   string `json:"cryptogram" binding:"required"`
	EciIndicator string `json:"eci_indicator" binding:"required"`
	Amount       string `json:"amount" binding:"required"`
	Currency     string `json:"currency" binding:"required"`
	Description  string `json:"description,omitempty"`
	SavePayment  bool   `json:"save_payment"` // Save Google Pay for future use
}

// GooglePayResponse represents Google Pay payment response
type GooglePayResponse struct {
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	TransactionID  string `json:"transaction_id,omitempty"`
	OrderID        string `json:"order_id,omitempty"`
	Amount         string `json:"amount,omitempty"`
	Currency       string `json:"currency,omitempty"`
	Status         string `json:"status,omitempty"`
	WalletProvider string `json:"wallet_provider,omitempty"`
	CardID         string `json:"card_id,omitempty"`
	IsSimulated    bool   `json:"is_simulated,omitempty"` // NEW: Indicates if simulated
}

// Pay processes a Google Pay payment
func (h *GooglePayHandler) Pay(c *gin.Context) {
	var req GooglePayRequest
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

	// Check if using saved Google Pay card or new Google Pay details
	if req.CardID != "" {
		// Pay with saved Google Pay card
		cardID, err = uuid.Parse(req.CardID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid card ID"})
			return
		}

		// Get saved Google Pay card from database
		card, err = h.cardRepo.GetCardByID(c.Request.Context(), cardID)
		if err != nil {
			if _, ok := err.(*repositories.NotFoundError); ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "Google Pay card not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Verify card belongs to user
		if card.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "Google Pay card does not belong to user"})
			return
		}

		// Verify it's a Google Pay payment method
		if card.PaymentMethodType != "google_pay" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "card is not a Google Pay payment method"})
			return
		}

		// Extract device payment data from saved card
		var cryptogram, eci string
		if card.DevicePaymentData != nil {
			if c, ok := card.DevicePaymentData["cryptogram"].(string); ok {
				cryptogram = c
			}
			if e, ok := card.DevicePaymentData["eci_indicator"].(string); ok {
				eci = e
			}
		}

		// Use card details for payment
		paymentResp, err = h.mastercardService.PayWithGooglePay(
			card.GatewayToken, // Use token for saved cards
			fmt.Sprintf("%02d", card.ExpiryMonth),
			strconv.Itoa(card.ExpiryYear),
			cryptogram,
			eci,
			req.Amount,
			req.Currency,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Google Pay payment failed",
				"details": err.Error(),
			})
			return
		}

	} else {
		// Pay with new Google Pay details (merchant-decrypted flow)
		if req.CardNumber == "" || req.ExpiryMonth == "" || req.ExpiryYear == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "card details required when not using saved Google Pay"})
			return
		}

		// Process payment with provided Google Pay details
		paymentResp, err = h.mastercardService.PayWithGooglePay(
			req.CardNumber,
			req.ExpiryMonth,
			req.ExpiryYear,
			req.Cryptogram,
			req.EciIndicator,
			req.Amount,
			req.Currency,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Google Pay payment failed",
				"details": err.Error(),
			})
			return
		}
	}

	// Validate payment response
	if paymentResp.Result != "SUCCESS" && paymentResp.GatewayCode != "APPROVED" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "Google Pay payment declined",
			"code":   paymentResp.GatewayCode,
			"result": paymentResp.Result,
		})
		return
	}

	// Save Google Pay as payment method if requested
	var savedCardID uuid.UUID
	if req.SavePayment && req.CardID == "" && req.CardNumber != "" {
		// Create Google Pay card record
		card := &models.Card{
			UserID:            userID,
			GatewayToken:      req.CardNumber, // For now, store card number. In production, use token.
			LastFour:          req.CardNumber[len(req.CardNumber)-4:],
			ExpiryMonth:       utils.MustParseInt(req.ExpiryMonth),
			ExpiryYear:        utils.MustParseInt(req.ExpiryYear),
			Scheme:            getCardScheme(req.CardNumber),
			IsDefault:         false, // Don't set as default automatically
			PaymentMethodType: "google_pay",
			WalletProvider:    "GOOGLE_PAY",
			DevicePaymentData: map[string]interface{}{
				"cryptogram":    req.Cryptogram,
				"eci_indicator": req.EciIndicator,
			},
		}

		// Save to database
		err = h.cardRepo.CreateCard(c.Request.Context(), card)
		if err != nil {
			// Log error but don't fail payment
			fmt.Printf("Warning: Failed to save Google Pay card: %v\n", err)
		} else {
			savedCardID = card.ID
		}
	}

	// Save transaction to database
	transaction := &models.Transaction{
		UserID:               userID,
		Amount:               utils.MustParseFloat(req.Amount),
		Currency:             req.Currency,
		Status:               paymentResp.Transaction.Status,
		GatewayTransactionID: paymentResp.Transaction.ID,
		Type:                 "manual",
		WalletProvider:       "GOOGLE_PAY",
		PaymentMethodType:    "google_pay",
		DevicePaymentData: map[string]interface{}{
			"cryptogram":    req.Cryptogram,
			"eci_indicator": req.EciIndicator,
			"is_simulated":  false, // Default to false
		},
	}

	// Check if this was a simulated payment
	if strings.Contains(paymentResp.Result, "simulated") || paymentResp.Order.WalletProvider == "" {
		// It was simulated
		transaction.DevicePaymentData["is_simulated"] = true
	}

	// Save transaction to database
	err = h.transactionRepo.CreateTransaction(c.Request.Context(), transaction)
	if err != nil {
		fmt.Printf("Warning: Failed to save Google Pay transaction: %v\n", err)
	}

	response := GooglePayResponse{
		Success:        paymentResp.Result == "SUCCESS",
		Message:        "Google Pay payment processed successfully",
		TransactionID:  paymentResp.Transaction.ID,
		OrderID:        paymentResp.Order.ID,
		Amount:         utils.ConvertToString(paymentResp.Order.Amount),
		Currency:       paymentResp.Order.Currency,
		Status:         paymentResp.Transaction.Status,
		WalletProvider: "GOOGLE_PAY",
	}

	// Check if simulated
	if strings.Contains(paymentResp.Result, "simulated") || paymentResp.Order.WalletProvider == "" {
		response.IsSimulated = true
		response.Message = "Google Pay payment simulated (Device Payments privilege not enabled)"
	}

	// Include saved card ID in response if saved
	if savedCardID != uuid.Nil {
		response.CardID = savedCardID.String()
	}

	c.JSON(http.StatusOK, response)
}

// TestGooglePay processes a test Google Pay payment (for Postman testing)
func (h *GooglePayHandler) TestGooglePay(c *gin.Context) {
	var req struct {
		UserID   string `json:"user_id" binding:"required,uuid4"`
		Amount   string `json:"amount" binding:"required"`
		Currency string `json:"currency" binding:"required"`
		TestType string `json:"test_type" enums:"dpan, fpan"` // "dpan" or "fpan"
	}

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

	// Use test data based on test type
	var cardNumber, expiryMonth, expiryYear string

	if req.TestType == "dpan" {
		// Use DPAN test data (mobile wallet payment)
		cardNumber = services.TestDPANVisa
		expiryMonth = services.TestDPANExpiryMonth
		expiryYear = services.TestDPANExpiryYear[2:]
	} else {
		// Default to FPAN test data (card saved to Google account)
		cardNumber = services.TestFPANVisa
		expiryMonth = services.TestFPANExpiryMonth
		expiryYear = services.TestFPANExpiryYear[2:]
	}

	// Use test cryptogram and ECI
	cryptogram := services.TestCryptogram
	eci := services.TestEciIndicator

	// Process test payment
	paymentResp, err := h.mastercardService.PayWithGooglePay(
		cardNumber,
		expiryMonth,
		expiryYear,
		cryptogram,
		eci,
		req.Amount,
		req.Currency,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Google Pay test payment failed",
			"details": err.Error(),
		})
		return
	}

	// Validate payment response
	if paymentResp.Result != "SUCCESS" && paymentResp.GatewayCode != "APPROVED" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "Google Pay test payment declined",
			"code":   paymentResp.GatewayCode,
			"result": paymentResp.Result,
		})
		return
	}

	// Save test transaction to database
	transaction := &models.Transaction{
		UserID:               userID,
		Amount:               utils.MustParseFloat(req.Amount),
		Currency:             req.Currency,
		Status:               paymentResp.Transaction.Status,
		GatewayTransactionID: paymentResp.Transaction.ID,
		Type:                 "test",
		WalletProvider:       "GOOGLE_PAY",
		PaymentMethodType:    "google_pay",
		DevicePaymentData: map[string]interface{}{
			"cryptogram":    cryptogram,
			"eci_indicator": eci,
			"test_type":     req.TestType,
		},
	}

	err = h.transactionRepo.CreateTransaction(c.Request.Context(), transaction)
	if err != nil {
		fmt.Printf("Warning: Failed to save test Google Pay transaction: %v\n", err)
	}

	response := GooglePayResponse{
		Success:        paymentResp.Result == "SUCCESS",
		Message:        "Google Pay test payment processed successfully",
		TransactionID:  paymentResp.Transaction.ID,
		OrderID:        paymentResp.Order.ID,
		Amount:         utils.ConvertToString(paymentResp.Order.Amount),
		Currency:       paymentResp.Order.Currency,
		Status:         paymentResp.Transaction.Status,
		WalletProvider: "GOOGLE_PAY",
	}

	c.JSON(http.StatusOK, response)
}

// GetUserGooglePayCards gets all Google Pay cards for a user
func (h *GooglePayHandler) GetUserGooglePayCards(c *gin.Context) {
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

	// Get all cards for user
	allCards, err := h.cardRepo.GetCardsByUserID(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Filter for Google Pay cards only
	var googlePayCards []models.Card
	for _, card := range allCards {
		if card.PaymentMethodType == "google_pay" {
			googlePayCards = append(googlePayCards, card)
		}
	}

	c.JSON(http.StatusOK, googlePayCards)
}

// DeleteGooglePayCard deletes a user's Google Pay card
func (h *GooglePayHandler) DeleteGooglePayCard(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id" binding:"required,uuid4"`
		CardID string `json:"card_id" binding:"required,uuid4"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

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

	// Validate user exists
	_, err = h.userRepo.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if _, ok := err.(*repositories.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get card to verify it's a Google Pay card
	card, err := h.cardRepo.GetCardByID(c.Request.Context(), cardID)
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

	// Verify it's a Google Pay card
	if card.PaymentMethodType != "google_pay" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "card is not a Google Pay payment method"})
		return
	}

	// Delete the card
	err = h.cardRepo.DeleteCard(c.Request.Context(), cardID)
	if err != nil {
		if _, ok := err.(*repositories.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Google Pay card deleted successfully",
	})
}

// Helper function to determine card scheme
func getCardScheme(cardNumber string) string {
	if len(cardNumber) < 1 {
		return "UNKNOWN"
	}

	firstDigit := cardNumber[0]
	switch firstDigit {
	case '4':
		return "VISA"
	case '5':
		return "MASTERCARD"
	case '3':
		return "AMEX"
	case '6':
		return "DISCOVER"
	default:
		return "UNKNOWN"
	}
}

// SimulateGooglePay simulates Google Pay without Device Payments privilege
func (h *GooglePayHandler) SimulateGooglePay(c *gin.Context) {
	var req struct {
		UserID      string `json:"user_id" binding:"required,uuid4"`
		CardNumber  string `json:"card_number" binding:"required"`
		ExpiryMonth string `json:"expiry_month" binding:"required"`
		ExpiryYear  string `json:"expiry_year" binding:"required"`
		Amount      string `json:"amount" binding:"required"`
		Currency    string `json:"currency" binding:"required"`
	}

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

	// Use regular card payment to simulate Google Pay
	paymentResp, err := h.mastercardService.PayWithCard(
		req.CardNumber,
		req.ExpiryMonth,
		req.ExpiryYear,
		"123", // Dummy CVV
		req.Amount,
		req.Currency,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Google Pay simulation failed",
			"details": err.Error(),
		})
		return
	}

	// Validate payment response
	if paymentResp.Result != "SUCCESS" && paymentResp.GatewayCode != "APPROVED" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "Google Pay simulation declined",
			"code":   paymentResp.GatewayCode,
			"result": paymentResp.Result,
		})
		return
	}

	// Save simulated transaction
	transaction := &models.Transaction{
		UserID:               userID,
		Amount:               utils.MustParseFloat(req.Amount),
		Currency:             req.Currency,
		Status:               paymentResp.Transaction.Status,
		GatewayTransactionID: paymentResp.Transaction.ID,
		Type:                 "manual",
		WalletProvider:       "GOOGLE_PAY",
		PaymentMethodType:    "google_pay",
		DevicePaymentData: map[string]interface{}{
			"is_simulated":    true,
			"simulation_note": "Device Payments privilege not enabled",
		},
	}

	err = h.transactionRepo.CreateTransaction(c.Request.Context(), transaction)
	if err != nil {
		fmt.Printf("Warning: Failed to save simulated Google Pay transaction: %v\n", err)
	}

	response := GooglePayResponse{
		Success:        paymentResp.Result == "SUCCESS",
		Message:        "Google Pay payment simulated successfully (Device Payments privilege not enabled)",
		TransactionID:  paymentResp.Transaction.ID,
		OrderID:        paymentResp.Order.ID,
		Amount:         utils.ConvertToString(paymentResp.Order.Amount),
		Currency:       paymentResp.Order.Currency,
		Status:         paymentResp.Transaction.Status,
		WalletProvider: "GOOGLE_PAY",
		IsSimulated:    true,
	}

	c.JSON(http.StatusOK, response)
}
