package registration

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPythonRunnerRunReturnsFatalErrorMessageOnNonZeroExit(t *testing.T) {
	repoRoot := createTestRepoRoot(t)
	pythonExecutable := createFakePythonExecutable(t, `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"fatal","success":false,"error_message":"business exploded"}'
exit 1
`)

	runner := &PythonRunner{
		pythonExecutable: pythonExecutable,
		repoRoot:         repoRoot,
	}

	_, err := runner.Run(context.Background(), RunnerRequest{
		TaskUUID: "task-fatal",
		StartRequest: StartRequest{
			EmailServiceType: "tempmail",
		},
	}, nil)
	if err == nil {
		t.Fatal("expected fatal error")
	}
	if !strings.Contains(err.Error(), "business exploded") {
		t.Fatalf("expected fatal error message to be preserved, got %v", err)
	}
}

func TestPythonRunnerRunHappyPath(t *testing.T) {
	repoRoot := createTestRepoRoot(t)
	pythonExecutable := createFakePythonExecutable(t, `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"log","level":"info","message":"bridge started"}'
printf '%s\n' '{"type":"result","success":true,"result":{"status":"completed","count":1}}'
`)

	runner := &PythonRunner{
		pythonExecutable: pythonExecutable,
		repoRoot:         repoRoot,
	}

	var logs []string
	result, err := runner.Run(context.Background(), RunnerRequest{
		TaskUUID: "task-happy",
		StartRequest: StartRequest{
			EmailServiceType: "tempmail",
		},
	}, func(level string, message string) error {
		logs = append(logs, level+":"+message)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]any{
		"status": "completed",
		"count":  float64(1),
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("unexpected result: got %#v want %#v", result, want)
	}
	if !reflect.DeepEqual(logs, []string{"info:bridge started"}) {
		t.Fatalf("unexpected logs: %#v", logs)
	}
}

func TestPythonRunnerRunStopsWhenControlMarksCancelled(t *testing.T) {
	repoRoot := createTestRepoRoot(t)
	pythonExecutable := createFakePythonExecutable(t, `#!/bin/sh
cat >/dev/null
if [ -z "$CODEX_CONSOLE_RUNNER_CONTROL_PATH" ]; then
  printf '%s\n' '{"type":"fatal","success":false,"error_message":"missing control path"}'
  exit 1
fi
printf '%s\n' '{"type":"log","level":"info","message":"bridge started"}'
while true; do
  status="$(cat "$CODEX_CONSOLE_RUNNER_CONTROL_PATH" 2>/dev/null | tr -d '\r\n')"
  if [ "$status" = "cancelled" ]; then
    printf '%s\n' '{"type":"fatal","success":false,"error_message":"cancelled by control"}'
    exit 1
  fi
  sleep 0.02
done
`)

	runner := &PythonRunner{
		pythonExecutable: pythonExecutable,
		repoRoot:         repoRoot,
	}

	var controlState atomic.Int32
	started := make(chan struct{}, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := runner.Run(ctx, RunnerRequest{
			TaskUUID: "task-cancel",
			StartRequest: StartRequest{
				EmailServiceType: "tempmail",
			},
			control: func(context.Context) (runnerControlState, error) {
				if controlState.Load() == 1 {
					return runnerControlStateCancelled, nil
				}
				return runnerControlStateRunning, nil
			},
		}, func(level string, message string) error {
			if level == "info" && message == "bridge started" {
				select {
				case started <- struct{}{}:
				default:
				}
			}
			return nil
		})
		errCh <- err
	}()

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatalf("runner did not start before timeout: %v", ctx.Err())
	}

	controlState.Store(1)

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected cancelled control error")
		}
		if !strings.Contains(err.Error(), "cancelled by control") {
			t.Fatalf("expected control cancellation error, got %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("runner did not stop after cancel: %v", ctx.Err())
	}
}

func TestPythonRunnerRunWaitsWhileControlIsPausedAndResumes(t *testing.T) {
	repoRoot := createTestRepoRoot(t)
	pythonExecutable := createFakePythonExecutable(t, `#!/bin/sh
cat >/dev/null
if [ -z "$CODEX_CONSOLE_RUNNER_CONTROL_PATH" ]; then
  printf '%s\n' '{"type":"fatal","success":false,"error_message":"missing control path"}'
  exit 1
fi
printf '%s\n' '{"type":"log","level":"info","message":"bridge started"}'
paused_seen=0
while true; do
  status="$(cat "$CODEX_CONSOLE_RUNNER_CONTROL_PATH" 2>/dev/null | tr -d '\r\n')"
  if [ "$status" = "paused" ]; then
    paused_seen=1
  fi
  if [ "$paused_seen" = "1" ] && [ "$status" = "running" ]; then
    printf '%s\n' '{"type":"result","success":true,"result":{"status":"completed","resumed":true}}'
    exit 0
  fi
  sleep 0.02
done
`)

	runner := &PythonRunner{
		pythonExecutable: pythonExecutable,
		repoRoot:         repoRoot,
	}

	var controlState atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	started := make(chan struct{}, 1)
	controlState.Store(1) // paused

	resultCh := make(chan map[string]any, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := runner.Run(ctx, RunnerRequest{
			TaskUUID: "task-pause",
			StartRequest: StartRequest{
				EmailServiceType: "tempmail",
			},
			control: func(context.Context) (runnerControlState, error) {
				switch controlState.Load() {
				case 1:
					return runnerControlStatePaused, nil
				case 2:
					return runnerControlStateCancelled, nil
				default:
					return runnerControlStateRunning, nil
				}
			},
		}, func(level string, message string) error {
			if level == "info" && message == "bridge started" {
				select {
				case started <- struct{}{}:
				default:
				}
			}
			return nil
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatalf("runner did not start before timeout: %v", ctx.Err())
	}

	select {
	case err := <-errCh:
		t.Fatalf("runner should stay paused instead of exiting early: %v", err)
	case result := <-resultCh:
		t.Fatalf("runner should stay paused instead of completing early: %#v", result)
	case <-time.After(150 * time.Millisecond):
	}

	controlState.Store(0)

	select {
	case err := <-errCh:
		t.Fatalf("unexpected runner error after resume: %v", err)
	case result := <-resultCh:
		if result["status"] != "completed" || result["resumed"] != true {
			t.Fatalf("unexpected resumed result: %#v", result)
		}
	case <-ctx.Done():
		t.Fatalf("runner did not finish after resume: %v", ctx.Err())
	}
}

func TestDetectRepoRootUsesEnvironmentVariable(t *testing.T) {
	repoRoot := createTestRepoRoot(t)
	restoreWd := chdirForTest(t, t.TempDir())
	defer restoreWd()

	t.Setenv("REGISTRATION_REPO_ROOT", repoRoot)

	detected, err := detectRepoRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detected != repoRoot {
		t.Fatalf("unexpected repo root: got %q want %q", detected, repoRoot)
	}
}

func TestDetectRepoRootFailsForInvalidEnvironmentVariable(t *testing.T) {
	restoreWd := chdirForTest(t, t.TempDir())
	defer restoreWd()

	t.Setenv("REGISTRATION_REPO_ROOT", filepath.Join(t.TempDir(), "missing-repo"))

	_, err := detectRepoRoot()
	if err == nil {
		t.Fatal("expected detect repo root error")
	}
	if !strings.Contains(err.Error(), "REGISTRATION_REPO_ROOT") {
		t.Fatalf("expected env error, got %v", err)
	}
}

func TestDetectRepoRootFallsBackToCurrentWorkingDirectory(t *testing.T) {
	repoRoot := createTestRepoRoot(t)
	restoreWd := chdirForTest(t, filepath.Join(repoRoot, "backend-go"))
	defer restoreWd()

	t.Setenv("REGISTRATION_REPO_ROOT", "")

	detected, err := detectRepoRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detected != repoRoot {
		t.Fatalf("unexpected repo root: got %q want %q", detected, repoRoot)
	}
}

func createTestRepoRoot(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "backend-go"))
	mustWriteFile(t, filepath.Join(repoRoot, "backend-go", "go.mod"), "module example.com/test\n")
	mustMkdirAll(t, filepath.Join(repoRoot, "src"))
	if resolved, err := filepath.EvalSymlinks(repoRoot); err == nil {
		return resolved
	}
	return repoRoot
}

func createFakePythonExecutable(t *testing.T, script string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-python")
	mustWriteFile(t, path, script)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod fake python executable: %v", err)
	}
	return path
}

func chdirForTest(t *testing.T, dir string) func() {
	t.Helper()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
	return func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore cwd %q: %v", original, err)
		}
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
