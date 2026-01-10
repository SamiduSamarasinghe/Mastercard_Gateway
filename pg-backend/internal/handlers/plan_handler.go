package handlers

import (
	"net/http"

	"pg-backend/internal/models"
	"pg-backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type PlanHandler struct {
	planService services.PlanService
}

func NewPlanHandler(planService services.PlanService) *PlanHandler {
	return &PlanHandler{
		planService: planService,
	}
}

// CreatePlanRequest represents plan creation request
type CreatePlanRequest struct {
	Name            string  `json:"name" binding:"required"`
	Amount          float64 `json:"amount" binding:"required,gt=0"`
	Currency        string  `json:"currency" binding:"required,iso4217"`
	Interval        string  `json:"interval" binding:"required,oneof=day week month year"`
	TrialPeriodDays int     `json:"trial_period_days" binding:"gte=0"`
	Description     string  `json:"description"`
	IsActive        bool    `json:"is_active"`
}

// CreatePlan creates a new subscription plan
func (h *PlanHandler) CreatePlan(c *gin.Context) {
	var req CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Default currency to LKR if not specified or invalid
	if req.Currency == "" {
		req.Currency = "LKR"
	}

	plan := &models.Plan{
		Name:            req.Name,
		Amount:          req.Amount,
		Currency:        req.Currency,
		Interval:        req.Interval,
		TrialPeriodDays: req.TrialPeriodDays,
		Description:     req.Description,
		IsActive:        req.IsActive,
	}

	if err := h.planService.CreatePlan(c.Request.Context(), plan); err != nil {
		status := http.StatusInternalServerError
		if _, ok := err.(*services.DuplicateError); ok {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, plan)
}

// GetPlan gets a plan by ID
func (h *PlanHandler) GetPlan(c *gin.Context) {
	planID := c.Param("id")

	id, err := uuid.Parse(planID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan ID"})
		return
	}

	plan, err := h.planService.GetPlan(c.Request.Context(), id)
	if err != nil {
		if _, ok := err.(*services.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "plan not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plan)
}

// GetPlans gets all plans (with optional active filter)
func (h *PlanHandler) GetPlans(c *gin.Context) {
	activeOnly := c.DefaultQuery("active", "true") == "true"

	plans, err := h.planService.GetAllPlans(c.Request.Context(), activeOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plans)
}

// UpdatePlanRequest represents plan update request
type UpdatePlanRequest struct {
	Name            string  `json:"name" binding:"required"`
	Amount          float64 `json:"amount" binding:"required,gt=0"`
	Currency        string  `json:"currency" binding:"required,iso4217"`
	Interval        string  `json:"interval" binding:"required,oneof=day week month year"`
	TrialPeriodDays int     `json:"trial_period_days" binding:"gte=0"`
	Description     string  `json:"description"`
	IsActive        bool    `json:"is_active"`
}

// UpdatePlan updates a plan
func (h *PlanHandler) UpdatePlan(c *gin.Context) {
	planID := c.Param("id")

	id, err := uuid.Parse(planID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan ID"})
		return
	}

	var req UpdatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	plan := &models.Plan{
		ID:              id,
		Name:            req.Name,
		Amount:          req.Amount,
		Currency:        req.Currency,
		Interval:        req.Interval,
		TrialPeriodDays: req.TrialPeriodDays,
		Description:     req.Description,
		IsActive:        req.IsActive,
	}

	if err := h.planService.UpdatePlan(c.Request.Context(), plan); err != nil {
		if _, ok := err.(*services.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "plan not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plan)
}

// DeletePlan deletes (deactivates) a plan
func (h *PlanHandler) DeletePlan(c *gin.Context) {
	planID := c.Param("id")

	id, err := uuid.Parse(planID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan ID"})
		return
	}

	if err := h.planService.DeletePlan(c.Request.Context(), id); err != nil {
		if _, ok := err.(*services.NotFoundError); ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "plan not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Plan deactivated successfully",
	})
}

// GetPlansByCurrency gets plans by currency
func (h *PlanHandler) GetPlansByCurrency(c *gin.Context) {
	currency := c.Param("currency")
	if currency == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "currency parameter required"})
		return
	}

	plans, err := h.planService.GetPlansByCurrency(c.Request.Context(), currency)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plans)
}
