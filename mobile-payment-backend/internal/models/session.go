package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name,omitempty"`
	LastName  string    `json:"last_name,omitempty"`
	Phone     string    `json:"phone,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Session represents a payment session
type Session struct {
	ID         uuid.UUID `json:"id"`
	GatewayID  string    `json:"gateway_session_id"`
	OrderDBID  uuid.UUID `json:"order_db_id"`
	OrderID    string    `json:"order_id"`
	UserID     uuid.UUID `json:"user_id,omitempty"`
	Amount     float64   `json:"amount"`
	Currency   string    `json:"currency"`
	Status     string    `json:"status"` // created, updated, completed, expired
	APIVersion string    `json:"api_version"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`

	// For 3DS
	AuthenticationParams *AuthenticationParams `json:"authentication_params,omitempty"`
}

type AuthenticationParams struct {
	AcceptVersions string `json:"accept_versions"` // "3DS2"
	Channel        string `json:"channel"`         // "PAYER_APP"
	Purpose        string `json:"purpose"`         // "PAYMENT_TRANSACTION"
}

// PaymentRequest for processing payment
type PaymentRequest struct {
	SessionID string `json:"session_id"`
	OrderID   string `json:"order_id"`
	Operation string `json:"operation"`          // "PAY" or "AUTHORIZE"
	Amount    string `json:"amount,omitempty"`   // Optional override
	Currency  string `json:"currency,omitempty"` // Optional override
}

// PaymentResponse from gateway
type PaymentResponse struct {
	Success         bool                   `json:"success"`
	GatewayCode     string                 `json:"gateway_code"`
	TransactionID   string                 `json:"transaction_id"`
	OrderID         string                 `json:"order_id"`
	Amount          float64                `json:"amount"`
	Currency        string                 `json:"currency"`
	Status          string                 `json:"status"`
	Recommendation  string                 `json:"recommendation,omitempty"`
	GatewayResponse map[string]interface{} `json:"gateway_response,omitempty"`
}

// MobileSDKConfig for frontend initialization
type MobileSDKConfig struct {
	MerchantID   string `json:"merchant_id"`
	MerchantName string `json:"merchant_name"`
	MerchantURL  string `json:"merchant_url"`
	Region       string `json:"region"`
	APIVersion   string `json:"api_version"`
}

// Order for session creation
type Order struct {
	ID          uuid.UUID              `json:"id"`
	UserID      uuid.UUID              `json:"user_id"`
	ReferenceID string                 `json:"reference_id"` // Human-readable: "ORD-001"
	Amount      float64                `json:"amount"`
	Currency    string                 `json:"currency"`
	Description string                 `json:"description,omitempty"`
	Status      string                 `json:"status"` // pending, paid, failed, refunded
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// SessionResponse to mobile app
type SessionResponse struct {
	SessionID  string          `json:"session_id"`
	OrderID    string          `json:"order_id"`
	Amount     string          `json:"amount"`
	Currency   string          `json:"currency"`
	APIVersion string          `json:"api_version"`
	SDKConfig  MobileSDKConfig `json:"sdk_config"`
}

// Add this to your existing models.go file
type PaymentToken struct {
	ID                uuid.UUID `json:"id"`
	UserID            uuid.UUID `json:"user_id"`
	GatewayToken      string    `json:"gateway_token"`
	LastFour          string    `json:"last_four"`
	ExpiryMonth       int       `json:"expiry_month"`
	ExpiryYear        int       `json:"expiry_year"`
	CardScheme        string    `json:"card_scheme,omitempty"`
	PaymentMethodType string    `json:"payment_method_type,omitempty"` // "card", "apple_pay", "google_pay"
	WalletProvider    string    `json:"wallet_provider,omitempty"`     // "APPLE_PAY", "GOOGLE_PAY"
	IsDefault         bool      `json:"is_default"`
	CreatedAt         time.Time `json:"created_at"`
}

// Transaction model for tracking payments
type Transaction struct {
	ID                   uuid.UUID              `json:"id"`
	SessionID            uuid.UUID              `json:"session_id"`
	OrderID              string                 `json:"order_id"`
	UserID               uuid.UUID              `json:"user_id"`
	Amount               float64                `json:"amount"`
	Currency             string                 `json:"currency"`
	GatewayTransactionID string                 `json:"gateway_transaction_id,omitempty"`
	Status               string                 `json:"status"`    // pending, processing, succeeded, failed
	Operation            string                 `json:"operation"` // PAY, AUTHORIZE, REFUND
	GatewayResponse      map[string]interface{} `json:"gateway_response,omitempty"`
	CreatedAt            time.Time              `json:"created_at"`
}
