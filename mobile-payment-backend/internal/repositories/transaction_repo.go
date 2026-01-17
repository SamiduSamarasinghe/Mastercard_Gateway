package repositories

import (
	"context"
	"database/sql"
	"encoding/json"

	"mobile-payment-backend/internal/models"

	"github.com/google/uuid"
)

type TransactionRepository interface {
	Create(ctx context.Context, transaction *models.Transaction) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Transaction, error)
	GetByOrderID(ctx context.Context, orderID string) ([]models.Transaction, error)
	GetBySessionID(ctx context.Context, sessionID uuid.UUID) ([]models.Transaction, error)
}

type transactionRepository struct {
	db *sql.DB
}

func NewTransactionRepository(db *sql.DB) TransactionRepository {
	return &transactionRepository{db: db}
}

func (r *transactionRepository) Create(ctx context.Context, transaction *models.Transaction) error {
	query := `
        INSERT INTO transactions (
            id, session_id, order_id, user_id, amount, currency,
            gateway_transaction_id, status, operation, gateway_response
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
        RETURNING created_at
    `

	var gatewayResponseJSON interface{}
	if transaction.GatewayResponse != nil {
		jsonData, err := json.Marshal(transaction.GatewayResponse)
		if err != nil {
			return err
		}
		gatewayResponseJSON = string(jsonData)
	} else {
		gatewayResponseJSON = nil
	}

	if transaction.ID == uuid.Nil {
		transaction.ID = uuid.New()
	}

	return r.db.QueryRowContext(ctx, query,
		transaction.ID,
		transaction.SessionID,
		transaction.OrderID,
		transaction.UserID,
		transaction.Amount,
		transaction.Currency,
		transaction.GatewayTransactionID,
		transaction.Status,
		transaction.Operation,
		gatewayResponseJSON,
	).Scan(&transaction.CreatedAt)
}

func (r *transactionRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Transaction, error) {
	query := `
        SELECT id, session_id, order_id, user_id, amount, currency,
               gateway_transaction_id, status, operation, gateway_response, created_at
        FROM transactions
        WHERE id = $1
    `

	transaction := &models.Transaction{}
	var gatewayResponseJSON sql.NullString
	var userID sql.NullString
	var sessionID sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&transaction.ID,
		&sessionID,
		&transaction.OrderID,
		&userID,
		&transaction.Amount,
		&transaction.Currency,
		&transaction.GatewayTransactionID,
		&transaction.Status,
		&transaction.Operation,
		&gatewayResponseJSON,
		&transaction.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "transaction not found"}
	}
	if err != nil {
		return nil, err
	}

	// Parse IDs
	if sessionID.Valid && sessionID.String != "" {
		if sid, err := uuid.Parse(sessionID.String); err == nil {
			transaction.SessionID = sid
		}
	}

	if userID.Valid && userID.String != "" {
		if uid, err := uuid.Parse(userID.String); err == nil {
			transaction.UserID = uid
		}
	}

	// Parse gateway response
	if gatewayResponseJSON.Valid && gatewayResponseJSON.String != "" {
		var response map[string]interface{}
		if err := json.Unmarshal([]byte(gatewayResponseJSON.String), &response); err == nil {
			transaction.GatewayResponse = response
		}
	}

	return transaction, nil
}

func (r *transactionRepository) GetByOrderID(ctx context.Context, orderID string) ([]models.Transaction, error) {
	query := `
        SELECT id, session_id, order_id, user_id, amount, currency,
               gateway_transaction_id, status, operation, gateway_response, created_at
        FROM transactions
        WHERE order_id = $1
        ORDER BY created_at DESC
    `

	rows, err := r.db.QueryContext(ctx, query, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []models.Transaction
	for rows.Next() {
		var transaction models.Transaction
		var gatewayResponseJSON sql.NullString
		var userID sql.NullString
		var sessionID sql.NullString

		err := rows.Scan(
			&transaction.ID,
			&sessionID,
			&transaction.OrderID,
			&userID,
			&transaction.Amount,
			&transaction.Currency,
			&transaction.GatewayTransactionID,
			&transaction.Status,
			&transaction.Operation,
			&gatewayResponseJSON,
			&transaction.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if sessionID.Valid && sessionID.String != "" {
			if sid, err := uuid.Parse(sessionID.String); err == nil {
				transaction.SessionID = sid
			}
		}

		if userID.Valid && userID.String != "" {
			if uid, err := uuid.Parse(userID.String); err == nil {
				transaction.UserID = uid
			}
		}

		if gatewayResponseJSON.Valid && gatewayResponseJSON.String != "" {
			var response map[string]interface{}
			if err := json.Unmarshal([]byte(gatewayResponseJSON.String), &response); err == nil {
				transaction.GatewayResponse = response
			}
		}

		transactions = append(transactions, transaction)
	}

	return transactions, nil
}

func (r *transactionRepository) GetBySessionID(ctx context.Context, sessionID uuid.UUID) ([]models.Transaction, error) {
	query := `
        SELECT id, session_id, order_id, user_id, amount, currency,
               gateway_transaction_id, status, operation, gateway_response, created_at
        FROM transactions
        WHERE session_id = $1
        ORDER BY created_at DESC
    `

	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []models.Transaction
	for rows.Next() {
		var transaction models.Transaction
		var gatewayResponseJSON sql.NullString
		var userID sql.NullString
		var dbSessionID sql.NullString

		err := rows.Scan(
			&transaction.ID,
			&dbSessionID,
			&transaction.OrderID,
			&userID,
			&transaction.Amount,
			&transaction.Currency,
			&transaction.GatewayTransactionID,
			&transaction.Status,
			&transaction.Operation,
			&gatewayResponseJSON,
			&transaction.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		transaction.SessionID = sessionID

		if userID.Valid && userID.String != "" {
			if uid, err := uuid.Parse(userID.String); err == nil {
				transaction.UserID = uid
			}
		}

		if gatewayResponseJSON.Valid && gatewayResponseJSON.String != "" {
			var response map[string]interface{}
			if err := json.Unmarshal([]byte(gatewayResponseJSON.String), &response); err == nil {
				transaction.GatewayResponse = response
			}
		}

		transactions = append(transactions, transaction)
	}

	return transactions, nil
}
