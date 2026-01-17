package repositories

import "fmt"

type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("not found: %s", e.Message)
}

type DuplicateError struct {
	Message string
}

func (e *DuplicateError) Error() string {
	return fmt.Sprintf("duplicate: %s", e.Message)
}
