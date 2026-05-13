package exec

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/s-z-z/web-exec/internal/config"
)

func echoScript() string {
	if runtime.GOOS == "windows" {
		return "@echo hello"
	}
	return "echo hello"
}

func sleepScript() string {
	if runtime.GOOS == "windows" {
		return "timeout /t 5 /nobreak >nul"
	}
	return "sleep 5"
}

func stdinScript() string {
	if runtime.GOOS == "windows" {
		// On Windows cmd, there's no simple cat equivalent for stdin.
		// Use a PowerShell-based approach as the script content.
		return "powershell -Command \"$input | Write-Output\""
	}
	return "cat -"
}

func pwdScript() string {
	if runtime.GOOS == "windows" {
		return "@echo %cd%"
	}
	return "pwd"
}

func TestExecService_HandleExec(t *testing.T) {
	store := config.NewConfigStore("")
	store.Add(context.Background(), &config.RouteConfig{
		Path:    "/test",
		WorkDir: t.TempDir(),
		Script:  echoScript(),
	})

	svc := NewExecService(store)
	res, err := svc.HandleExec(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "hello") {
		t.Errorf("expected stdout to contain 'hello', got: %s", res.Stdout)
	}
}

func TestExecService_RouteNotFound(t *testing.T) {
	store := config.NewConfigStore("")
	svc := NewExecService(store)

	_, err := svc.HandleExec(context.Background(), "/nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent route")
	}
}

func TestExecService_Timeout(t *testing.T) {
	store := config.NewConfigStore("")
	store.Add(context.Background(), &config.RouteConfig{
		Path:    "/sleep",
		WorkDir: t.TempDir(),
		Script:  sleepScript(),
	})

	svc := NewExecService(store)
	svc.SetTimeout(100 * time.Millisecond)

	res, err := svc.HandleExec(context.Background(), "/sleep", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != -1 {
		t.Errorf("expected exit code -1 for timeout, got %d", res.ExitCode)
	}
	if res.Error != "execution timeout" {
		t.Errorf("expected timeout error, got: %s", res.Error)
	}
}

func TestExecService_Stdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Stdin piping with PowerShell is complex; skip on Windows for now.
		t.Skip("stdin test skipped on Windows (complex PowerShell piping)")
	}

	store := config.NewConfigStore("")
	store.Add(context.Background(), &config.RouteConfig{
		Path:    "/stdin",
		WorkDir: t.TempDir(),
		Script:  stdinScript(),
	})

	svc := NewExecService(store)
	res, err := svc.HandleExec(context.Background(), "/stdin", []byte("input data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "input data") {
		t.Errorf("expected stdout to contain 'input data', got: %s", res.Stdout)
	}
}

func TestExecService_WorkDir(t *testing.T) {
	store := config.NewConfigStore("")
	workDir := t.TempDir()
	store.Add(context.Background(), &config.RouteConfig{
		Path:    "/pwd",
		WorkDir: workDir,
		Script:  pwdScript(),
	})

	svc := NewExecService(store)
	res, err := svc.HandleExec(context.Background(), "/pwd", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}

	// Normalize paths for comparison (Windows uses backslashes)
	expected := filepath.ToSlash(workDir)
	actual := filepath.ToSlash(strings.TrimSpace(res.Stdout))
	if runtime.GOOS == "windows" {
		// On Windows, %cd% outputs the path; normalize for comparison
		actual = filepath.ToSlash(strings.TrimSpace(res.Stdout))
		// Remove trailing newline/carriage return
		actual = strings.TrimRight(actual, "\r\n")
	}
	if !strings.Contains(actual, expected) {
		t.Errorf("expected stdout to contain workDir %s, got: %s", expected, actual)
	}
}

func TestExecService_TempFileCleanup(t *testing.T) {
	store := config.NewConfigStore("")
	workDir := t.TempDir()
	store.Add(context.Background(), &config.RouteConfig{
		Path:    "/cleanup",
		WorkDir: workDir,
		Script:  echoScript(),
	})

	svc := NewExecService(store)
	_, err := svc.HandleExec(context.Background(), "/cleanup", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify no temp files remain in .web-exec directory
	tmpDir := filepath.Join(workDir, ".web-exec")
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		// Directory may not exist (was never created if MkdirAll failed)
		t.Logf(".web-exec directory does not exist (acceptable)")
		return
	}
	if len(entries) != 0 {
		t.Errorf("expected .web-exec directory to be empty after cleanup, found %d files", len(entries))
	}
}