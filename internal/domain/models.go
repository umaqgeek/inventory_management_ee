package domain

import "time"

type ReservationStatus string

const (
	ReservationStatusActive    ReservationStatus = "active"
	ReservationStatusConfirmed ReservationStatus = "confirmed"
	ReservationStatusCancelled ReservationStatus = "cancelled"
	ReservationStatusExpired   ReservationStatus = "expired"
)

type Product struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	TotalStock     int       `json:"total_stock"`
	ConfirmedSales int       `json:"confirmed_sales"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Reservation struct {
	ID        string            `json:"id"`
	ProductID string            `json:"product_id"`
	UserID    string            `json:"user_id"`
	Quantity  int               `json:"quantity"`
	Status    ReservationStatus `json:"status"`
	ExpiresAt time.Time         `json:"expires_at"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type ProductSnapshot struct {
	Product             Product `json:"product"`
	ActiveReservedUnits int     `json:"active_reserved_units"`
	AvailableStock      int     `json:"available_stock"`
}
