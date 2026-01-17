package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type Card struct {
	ID           uuid.UUID `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	GatewayToken string    `json:"gateway_token"`
	LastFour     string    `json:"last_four"`
	ExpiryMonth  int       `json:"expiry_month"`
	ExpiryYear   int       `json:"expiry_year"`
	Scheme       string    `json:"scheme"`
	IsDefault    bool      `json:"is_default"`

	// NEW FIELDS for Google Pay:
	PaymentMethodType string                 `json:"payment_method_type"`       // "card", "google_pay"
	WalletProvider    string                 `json:"wallet_provider,omitempty"` // "GOOGLE_PAY"
	DevicePaymentData map[string]interface{} `json:"device_payment_data,omitempty"`
	GooglePayToken    string                 `json:"google_pay_token,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

type Transaction struct {
	ID                   uuid.UUID      `json:"id"`
	UserID               uuid.UUID      `json:"user_id"`
	CardID               uuid.UUID      `json:"card_id"`
	SubscriptionID       uuid.NullUUID  `json:"subscription_id,omitempty"`
	BillingAttemptID     uuid.NullUUID  `json:"billing_attempt_id,omitempty"`
	InvoiceID            sql.NullString `json:"invoice_id,omitempty"`
	Amount               float64        `json:"amount"`
	Currency             string         `json:"currency"`
	Status               string         `json:"status"`
	GatewayTransactionID string         `json:"gateway_transaction_id"`
	Type                 string         `json:"type"` // "manual", "recurring", "authorization", "capture", "void", "refund"

	// NEW FIELDS for Google Pay:
	WalletProvider    string                 `json:"wallet_provider,omitempty"`     // "GOOGLE_PAY"
	PaymentMethodType string                 `json:"payment_method_type,omitempty"` // "card", "google_pay"
	DevicePaymentData map[string]interface{} `json:"device_payment_data,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

type Plan struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Amount          float64   `json:"amount"`
	Currency        string    `json:"currency"`
	Interval        string    `json:"interval"` // "day", "week", "month", "year"
	TrialPeriodDays int       `json:"trial_period_days"`
	Description     string    `json:"description"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type SubscriptionStatus string

const (
	SubscriptionStatusActive            SubscriptionStatus = "active"
	SubscriptionStatusPastDue           SubscriptionStatus = "past_due"
	SubscriptionStatusCanceled          SubscriptionStatus = "canceled"
	SubscriptionStatusIncomplete        SubscriptionStatus = "incomplete"
	SubscriptionStatusIncompleteExpired SubscriptionStatus = "incomplete_expired"
	SubscriptionStatusTrialing          SubscriptionStatus = "trialing"
	SubscriptionStatusUnpaid            SubscriptionStatus = "unpaid"
)

// SubscriptionInterval type for type safety
type SubscriptionInterval string

const (
	IntervalDay   SubscriptionInterval = "day"
	IntervalWeek  SubscriptionInterval = "week"
	IntervalMonth SubscriptionInterval = "month"
	IntervalYear  SubscriptionInterval = "year"
)

type Subscription struct {
	ID                 uuid.UUID            `json:"id"`
	UserID             uuid.UUID            `json:"user_id"`
	PlanID             uuid.NullUUID        `json:"plan_id,omitempty"`
	CardID             uuid.NullUUID        `json:"card_id,omitempty"`
	PlanName           string               `json:"plan_name"`
	Amount             float64              `json:"amount"`
	Currency           string               `json:"currency"`
	Status             SubscriptionStatus   `json:"status"`
	Interval           SubscriptionInterval `json:"interval"`
	CurrentPeriodStart sql.NullTime         `json:"current_period_start,omitempty"`
	CurrentPeriodEnd   sql.NullTime         `json:"current_period_end,omitempty"`
	TrialStart         sql.NullTime         `json:"trial_start,omitempty"`
	TrialEnd           sql.NullTime         `json:"trial_end,omitempty"`
	CancelAtPeriodEnd  bool                 `json:"cancel_at_period_end"`
	CanceledAt         sql.NullTime         `json:"canceled_at,omitempty"`
	Metadata           map[string]string    `json:"metadata,omitempty"`
	BillingCycleAnchor sql.NullTime         `json:"billing_cycle_anchor,omitempty"`
	NextBillingAt      time.Time            `json:"next_billing_at"`
	CreatedAt          time.Time            `json:"created_at"`
}

// BillingAttemptStatus type for type safety
type BillingAttemptStatus string

const (
	BillingAttemptStatusPending        BillingAttemptStatus = "pending"
	BillingAttemptStatusProcessing     BillingAttemptStatus = "processing"
	BillingAttemptStatusSucceeded      BillingAttemptStatus = "succeeded"
	BillingAttemptStatusFailed         BillingAttemptStatus = "failed"
	BillingAttemptStatusRequiresAction BillingAttemptStatus = "requires_action"
)

// BillingAttempt model (NEW)
type BillingAttempt struct {
	ID                   uuid.UUID            `json:"id"`
	SubscriptionID       uuid.UUID            `json:"subscription_id"`
	Amount               float64              `json:"amount"`
	Currency             string               `json:"currency"`
	Status               BillingAttemptStatus `json:"status"`
	GatewayTransactionID sql.NullString       `json:"gateway_transaction_id,omitempty"`
	ErrorCode            sql.NullString       `json:"error_code,omitempty"`
	ErrorMessage         sql.NullString       `json:"error_message,omitempty"`
	AttemptNumber        int                  `json:"attempt_number"`
	ScheduledAt          time.Time            `json:"scheduled_at"`
	ProcessedAt          sql.NullTime         `json:"processed_at,omitempty"`
	CreatedAt            time.Time            `json:"created_at"`
}

type GooglePayToken struct {
	ID             uuid.UUID `json:"id"`
	UserID         uuid.UUID `json:"user_id"`
	GatewayToken   string    `json:"gateway_token"`
	GooglePayToken string    `json:"google_pay_token"` // Original encrypted token
	LastFour       string    `json:"last_four"`
	ExpiryMonth    int       `json:"expiry_month"`
	ExpiryYear     int       `json:"expiry_year"`
	Scheme         string    `json:"scheme,omitempty"`
	IsDefault      bool      `json:"is_default"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Add to PaymentMethodType constants
const (
	PaymentMethodTypeCard      = "card"
	PaymentMethodTypeGooglePay = "google_pay"
	PaymentMethodTypeApplePay  = "apple_pay"
)

// Add to WalletProvider constants
const (
	WalletProviderGooglePay = "GOOGLE_PAY"
	WalletProviderApplePay  = "APPLE_PAY" 
)
