package repositories

import (
	"context"
	"database/sql"
	"pg-backend/internal/database"
	"pg-backend/internal/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type UserRepository interface {
	CreateUser(ctx context.Context, email string) (*models.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
}

type userRepository struct {
	db *sql.DB
}

func NewUserRepository() UserRepository {
	return &userRepository{
		db: database.DB,
	}
}

func (r *userRepository) CreateUser(ctx context.Context, email string) (*models.User, error) {
	query := `
		INSERT INTO users (email)
		VALUES ($1)
		RETURNING id, email, created_at
	`

	user := &models.User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.CreatedAt,
	)

	if err != nil {
		// Check if it's a duplicate email error
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return nil, &DuplicateError{Message: "user with this email already exists"}
		}
		return nil, err
	}

	return user, nil
}

func (r *userRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, email, created_at
		FROM users
		WHERE id = $1
	`

	user := &models.User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "user not found"}
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (r *userRepository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, created_at
		FROM users
		WHERE email = $1
	`

	user := &models.User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Message: "user not found"}
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

// Custom error types
type DuplicateError struct {
	Message string
}

func (e *DuplicateError) Error() string {
	return e.Message
}

type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}
