package config

import (
	"errors"
	"os"
)

type Config struct {
	Port            string
	DatabaseURL     string
	Env             string
	PublicBaseURL   string
	TranslateAPIURL string
	TranslateAPIKey string
	PrimaryLanguage string

	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	AppEncryptionKey string
}

func Load() (Config, error) {
	c := Config{
		Port:            getenv("PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		Env:             getenv("APP_ENV", "development"),
		PublicBaseURL:   os.Getenv("PUBLIC_BASE_URL"),
		TranslateAPIURL: os.Getenv("TRANSLATE_API_URL"),
		TranslateAPIKey: os.Getenv("TRANSLATE_API_KEY"),
		PrimaryLanguage: getenv("PRIMARY_LANGUAGE", "en"),

		SMTPHost:         getenv("SMTP_HOST", ""),
		SMTPPort:         getenv("SMTP_PORT", "587"),
		SMTPUser:         os.Getenv("SMTP_USER"),
		SMTPPass:         os.Getenv("SMTP_PASS"),
		SMTPFrom:         getenv("SMTP_FROM", "noreply@church.local"),
		AppEncryptionKey: os.Getenv("APP_ENCRYPTION_KEY"),
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
