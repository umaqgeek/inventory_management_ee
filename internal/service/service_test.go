package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/umarmukhtar/inventory-reservation-system/internal/domain"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestReservationLifecycle(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)}
	svc, err := New(nil, 2*time.Minute, time.Second)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.clock = clock

	if _, err := svc.CreateProduct(context.Background(), "sku-1", "Widget", 2); err != nil {
		t.Fatalf("create product: %v", err)
	}

	reservation, product, err := svc.CreateReservation(context.Background(), "sku-1", "user-1", 1)
	if err != nil {
		t.Fatalf("create reservation: %v", err)
	}
	if reservation.Status != domain.ReservationStatusActive {
		t.Fatalf("expected active reservation, got %s", reservation.Status)
	}
	if product.AvailableStock != 1 {
		t.Fatalf("expected available stock 1, got %d", product.AvailableStock)
	}

	confirmed, product, err := svc.ConfirmReservation(context.Background(), reservation.ID)
	if err != nil {
		t.Fatalf("confirm reservation: %v", err)
	}
	if confirmed.Status != domain.ReservationStatusConfirmed {
		t.Fatalf("expected confirmed reservation, got %s", confirmed.Status)
	}
	if product.Product.ConfirmedSales != 1 {
		t.Fatalf("expected confirmed sales 1, got %d", product.Product.ConfirmedSales)
	}
	if product.AvailableStock != 1 {
		t.Fatalf("expected available stock 1 after confirm, got %d", product.AvailableStock)
	}
}

func TestReservationExpiryReleasesStock(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)}
	svc, err := New(nil, 2*time.Minute, time.Second)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.clock = clock

	if _, err := svc.CreateProduct(context.Background(), "sku-1", "Widget", 1); err != nil {
		t.Fatalf("create product: %v", err)
	}

	reservation, _, err := svc.CreateReservation(context.Background(), "sku-1", "user-1", 1)
	if err != nil {
		t.Fatalf("create reservation: %v", err)
	}

	clock.Advance(3 * time.Minute)
	if expired := svc.ExpireReservations(context.Background()); expired != 1 {
		t.Fatalf("expected 1 expired reservation, got %d", expired)
	}

	stored, err := svc.GetReservation(reservation.ID)
	if err != nil {
		t.Fatalf("get reservation: %v", err)
	}
	if stored.Status != domain.ReservationStatusExpired {
		t.Fatalf("expected expired reservation, got %s", stored.Status)
	}

	product, err := svc.GetProduct("sku-1")
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if product.AvailableStock != 1 {
		t.Fatalf("expected released stock to be 1, got %d", product.AvailableStock)
	}
}

func TestConcurrentReservationsOnlyOneSucceeds(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)}
	svc, err := New(nil, 2*time.Minute, time.Second)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.clock = clock

	if _, err := svc.CreateProduct(context.Background(), "sku-1", "Flash Sale", 1); err != nil {
		t.Fatalf("create product: %v", err)
	}

	var wg sync.WaitGroup
	successes := 0
	failures := 0
	var resultMu sync.Mutex

	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := svc.CreateReservation(context.Background(), "sku-1", fmt.Sprintf("user-%d", i), 1)

			resultMu.Lock()
			defer resultMu.Unlock()
			if err == nil {
				successes++
				return
			}
			if err == domain.ErrInsufficientStock {
				failures++
				return
			}
			t.Errorf("unexpected error: %v", err)
		}(i)
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly 1 success, got %d", successes)
	}
	if failures != 499 {
		t.Fatalf("expected 499 failures, got %d", failures)
	}

	product, err := svc.GetProduct("sku-1")
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if product.AvailableStock != 0 {
		t.Fatalf("expected no available stock after reservation, got %d", product.AvailableStock)
	}
}
