package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"pg-backend/internal/database"
	"pg-backend/internal/models"
	"time"

	"github.com/google/uuid"
)

type SubscriptionRepository interface {
	CreateSubscription(ctx context.Context, subscription *models.Subscription) error
	GetSubscriptionByID(ctx context.Context, id uuid.UUID) (*models.Subscription, error)
	GetSubscriptionsByUserID(ctx context.Context, userID uuid.UUID, status string) ([]models.Subscription, error)
	UpdateSubscription(ctx context.Context, subscription *models.Subscription) error
	CancelSubscription(ctx context.Context, id uuid.UUID, cancelAtPeriodEnd bool) error
	GetSubscriptionsDueForBilling(ctx context.Context, cutoffTime time.Time) ([]models.Subscription, error)
	GetActiveSubscriptionCount(ctx context.Context) (int, error)
}

type subscriptionRepository struct {
	db *sql.DB
}

func NewSubscriptionRepository() SubscriptionRepository {
	return &subscriptionRepository{
		db: database.DB,
	}
}

func (r *subscriptionRepository) CreateSubscription(ctx context.Context, subscription *models.Subscription) error {
	// Convert metadata map to JSON
	metadataJSON := "{}"
	if subscription.Metadata != nil && len(subscription.Metadata) > 0 {
		metadataBytes, err := json.Marshal(subscription.Metadata)
		if err != nil {
			return err
		}
		metadataJSON = string(metadataBytes)
	}

	query := `
		INSERT INTO subscriptions (
			user_id, plan_id, card_id, plan_name, amount, currency, status, 
			interval, current_period_start, current_period_end, trial_start, 
			trial_end, cancel_at_period_end, metadata, billing_cycle_anchor, 
			next_billing_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id, created_at
	`

	err := r.db.QueryRowContext(ctx, query,
		subscription.UserID,
		subscription.PlanID,
		subscription.CardID,
		subscription.PlanName,
		subscription.Amount,
		subscription.Currency,
		subscription.Status,
		subscription.Interval,
		subscription.CurrentPeriodStart,
		subscription.CurrentPeriodEnd,
		subscription.TrialStart,
		subscription.TrialEnd,
		subscription.CancelAtPeriodEnd,
		metadataJSON,
		subscription.BillingCycleAnchor,
		subscription.NextBillingAt,
	).Scan(&subscription.ID, &subscription.CreatedAt)

	return err
}

func (r *subscriptionRepository) GetSubscriptionByID(ctx context.Context, id uuid.UUID) (*models.Subscription, error) {
	query := `
		SELECT 
			id, user_id, plan_id, card_id, plan_name, amount, currency, status,
			interval, current_period_start, current_period_end, trial_start,
			trial_end, cancel_at_period_end, canceled_at, metadata, 
			billing_cycle_anchor, next_billing_at, created_at
		FROM subscriptions
		WHERE id = $1
	`

	var (
		subscription models.Subscription
		metadataJSON sql.NullString
		planID       sql.NullString
		cardID       sql.NullString
	)

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&subscription.ID,
		&subscription.UserID,
		&planID,
		&cardID,
		&subscription.PlanName,
		&subscription.Amount,
		&subscription.Currency,
		&subscription.Status,
		&subscription.Interval,
		&subscription.CurrentPeriodStart,
		&subscription.CurrentPeriodEnd,
		&subscription.TrialStart,
		&subscription.TrialEnd,
		&subscription.CancelAtPeriodEnd,
		&subscription.CanceledAt,
		&metadataJSON,
		&subscription.BillingCycleAnchor,
		&subscription.NextBillingAt,
		&subscription.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "subscription not found"}
	}
	if err != nil {
		return nil, err
	}

	// Parse UUIDs
	if planID.Valid {
		if parsedID, err := uuid.Parse(planID.String); err == nil {
			subscription.PlanID = uuid.NullUUID{UUID: parsedID, Valid: true}
		}
	}
	if cardID.Valid {
		if parsedID, err := uuid.Parse(cardID.String); err == nil {
			subscription.CardID = uuid.NullUUID{UUID: parsedID, Valid: true}
		}
	}

	// Parse metadata
	if metadataJSON.Valid && metadataJSON.String != "" {
		metadata := make(map[string]string)
		if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
			subscription.Metadata = metadata
		}
	}

	return &subscription, nil
}

func (r *subscriptionRepository) GetSubscriptionsByUserID(ctx context.Context, userID uuid.UUID, status string) ([]models.Subscription, error) {
	var query string
	var args []interface{}

	if status != "" {
		query = `
			SELECT 
				id, user_id, plan_id, card_id, plan_name, amount, currency, status,
				interval, current_period_start, current_period_end, trial_start,
				trial_end, cancel_at_period_end, canceled_at, metadata, 
				billing_cycle_anchor, next_billing_at, created_at
			FROM subscriptions
			WHERE user_id = $1 AND status = $2
			ORDER BY created_at DESC
		`
		args = []interface{}{userID, status}
	} else {
		query = `
			SELECT 
				id, user_id, plan_id, card_id, plan_name, amount, currency, status,
				interval, current_period_start, current_period_end, trial_start,
				trial_end, cancel_at_period_end, canceled_at, metadata, 
				billing_cycle_anchor, next_billing_at, created_at
			FROM subscriptions
			WHERE user_id = $1
			ORDER BY 
				CASE status 
					WHEN 'active' THEN 1
					WHEN 'trialing' THEN 2
					WHEN 'past_due' THEN 3
					ELSE 4
				END,
				created_at DESC
		`
		args = []interface{}{userID}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subscriptions []models.Subscription
	for rows.Next() {
		var (
			subscription models.Subscription
			metadataJSON sql.NullString
			planID       sql.NullString
			cardID       sql.NullString
		)

		err := rows.Scan(
			&subscription.ID,
			&subscription.UserID,
			&planID,
			&cardID,
			&subscription.PlanName,
			&subscription.Amount,
			&subscription.Currency,
			&subscription.Status,
			&subscription.Interval,
			&subscription.CurrentPeriodStart,
			&subscription.CurrentPeriodEnd,
			&subscription.TrialStart,
			&subscription.TrialEnd,
			&subscription.CancelAtPeriodEnd,
			&subscription.CanceledAt,
			&metadataJSON,
			&subscription.BillingCycleAnchor,
			&subscription.NextBillingAt,
			&subscription.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		// Parse UUIDs
		if planID.Valid {
			if parsedID, err := uuid.Parse(planID.String); err == nil {
				subscription.PlanID = uuid.NullUUID{UUID: parsedID, Valid: true}
			}
		}
		if cardID.Valid {
			if parsedID, err := uuid.Parse(cardID.String); err == nil {
				subscription.CardID = uuid.NullUUID{UUID: parsedID, Valid: true}
			}
		}

		// Parse metadata
		if metadataJSON.Valid && metadataJSON.String != "" {
			metadata := make(map[string]string)
			if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
				subscription.Metadata = metadata
			}
		}

		subscriptions = append(subscriptions, subscription)
	}

	return subscriptions, nil
}

func (r *subscriptionRepository) UpdateSubscription(ctx context.Context, subscription *models.Subscription) error {
	// Convert metadata map to JSON
	metadataJSON := "{}"
	if subscription.Metadata != nil && len(subscription.Metadata) > 0 {
		metadataBytes, err := json.Marshal(subscription.Metadata)
		if err != nil {
			return err
		}
		metadataJSON = string(metadataBytes)
	}

	query := `
		UPDATE subscriptions
		SET 
			plan_id = $1,
			card_id = $2,
			plan_name = $3,
			amount = $4,
			currency = $5,
			status = $6,
			interval = $7,
			current_period_start = $8,
			current_period_end = $9,
			trial_start = $10,
			trial_end = $11,
			cancel_at_period_end = $12,
			canceled_at = $13,
			metadata = $14,
			billing_cycle_anchor = $15,
			next_billing_at = $16
		WHERE id = $17
		RETURNING created_at
	`

	err := r.db.QueryRowContext(ctx, query,
		subscription.PlanID,
		subscription.CardID,
		subscription.PlanName,
		subscription.Amount,
		subscription.Currency,
		subscription.Status,
		subscription.Interval,
		subscription.CurrentPeriodStart,
		subscription.CurrentPeriodEnd,
		subscription.TrialStart,
		subscription.TrialEnd,
		subscription.CancelAtPeriodEnd,
		subscription.CanceledAt,
		metadataJSON,
		subscription.BillingCycleAnchor,
		subscription.NextBillingAt,
		subscription.ID,
	).Scan(&subscription.CreatedAt)

	if err == sql.ErrNoRows {
		return &NotFoundError{Message: "subscription not found"}
	}
	if err != nil {
		return err
	}

	return nil
}

func (r *subscriptionRepository) CancelSubscription(ctx context.Context, id uuid.UUID, cancelAtPeriodEnd bool) error {
	query := `
		UPDATE subscriptions
		SET 
			status = CASE 
				WHEN $1 = true THEN status
				ELSE 'canceled'
			END,
			cancel_at_period_end = $1,
			canceled_at = CASE 
				WHEN $1 = true THEN canceled_at
				ELSE CURRENT_TIMESTAMP
			END
		WHERE id = $2
	`

	result, err := r.db.ExecContext(ctx, query, cancelAtPeriodEnd, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return &NotFoundError{Message: "subscription not found"}
	}

	return nil
}

func (r *subscriptionRepository) GetSubscriptionsDueForBilling(ctx context.Context, cutoffTime time.Time) ([]models.Subscription, error) {
	query := `
		SELECT 
			id, user_id, plan_id, card_id, plan_name, amount, currency, status,
			interval, current_period_start, current_period_end, trial_start,
			trial_end, cancel_at_period_end, canceled_at, metadata, 
			billing_cycle_anchor, next_billing_at, created_at
		FROM subscriptions
		WHERE 
			status IN ('active', 'trialing')
			AND cancel_at_period_end = false
			AND next_billing_at <= $1
			AND (trial_end IS NULL OR trial_end <= CURRENT_TIMESTAMP)
		ORDER BY next_billing_at ASC
		LIMIT 100
	`

	rows, err := r.db.QueryContext(ctx, query, cutoffTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subscriptions []models.Subscription
	for rows.Next() {
		var (
			subscription models.Subscription
			metadataJSON sql.NullString
			planID       sql.NullString
			cardID       sql.NullString
		)

		err := rows.Scan(
			&subscription.ID,
			&subscription.UserID,
			&planID,
			&cardID,
			&subscription.PlanName,
			&subscription.Amount,
			&subscription.Currency,
			&subscription.Status,
			&subscription.Interval,
			&subscription.CurrentPeriodStart,
			&subscription.CurrentPeriodEnd,
			&subscription.TrialStart,
			&subscription.TrialEnd,
			&subscription.CancelAtPeriodEnd,
			&subscription.CanceledAt,
			&metadataJSON,
			&subscription.BillingCycleAnchor,
			&subscription.NextBillingAt,
			&subscription.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		// Parse UUIDs
		if planID.Valid {
			if parsedID, err := uuid.Parse(planID.String); err == nil {
				subscription.PlanID = uuid.NullUUID{UUID: parsedID, Valid: true}
			}
		}
		if cardID.Valid {
			if parsedID, err := uuid.Parse(cardID.String); err == nil {
				subscription.CardID = uuid.NullUUID{UUID: parsedID, Valid: true}
			}
		}

		// Parse metadata
		if metadataJSON.Valid && metadataJSON.String != "" {
			metadata := make(map[string]string)
			if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
				subscription.Metadata = metadata
			}
		}

		subscriptions = append(subscriptions, subscription)
	}

	return subscriptions, nil
}

func (r *subscriptionRepository) GetActiveSubscriptionCount(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM subscriptions 
		WHERE status IN ('active', 'trialing') 
		AND cancel_at_period_end = false
	`

	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}
