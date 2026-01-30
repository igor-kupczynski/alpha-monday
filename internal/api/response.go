package api

import "github.com/igor-kupczynski/alpha-monday/internal/db"

type healthResponse struct {
	Ok   bool `json:"ok"`
	DBOk bool `json:"db_ok"`
}

type batchResponse struct {
	ID                    string `json:"id"`
	RunDate               string `json:"run_date"`
	Status                string `json:"status"`
	BenchmarkSymbol       string `json:"benchmark_symbol"`
	BenchmarkInitialPrice string `json:"benchmark_initial_price"`
}

type pickResponse struct {
	ID           string `json:"id"`
	Ticker       string `json:"ticker"`
	Action       string `json:"action"`
	Reasoning    string `json:"reasoning"`
	InitialPrice string `json:"initial_price"`
}

type pickMetricResponse struct {
	ID                string `json:"id"`
	PickID            string `json:"pick_id"`
	CurrentPrice      string `json:"current_price"`
	AbsoluteReturnPct string `json:"absolute_return_pct"`
	VsBenchmarkPct    string `json:"vs_benchmark_pct"`
}

type checkpointResponse struct {
	ID                 string               `json:"id"`
	CheckpointDate     string               `json:"checkpoint_date"`
	Status             string               `json:"status"`
	BenchmarkPrice     *string              `json:"benchmark_price"`
	BenchmarkReturnPct *string              `json:"benchmark_return_pct"`
	Metrics            []pickMetricResponse `json:"metrics"`
}

type latestResponse struct {
	Batch            *batchResponse      `json:"batch"`
	Picks            []pickResponse      `json:"picks"`
	LatestCheckpoint *checkpointResponse `json:"latest_checkpoint"`
}

type batchesResponse struct {
	Batches    []batchResponse `json:"batches"`
	NextCursor *string         `json:"next_cursor"`
}

type batchDetailResponse struct {
	Batch       batchResponse        `json:"batch"`
	Picks       []pickResponse       `json:"picks"`
	Checkpoints []checkpointResponse `json:"checkpoints"`
}

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func toBatchResponse(batch db.Batch) batchResponse {
	return batchResponse{
		ID:                    batch.ID,
		RunDate:               batch.RunDate,
		Status:                batch.Status,
		BenchmarkSymbol:       batch.BenchmarkSymbol,
		BenchmarkInitialPrice: batch.BenchmarkInitialPrice,
	}
}

func toBatchResponsePtr(batch db.Batch) *batchResponse {
	resp := toBatchResponse(batch)
	return &resp
}

func toBatchResponses(batches []db.Batch) []batchResponse {
	if len(batches) == 0 {
		return []batchResponse{}
	}
	result := make([]batchResponse, 0, len(batches))
	for _, batch := range batches {
		result = append(result, toBatchResponse(batch))
	}
	return result
}

func toPickResponses(picks []db.Pick) []pickResponse {
	if len(picks) == 0 {
		return []pickResponse{}
	}
	result := make([]pickResponse, 0, len(picks))
	for _, pick := range picks {
		result = append(result, pickResponse{
			ID:           pick.ID,
			Ticker:       pick.Ticker,
			Action:       pick.Action,
			Reasoning:    pick.Reasoning,
			InitialPrice: pick.InitialPrice,
		})
	}
	return result
}

func toCheckpointResponse(checkpoint *db.Checkpoint) *checkpointResponse {
	if checkpoint == nil {
		return nil
	}
	resp := checkpointResponse{
		ID:                 checkpoint.ID,
		CheckpointDate:     checkpoint.CheckpointDate,
		Status:             checkpoint.Status,
		BenchmarkPrice:     checkpoint.BenchmarkPrice,
		BenchmarkReturnPct: checkpoint.BenchmarkReturnPct,
		Metrics:            toMetricResponses(checkpoint.Metrics),
	}
	return &resp
}

func toCheckpointResponses(checkpoints []db.Checkpoint) []checkpointResponse {
	if len(checkpoints) == 0 {
		return []checkpointResponse{}
	}
	result := make([]checkpointResponse, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		result = append(result, checkpointResponse{
			ID:                 checkpoint.ID,
			CheckpointDate:     checkpoint.CheckpointDate,
			Status:             checkpoint.Status,
			BenchmarkPrice:     checkpoint.BenchmarkPrice,
			BenchmarkReturnPct: checkpoint.BenchmarkReturnPct,
			Metrics:            toMetricResponses(checkpoint.Metrics),
		})
	}
	return result
}

func toMetricResponses(metrics []db.PickMetric) []pickMetricResponse {
	if len(metrics) == 0 {
		return []pickMetricResponse{}
	}
	result := make([]pickMetricResponse, 0, len(metrics))
	for _, metric := range metrics {
		result = append(result, pickMetricResponse{
			ID:                metric.ID,
			PickID:            metric.PickID,
			CurrentPrice:      metric.CurrentPrice,
			AbsoluteReturnPct: metric.AbsoluteReturnPct,
			VsBenchmarkPct:    metric.VsBenchmarkPct,
		})
	}
	return result
}
