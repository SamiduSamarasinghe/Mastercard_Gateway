package repositories

import (
	"context"
	"database/sql"

	"mobile-payment-backend/internal/models"

	"github.com/google/uuid"
)

type TokenRepository interface {
	Create(ctx context.Context, token *models.PaymentToken) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.PaymentToken, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.PaymentToken, error)
	GetByGatewayToken(ctx context.Context, gatewayToken string) (*models.PaymentToken, error)
	SetDefault(ctx context.Context, userID, tokenID uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type tokenRepository struct {
	db *sql.DB
}

func NewTokenRepository(db *sql.DB) TokenRepository {
	return &tokenRepository{db: db}
}

func (r *tokenRepository) Create(ctx context.Context, token *models.PaymentToken) error {
	query := `
        INSERT INTO payment_tokens (
            id, user_id, gateway_token, last_four, expiry_month, expiry_year,
            card_scheme, payment_method_type, wallet_provider, is_default
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
        RETURNING created_at
    `

	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}

	// If this is the first token for user, set as default
	if token.IsDefault {
		countQuery := `SELECT COUNT(*) FROM payment_tokens WHERE user_id = $1`
		var count int
		err := r.db.QueryRowContext(ctx, countQuery, token.UserID).Scan(&count)
		if err != nil {
			return err
		}
		token.IsDefault = count == 0
	}

	return r.db.QueryRowContext(ctx, query,
		token.ID,
		token.UserID,
		token.GatewayToken,
		token.LastFour,
		token.ExpiryMonth,
		token.ExpiryYear,
		token.CardScheme,
		token.PaymentMethodType,
		token.WalletProvider,
		token.IsDefault,
	).Scan(&token.CreatedAt)
}

func (r *tokenRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.PaymentToken, error) {
	query := `
        SELECT id, user_id, gateway_token, last_four, expiry_month, expiry_year,
               card_scheme, payment_method_type, wallet_provider, is_default, created_at
        FROM payment_tokens
        WHERE id = $1
    `

	token := &models.PaymentToken{}
	var walletProvider, paymentMethodType sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&token.ID,
		&token.UserID,
		&token.GatewayToken,
		&token.LastFour,
		&token.ExpiryMonth,
		&token.ExpiryYear,
		&token.CardScheme,
		&paymentMethodType,
		&walletProvider,
		&token.IsDefault,
		&token.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "token not found"}
	}
	if err != nil {
		return nil, err
	}

	if walletProvider.Valid {
		token.WalletProvider = walletProvider.String
	}
	if paymentMethodType.Valid {
		token.PaymentMethodType = paymentMethodType.String
	}

	return token, nil
}

func (r *tokenRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.PaymentToken, error) {
	query := `
        SELECT id, user_id, gateway_token, last_four, expiry_month, expiry_year,
               card_scheme, payment_method_type, wallet_provider, is_default, created_at
        FROM payment_tokens
        WHERE user_id = $1
        ORDER BY is_default DESC, created_at DESC
    `

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []models.PaymentToken
	for rows.Next() {
		var token models.PaymentToken
		var walletProvider, paymentMethodType sql.NullString

		err := rows.Scan(
			&token.ID,
			&token.UserID,
			&token.GatewayToken,
			&token.LastFour,
			&token.ExpiryMonth,
			&token.ExpiryYear,
			&token.CardScheme,
			&paymentMethodType,
			&walletProvider,
			&token.IsDefault,
			&token.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if walletProvider.Valid {
			token.WalletProvider = walletProvider.String
		}
		if paymentMethodType.Valid {
			token.PaymentMethodType = paymentMethodType.String
		}

		tokens = append(tokens, token)
	}

	return tokens, nil
}

func (r *tokenRepository) GetByGatewayToken(ctx context.Context, gatewayToken string) (*models.PaymentToken, error) {
	query := `
        SELECT id, user_id, gateway_token, last_four, expiry_month, expiry_year,
               card_scheme, payment_method_type, wallet_provider, is_default, created_at
        FROM payment_tokens
        WHERE gateway_token = $1
    `

	token := &models.PaymentToken{}
	var walletProvider, paymentMethodType sql.NullString

	err := r.db.QueryRowContext(ctx, query, gatewayToken).Scan(
		&token.ID,
		&token.UserID,
		&token.GatewayToken,
		&token.LastFour,
		&token.ExpiryMonth,
		&token.ExpiryYear,
		&token.CardScheme,
		&paymentMethodType,
		&walletProvider,
		&token.IsDefault,
		&token.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "token not found"}
	}
	if err != nil {
		return nil, err
	}

	if walletProvider.Valid {
		token.WalletProvider = walletProvider.String
	}
	if paymentMethodType.Valid {
		token.PaymentMethodType = paymentMethodType.String
	}

	return token, nil
}

func (r *tokenRepository) SetDefault(ctx context.Context, userID, tokenID uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Reset all tokens for this user to non-default
	_, err = tx.ExecContext(ctx,
		"UPDATE payment_tokens SET is_default = false WHERE user_id = $1",
		userID)
	if err != nil {
		return err
	}

	// Set the specified token as default
	_, err = tx.ExecContext(ctx,
		"UPDATE payment_tokens SET is_default = true WHERE id = $1 AND user_id = $2",
		tokenID, userID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *tokenRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM payment_tokens WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return &NotFoundError{Message: "token not found"}
	}

	return nil
}
