package registration

import (
	"context"
	"errors"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
)

type NativeRunner interface {
	RunNative(ctx context.Context, req RunnerRequest, logf func(level string, message string) error) (NativeRunnerResult, error)
}

type NativeRunnerResult struct {
	Result             map[string]any
	AccountPersistence *accounts.UpsertAccountRequest
}

type nativeRunnerAdapter struct {
	runner NativeRunner
}

func NewNativeRunner(runner NativeRunner) Runner {
	return &nativeRunnerAdapter{runner: runner}
}

func (r *nativeRunnerAdapter) Run(ctx context.Context, req RunnerRequest, logf func(level string, message string) error) (map[string]any, error) {
	if r == nil || r.runner == nil {
		return nil, errors.New("native runner is required")
	}
	if logf == nil {
		logf = func(string, string) error { return nil }
	}

	nativeResult, err := r.runner.RunNative(ctx, req, logf)
	if err != nil {
		return nil, err
	}

	result := cloneNativeRunnerResult(nativeResult.Result)
	if result == nil && nativeResult.AccountPersistence != nil {
		result = map[string]any{}
	}
	if nativeResult.AccountPersistence != nil {
		result[runnerAccountPersistenceResultKey] = nativeResult.AccountPersistence
	}

	return result, nil
}

func cloneNativeRunnerResult(result map[string]any) map[string]any {
	if result == nil {
		return nil
	}

	cloned := make(map[string]any, len(result))
	for key, value := range result {
		cloned[key] = value
	}
	return cloned
}
