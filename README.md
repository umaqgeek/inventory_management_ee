# Inventory Reservation System

Go implementation of the Everest Engineering inventory reservation coding challenge. The service supports:

- Basic inventory reservation
- Reservation lifecycle: `active`, `confirmed`, `cancelled`, `expired`
- Automatic expiry and stock release
- Concurrency-safe reservation handling
- SQLite persistence
- Docker and Docker Compose for local development

## Tech Stack

- Go 1.23
- Standard library HTTP server
- SQLite via `modernc.org/sqlite`

## Business Rules

- Available stock = `total_stock - confirmed_sales - active_reservations`
- Reservations that exceed available stock fail
- Confirmed purchases are final
- Expired reservations automatically release stock
- High-contention requests are serialized with a single in-process mutex to prevent overselling

## Run Locally

```bash
GOCACHE=$(pwd)/.gocache go run ./cmd/server
```

The API starts on `http://localhost:8080`.

The app seeds initial products from [seeds/products.json](/Users/umarmukhtar/softwareProjects/inventoryManagement_everestEngineering/seeds/products.json).

## Run With Docker

```bash
docker compose up --build
```

## Test

```bash
GOCACHE=$(pwd)/.gocache go test ./...
```

The test suite includes the challenge concurrency scenario: stock `= 1` with `500` simultaneous reservation requests, where exactly `1` request succeeds and `499` fail.

## API

### Health

```bash
curl http://localhost:8080/healthz
```

### List products

```bash
curl http://localhost:8080/products
```

### Create product

```bash
curl -X POST http://localhost:8080/products \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "sku-monitor",
    "name": "4K Monitor",
    "total_stock": 5
  }'
```

### Reserve inventory

```bash
curl -X POST http://localhost:8080/reservations \
  -H 'Content-Type: application/json' \
  -d '{
    "product_id": "sku-monitor",
    "user_id": "user-123",
    "quantity": 1
  }'
```

### Confirm reservation

```bash
curl -X POST http://localhost:8080/reservations/{reservation_id}/confirm
```

### Cancel reservation

```bash
curl -X POST http://localhost:8080/reservations/{reservation_id}/cancel
```

### Get reservation

```bash
curl http://localhost:8080/reservations/{reservation_id}
```

### Get product snapshot

```bash
curl http://localhost:8080/products/{product_id}
```

## Notes

- I kept the frontend out of scope because the challenge PDF focuses on backend behavior, concurrency, expiry logic, and tests.
- SQLite is used for simple persistence in development and Docker.
- The locking strategy is intentionally simple and explicit for correctness under contention.
