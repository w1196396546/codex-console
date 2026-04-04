package registration_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
)

func TestStartRegistrationCreatesPendingTaskResponse(t *testing.T) {
	fakeJobs := &fakeJobsService{
		createResponse: jobs.Job{
			JobID:  "job-1",
			Status: jobs.StatusPending,
		},
	}
	svc := registration.NewService(fakeJobs)

	req := registration.StartRequest{
		EmailServiceType: "tempmail",
	}
	resp, err := svc.StartRegistration(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TaskUUID != "job-1" {
		t.Fatalf("expected job-1, got %s", resp.TaskUUID)
	}
	if resp.Status != "pending" {
		t.Fatalf("expected pending, got %s", resp.Status)
	}
	if fakeJobs.enqueuedJobID != "job-1" {
		t.Fatalf("expected enqueue for job-1, got %q", fakeJobs.enqueuedJobID)
	}
	if fakeJobs.createParams.JobType != "registration_single" {
		t.Fatalf("expected registration_single job type, got %q", fakeJobs.createParams.JobType)
	}
	if fakeJobs.createParams.ScopeType != "registration" {
		t.Fatalf("expected registration scope type, got %q", fakeJobs.createParams.ScopeType)
	}
	if fakeJobs.createParams.ScopeID != "single" {
		t.Fatalf("expected single scope id, got %q", fakeJobs.createParams.ScopeID)
	}

	var decoded registration.StartRequest
	if err := json.Unmarshal(fakeJobs.createParams.Payload, &decoded); err != nil {
		t.Fatalf("unexpected payload decode error: %v", err)
	}
	if decoded.EmailServiceType != req.EmailServiceType {
		t.Fatalf("expected payload email service type %q, got %q", req.EmailServiceType, decoded.EmailServiceType)
	}
}

func TestStartRegistrationReturnsWrappedCreateError(t *testing.T) {
	svc := registration.NewService(&fakeJobsService{
		createError: errors.New("db down"),
	})

	_, err := svc.StartRegistration(context.Background(), registration.StartRequest{
		EmailServiceType: "tempmail",
	})
	if err == nil {
		t.Fatal("expected create error")
	}
	if !strings.Contains(err.Error(), "create registration job") {
		t.Fatalf("expected wrapped create error, got %v", err)
	}
}

func TestStartRegistrationReturnsWrappedEnqueueError(t *testing.T) {
	svc := registration.NewService(&fakeJobsService{
		createResponse: jobs.Job{
			JobID:  "job-2",
			Status: jobs.StatusPending,
		},
		enqueueError: errors.New("queue down"),
	})

	_, err := svc.StartRegistration(context.Background(), registration.StartRequest{
		EmailServiceType: "tempmail",
	})
	if err == nil {
		t.Fatal("expected enqueue error")
	}
	if !strings.Contains(err.Error(), "enqueue registration job") {
		t.Fatalf("expected wrapped enqueue error, got %v", err)
	}
}

type fakeJobsService struct {
	createParams   jobs.CreateJobParams
	createResponse jobs.Job
	createError    error
	enqueuedJobID  string
	enqueueError   error
}

func (f *fakeJobsService) CreateJob(_ context.Context, params jobs.CreateJobParams) (jobs.Job, error) {
	f.createParams = params
	if f.createError != nil {
		return jobs.Job{}, f.createError
	}
	return f.createResponse, nil
}

func (f *fakeJobsService) EnqueueJob(_ context.Context, jobID string) error {
	f.enqueuedJobID = jobID
	return f.enqueueError
}
