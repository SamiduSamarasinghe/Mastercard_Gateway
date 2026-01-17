package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"pg-backend/internal/models"
	"pg-backend/internal/repositories"
	"pg-backend/internal/services"
	"pg-backend/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ApplePayHandler struct {
	mastercardService services.MastercardService
	userRepo          repositories.UserRepository
	cardRepo          repositories.CardRepository
	transactionRepo   repositories.TransactionRepository
}

func NewApplePayHandler(
	mastercardService services.MastercardService,
	userRepo repositories.UserRepository,
	cardRepo repositories.CardRepository,
	transactionRepo repositories.TransactionRepository,
) *ApplePayHandler {
	return &ApplePayHandler{
		mastercardService: mastercardService,
		userRepo:          userRepo,
		cardRepo:          cardRepo,
		transactionRepo:   transactionRepo,
	}
}

// ApplePayRequest represents Apple Pay payment request
type ApplePayRequest struct {
	UserID string `json:"user_id" binding:"required,uuid4"`

	// Option 1: Encrypted payment token (from iOS device)
	PaymentToken string `json:"payment_token,omitempty"`

	// Option 2: Decrypted card details (for testing/fallback)
	CardNumber   string `json:"card_number,omitempty"`
	ExpiryMonth  string `json:"expiry_month,omitempty"`
	ExpiryYear   string `json:"expiry_year,omitempty"`
	Cryptogram   string `json:"cryptogram,omitempty"`
	EciIndicator string `json:"eci_indicator,omitempty"`

	// Common fields
	Amount      string `json:"amount" binding:"required"`
	Currency    string `json:"currency" binding:"required"`
	Description string `json:"description,omitempty"`
	SavePayment bool   `json:"save_payment"`
}

// ApplePayResponse represents Apple Pay payment response
type ApplePayResponse struct {
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	TransactionID  string `json:"transaction_id,omitempty"`
	OrderID        string `json:"order_id,omitempty"`
	Amount         string `json:"amount,omitempty"`
	Currency       string `json:"currency,omitempty"`
	Status         string `json:"status,omitempty"`
	WalletProvider string `json:"wallet_provider,omitempty"`
	CardID         string `json:"card_id,omitempty"`
	IsSimulated    bool   `json:"is_simulated,omitempty"`
	UsedFallback   bool   `json:"used_fallback,omitempty"`
}

// Pay processes an Apple Pay payment
func (h *ApplePayHandler) Pay(c *gin.Context) {
	var req ApplePayRequest
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
	var usedFallback bool
	var isSimulated bool

	// Determine which payment method to use
	if req.PaymentToken != "" {
		// Method 1: Try with encrypted payment token (requires Device Payments privilege)
		paymentResp, err = h.processWithPaymentToken(req)
		if err != nil && strings.Contains(err.Error(), "Missing merchant privilege") {
			// Fallback to simulation if privilege missing
			usedFallback = true
			paymentResp, err = h.simulateApplePay(req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "Apple Pay payment failed",
					"details": err.Error(),
				})
				return
			}
			isSimulated = true
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Apple Pay payment failed",
				"details": err.Error(),
			})
			return
		}
	} else if req.CardNumber != "" && req.Cryptogram != "" {
		// Method 2: Use decrypted card details (for testing)
		paymentResp, err = h.processWithCardDetails(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Apple Pay payment failed",
				"details": err.Error(),
			})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "either payment_token or (card_number + cryptogram) required",
		})
		return
	}

	// Validate payment response
	if paymentResp.Result != "SUCCESS" && paymentResp.GatewayCode != "APPROVED" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "Apple Pay payment declined",
			"code":   paymentResp.GatewayCode,
			"result": paymentResp.Result,
		})
		return
	}

	// Save Apple Pay as payment method if requested
	var savedCardID uuid.UUID
	if req.SavePayment {
		card := h.createApplePayCardModel(userID, req, paymentResp)
		err = h.cardRepo.CreateCard(c.Request.Context(), card)
		if err != nil {
			fmt.Printf("Warning: Failed to save Apple Pay card: %v\n", err)
		} else {
			savedCardID = card.ID
		}
	}

	// Save transaction to database
	transaction := h.createApplePayTransactionModel(userID, req, paymentResp, isSimulated)
	err = h.transactionRepo.CreateTransaction(c.Request.Context(), transaction)
	if err != nil {
		fmt.Printf("Warning: Failed to save Apple Pay transaction: %v\n", err)
	}

	// Prepare response
	response := ApplePayResponse{
		Success:        paymentResp.Result == "SUCCESS",
		Message:        "Apple Pay payment processed successfully",
		TransactionID:  paymentResp.Transaction.ID,
		OrderID:        paymentResp.Order.ID,
		Amount:         utils.ConvertToString(paymentResp.Order.Amount),
		Currency:       paymentResp.Order.Currency,
		Status:         paymentResp.Transaction.Status,
		WalletProvider: "APPLE_PAY",
		CardID:         "",
		IsSimulated:    isSimulated,
		UsedFallback:   usedFallback,
	}

	if savedCardID != uuid.Nil {
		response.CardID = savedCardID.String()
	}

	if isSimulated {
		response.Message = "Apple Pay payment simulated (Device Payments privilege not enabled)"
	}

	c.JSON(http.StatusOK, response)
}

// processWithPaymentToken handles encrypted payment token
func (h *ApplePayHandler) processWithPaymentToken(req ApplePayRequest) (*services.PaymentResponse, error) {
	// Note: You'll need to add this method to your MastercardService interface
	// For now, we'll simulate it
	return nil, fmt.Errorf("Missing merchant privilege 'Device Payments'")
}

// processWithCardDetails handles decrypted card details
func (h *ApplePayHandler) processWithCardDetails(req ApplePayRequest) (*services.PaymentResponse, error) {
	// Use Google Pay method as fallback (similar structure)
	return h.mastercardService.PayWithGooglePay(
		req.CardNumber,
		req.ExpiryMonth,
		req.ExpiryYear,
		req.Cryptogram,
		req.EciIndicator,
		req.Amount,
		req.Currency,
	)
}

// simulateApplePay simulates Apple Pay for testing
func (h *ApplePayHandler) simulateApplePay(req ApplePayRequest) (*services.PaymentResponse, error) {
	// Use test data for simulation
	// You can add Apple Pay test constants to services/mastercard_service.go
	return h.mastercardService.PayWithCard(
		"4111111111111111", // Test Visa
		"12",
		"2028",
		"123",
		req.Amount,
		req.Currency,
	)
}

// Helper methods to create models
func (h *ApplePayHandler) createApplePayCardModel(userID uuid.UUID, req ApplePayRequest, paymentResp *services.PaymentResponse) *models.Card {
	card := &models.Card{
		UserID:            userID,
		GatewayToken:      req.PaymentToken, // Or card number for decrypted
		LastFour:          extractLastFour(req),
		ExpiryMonth:       extractExpiryMonth(req),
		ExpiryYear:        extractExpiryYear(req),
		Scheme:            extractCardScheme(req),
		IsDefault:         false,
		PaymentMethodType: models.PaymentMethodTypeApplePay,
		WalletProvider:    models.WalletProviderApplePay,
		DevicePaymentData: map[string]interface{}{
			"cryptogram":    req.Cryptogram,
			"eci_indicator": req.EciIndicator,
			"is_simulated":  req.PaymentToken != "" && req.CardNumber == "",
		},
	}

	if req.PaymentToken != "" {
		card.DevicePaymentData["payment_token"] = req.PaymentToken
	}

	return card
}

func (h *ApplePayHandler) createApplePayTransactionModel(userID uuid.UUID, req ApplePayRequest, paymentResp *services.PaymentResponse, isSimulated bool) *models.Transaction {
	deviceData := map[string]interface{}{
		"is_simulated": isSimulated,
	}

	if req.Cryptogram != "" {
		deviceData["cryptogram"] = req.Cryptogram
		deviceData["eci_indicator"] = req.EciIndicator
	}

	if req.PaymentToken != "" {
		deviceData["has_payment_token"] = true
	}

	return &models.Transaction{
		UserID:               userID,
		Amount:               utils.MustParseFloat(req.Amount),
		Currency:             req.Currency,
		Status:               paymentResp.Transaction.Status,
		GatewayTransactionID: paymentResp.Transaction.ID,
		Type:                 "manual",
		WalletProvider:       models.WalletProviderApplePay,
		PaymentMethodType:    models.PaymentMethodTypeApplePay,
		DevicePaymentData:    deviceData,
	}
}

// Helper extraction functions
func extractLastFour(req ApplePayRequest) string {
	if req.CardNumber != "" && len(req.CardNumber) >= 4 {
		return req.CardNumber[len(req.CardNumber)-4:]
	}
	return "0000"
}

func extractExpiryMonth(req ApplePayRequest) int {
	if req.ExpiryMonth != "" {
		return utils.MustParseInt(req.ExpiryMonth)
	}
	return 12
}

func extractExpiryYear(req ApplePayRequest) int {
	if req.ExpiryYear != "" {
		return utils.MustParseInt(req.ExpiryYear)
	}
	return 2028
}

func extractCardScheme(req ApplePayRequest) string {
	if req.CardNumber == "" {
		return "VISA" // Default for simulation
	}
	return getCardScheme(req.CardNumber)
}

// TestApplePay for Postman testing
func (h *ApplePayHandler) TestApplePay(c *gin.Context) {
	var req struct {
		UserID   string `json:"user_id" binding:"required,uuid4"`
		Amount   string `json:"amount" binding:"required"`
		Currency string `json:"currency" binding:"required"`
		TestType string `json:"test_type" enums:"success,decline,error"` // Test scenarios
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
	var cardNumber, expiryMonth, expiryYear, amount string

	// Special amounts for testing (from Apple Pay documentation)
	switch req.TestType {
	case "decline":
		amount = "57.57" // Test decline amount
		cardNumber = services.TestFPANVisa
	case "error":
		amount = "58.58" // Test error amount
		cardNumber = services.TestFPANVisa
	default: // success
		amount = req.Amount
		cardNumber = services.TestFPANVisa
	}

	expiryMonth = services.TestFPANExpiryMonth
	expiryYear = services.TestFPANExpiryYear[2:] // Last 2 digits

	// For Apple Pay simulation, we need cryptogram and ECI
	cryptogram := services.TestCryptogram
	eci := services.TestEciIndicator

	paymentResp, err := h.mastercardService.PayWithGooglePay(
		cardNumber,
		expiryMonth,
		expiryYear,
		cryptogram,
		eci,
		amount,
		req.Currency,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Apple Pay test payment failed",
			"details": err.Error(),
		})
		return
	}

	// Save test transaction
	transaction := &models.Transaction{
		UserID:               userID,
		Amount:               utils.MustParseFloat(amount),
		Currency:             req.Currency,
		Status:               paymentResp.Transaction.Status,
		GatewayTransactionID: paymentResp.Transaction.ID,
		Type:                 "test",
		WalletProvider:       models.WalletProviderApplePay,
		PaymentMethodType:    models.PaymentMethodTypeApplePay,
		DevicePaymentData: map[string]interface{}{
			"cryptogram":    cryptogram,
			"eci_indicator": eci,
			"test_type":     req.TestType,
			"is_simulated":  true,
		},
	}

	err = h.transactionRepo.CreateTransaction(c.Request.Context(), transaction)
	if err != nil {
		fmt.Printf("Warning: Failed to save test Apple Pay transaction: %v\n", err)
	}

	response := ApplePayResponse{
		Success:        paymentResp.Result == "SUCCESS",
		Message:        fmt.Sprintf("Apple Pay test payment processed (%s)", req.TestType),
		TransactionID:  paymentResp.Transaction.ID,
		OrderID:        paymentResp.Order.ID,
		Amount:         utils.ConvertToString(paymentResp.Order.Amount),
		Currency:       paymentResp.Order.Currency,
		Status:         paymentResp.Transaction.Status,
		WalletProvider: models.WalletProviderApplePay,
		IsSimulated:    true,
	}

	c.JSON(http.StatusOK, response)
}

// GetUserApplePayCards gets all Apple Pay cards for a user
func (h *ApplePayHandler) GetUserApplePayCards(c *gin.Context) {
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

	// Filter for Apple Pay cards only
	var applePayCards []models.Card
	for _, card := range allCards {
		if card.PaymentMethodType == models.PaymentMethodTypeApplePay {
			applePayCards = append(applePayCards, card)
		}
	}

	c.JSON(http.StatusOK, applePayCards)
}

// DeleteApplePayCard deletes a user's Apple Pay card
func (h *ApplePayHandler) DeleteApplePayCard(c *gin.Context) {
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

	// Get card to verify it's an Apple Pay card
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

	// Verify it's an Apple Pay card
	if card.PaymentMethodType != models.PaymentMethodTypeApplePay {
		c.JSON(http.StatusBadRequest, gin.H{"error": "card is not an Apple Pay payment method"})
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
		"message": "Apple Pay card deleted successfully",
	})
}
