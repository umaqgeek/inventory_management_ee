FROM golang:1.23 AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/inventory-reservation ./cmd/server

FROM gcr.io/distroless/base-debian12

WORKDIR /app

COPY --from=build /bin/inventory-reservation /app/inventory-reservation
COPY --from=build /app/seeds /app/seeds

EXPOSE 8080

ENV HTTP_ADDR=:8080
ENV SQLITE_DSN=file:/app/data/inventory.db?_pragma=busy_timeout(5000)
ENV HOLD_DURATION=2m
ENV EXPIRY_SWEEP_PERIOD=5s
ENV SEED_FILE=/app/seeds/products.json

CMD ["/app/inventory-reservation"]
