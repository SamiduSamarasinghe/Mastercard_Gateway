package services

import (
	"context"
	"fmt"
	"pg-backend/internal/models"
	"pg-backend/internal/repositories"

	"github.com/google/uuid"
)

type PlanService interface {
	CreatePlan(ctx context.Context, plan *models.Plan) error
	GetPlan(ctx context.Context, id uuid.UUID) (*models.Plan, error)
	GetPlanByName(ctx context.Context, name string) (*models.Plan, error)
	GetAllPlans(ctx context.Context, activeOnly bool) ([]models.Plan, error)
	UpdatePlan(ctx context.Context, plan *models.Plan) error
	DeletePlan(ctx context.Context, id uuid.UUID) error
	GetPlansByCurrency(ctx context.Context, currency string) ([]models.Plan, error)
}

type planService struct {
	planRepo repositories.PlanRepository
}

func NewPlanService(planRepo repositories.PlanRepository) PlanService {
	return &planService{
		planRepo: planRepo,
	}
}

func (s *planService) CreatePlan(ctx context.Context, plan *models.Plan) error {
	// Validate interval
	if !isValidInterval(plan.Interval) {
		return fmt.Errorf("invalid interval. Must be one of: day, week, month, year")
	}

	// Validate amount
	if plan.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	// Validate trial period
	if plan.TrialPeriodDays < 0 {
		return fmt.Errorf("trial period days cannot be negative")
	}

	// Default currency to LKR if not specified
	if plan.Currency == "" {
		plan.Currency = "LKR"
	}

	// Set default active status
	if !plan.IsActive {
		plan.IsActive = true
	}

	return s.planRepo.CreatePlan(ctx, plan)
}

func (s *planService) GetPlan(ctx context.Context, id uuid.UUID) (*models.Plan, error) {
	return s.planRepo.GetPlanByID(ctx, id)
}

func (s *planService) GetPlanByName(ctx context.Context, name string) (*models.Plan, error) {
	return s.planRepo.GetPlanByName(ctx, name)
}

func (s *planService) GetAllPlans(ctx context.Context, activeOnly bool) ([]models.Plan, error) {
	return s.planRepo.GetAllPlans(ctx, activeOnly)
}

func (s *planService) UpdatePlan(ctx context.Context, plan *models.Plan) error {
	// Validate interval
	if !isValidInterval(plan.Interval) {
		return fmt.Errorf("invalid interval. Must be one of: day, week, month, year")
	}

	// Validate amount
	if plan.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	existingPlan, err := s.planRepo.GetPlanByID(ctx, plan.ID)
	if err != nil {
		return fmt.Errorf("plan not found: %w", err)
	}

	// Don't allow changing currency if there are existing subscriptions
	if existingPlan.Currency != plan.Currency {
		// In production, you would check if there are existing subscriptions
		// For now, we'll allow it but log a warning
		fmt.Printf("Warning: Changing plan currency from %s to %s\n", existingPlan.Currency, plan.Currency)
	}

	return s.planRepo.UpdatePlan(ctx, plan)
}

func (s *planService) DeletePlan(ctx context.Context, id uuid.UUID) error {
	// In production, you would check if there are active subscriptions
	// before deleting. For now, we'll just deactivate.
	plan, err := s.planRepo.GetPlanByID(ctx, id)
	if err != nil {
		return fmt.Errorf("plan not found: %w", err)
	}

	// Instead of deleting, deactivate the plan
	plan.IsActive = false
	return s.planRepo.UpdatePlan(ctx, plan)
}

func (s *planService) GetPlansByCurrency(ctx context.Context, currency string) ([]models.Plan, error) {
	allPlans, err := s.planRepo.GetAllPlans(ctx, true)
	if err != nil {
		return nil, err
	}

	var filteredPlans []models.Plan
	for _, plan := range allPlans {
		if plan.Currency == currency {
			filteredPlans = append(filteredPlans, plan)
		}
	}

	return filteredPlans, nil
}

func isValidInterval(interval string) bool {
	validIntervals := map[string]bool{
		"day":   true,
		"week":  true,
		"month": true,
		"year":  true,
	}
	return validIntervals[interval]
}
