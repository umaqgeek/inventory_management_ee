package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/umarmukhtar/inventory-reservation-system/internal/domain"
)

func TestSQLiteStorePersistsState(t *testing.T) {
	t.Parallel()
	t.Log("verifies product and reservation state round-trips through SQLite")

	dsn := "file:" + filepath.Join(t.TempDir(), "inventory.db")
	store, err := NewSQLiteStore(dsn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	products := []domain.Product{{
		ID:             "sku-1",
		Name:           "Widget",
		TotalStock:     5,
		ConfirmedSales: 1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}}
	reservations := []domain.Reservation{{
		ID:        "res-1",
		ProductID: "sku-1",
		UserID:    "user-1",
		Quantity:  2,
		Status:    domain.ReservationStatusActive,
		ExpiresAt: now.Add(2 * time.Minute),
		CreatedAt: now,
		UpdatedAt: now,
	}}

	if err := store.Save(context.Background(), products, reservations); err != nil {
		t.Fatalf("save: %v", err)
	}
	t.Log("state written to SQLite successfully")

	loadedProducts, loadedReservations, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loadedProducts) != 1 {
		t.Fatalf("expected 1 product, got %d", len(loadedProducts))
	}
	if len(loadedReservations) != 1 {
		t.Fatalf("expected 1 reservation, got %d", len(loadedReservations))
	}
	if loadedProducts[0].ID != "sku-1" || loadedReservations[0].ID != "res-1" {
		t.Fatalf("unexpected persisted state: %+v %+v", loadedProducts[0], loadedReservations[0])
	}
	t.Logf("state loaded successfully: products=%d reservations=%d", len(loadedProducts), len(loadedReservations))
}
