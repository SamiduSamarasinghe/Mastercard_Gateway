package repositories

import (
	"context"
	"database/sql"
	"pg-backend/internal/database"
	"pg-backend/internal/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type PlanRepository interface {
	CreatePlan(ctx context.Context, plan *models.Plan) error
	GetPlanByID(ctx context.Context, id uuid.UUID) (*models.Plan, error)
	GetPlanByName(ctx context.Context, name string) (*models.Plan, error)
	GetAllPlans(ctx context.Context, activeOnly bool) ([]models.Plan, error)
	UpdatePlan(ctx context.Context, plan *models.Plan) error
	DeletePlan(ctx context.Context, id uuid.UUID) error
}

type planRepository struct {
	db *sql.DB
}

func NewPlanRepository() PlanRepository {
	return &planRepository{
		db: database.DB,
	}
}

func (r *planRepository) CreatePlan(ctx context.Context, plan *models.Plan) error {
	query := `
		INSERT INTO plans (name, amount, currency, interval, trial_period_days, description, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		plan.Name,
		plan.Amount,
		plan.Currency,
		plan.Interval,
		plan.TrialPeriodDays,
		plan.Description,
		plan.IsActive,
	).Scan(&plan.ID, &plan.CreatedAt, &plan.UpdatedAt)

	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return &DuplicateError{Message: "plan with this name already exists"}
		}
		return err
	}

	return nil
}

func (r *planRepository) GetPlanByID(ctx context.Context, id uuid.UUID) (*models.Plan, error) {
	query := `
		SELECT id, name, amount, currency, interval, trial_period_days, 
		       description, is_active, created_at, updated_at
		FROM plans
		WHERE id = $1
	`

	plan := &models.Plan{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&plan.ID,
		&plan.Name,
		&plan.Amount,
		&plan.Currency,
		&plan.Interval,
		&plan.TrialPeriodDays,
		&plan.Description,
		&plan.IsActive,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "plan not found"}
	}
	if err != nil {
		return nil, err
	}

	return plan, nil
}

func (r *planRepository) GetPlanByName(ctx context.Context, name string) (*models.Plan, error) {
	query := `
		SELECT id, name, amount, currency, interval, trial_period_days, 
		       description, is_active, created_at, updated_at
		FROM plans
		WHERE name = $1
	`

	plan := &models.Plan{}
	err := r.db.QueryRowContext(ctx, query, name).Scan(
		&plan.ID,
		&plan.Name,
		&plan.Amount,
		&plan.Currency,
		&plan.Interval,
		&plan.TrialPeriodDays,
		&plan.Description,
		&plan.IsActive,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "plan not found"}
	}
	if err != nil {
		return nil, err
	}

	return plan, nil
}

func (r *planRepository) GetAllPlans(ctx context.Context, activeOnly bool) ([]models.Plan, error) {
	var query string
	var args []interface{}

	if activeOnly {
		query = `
			SELECT id, name, amount, currency, interval, trial_period_days, 
			       description, is_active, created_at, updated_at
			FROM plans
			WHERE is_active = true
			ORDER BY amount ASC, name ASC
		`
	} else {
		query = `
			SELECT id, name, amount, currency, interval, trial_period_days, 
			       description, is_active, created_at, updated_at
			FROM plans
			ORDER BY is_active DESC, amount ASC, name ASC
		`
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []models.Plan
	for rows.Next() {
		var plan models.Plan
		err := rows.Scan(
			&plan.ID,
			&plan.Name,
			&plan.Amount,
			&plan.Currency,
			&plan.Interval,
			&plan.TrialPeriodDays,
			&plan.Description,
			&plan.IsActive,
			&plan.CreatedAt,
			&plan.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}

	return plans, nil
}

func (r *planRepository) UpdatePlan(ctx context.Context, plan *models.Plan) error {
	query := `
		UPDATE plans
		SET name = $1, amount = $2, currency = $3, interval = $4, 
		    trial_period_days = $5, description = $6, is_active = $7
		WHERE id = $8
		RETURNING updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		plan.Name,
		plan.Amount,
		plan.Currency,
		plan.Interval,
		plan.TrialPeriodDays,
		plan.Description,
		plan.IsActive,
		plan.ID,
	).Scan(&plan.UpdatedAt)

	if err == sql.ErrNoRows {
		return &NotFoundError{Message: "plan not found"}
	}
	if err != nil {
		return err
	}

	return nil
}

func (r *planRepository) DeletePlan(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM plans WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return &NotFoundError{Message: "plan not found"}
	}

	return nil
}
