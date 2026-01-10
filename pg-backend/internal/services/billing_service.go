package services

import (
	"context"
	"database/sql"
	"fmt"
	"pg-backend/internal/models"
	"pg-backend/internal/repositories"
	"time"

	"github.com/google/uuid"
)

type BillingService interface {
	CreateManualPayment(ctx context.Context, userID, cardID uuid.UUID, amount float64, currency, description string) (*models.Transaction, error)
	GetBillingHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Transaction, error)
	GetSubscriptionBillingHistory(ctx context.Context, subscriptionID uuid.UUID) ([]models.BillingAttempt, error)
	ProcessPendingBillingAttempts(ctx context.Context, limit int) (int, error)
}

type billingService struct {
	transactionRepo   repositories.TransactionRepository
	billingRepo       repositories.BillingRepository
	subscriptionRepo  repositories.SubscriptionRepository
	cardRepo          repositories.CardRepository
	userRepo          repositories.UserRepository
	mastercardService MastercardService
}

func NewBillingService(
	transactionRepo repositories.TransactionRepository,
	billingRepo repositories.BillingRepository,
	cardRepo repositories.CardRepository,
	subscriptionRepo repositories.SubscriptionRepository,
	userRepo repositories.UserRepository,
	mastercardService MastercardService,
) BillingService {
	return &billingService{
		transactionRepo:   transactionRepo,
		billingRepo:       billingRepo,
		subscriptionRepo:  subscriptionRepo,
		cardRepo:          cardRepo,
		userRepo:          userRepo,
		mastercardService: mastercardService,
	}
}

func (s *billingService) CreateManualPayment(ctx context.Context, userID, cardID uuid.UUID, amount float64, currency, description string) (*models.Transaction, error) {
	// 1. Validate user exists
	_, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// 2. Validate card belongs to user
	card, err := s.cardRepo.GetCardByID(ctx, cardID)
	if err != nil {
		return nil, fmt.Errorf("card not found: %w", err)
	}
	if card.UserID != userID {
		return nil, fmt.Errorf("card does not belong to user")
	}

	// 3. Validate amount
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be greater than 0")
	}

	// 4. Default currency to LKR if not specified
	if currency == "" {
		currency = "LKR"
	}

	// 5. Process payment via Mastercard
	amountStr := fmt.Sprintf("%.2f", amount)
	paymentResp, err := s.mastercardService.PayWithToken(
		card.GatewayToken,
		amountStr,
		currency,
	)
	if err != nil {
		return nil, fmt.Errorf("payment failed: %w", err)
	}

	// 6. Check payment result
	if paymentResp.Result != "SUCCESS" || paymentResp.GatewayCode != "APPROVED" {
		return nil, fmt.Errorf("payment declined: %s", paymentResp.GatewayCode)
	}

	// 7. Record transaction
	transaction := &models.Transaction{
		UserID:               userID,
		CardID:               cardID,
		Amount:               amount,
		Currency:             currency,
		Status:               paymentResp.Transaction.Status,
		GatewayTransactionID: paymentResp.Transaction.ID,
		Type:                 "manual",
	}

	if err := s.transactionRepo.CreateTransaction(ctx, transaction); err != nil {
		// Log error but return payment success
		fmt.Printf("Warning: Failed to save transaction to database: %v\n", err)
	}

	return transaction, nil
}

func (s *billingService) GetBillingHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Transaction, error) {
	// Validate user exists
	_, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Get all transactions for user
	allTransactions, err := s.transactionRepo.GetTransactionsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Apply pagination manually (for simplicity)
	// In production, you'd want to add pagination to the repository
	start := offset
	end := offset + limit
	if end > len(allTransactions) {
		end = len(allTransactions)
	}
	if start > len(allTransactions) {
		return []models.Transaction{}, nil
	}

	return allTransactions[start:end], nil
}

func (s *billingService) GetSubscriptionBillingHistory(ctx context.Context, subscriptionID uuid.UUID) ([]models.BillingAttempt, error) {
	return s.billingRepo.GetBillingAttemptsBySubscriptionID(ctx, subscriptionID)
}

func (s *billingService) ProcessPendingBillingAttempts(ctx context.Context, limit int) (int, error) {
	// Get pending billing attempts
	attempts, err := s.billingRepo.GetPendingBillingAttempts(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("failed to get pending attempts: %w", err)
	}

	processedCount := 0
	for _, attempt := range attempts {
		if err := s.processBillingAttempt(ctx, &attempt); err != nil {
			fmt.Printf("Failed to process billing attempt %s: %v\n", attempt.ID, err)
			continue
		}
		processedCount++
	}

	return processedCount, nil
}

func (s *billingService) processBillingAttempt(ctx context.Context, attempt *models.BillingAttempt) error {
	// 1. Update attempt status to processing
	attempt.Status = models.BillingAttemptStatusProcessing
	attempt.ProcessedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if err := s.billingRepo.UpdateBillingAttempt(ctx, attempt); err != nil {
		return fmt.Errorf("failed to update attempt status: %w", err)
	}

	// 2. Get subscription
	subscription, err := s.subscriptionRepo.GetSubscriptionByID(ctx, attempt.SubscriptionID)
	if err != nil {
		attempt.Status = models.BillingAttemptStatusFailed
		attempt.ErrorMessage = sql.NullString{String: "Subscription not found", Valid: true}
		s.billingRepo.UpdateBillingAttempt(ctx, attempt)
		return fmt.Errorf("subscription not found: %w", err)
	}

	// 3. Get card
	card, err := s.cardRepo.GetCardByID(ctx, subscription.CardID.UUID)
	if err != nil {
		attempt.Status = models.BillingAttemptStatusFailed
		attempt.ErrorMessage = sql.NullString{String: "Card not found", Valid: true}
		s.billingRepo.UpdateBillingAttempt(ctx, attempt)
		return fmt.Errorf("card not found: %w", err)
	}

	// 4. Process payment
	amountStr := fmt.Sprintf("%.2f", attempt.Amount)
	paymentResp, err := s.mastercardService.PayWithToken(
		card.GatewayToken,
		amountStr,
		attempt.Currency,
	)
	if err != nil {
		attempt.Status = models.BillingAttemptStatusFailed
		attempt.ErrorMessage = sql.NullString{String: err.Error(), Valid: true}
		s.billingRepo.UpdateBillingAttempt(ctx, attempt)
		return fmt.Errorf("payment failed: %w", err)
	}

	// 5. Check payment result
	if paymentResp.Result != "SUCCESS" || paymentResp.GatewayCode != "APPROVED" {
		attempt.Status = models.BillingAttemptStatusFailed
		attempt.ErrorCode = sql.NullString{String: paymentResp.GatewayCode, Valid: true}
		attempt.ErrorMessage = sql.NullString{String: paymentResp.Result, Valid: true}
		s.billingRepo.UpdateBillingAttempt(ctx, attempt)
		return fmt.Errorf("payment declined: %s", paymentResp.GatewayCode)
	}

	// 6. Payment succeeded
	attempt.Status = models.BillingAttemptStatusSucceeded
	attempt.GatewayTransactionID = sql.NullString{String: paymentResp.Transaction.ID, Valid: true}
	if err := s.billingRepo.UpdateBillingAttempt(ctx, attempt); err != nil {
		return fmt.Errorf("failed to update attempt: %w", err)
	}

	// 7. Record transaction
	transaction := &models.Transaction{
		UserID:               subscription.UserID,
		CardID:               subscription.CardID.UUID,
		Amount:               attempt.Amount,
		Currency:             attempt.Currency,
		Status:               paymentResp.Transaction.Status,
		GatewayTransactionID: paymentResp.Transaction.ID,
		Type:                 "recurring",
		InvoiceID:            sql.NullString{String: fmt.Sprintf("INV-%d", time.Now().Unix()), Valid: true},
	}

	if err := s.transactionRepo.CreateSubscriptionTransaction(
		ctx, transaction, subscription.ID, attempt.ID,
	); err != nil {
		fmt.Printf("Warning: Failed to record transaction: %v\n", err)
	}

	return nil
}
