package config

import (
	"strings"
	"testing"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("Load() error = %v, want DATABASE_URL requirement", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://church:test@localhost/churchplanner")
	t.Setenv("PORT", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("PRIMARY_LANGUAGE", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != "8080" || cfg.Env != "development" ||
		cfg.PrimaryLanguage != "en" {
		t.Fatalf("unexpected defaults: port=%q env=%q language=%q",
			cfg.Port, cfg.Env, cfg.PrimaryLanguage)
	}
}
