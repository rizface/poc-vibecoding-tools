package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/genai"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type config struct {
	dbHost     string
	dbUser     string
	dbPassword string
	dbName     string
	dbPort     string
	dbTimezone string

	geminiApiKey string
}

func loadEnvOr(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}

	return defaultVal
}

func loadEnvOrError(key string) string {
	if val := os.Getenv(key); val == "" {
		panic(fmt.Sprintf("env var %s is empty", key))
	} else {
		return val
	}
}

func loadConfig() *config {
	return &config{
		dbHost:       loadEnvOr("DB_HOST", "localhost"),
		dbUser:       loadEnvOr("DB_USER", "postgres"),
		dbPassword:   loadEnvOr("DB_PASSWORD", "postgres"),
		dbName:       loadEnvOr("DB_NAME", "postgres"),
		dbPort:       loadEnvOr("DB_PORT", "5432"),
		dbTimezone:   loadEnvOr("DB_TIMEZONE", "Asia/Jakarta"),
		geminiApiKey: loadEnvOrError("GEMINI_API_KEY"),
	}
}

func openDB(cfg *config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=%s",
		cfg.dbHost,
		cfg.dbUser,
		cfg.dbPassword,
		cfg.dbName,
		cfg.dbPort,
		cfg.dbTimezone,
	)

	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func openGeminiClient(ctx context.Context, cfg *config) (*genai.Client, error) {
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.geminiApiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	return geminiClient, nil
}
