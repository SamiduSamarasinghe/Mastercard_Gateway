package repositories

import (
    "context"
    "database/sql"
    "encoding/json"
    // "time"
    
    "mobile-payment-backend/internal/models"
    "github.com/google/uuid"
)

type OrderRepository interface {
    Create(ctx context.Context, order *models.Order) error
    GetByID(ctx context.Context, id uuid.UUID) (*models.Order, error)
    GetByReferenceID(ctx context.Context, referenceID string) (*models.Order, error)
    GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.Order, error)
    UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}

type orderRepository struct {
    db *sql.DB
}

func NewOrderRepository(db *sql.DB) OrderRepository {
    return &orderRepository{db: db}
}

func (r *orderRepository) Create(ctx context.Context, order *models.Order) error {
    query := `
        INSERT INTO orders (
            id, user_id, reference_id, amount, currency, 
            description, status, metadata
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        RETURNING created_at, updated_at
    `
    
    if order.ID == uuid.Nil {
        order.ID = uuid.New()
    }
    
    var metadataJSON interface{}
    if order.Metadata != nil {
        jsonData, err := json.Marshal(order.Metadata)
        if err != nil {
            return err
        }
        metadataJSON = string(jsonData)
    } else {
        metadataJSON = nil
    }
    
    return r.db.QueryRowContext(ctx, query,
        order.ID,
        order.UserID,
        order.ReferenceID,
        order.Amount,
        order.Currency,
        order.Description,
        order.Status,
        metadataJSON,
    ).Scan(&order.CreatedAt, &order.UpdatedAt)
}

func (r *orderRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Order, error) {
    query := `
        SELECT id, user_id, reference_id, amount, currency, 
               description, status, metadata, created_at, updated_at
        FROM orders
        WHERE id = $1
    `
    
    order := &models.Order{}
    var metadataJSON sql.NullString
    
    err := r.db.QueryRowContext(ctx, query, id).Scan(
        &order.ID,
        &order.UserID,
        &order.ReferenceID,
        &order.Amount,
        &order.Currency,
        &order.Description,
        &order.Status,
        &metadataJSON,
        &order.CreatedAt,
        &order.UpdatedAt,
    )
    
    if err == sql.ErrNoRows {
        return nil, &NotFoundError{Message: "order not found"}
    }
    if err != nil {
        return nil, err
    }
    
    if metadataJSON.Valid && metadataJSON.String != "" {
        var metadata map[string]interface{}
        if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
            order.Metadata = metadata
        }
    }
    
    return order, nil
}

func (r *orderRepository) GetByReferenceID(ctx context.Context, referenceID string) (*models.Order, error) {
    query := `
        SELECT id, user_id, reference_id, amount, currency, 
               description, status, metadata, created_at, updated_at
        FROM orders
        WHERE reference_id = $1
    `
    
    order := &models.Order{}
    var metadataJSON sql.NullString
    
    err := r.db.QueryRowContext(ctx, query, referenceID).Scan(
        &order.ID,
        &order.UserID,
        &order.ReferenceID,
        &order.Amount,
        &order.Currency,
        &order.Description,
        &order.Status,
        &metadataJSON,
        &order.CreatedAt,
        &order.UpdatedAt,
    )
    
    if err == sql.ErrNoRows {
        return nil, &NotFoundError{Message: "order not found"}
    }
    if err != nil {
        return nil, err
    }
    
    if metadataJSON.Valid && metadataJSON.String != "" {
        var metadata map[string]interface{}
        if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
            order.Metadata = metadata
        }
    }
    
    return order, nil
}

func (r *orderRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.Order, error) {
    query := `
        SELECT id, user_id, reference_id, amount, currency, 
               description, status, metadata, created_at, updated_at
        FROM orders
        WHERE user_id = $1
        ORDER BY created_at DESC
    `
    
    rows, err := r.db.QueryContext(ctx, query, userID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var orders []models.Order
    for rows.Next() {
        var order models.Order
        var metadataJSON sql.NullString
        
        err := rows.Scan(
            &order.ID,
            &order.UserID,
            &order.ReferenceID,
            &order.Amount,
            &order.Currency,
            &order.Description,
            &order.Status,
            &metadataJSON,
            &order.CreatedAt,
            &order.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        
        if metadataJSON.Valid && metadataJSON.String != "" {
            var metadata map[string]interface{}
            if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
                order.Metadata = metadata
            }
        }
        
        orders = append(orders, order)
    }
    
    return orders, nil
}

func (r *orderRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
    query := `
        UPDATE orders
        SET status = $1, updated_at = NOW()
        WHERE id = $2
        RETURNING updated_at
    `
    
    result, err := r.db.ExecContext(ctx, query, status, id)
    if err != nil {
        return err
    }
    
    rows, err := result.RowsAffected()
    if err != nil {
        return err
    }
    
    if rows == 0 {
        return &NotFoundError{Message: "order not found"}
    }
    
    return nil
}