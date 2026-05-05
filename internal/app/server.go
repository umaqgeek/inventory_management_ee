package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/umarmukhtar/inventory-reservation-system/internal/domain"
	"github.com/umarmukhtar/inventory-reservation-system/internal/service"
	"github.com/umarmukhtar/inventory-reservation-system/internal/store"
)

type Server struct {
	httpServer   *http.Server
	service      *service.Service
	expiryCancel context.CancelFunc
	closer       func() error
}

func NewServer(cfg Config) (*Server, error) {
	dbPath := strings.TrimPrefix(cfg.SQLiteDSN, "file:")
	if idx := strings.Index(dbPath, "?"); idx >= 0 {
		dbPath = dbPath[:idx]
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	sqliteStore, err := store.NewSQLiteStore(cfg.SQLiteDSN)
	if err != nil {
		return nil, err
	}

	svc, err := service.New(sqliteStore, cfg.HoldDuration, cfg.ExpirySweepPeriod)
	if err != nil {
		return nil, err
	}
	if err := seedProducts(context.Background(), svc, cfg.SeedFile); err != nil {
		return nil, err
	}

	expiryCtx, cancel := context.WithCancel(context.Background())
	go svc.StartExpiryWorker(expiryCtx)

	handler := newHandler(svc)
	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: handler.routes(),
	}

	return &Server{
		httpServer:   httpServer,
		service:      svc,
		expiryCancel: cancel,
		closer:       sqliteStore.Close,
	}, nil
}

type seedProduct struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	TotalStock int    `json:"total_stock"`
}

func seedProducts(ctx context.Context, svc *service.Service, path string) error {
	if path == "" {
		return nil
	}
	if len(svc.ListProducts()) > 0 {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open seed file: %w", err)
	}
	defer file.Close()

	var products []seedProduct
	if err := json.NewDecoder(file).Decode(&products); err != nil {
		return fmt.Errorf("decode seed file: %w", err)
	}

	for _, product := range products {
		if _, err := svc.CreateProduct(ctx, product.ID, product.Name, product.TotalStock); err != nil {
			return fmt.Errorf("seed product %s: %w", product.ID, err)
		}
	}

	return nil
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.expiryCancel()
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return err
	}
	if s.closer != nil {
		return s.closer()
	}
	return nil
}

type handler struct {
	service *service.Service
}

func newHandler(svc *service.Service) *handler {
	return &handler{service: svc}
}

func (h *handler) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.handleSwaggerUI)
	mux.HandleFunc("/openapi.json", h.handleOpenAPI)
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/products", h.handleProducts)
	mux.HandleFunc("/products/", h.handleProductByID)
	mux.HandleFunc("/reservations", h.handleReservations)
	mux.HandleFunc("/reservations/", h.handleReservationByID)
	return mux
}

func (h *handler) handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, swaggerUIHTML)
}

func (h *handler) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(openAPISpec))
}

func (h *handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type createProductRequest struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	TotalStock int    `json:"total_stock"`
}

func (h *handler) handleProducts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"products": h.service.ListProducts()})
	case http.MethodPost:
		var req createProductRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json payload")
			return
		}
		product, err := h.service.CreateProduct(r.Context(), req.ID, req.Name, req.TotalStock)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, product)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *handler) handleProductByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/products/")
	product, err := h.service.GetProduct(id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

type createReservationRequest struct {
	ProductID string `json:"product_id"`
	UserID    string `json:"user_id"`
	Quantity  int    `json:"quantity"`
}

func (h *handler) handleReservations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req createReservationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json payload")
			return
		}
		reservation, product, err := h.service.CreateReservation(r.Context(), req.ProductID, req.UserID, req.Quantity)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"reservation": reservation,
			"product":     product,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *handler) handleReservationByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/reservations/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "reservation not found")
		return
	}

	id := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		reservation, err := h.service.GetReservation(id)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, reservation)
		return
	}

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	action := parts[1]
	switch action {
	case "confirm":
		reservation, product, err := h.service.ConfirmReservation(r.Context(), id)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"reservation": reservation,
			"product":     product,
		})
	case "cancel":
		reservation, product, err := h.service.CancelReservation(r.Context(), id)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"reservation": reservation,
			"product":     product,
		})
	default:
		writeError(w, http.StatusNotFound, "unsupported reservation action")
	}
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrProductNotFound), errors.Is(err, domain.ErrReservationNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInsufficientStock), errors.Is(err, domain.ErrReservationNotActive):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrDuplicateProductID), errors.Is(err, domain.ErrInvalidQuantity), errors.Is(err, domain.ErrInvalidProductID), errors.Is(err, domain.ErrInvalidUserID):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Inventory Reservation API Docs</title>
    <link
      rel="stylesheet"
      href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"
    />
    <style>
      html {
        box-sizing: border-box;
        overflow-y: scroll;
      }
      *,
      *::before,
      *::after {
        box-sizing: inherit;
      }
      body {
        margin: 0;
        background: #f6f8fb;
      }
      .topbar {
        display: none;
      }
    </style>
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
      window.onload = function () {
        window.ui = SwaggerUIBundle({
          url: "/openapi.json",
          dom_id: "#swagger-ui",
          deepLinking: true,
          presets: [SwaggerUIBundle.presets.apis],
        });
      };
    </script>
  </body>
</html>
`

const openAPISpec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "Inventory Reservation API",
    "description": "API for the Everest Engineering inventory reservation coding challenge.",
    "version": "1.0.0"
  },
  "servers": [
    {
      "url": "http://localhost:8080"
    }
  ],
  "paths": {
    "/healthz": {
      "get": {
        "summary": "Health check",
        "responses": {
          "200": {
            "description": "Service is healthy",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "status": {
                      "type": "string",
                      "example": "ok"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/products": {
      "get": {
        "summary": "List products",
        "responses": {
          "200": {
            "description": "Product snapshots",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "products": {
                      "type": "array",
                      "items": {
                        "$ref": "#/components/schemas/ProductSnapshot"
                      }
                    }
                  }
                }
              }
            }
          }
        }
      },
      "post": {
        "summary": "Create product",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/CreateProductRequest"
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Product created",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ProductSnapshot"
                }
              }
            }
          }
        }
      }
    },
    "/products/{productId}": {
      "get": {
        "summary": "Get product snapshot",
        "parameters": [
          {
            "name": "productId",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Product snapshot",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ProductSnapshot"
                }
              }
            }
          },
          "404": {
            "$ref": "#/components/responses/ErrorResponse"
          }
        }
      }
    },
    "/reservations": {
      "post": {
        "summary": "Create reservation",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/CreateReservationRequest"
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Reservation created",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ReservationActionResponse"
                }
              }
            }
          },
          "409": {
            "$ref": "#/components/responses/ErrorResponse"
          }
        }
      }
    },
    "/reservations/{reservationId}": {
      "get": {
        "summary": "Get reservation",
        "parameters": [
          {
            "name": "reservationId",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Reservation details",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Reservation"
                }
              }
            }
          },
          "404": {
            "$ref": "#/components/responses/ErrorResponse"
          }
        }
      }
    },
    "/reservations/{reservationId}/confirm": {
      "post": {
        "summary": "Confirm reservation",
        "parameters": [
          {
            "name": "reservationId",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Reservation confirmed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ReservationActionResponse"
                }
              }
            }
          },
          "404": {
            "$ref": "#/components/responses/ErrorResponse"
          },
          "409": {
            "$ref": "#/components/responses/ErrorResponse"
          }
        }
      }
    },
    "/reservations/{reservationId}/cancel": {
      "post": {
        "summary": "Cancel reservation",
        "parameters": [
          {
            "name": "reservationId",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Reservation cancelled",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ReservationActionResponse"
                }
              }
            }
          },
          "404": {
            "$ref": "#/components/responses/ErrorResponse"
          },
          "409": {
            "$ref": "#/components/responses/ErrorResponse"
          }
        }
      }
    }
  },
  "components": {
    "responses": {
      "ErrorResponse": {
        "description": "Error response",
        "content": {
          "application/json": {
            "schema": {
              "$ref": "#/components/schemas/ErrorResponse"
            }
          }
        }
      }
    },
    "schemas": {
      "CreateProductRequest": {
        "type": "object",
        "required": ["id", "name", "total_stock"],
        "properties": {
          "id": {
            "type": "string",
            "example": "sku-monitor"
          },
          "name": {
            "type": "string",
            "example": "4K Monitor"
          },
          "total_stock": {
            "type": "integer",
            "example": 5
          }
        }
      },
      "CreateReservationRequest": {
        "type": "object",
        "required": ["product_id", "user_id", "quantity"],
        "properties": {
          "product_id": {
            "type": "string",
            "example": "sku-monitor"
          },
          "user_id": {
            "type": "string",
            "example": "user-123"
          },
          "quantity": {
            "type": "integer",
            "example": 1
          }
        }
      },
      "Product": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string"
          },
          "name": {
            "type": "string"
          },
          "total_stock": {
            "type": "integer"
          },
          "confirmed_sales": {
            "type": "integer"
          },
          "created_at": {
            "type": "string",
            "format": "date-time"
          },
          "updated_at": {
            "type": "string",
            "format": "date-time"
          }
        }
      },
      "ProductSnapshot": {
        "type": "object",
        "properties": {
          "product": {
            "$ref": "#/components/schemas/Product"
          },
          "active_reserved_units": {
            "type": "integer"
          },
          "available_stock": {
            "type": "integer"
          }
        }
      },
      "Reservation": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string"
          },
          "product_id": {
            "type": "string"
          },
          "user_id": {
            "type": "string"
          },
          "quantity": {
            "type": "integer"
          },
          "status": {
            "type": "string",
            "enum": ["active", "confirmed", "cancelled", "expired"]
          },
          "expires_at": {
            "type": "string",
            "format": "date-time"
          },
          "created_at": {
            "type": "string",
            "format": "date-time"
          },
          "updated_at": {
            "type": "string",
            "format": "date-time"
          }
        }
      },
      "ReservationActionResponse": {
        "type": "object",
        "properties": {
          "reservation": {
            "$ref": "#/components/schemas/Reservation"
          },
          "product": {
            "$ref": "#/components/schemas/ProductSnapshot"
          }
        }
      },
      "ErrorResponse": {
        "type": "object",
        "properties": {
          "error": {
            "type": "string",
            "example": "insufficient stock"
          }
        }
      }
    }
  }
}
`
