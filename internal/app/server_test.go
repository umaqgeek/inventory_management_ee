package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/umarmukhtar/inventory-reservation-system/internal/service"
)

func TestRootServesSwaggerUI(t *testing.T) {
	svc, err := service.New(nil, 2*time.Minute, time.Second)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	newHandler(svc).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("expected html content type, got %q", contentType)
	}
	if !strings.Contains(rec.Body.String(), "SwaggerUIBundle") {
		t.Fatalf("expected swagger UI bootstrap in response body")
	}
}

func TestOpenAPISpecServed(t *testing.T) {
	svc, err := service.New(nil, 2*time.Minute, time.Second)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	newHandler(svc).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("expected json content type, got %q", contentType)
	}
	if !strings.Contains(rec.Body.String(), "\"openapi\": \"3.0.3\"") {
		t.Fatalf("expected openapi version in response body")
	}
}
