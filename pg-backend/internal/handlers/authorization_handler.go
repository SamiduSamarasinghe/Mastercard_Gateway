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

	type AuthorizationHandler struct {
		mastercardService services.MastercardService
		userRepo          repositories.UserRepository
		cardRepo          repositories.CardRepository
		transactionRepo   repositories.TransactionRepository
	}

	func NewAuthorizationHandler(
		mastercardService services.MastercardService,
		userRepo repositories.UserRepository,
		cardRepo repositories.CardRepository,
		transactionRepo repositories.TransactionRepository,
	) *AuthorizationHandler {
		return &AuthorizationHandler{
			mastercardService: mastercardService,
			userRepo:          userRepo,
			cardRepo:          cardRepo,
			transactionRepo:   transactionRepo,
		}
	}

	// AuthorizeRequest for authorization (hold funds)
	type AuthorizeRequest struct {
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

	// AuthorizeResponse for authorization response
	type AuthorizeResponse struct {
		Success       bool   `json:"success"`
		Message       string `json:"message"`
		TransactionID string `json:"transaction_id,omitempty"`
		OrderID       string `json:"order_id,omitempty"`
		Amount        string `json:"amount,omitempty"`
		Currency      string `json:"currency,omitempty"`
		Status        string `json:"status,omitempty"`
		Type          string `json:"type,omitempty"` // "authorization"
	}

	// Authorize holds funds without charging
	func (h *AuthorizationHandler) Authorize(c *gin.Context) {
		var req AuthorizeRequest
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

		var authResp *services.PaymentResponse
		var cardID uuid.UUID
		var card *models.Card

		// Check if using saved card or new card
		if req.CardID != "" {
			// Authorize with saved card (using token)
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

			// Authorize with token
			authResp, err = h.mastercardService.AuthorizeWithToken(
				card.GatewayToken,
				req.Amount,
				req.Currency,
			)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "authorization failed",
					"details": err.Error(),
				})
				return
			}

		} else {
			// Authorize with new card details
			if req.CardNumber == "" || req.ExpiryMonth == "" || req.ExpiryYear == "" || req.CVV == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "card details required when not using saved card"})
				return
			}

			authResp, err = h.mastercardService.AuthorizeWithCard(
				req.CardNumber,
				req.ExpiryMonth,
				req.ExpiryYear,
				req.CVV,
				req.Amount,
				req.Currency,
			)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "authorization failed",
					"details": err.Error(),
				})
				return
			}
		}

		// Validate authorization response
		if authResp.Result != "SUCCESS" && authResp.GatewayCode != "APPROVED" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  "authorization declined",
				"code":   authResp.GatewayCode,
				"result": authResp.Result,
			})
			return
		}

		// Save authorization transaction to database
		transaction := &models.Transaction{
			UserID:               userID,
			Amount:               utils.MustParseFloat(req.Amount),
			Currency:             req.Currency,
			Status:               authResp.Transaction.Status,
			GatewayTransactionID: authResp.Transaction.ID,
			Type:                 "authorization",
			// Store order ID for future capture/void
		}

		// If using saved card, set card ID
		if req.CardID != "" {
			transaction.CardID = cardID
		}

		// Save authorization to database
		err = h.transactionRepo.CreateTransaction(c.Request.Context(), transaction)
		if err != nil {
			// Log error but still return success
			println("Warning: Failed to save authorization to database:", err.Error())
		}

		response := AuthorizeResponse{
			Success:       authResp.Result == "SUCCESS",
			Message:       "Funds authorized successfully",
			TransactionID: authResp.Transaction.ID,
			OrderID:       authResp.Order.ID,
			Amount:        utils.ConvertToString(authResp.Order.Amount),
			Currency:      authResp.Order.Currency,
			Status:        authResp.Transaction.Status,
			Type:          "authorization",
		}

		c.JSON(http.StatusOK, response)
	}

	// CaptureRequest for capturing authorized funds
	type CaptureRequest struct {
		OrderID  string `json:"order_id" binding:"required"`
		Amount   string `json:"amount" binding:"required"`
		Currency string `json:"currency" binding:"required"`
	}

	// Capture captures previously authorized funds
	func (h *AuthorizationHandler) Capture(c *gin.Context) {
		var req CaptureRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		captureResp, err := h.mastercardService.CaptureAuthorization(
			req.OrderID,
			req.Amount,
			req.Currency,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "capture failed",
				"details": err.Error(),
			})
			return
		}

		// Save capture transaction to database
		captureTransaction := &models.Transaction{
			Amount:               utils.MustParseFloat(req.Amount),
			Currency:             req.Currency,
			Status:               captureResp.Transaction.Status,
			GatewayTransactionID: captureResp.Transaction.ID,
			Type:                 "capture",
			// Note: We would need to lookup the original authorization to set userID/cardID
		}

		// Save capture to database
		if h.transactionRepo != nil {
			_ = h.transactionRepo.CreateTransaction(c.Request.Context(), captureTransaction)
		}

		c.JSON(http.StatusOK, gin.H{
			"success":        captureResp.Result == "SUCCESS",
			"message":        "Funds captured successfully",
			"transaction_id": captureResp.Transaction.ID,
			"amount":         captureResp.Transaction.Amount,
			"currency":       captureResp.Transaction.Currency,
			"status":         captureResp.Transaction.Status,
		})
	}

	// VoidRequest for voiding an authorization
	type VoidRequest struct {
		OrderID string `json:"order_id" binding:"required"`
	}

	// Void cancels an authorization
	func (h *AuthorizationHandler) Void(c *gin.Context) {
		var req VoidRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		voidResp, err := h.mastercardService.VoidAuthorization(req.OrderID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "void failed",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success":        voidResp.Result == "SUCCESS",
			"message":        "Authorization voided successfully",
			"transaction_id": voidResp.Transaction.ID,
			"status":         voidResp.Transaction.Status,
		})
	}

	// UpdateAuthorizationRequest for updating authorization amount
	type UpdateAuthorizationRequest struct {
		OrderID  string `json:"order_id" binding:"required"`
		Amount   string `json:"amount" binding:"required"`
		Currency string `json:"currency" binding:"required"`
	}

	// UpdateAuthorization updates authorization amount
	func (h *AuthorizationHandler) UpdateAuthorization(c *gin.Context) {
		var req UpdateAuthorizationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		updateResp, err := h.mastercardService.UpdateAuthorization(
			req.OrderID,
			req.Amount,
			req.Currency,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "update authorization failed",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success":        updateResp.Result == "SUCCESS",
			"message":        "Authorization updated successfully",
			"transaction_id": updateResp.Transaction.ID,
			"amount":         updateResp.Transaction.Amount,
			"currency":       updateResp.Transaction.Currency,
			"status":         updateResp.Transaction.Status,
		})
	}


