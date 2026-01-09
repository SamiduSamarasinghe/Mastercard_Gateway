package repositories

import (
	"context"
	"database/sql"
	"pg-backend/internal/database"
	"pg-backend/internal/models"
	"time"

	"github.com/google/uuid"
)

type BillingRepository interface {
	CreateBillingAttempt(ctx context.Context, attempt *models.BillingAttempt) error
	GetBillingAttemptByID(ctx context.Context, id uuid.UUID) (*models.BillingAttempt, error)
	GetBillingAttemptsBySubscriptionID(ctx context.Context, subscriptionID uuid.UUID) ([]models.BillingAttempt, error)
	UpdateBillingAttempt(ctx context.Context, attempt *models.BillingAttempt) error
	GetPendingBillingAttempts(ctx context.Context, limit int) ([]models.BillingAttempt, error)
	GetFailedBillingAttemptsForRetry(ctx context.Context, maxAttempts int, olderThan time.Time) ([]models.BillingAttempt, error)
}

type billingRepository struct {
	db *sql.DB
}

func NewBillingRepository() BillingRepository {
	return &billingRepository{
		db: database.DB,
	}
}

func (r *billingRepository) CreateBillingAttempt(ctx context.Context, attempt *models.BillingAttempt) error {
	query := `
		INSERT INTO billing_attempts (
			subscription_id, amount, currency, status, gateway_transaction_id,
			error_code, error_message, attempt_number, scheduled_at, processed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`

	err := r.db.QueryRowContext(ctx, query,
		attempt.SubscriptionID,
		attempt.Amount,
		attempt.Currency,
		attempt.Status,
		attempt.GatewayTransactionID,
		attempt.ErrorCode,
		attempt.ErrorMessage,
		attempt.AttemptNumber,
		attempt.ScheduledAt,
		attempt.ProcessedAt,
	).Scan(&attempt.ID, &attempt.CreatedAt)

	return err
}

func (r *billingRepository) GetBillingAttemptByID(ctx context.Context, id uuid.UUID) (*models.BillingAttempt, error) {
	query := `
		SELECT 
			id, subscription_id, amount, currency, status, gateway_transaction_id,
			error_code, error_message, attempt_number, scheduled_at, processed_at, created_at
		FROM billing_attempts
		WHERE id = $1
	`

	attempt := &models.BillingAttempt{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&attempt.ID,
		&attempt.SubscriptionID,
		&attempt.Amount,
		&attempt.Currency,
		&attempt.Status,
		&attempt.GatewayTransactionID,
		&attempt.ErrorCode,
		&attempt.ErrorMessage,
		&attempt.AttemptNumber,
		&attempt.ScheduledAt,
		&attempt.ProcessedAt,
		&attempt.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "billing attempt not found"}
	}
	if err != nil {
		return nil, err
	}

	return attempt, nil
}

func (r *billingRepository) GetBillingAttemptsBySubscriptionID(ctx context.Context, subscriptionID uuid.UUID) ([]models.BillingAttempt, error) {
	query := `
		SELECT 
			id, subscription_id, amount, currency, status, gateway_transaction_id,
			error_code, error_message, attempt_number, scheduled_at, processed_at, created_at
		FROM billing_attempts
		WHERE subscription_id = $1
		ORDER BY created_at DESC
		LIMIT 50
	`

	rows, err := r.db.QueryContext(ctx, query, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []models.BillingAttempt
	for rows.Next() {
		var attempt models.BillingAttempt
		err := rows.Scan(
			&attempt.ID,
			&attempt.SubscriptionID,
			&attempt.Amount,
			&attempt.Currency,
			&attempt.Status,
			&attempt.GatewayTransactionID,
			&attempt.ErrorCode,
			&attempt.ErrorMessage,
			&attempt.AttemptNumber,
			&attempt.ScheduledAt,
			&attempt.ProcessedAt,
			&attempt.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}

	return attempts, nil
}

func (r *billingRepository) UpdateBillingAttempt(ctx context.Context, attempt *models.BillingAttempt) error {
	query := `
		UPDATE billing_attempts
		SET 
			status = $1,
			gateway_transaction_id = $2,
			error_code = $3,
			error_message = $4,
			attempt_number = $5,
			processed_at = $6
		WHERE id = $7
	`

	result, err := r.db.ExecContext(ctx, query,
		attempt.Status,
		attempt.GatewayTransactionID,
		attempt.ErrorCode,
		attempt.ErrorMessage,
		attempt.AttemptNumber,
		attempt.ProcessedAt,
		attempt.ID,
	)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return &NotFoundError{Message: "billing attempt not found"}
	}

	return nil
}

func (r *billingRepository) GetPendingBillingAttempts(ctx context.Context, limit int) ([]models.BillingAttempt, error) {
	query := `
		SELECT 
			id, subscription_id, amount, currency, status, gateway_transaction_id,
			error_code, error_message, attempt_number, scheduled_at, processed_at, created_at
		FROM billing_attempts
		WHERE status IN ('pending', 'requires_action')
		AND scheduled_at <= CURRENT_TIMESTAMP
		ORDER BY scheduled_at ASC
		LIMIT $1
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []models.BillingAttempt
	for rows.Next() {
		var attempt models.BillingAttempt
		err := rows.Scan(
			&attempt.ID,
			&attempt.SubscriptionID,
			&attempt.Amount,
			&attempt.Currency,
			&attempt.Status,
			&attempt.GatewayTransactionID,
			&attempt.ErrorCode,
			&attempt.ErrorMessage,
			&attempt.AttemptNumber,
			&attempt.ScheduledAt,
			&attempt.ProcessedAt,
			&attempt.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}

	return attempts, nil
}

func (r *billingRepository) GetFailedBillingAttemptsForRetry(ctx context.Context, maxAttempts int, olderThan time.Time) ([]models.BillingAttempt, error) {
	query := `
		SELECT 
			id, subscription_id, amount, currency, status, gateway_transaction_id,
			error_code, error_message, attempt_number, scheduled_at, processed_at, created_at
		FROM billing_attempts
		WHERE status = 'failed'
		AND attempt_number < $1
		AND processed_at < $2
		AND error_code NOT IN ('card_declined', 'insufficient_funds', 'invalid_card')
		ORDER BY processed_at ASC
		LIMIT 50
	`

	rows, err := r.db.QueryContext(ctx, query, maxAttempts, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []models.BillingAttempt
	for rows.Next() {
		var attempt models.BillingAttempt
		err := rows.Scan(
			&attempt.ID,
			&attempt.SubscriptionID,
			&attempt.Amount,
			&attempt.Currency,
			&attempt.Status,
			&attempt.GatewayTransactionID,
			&attempt.ErrorCode,
			&attempt.ErrorMessage,
			&attempt.AttemptNumber,
			&attempt.ScheduledAt,
			&attempt.ProcessedAt,
			&attempt.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}

	return attempts, nil
}
