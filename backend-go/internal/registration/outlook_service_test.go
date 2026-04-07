package registration_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
)

func TestListOutlookAccounts(t *testing.T) {
	service := registration.NewOutlookService(outlookFakeRepository{
		services: []registration.EmailServiceRecord{
			{
				ID:          101,
				ServiceType: "outlook",
				Name:        "Outlook A",
				Config: map[string]any{
					"email":         "alpha@example.com",
					"client_id":     "client-alpha",
					"refresh_token": "oauth-refresh-alpha",
				},
			},
			{
				ID:          102,
				ServiceType: "outlook",
				Name:        "Outlook B",
				Config: map[string]any{
					"email": "beta@example.com",
				},
			},
			{
				ID:          103,
				ServiceType: "outlook",
				Name:        "Outlook C",
				Config:      map[string]any{},
			},
		},
		accounts: []registration.RegisteredAccountRecord{
			{
				ID:           9001,
				Email:        "alpha@example.com",
				RefreshToken: "account-refresh-alpha",
			},
			{
				ID:           9002,
				Email:        "beta@example.com",
				RefreshToken: "",
			},
		},
	}, nil)

	response, err := service.ListOutlookAccounts(context.Background())
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}

	if response.Total != 3 {
		t.Fatalf("expected total=3, got %+v", response)
	}
	if response.RegisteredCount != 2 {
		t.Fatalf("expected registered_count=2, got %+v", response)
	}
	if response.UnregisteredCount != 1 {
		t.Fatalf("expected unregistered_count=1, got %+v", response)
	}
	if len(response.Accounts) != 3 {
		t.Fatalf("expected three outlook accounts, got %+v", response.Accounts)
	}

	first := response.Accounts[0]
	if first.ID != 101 || first.Email != "alpha@example.com" || first.Name != "Outlook A" {
		t.Fatalf("unexpected first account mapping: %+v", first)
	}
	if !first.HasOAuth || !first.IsRegistered || !first.HasRefreshToken || first.NeedsTokenRefresh || !first.IsRegistrationComplete {
		t.Fatalf("unexpected first account state: %+v", first)
	}
	if first.RegisteredAccountID == nil || *first.RegisteredAccountID != 9001 {
		t.Fatalf("expected first registered_account_id=9001, got %+v", first.RegisteredAccountID)
	}

	second := response.Accounts[1]
	if second.ID != 102 || second.Email != "beta@example.com" {
		t.Fatalf("unexpected second account mapping: %+v", second)
	}
	if second.HasOAuth || !second.IsRegistered || second.HasRefreshToken || !second.NeedsTokenRefresh || second.IsRegistrationComplete {
		t.Fatalf("unexpected second account state: %+v", second)
	}
	if second.RegisteredAccountID == nil || *second.RegisteredAccountID != 9002 {
		t.Fatalf("expected second registered_account_id=9002, got %+v", second.RegisteredAccountID)
	}

	third := response.Accounts[2]
	if third.ID != 103 || third.Email != "Outlook C" {
		t.Fatalf("expected fallback email from service name, got %+v", third)
	}
	if third.IsRegistered || third.HasRefreshToken || third.NeedsTokenRefresh || third.IsRegistrationComplete {
		t.Fatalf("unexpected third account state: %+v", third)
	}
	if third.RegisteredAccountID != nil {
		t.Fatalf("expected third registered_account_id=nil, got %+v", third.RegisteredAccountID)
	}
}

func TestStartOutlookBatch(t *testing.T) {
	jobsService := newRecordingBatchJobsService()
	batchService := registration.NewBatchService(jobsService)
	service := registration.NewOutlookService(outlookFakeRepository{}, batchService)

	response, err := service.StartOutlookBatch(context.Background(), registration.OutlookBatchStartRequest{
		ServiceIDs:              []int{101, 202},
		ChatGPTRegistrationMode: "access_token_only",
		Proxy:                   "http://proxy.internal:8080",
		IntervalMin:             5,
		IntervalMax:             30,
		Concurrency:             3,
		Mode:                    "pipeline",
		AutoUploadCPA:           true,
		CPAServiceIDs:           []int{7, 8},
		AutoUploadSub2API:       true,
		Sub2APIServiceIDs:       []int{9},
		AutoUploadTM:            true,
		TMServiceIDs:            []int{10, 11},
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	if response.BatchID == "" {
		t.Fatalf("expected batch_id, got %+v", response)
	}
	if response.Total != 2 || response.Skipped != 0 || response.ToRegister != 2 {
		t.Fatalf("unexpected batch response counts: %+v", response)
	}
	if !reflect.DeepEqual(response.ServiceIDs, []int{101, 202}) {
		t.Fatalf("expected service_ids [101 202], got %+v", response.ServiceIDs)
	}
	if len(jobsService.createParams) != 2 {
		t.Fatalf("expected 2 created jobs, got %d", len(jobsService.createParams))
	}

	var payloadOne registration.StartRequest
	if err := json.Unmarshal(jobsService.createParams[0].Payload, &payloadOne); err != nil {
		t.Fatalf("decode first payload: %v", err)
	}
	if payloadOne.EmailServiceType != "outlook" || payloadOne.EmailServiceID == nil || *payloadOne.EmailServiceID != 101 {
		t.Fatalf("unexpected first payload: %+v", payloadOne)
	}
	if payloadOne.Proxy != "http://proxy.internal:8080" {
		t.Fatalf("expected first payload proxy, got %+v", payloadOne)
	}
	if payloadOne.IntervalMin != 5 || payloadOne.IntervalMax != 30 || payloadOne.Concurrency != 3 || payloadOne.Mode != "pipeline" {
		t.Fatalf("expected first payload scheduling fields, got %+v", payloadOne)
	}
	if payloadOne.ChatGPTRegistrationMode != "access_token_only" {
		t.Fatalf("expected first payload registration mode, got %+v", payloadOne)
	}
	if !payloadOne.AutoUploadCPA || !reflect.DeepEqual(payloadOne.CPAServiceIDs, []int{7, 8}) {
		t.Fatalf("expected first payload CPA upload fields, got %+v", payloadOne)
	}
	if !payloadOne.AutoUploadSub2API || !reflect.DeepEqual(payloadOne.Sub2APIServiceIDs, []int{9}) {
		t.Fatalf("expected first payload Sub2API upload fields, got %+v", payloadOne)
	}
	if !payloadOne.AutoUploadTM || !reflect.DeepEqual(payloadOne.TMServiceIDs, []int{10, 11}) {
		t.Fatalf("expected first payload TM upload fields, got %+v", payloadOne)
	}

	var payloadTwo registration.StartRequest
	if err := json.Unmarshal(jobsService.createParams[1].Payload, &payloadTwo); err != nil {
		t.Fatalf("decode second payload: %v", err)
	}
	if payloadTwo.EmailServiceType != "outlook" || payloadTwo.EmailServiceID == nil || *payloadTwo.EmailServiceID != 202 {
		t.Fatalf("unexpected second payload: %+v", payloadTwo)
	}
	if payloadTwo.IntervalMin != 5 || payloadTwo.IntervalMax != 30 || payloadTwo.Concurrency != 3 || payloadTwo.Mode != "pipeline" {
		t.Fatalf("expected second payload scheduling fields, got %+v", payloadTwo)
	}
	if payloadTwo.ChatGPTRegistrationMode != "access_token_only" {
		t.Fatalf("expected second payload registration mode, got %+v", payloadTwo)
	}

	status, err := service.GetOutlookBatch(context.Background(), response.BatchID, 0)
	if err != nil {
		t.Fatalf("unexpected get batch error: %v", err)
	}
	if status.BatchID != response.BatchID {
		t.Fatalf("expected batch id %q, got %+v", response.BatchID, status)
	}
	if status.Total != 2 || status.Skipped != 0 || status.Status != jobs.StatusRunning {
		t.Fatalf("unexpected outlook batch status: %+v", status)
	}
}

func TestOutlookBatchEndpointsCompatibilityProgressFields(t *testing.T) {
	jobsService := newRecordingBatchJobsService()
	batchService := registration.NewBatchService(jobsService)
	service := registration.NewOutlookService(outlookFakeRepository{}, batchService)

	response, err := service.StartOutlookBatch(context.Background(), registration.OutlookBatchStartRequest{
		ServiceIDs: []int{101, 202},
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	initial, err := service.GetOutlookBatch(context.Background(), response.BatchID, 0)
	if err != nil {
		t.Fatalf("unexpected initial get error: %v", err)
	}
	if initial.Skipped != 0 {
		t.Fatalf("expected skipped=0, got %#v", initial.Skipped)
	}
	if initial.CurrentIndex != 0 {
		t.Fatalf("expected current_index=0, got %#v", initial.CurrentIndex)
	}
	if initial.LogBaseIndex != 0 {
		t.Fatalf("expected log_base_index=0, got %#v", initial.LogBaseIndex)
	}

	if _, err := batchService.CancelBatch(context.Background(), response.BatchID); err != nil {
		t.Fatalf("unexpected cancel error: %v", err)
	}

	cancelling, err := service.GetOutlookBatch(context.Background(), response.BatchID, 0)
	if err != nil {
		t.Fatalf("unexpected cancelling get error: %v", err)
	}
	if cancelling.Status != "cancelling" || !cancelling.Cancelled || cancelling.Finished {
		t.Fatalf("expected cancelling snapshot, got %+v", cancelling)
	}

	settled, err := service.GetOutlookBatch(context.Background(), response.BatchID, 0)
	if err != nil {
		t.Fatalf("unexpected settled get error: %v", err)
	}
	if settled.Status != jobs.StatusCancelled || !settled.Cancelled || !settled.Finished {
		t.Fatalf("expected settled cancelled snapshot, got %+v", settled)
	}
}

type outlookFakeRepository struct {
	services []registration.EmailServiceRecord
	accounts []registration.RegisteredAccountRecord
}

func (f outlookFakeRepository) ListOutlookServices(_ context.Context) ([]registration.EmailServiceRecord, error) {
	return append([]registration.EmailServiceRecord(nil), f.services...), nil
}

func (f outlookFakeRepository) ListAccountsByEmails(_ context.Context, _ []string) ([]registration.RegisteredAccountRecord, error) {
	return append([]registration.RegisteredAccountRecord(nil), f.accounts...), nil
}

type recordingBatchJobsService struct {
	createParams []jobs.CreateJobParams
	jobs         map[string]jobs.Job
	jobLogs      map[string][]jobs.JobLog
	nextID       int
}

func newRecordingBatchJobsService() *recordingBatchJobsService {
	return &recordingBatchJobsService{
		jobs:    make(map[string]jobs.Job),
		jobLogs: make(map[string][]jobs.JobLog),
		nextID:  1,
	}
}

func (s *recordingBatchJobsService) CreateJob(_ context.Context, params jobs.CreateJobParams) (jobs.Job, error) {
	jobID := "job-" + strconv.Itoa(s.nextID)
	s.nextID++

	job := jobs.Job{
		JobID:     jobID,
		JobType:   params.JobType,
		ScopeType: params.ScopeType,
		ScopeID:   params.ScopeID,
		Status:    jobs.StatusPending,
		Payload:   append([]byte(nil), params.Payload...),
	}
	s.createParams = append(s.createParams, params)
	s.jobs[jobID] = job
	return job, nil
}

func (s *recordingBatchJobsService) EnqueueJob(context.Context, string) error {
	return nil
}

func (s *recordingBatchJobsService) GetJob(_ context.Context, jobID string) (jobs.Job, error) {
	job, ok := s.jobs[jobID]
	if !ok {
		return jobs.Job{}, jobs.ErrJobNotFound
	}
	return job, nil
}

func (s *recordingBatchJobsService) ListJobLogs(_ context.Context, jobID string) ([]jobs.JobLog, error) {
	return append([]jobs.JobLog(nil), s.jobLogs[jobID]...), nil
}

func (s *recordingBatchJobsService) PauseJob(_ context.Context, jobID string) (jobs.Job, error) {
	return s.updateStatus(jobID, jobs.StatusPaused)
}

func (s *recordingBatchJobsService) ResumeJob(_ context.Context, jobID string) (jobs.Job, error) {
	return s.updateStatus(jobID, jobs.StatusRunning)
}

func (s *recordingBatchJobsService) CancelJob(_ context.Context, jobID string) (jobs.Job, error) {
	return s.updateStatus(jobID, jobs.StatusCancelled)
}

func (s *recordingBatchJobsService) updateStatus(jobID string, status string) (jobs.Job, error) {
	job, ok := s.jobs[jobID]
	if !ok {
		return jobs.Job{}, jobs.ErrJobNotFound
	}
	job.Status = status
	s.jobs[jobID] = job
	return job, nil
}
