package main

import (
	"log"
	"os"

	"pg-backend/internal/config"
	"pg-backend/internal/database"
	"pg-backend/internal/handlers"
	"pg-backend/internal/repositories"
	"pg-backend/internal/services"
	"pg-backend/internal/worker"

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

	// Initialize repositories
	userRepo := repositories.NewUserRepository()
	cardRepo := repositories.NewCardRepository()
	transactionRepo := repositories.NewTransactionRepository()

	// NEW: Initialize subscription repositories
	planRepo := repositories.NewPlanRepository()
	subscriptionRepo := repositories.NewSubscriptionRepository()
	billingRepo := repositories.NewBillingRepository()

	// Initialize services
	mastercardService := services.NewMastercardService(cfg)

	// NEW: Initialize subscription services
	planService := services.NewPlanService(planRepo)
	billingService := services.NewBillingService(
		transactionRepo,
		billingRepo,
		cardRepo,
		subscriptionRepo,
		userRepo,
		mastercardService,
	)
	subscriptionService := services.NewSubscriptionService(
		subscriptionRepo,
		planRepo,
		cardRepo,
		billingRepo,
		transactionRepo,
		mastercardService,
	)

	// Initialize handlers
	cardHandler := handlers.NewCardHandler(mastercardService, userRepo, cardRepo)
	paymentHandler := handlers.NewPaymentHandler(mastercardService, userRepo, cardRepo, transactionRepo)
	authorizationHandler := handlers.NewAuthorizationHandler(mastercardService, userRepo, cardRepo, transactionRepo)

	// NEW: Initialize subscription handlers
	planHandler := handlers.NewPlanHandler(planService)
	subscriptionHandler := handlers.NewSubscriptionHandler(subscriptionService)
	billingHandler := handlers.NewBillingHandler(billingService)

	// NEW: Initialize worker
	workerManager := worker.NewWorkerManager()

	// Create billing worker
	billingWorker := worker.NewBillingWorker(
		subscriptionService,
		billingService,
		cfg,
	)

	// Register worker
	workerManager.RegisterWorker(billingWorker)

	// NEW: Initialize worker handler
	workerHandler := handlers.NewWorkerHandler(workerManager)

	// Start worker in background
	go func() {
		if err := workerManager.StartAll(); err != nil {
			log.Printf("Failed to start workers: %v", err)
		}
	}()

	// Ensure worker stops gracefully on shutdown
	defer workerManager.StopAll()

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

		// NEW: Plan endpoints
		api.GET("/plans", planHandler.GetPlans)
		api.GET("/plans/:id", planHandler.GetPlan)
		api.POST("/plans", planHandler.CreatePlan)
		api.PUT("/plans/:id", planHandler.UpdatePlan)
		api.DELETE("/plans/:id", planHandler.DeletePlan)
		api.GET("/plans/currency/:currency", planHandler.GetPlansByCurrency)

		// NEW: Subscription endpoints
		api.POST("/subscriptions", subscriptionHandler.CreateSubscription)
		api.GET("/subscriptions/:id", subscriptionHandler.GetSubscription)
		api.GET("/users/:user_id/subscriptions", subscriptionHandler.GetUserSubscriptions)
		api.POST("/subscriptions/:id/cancel", subscriptionHandler.CancelSubscription)
		api.PUT("/subscriptions/:id/card", subscriptionHandler.UpdateSubscriptionCard)

		// NEW: Billing endpoints
		api.POST("/billing/manual", billingHandler.CreateManualPayment)
		api.GET("/users/:user_id/billing-history", billingHandler.GetBillingHistory)
		api.GET("/subscriptions/:id/billing-history", billingHandler.GetSubscriptionBillingHistory)
		api.POST("/billing/process", billingHandler.ProcessBillingAttempts)

		// NEW: Add worker endpoints
		api.GET("/worker/status", workerHandler.GetWorkerStatus)
		api.POST("/worker/restart", workerHandler.RestartWorkers)
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
