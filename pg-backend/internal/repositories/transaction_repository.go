package repositories

import (
	"context"
	"database/sql"
	"pg-backend/internal/database"
	"pg-backend/internal/models"

	"github.com/google/uuid"
)

type TransactionRepository interface {
	CreateTransaction(ctx context.Context, transaction *models.Transaction) error
	GetTransactionByID(ctx context.Context, id uuid.UUID) (*models.Transaction, error)
	GetTransactionsByUserID(ctx context.Context, userID uuid.UUID) ([]models.Transaction, error)
	GetTransactionsByCardID(ctx context.Context, cardID uuid.UUID) ([]models.Transaction, error)

	//NEW
	GetTransactionsBySubscriptionID(ctx context.Context, subscriptionID uuid.UUID) ([]models.Transaction, error)
	GetTransactionsByBillingAttemptID(ctx context.Context, billingAttemptID uuid.UUID) ([]models.Transaction, error)
	CreateSubscriptionTransaction(ctx context.Context, transaction *models.Transaction, subscriptionID, billingAttemptID uuid.UUID) error
}

type transactionRepository struct {
	db *sql.DB
}

func NewTransactionRepository() TransactionRepository {
	return &transactionRepository{
		db: database.DB,
	}
}

func (r *transactionRepository) CreateTransaction(ctx context.Context, transaction *models.Transaction) error {
	query := `
		INSERT INTO transactions 
		(user_id, card_id, amount, currency, status, gateway_transaction_id, type)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`

	err := r.db.QueryRowContext(ctx, query,
		transaction.UserID,
		transaction.CardID,
		transaction.Amount,
		transaction.Currency,
		transaction.Status,
		transaction.GatewayTransactionID,
		transaction.Type,
	).Scan(&transaction.ID, &transaction.CreatedAt)

	return err
}

func (r *transactionRepository) GetTransactionByID(ctx context.Context, id uuid.UUID) (*models.Transaction, error) {
	query := `
		SELECT id, user_id, card_id, amount, currency, status, 
		       gateway_transaction_id, type, created_at
		FROM transactions
		WHERE id = $1
	`

	transaction := &models.Transaction{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&transaction.ID,
		&transaction.UserID,
		&transaction.CardID,
		&transaction.Amount,
		&transaction.Currency,
		&transaction.Status,
		&transaction.GatewayTransactionID,
		&transaction.Type,
		&transaction.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "transaction not found"}
	}
	if err != nil {
		return nil, err
	}

	return transaction, nil
}

func (r *transactionRepository) GetTransactionsByUserID(ctx context.Context, userID uuid.UUID) ([]models.Transaction, error) {
	query := `
		SELECT id, user_id, card_id, amount, currency, status, 
		       gateway_transaction_id, type, created_at
		FROM transactions
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []models.Transaction
	for rows.Next() {
		var transaction models.Transaction
		err := rows.Scan(
			&transaction.ID,
			&transaction.UserID,
			&transaction.CardID,
			&transaction.Amount,
			&transaction.Currency,
			&transaction.Status,
			&transaction.GatewayTransactionID,
			&transaction.Type,
			&transaction.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}

	return transactions, nil
}

func (r *transactionRepository) GetTransactionsByCardID(ctx context.Context, cardID uuid.UUID) ([]models.Transaction, error) {
	query := `
		SELECT id, user_id, card_id, amount, currency, status, 
		       gateway_transaction_id, type, created_at
		FROM transactions
		WHERE card_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []models.Transaction
	for rows.Next() {
		var transaction models.Transaction
		err := rows.Scan(
			&transaction.ID,
			&transaction.UserID,
			&transaction.CardID,
			&transaction.Amount,
			&transaction.Currency,
			&transaction.Status,
			&transaction.GatewayTransactionID,
			&transaction.Type,
			&transaction.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}

	return transactions, nil
}

func (r *transactionRepository) GetTransactionsBySubscriptionID(ctx context.Context, subscriptionID uuid.UUID) ([]models.Transaction, error) {
	query := `
		SELECT 
			id, user_id, card_id, subscription_id, billing_attempt_id, invoice_id,
			amount, currency, status, gateway_transaction_id, type, created_at
		FROM transactions
		WHERE subscription_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []models.Transaction
	for rows.Next() {
		var transaction models.Transaction
		err := rows.Scan(
			&transaction.ID,
			&transaction.UserID,
			&transaction.CardID,
			&transaction.SubscriptionID,
			&transaction.BillingAttemptID,
			&transaction.InvoiceID,
			&transaction.Amount,
			&transaction.Currency,
			&transaction.Status,
			&transaction.GatewayTransactionID,
			&transaction.Type,
			&transaction.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}

	return transactions, nil
}

func (r *transactionRepository) GetTransactionsByBillingAttemptID(ctx context.Context, billingAttemptID uuid.UUID) ([]models.Transaction, error) {
	query := `
		SELECT 
			id, user_id, card_id, subscription_id, billing_attempt_id, invoice_id,
			amount, currency, status, gateway_transaction_id, type, created_at
		FROM transactions
		WHERE billing_attempt_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, billingAttemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []models.Transaction
	for rows.Next() {
		var transaction models.Transaction
		err := rows.Scan(
			&transaction.ID,
			&transaction.UserID,
			&transaction.CardID,
			&transaction.SubscriptionID,
			&transaction.BillingAttemptID,
			&transaction.InvoiceID,
			&transaction.Amount,
			&transaction.Currency,
			&transaction.Status,
			&transaction.GatewayTransactionID,
			&transaction.Type,
			&transaction.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}

	return transactions, nil
}

func (r *transactionRepository) CreateSubscriptionTransaction(ctx context.Context, transaction *models.Transaction, subscriptionID, billingAttemptID uuid.UUID) error {
	query := `
		INSERT INTO transactions 
		(user_id, card_id, subscription_id, billing_attempt_id, invoice_id,
		 amount, currency, status, gateway_transaction_id, type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`

	err := r.db.QueryRowContext(ctx, query,
		transaction.UserID,
		transaction.CardID,
		subscriptionID,
		billingAttemptID,
		transaction.InvoiceID,
		transaction.Amount,
		transaction.Currency,
		transaction.Status,
		transaction.GatewayTransactionID,
		transaction.Type,
	).Scan(&transaction.ID, &transaction.CreatedAt)

	return err
}
