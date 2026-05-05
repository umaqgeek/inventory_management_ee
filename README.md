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

For reviewer-friendly output, run the tests in verbose mode:

```bash
GOCACHE=$(pwd)/.gocache go test -v ./...
```

That prints each scenario and a short proof point, for example:

```text
=== RUN   TestReservationLifecycle
    service_test.go:29: verifies active -> confirmed lifecycle and stock accounting
    service_test.go:43: reservation created: id=... status=active available_stock=1
    service_test.go:54: reservation confirmed: id=... status=confirmed confirmed_sales=1 available_stock=1
--- PASS: TestReservationLifecycle (0.00s)
=== RUN   TestReservationExpiryReleasesStock
    service_test.go:67: verifies expired reservations release stock back to inventory
    service_test.go:81: reservation created: id=... expires_at=2026-05-05T00:02:00Z
    service_test.go:87: expiry sweep marked the reservation as expired
    service_test.go:102: stock released successfully: available_stock=1
--- PASS: TestReservationExpiryReleasesStock (0.00s)
=== RUN   TestConcurrentReservationsOnlyOneSucceeds
    service_test.go:106: verifies the flash-sale contention scenario from the challenge: stock=1, requests=500
    service_test.go:146: contention results: successes=1 failures=499
    service_test.go:156: final product snapshot: confirmed_sales=0 active_reserved_units=1 available_stock=0
--- PASS: TestConcurrentReservationsOnlyOneSucceeds (0.00s)
=== RUN   TestSQLiteStorePersistsState
    sqlite_test.go:15: verifies product and reservation state round-trips through SQLite
    sqlite_test.go:42: state written to SQLite successfully
    sqlite_test.go:56: state loaded successfully: products=1 reservations=1
--- PASS: TestSQLiteStorePersistsState (0.00s)
PASS
```

What each test proves:

- `TestReservationLifecycle`: a reservation starts as `active`, can be confirmed, and updates stock counters correctly.
- `TestReservationExpiryReleasesStock`: an expired reservation changes state to `expired` and releases stock automatically.
- `TestConcurrentReservationsOnlyOneSucceeds`: under `500` simultaneous requests against stock `1`, only one reservation succeeds and overselling does not occur.
- `TestSQLiteStorePersistsState`: the service can save and reload state from SQLite without losing product or reservation data.

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
