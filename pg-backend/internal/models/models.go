package models

import (
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
	CreatedAt    time.Time `json:"created_at"`
}

type Transaction struct {
	ID                   uuid.UUID `json:"id"`
	UserID               uuid.UUID `json:"user_id"`
	CardID               uuid.UUID `json:"card_id"`
	Amount               float64   `json:"amount"`
	Currency             string    `json:"currency"`
	Status               string    `json:"status"`
	GatewayTransactionID string    `json:"gateway_transaction_id"`
	Type                 string    `json:"type"`
	CreatedAt            time.Time `json:"created_at"`
}

type Subscription struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	CardID        uuid.UUID `json:"card_id"`
	PlanName      string    `json:"plan_name"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	Interval      string    `json:"interval"`
	NextBillingAt time.Time `json:"next_billing_at"`
	CreatedAt     time.Time `json:"created_at"`
}
