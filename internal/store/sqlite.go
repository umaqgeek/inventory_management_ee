package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/umarmukhtar/inventory-reservation-system/internal/domain"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	schema := `
PRAGMA journal_mode = WAL;
CREATE TABLE IF NOT EXISTS products (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  total_stock INTEGER NOT NULL,
  confirmed_sales INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS reservations (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  quantity INTEGER NOT NULL,
  status TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(product_id) REFERENCES products(id)
);
`

	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Load(ctx context.Context) ([]domain.Product, []domain.Reservation, error) {
	productRows, err := s.db.QueryContext(ctx, `
SELECT id, name, total_stock, confirmed_sales, created_at, updated_at
FROM products
ORDER BY id
`)
	if err != nil {
		return nil, nil, fmt.Errorf("query products: %w", err)
	}
	defer productRows.Close()

	var products []domain.Product
	for productRows.Next() {
		var product domain.Product
		var createdAt, updatedAt string
		if err := productRows.Scan(
			&product.ID,
			&product.Name,
			&product.TotalStock,
			&product.ConfirmedSales,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan product: %w", err)
		}
		product.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, nil, fmt.Errorf("parse product created_at: %w", err)
		}
		product.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, nil, fmt.Errorf("parse product updated_at: %w", err)
		}
		products = append(products, product)
	}

	reservationRows, err := s.db.QueryContext(ctx, `
SELECT id, product_id, user_id, quantity, status, expires_at, created_at, updated_at
FROM reservations
ORDER BY id
`)
	if err != nil {
		return nil, nil, fmt.Errorf("query reservations: %w", err)
	}
	defer reservationRows.Close()

	var reservations []domain.Reservation
	for reservationRows.Next() {
		var reservation domain.Reservation
		var status string
		var expiresAt, createdAt, updatedAt string
		if err := reservationRows.Scan(
			&reservation.ID,
			&reservation.ProductID,
			&reservation.UserID,
			&reservation.Quantity,
			&status,
			&expiresAt,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan reservation: %w", err)
		}
		reservation.Status = domain.ReservationStatus(status)
		reservation.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil {
			return nil, nil, fmt.Errorf("parse reservation expires_at: %w", err)
		}
		reservation.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, nil, fmt.Errorf("parse reservation created_at: %w", err)
		}
		reservation.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, nil, fmt.Errorf("parse reservation updated_at: %w", err)
		}
		reservations = append(reservations, reservation)
	}

	return products, reservations, nil
}

func (s *SQLiteStore) Save(ctx context.Context, products []domain.Product, reservations []domain.Reservation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM reservations`); err != nil {
		return fmt.Errorf("clear reservations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM products`); err != nil {
		return fmt.Errorf("clear products: %w", err)
	}

	for _, product := range products {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO products (id, name, total_stock, confirmed_sales, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
`, product.ID, product.Name, product.TotalStock, product.ConfirmedSales, product.CreatedAt.Format(time.RFC3339Nano), product.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert product: %w", err)
		}
	}

	for _, reservation := range reservations {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO reservations (id, product_id, user_id, quantity, status, expires_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, reservation.ID, reservation.ProductID, reservation.UserID, reservation.Quantity, reservation.Status, reservation.ExpiresAt.Format(time.RFC3339Nano), reservation.CreatedAt.Format(time.RFC3339Nano), reservation.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert reservation: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
