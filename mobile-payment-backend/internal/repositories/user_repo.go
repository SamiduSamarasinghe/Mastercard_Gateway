package repositories

import (
	"context"
	"database/sql"

	"mobile-payment-backend/internal/models"

	"github.com/google/uuid"
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type userRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	query := `
        INSERT INTO users (id, email, first_name, last_name, phone)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING created_at, updated_at
    `

	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	return r.db.QueryRowContext(ctx, query,
		user.ID,
		user.Email,
		user.FirstName,
		user.LastName,
		user.Phone,
	).Scan(&user.CreatedAt, &user.UpdatedAt)
}

func (r *userRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
        SELECT id, email, first_name, last_name, phone, created_at, updated_at
        FROM users
        WHERE id = $1
    `

	user := &models.User{}

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.FirstName,
		&user.LastName,
		&user.Phone,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "user not found"}
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
        SELECT id, email, first_name, last_name, phone, created_at, updated_at
        FROM users
        WHERE email = $1
    `

	user := &models.User{}

	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.FirstName,
		&user.LastName,
		&user.Phone,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "user not found"}
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (r *userRepository) Update(ctx context.Context, user *models.User) error {
	query := `
        UPDATE users
        SET email = $2, first_name = $3, last_name = $4, phone = $5, updated_at = NOW()
        WHERE id = $1
        RETURNING updated_at
    `

	return r.db.QueryRowContext(ctx, query,
		user.ID,
		user.Email,
		user.FirstName,
		user.LastName,
		user.Phone,
	).Scan(&user.UpdatedAt)
}

func (r *userRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM users WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return &NotFoundError{Message: "user not found"}
	}

	return nil
}
