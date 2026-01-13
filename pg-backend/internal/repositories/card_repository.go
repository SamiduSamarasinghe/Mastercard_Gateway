package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"pg-backend/internal/database"
	"pg-backend/internal/models"

	"github.com/google/uuid"
)

type CardRepository interface {
	CreateCard(ctx context.Context, card *models.Card) error
	GetCardByID(ctx context.Context, id uuid.UUID) (*models.Card, error)
	GetCardsByUserID(ctx context.Context, userID uuid.UUID) ([]models.Card, error)
	GetDefaultCardByUserID(ctx context.Context, userID uuid.UUID) (*models.Card, error)
	UpdateCardAsDefault(ctx context.Context, userID, cardID uuid.UUID) error
	DeleteCard(ctx context.Context, id uuid.UUID) error
}

type cardRepository struct {
	db *sql.DB
}

func NewCardRepository() CardRepository {
	return &cardRepository{
		db: database.DB,
	}
}

func (r *cardRepository) CreateCard(ctx context.Context, card *models.Card) error {
	query := `
        INSERT INTO cards (
            user_id, gateway_token, last_four, expiry_month, expiry_year, 
            scheme, is_default, payment_method_type, wallet_provider, 
            device_payment_data, google_pay_token
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
        RETURNING id, created_at
    `

	// Convert device payment data to JSON
	var devicePaymentDataJSON interface{}
	if card.DevicePaymentData != nil {
		jsonData, err := json.Marshal(card.DevicePaymentData)
		if err != nil {
			return err
		}
		devicePaymentDataJSON = string(jsonData)
	} else {
		devicePaymentDataJSON = nil
	}

	// If this is the first card, set it as default
	if card.IsDefault {
		countQuery := `SELECT COUNT(*) FROM cards WHERE user_id = $1`
		var count int
		err := r.db.QueryRowContext(ctx, countQuery, card.UserID).Scan(&count)
		if err != nil {
			return err
		}
		card.IsDefault = count == 0
	}

	// Set default payment method type if not specified
	if card.PaymentMethodType == "" {
		card.PaymentMethodType = "card"
	}

	err := r.db.QueryRowContext(ctx, query,
		card.UserID,
		card.GatewayToken,
		card.LastFour,
		card.ExpiryMonth,
		card.ExpiryYear,
		card.Scheme,
		card.IsDefault,
		card.PaymentMethodType,
		card.WalletProvider,
		devicePaymentDataJSON,
		card.GooglePayToken,
	).Scan(&card.ID, &card.CreatedAt)

	return err
}

func (r *cardRepository) GetCardByID(ctx context.Context, id uuid.UUID) (*models.Card, error) {
	query := `
        SELECT id, user_id, gateway_token, last_four, expiry_month, expiry_year, 
               scheme, is_default, payment_method_type, wallet_provider, 
               device_payment_data, google_pay_token, created_at
        FROM cards
        WHERE id = $1
    `

	card := &models.Card{}
	var devicePaymentDataJSON sql.NullString
	var walletProvider, googlePayToken sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&card.ID,
		&card.UserID,
		&card.GatewayToken,
		&card.LastFour,
		&card.ExpiryMonth,
		&card.ExpiryYear,
		&card.Scheme,
		&card.IsDefault,
		&card.PaymentMethodType,
		&walletProvider,
		&devicePaymentDataJSON,
		&googlePayToken,
		&card.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "card not found"}
	}
	if err != nil {
		return nil, err
	}

	// Parse device payment data
	if devicePaymentDataJSON.Valid && devicePaymentDataJSON.String != "" {
		var deviceData map[string]interface{}
		if err := json.Unmarshal([]byte(devicePaymentDataJSON.String), &deviceData); err == nil {
			card.DevicePaymentData = deviceData
		}
	}

	// Parse nullable strings
	if walletProvider.Valid {
		card.WalletProvider = walletProvider.String
	}
	if googlePayToken.Valid {
		card.GooglePayToken = googlePayToken.String
	}

	// Set default payment method type if not set
	if card.PaymentMethodType == "" {
		card.PaymentMethodType = "card"
	}

	return card, nil
}

func (r *cardRepository) GetCardsByUserID(ctx context.Context, userID uuid.UUID) ([]models.Card, error) {
	query := `
        SELECT id, user_id, gateway_token, last_four, expiry_month, expiry_year, 
               scheme, is_default, payment_method_type, wallet_provider, 
               device_payment_data, google_pay_token, created_at
        FROM cards
        WHERE user_id = $1
        ORDER BY is_default DESC, created_at DESC
    `

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []models.Card
	for rows.Next() {
		var card models.Card
		var devicePaymentDataJSON sql.NullString
		var walletProvider, googlePayToken sql.NullString

		err := rows.Scan(
			&card.ID,
			&card.UserID,
			&card.GatewayToken,
			&card.LastFour,
			&card.ExpiryMonth,
			&card.ExpiryYear,
			&card.Scheme,
			&card.IsDefault,
			&card.PaymentMethodType,
			&walletProvider,
			&devicePaymentDataJSON,
			&googlePayToken,
			&card.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		// Parse device payment data
		if devicePaymentDataJSON.Valid && devicePaymentDataJSON.String != "" {
			var deviceData map[string]interface{}
			if err := json.Unmarshal([]byte(devicePaymentDataJSON.String), &deviceData); err == nil {
				card.DevicePaymentData = deviceData
			}
		}

		// Parse nullable strings
		if walletProvider.Valid {
			card.WalletProvider = walletProvider.String
		}
		if googlePayToken.Valid {
			card.GooglePayToken = googlePayToken.String
		}

		// Set default payment method type if not set
		if card.PaymentMethodType == "" {
			card.PaymentMethodType = "card"
		}

		cards = append(cards, card)
	}

	return cards, nil
}

func (r *cardRepository) GetDefaultCardByUserID(ctx context.Context, userID uuid.UUID) (*models.Card, error) {
	query := `
        SELECT id, user_id, gateway_token, last_four, expiry_month, expiry_year, 
               scheme, is_default, payment_method_type, wallet_provider, 
               device_payment_data, google_pay_token, created_at
        FROM cards
        WHERE user_id = $1 AND is_default = true
    `

	card := &models.Card{}
	var devicePaymentDataJSON sql.NullString
	var walletProvider, googlePayToken sql.NullString

	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&card.ID,
		&card.UserID,
		&card.GatewayToken,
		&card.LastFour,
		&card.ExpiryMonth,
		&card.ExpiryYear,
		&card.Scheme,
		&card.IsDefault,
		&card.PaymentMethodType,
		&walletProvider,
		&devicePaymentDataJSON,
		&googlePayToken,
		&card.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "no default card found"}
	}
	if err != nil {
		return nil, err
	}

	// Parse device payment data
	if devicePaymentDataJSON.Valid && devicePaymentDataJSON.String != "" {
		var deviceData map[string]interface{}
		if err := json.Unmarshal([]byte(devicePaymentDataJSON.String), &deviceData); err == nil {
			card.DevicePaymentData = deviceData
		}
	}

	// Parse nullable strings
	if walletProvider.Valid {
		card.WalletProvider = walletProvider.String
	}
	if googlePayToken.Valid {
		card.GooglePayToken = googlePayToken.String
	}

	// Set default payment method type if not set
	if card.PaymentMethodType == "" {
		card.PaymentMethodType = "card"
	}

	return card, nil
}

func (r *cardRepository) UpdateCardAsDefault(ctx context.Context, userID, cardID uuid.UUID) error {
	// Start transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Reset all cards for this user to non-default
	_, err = tx.ExecContext(ctx,
		"UPDATE cards SET is_default = false WHERE user_id = $1",
		userID)
	if err != nil {
		return err
	}

	// Set the specified card as default
	_, err = tx.ExecContext(ctx,
		"UPDATE cards SET is_default = true WHERE id = $1 AND user_id = $2",
		cardID, userID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *cardRepository) DeleteCard(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM cards WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return &NotFoundError{Message: "card not found"}
	}

	return nil
}
