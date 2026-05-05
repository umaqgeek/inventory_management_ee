package store

import (
	"context"
	"sync"

	"github.com/umarmukhtar/inventory-reservation-system/internal/domain"
)

type MemoryStore struct {
	mu           sync.RWMutex
	products     map[string]domain.Product
	reservations map[string]domain.Reservation
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		products:     make(map[string]domain.Product),
		reservations: make(map[string]domain.Reservation),
	}
}

func (s *MemoryStore) Load(context.Context) ([]domain.Product, []domain.Reservation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	products := make([]domain.Product, 0, len(s.products))
	for _, product := range s.products {
		products = append(products, product)
	}

	reservations := make([]domain.Reservation, 0, len(s.reservations))
	for _, reservation := range s.reservations {
		reservations = append(reservations, reservation)
	}

	return products, reservations, nil
}

func (s *MemoryStore) Save(context.Context, []domain.Product, []domain.Reservation) error {
	return nil
}
