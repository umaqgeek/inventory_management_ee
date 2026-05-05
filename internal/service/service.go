package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/umarmukhtar/inventory-reservation-system/internal/domain"
)

type StateStore interface {
	Load(ctx context.Context) ([]domain.Product, []domain.Reservation, error)
	Save(ctx context.Context, products []domain.Product, reservations []domain.Reservation) error
}

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	mu                sync.Mutex
	products          map[string]domain.Product
	reservations      map[string]domain.Reservation
	holdDuration      time.Duration
	expirySweepPeriod time.Duration
	store             StateStore
	clock             Clock
}

func New(store StateStore, holdDuration, expirySweepPeriod time.Duration) (*Service, error) {
	svc := &Service{
		products:          make(map[string]domain.Product),
		reservations:      make(map[string]domain.Reservation),
		holdDuration:      holdDuration,
		expirySweepPeriod: expirySweepPeriod,
		store:             store,
		clock:             realClock{},
	}

	if err := svc.restore(context.Background()); err != nil {
		return nil, err
	}

	return svc, nil
}

func (s *Service) StartExpiryWorker(ctx context.Context) {
	ticker := time.NewTicker(s.expirySweepPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ExpireReservations(ctx)
		}
	}
}

func (s *Service) CreateProduct(ctx context.Context, id, name string, totalStock int) (domain.ProductSnapshot, error) {
	if id == "" {
		return domain.ProductSnapshot{}, domain.ErrInvalidProductID
	}
	if totalStock < 0 {
		return domain.ProductSnapshot{}, domain.ErrInvalidQuantity
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.products[id]; exists {
		return domain.ProductSnapshot{}, domain.ErrDuplicateProductID
	}

	now := s.clock.Now()
	product := domain.Product{
		ID:         id,
		Name:       name,
		TotalStock: totalStock,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.products[id] = product

	if err := s.persistLocked(ctx); err != nil {
		return domain.ProductSnapshot{}, err
	}

	return s.snapshotLocked(product.ID), nil
}

func (s *Service) ListProducts() []domain.ProductSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.expireReservationsLocked(context.Background())

	snapshots := make([]domain.ProductSnapshot, 0, len(s.products))
	for _, product := range s.products {
		snapshots = append(snapshots, s.snapshotLocked(product.ID))
	}
	return snapshots
}

func (s *Service) GetProduct(id string) (domain.ProductSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.expireReservationsLocked(context.Background())

	if _, ok := s.products[id]; !ok {
		return domain.ProductSnapshot{}, domain.ErrProductNotFound
	}
	return s.snapshotLocked(id), nil
}

func (s *Service) CreateReservation(ctx context.Context, productID, userID string, quantity int) (domain.Reservation, domain.ProductSnapshot, error) {
	if productID == "" {
		return domain.Reservation{}, domain.ProductSnapshot{}, domain.ErrInvalidProductID
	}
	if userID == "" {
		return domain.Reservation{}, domain.ProductSnapshot{}, domain.ErrInvalidUserID
	}
	if quantity <= 0 {
		return domain.Reservation{}, domain.ProductSnapshot{}, domain.ErrInvalidQuantity
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.expireReservationsLocked(ctx)

	product, ok := s.products[productID]
	if !ok {
		return domain.Reservation{}, domain.ProductSnapshot{}, domain.ErrProductNotFound
	}

	if s.availableStockLocked(productID) < quantity {
		return domain.Reservation{}, domain.ProductSnapshot{}, domain.ErrInsufficientStock
	}

	now := s.clock.Now()
	reservation := domain.Reservation{
		ID:        newID(),
		ProductID: productID,
		UserID:    userID,
		Quantity:  quantity,
		Status:    domain.ReservationStatusActive,
		ExpiresAt: now.Add(s.holdDuration),
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.reservations[reservation.ID] = reservation

	if err := s.persistLocked(ctx); err != nil {
		return domain.Reservation{}, domain.ProductSnapshot{}, err
	}

	return reservation, s.snapshotLocked(product.ID), nil
}

func (s *Service) ConfirmReservation(ctx context.Context, id string) (domain.Reservation, domain.ProductSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.expireReservationsLocked(ctx)

	reservation, ok := s.reservations[id]
	if !ok {
		return domain.Reservation{}, domain.ProductSnapshot{}, domain.ErrReservationNotFound
	}
	if reservation.Status != domain.ReservationStatusActive {
		return domain.Reservation{}, domain.ProductSnapshot{}, domain.ErrReservationNotActive
	}

	product, ok := s.products[reservation.ProductID]
	if !ok {
		return domain.Reservation{}, domain.ProductSnapshot{}, domain.ErrProductNotFound
	}

	now := s.clock.Now()
	reservation.Status = domain.ReservationStatusConfirmed
	reservation.UpdatedAt = now
	s.reservations[reservation.ID] = reservation

	product.ConfirmedSales += reservation.Quantity
	product.UpdatedAt = now
	s.products[product.ID] = product

	if err := s.persistLocked(ctx); err != nil {
		return domain.Reservation{}, domain.ProductSnapshot{}, err
	}

	return reservation, s.snapshotLocked(product.ID), nil
}

func (s *Service) CancelReservation(ctx context.Context, id string) (domain.Reservation, domain.ProductSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.expireReservationsLocked(ctx)

	reservation, ok := s.reservations[id]
	if !ok {
		return domain.Reservation{}, domain.ProductSnapshot{}, domain.ErrReservationNotFound
	}
	if reservation.Status != domain.ReservationStatusActive {
		return domain.Reservation{}, domain.ProductSnapshot{}, domain.ErrReservationNotActive
	}

	now := s.clock.Now()
	reservation.Status = domain.ReservationStatusCancelled
	reservation.UpdatedAt = now
	s.reservations[reservation.ID] = reservation

	if err := s.persistLocked(ctx); err != nil {
		return domain.Reservation{}, domain.ProductSnapshot{}, err
	}

	return reservation, s.snapshotLocked(reservation.ProductID), nil
}

func (s *Service) GetReservation(id string) (domain.Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.expireReservationsLocked(context.Background())

	reservation, ok := s.reservations[id]
	if !ok {
		return domain.Reservation{}, domain.ErrReservationNotFound
	}
	return reservation, nil
}

func (s *Service) ExpireReservations(ctx context.Context) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.expireReservationsLocked(ctx)
}

func (s *Service) expireReservationsLocked(ctx context.Context) int {
	now := s.clock.Now()
	expired := 0
	changed := false

	for id, reservation := range s.reservations {
		if reservation.Status == domain.ReservationStatusActive && !reservation.ExpiresAt.After(now) {
			reservation.Status = domain.ReservationStatusExpired
			reservation.UpdatedAt = now
			s.reservations[id] = reservation
			expired++
			changed = true
		}
	}

	if changed {
		_ = s.persistLocked(ctx)
	}

	return expired
}

func (s *Service) restore(ctx context.Context) error {
	if s.store == nil {
		return nil
	}

	products, reservations, err := s.store.Load(ctx)
	if err != nil {
		return err
	}

	for _, product := range products {
		s.products[product.ID] = product
	}
	for _, reservation := range reservations {
		s.reservations[reservation.ID] = reservation
	}

	return nil
}

func (s *Service) persistLocked(ctx context.Context) error {
	if s.store == nil {
		return nil
	}

	products := make([]domain.Product, 0, len(s.products))
	for _, product := range s.products {
		products = append(products, product)
	}

	reservations := make([]domain.Reservation, 0, len(s.reservations))
	for _, reservation := range s.reservations {
		reservations = append(reservations, reservation)
	}

	return s.store.Save(ctx, products, reservations)
}

func (s *Service) activeReservedUnitsLocked(productID string) int {
	total := 0
	for _, reservation := range s.reservations {
		if reservation.ProductID == productID && reservation.Status == domain.ReservationStatusActive {
			total += reservation.Quantity
		}
	}
	return total
}

func (s *Service) availableStockLocked(productID string) int {
	product := s.products[productID]
	return product.TotalStock - product.ConfirmedSales - s.activeReservedUnitsLocked(productID)
}

func (s *Service) snapshotLocked(productID string) domain.ProductSnapshot {
	product := s.products[productID]
	activeReservedUnits := s.activeReservedUnitsLocked(productID)
	return domain.ProductSnapshot{
		Product:             product,
		ActiveReservedUnits: activeReservedUnits,
		AvailableStock:      product.TotalStock - product.ConfirmedSales - activeReservedUnits,
	}
}

func newID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
