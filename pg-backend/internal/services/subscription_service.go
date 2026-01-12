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

type SubscriptionService interface {
	CreateSubscription(ctx context.Context, userID, planID, cardID uuid.UUID, metadata map[string]string) (*models.Subscription, error)
	GetSubscription(ctx context.Context, subscriptionID uuid.UUID) (*models.Subscription, error)
	GetUserSubscriptions(ctx context.Context, userID uuid.UUID, status string) ([]models.Subscription, error)
	CancelSubscription(ctx context.Context, subscriptionID uuid.UUID, cancelAtPeriodEnd bool) error
	UpdateSubscriptionCard(ctx context.Context, subscriptionID, cardID uuid.UUID) error
	ProcessDueSubscriptions(ctx context.Context, limit int) (int, error)
	RetryFailedBilling(ctx context.Context, maxAttempts int) (int, error)
}

type subscriptionService struct {
	subscriptionRepo  repositories.SubscriptionRepository
	planRepo          repositories.PlanRepository
	cardRepo          repositories.CardRepository
	billingRepo       repositories.BillingRepository
	transactionRepo   repositories.TransactionRepository
	mastercardService MastercardService
}

func NewSubscriptionService(
	subscriptionRepo repositories.SubscriptionRepository,
	planRepo repositories.PlanRepository,
	cardRepo repositories.CardRepository,
	billingRepo repositories.BillingRepository,
	transactionRepo repositories.TransactionRepository,
	mastercardService MastercardService,
) SubscriptionService {
	return &subscriptionService{
		subscriptionRepo:  subscriptionRepo,
		planRepo:          planRepo,
		cardRepo:          cardRepo,
		billingRepo:       billingRepo,
		transactionRepo:   transactionRepo,
		mastercardService: mastercardService,
	}
}

func (s *subscriptionService) CreateSubscription(ctx context.Context, userID, planID, cardID uuid.UUID, metadata map[string]string) (*models.Subscription, error) {
	// 1. Validate plan exists and is active
	plan, err := s.planRepo.GetPlanByID(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("invalid plan: %w", err)
	}
	if !plan.IsActive {
		return nil, fmt.Errorf("plan is not active")
	}

	// 2. Validate card belongs to user
	card, err := s.cardRepo.GetCardByID(ctx, cardID)
	if err != nil {
		return nil, fmt.Errorf("invalid card: %w", err)
	}
	if card.UserID != userID {
		return nil, fmt.Errorf("card does not belong to user")
	}

	// 3. Check if user already has active subscription for this plan
	existingSubs, err := s.subscriptionRepo.GetSubscriptionsByUserID(ctx, userID, "active")
	if err == nil {
		for _, sub := range existingSubs {
			if sub.PlanID.UUID == planID && sub.Status == models.SubscriptionStatusActive {
				return nil, fmt.Errorf("user already has active subscription for this plan")
			}
		}
	}

	// 4. Calculate dates
	now := time.Now()
	subscription := &models.Subscription{
		UserID:    userID,
		PlanID:    uuid.NullUUID{UUID: planID, Valid: true},
		CardID:    uuid.NullUUID{UUID: cardID, Valid: true},
		PlanName:  plan.Name,
		Amount:    plan.Amount,
		Currency:  plan.Currency,
		Status:    models.SubscriptionStatusActive,
		Interval:  models.SubscriptionInterval(plan.Interval),
		Metadata:  metadata,
		CreatedAt: now,
	}

	// 5. Handle trial period
	if plan.TrialPeriodDays > 0 {
		subscription.Status = models.SubscriptionStatusTrialing
		subscription.TrialStart = sql.NullTime{Time: now, Valid: true}
		subscription.TrialEnd = sql.NullTime{Time: now.AddDate(0, 0, plan.TrialPeriodDays), Valid: true}
		subscription.NextBillingAt = subscription.TrialEnd.Time
	} else {
		// No trial - set first billing cycle
		subscription.CurrentPeriodStart = sql.NullTime{Time: now, Valid: true}
		subscription.BillingCycleAnchor = sql.NullTime{Time: now, Valid: true}
		subscription.NextBillingAt = s.calculateNextBillingDate(now, plan.Interval)
		subscription.CurrentPeriodEnd = sql.NullTime{Time: subscription.NextBillingAt, Valid: true}
	}

	// 6. Create subscription in database
	if err := s.subscriptionRepo.CreateSubscription(ctx, subscription); err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	// 7. If no trial, create first billing attempt immediately
	if plan.TrialPeriodDays == 0 {
		billingAttempt := &models.BillingAttempt{
			SubscriptionID: subscription.ID,
			Amount:         plan.Amount,
			Currency:       plan.Currency,
			Status:         models.BillingAttemptStatusPending,
			AttemptNumber:  1,
			ScheduledAt:    now,
		}
		if err := s.billingRepo.CreateBillingAttempt(ctx, billingAttempt); err != nil {
			// Log error but don't fail subscription creation
			fmt.Printf("Warning: Failed to create initial billing attempt: %v\n", err)
		}
	}

	return subscription, nil
}

func (s *subscriptionService) GetSubscription(ctx context.Context, subscriptionID uuid.UUID) (*models.Subscription, error) {
	return s.subscriptionRepo.GetSubscriptionByID(ctx, subscriptionID)
}

func (s *subscriptionService) GetUserSubscriptions(ctx context.Context, userID uuid.UUID, status string) ([]models.Subscription, error) {
	return s.subscriptionRepo.GetSubscriptionsByUserID(ctx, userID, status)
}

func (s *subscriptionService) CancelSubscription(ctx context.Context, subscriptionID uuid.UUID, cancelAtPeriodEnd bool) error {
	return s.subscriptionRepo.CancelSubscription(ctx, subscriptionID, cancelAtPeriodEnd)
}

func (s *subscriptionService) UpdateSubscriptionCard(ctx context.Context, subscriptionID, cardID uuid.UUID) error {
	// 1. Get subscription
	subscription, err := s.subscriptionRepo.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	// 2. Validate card belongs to user
	card, err := s.cardRepo.GetCardByID(ctx, cardID)
	if err != nil {
		return fmt.Errorf("invalid card: %w", err)
	}
	if card.UserID != subscription.UserID {
		return fmt.Errorf("card does not belong to user")
	}

	// 3. Update subscription with new card
	subscription.CardID = uuid.NullUUID{UUID: cardID, Valid: true}
	return s.subscriptionRepo.UpdateSubscription(ctx, subscription)
}

func (s *subscriptionService) ProcessDueSubscriptions(ctx context.Context, limit int) (int, error) {
	// Get subscriptions due for billing
	cutoffTime := time.Now().Add(5 * time.Minute) // Process items due in next 5 minutes
	subscriptions, err := s.subscriptionRepo.GetSubscriptionsDueForBilling(ctx, cutoffTime)
	if err != nil {
		return 0, fmt.Errorf("failed to get due subscriptions: %w", err)
	}

	processedCount := 0
	for _, subscription := range subscriptions {
		if err := s.processSingleSubscription(ctx, &subscription); err != nil {
			fmt.Printf("Failed to process subscription %s: %v\n", subscription.ID, err)
			continue
		}
		processedCount++

		if processedCount >= limit {
			break
		}
	}

	return processedCount, nil
}

func (s *subscriptionService) processSingleSubscription(ctx context.Context, subscription *models.Subscription) error {
	// 1. Create billing attempt
	billingAttempt := &models.BillingAttempt{
		SubscriptionID: subscription.ID,
		Amount:         subscription.Amount,
		Currency:       subscription.Currency,
		Status:         models.BillingAttemptStatusProcessing,
		AttemptNumber:  1,
		ScheduledAt:    time.Now(),
		ProcessedAt:    sql.NullTime{Time: time.Now(), Valid: true},
	}

	if err := s.billingRepo.CreateBillingAttempt(ctx, billingAttempt); err != nil {
		return fmt.Errorf("failed to create billing attempt: %w", err)
	}

	// 2. Get card for payment
	card, err := s.cardRepo.GetCardByID(ctx, subscription.CardID.UUID)
	if err != nil {
		billingAttempt.Status = models.BillingAttemptStatusFailed
		billingAttempt.ErrorMessage = sql.NullString{String: "Card not found", Valid: true}
		s.billingRepo.UpdateBillingAttempt(ctx, billingAttempt)
		return fmt.Errorf("card not found: %w", err)
	}

	// 3. Process payment via Mastercard
	amountStr := fmt.Sprintf("%.2f", subscription.Amount)
	paymentResp, err := s.mastercardService.PayWithToken(
		card.GatewayToken,
		amountStr,
		subscription.Currency,
	)
	if err != nil {
		billingAttempt.Status = models.BillingAttemptStatusFailed
		billingAttempt.ErrorMessage = sql.NullString{String: err.Error(), Valid: true}
		s.billingRepo.UpdateBillingAttempt(ctx, billingAttempt)
		return fmt.Errorf("payment failed: %w", err)
	}

	// 4. Check payment result
	if paymentResp.Result != "SUCCESS" || paymentResp.GatewayCode != "APPROVED" {
		billingAttempt.Status = models.BillingAttemptStatusFailed
		billingAttempt.ErrorCode = sql.NullString{String: paymentResp.GatewayCode, Valid: true}
		billingAttempt.ErrorMessage = sql.NullString{String: paymentResp.Result, Valid: true}
		s.billingRepo.UpdateBillingAttempt(ctx, billingAttempt)

		// Update subscription status if payment failed
		if subscription.Status == models.SubscriptionStatusActive {
			subscription.Status = models.SubscriptionStatusPastDue
			s.subscriptionRepo.UpdateSubscription(ctx, subscription)
		}
		return fmt.Errorf("payment declined: %s", paymentResp.GatewayCode)
	}

	// 5. Payment succeeded - update billing attempt
	billingAttempt.Status = models.BillingAttemptStatusSucceeded
	billingAttempt.GatewayTransactionID = sql.NullString{String: paymentResp.Transaction.ID, Valid: true}
	if err := s.billingRepo.UpdateBillingAttempt(ctx, billingAttempt); err != nil {
		return fmt.Errorf("failed to update billing attempt: %w", err)
	}

	// 6. Record transaction
	transaction := &models.Transaction{
		UserID:               subscription.UserID,
		CardID:               subscription.CardID.UUID,
		Amount:               subscription.Amount,
		Currency:             subscription.Currency,
		Status:               paymentResp.Transaction.Status,
		GatewayTransactionID: paymentResp.Transaction.ID,
		Type:                 "recurring",
		InvoiceID:            sql.NullString{String: fmt.Sprintf("INV-%d", time.Now().Unix()), Valid: true},
	}

	if err := s.transactionRepo.CreateSubscriptionTransaction(
		ctx, transaction, subscription.ID, billingAttempt.ID,
	); err != nil {
		fmt.Printf("Warning: Failed to record transaction: %v\n", err)
	}

	// 7. Update subscription dates for next billing
	subscription.CurrentPeriodStart = sql.NullTime{Time: subscription.NextBillingAt, Valid: true}
	subscription.NextBillingAt = s.calculateNextBillingDate(subscription.NextBillingAt, string(subscription.Interval))
	subscription.CurrentPeriodEnd = sql.NullTime{Time: subscription.NextBillingAt, Valid: true}

	// If subscription was past_due, set back to active
	if subscription.Status == models.SubscriptionStatusPastDue {
		subscription.Status = models.SubscriptionStatusActive
	}

	return s.subscriptionRepo.UpdateSubscription(ctx, subscription)
}

// internal/services/subscription_service.go (Update existing method)
func (s *subscriptionService) RetryFailedBilling(ctx context.Context, maxAttempts int) (int, error) {
	// Get failed billing attempts older than appropriate times based on attempt number
	olderThan := time.Now().Add(-24 * time.Hour)
	attempts, err := s.billingRepo.GetFailedBillingAttemptsForRetry(ctx, maxAttempts, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to get failed attempts: %w", err)
	}

	retryCount := 0
	for _, attempt := range attempts {
		// Get subscription
		subscription, err := s.subscriptionRepo.GetSubscriptionByID(ctx, attempt.SubscriptionID)
		if err != nil {
			fmt.Printf("Subscription not found for attempt %s: %v\n", attempt.ID, err)
			continue
		}

		// Check if subscription is still active
		if subscription.Status != models.SubscriptionStatusActive &&
			subscription.Status != models.SubscriptionStatusPastDue {
			continue // Don't retry for canceled/inactive subscriptions
		}

		// Calculate exponential backoff: immediate, 3 days, 7 days
		var retryDelay time.Duration
		switch attempt.AttemptNumber {
		case 1:
			retryDelay = 0 // Immediate retry
		case 2:
			retryDelay = 72 * time.Hour // 3 days
		case 3:
			retryDelay = 168 * time.Hour // 7 days
		default:
			continue // No more retries
		}

		// Create new billing attempt
		newAttempt := &models.BillingAttempt{
			SubscriptionID: attempt.SubscriptionID,
			Amount:         attempt.Amount,
			Currency:       attempt.Currency,
			Status:         models.BillingAttemptStatusPending,
			AttemptNumber:  attempt.AttemptNumber + 1,
			ScheduledAt:    time.Now().Add(retryDelay),
		}

		if err := s.billingRepo.CreateBillingAttempt(ctx, newAttempt); err != nil {
			fmt.Printf("Failed to create retry attempt: %v\n", err)
			continue
		}

		// Update subscription status to past_due if first failure
		if attempt.AttemptNumber == 1 && subscription.Status == models.SubscriptionStatusActive {
			subscription.Status = models.SubscriptionStatusPastDue
			if err := s.subscriptionRepo.UpdateSubscription(ctx, subscription); err != nil {
				fmt.Printf("Failed to update subscription status: %v\n", err)
			}
		}

		retryCount++
	}

	return retryCount, nil
}

// Helper function to calculate next billing date
func (s *subscriptionService) calculateNextBillingDate(from time.Time, interval string) time.Time {
	switch interval {
	case "day":
		return from.AddDate(0, 0, 1)
	case "week":
		return from.AddDate(0, 0, 7)
	case "month":
		return from.AddDate(0, 1, 0)
	case "year":
		return from.AddDate(1, 0, 0)
	default:
		return from.AddDate(0, 1, 0) // Default to monthly
	}
}
