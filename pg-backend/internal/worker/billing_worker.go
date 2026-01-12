package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"pg-backend/internal/config"
	"pg-backend/internal/services"
)

type BillingWorker struct {
	subscriptionService services.SubscriptionService
	billingService      services.BillingService
	cfg                 *config.Config
	logger              *log.Logger
	stopChan            chan bool
}

func NewBillingWorker(
	subscriptionService services.SubscriptionService,
	billingService services.BillingService,
	cfg *config.Config,
) *BillingWorker {
	return &BillingWorker{
		subscriptionService: subscriptionService,
		billingService:      billingService,
		cfg:                 cfg,
		logger:              log.New(log.Writer(), "[BILLING-WORKER] ", log.LstdFlags|log.Lshortfile),
		stopChan:            make(chan bool),
	}
}

// Start begins the billing worker with specified interval
func (w *BillingWorker) Start(ctx context.Context) error {
	w.logger.Println("Starting billing worker...")

	// Run immediately on startup
	w.runBillingCycle(ctx)

	// Schedule periodic runs
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Println("Stopping billing worker due to context cancellation")
			return ctx.Err()

		case <-w.stopChan:
			w.logger.Println("Stopping billing worker on request")
			return nil

		case <-ticker.C:
			w.runBillingCycle(ctx)
		}
	}
}

// Stop gracefully stops the billing worker
func (w *BillingWorker) Stop() {
	w.logger.Println("Shutting down billing worker...")
	close(w.stopChan)
}

// runBillingCycle executes all billing tasks
func (w *BillingWorker) runBillingCycle(ctx context.Context) {
	startTime := time.Now()
	w.logger.Println("Starting billing cycle at", startTime.Format("2006-01-02 15:04:05"))

	// Execute tasks sequentially
	tasks := []struct {
		name string
		fn   func(context.Context) (int, error)
	}{
		{"Process Due Subscriptions", w.processDueSubscriptions},
		{"Process Pending Billing Attempts", w.processPendingBillingAttempts},
		{"Retry Failed Payments", w.retryFailedPayments},
	}

	totalProcessed := 0
	for _, task := range tasks {
		processed, err := task.fn(ctx)
		if err != nil {
			w.logger.Printf("Error in task %s: %v", task.name, err)
		} else {
			w.logger.Printf("%s: Processed %d items", task.name, processed)
			totalProcessed += processed
		}
	}

	duration := time.Since(startTime)
	w.logger.Printf("Billing cycle completed in %v. Total processed: %d\n", duration, totalProcessed)
}

// processDueSubscriptions finds and processes subscriptions due for billing
func (w *BillingWorker) processDueSubscriptions(ctx context.Context) (int, error) {
	w.logger.Println("Processing due subscriptions...")

	// Process up to 100 subscriptions at a time
	processed, err := w.subscriptionService.ProcessDueSubscriptions(ctx, 100)
	if err != nil {
		return 0, fmt.Errorf("failed to process due subscriptions: %w", err)
	}

	if processed > 0 {
		w.logger.Printf("Successfully processed %d due subscriptions", processed)
	}

	return processed, nil
}

// processPendingBillingAttempts processes manual/admin-initiated billing attempts
func (w *BillingWorker) processPendingBillingAttempts(ctx context.Context) (int, error) {
	w.logger.Println("Processing pending billing attempts...")

	// Process up to 50 pending attempts at a time
	processed, err := w.billingService.ProcessPendingBillingAttempts(ctx, 50)
	if err != nil {
		return 0, fmt.Errorf("failed to process pending billing attempts: %w", err)
	}

	if processed > 0 {
		w.logger.Printf("Successfully processed %d pending billing attempts", processed)
	}

	return processed, nil
}

// retryFailedPayments retries failed billing attempts with exponential backoff
func (w *BillingWorker) retryFailedPayments(ctx context.Context) (int, error) {
	w.logger.Println("Retrying failed payments...")

	// Retry failed attempts (max 3 attempts total)
	retried, err := w.subscriptionService.RetryFailedBilling(ctx, 3)
	if err != nil {
		return 0, fmt.Errorf("failed to retry failed payments: %w", err)
	}

	if retried > 0 {
		w.logger.Printf("Created %d retry attempts for failed payments", retried)
	}

	return retried, nil
}

// HealthCheck returns worker status
func (w *BillingWorker) HealthCheck() map[string]interface{} {
	return map[string]interface{}{
		"status":    "running",
		"timestamp": time.Now().Format(time.RFC3339),
	}
}
