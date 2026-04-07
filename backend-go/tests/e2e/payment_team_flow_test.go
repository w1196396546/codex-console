package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
				ExtraData: map[string]any{
					"payment_subscription_status":     "team",
					"payment_subscription_confidence": "high",
					"payment_subscription_source":     "chatgpt_web",
				},
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
				ExtraData: map[string]any{
					"payment_subscription_status":     "team",
					"payment_subscription_confidence": "high",
					"payment_subscription_source":     "chatgpt_web",
				},
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
	adapters := paymentpkg.NewTransitionAdapters()
	service := paymentpkg.NewService(
		repo,
		accountRepo,
		paymentpkg.WithNow(func() time.Time { return now }),
		paymentpkg.WithCheckoutLinkGenerator(adapters.CheckoutLinkGenerator),
		paymentpkg.WithBillingProfileGenerator(adapters.BillingProfileGenerator),
		paymentpkg.WithBrowserOpener(adapters.BrowserOpener),
		paymentpkg.WithSessionAdapter(adapters.SessionAdapter),
		paymentpkg.WithSubscriptionChecker(adapters.SubscriptionChecker),
		paymentpkg.WithAutoBinder(adapters.AutoBinder),
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
	link, _ := generatePayload["link"].(string)
	if !strings.Contains(link, "/checkout/openai_llc/cs_transition_team_7_ops") || generatePayload["source"] != "openai_checkout" {
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
	checkoutURL, _ := task["checkout_url"].(string)
	if task["status"] != paymentpkg.StatusLinkReady || !strings.Contains(checkoutURL, "/checkout/openai_llc/cs_transition_team_7_ops") {
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

	openPayload := mustRequestStatusJSON(t, server, http.StatusInternalServerError, http.MethodPost, "/api/payment/bind-card/tasks/1/open", map[string]any{}).(map[string]any)
	if openPayload["detail"] != "未找到可用的浏览器，请手动复制链接" {
		t.Fatalf("expected truthful no-browser fallback detail, got %#v", openPayload)
	}
	postOpenListPayload := mustRequestJSON(t, server, http.MethodGet, "/api/payment/bind-card/tasks?page=1&page_size=20&status=link_ready&search=ops@example.com", nil).(map[string]any)
	postOpenTasks, ok := postOpenListPayload["tasks"].([]any)
	if !ok || len(postOpenTasks) != 1 {
		t.Fatalf("expected one bind-card task row after failed open, got %#v", postOpenListPayload["tasks"])
	}
	postOpenTask := postOpenTasks[0].(map[string]any)
	if postOpenTask["status"] != paymentpkg.StatusLinkReady || postOpenTask["last_error"] != "未找到可用的浏览器" {
		t.Fatalf("expected failed open to preserve link_ready with last_error, got %#v", postOpenTask)
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
	if bootstrapPayload["success"] != true {
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
	installPhaseFourTeamHTTPStub(t, func(req *http.Request) (*http.Response, error) {
		if auth := req.Header.Get("Authorization"); auth != "Bearer owner-token" {
			t.Fatalf("unexpected transition authorization header: %q", auth)
		}
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/backend-api/accounts/check/v4-2023-04-27":
			return phaseFourTeamJSONResponse(http.StatusOK, map[string]any{
				"accounts": map[string]any{
					"acct_101": map[string]any{
						"account": map[string]any{
							"plan_type":         "team",
							"name":              "Alpha Team",
							"account_user_role": "account-owner",
						},
						"entitlement": map[string]any{
							"subscription_plan": "chatgpt-team",
						},
					},
					"acct_202": map[string]any{
						"account": map[string]any{
							"plan_type":         "team",
							"name":              "Beta Team",
							"account_user_role": "account-owner",
						},
						"entitlement": map[string]any{
							"subscription_plan": "chatgpt-team",
						},
					},
				},
			})
		case req.Method == http.MethodGet && req.URL.Path == "/backend-api/accounts/acct_101/users":
			return phaseFourTeamJSONResponse(http.StatusOK, map[string]any{
				"items": []any{
					map[string]any{
						"id":           "user-9",
						"email":        "member@example.com",
						"role":         "member",
						"created_time": "2026-04-05T00:00:00Z",
					},
				},
				"total": 1,
				"limit": 100,
			})
		case req.Method == http.MethodGet && req.URL.Path == "/backend-api/accounts/acct_101/invites":
			return phaseFourTeamJSONResponse(http.StatusOK, map[string]any{
				"items": []any{
					map[string]any{
						"email_address": "pending@example.com",
						"role":          "member",
						"created_time":  "2026-04-05T01:00:00Z",
					},
				},
			})
		case req.Method == http.MethodGet && req.URL.Path == "/backend-api/accounts/acct_202/users":
			return phaseFourTeamJSONResponse(http.StatusOK, map[string]any{
				"items": []any{
					map[string]any{
						"id":           "user-11",
						"email":        "second@example.com",
						"role":         "member",
						"created_time": "2026-04-05T02:00:00Z",
					},
				},
				"total": 1,
				"limit": 100,
			})
		case req.Method == http.MethodGet && req.URL.Path == "/backend-api/accounts/acct_202/invites":
			return phaseFourTeamJSONResponse(http.StatusOK, map[string]any{
				"items": []any{},
			})
		case req.Method == http.MethodPost && req.URL.Path == "/backend-api/accounts/acct_101/invites":
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode invite payload: %v", err)
			}
			emails, ok := payload["email_addresses"].([]any)
			if !ok || len(emails) != 1 || emails[0] != "child@example.com" {
				t.Fatalf("unexpected invite payload: %#v", payload)
			}
			return phaseFourTeamJSONResponse(http.StatusOK, map[string]any{"ok": true})
		default:
			t.Fatalf("unexpected transition request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})

	repo := &phaseFourTeamRepository{
		accounts: map[int64]teampkg.AccountRecord{
			7:  {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
			9:  {ID: 9, Email: "member@example.com", Status: "active"},
			10: {ID: 10, Email: "child@example.com", Status: "active"},
			11: {ID: 11, Email: "second@example.com", Status: "active"},
		},
		teams:       map[int64]teampkg.TeamRecord{},
		memberships: map[int64]teampkg.TeamMembershipRecord{},
		tasks:       map[string]teampkg.TeamTaskRecord{},
		taskItems:   map[int64][]teampkg.TeamTaskItemRecord{},
	}
	jobService := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	readService := teampkg.NewService(repo, nil)
	taskService := teampkg.NewTaskService(repo, readService, jobService, teampkg.NewTransitionTaskExecutor(repo, teampkg.TransitionExecutorHooks{}))

	server := httptest.NewServer(internalhttp.NewRouter(jobService, readService, taskService))
	defer server.Close()

	discoveryPayload := mustRequestStatusJSON(t, server, http.StatusAccepted, http.MethodPost, "/api/team/discovery/7", nil).(map[string]any)
	discoveryTaskUUID, _ := discoveryPayload["task_uuid"].(string)
	discoveryDetail := waitForPhaseFourTeamTask(t, server, discoveryTaskUUID)
	if discoveryDetail["status"] != jobs.StatusCompleted {
		t.Fatalf("expected discovery task to complete, got %#v", discoveryDetail)
	}

	teamsPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/teams", nil).(map[string]any)
	if teamsPayload["total"] != float64(2) {
		t.Fatalf("expected team list total=2, got %#v", teamsPayload)
	}
	teamRows, ok := teamsPayload["items"].([]any)
	if !ok || len(teamRows) != 2 {
		t.Fatalf("expected two discovered team rows, got %#v", teamsPayload["items"])
	}
	teamIDsByUpstream := make(map[string]int64, len(teamRows))
	for _, row := range teamRows {
		record := row.(map[string]any)
		upstreamID, _ := record["upstream_account_id"].(string)
		teamIDsByUpstream[upstreamID] = int64(record["id"].(float64))
	}
	alphaTeamID := teamIDsByUpstream["acct_101"]
	betaTeamID := teamIDsByUpstream["acct_202"]
	if alphaTeamID == 0 || betaTeamID == 0 {
		t.Fatalf("expected discovery to persist both upstream teams, got %#v", teamIDsByUpstream)
	}
	detailPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/teams/"+strconv.FormatInt(alphaTeamID, 10), nil).(map[string]any)
	if detailPayload["team_name"] != "Alpha Team" || detailPayload["active_member_count"] != float64(0) {
		t.Fatalf("unexpected alpha team detail payload: %#v", detailPayload)
	}
	betaDetailPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/teams/"+strconv.FormatInt(betaTeamID, 10), nil).(map[string]any)
	if betaDetailPayload["team_name"] != "Beta Team" || betaDetailPayload["active_member_count"] != float64(0) {
		t.Fatalf("unexpected beta team detail payload: %#v", betaDetailPayload)
	}
	if repoTeam := repo.teams[alphaTeamID]; repoTeam.UpstreamAccountID != "acct_101" || repoTeam.SyncStatus != "synced" {
		t.Fatalf("expected alpha discovery state, got %#v", repoTeam)
	}
	if repoTeam := repo.teams[betaTeamID]; repoTeam.UpstreamAccountID != "acct_202" || repoTeam.SyncStatus != "synced" {
		t.Fatalf("expected beta discovery state, got %#v", repoTeam)
	}

	acceptedPayload := mustRequestStatusJSON(t, server, http.StatusAccepted, http.MethodPost, "/api/team/teams/sync-batch", map[string]any{
		"ids": []int64{alphaTeamID, betaTeamID},
	}).(map[string]any)
	taskUUID, _ := acceptedPayload["task_uuid"].(string)
	if taskUUID == "" || acceptedPayload["ws_channel"] != "/api/ws/task/"+taskUUID {
		t.Fatalf("unexpected accepted payload: %#v", acceptedPayload)
	}
	if acceptedPayload["team_id"] != float64(alphaTeamID) || acceptedPayload["accepted_count"] != float64(2) {
		t.Fatalf("expected sync-batch accepted payload to preserve team context, got %#v", acceptedPayload)
	}

	conn := dialTestWebSocket(t, server.URL+"/api/ws/task/"+taskUUID)
	defer conn.Close()

	initial := conn.readJSON(t)
	assertWebSocketMessageField(t, initial, "type", "status")
	assertWebSocketMessageField(t, initial, "task_uuid", taskUUID)
	initialStatus, _ := initial["status"].(string)
	if initialStatus == "" {
		t.Fatalf("expected initial websocket status payload, got %#v", initial)
	}
	if initialStatus != jobs.StatusCompleted {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			message := conn.readJSON(t)
			if message["type"] == "status" && message["status"] == jobs.StatusCompleted {
				break
			}
		}
	}

	syncMembershipPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/teams/"+strconv.FormatInt(alphaTeamID, 10)+"/memberships?binding=all", nil).(map[string]any)
	if syncMembershipPayload["total"] != float64(2) {
		t.Fatalf("expected alpha sync to persist two memberships, got %#v", syncMembershipPayload)
	}
	detailPayload = mustRequestJSON(t, server, http.MethodGet, "/api/team/teams/"+strconv.FormatInt(alphaTeamID, 10), nil).(map[string]any)
	if detailPayload["active_member_count"] != float64(2) || detailPayload["joined_count"] != float64(1) || detailPayload["invited_count"] != float64(1) {
		t.Fatalf("expected alpha sync detail aggregates to update, got %#v", detailPayload)
	}
	if repoMembership := findPhaseFourTeamMembership(repo.memberships, "member@example.com"); repoMembership.TeamID != alphaTeamID || repoMembership.LocalAccountID == nil || *repoMembership.LocalAccountID != 9 || repoMembership.MembershipStatus != "joined" {
		t.Fatalf("expected alpha sync to persist joined local membership, got %#v", repoMembership)
	}
	betaMembershipPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/teams/"+strconv.FormatInt(betaTeamID, 10)+"/memberships?binding=all", nil).(map[string]any)
	if betaMembershipPayload["total"] != float64(1) {
		t.Fatalf("expected beta sync to persist one membership, got %#v", betaMembershipPayload)
	}
	betaDetailPayload = mustRequestJSON(t, server, http.MethodGet, "/api/team/teams/"+strconv.FormatInt(betaTeamID, 10), nil).(map[string]any)
	if betaDetailPayload["joined_count"] != float64(1) || betaDetailPayload["invited_count"] != float64(0) {
		t.Fatalf("expected beta sync detail aggregates to update, got %#v", betaDetailPayload)
	}
	if repoMembership := findPhaseFourTeamMembership(repo.memberships, "second@example.com"); repoMembership.TeamID != betaTeamID || repoMembership.LocalAccountID == nil || *repoMembership.LocalAccountID != 11 || repoMembership.MembershipStatus != "joined" {
		t.Fatalf("expected beta sync to persist joined local membership, got %#v", repoMembership)
	}

	invitePayload := mustRequestStatusJSON(t, server, http.StatusAccepted, http.MethodPost, "/api/team/teams/"+strconv.FormatInt(alphaTeamID, 10)+"/invite-accounts", map[string]any{
		"ids":        []int64{10},
		"select_all": false,
	}).(map[string]any)
	inviteTaskUUID, _ := invitePayload["task_uuid"].(string)
	inviteDetailPayload := waitForPhaseFourTeamTask(t, server, inviteTaskUUID)
	if inviteDetailPayload["status"] != jobs.StatusCompleted {
		t.Fatalf("expected invite task to complete, got %#v", inviteDetailPayload)
	}

	membershipPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/teams/"+strconv.FormatInt(alphaTeamID, 10)+"/memberships?binding=all", nil).(map[string]any)
	if membershipPayload["total"] != float64(3) {
		t.Fatalf("expected invite to persist third membership, got %#v", membershipPayload)
	}
	if repoMembership := findPhaseFourTeamMembership(repo.memberships, "child@example.com"); repoMembership.TeamID != alphaTeamID || repoMembership.LocalAccountID == nil || *repoMembership.LocalAccountID != 10 || repoMembership.MembershipStatus != "invited" {
		t.Fatalf("expected invite to persist local membership, got %#v", repoMembership)
	}

	tasksPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/tasks?team_id="+strconv.FormatInt(alphaTeamID, 10), nil).(map[string]any)
	if tasksPayload["total"] != float64(2) {
		t.Fatalf("expected task list total=1, got %#v", tasksPayload)
	}
	taskRows, ok := tasksPayload["items"].([]any)
	if !ok || len(taskRows) != 2 {
		t.Fatalf("expected two team-scoped task rows, got %#v", tasksPayload["items"])
	}
	taskRow := taskRows[0].(map[string]any)
	if taskRow["status"] != jobs.StatusCompleted {
		t.Fatalf("unexpected team task listing row: %#v", taskRow)
	}

	taskDetailPayload := mustRequestJSON(t, server, http.MethodGet, "/api/team/tasks/"+inviteTaskUUID, nil).(map[string]any)
	if taskDetailPayload["status"] != jobs.StatusCompleted {
		t.Fatalf("expected completed team task detail, got %#v", taskDetailPayload)
	}
	logs, ok := taskDetailPayload["logs"].([]any)
	if !ok || len(logs) < 3 {
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
	nextTeamID     int64
	nextMemberID   int64
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

func (r *phaseFourTeamRepository) UpsertMembership(_ context.Context, membership teampkg.TeamMembershipRecord) (teampkg.TeamMembershipRecord, error) {
	for id, existing := range r.memberships {
		if existing.TeamID == membership.TeamID && strings.EqualFold(existing.MemberEmail, membership.MemberEmail) {
			membership.ID = id
			r.memberships[id] = membership
			return membership, nil
		}
	}
	r.nextMemberID++
	membership.ID = r.nextMemberID
	r.memberships[membership.ID] = membership
	return membership, nil
}

func (r *phaseFourTeamRepository) SaveTeam(_ context.Context, team teampkg.TeamRecord) (teampkg.TeamRecord, error) {
	r.teams[team.ID] = team
	return team, nil
}

func (r *phaseFourTeamRepository) UpsertTeam(_ context.Context, team teampkg.TeamRecord) (teampkg.TeamRecord, error) {
	for id, existing := range r.teams {
		if existing.OwnerAccountID == team.OwnerAccountID && strings.EqualFold(existing.UpstreamAccountID, team.UpstreamAccountID) {
			team.ID = id
			r.teams[id] = team
			return team, nil
		}
	}
	r.nextTeamID++
	team.ID = r.nextTeamID
	r.teams[team.ID] = team
	return team, nil
}

func (r *phaseFourTeamRepository) ListAccountsByEmails(_ context.Context, emails []string) (map[string]teampkg.AccountRecord, error) {
	result := make(map[string]teampkg.AccountRecord, len(emails))
	for _, email := range emails {
		normalized := strings.ToLower(strings.TrimSpace(email))
		for _, record := range r.accounts {
			if strings.ToLower(strings.TrimSpace(record.Email)) == normalized {
				result[normalized] = record
				break
			}
		}
	}
	return result, nil
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

func int64Ptr(value int64) *int64 {
	return &value
}

func installPhaseFourTeamHTTPStub(t *testing.T, fn func(req *http.Request) (*http.Response, error)) {
	t.Helper()

	previous := http.DefaultTransport
	http.DefaultTransport = phaseFourTeamRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "chatgpt.com" {
			return previous.RoundTrip(req)
		}
		return fn(req)
	})
	t.Cleanup(func() {
		http.DefaultTransport = previous
	})
}

type phaseFourTeamRoundTripFunc func(req *http.Request) (*http.Response, error)

func (fn phaseFourTeamRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func phaseFourTeamJSONResponse(status int, payload map[string]any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func waitForPhaseFourTeamTask(t *testing.T, server *httptest.Server, taskUUID string) map[string]any {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		payload := mustRequestJSON(t, server, http.MethodGet, "/api/team/tasks/"+taskUUID, nil).(map[string]any)
		status, _ := payload["status"].(string)
		if status != jobs.StatusPending && status != jobs.StatusRunning {
			return payload
		}
		time.Sleep(10 * time.Millisecond)
	}

	return mustRequestJSON(t, server, http.MethodGet, "/api/team/tasks/"+taskUUID, nil).(map[string]any)
}

func findPhaseFourTeamMembership(records map[int64]teampkg.TeamMembershipRecord, email string) teampkg.TeamMembershipRecord {
	normalized := strings.ToLower(strings.TrimSpace(email))
	for _, record := range records {
		if strings.ToLower(strings.TrimSpace(record.MemberEmail)) == normalized {
			return record
		}
	}
	return teampkg.TeamMembershipRecord{}
}
