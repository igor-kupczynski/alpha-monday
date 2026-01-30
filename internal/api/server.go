package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/igor-kupczynski/alpha-monday/internal/db"
	"log/slog"
)

type Server struct {
	store  *db.Store
	logger *slog.Logger
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	dbOK := true
	if err := s.store.Ping(ctx); err != nil {
		dbOK = false
		s.logger.Warn("health check failed", "error", err)
	}

	status := http.StatusOK
	if !dbOK {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, healthResponse{Ok: dbOK, DBOk: dbOK})
}

func (s *Server) handleLatest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	latest, err := s.store.LatestBatch(ctx)
	if err != nil {
		s.logger.Error("latest batch query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "unexpected error")
		return
	}

	if latest == nil {
		writeJSON(w, http.StatusOK, latestResponse{
			Batch:            nil,
			Picks:            []pickResponse{},
			LatestCheckpoint: nil,
		})
		return
	}

	resp := latestResponse{
		Batch:            toBatchResponsePtr(latest.Batch),
		Picks:            toPickResponses(latest.Picks),
		LatestCheckpoint: toCheckpointResponse(latest.LatestCheckpoint),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleBatches(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return
	}

	cursor, err := parseCursor(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	page, err := s.store.ListBatches(ctx, limit, cursor)
	if err != nil {
		s.logger.Error("list batches failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "unexpected error")
		return
	}

	resp := batchesResponse{
		Batches:    toBatchResponses(page.Batches),
		NextCursor: page.NextCursor,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleBatchDetails(w http.ResponseWriter, r *http.Request) {
	batchID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(batchID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", "invalid batch id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	detail, err := s.store.BatchDetails(ctx, batchID)
	if err != nil {
		s.logger.Error("batch detail failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "unexpected error")
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "not_found", "batch not found")
		return
	}

	resp := batchDetailResponse{
		Batch:       toBatchResponse(detail.Batch),
		Picks:       toPickResponses(detail.Picks),
		Checkpoints: toCheckpointResponses(detail.Checkpoints),
	}

	writeJSON(w, http.StatusOK, resp)
}

func parseLimit(r *http.Request) (int, error) {
	value := r.URL.Query().Get("limit")
	if value == "" {
		return 20, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, errInvalidLimit
	}
	if parsed < 1 || parsed > 100 {
		return 0, errInvalidLimit
	}
	return parsed, nil
}

func parseCursor(r *http.Request) (*string, error) {
	value := r.URL.Query().Get("cursor")
	if value == "" {
		return nil, nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return nil, errInvalidCursor
	}
	return &value, nil
}

var (
	errInvalidLimit  = &paramError{"limit must be between 1 and 100"}
	errInvalidCursor = &paramError{"cursor must be YYYY-MM-DD"}
)

type paramError struct {
	message string
}

func (e *paramError) Error() string {
	return e.message
}
