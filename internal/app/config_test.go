package app

import "testing"

func TestLoadConfigUsesPortWhenPresent(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("PORT", "54321")
	t.Setenv("DYNO", "")
	t.Setenv("SQLITE_DSN", "")

	cfg := LoadConfig()

	if cfg.HTTPAddr != ":54321" {
		t.Fatalf("expected HTTPAddr to use PORT, got %q", cfg.HTTPAddr)
	}
}

func TestLoadConfigUsesTmpSQLiteOnHerokuWhenUnset(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("DYNO", "web.1")
	t.Setenv("SQLITE_DSN", "")

	cfg := LoadConfig()

	want := "file:/tmp/inventory.db?_pragma=busy_timeout(5000)"
	if cfg.SQLiteDSN != want {
		t.Fatalf("expected SQLiteDSN %q, got %q", want, cfg.SQLiteDSN)
	}
}
