package jobs

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusPaused    = "paused"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

type Job struct {
	JobID     string
	JobType   string
	ScopeType string
	ScopeID   string
	Status    string
	Payload   []byte
}

type CreateJobParams struct {
	JobType   string
	ScopeType string
	ScopeID   string
	Payload   []byte
}
