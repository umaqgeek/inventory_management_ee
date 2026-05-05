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
	t.Log("verifies active -> confirmed lifecycle and stock accounting")

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
	t.Logf("reservation created: id=%s status=%s available_stock=%d", reservation.ID, reservation.Status, product.AvailableStock)
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
	t.Logf("reservation confirmed: id=%s status=%s confirmed_sales=%d available_stock=%d", confirmed.ID, confirmed.Status, product.Product.ConfirmedSales, product.AvailableStock)
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
	t.Log("verifies expired reservations release stock back to inventory")

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
	t.Logf("reservation created: id=%s expires_at=%s", reservation.ID, reservation.ExpiresAt.Format(time.RFC3339))

	clock.Advance(3 * time.Minute)
	if expired := svc.ExpireReservations(context.Background()); expired != 1 {
		t.Fatalf("expected 1 expired reservation, got %d", expired)
	}
	t.Log("expiry sweep marked the reservation as expired")

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
	t.Logf("stock released successfully: available_stock=%d", product.AvailableStock)
}

func TestConcurrentReservationsOnlyOneSucceeds(t *testing.T) {
	t.Log("verifies the flash-sale contention scenario from the challenge: stock=1, requests=500")
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
	t.Logf("contention results: successes=%d failures=%d", successes, failures)

	product, err := svc.GetProduct("sku-1")
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if product.AvailableStock != 0 {
		t.Fatalf("expected no available stock after reservation, got %d", product.AvailableStock)
	}
	t.Logf("final product snapshot: confirmed_sales=%d active_reserved_units=%d available_stock=%d", product.Product.ConfirmedSales, product.ActiveReservedUnits, product.AvailableStock)
}
