package registration_test

import (
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
)

func TestResolveTaskMetadataOnlyExposesEmailForCompletedJobs(t *testing.T) {
	job := jobs.Job{
		Status:  jobs.StatusPending,
		Payload: []byte(`{"email_service_type":"tempmail"}`),
		Result:  []byte(`{"email":"stale@example.com"}`),
	}

	metadata := registration.ResolveTaskMetadata(job)
	if metadata.Email != nil {
		t.Fatalf("expected pending job email=nil, got %#v", metadata.Email)
	}
	if metadata.EmailService != "tempmail" {
		t.Fatalf("expected email_service=tempmail, got %#v", metadata.EmailService)
	}

	job.Status = jobs.StatusCompleted
	metadata = registration.ResolveTaskMetadata(job)
	if metadata.Email != "stale@example.com" {
		t.Fatalf("expected completed job email, got %#v", metadata.Email)
	}
}
