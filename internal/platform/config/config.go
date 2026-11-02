package config

import (
	"errors"
	"os"
)

type Config struct {
	Port            string
	DatabaseURL     string
	Env             string
	PublicBaseURL   string // e.g. https://church.example.com, used for practice links/QR
	TranslateAPIURL string // empty = translations must be pasted manually
	TranslateAPIKey string // optional, passed through to the endpoint
	PrimaryLanguage string // church's primary language, default "en"
}

func Load() (Config, error) {
	c := Config{
		Port:        getenv("PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		Env:             getenv("APP_ENV", "development"),
		PublicBaseURL:   os.Getenv("PUBLIC_BASE_URL"),
		TranslateAPIURL: os.Getenv("TRANSLATE_API_URL"),
		TranslateAPIKey: os.Getenv("TRANSLATE_API_KEY"),
		PrimaryLanguage: getenv("PRIMARY_LANGUAGE", "en"),
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
