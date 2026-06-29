package types

type OutboxRelayWorkerSnapshot struct {
	TotalErrors        uint64 `json:"total_errors"`
	ConsecutiveErrors  uint64 `json:"consecutive_errors"`
	TotalFetched       uint64 `json:"total_fetched"`
	TotalPublished     uint64 `json:"total_published"`
	TotalRetried       uint64 `json:"total_retried"`
	TotalDeadLettered  uint64 `json:"total_dead_lettered"`
	WorkerCount        int    `json:"worker_count"`
	LastRunDurationMS  int64  `json:"last_run_duration_ms"`
	LastPublishMS      int64  `json:"last_publish_ms"`
	LastErrorAtMS      int64  `json:"last_error_at_ms"`
	LastSuccessAtMS    int64  `json:"last_success_at_ms"`
	LastPublishedAtMS  int64  `json:"last_published_at_ms"`
	LastErrorBackoffMS int64  `json:"last_error_backoff_ms"`
}

type ProjectionWorkerSnapshot struct {
	TotalErrors        uint64 `json:"total_errors"`
	ConsecutiveErrors  uint64 `json:"consecutive_errors"`
	LastErrorAtMS      int64  `json:"last_error_at_ms"`
	LastSuccessAtMS    int64  `json:"last_success_at_ms"`
	LastCommitAtMS     int64  `json:"last_commit_at_ms"`
	LastErrorBackoffMS int64  `json:"last_error_backoff_ms"`
}
