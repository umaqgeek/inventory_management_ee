package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/products", h.handleProducts)
	mux.HandleFunc("/products/", h.handleProductByID)
	mux.HandleFunc("/reservations", h.handleReservations)
	mux.HandleFunc("/reservations/", h.handleReservationByID)
	return mux
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
