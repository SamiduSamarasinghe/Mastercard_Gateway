package repositories

import (
	"context"
	"database/sql"
	"encoding/json"

	"mobile-payment-backend/internal/models"

	"github.com/google/uuid"
)

type SessionRepository interface {
	Create(ctx context.Context, session *models.Session) error
	GetByGatewayID(ctx context.Context, gatewayID string) (*models.Session, error)
	GetByOrderID(ctx context.Context, orderID string) (*models.Session, error)
	UpdateStatus(ctx context.Context, gatewayID, status string) error
	DeleteExpired(ctx context.Context) (int64, error)
}

type sessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) SessionRepository {
	return &sessionRepository{db: db}
}

func (r *sessionRepository) Create(ctx context.Context, session *models.Session) error {
	query := `
        INSERT INTO sessions (
            id, gateway_session_id, order_id, user_id, amount, currency,
            status, api_version, authentication_params, created_at, expires_at
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
        RETURNING id, created_at
    `

	var authParamsJSON interface{}
	if session.AuthenticationParams != nil {
		jsonData, err := json.Marshal(session.AuthenticationParams)
		if err != nil {
			return err
		}
		authParamsJSON = string(jsonData)
	} else {
		authParamsJSON = nil
	}

	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}

	return r.db.QueryRowContext(ctx, query,
		session.ID,
		session.GatewayID,
		session.OrderID,
		session.UserID,
		session.Amount,
		session.Currency,
		session.Status,
		session.APIVersion,
		authParamsJSON,
		session.CreatedAt,
		session.ExpiresAt,
	).Scan(&session.ID, &session.CreatedAt)
}

func (r *sessionRepository) GetByGatewayID(ctx context.Context, gatewayID string) (*models.Session, error) {
	query := `
        SELECT id, gateway_session_id, order_id, user_id, amount, currency,
               status, api_version, authentication_params, created_at, expires_at
        FROM sessions
        WHERE gateway_session_id = $1
    `

	session := &models.Session{}
	var authParamsJSON sql.NullString
	var userID sql.NullString

	err := r.db.QueryRowContext(ctx, query, gatewayID).Scan(
		&session.ID,
		&session.GatewayID,
		&session.OrderID,
		&userID,
		&session.Amount,
		&session.Currency,
		&session.Status,
		&session.APIVersion,
		&authParamsJSON,
		&session.CreatedAt,
		&session.ExpiresAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "session not found"}
	}
	if err != nil {
		return nil, err
	}

	// Parse user ID
	if userID.Valid && userID.String != "" {
		if uid, err := uuid.Parse(userID.String); err == nil {
			session.UserID = uid
		}
	}

	// Parse authentication params
	if authParamsJSON.Valid && authParamsJSON.String != "" {
		var authParams models.AuthenticationParams
		if err := json.Unmarshal([]byte(authParamsJSON.String), &authParams); err == nil {
			session.AuthenticationParams = &authParams
		}
	}

	return session, nil
}

func (r *sessionRepository) GetByOrderID(ctx context.Context, orderID string) (*models.Session, error) {
	query := `
        SELECT id, gateway_session_id, order_id, user_id, amount, currency,
               status, api_version, authentication_params, created_at, expires_at
        FROM sessions
        WHERE order_id = $1
        ORDER BY created_at DESC
        LIMIT 1
    `

	session := &models.Session{}
	var authParamsJSON sql.NullString
	var userID sql.NullString

	err := r.db.QueryRowContext(ctx, query, orderID).Scan(
		&session.ID,
		&session.GatewayID,
		&session.OrderID,
		&userID,
		&session.Amount,
		&session.Currency,
		&session.Status,
		&session.APIVersion,
		&authParamsJSON,
		&session.CreatedAt,
		&session.ExpiresAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "session not found"}
	}
	if err != nil {
		return nil, err
	}

	if userID.Valid && userID.String != "" {
		if uid, err := uuid.Parse(userID.String); err == nil {
			session.UserID = uid
		}
	}

	if authParamsJSON.Valid && authParamsJSON.String != "" {
		var authParams models.AuthenticationParams
		if err := json.Unmarshal([]byte(authParamsJSON.String), &authParams); err == nil {
			session.AuthenticationParams = &authParams
		}
	}

	return session, nil
}

func (r *sessionRepository) UpdateStatus(ctx context.Context, gatewayID, status string) error {
	query := `
        UPDATE sessions
        SET status = $1, updated_at = NOW()
        WHERE gateway_session_id = $2
    `

	result, err := r.db.ExecContext(ctx, query, status, gatewayID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return &NotFoundError{Message: "session not found"}
	}

	return nil
}

func (r *sessionRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `
        DELETE FROM sessions
        WHERE expires_at < NOW() AND status != 'completed'
    `

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
