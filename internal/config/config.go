package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port               string
	DBURL              string
	JWTSecret          string
	JWTExpiry          time.Duration
	AIServiceURL       string
	AIServiceToken     string
	AIRequestTimeout   time.Duration
	ScrapeInterval     time.Duration
	MaxConcurrency     int
	QueueWorkers       int
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	CacheTTL           time.Duration
	DBQueryTimeout     time.Duration
	LLMRequestTimeout  time.Duration
}

func (c Config) Validate() error {
	port, err := strconv.Atoi(c.Port)
	if err != nil {
		return fmt.Errorf("PORT must be a valid number")
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("PORT must be between 1 and 65535")
	}

	if c.DBURL == "" {
		return fmt.Errorf("DB_URL is not set")
	}

	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is not set")
	}

	if c.JWTExpiry <= 0 {
		return fmt.Errorf("JWT_EXPIRY must be greater than 0")
	}

	if c.MaxConcurrency <= 0 {
		return fmt.Errorf("MAX_CONCURRENCY must be greater than 0")
	}

	if c.MaxConcurrency > 100 {
		return fmt.Errorf("MAX_CONCURRENCY cannot exceed 100")
	}

	if c.QueueWorkers <= 0 {
		return fmt.Errorf("QUEUE_WORKERS must be greater than 0")
	}

	if c.ScrapeInterval < time.Second {
		return fmt.Errorf("SCRAPE_INTERVAL must be at least 1 second")
	}

	if c.AccessTokenExpiry <= 0 {
		return fmt.Errorf("ACCESS_TOKEN_EXPIRY must be greater than 0")
	}

	if c.RefreshTokenExpiry <= 0 {
		return fmt.Errorf("REFRESH_TOKEN_EXPIRY must be greater than 0")
	}

	if c.CacheTTL <= 0 {
		return fmt.Errorf("CACHE_TTL must be greater than 0")
	}

	if c.DBQueryTimeout <= 0 {
		return fmt.Errorf("DB_QUERY_TIMEOUT must be greater than 0")
	}

	if c.LLMRequestTimeout <= 0 {
		return fmt.Errorf("LLM_REQUEST_TIMEOUT must be greater than 0")
	}

	if c.AIRequestTimeout <= 0 {
		return fmt.Errorf("AI_REQUEST_TIMEOUT must be greater than 0")
	}

	if c.AIServiceURL != "" {
		parsed, err := url.Parse(c.AIServiceURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("AI_SERVICE_URL must be an absolute URL such as http://ai-service:8000")
		}
	}

	return nil
}

func LoadConfig() (Config, error) {
	_ = godotenv.Load(".env")

	cfg := Config{}

	cfg.Port = os.Getenv("PORT")
	cfg.DBURL = os.Getenv("DB_URL")
	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	cfg.AIServiceURL = strings.TrimRight(strings.TrimSpace(os.Getenv("AI_SERVICE_URL")), "/")
	cfg.AIServiceToken = strings.TrimSpace(os.Getenv("AI_SERVICE_TOKEN"))

	// ---------------- JWT_EXPIRY ----------------
	jwtExpiry := os.Getenv("JWT_EXPIRY")
	if jwtExpiry == "" {
		cfg.JWTExpiry = 24 * time.Hour
	} else {
		duration, err := time.ParseDuration(jwtExpiry)
		if err != nil {
			return Config{}, fmt.Errorf("invalid JWT_EXPIRY: %w", err)
		}
		cfg.JWTExpiry = duration
	}

	// ---------------- SCRAPE_INTERVAL ----------------
	interval := os.Getenv("SCRAPE_INTERVAL")

	if interval == "" {
		cfg.ScrapeInterval = time.Minute
	} else {
		duration, err := time.ParseDuration(interval)
		if err != nil {
			return Config{}, fmt.Errorf("invalid SCRAPE_INTERVAL: %w", err)
		}
		cfg.ScrapeInterval = duration
	}

	// ---------------- MAX_CONCURRENCY ----------------
	concurrency := os.Getenv("MAX_CONCURRENCY")

	if concurrency == "" {
		cfg.MaxConcurrency = 5
	} else {
		value, err := strconv.Atoi(concurrency)
		if err != nil {
			return Config{}, fmt.Errorf("invalid MAX_CONCURRENCY: %w", err)
		}
		cfg.MaxConcurrency = value
	}

	// ---------------- QUEUE_WORKERS ----------------
	queueWorkers := os.Getenv("QUEUE_WORKERS")
	if queueWorkers == "" {
		cfg.QueueWorkers = 4
	} else {
		value, err := strconv.Atoi(queueWorkers)
		if err != nil {
			return Config{}, fmt.Errorf("invalid QUEUE_WORKERS: %w", err)
		}
		cfg.QueueWorkers = value
	}

	// ---------------- ACCESS_TOKEN_EXPIRY ----------------
	access := os.Getenv("ACCESS_TOKEN_EXPIRY")

	if access == "" {
		cfg.AccessTokenExpiry = 15 * time.Minute
	} else {
		duration, err := time.ParseDuration(access)
		if err != nil {
			return Config{}, fmt.Errorf("invalid ACCESS_TOKEN_EXPIRY: %w", err)
		}
		cfg.AccessTokenExpiry = duration
	}

	// ---------------- REFRESH_TOKEN_EXPIRY ----------------
	refresh := os.Getenv("REFRESH_TOKEN_EXPIRY")

	if refresh == "" {
		cfg.RefreshTokenExpiry = 7 * 24 * time.Hour
	} else {
		duration, err := time.ParseDuration(refresh)
		if err != nil {
			return Config{}, fmt.Errorf("invalid REFRESH_TOKEN_EXPIRY: %w", err)
		}
		cfg.RefreshTokenExpiry = duration
	}

	// ---------------- CACHE_TTL ----------------
	cacheTTL := os.Getenv("CACHE_TTL")
	if cacheTTL == "" {
		cfg.CacheTTL = 5 * time.Minute
	} else {
		duration, err := time.ParseDuration(cacheTTL)
		if err != nil {
			return Config{}, fmt.Errorf("invalid CACHE_TTL: %w", err)
		}
		cfg.CacheTTL = duration
	}

	// ---------------- DB_QUERY_TIMEOUT ----------------
	dbQueryTimeout := os.Getenv("DB_QUERY_TIMEOUT")
	if dbQueryTimeout == "" {
		cfg.DBQueryTimeout = 5 * time.Second
	} else {
		duration, err := time.ParseDuration(dbQueryTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("invalid DB_QUERY_TIMEOUT: %w", err)
		}
		cfg.DBQueryTimeout = duration
	}

	// ---------------- LLM_REQUEST_TIMEOUT ----------------
	llmRequestTimeout := os.Getenv("LLM_REQUEST_TIMEOUT")
	if llmRequestTimeout == "" {
		cfg.LLMRequestTimeout = 30 * time.Second
	} else {
		duration, err := time.ParseDuration(llmRequestTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("invalid LLM_REQUEST_TIMEOUT: %w", err)
		}
		cfg.LLMRequestTimeout = duration
	}

	// ---------------- AI_REQUEST_TIMEOUT ----------------
	aiRequestTimeout := os.Getenv("AI_REQUEST_TIMEOUT")
	if aiRequestTimeout == "" {
		cfg.AIRequestTimeout = 60 * time.Second
	} else {
		duration, err := time.ParseDuration(aiRequestTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("invalid AI_REQUEST_TIMEOUT: %w", err)
		}
		cfg.AIRequestTimeout = duration
	}

	// ---------------- VALIDATE ----------------
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
