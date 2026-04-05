package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	accountspkg "github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	paymentpkg "github.com/dou-jiang/codex-console/backend-go/internal/payment"
	teampkg "github.com/dou-jiang/codex-console/backend-go/internal/team"
)

func TestPaymentPhaseFourCompatibilityRoutes(t *testing.T) {
	now := time.Date(2026, 4, 6, 1, 0, 0, 0, time.UTC)
	accountRepo := &phaseFourPaymentAccountsRepository{
		accounts: map[int]accountspkg.Account{
			7: {
				ID:               7,
				Email:            "ops@example.com",
				Password:         "secret-7",
				EmailService:     "manual",
				SessionToken:     "session-old",
				Cookies:          "__Secure-next-auth.session-token=session-old; oai-did=device-7",
				SubscriptionType: "",
			},
			8: {
				ID:               8,
				Email:            "paid@example.com",
				Password:         "secret-8",
				EmailService:     "manual",
				SessionToken:     "session-8",
				Cookies:          "__Secure-next-auth.session-token=session-8; oai-did=device-8",
				SubscriptionType: "team",
				SubscriptionAt:   timePtr(now.Add(-2 * time.Hour)),
			},
		},
	}
	repo := &phaseFourPaymentRepository{
		now: func() time.Time { return now },
		accountEmails: map[int]string{
			7: "ops@example.com",
			8: "paid@example.com",
		},
		tasks: map[int]paymentpkg.BindCardTask{},
	}
	service := paymentpkg.NewService(
		repo,
		accountRepo,
		paymentpkg.WithNow(func() time.Time { return now }),
		paymentpkg.WithCheckoutLinkGenerator(phaseFourCheckoutGenerator{}),
		paymentpkg.WithBillingProfileGenerator(phaseFourBillingProfileGenerator{}),
		paymentpkg.WithBrowserOpener(phaseFourBrowserOpener{}),
		paymentpkg.WithSessionAdapter(phaseFourSessionAdapter{
			result: paymentpkg.SessionBootstrapResult{
				SessionToken: "session-bootstrap-7",
				AccessToken:  "access-bootstrap-7",
				Cookies:      "__Secure-next-auth.session-token=session-bootstrap-7; oai-did=device-7",
			},
		}),
		paymentpkg.WithSubscriptionChecker(phaseFourSubscriptionChecker{
			details: map[int]paymentpkg.SubscriptionCheckDetail{
				7: {
					Status:         "team",
					Confidence:     "high",
					Source:         "chatgpt_web",
					RefreshedToken: true,
				},
				8: {
					Status:     "team",
					Confidence: "high",
					Source:     "chatgpt_web",
				},
			},
		}),
	)

	server := httptest.NewServer(internalhttp.NewRouter(nil, service))
	defer server.Close()

	generatePayload := mustRequestJSON(t, server, http.MethodPost, "/api/payment/generate-link", map[string]any{
		"account_id":     7,
		"plan_type":      "team",
		"workspace_name": "Ops",
		"price_interval": "month",
		"seat_quantity":  5,
		"country":        "GB",
		"currency":       "GBP",
	}).(map[string]any)
	if generatePayload["link"] != "https://chatgpt.com/pay/cs_phase4_7" || generatePayload["source"] != "openai_checkout" {
		t.Fatalf("unexpected generate-link payload: %#v", generatePayload)
	}

	taskPayload := mustRequestJSON(t, server, http.MethodPost, "/api/payment/bind-card/tasks", map[string]any{
		"account_id":     7,
		"plan_type":      "team",
		"workspace_name": "Ops",
		"price_interval": "month",
		"seat_quantity":  5,
		"country":        "GB",
		"currency":       "GBP",
		"bind_mode":      "semi_auto",
	}).(map[string]any)
	task, ok := taskPayload["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected created task object, got %#v", taskPayload["task"])
	}
	if task["status"] != paymentpkg.StatusLinkReady || task["checkout_url"] != "https://chatgpt.com/pay/cs_phase4_7" {
		t.Fatalf("unexpected created bind-card task payload: %#v", task)
	}

	listPayload := mustRequestJSON(t, server, http.MethodGet, "/api/payment/bind-card/tasks?page=1&page_size=20&status=link_ready&search=ops@example.com", nil).(map[string]any)
	if listPayload["total"] != float64(1) {
		t.Fatalf("expected filtered bind-card task total=1, got %#v", listPayload)
	}
	tasks, ok := listPayload["tasks"].([]any)
	if !ok || len(tasks) != 1 {
		t.Fatalf("expected one bind-card task row, got %#v", listPayload["tasks"])
	}
	listedTask := tasks[0].(map[string]any)
	if listedTask["account_email"] != "ops@example.com" || listedTask["status"] != paymentpkg.StatusLinkReady {
		t.Fatalf("unexpected bind-card task listing row: %#v", listedTask)
	}

	openPayload := mustRequestJSON(t, server, http.MethodPost, "/api/payment/bind-card/tasks/1/open", map[string]any{}).(map[string]any)
	openTask := openPayload["task"].(map[string]any)
	if openTask["status"] != paymentpkg.StatusOpened {
		t.Fatalf("expected opened task status, got %#v", openTask)
	}

	verifyPayload := mustRequestJSON(t, server, http.MethodPost, "/api/payment/bind-card/tasks/1/mark-user-action", map[string]any{
		"timeout_seconds":  180,
		"interval_seconds": 10,
	}).(map[string]any)
	if verifyPayload["verified"] != true || verifyPayload["subscription_type"] != "team" {
		t.Fatalf("unexpected mark-user-action payload: %#v", verifyPayload)
	}
	verifyTask := verifyPayload["task"].(map[string]any)
	if verifyTask["status"] != paymentpkg.StatusCompleted {
		t.Fatalf("expected completed bind-card task after verification, got %#v", verifyTask)
	}

	bootstrapPayload := mustRequestJSON(t, server, http.MethodPost, "/api/payment/accounts/7/session-bootstrap", map[string]any{}).(map[string]any)
	if bootstrapPayload["success"] != true || bootstrapPayload["session_token_len"] != float64(len("session-bootstrap-7")) {
		t.Fatalf("unexpected session-bootstrap payload: %#v", bootstrapPayload)
	}

	batchPayload := mustRequestJSON(t, server, http.MethodPost, "/api/payment/accounts/batch-check-subscription", map[string]any{
		"ids":        []int{7, 8},
		"select_all": false,
	}).(map[string]any)
	if batchPayload["success_count"] != float64(2) || batchPayload["failed_count"] != float64(0) {
		t.Fatalf("unexpected batch-check payload counters: %#v", batchPayload)
	}
	details, ok := batchPayload["details"].([]any)
	if !ok || len(details) != 2 {
		t.Fatalf("expected 2 batch-check details, got %#v", batchPayload["details"])
	}
	firstDetail := details[0].(map[string]any)
	if firstDetail["subscription_type"] != "team" || firstDetail["source"] != "chatgpt_web" {
		t.Fatalf("unexpected batch-check detail: %#v", firstDetail)
	}

	markPayload := mustRequestJSON(t, server, http.MethodPost, "/api/payment/accounts/7/mark-subscription", map[string]any{
		"subscription_type": "team",
	}).(map[string]any)
	if markPayload["success"] != true || markPayload["subscription_type"] != "team" {
		t.Fatalf("unexpected mark-subscription payload: %#v", markPayload)
	}
}

func TestTeamPhaseFourAcceptedTaskLiveFlow(t *testing.T) {
	now := time.Date(2026, 4, 6, 2, 0, 0, 0, time.UTC)
	repo := &phaseFourTeamRepository{
		accounts: map[int64]teampkg.AccountRecord{
			7: {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
			9: {ID: 9, Email: "member@example.com", Status: "active"},
		},
		teams: map[int64]teampkg.TeamRecord{
			101: {
				ID:                101,
				OwnerAccountID:    7,
				UpstreamTeamID:    "team-up-101",
				UpstreamAccountID: "acct_101",
				TeamName:          "Alpha Team",
				PlanType:          "team",
				Status:            "active",
				CurrentMembers:    1,
				MaxMembers:        intPtr(5),
				SeatsAvailable:    intPtr(4),
				SyncStatus:        "success",
				UpdatedAt:         now,
			},
		},
		memberships: map[int64]teampkg.TeamMembershipRecord{
			201: {
				ID:               201,
				TeamID:           101,
				LocalAccountID:   int64Ptr(9),
				MemberEmail:      "member@example.com",
				MemberRole:       "member",
				MembershipStatus: "joined",
				JoinedAt:         timePtr(now.Add(-24 * time.Hour)),
				LastSeenAt:       timePtr(now.Add(-5 * time.Minute)),
				CreatedAt:        now.Add(-48 * time.Hour),
				UpdatedAt:        now,
			},
		},
		tasks:     map[string]teampkg.TeamTaskRecord{},
		taskItems: map[int64][]teampkg.TeamTaskItemRecord{},
	}
	jobService := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	readService := teampkg.NewService(repo, nil)
	taskService := teampkg.NewTaskService(repo, readService, jobService, phaseFourTeamExecutor{
		results: map[string]teampkg.TaskExecutionResult{
			"sync_all_teams": {
				Status: jobs.StatusCompleted,
				Summary: map[string]any{
					"synced": true,
					"detail": "sync finished",
				},
				Logs: []string{
					"sync started",
					"sync finished",
				},
				Items: []teampkg.TaskExecutionItem{
					{
						TargetEmail: "member@example.com",
						ItemStatus:  "completed",
						Before:      map[string]any{"membership_status": "joined"},
						After:       map[string]any{"membership_status": "joined"},
						Message:     "member state confirmed",
					},
				},
			},
		},
	})

	server := httptest.NewServer(internalhttp.NewRouter(jobService, readService, taskService))
	defer server.Close()

	teamsPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/teams", nil).(map[string]any)
	if teamsPayload["total"] != float64(1) {
		t.Fatalf("expected team list total=1, got %#v", teamsPayload)
	}
	detailPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/teams/101", nil).(map[string]any)
	if detailPayload["team_name"] != "Alpha Team" || detailPayload["active_member_count"] != float64(1) {
		t.Fatalf("unexpected team detail payload: %#v", detailPayload)
	}
	membershipPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/teams/101/memberships?binding=all", nil).(map[string]any)
	if membershipPayload["total"] != float64(1) {
		t.Fatalf("expected membership total=1, got %#v", membershipPayload)
	}

	acceptedPayload := mustRequestStatusJSON(t, server, http.StatusAccepted, http.MethodPost, "/api/team/teams/sync-batch", map[string]any{
		"ids": []int64{101},
	}).(map[string]any)
	taskUUID, _ := acceptedPayload["task_uuid"].(string)
	if taskUUID == "" || acceptedPayload["ws_channel"] != "/api/ws/task/"+taskUUID {
		t.Fatalf("unexpected accepted payload: %#v", acceptedPayload)
	}
	if acceptedPayload["team_id"] != float64(101) || acceptedPayload["accepted_count"] != float64(1) {
		t.Fatalf("expected sync-batch accepted payload to preserve team context, got %#v", acceptedPayload)
	}

	conn := dialTestWebSocket(t, server.URL+"/api/ws/task/"+taskUUID)
	defer conn.Close()

	initial := conn.readJSON(t)
	assertWebSocketMessageField(t, initial, "type", "status")
	assertWebSocketMessageField(t, initial, "task_uuid", taskUUID)
	assertWebSocketMessageField(t, initial, "status", jobs.StatusPending)

	done := make(chan error, 1)
	go func() {
		done <- taskService.ExecuteTask(context.Background(), taskUUID)
	}()

	var sawCompleted bool
	var sawLog bool
	deadline := time.Now().Add(2 * time.Second)
	for !(sawCompleted && sawLog) {
		if time.Now().After(deadline) {
			t.Fatalf("expected websocket live flow to deliver completed status and logs, got completed=%v log=%v", sawCompleted, sawLog)
		}
		message := conn.readJSON(t)
		switch message["type"] {
		case "status":
			if message["status"] == jobs.StatusCompleted {
				sawCompleted = true
			}
		case "log":
			sawLog = true
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("execute team task: %v", err)
	}

	tasksPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/tasks?team_id=101", nil).(map[string]any)
	if tasksPayload["total"] != float64(1) {
		t.Fatalf("expected task list total=1, got %#v", tasksPayload)
	}
	taskRows, ok := tasksPayload["items"].([]any)
	if !ok || len(taskRows) != 1 {
		t.Fatalf("expected one team task row, got %#v", tasksPayload["items"])
	}
	taskRow := taskRows[0].(map[string]any)
	if taskRow["task_uuid"] != taskUUID || taskRow["status"] != jobs.StatusCompleted {
		t.Fatalf("unexpected team task listing row: %#v", taskRow)
	}

	taskDetailPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/tasks/"+taskUUID, nil).(map[string]any)
	if taskDetailPayload["status"] != jobs.StatusCompleted {
		t.Fatalf("expected completed team task detail, got %#v", taskDetailPayload)
	}
	logs, ok := taskDetailPayload["logs"].([]any)
	if !ok || len(logs) < 2 {
		t.Fatalf("expected persisted team task logs, got %#v", taskDetailPayload["logs"])
	}
	items, ok := taskDetailPayload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one team task detail item, got %#v", taskDetailPayload["items"])
	}
}

func mustRequestStatusJSON(t *testing.T, server *httptest.Server, status int, method string, path string, body any) any {
	t.Helper()

	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s body: %v", method, path, err)
		}
	}

	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("build %s %s request: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		t.Fatalf("expected %s %s status %d, got %d", method, path, status, resp.StatusCode)
	}

	var decoded any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode %s %s response: %v", method, path, err)
	}
	return decoded
}

type phaseFourCheckoutGenerator struct{}

func (phaseFourCheckoutGenerator) GenerateCheckoutLink(_ context.Context, account accountspkg.Account, req paymentpkg.GenerateLinkRequest, _ string) (paymentpkg.CheckoutLinkResult, error) {
	return paymentpkg.CheckoutLinkResult{
		Link:              "https://chatgpt.com/pay/cs_phase4_" + strconv.Itoa(account.ID),
		Source:            "openai_checkout",
		CheckoutSessionID: "cs_phase4_" + strconv.Itoa(account.ID),
		PublishableKey:    "pk_test_phase4",
		ClientSecret:      "cs_secret_phase4",
	}, nil
}

type phaseFourBillingProfileGenerator struct{}

func (phaseFourBillingProfileGenerator) GenerateRandomBillingProfile(_ context.Context, country string, _ string) (map[string]any, error) {
	return map[string]any{
		"name":    "Ada",
		"country": country,
	}, nil
}

type phaseFourBrowserOpener struct{}

func (phaseFourBrowserOpener) OpenIncognito(_ context.Context, _ string, _ string) (bool, error) {
	return true, nil
}

type phaseFourSessionAdapter struct {
	result paymentpkg.SessionBootstrapResult
}

func (a phaseFourSessionAdapter) BootstrapSessionToken(_ context.Context, _ accountspkg.Account, _ string) (paymentpkg.SessionBootstrapResult, error) {
	return a.result, nil
}

func (phaseFourSessionAdapter) ProbeSession(_ context.Context, _ accountspkg.Account, _ string) (*paymentpkg.SessionProbeResult, error) {
	return &paymentpkg.SessionProbeResult{OK: true, HTTPStatus: http.StatusOK, SessionTokenFound: true}, nil
}

type phaseFourSubscriptionChecker struct {
	details map[int]paymentpkg.SubscriptionCheckDetail
}

func (c phaseFourSubscriptionChecker) CheckSubscription(_ context.Context, account accountspkg.Account, _ string, _ bool) (paymentpkg.SubscriptionCheckDetail, error) {
	if detail, ok := c.details[account.ID]; ok {
		return detail, nil
	}
	return paymentpkg.SubscriptionCheckDetail{Status: "free", Confidence: "high", Source: "chatgpt_web"}, nil
}

type phaseFourPaymentRepository struct {
	now           func() time.Time
	nextID        int
	accountEmails map[int]string
	tasks         map[int]paymentpkg.BindCardTask
}

func (r *phaseFourPaymentRepository) CreateBindCardTask(_ context.Context, params paymentpkg.CreateBindCardTaskParams) (paymentpkg.BindCardTask, error) {
	r.nextID++
	now := r.now()
	task := paymentpkg.BindCardTask{
		ID:                r.nextID,
		AccountID:         params.AccountID,
		AccountEmail:      r.accountEmails[params.AccountID],
		PlanType:          params.PlanType,
		WorkspaceName:     params.WorkspaceName,
		PriceInterval:     params.PriceInterval,
		SeatQuantity:      params.SeatQuantity,
		Country:           params.Country,
		Currency:          params.Currency,
		CheckoutURL:       params.CheckoutURL,
		CheckoutSessionID: params.CheckoutSessionID,
		PublishableKey:    params.PublishableKey,
		ClientSecret:      params.ClientSecret,
		CheckoutSource:    params.CheckoutSource,
		BindMode:          params.BindMode,
		Status:            params.Status,
		LastError:         params.LastError,
		OpenedAt:          params.OpenedAt,
		LastCheckedAt:     params.LastCheckedAt,
		CompletedAt:       params.CompletedAt,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	r.tasks[task.ID] = task
	return task, nil
}

func (r *phaseFourPaymentRepository) GetBindCardTask(_ context.Context, taskID int) (paymentpkg.BindCardTask, error) {
	task, ok := r.tasks[taskID]
	if !ok {
		return paymentpkg.BindCardTask{}, paymentpkg.ErrBindCardTaskNotFound
	}
	return task, nil
}

func (r *phaseFourPaymentRepository) ListBindCardTasks(_ context.Context, req paymentpkg.ListBindCardTasksRequest) (paymentpkg.ListBindCardTasksResponse, error) {
	filtered := make([]paymentpkg.BindCardTask, 0, len(r.tasks))
	search := strings.ToLower(strings.TrimSpace(req.Search))
	status := strings.TrimSpace(req.Status)
	for _, task := range r.tasks {
		if status != "" && task.Status != status {
			continue
		}
		if search != "" {
			text := strings.ToLower(task.AccountEmail + " " + task.WorkspaceName + " " + task.CheckoutURL)
			if !strings.Contains(text, search) {
				continue
			}
		}
		filtered = append(filtered, task)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID > filtered[j].ID })
	return paymentpkg.ListBindCardTasksResponse{
		Total: len(filtered),
		Tasks: filtered,
	}, nil
}

func (r *phaseFourPaymentRepository) UpdateBindCardTask(_ context.Context, task paymentpkg.BindCardTask) (paymentpkg.BindCardTask, error) {
	task.UpdatedAt = r.now()
	r.tasks[task.ID] = task
	return task, nil
}

func (r *phaseFourPaymentRepository) DeleteBindCardTask(_ context.Context, taskID int) error {
	if _, ok := r.tasks[taskID]; !ok {
		return paymentpkg.ErrBindCardTaskNotFound
	}
	delete(r.tasks, taskID)
	return nil
}

type phaseFourPaymentAccountsRepository struct {
	accounts map[int]accountspkg.Account
}

func (r *phaseFourPaymentAccountsRepository) GetAccountByID(_ context.Context, accountID int) (accountspkg.Account, error) {
	account, ok := r.accounts[accountID]
	if !ok {
		return accountspkg.Account{}, accountspkg.ErrAccountNotFound
	}
	return account, nil
}

func (r *phaseFourPaymentAccountsRepository) ListAccountsBySelection(_ context.Context, req accountspkg.AccountSelectionRequest) ([]accountspkg.Account, error) {
	items := make([]accountspkg.Account, 0)
	for _, id := range req.IDs {
		if account, ok := r.accounts[id]; ok {
			items = append(items, account)
		}
	}
	return items, nil
}

func (r *phaseFourPaymentAccountsRepository) UpsertAccount(_ context.Context, account accountspkg.Account) (accountspkg.Account, error) {
	r.accounts[account.ID] = account
	return account, nil
}

type phaseFourTeamRepository struct {
	accounts       map[int64]teampkg.AccountRecord
	teams          map[int64]teampkg.TeamRecord
	memberships    map[int64]teampkg.TeamMembershipRecord
	tasks          map[string]teampkg.TeamTaskRecord
	taskItems      map[int64][]teampkg.TeamTaskItemRecord
	nextTaskID     int64
	nextTaskItemID int64
}

func (r *phaseFourTeamRepository) ListTeams(_ context.Context, req teampkg.ListTeamsRequest) ([]teampkg.TeamRecord, int, error) {
	items := make([]teampkg.TeamRecord, 0, len(r.teams))
	for _, record := range r.teams {
		if req.OwnerAccountID > 0 && record.OwnerAccountID != req.OwnerAccountID {
			continue
		}
		if strings.TrimSpace(req.Status) != "" && record.Status != req.Status {
			continue
		}
		if search := strings.ToLower(strings.TrimSpace(req.Search)); search != "" {
			if !strings.Contains(strings.ToLower(record.TeamName), search) && !strings.Contains(strings.ToLower(record.UpstreamAccountID), search) {
				continue
			}
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, len(items), nil
}

func (r *phaseFourTeamRepository) GetTeam(_ context.Context, teamID int64) (teampkg.TeamRecord, error) {
	record, ok := r.teams[teamID]
	if !ok {
		return teampkg.TeamRecord{}, teampkg.ErrNotFound
	}
	return record, nil
}

func (r *phaseFourTeamRepository) GetAccount(_ context.Context, accountID int64) (teampkg.AccountRecord, error) {
	record, ok := r.accounts[accountID]
	if !ok {
		return teampkg.AccountRecord{}, teampkg.ErrNotFound
	}
	return record, nil
}

func (r *phaseFourTeamRepository) ListAccountsByIDs(_ context.Context, accountIDs []int64) (map[int64]teampkg.AccountRecord, error) {
	result := make(map[int64]teampkg.AccountRecord, len(accountIDs))
	for _, id := range accountIDs {
		if record, ok := r.accounts[id]; ok {
			result[id] = record
		}
	}
	return result, nil
}

func (r *phaseFourTeamRepository) ListMembershipsByTeam(_ context.Context, teamID int64) ([]teampkg.TeamMembershipRecord, error) {
	items := make([]teampkg.TeamMembershipRecord, 0)
	for _, membership := range r.memberships {
		if membership.TeamID == teamID {
			items = append(items, membership)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (r *phaseFourTeamRepository) GetMembership(_ context.Context, membershipID int64) (teampkg.TeamMembershipRecord, error) {
	record, ok := r.memberships[membershipID]
	if !ok {
		return teampkg.TeamMembershipRecord{}, teampkg.ErrNotFound
	}
	return record, nil
}

func (r *phaseFourTeamRepository) SaveMembership(_ context.Context, membership teampkg.TeamMembershipRecord) (teampkg.TeamMembershipRecord, error) {
	r.memberships[membership.ID] = membership
	return membership, nil
}

func (r *phaseFourTeamRepository) SaveTeam(_ context.Context, team teampkg.TeamRecord) (teampkg.TeamRecord, error) {
	r.teams[team.ID] = team
	return team, nil
}

func (r *phaseFourTeamRepository) ListTasks(_ context.Context, req teampkg.ListTasksRequest) ([]teampkg.TeamTaskRecord, error) {
	items := make([]teampkg.TeamTaskRecord, 0)
	for _, task := range r.tasks {
		if req.TeamID != nil {
			if task.TeamID == nil || *task.TeamID != *req.TeamID {
				continue
			}
		}
		items = append(items, task)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (r *phaseFourTeamRepository) GetTaskByUUID(_ context.Context, taskUUID string) (teampkg.TeamTaskRecord, error) {
	record, ok := r.tasks[taskUUID]
	if !ok {
		return teampkg.TeamTaskRecord{}, teampkg.ErrNotFound
	}
	return record, nil
}

func (r *phaseFourTeamRepository) ListTaskItems(_ context.Context, taskID int64) ([]teampkg.TeamTaskItemRecord, error) {
	return append([]teampkg.TeamTaskItemRecord(nil), r.taskItems[taskID]...), nil
}

func (r *phaseFourTeamRepository) CreateTask(_ context.Context, task teampkg.TeamTaskRecord) (teampkg.TeamTaskRecord, error) {
	scopeKey := ""
	if task.ActiveScopeKey != nil {
		scopeKey = *task.ActiveScopeKey
	}
	for _, existing := range r.tasks {
		if existing.ActiveScopeKey == nil || scopeKey == "" {
			continue
		}
		if *existing.ActiveScopeKey == scopeKey && existing.Status != jobs.StatusCompleted && existing.Status != jobs.StatusFailed && existing.Status != jobs.StatusCancelled {
			return teampkg.TeamTaskRecord{}, teampkg.ErrActiveScopeConflict
		}
	}
	r.nextTaskID++
	task.ID = r.nextTaskID
	r.tasks[task.TaskUUID] = task
	return task, nil
}

func (r *phaseFourTeamRepository) SaveTask(_ context.Context, task teampkg.TeamTaskRecord) (teampkg.TeamTaskRecord, error) {
	r.tasks[task.TaskUUID] = task
	return task, nil
}

func (r *phaseFourTeamRepository) SaveTaskItem(_ context.Context, item teampkg.TeamTaskItemRecord) (teampkg.TeamTaskItemRecord, error) {
	r.nextTaskItemID++
	item.ID = r.nextTaskItemID
	r.taskItems[item.TaskID] = append(r.taskItems[item.TaskID], item)
	return item, nil
}

func (r *phaseFourTeamRepository) FindActiveTask(_ context.Context, scopeType string, scopeID string, taskType string) (teampkg.TeamTaskRecord, error) {
	for _, task := range r.tasks {
		if task.ScopeType != scopeType || task.ScopeID != scopeID {
			continue
		}
		if taskType != "" && task.TaskType != taskType {
			continue
		}
		if task.Status != jobs.StatusCompleted && task.Status != jobs.StatusFailed && task.Status != jobs.StatusCancelled {
			return task, nil
		}
	}
	return teampkg.TeamTaskRecord{}, teampkg.ErrNotFound
}

type phaseFourTeamExecutor struct {
	results map[string]teampkg.TaskExecutionResult
}

func (e phaseFourTeamExecutor) Execute(_ context.Context, task teampkg.TaskExecutionRequest) (teampkg.TaskExecutionResult, error) {
	result, ok := e.results[task.TaskType]
	if !ok {
		return teampkg.TaskExecutionResult{}, teampkg.ErrNotFound
	}
	return result, nil
}

func int64Ptr(value int64) *int64 {
	return &value
}
