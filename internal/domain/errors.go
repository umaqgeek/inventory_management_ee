package domain

import "errors"

var (
	ErrProductNotFound      = errors.New("product not found")
	ErrReservationNotFound  = errors.New("reservation not found")
	ErrInsufficientStock    = errors.New("insufficient stock")
	ErrInvalidQuantity      = errors.New("quantity must be greater than zero")
	ErrInvalidTransition    = errors.New("invalid reservation state transition")
	ErrReservationNotActive = errors.New("reservation is not active")
	ErrDuplicateProductID   = errors.New("product already exists")
	ErrInvalidProductID     = errors.New("product_id is required")
	ErrInvalidUserID        = errors.New("user_id is required")
)
