package registration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	result, err := runner.Run(context.Background(), RunnerRequest{
		TaskUUID: "task-persist",
		StartRequest: StartRequest{
			EmailServiceType: "outlook",
		},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	payload, ok := result[runnerAccountPersistenceResultKey].(map[string]any)
	if !ok {
		t.Fatalf("expected internal account persistence payload, got %#v", result[runnerAccountPersistenceResultKey])
	}
	if payload["email"] != "alice@example.com" || payload["access_token"] != "access-1" {
		t.Fatalf("unexpected internal persistence payload: %#v", payload)
	}
	if result["email"] != "alice@example.com" || result["status"] != "completed" {
		t.Fatalf("expected user-facing result to stay intact, got %#v", result)
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

	result, err := runner.Run(context.Background(), RunnerRequest{
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

	payload, ok := result[runnerAccountPersistenceResultKey].(map[string]any)
	if !ok {
		t.Fatalf("expected internal account persistence payload, got %#v", result[runnerAccountPersistenceResultKey])
	}
	if payload["email"] != "alice@example.com" || payload["access_token"] != "bridge-access-token" {
		t.Fatalf("unexpected internal persistence payload: %#v", payload)
	}
	if result["email"] != "alice@example.com" || result["status"] != "completed" {
		t.Fatalf("expected user-facing result to stay intact, got %#v", result)
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

	result, err := runner.Run(context.Background(), RunnerRequest{
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

	payload, ok := result[runnerAccountPersistenceResultKey].(map[string]any)
	if !ok {
		t.Fatalf("expected internal account persistence payload, got %#v", result[runnerAccountPersistenceResultKey])
	}
	if payload["email"] != "alice@example.com" || payload["access_token"] != "bridge-access-token" {
		t.Fatalf("unexpected internal persistence payload: %#v", payload)
	}
	if result["email"] != "alice@example.com" || result["status"] != "completed" {
		t.Fatalf("expected user-facing result to stay intact, got %#v", result)
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


class Result:
    def __init__(self):
        self.success = True
        self.email = "alice@example.com"
        self.password = "secret"
        self.account_id = "account-1"
        self.workspace_id = "workspace-1"
        self.access_token = "bridge-access-token"
        self.refresh_token = "refresh-1"
        self.id_token = "id-token-1"
        self.session_token = "session-token-1"
        self.device_id = "device-1"
        self.source = "register"
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
