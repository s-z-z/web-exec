package history

import "time"

// ExecRecord represents a single execution of a route script.
type ExecRecord struct {
	ID         string    `json:"id"`
	RouteID    string    `json:"routeId"`
	RoutePath  string    `json:"routePath"`
	Trigger    string    `json:"trigger"`
	Stdout     string    `json:"stdout"`
	Stderr     string    `json:"stderr"`
	ExitCode   int       `json:"exitCode"`
	DurationMs int64     `json:"durationMs"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}
