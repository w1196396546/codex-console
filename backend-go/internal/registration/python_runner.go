package registration

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type PythonRunnerOptions struct {
	PythonExecutable string
	RepoRoot         string
}

type PythonRunner struct {
	pythonExecutable string
	repoRoot         string
}

type pythonRunnerInput struct {
	TaskUUID             string        `json:"task_uuid"`
	StartRequest         StartRequest  `json:"start_request"`
	Plan                 ExecutionPlan `json:"plan"`
	GoPersistenceEnabled bool          `json:"go_persistence_enabled"`
}

type pythonRunnerEvent struct {
	Type               string         `json:"type"`
	Level              string         `json:"level"`
	Message            string         `json:"message"`
	Success            bool           `json:"success"`
	Result             map[string]any `json:"result"`
	AccountPersistence map[string]any `json:"account_persistence"`
	ErrorMessage       string         `json:"error_message"`
}

const registrationRepoRootEnvVar = "REGISTRATION_REPO_ROOT"
const pythonRunnerControlPathEnvVar = "CODEX_CONSOLE_RUNNER_CONTROL_PATH"
const pythonRunnerControlPollInterval = 50 * time.Millisecond
const pythonRunnerControlCancelGracePeriod = 200 * time.Millisecond

func NewPythonRunner(options PythonRunnerOptions) (*PythonRunner, error) {
	pythonExecutable := strings.TrimSpace(options.PythonExecutable)
	if pythonExecutable == "" {
		var err error
		pythonExecutable, err = resolvePythonExecutable()
		if err != nil {
			return nil, err
		}
	}

	repoRoot := strings.TrimSpace(options.RepoRoot)
	if repoRoot == "" {
		var err error
		repoRoot, err = detectRepoRoot()
		if err != nil {
			return nil, err
		}
	}

	return &PythonRunner{
		pythonExecutable: pythonExecutable,
		repoRoot:         repoRoot,
	}, nil
}

func (r *PythonRunner) Run(ctx context.Context, req RunnerRequest, logf func(level string, message string) error) (map[string]any, error) {
	if r == nil {
		return nil, errors.New("python runner is required")
	}
	if logf == nil {
		logf = func(string, string) error { return nil }
	}

	scriptPath, err := r.writeScriptFile()
	if err != nil {
		return nil, err
	}
	defer os.Remove(scriptPath)

	processCtx := ctx
	cancelProcess := func() {}

	var controlFile *pythonRunnerControlFile
	if req.control != nil {
		controlFile, err = newPythonRunnerControlFile(runnerControlStateRunning)
		if err != nil {
			return nil, err
		}
		defer controlFile.close()

		processCtx, cancelProcess = context.WithCancel(ctx)
		defer cancelProcess()
	}

	cmd := exec.CommandContext(processCtx, r.pythonExecutable, scriptPath)
	cmd.Dir = r.repoRoot
	if controlFile != nil {
		cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", pythonRunnerControlPathEnvVar, controlFile.path))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open python runner stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open python runner stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open python runner stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start python runner: %w", err)
	}

	var (
		mu          sync.Mutex
		finalEvent  *pythonRunnerEvent
		callbackErr error
	)
	setCallbackErr := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if callbackErr == nil {
			callbackErr = err
		}
	}
	setFinalEvent := func(event pythonRunnerEvent) {
		copyEvent := event
		mu.Lock()
		finalEvent = &copyEvent
		mu.Unlock()
	}

	processDone := make(chan struct{})
	if controlFile != nil {
		go func() {
			ticker := time.NewTicker(pythonRunnerControlPollInterval)
			defer ticker.Stop()

			state := runnerControlStateRunning
			cancelledAt := time.Time{}

			for {
				nextState, err := req.control(ctx)
				if err != nil {
					setCallbackErr(fmt.Errorf("observe python runner control: %w", err))
					cancelProcess()
					return
				}
				if nextState == "" {
					nextState = runnerControlStateRunning
				}
				if nextState != state {
					if err := controlFile.write(nextState); err != nil {
						setCallbackErr(fmt.Errorf("write python runner control: %w", err))
						cancelProcess()
						return
					}
					state = nextState
					if state == runnerControlStateCancelled {
						cancelledAt = time.Now()
					} else {
						cancelledAt = time.Time{}
					}
				}
				if state == runnerControlStateCancelled && !cancelledAt.IsZero() && time.Since(cancelledAt) >= pythonRunnerControlCancelGracePeriod {
					cancelProcess()
					return
				}

				select {
				case <-processDone:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}

	stdoutErrCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var event pythonRunnerEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil || event.Type == "" {
				setCallbackErr(logf("info", line))
				continue
			}

			switch event.Type {
			case "log":
				level := strings.TrimSpace(event.Level)
				if level == "" {
					level = "info"
				}
				setCallbackErr(logf(level, event.Message))
			case "result", "fatal":
				setFinalEvent(event)
			default:
				setCallbackErr(logf("info", line))
			}
		}
		stdoutErrCh <- scanner.Err()
	}()

	stderrErrCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 32*1024), 512*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			setCallbackErr(logf("warning", line))
		}
		stderrErrCh <- scanner.Err()
	}()

	encodeErr := json.NewEncoder(stdin).Encode(pythonRunnerInput{
		TaskUUID:             req.TaskUUID,
		StartRequest:         req.StartRequest,
		Plan:                 req.Plan,
		GoPersistenceEnabled: req.GoPersistenceEnabled,
	})
	_ = stdin.Close()
	if encodeErr != nil {
		_ = cmd.Process.Kill()
		<-stdoutErrCh
		<-stderrErrCh
		return nil, fmt.Errorf("encode python runner payload: %w", encodeErr)
	}

	waitErr := cmd.Wait()
	close(processDone)
	stdoutErr := <-stdoutErrCh
	stderrErr := <-stderrErrCh

	if stdoutErr != nil {
		return nil, fmt.Errorf("read python runner stdout: %w", stdoutErr)
	}
	if stderrErr != nil {
		return nil, fmt.Errorf("read python runner stderr: %w", stderrErr)
	}

	mu.Lock()
	defer mu.Unlock()
	if callbackErr != nil {
		return nil, callbackErr
	}
	if finalEvent == nil {
		if waitErr != nil {
			return nil, fmt.Errorf("python runner failed: %w", waitErr)
		}
		return nil, errors.New("python runner did not return a result")
	}
	if finalEvent.Type == "fatal" {
		return nil, pythonRunnerEventError(finalEvent, waitErr, "python runner failed")
	}
	if !finalEvent.Success {
		return nil, pythonRunnerEventError(finalEvent, waitErr, "registration failed")
	}
	if finalEvent.Result == nil {
		return nil, errors.New("python runner returned empty result")
	}
	if len(finalEvent.AccountPersistence) > 0 {
		finalEvent.Result[runnerAccountPersistenceResultKey] = finalEvent.AccountPersistence
	}

	return finalEvent.Result, nil
}

type pythonRunnerControlFile struct {
	path string
}

func newPythonRunnerControlFile(initial runnerControlState) (*pythonRunnerControlFile, error) {
	file, err := os.CreateTemp("", "codex-console-registration-control-*.state")
	if err != nil {
		return nil, fmt.Errorf("create python runner control file: %w", err)
	}

	controlFile := &pythonRunnerControlFile{path: file.Name()}
	if _, err := file.WriteString(string(initial)); err != nil {
		_ = file.Close()
		controlFile.close()
		return nil, fmt.Errorf("write python runner control file: %w", err)
	}
	if err := file.Close(); err != nil {
		controlFile.close()
		return nil, fmt.Errorf("close python runner control file: %w", err)
	}

	return controlFile, nil
}

func (f *pythonRunnerControlFile) write(state runnerControlState) error {
	if f == nil {
		return nil
	}
	return os.WriteFile(f.path, []byte(state), 0o600)
}

func (f *pythonRunnerControlFile) close() {
	if f == nil || strings.TrimSpace(f.path) == "" {
		return
	}
	_ = os.Remove(f.path)
}

func pythonRunnerEventError(event *pythonRunnerEvent, waitErr error, fallback string) error {
	if event == nil {
		if waitErr != nil {
			return fmt.Errorf("python runner failed: %w", waitErr)
		}
		return errors.New(fallback)
	}

	message := strings.TrimSpace(event.ErrorMessage)
	if message == "" {
		message = strings.TrimSpace(event.Message)
	}
	if message == "" {
		message = fallback
	}
	if waitErr != nil {
		return fmt.Errorf("%s (process exit: %w)", message, waitErr)
	}
	return errors.New(message)
}

func (r *PythonRunner) writeScriptFile() (string, error) {
	file, err := os.CreateTemp("", "codex-console-registration-runner-*.py")
	if err != nil {
		return "", fmt.Errorf("create python runner script: %w", err)
	}
	if _, err := file.WriteString(pythonRunnerScript); err != nil {
		file.Close()
		return "", fmt.Errorf("write python runner script: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close python runner script: %w", err)
	}
	return file.Name(), nil
}

func resolvePythonExecutable() (string, error) {
	candidates := []string{
		strings.TrimSpace(os.Getenv("REGISTRATION_PYTHON_EXECUTABLE")),
		strings.TrimSpace(os.Getenv("PYTHON")),
		"python3",
		"python",
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("python executable not found")
}

func detectRepoRoot() (string, error) {
	if configuredRoot := strings.TrimSpace(os.Getenv(registrationRepoRootEnvVar)); configuredRoot != "" {
		repoRoot, err := validateRepoRoot(configuredRoot)
		if err != nil {
			return "", fmt.Errorf("validate %s: %w", registrationRepoRootEnvVar, err)
		}
		return repoRoot, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("detect repo root: %w", err)
	}

	current := cwd
	for {
		if looksLikeRepoRoot(current) {
			return normalizeRepoRoot(current), nil
		}
		if looksLikeBackendGoDir(current) {
			return normalizeRepoRoot(filepath.Dir(current)), nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", errors.New("could not detect codex-console repository root")
}

func validateRepoRoot(path string) (string, error) {
	repoRoot := normalizeRepoRoot(path)
	if !looksLikeRepoRoot(repoRoot) {
		return "", errors.New("expected backend-go/go.mod and src directory")
	}
	return repoRoot, nil
}

func looksLikeRepoRoot(path string) bool {
	return fileExists(filepath.Join(path, "backend-go", "go.mod")) && dirExists(filepath.Join(path, "src"))
}

func looksLikeBackendGoDir(path string) bool {
	return filepath.Base(path) == "backend-go" &&
		fileExists(filepath.Join(path, "go.mod")) &&
		dirExists(filepath.Join(filepath.Dir(path), "src"))
}

func normalizeRepoRoot(path string) string {
	normalized := strings.TrimSpace(path)
	if normalized == "" {
		return normalized
	}
	if absPath, err := filepath.Abs(normalized); err == nil {
		normalized = absPath
	}
	if resolvedPath, err := filepath.EvalSymlinks(normalized); err == nil {
		normalized = resolvedPath
	}
	return normalized
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
