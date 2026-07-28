package workcell

const SchemaVersion = 1

type Owner struct {
	ReservationID string  `json:"reservation_id"`
	Session       string  `json:"session"`
	Actor         string  `json:"actor"`
	Host          string  `json:"host"`
	PID           int     `json:"pid"`
	Repository    string  `json:"repository,omitempty"`
	Branch        string  `json:"branch,omitempty"`
	CWD           string  `json:"cwd"`
	Command       string  `json:"command"`
	StartedAt     string  `json:"started_at"`
	Elapsed       float64 `json:"elapsed_seconds"`
	LogPath       string  `json:"log_path"`
}

type NextAction struct {
	Kind     string   `json:"kind"`
	WaitArgv []string `json:"wait_argv"`
}

type DurationEstimate struct {
	SampleCount               int      `json:"sample_count"`
	AverageSeconds            float64  `json:"average_seconds"`
	EstimatedRemainingSeconds *float64 `json:"estimated_remaining_seconds,omitempty"`
}

type BusyResult struct {
	SchemaVersion    int               `json:"schema_version"`
	Resource         string            `json:"resource"`
	Decision         string            `json:"decision"`
	Owner            *Owner            `json:"owner,omitempty"`
	QueueAhead       int               `json:"queue_ahead"`
	DurationEstimate *DurationEstimate `json:"duration_estimate,omitempty"`
	NextAction       NextAction        `json:"next_action"`
}

type CompletedResult struct {
	SchemaVersion    int               `json:"schema_version"`
	Resource         string            `json:"resource"`
	Decision         string            `json:"decision"`
	ReservationID    string            `json:"reservation_id"`
	Session          string            `json:"session"`
	ExitCode         int               `json:"exit_code"`
	Waited           float64           `json:"waited_seconds"`
	Ran              float64           `json:"ran_seconds"`
	LogPath          string            `json:"log_path"`
	DurationEstimate *DurationEstimate `json:"duration_estimate,omitempty"`
}

type StatusResult struct {
	SchemaVersion    int               `json:"schema_version"`
	Resource         string            `json:"resource"`
	Decision         string            `json:"decision"`
	Owner            *Owner            `json:"owner,omitempty"`
	QueueAhead       int               `json:"queue_ahead"`
	DurationEstimate *DurationEstimate `json:"duration_estimate,omitempty"`
}
