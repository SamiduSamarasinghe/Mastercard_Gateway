package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"mobile-payment-backend/internal/models"
	"mobile-payment-backend/internal/repositories"
)

type OrderHandler struct {
	orderRepo repositories.OrderRepository
	userRepo  repositories.UserRepository
}

func NewOrderHandler(orderRepo repositories.OrderRepository, userRepo repositories.UserRepository) *OrderHandler {
	return &OrderHandler{
		orderRepo: orderRepo,
		userRepo:  userRepo,
	}
}

// CreateOrderRequest from frontend
type CreateOrderRequest struct {
	UserID      string  `json:"user_id" binding:"required,uuid4"`
	Amount      float64 `json:"amount" binding:"required,min=0.01"`
	Currency    string  `json:"currency" binding:"required,len=3"`
	Description string  `json:"description,omitempty"`
}

// CreateOrder creates a new order
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	var req CreateOrderRequest
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

	_, err = h.userRepo.GetByID(c.Request.Context(), userID)
	if err != nil {
		if _, ok := err.(*repositories.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Generate unique reference ID
	referenceID := fmt.Sprintf("ORD-%d-%s",
		time.Now().Unix(),
		uuid.New().String()[:8],
	)

	order := &models.Order{
		UserID:      userID,
		ReferenceID: referenceID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Description: req.Description,
		Status:      "pending",
		Metadata: map[string]interface{}{
			"created_via": "api",
			"ip_address":  c.ClientIP(),
		},
	}

	if err := h.orderRepo.Create(c.Request.Context(), order); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to create order",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"order":   order,
		"message": "order created successfully",
	})
}

// GetOrder gets order by ID
func (h *OrderHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("id")

	oid, err := uuid.Parse(orderID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order ID"})
		return
	}

	order, err := h.orderRepo.GetByID(c.Request.Context(), oid)
	if err != nil {
		if _, ok := err.(*repositories.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"order":   order,
	})
}

// GetOrdersByUser gets all orders for a user
func (h *OrderHandler) GetOrdersByUser(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		userID = c.Param("id")
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	orders, err := h.orderRepo.GetByUserID(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"orders":  orders,
		"count":   len(orders),
	})
}
