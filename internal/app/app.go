package app

import (
	"context"
	"database/sql"
	"log"
	"log/slog"
	"net/http"
	"time"

	_ "github.com/lib/pq"
	redis "github.com/redis/go-redis/v9"
	"github.com/shreyasreddy45/RSS-server/internal/aiclient"
	"github.com/shreyasreddy45/RSS-server/internal/config"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	redisdb "github.com/shreyasreddy45/RSS-server/internal/redis"
)

// App holds the shared dependencies that used to live on the package-level
// `apiConfig` struct in main.go. It is passed explicitly to routers,
// middleware, and handlers instead of being accessed via methods.
type App struct {
	DB     *database.Queries
	SQLDB  *sql.DB
	Redis  *redis.Client
	Config config.Config
	Logger *slog.Logger
	// AI is the HTTP client for the Python AI service. It is nil when
	// AI_SERVICE_URL is not configured, in which case AI features are disabled.
	AI *aiclient.Client
}

// AuthedHandler is a handler that has already been resolved to an
// authenticated database.User by the auth/JWT middleware.
type AuthedHandler func(http.ResponseWriter, *http.Request, database.User)

// New connects to Postgres and Redis and assembles the App.
func New(cfg config.Config, logger *slog.Logger) *App {

	conn := connectDatabase(cfg.DBURL)

	redisClient := redisdb.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := redisClient.Ping(ctx).Err()
	if err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	logger.Info("Connected to Redis")

	var aiClient *aiclient.Client
	if cfg.AIServiceURL != "" {
		aiClient = aiclient.New(cfg.AIServiceURL, cfg.AIServiceToken)
		logger.Info("AI service configured", "url", cfg.AIServiceURL)
	} else {
		logger.Warn("AI_SERVICE_URL is not set; summaries, semantic search, Ask and the agent are disabled")
	}

	return &App{
		DB:     database.New(conn),
		SQLDB:  conn,
		Redis:  redisClient,
		Config: cfg,
		Logger: logger,
		AI:     aiClient,
	}
}

func connectDatabase(dbURL string) *sql.DB {

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Failed to connect database:", err)
	}

	return conn
}
