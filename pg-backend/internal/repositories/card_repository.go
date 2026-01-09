package repositories

import (
	"context"
	"database/sql"
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
		INSERT INTO cards (user_id, gateway_token, last_four, expiry_month, expiry_year, scheme, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`

	// If this is the first card, set it as default
	if card.IsDefault {
		// Check if user has any cards
		countQuery := `SELECT COUNT(*) FROM cards WHERE user_id = $1`
		var count int
		err := r.db.QueryRowContext(ctx, countQuery, card.UserID).Scan(&count)
		if err != nil {
			return err
		}
		card.IsDefault = count == 0
	}

	err := r.db.QueryRowContext(ctx, query,
		card.UserID,
		card.GatewayToken,
		card.LastFour,
		card.ExpiryMonth,
		card.ExpiryYear,
		card.Scheme,
		card.IsDefault,
	).Scan(&card.ID, &card.CreatedAt)

	return err
}

func (r *cardRepository) GetCardByID(ctx context.Context, id uuid.UUID) (*models.Card, error) {
	query := `
		SELECT id, user_id, gateway_token, last_four, expiry_month, expiry_year, scheme, is_default, created_at
		FROM cards
		WHERE id = $1
	`

	card := &models.Card{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&card.ID,
		&card.UserID,
		&card.GatewayToken,
		&card.LastFour,
		&card.ExpiryMonth,
		&card.ExpiryYear,
		&card.Scheme,
		&card.IsDefault,
		&card.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "card not found"}
	}
	if err != nil {
		return nil, err
	}

	return card, nil
}

func (r *cardRepository) GetCardsByUserID(ctx context.Context, userID uuid.UUID) ([]models.Card, error) {
	query := `
		SELECT id, user_id, gateway_token, last_four, expiry_month, expiry_year, scheme, is_default, created_at
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
		err := rows.Scan(
			&card.ID,
			&card.UserID,
			&card.GatewayToken,
			&card.LastFour,
			&card.ExpiryMonth,
			&card.ExpiryYear,
			&card.Scheme,
			&card.IsDefault,
			&card.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}

	return cards, nil
}

func (r *cardRepository) GetDefaultCardByUserID(ctx context.Context, userID uuid.UUID) (*models.Card, error) {
	query := `
		SELECT id, user_id, gateway_token, last_four, expiry_month, expiry_year, scheme, is_default, created_at
		FROM cards
		WHERE user_id = $1 AND is_default = true
	`

	card := &models.Card{}
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&card.ID,
		&card.UserID,
		&card.GatewayToken,
		&card.LastFour,
		&card.ExpiryMonth,
		&card.ExpiryYear,
		&card.Scheme,
		&card.IsDefault,
		&card.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "no default card found"}
	}
	if err != nil {
		return nil, err
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
