package main

import (
	"log"
	"os"

	"pg-backend/internal/config"
	"pg-backend/internal/database"
	"pg-backend/internal/handlers"
	"pg-backend/internal/repositories"
	"pg-backend/internal/services"

	"github.com/joho/godotenv"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Load configuration
	cfg := config.LoadConfig()

	// Connect to database
	err = database.ConnectDB(cfg)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer database.DB.Close()

	// Test database connection
	var result string
	err = database.DB.QueryRow("SELECT 'Database connection successful'").Scan(&result)
	if err != nil {
		log.Fatal("Database test query failed:", err)
	}
	log.Println(result)

	// Run database migrations
	log.Println("Running database migrations...")
	if err := database.RunMigrations(database.DB); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}
	log.Println("Migrations completed successfully")

	// Initialize repositories
	userRepo := repositories.NewUserRepository()
	cardRepo := repositories.NewCardRepository()
	transactionRepo := repositories.NewTransactionRepository()

	// Initialize services
	mastercardService := services.NewMastercardService(cfg)

	// Initialize handlers
	cardHandler := handlers.NewCardHandler(mastercardService, userRepo, cardRepo)
	paymentHandler := handlers.NewPaymentHandler(mastercardService, userRepo, cardRepo, transactionRepo)
	authorizationHandler := handlers.NewAuthorizationHandler(mastercardService, userRepo, cardRepo, transactionRepo)

	// Setup Gin router
	router := gin.Default()

	// API routes
	api := router.Group("/api/v1")
	{
		// User endpoints
		api.POST("/users", paymentHandler.CreateUser)

		// Card endpoints
		api.POST("/cards/verify", cardHandler.VerifyAndSaveCard)
		api.GET("/users/:user_id/cards", cardHandler.GetUserCards)
		api.DELETE("/cards", cardHandler.DeleteCard)

		// Payment endpoints
		api.POST("/pay", paymentHandler.Pay)
		api.POST("/refund", paymentHandler.Refund)

		// Authorization flow endpoints (AUTHORIZE-CAPTURE-VOID)
		api.POST("/authorize", authorizationHandler.Authorize)
		api.POST("/capture", authorizationHandler.Capture)
		api.POST("/void", authorizationHandler.Void)
		api.POST("/update-authorization", authorizationHandler.UpdateAuthorization)

		// Transaction endpoints
		api.GET("/users/:user_id/transactions", paymentHandler.GetTransactions)
		api.GET("/transactions/:transaction_id", paymentHandler.GetTransactionByID)
	}

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
