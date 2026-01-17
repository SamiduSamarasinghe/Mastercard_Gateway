package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"mobile-payment-backend/internal/config"
	"mobile-payment-backend/internal/database"
	"mobile-payment-backend/internal/handlers"
	"mobile-payment-backend/internal/models"
	"mobile-payment-backend/internal/repositories"
	"mobile-payment-backend/internal/services"
)

func main() {
	// Load environment
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Load config
	cfg := config.LoadConfig()

	// Connect to database
	if err := database.ConnectDB(cfg); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer database.DB.Close()

	// Initialize repositories
	sessionRepo := repositories.NewSessionRepository(database.DB)
	transactionRepo := repositories.NewTransactionRepository(database.DB)
	tokenRepo := repositories.NewTokenRepository(database.DB)
	userRepo := repositories.NewUserRepository(database.DB)
	orderRepo := repositories.NewOrderRepository(database.DB)

	// Validate required config
	if cfg.MastercardMerchantID == "" || cfg.MastercardAPIPassword == "" {
		log.Fatal("Missing required Mastercard configuration")
	}

	// Create SDK config for mobile app
	sdkConfig := &models.MobileSDKConfig{
		MerchantID:   cfg.MastercardMerchantID,
		MerchantName: "Your Merchant Name", // From your config
		MerchantURL:  "https://" + cfg.MastercardHost,
		Region:       cfg.MastercardRegion,
		APIVersion:   cfg.APIVersion,
	}

	// Initialize services
	gatewayService := services.NewGatewayService(cfg, sessionRepo, transactionRepo, tokenRepo)

	// Initialize handlers
	userHandler := handlers.NewUserHandler(userRepo)
	orderHandler := handlers.NewOrderHandler(orderRepo, userRepo) // NEW
	sessionHandler := handlers.NewSessionHandler(gatewayService, orderRepo, sessionRepo, sdkConfig)
	paymentHandler := handlers.NewPaymentHandler(gatewayService)

	// Setup Gin
	router := gin.Default()

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// API routes
	api := router.Group("/api/v1")
	{
		// User management (NEW)
		api.POST("/users", userHandler.CreateUser)
		api.GET("/users/:id", userHandler.GetUser)
		api.GET("/users", userHandler.GetUserByEmail)

		// Order management (NEW)
		api.POST("/orders", orderHandler.CreateOrder)
		api.GET("/orders/:id", orderHandler.GetOrder)
		api.GET("/users/:id/orders", orderHandler.GetOrdersByUser)

		// Session management
		api.POST("/sessions", sessionHandler.CreateSession)
		api.GET("/sdk-config", sessionHandler.GetSDKConfig)
		api.GET("/sessions/:session_id/verify", sessionHandler.VerifySession)

		// Payment processing
		api.POST("/payments/process", paymentHandler.ProcessPayment)
		api.POST("/payments/refund", paymentHandler.RefundPayment)

		// Webhooks (for future use)
		api.POST("/webhooks/gateway", func(c *gin.Context) {
			c.JSON(200, gin.H{"received": true})
		})
	}

	// Start server
	port := cfg.ServerPort
	log.Printf("Mobile Backend starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
