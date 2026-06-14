package types

type ConsumerWorkerSnapshot struct {
	TotalErrors        uint64 `json:"total_errors"`
	ConsecutiveErrors  uint64 `json:"consecutive_errors"`
	LastErrorAtMS      int64  `json:"last_error_at_ms"`
	LastSuccessAtMS    int64  `json:"last_success_at_ms"`
	LastCommitAtMS     int64  `json:"last_commit_at_ms"`
	LastErrorBackoffMS int64  `json:"last_error_backoff_ms"`
}

type RedisSubscriberWorkerSnapshot struct {
	TotalErrors        uint64 `json:"total_errors"`
	ConsecutiveErrors  uint64 `json:"consecutive_errors"`
	LastErrorAtMS      int64  `json:"last_error_at_ms"`
	LastSuccessAtMS    int64  `json:"last_success_at_ms"`
	LastErrorBackoffMS int64  `json:"last_error_backoff_ms"`
}
