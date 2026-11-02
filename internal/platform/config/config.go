package config

import (
	"errors"
	"os"
)

type Config struct {
	Port          string
	DatabaseURL   string
	Env           string
	PublicBaseURL string // e.g. https://church.example.com, used for practice links/QR
}

func Load() (Config, error) {
	c := Config{
		Port:        getenv("PORT", "8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		Env:           getenv("APP_ENV", "development"),
		PublicBaseURL: os.Getenv("PUBLIC_BASE_URL"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	return c, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
