package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/igor-kupczynski/alpha-monday/internal/db"
	"log/slog"
)

const (
	readTimeout  = 10 * time.Second
	writeTimeout = 10 * time.Second
	idleTimeout  = 60 * time.Second
)

func NewRouter(store *db.Store, logger *slog.Logger, corsOrigins []string) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	server := &Server{store: store, logger: logger}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Second))
	r.Use(requestLogger(logger))

	if len(corsOrigins) > 0 {
		r.Use(cors.New(cors.Options{
			AllowedOrigins: corsOrigins,
			AllowedMethods: []string{"GET", "OPTIONS"},
			AllowedHeaders: []string{"Accept", "Content-Type"},
			MaxAge:         300,
		}).Handler)
	}

	r.Get("/health", server.handleHealth)
	r.Get("/latest", server.handleLatest)
	r.Get("/batches", server.handleBatches)
	r.Get("/batches/{id}", server.handleBatchDetails)

	return r
}

func NewHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}
}
