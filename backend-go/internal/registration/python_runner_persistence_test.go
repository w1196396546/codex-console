package registration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPythonRunnerRunIncludesInternalAccountPersistencePayload(t *testing.T) {
	repoRoot := createTestRepoRoot(t)
	pythonExecutable := createFakePythonExecutable(t, `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"result","success":true,"result":{"status":"completed","email":"alice@example.com"},"account_persistence":{"email":"alice@example.com","email_service":"outlook","access_token":"access-1"}}'
`)

	runner := &PythonRunner{
		pythonExecutable: pythonExecutable,
		repoRoot:         repoRoot,
	}

	output, err := runner.Run(context.Background(), RunnerRequest{
		TaskUUID: "task-persist",
		StartRequest: StartRequest{
			EmailServiceType: "outlook",
		},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.AccountPersistence == nil {
		t.Fatal("expected account persistence request")
	}
	if output.AccountPersistence.Email != "alice@example.com" || output.AccountPersistence.AccessToken != "access-1" {
		t.Fatalf("unexpected account persistence request: %+v", output.AccountPersistence)
	}
	if output.Result["email"] != "alice@example.com" || output.Result["status"] != "completed" {
		t.Fatalf("expected user-facing result to stay intact, got %#v", output.Result)
	}
}

func TestPythonRunnerRunDoesNotLeakAccountPersistenceIntoUserResult(t *testing.T) {
	repoRoot := createTestRepoRoot(t)
	pythonExecutable := createFakePythonExecutable(t, `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"result","success":true,"result":{"status":"completed","email":"alice@example.com"},"account_persistence":{"email":"alice@example.com","email_service":"outlook","access_token":"access-1"}}'
`)

	runner := &PythonRunner{
		pythonExecutable: pythonExecutable,
		repoRoot:         repoRoot,
	}

	output, err := runner.Run(context.Background(), RunnerRequest{
		TaskUUID: "task-no-leak",
		StartRequest: StartRequest{
			EmailServiceType: "outlook",
		},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.AccountPersistence == nil {
		t.Fatal("expected account persistence to be returned separately")
	}
	if output.Result["email"] != "alice@example.com" || output.Result["status"] != "completed" {
		t.Fatalf("expected user-facing result to stay intact, got %#v", output.Result)
	}
}

func TestPythonRunnerRunSkipsPythonPersistenceAndUploadsWhenGoPersistenceEnabled(t *testing.T) {
	pythonExecutable, err := resolvePythonExecutable()
	if err != nil {
		t.Skipf("python executable not available: %v", err)
	}

	repoRoot := createPythonBridgeRepoRoot(t)
	saveMarker := filepath.Join(t.TempDir(), "save.marker")
	uploadMarker := filepath.Join(t.TempDir(), "upload.marker")
	t.Setenv("SAVE_MARKER", saveMarker)
	t.Setenv("UPLOAD_MARKER", uploadMarker)

	runner := &PythonRunner{
		pythonExecutable: pythonExecutable,
		repoRoot:         repoRoot,
	}

	output, err := runner.Run(context.Background(), RunnerRequest{
		TaskUUID:             "task-go-persistence",
		GoPersistenceEnabled: true,
		StartRequest: StartRequest{
			EmailServiceType: "tempmail",
			AutoUploadCPA:    true,
		},
		Plan: ExecutionPlan{
			EmailService: PreparedEmailService{
				Prepared: true,
				Type:     "tempmail",
				Config:   map[string]any{"mode": "prepared"},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(saveMarker); !os.IsNotExist(err) {
		t.Fatalf("expected python save_to_database to be skipped, stat err=%v", err)
	}
	if _, err := os.Stat(uploadMarker); !os.IsNotExist(err) {
		t.Fatalf("expected python optional uploads to be skipped, stat err=%v", err)
	}

	if output.AccountPersistence == nil {
		t.Fatal("expected account persistence request")
	}
	if output.AccountPersistence.Email != "alice@example.com" || output.AccountPersistence.AccessToken != "runner-access-token" {
		t.Fatalf("unexpected account persistence request: %+v", output.AccountPersistence)
	}
	if output.Result["email"] != "alice@example.com" || output.Result["status"] != "completed" {
		t.Fatalf("expected user-facing result to stay intact, got %#v", output.Result)
	}
}

func TestPythonRunnerRunKeepsPythonPersistenceAndUploadsWhenGoPersistenceDisabled(t *testing.T) {
	pythonExecutable, err := resolvePythonExecutable()
	if err != nil {
		t.Skipf("python executable not available: %v", err)
	}

	repoRoot := createPythonBridgeRepoRoot(t)
	saveMarker := filepath.Join(t.TempDir(), "save.marker")
	uploadMarker := filepath.Join(t.TempDir(), "upload.marker")
	t.Setenv("SAVE_MARKER", saveMarker)
	t.Setenv("UPLOAD_MARKER", uploadMarker)

	runner := &PythonRunner{
		pythonExecutable: pythonExecutable,
		repoRoot:         repoRoot,
	}

	output, err := runner.Run(context.Background(), RunnerRequest{
		TaskUUID:             "task-python-persistence",
		GoPersistenceEnabled: false,
		StartRequest: StartRequest{
			EmailServiceType: "tempmail",
			AutoUploadCPA:    true,
		},
		Plan: ExecutionPlan{
			EmailService: PreparedEmailService{
				Prepared: true,
				Type:     "tempmail",
				Config:   map[string]any{"mode": "prepared"},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertFileContains(t, saveMarker, "saved")
	assertFileContains(t, uploadMarker, "uploaded")

	if output.AccountPersistence == nil {
		t.Fatal("expected account persistence request")
	}
	if output.AccountPersistence.Email != "alice@example.com" || output.AccountPersistence.AccessToken != "runner-access-token" {
		t.Fatalf("unexpected account persistence request: %+v", output.AccountPersistence)
	}
	if output.Result["email"] != "alice@example.com" || output.Result["status"] != "completed" {
		t.Fatalf("expected user-facing result to stay intact, got %#v", output.Result)
	}
}

func TestPythonRunnerRunIncludesOptionalAccountPersistenceFields(t *testing.T) {
	pythonExecutable, err := resolvePythonExecutable()
	if err != nil {
		t.Skipf("python executable not available: %v", err)
	}

	repoRoot := createPythonBridgeRepoRoot(t)
	runner := &PythonRunner{
		pythonExecutable: pythonExecutable,
		repoRoot:         repoRoot,
	}

	output, err := runner.Run(context.Background(), RunnerRequest{
		TaskUUID:             "task-optional-persist",
		GoPersistenceEnabled: true,
		StartRequest: StartRequest{
			EmailServiceType: "tempmail",
		},
		Plan: ExecutionPlan{
			EmailService: PreparedEmailService{
				Prepared: true,
				Type:     "tempmail",
				Config:   map[string]any{"mode": "prepared"},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.AccountPersistence == nil {
		t.Fatal("expected account persistence request")
	}
	if output.AccountPersistence.LastRefresh == nil || output.AccountPersistence.LastRefresh.Format(time.RFC3339) != "2026-04-04T08:00:00Z" {
		t.Fatalf("expected last_refresh in persistence request, got %+v", output.AccountPersistence)
	}
	if output.AccountPersistence.ExpiresAt == nil || output.AccountPersistence.ExpiresAt.Format(time.RFC3339) != "2026-04-04T09:30:00Z" {
		t.Fatalf("expected expires_at in persistence request, got %+v", output.AccountPersistence)
	}
	if output.AccountPersistence.SubscriptionType != "team" {
		t.Fatalf("expected subscription_type in persistence request, got %+v", output.AccountPersistence)
	}
	if output.AccountPersistence.SubscriptionAt == nil || output.AccountPersistence.SubscriptionAt.Format(time.RFC3339) != "2026-04-04T10:00:00Z" {
		t.Fatalf("expected subscription_at in persistence request, got %+v", output.AccountPersistence)
	}
}

func TestPythonRunnerRunFatalErrorCarriesAccountPersistenceOutput(t *testing.T) {
	repoRoot := createTestRepoRoot(t)
	pythonExecutable := createFakePythonExecutable(t, `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"fatal","success":false,"error_message":"token completion blocked","account_persistence":{"email":"alice@example.com","email_service":"tempmail","status":"token_pending"}}'
exit 1
`)

	runner := &PythonRunner{
		pythonExecutable: pythonExecutable,
		repoRoot:         repoRoot,
	}

	_, err := runner.Run(context.Background(), RunnerRequest{
		TaskUUID: "task-fatal-persistence",
		StartRequest: StartRequest{
			EmailServiceType: "tempmail",
		},
	}, nil)
	if err == nil {
		t.Fatal("expected fatal error")
	}

	var runnerErr *RunnerError
	if !errors.As(err, &runnerErr) || runnerErr == nil {
		t.Fatalf("expected RunnerError carrying persistence output, got %T %v", err, err)
	}
	if runnerErr.Output.AccountPersistence == nil || runnerErr.Output.AccountPersistence.Email != "alice@example.com" {
		t.Fatalf("expected fatal error to carry account persistence, got %+v", runnerErr.Output.AccountPersistence)
	}
}

func createPythonBridgeRepoRoot(t *testing.T) string {
	t.Helper()

	repoRoot := createTestRepoRoot(t)

	mustWriteFile(t, filepath.Join(repoRoot, "src", "__init__.py"), "")
	mustMkdirAll(t, filepath.Join(repoRoot, "src", "config"))
	mustWriteFile(t, filepath.Join(repoRoot, "src", "config", "__init__.py"), "")
	mustWriteFile(t, filepath.Join(repoRoot, "src", "config", "settings.py"), `class Settings:
    openai_client_id = "client-123"
    tempmail_enabled = True
    tempmail_base_url = "https://example.com"
    tempmail_timeout = 30
    tempmail_max_retries = 1
    custom_domain_base_url = ""
    custom_domain_api_key = None


def get_settings():
    return Settings()
`)

	mustMkdirAll(t, filepath.Join(repoRoot, "src", "core"))
	mustWriteFile(t, filepath.Join(repoRoot, "src", "core", "__init__.py"), "")
	mustWriteFile(t, filepath.Join(repoRoot, "src", "core", "register.py"), `import os
from datetime import datetime, timezone


class Result:
    def __init__(self):
        self.success = True
        self.email = "alice@example.com"
        self.password = "secret"
        self.account_id = "account-1"
        self.workspace_id = "workspace-1"
        self.access_token = "runner-access-token"
        self.refresh_token = "refresh-1"
        self.id_token = "id-token-1"
        self.session_token = "session-token-1"
        self.device_id = "device-1"
        self.source = "register"
        self.last_refresh = datetime(2026, 4, 4, 8, 0, 0, tzinfo=timezone.utc)
        self.expires_at = datetime(2026, 4, 4, 9, 30, 0, tzinfo=timezone.utc)
        self.subscription_type = "team"
        self.subscription_at = datetime(2026, 4, 4, 10, 0, 0, tzinfo=timezone.utc)
        self.metadata = {"python_runner": "active"}
        self.error_message = ""

    def to_dict(self):
        return {
            "status": "completed",
            "email": self.email,
            "account_id": self.account_id,
        }


class RegistrationEngine:
    def __init__(self, email_service, proxy_url=None, callback_logger=None, task_uuid=None, check_cancelled=None):
        self.email_service = email_service
        self.proxy_url = proxy_url
        self.callback_logger = callback_logger
        self.task_uuid = task_uuid
        self.check_cancelled = check_cancelled
        self.email_info = {"service_id": "prepared-service"}

    def run(self):
        return Result()

    def save_to_database(self, result):
        marker = os.getenv("SAVE_MARKER", "")
        if marker:
            with open(marker, "a", encoding="utf-8") as handle:
                handle.write("saved\n")
        return True

    def _resolve_persisted_account_status(self, result, metadata):
        return "completed"

    def _dump_session_cookies(self):
        return "cookie=1"
`)

	mustMkdirAll(t, filepath.Join(repoRoot, "src", "core", "upload"))
	mustWriteFile(t, filepath.Join(repoRoot, "src", "core", "upload", "__init__.py"), "")
	mustWriteFile(t, filepath.Join(repoRoot, "src", "core", "upload", "cpa_upload.py"), `import os


def generate_token_json(account):
    return {"email": account.email}


def upload_to_cpa(token_data, api_url=None, api_token=None):
    marker = os.getenv("UPLOAD_MARKER", "")
    if marker:
        with open(marker, "a", encoding="utf-8") as handle:
            handle.write("uploaded\n")
    return True, "ok"
`)

	mustMkdirAll(t, filepath.Join(repoRoot, "src", "services"))
	mustWriteFile(t, filepath.Join(repoRoot, "src", "services", "__init__.py"), `class EmailServiceType(str):
    def __new__(cls, value):
        return str.__new__(cls, value)


class EmailServiceFactory:
    @staticmethod
    def create(service_type, config):
        return {"type": str(service_type), "config": dict(config or {})}
`)

	mustMkdirAll(t, filepath.Join(repoRoot, "src", "database"))
	mustWriteFile(t, filepath.Join(repoRoot, "src", "database", "__init__.py"), "")
	mustWriteFile(t, filepath.Join(repoRoot, "src", "database", "models.py"), `class Account:
    def __init__(self, email="", access_token="persisted-access-token"):
        self.email = email
        self.access_token = access_token
        self.cpa_uploaded = False
        self.cpa_uploaded_at = None
        self.sub2api_uploaded = False
        self.sub2api_uploaded_at = None


class CPAService:
    def __init__(self, service_id, name, api_url, api_token):
        self.id = service_id
        self.name = name
        self.api_url = api_url
        self.api_token = api_token
`)
	mustWriteFile(t, filepath.Join(repoRoot, "src", "database", "session.py"), `from contextlib import contextmanager

from .models import Account


class FakeQuery:
    def __init__(self, model):
        self.model = model
        self.filters = {}

    def filter_by(self, **kwargs):
        self.filters.update(kwargs)
        return self

    def first(self):
        if self.model is Account:
            return Account(email=self.filters.get("email", "alice@example.com"))
        return None


class FakeDB:
    def query(self, model):
        return FakeQuery(model)

    def commit(self):
        return None


@contextmanager
def get_db():
    yield FakeDB()
`)
	mustWriteFile(t, filepath.Join(repoRoot, "src", "database", "crud.py"), `from .models import CPAService


def get_cpa_services(db, enabled=True):
    return [CPAService(101, "primary-cpa", "https://example.com/cpa", "token-1")]


def get_cpa_service_by_id(db, service_id):
    return CPAService(service_id, "primary-cpa", "https://example.com/cpa", "token-1")


def get_sub2api_services(db, enabled=True):
    return []


def get_sub2api_service_by_id(db, service_id):
    return None


def get_tm_services(db, enabled=True):
    return []


def get_tm_service_by_id(db, service_id):
    return None
`)

	return repoRoot
}

func TestDecodeAccountPersistenceRequestPreservesOptionalAccountFields(t *testing.T) {
	req, err := decodeAccountPersistenceRequest(map[string]any{
		"email":             "alice@example.com",
		"email_service":     "outlook",
		"last_refresh":      "2026-04-04T08:00:00Z",
		"expires_at":        "2026-04-04T09:30:00Z",
		"subscription_type": "team",
		"subscription_at":   "2026-04-04T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if req == nil {
		t.Fatal("expected persistence payload to be decoded")
	}
	lastRefresh := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 4, 4, 9, 30, 0, 0, time.UTC)
	subscriptionAt := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	if req.LastRefresh == nil || !req.LastRefresh.Equal(lastRefresh) {
		t.Fatalf("expected last_refresh %v, got %#v", lastRefresh, req.LastRefresh)
	}
	if req.ExpiresAt == nil || !req.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at %v, got %#v", expiresAt, req.ExpiresAt)
	}
	if req.SubscriptionType != "team" {
		t.Fatalf("expected subscription_type team, got %+v", req)
	}
	if req.SubscriptionAt == nil || !req.SubscriptionAt.Equal(subscriptionAt) {
		t.Fatalf("expected subscription_at %v, got %#v", subscriptionAt, req.SubscriptionAt)
	}
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(data) == "" || !containsLine(string(data), want) {
		t.Fatalf("expected %q to contain %q, got %q", path, want, string(data))
	}
}

func containsLine(content string, want string) bool {
	for _, line := range strings.Split(content, "\n") {
		if line == want {
			return true
		}
	}
	return false
}
