// Package exec provides the execution service for route scripts.
// It handles script creation, execution with timeout support, and result capture.
package exec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/s-z-z/web-exec/internal/config"
)

// ExecResult holds the output and metadata from a script execution.
type ExecResult struct {
	// Stdout contains the standard output from the script.
	Stdout string `json:"stdout"`
	// Stderr contains the standard error from the script.
	Stderr string `json:"stderr"`
	// ExitCode is the exit code from the script. -1 indicates a signal or timeout.
	ExitCode int `json:"exitCode"`
	// Duration is the execution time in milliseconds.
	Duration int64 `json:"durationMs"`
	// Error contains an error message if execution failed (non-exit-code errors).
	Error string `json:"error,omitempty"`
}

// ExecService handles script execution for configured routes.
type ExecService struct {
	store   *config.ConfigStore
	timeout time.Duration
}

// NewExecService creates a new ExecService with the given ConfigStore.
// It defaults to a 30-second script execution timeout.
func NewExecService(store *config.ConfigStore) *ExecService {
	return &ExecService{
		store:   store,
		timeout: 30 * time.Second,
	}
}

// SetTimeout overrides the default script execution timeout.
func (s *ExecService) SetTimeout(d time.Duration) {
	s.timeout = d
}

// HandleExec processes a request by looking up the route, writing the script
// to a temporary file, executing it, and returning the result.
func (s *ExecService) HandleExec(ctx context.Context, routePath string, requestBody []byte) (*ExecResult, error) {
	// 1. Lookup route
	cfg, ok := s.store.GetByPath(routePath)
	if !ok {
		return nil, fmt.Errorf("route not found: %s", routePath)
	}

	// 2. Write script to temporary file
	tmpDir := filepath.Join(cfg.WorkDir, ".web-exec")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, fmt.Errorf("create temp directory: %w", err)
	}

	tmpExt := "*.sh"
	if runtime.GOOS == "windows" {
		tmpExt = "*.bat"
	}
	tmpFile, err := os.CreateTemp(tmpDir, "script-"+tmpExt)
	tmpPath := tmpFile.Name()

	// Ensure cleanup
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.WriteString(cfg.Script); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("write script: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}

	// 3. Execute script with timeout
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", tmpPath)
	} else {
		cmd = exec.CommandContext(ctx, "sh", tmpPath)
	}
	cmd.Dir = cfg.WorkDir

	// Pass request body as stdin
	if len(requestBody) > 0 {
		cmd.Stdin = bytes.NewReader(requestBody)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start).Milliseconds()

	result := &ExecResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Duration: duration,
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
			result.Error = "execution timeout"
			return result, nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			result.Error = err.Error()
		}
		return result, nil
	}

	result.ExitCode = 0
	return result, nil
}
