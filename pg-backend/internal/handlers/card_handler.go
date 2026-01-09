package handlers

import (
	"net/http"

	"pg-backend/internal/models"
	"pg-backend/internal/repositories"
	"pg-backend/internal/services"
	"pg-backend/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CardHandler struct {
	mastercardService services.MastercardService
	userRepo          repositories.UserRepository
	cardRepo          repositories.CardRepository
}

func NewCardHandler(
	mastercardService services.MastercardService,
	userRepo repositories.UserRepository,
	cardRepo repositories.CardRepository,
) *CardHandler {
	return &CardHandler{
		mastercardService: mastercardService,
		userRepo:          userRepo,
		cardRepo:          cardRepo,
	}
}

// VerifyAndSaveCardRequest for card verification and saving
type VerifyAndSaveCardRequest struct {
	UserID      string `json:"user_id" binding:"required,uuid4"`
	CardNumber  string `json:"card_number" binding:"required,credit_card"`
	ExpiryMonth string `json:"expiry_month" binding:"required"`
	ExpiryYear  string `json:"expiry_year" binding:"required"`
	CVV         string `json:"cvv" binding:"required"`
	Currency    string `json:"currency" binding:"required"`
	MakeDefault bool   `json:"make_default"`
}

// VerifyAndSaveCardResponse for card verification response
type VerifyAndSaveCardResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	CardID       string `json:"card_id,omitempty"`
	GatewayToken string `json:"gateway_token,omitempty"`
	LastFour     string `json:"last_four,omitempty"`
}

// VerifyAndSaveCard verifies and saves a card
func (h *CardHandler) VerifyAndSaveCard(c *gin.Context) {
	var req VerifyAndSaveCardRequest
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

	// Step 1: Verify card with Mastercard
	verifyResp, err := h.mastercardService.VerifyCard(
		req.CardNumber,
		req.ExpiryMonth,
		req.ExpiryYear,
		req.CVV,
		req.Currency,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "card verification failed",
			"details": err.Error(),
		})
		return
	}

	// Check if verification was successful
	if verifyResp.GatewayCode != "APPROVED" && verifyResp.Response.GatewayCode != "APPROVED" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "card verification declined",
			"code":  verifyResp.GatewayCode,
		})
		return
	}

	// Step 2: Create payment token
	tokenResp, err := h.mastercardService.CreatePaymentToken(
		req.CardNumber,
		req.ExpiryMonth,
		req.ExpiryYear,
		req.CVV,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to create payment token",
			"details": err.Error(),
		})
		return
	}

	expiryMonth := tokenResp.SourceOfFunds.Provided.Card.Expiry[:2]       // First 2 chars for month
	expiryYear := "20" + tokenResp.SourceOfFunds.Provided.Card.Expiry[2:] // Last 2 chars for year

	// Step 3: Save card to database
	card := &models.Card{
		UserID:       userID,
		GatewayToken: tokenResp.Token,
		LastFour:     tokenResp.SourceOfFunds.Provided.Card.Last4,
		ExpiryMonth:  utils.MustParseInt(expiryMonth),
		ExpiryYear:   utils.MustParseInt(expiryYear),
		Scheme:       tokenResp.SourceOfFunds.Provided.Card.Scheme,
		IsDefault:    req.MakeDefault,
	}

	err = h.cardRepo.CreateCard(c.Request.Context(), card)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to save card",
			"details": err.Error(),
		})
		return
	}

	response := VerifyAndSaveCardResponse{
		Success:      true,
		Message:      "Card verified and saved successfully",
		CardID:       card.ID.String(),
		GatewayToken: card.GatewayToken,
		LastFour:     card.LastFour,
	}

	c.JSON(http.StatusCreated, response)
}

// GetUserCardsRequest for getting user's cards
type GetUserCardsRequest struct {
	UserID string `json:"user_id" binding:"required,uuid4"`
}

// GetUserCards gets all cards for a user
func (h *CardHandler) GetUserCards(c *gin.Context) {
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

	// Get user's cards
	cards, err := h.cardRepo.GetCardsByUserID(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, cards)
}

// DeleteCardRequest for deleting a card
type DeleteCardRequest struct {
	UserID string `json:"user_id" binding:"required,uuid4"`
	CardID string `json:"card_id" binding:"required,uuid4"`
}

// DeleteCard deletes a user's card
func (h *CardHandler) DeleteCard(c *gin.Context) {
	var req DeleteCardRequest
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
		"message": "Card deleted successfully",
	})
}

// // Helper function to parse string to int
// func mustParseInt(s string) int {
// 	i, _ := strconv.Atoi(s)
// 	return i
// }
