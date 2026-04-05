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

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
)

type PythonRunnerOptions struct {
	PythonExecutable        string
	RepoRoot                string
	ResolvePythonExecutable func() (string, error)
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

type pythonRunnerLaunchSpec struct {
	command string
	args    []string
	dir     string
	env     []string
	input   pythonRunnerInput
}

const registrationRepoRootEnvVar = "REGISTRATION_REPO_ROOT"
const pythonRunnerControlPathEnvVar = "CODEX_CONSOLE_RUNNER_CONTROL_PATH"
const pythonRunnerScriptEnvVar = "CODEX_CONSOLE_RUNNER_SCRIPT"
const pythonRunnerControlPollInterval = 50 * time.Millisecond
const pythonRunnerControlCancelGracePeriod = 200 * time.Millisecond
const pythonRunnerControlPipePulseInterval = 50 * time.Millisecond
const registrationRunnerErrorPrefix = "registration runner"
const pythonRunnerBootstrap = "import os; exec(compile(os.environ[%q], '<codex-console-registration-runner>', 'exec'))"

func NewPythonRunner(options PythonRunnerOptions) (*PythonRunner, error) {
	pythonExecutable := strings.TrimSpace(options.PythonExecutable)
	if pythonExecutable == "" {
		resolveExecutable := options.ResolvePythonExecutable
		if resolveExecutable == nil {
			resolveExecutable = resolvePythonExecutable
		}
		var err error
		pythonExecutable, err = resolveExecutable()
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

func (r *PythonRunner) Run(ctx context.Context, req RunnerRequest, logf func(level string, message string) error) (RunnerOutput, error) {
	if r == nil {
		return RunnerOutput{}, errors.New(registrationRunnerErrorPrefix + " is required")
	}
	if logf == nil {
		logf = func(string, string) error { return nil }
	}

	processCtx := ctx
	cancelProcess := func() {}
	var err error

	var controlFile *pythonRunnerControlFile
	if req.control != nil {
		controlFile, err = newPythonRunnerControlFile(runnerControlStateRunning)
		if err != nil {
			return RunnerOutput{}, err
		}
		defer controlFile.close()

		processCtx, cancelProcess = context.WithCancel(ctx)
		defer cancelProcess()
	}

	launchSpec, err := r.buildLaunchSpec(req, controlFile)
	if err != nil {
		return RunnerOutput{}, err
	}

	cmd := exec.CommandContext(processCtx, launchSpec.command, launchSpec.args...)
	cmd.Dir = launchSpec.dir
	cmd.Env = append(os.Environ(), launchSpec.env...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return RunnerOutput{}, fmt.Errorf("open %s stdin: %w", registrationRunnerErrorPrefix, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return RunnerOutput{}, fmt.Errorf("open %s stdout: %w", registrationRunnerErrorPrefix, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return RunnerOutput{}, fmt.Errorf("open %s stderr: %w", registrationRunnerErrorPrefix, err)
	}

	if err := cmd.Start(); err != nil {
		return RunnerOutput{}, fmt.Errorf("start %s: %w", registrationRunnerErrorPrefix, err)
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
					setCallbackErr(fmt.Errorf("observe %s control: %w", registrationRunnerErrorPrefix, err))
					cancelProcess()
					return
				}
				if nextState == "" {
					nextState = runnerControlStateRunning
				}
				if nextState != state {
					if err := controlFile.write(nextState); err != nil {
						setCallbackErr(fmt.Errorf("write %s control: %w", registrationRunnerErrorPrefix, err))
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

	encodeErr := json.NewEncoder(stdin).Encode(launchSpec.input)
	_ = stdin.Close()
	if encodeErr != nil {
		_ = cmd.Process.Kill()
		<-stdoutErrCh
		<-stderrErrCh
		return RunnerOutput{}, fmt.Errorf("encode %s payload: %w", registrationRunnerErrorPrefix, encodeErr)
	}

	waitErr := cmd.Wait()
	close(processDone)
	stdoutErr := <-stdoutErrCh
	stderrErr := <-stderrErrCh

	if stdoutErr != nil {
		return RunnerOutput{}, fmt.Errorf("read %s stdout: %w", registrationRunnerErrorPrefix, stdoutErr)
	}
	if stderrErr != nil {
		return RunnerOutput{}, fmt.Errorf("read %s stderr: %w", registrationRunnerErrorPrefix, stderrErr)
	}

	mu.Lock()
	defer mu.Unlock()
	if callbackErr != nil {
		return RunnerOutput{}, callbackErr
	}
	if finalEvent == nil {
		if waitErr != nil {
			return RunnerOutput{}, fmt.Errorf("%s failed: %w", registrationRunnerErrorPrefix, waitErr)
		}
		return RunnerOutput{}, errors.New(registrationRunnerErrorPrefix + " did not return a result")
	}
	if finalEvent.Type == "fatal" {
		return RunnerOutput{}, pythonRunnerEventError(finalEvent, waitErr, registrationRunnerErrorPrefix+" failed")
	}
	if !finalEvent.Success {
		return RunnerOutput{}, pythonRunnerEventError(finalEvent, waitErr, "registration failed")
	}
	if finalEvent.Result == nil {
		return RunnerOutput{}, errors.New(registrationRunnerErrorPrefix + " returned empty result")
	}

	output := RunnerOutput{Result: finalEvent.Result}
	if len(finalEvent.AccountPersistence) > 0 {
		req, err := decodeAccountPersistenceRequest(finalEvent.AccountPersistence)
		if err != nil {
			return RunnerOutput{}, fmt.Errorf("decode runner account persistence payload: %w", err)
		}
		output.AccountPersistence = req
	}

	return output, nil
}

func decodeAccountPersistenceRequest(raw map[string]any) (*accounts.UpsertAccountRequest, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	var req accounts.UpsertAccountRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *PythonRunner) buildLaunchSpec(req RunnerRequest, controlFile *pythonRunnerControlFile) (pythonRunnerLaunchSpec, error) {
	if r == nil {
		return pythonRunnerLaunchSpec{}, errors.New(registrationRunnerErrorPrefix + " is required")
	}

	pythonExecutable := strings.TrimSpace(r.pythonExecutable)
	if pythonExecutable == "" {
		return pythonRunnerLaunchSpec{}, errors.New(registrationRunnerErrorPrefix + " executable is required")
	}

	repoRoot := strings.TrimSpace(r.repoRoot)
	if repoRoot == "" {
		return pythonRunnerLaunchSpec{}, errors.New(registrationRunnerErrorPrefix + " repo root is required")
	}

	validatedRepoRoot, err := validateRepoRoot(repoRoot)
	if err != nil {
		return pythonRunnerLaunchSpec{}, fmt.Errorf("validate %s repo root: %w", registrationRunnerErrorPrefix, err)
	}

	launchSpec := pythonRunnerLaunchSpec{
		command: pythonExecutable,
		args:    []string{"-c", fmt.Sprintf(pythonRunnerBootstrap, pythonRunnerScriptEnvVar)},
		dir:     validatedRepoRoot,
		env: []string{
			fmt.Sprintf("%s=%s", pythonRunnerScriptEnvVar, pythonRunnerScript),
		},
		input: pythonRunnerInput{
			TaskUUID:             req.TaskUUID,
			StartRequest:         req.StartRequest,
			Plan:                 req.Plan,
			GoPersistenceEnabled: req.GoPersistenceEnabled,
		},
	}
	if controlFile != nil && strings.TrimSpace(controlFile.path) != "" {
		launchSpec.env = append(launchSpec.env, fmt.Sprintf("%s=%s", pythonRunnerControlPathEnvVar, controlFile.path))
	}

	return launchSpec, nil
}

type pythonRunnerControlFile struct {
	path    string
	writeFn func(runnerControlState) error
	closeFn func()
}

func newPythonRunnerControlFile(initial runnerControlState) (*pythonRunnerControlFile, error) {
	if controlPipe, err := newPythonRunnerNamedPipeControlFile(initial); err == nil {
		return controlPipe, nil
	}

	return newPythonRunnerStateControlFile(initial)
}

func newPythonRunnerNamedPipeControlFile(initial runnerControlState) (*pythonRunnerControlFile, error) {
	file, err := os.CreateTemp("", "codex-console-registration-control-*.pipe")
	if err != nil {
		return nil, fmt.Errorf("create %s control pipe path: %w", registrationRunnerErrorPrefix, err)
	}

	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close %s control pipe path: %w", registrationRunnerErrorPrefix, err)
	}
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("prepare %s control pipe path: %w", registrationRunnerErrorPrefix, err)
	}

	if err := exec.Command("mkfifo", path).Run(); err != nil {
		return nil, fmt.Errorf("create %s control pipe: %w", registrationRunnerErrorPrefix, err)
	}

	controlPipe := &pythonRunnerNamedPipeControl{
		path:   path,
		state:  initial,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
		pokeCh: make(chan struct{}, 1),
	}
	go controlPipe.serve()
	controlPipe.signal()

	return &pythonRunnerControlFile{
		path:    path,
		writeFn: controlPipe.write,
		closeFn: controlPipe.close,
	}, nil
}

func newPythonRunnerStateControlFile(initial runnerControlState) (*pythonRunnerControlFile, error) {
	file, err := os.CreateTemp("", "codex-console-registration-control-*.state")
	if err != nil {
		return nil, fmt.Errorf("create %s control file: %w", registrationRunnerErrorPrefix, err)
	}

	controlFile := &pythonRunnerControlFile{path: file.Name()}
	if _, err := file.WriteString(string(initial)); err != nil {
		_ = file.Close()
		controlFile.close()
		return nil, fmt.Errorf("write %s control file: %w", registrationRunnerErrorPrefix, err)
	}
	if err := file.Close(); err != nil {
		controlFile.close()
		return nil, fmt.Errorf("close %s control file: %w", registrationRunnerErrorPrefix, err)
	}

	controlFile.writeFn = func(state runnerControlState) error {
		return os.WriteFile(controlFile.path, []byte(state), 0o600)
	}
	controlFile.closeFn = func() {
		if strings.TrimSpace(controlFile.path) == "" {
			return
		}
		_ = os.Remove(controlFile.path)
	}

	return controlFile, nil
}

func (f *pythonRunnerControlFile) write(state runnerControlState) error {
	if f == nil {
		return nil
	}
	if f.writeFn == nil {
		return nil
	}
	return f.writeFn(state)
}

func (f *pythonRunnerControlFile) close() {
	if f == nil || strings.TrimSpace(f.path) == "" {
		return
	}
	if f.closeFn != nil {
		f.closeFn()
	}
}

type pythonRunnerNamedPipeControl struct {
	path     string
	mu       sync.RWMutex
	state    runnerControlState
	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
	pokeCh   chan struct{}
}

func (p *pythonRunnerNamedPipeControl) write(state runnerControlState) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	p.state = state
	p.mu.Unlock()
	p.signal()
	return nil
}

func (p *pythonRunnerNamedPipeControl) currentState() runnerControlState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

func (p *pythonRunnerNamedPipeControl) signal() {
	if p == nil {
		return
	}
	select {
	case p.pokeCh <- struct{}{}:
	default:
	}
}

func (p *pythonRunnerNamedPipeControl) serve() {
	defer close(p.doneCh)

	ticker := time.NewTicker(pythonRunnerControlPipePulseInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-p.pokeCh:
		case <-ticker.C:
		}

		file, err := os.OpenFile(p.path, os.O_WRONLY, 0)
		if err != nil {
			select {
			case <-p.stopCh:
				return
			default:
				continue
			}
		}

		_, writeErr := file.WriteString(string(p.currentState()))
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			select {
			case <-p.stopCh:
				return
			default:
			}
		}

		select {
		case <-p.stopCh:
			return
		default:
		}
	}
}

func (p *pythonRunnerNamedPipeControl) close() {
	if p == nil {
		return
	}

	p.stopOnce.Do(func() {
		close(p.stopCh)
		unblock, err := os.OpenFile(p.path, os.O_RDWR, 0)
		if err == nil {
			_ = unblock.Close()
		}
		<-p.doneCh
		_ = os.Remove(p.path)
	})
}

func pythonRunnerEventError(event *pythonRunnerEvent, waitErr error, fallback string) error {
	if event == nil {
		if waitErr != nil {
			return fmt.Errorf("%s failed: %w", registrationRunnerErrorPrefix, waitErr)
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
		message = fmt.Sprintf("%s (process exit: %v)", message, waitErr)
	}

	err := error(errors.New(message))
	if event != nil && len(event.AccountPersistence) > 0 {
		req, decodeErr := decodeAccountPersistenceRequest(event.AccountPersistence)
		if decodeErr == nil && req != nil && strings.TrimSpace(req.Email) != "" {
			return &RunnerError{
				Output: RunnerOutput{AccountPersistence: req},
				Err:    err,
			}
		}
	}
	return err
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
