package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	paymentpkg "github.com/dou-jiang/codex-console/backend-go/internal/payment"
	paymenthttp "github.com/dou-jiang/codex-console/backend-go/internal/payment/http"
	"github.com/go-chi/chi/v5"
)

func TestPaymentHandlerCompatibilityRoutes(t *testing.T) {
	service := &fakePaymentService{
		randomBillingResp: paymentpkg.RandomBillingResponse{
			Success: true,
			Profile: map[string]any{
				"name":    "Ada",
				"country": "GB",
			},
		},
		generateLinkResp: paymentpkg.GenerateLinkResponse{
			Success:            true,
			Link:               "https://chatgpt.com/pay/cs_test_123",
			IsOfficialCheckout: true,
			PlanType:           "team",
			Country:            "GB",
			Currency:           "GBP",
			Source:             "openai_checkout",
			CheckoutSessionID:  "cs_test_123",
			HasClientSecret:    true,
		},
		openIncognitoResp: paymentpkg.OpenIncognitoResponse{
			Success: false,
			Message: "未找到可用浏览器，请手动复制链接",
		},
	}
	router := newTestRouter(service)

	randomRec := httptest.NewRecorder()
	randomReq := httptest.NewRequest(http.MethodGet, "/api/payment/random-billing?country=GB&proxy=http://proxy.example", nil)
	router.ServeHTTP(randomRec, randomReq)
	if randomRec.Code != http.StatusOK {
		t.Fatalf("expected random billing 200, got %d", randomRec.Code)
	}
	if service.randomBillingCountry != "GB" || service.randomBillingProxy != "http://proxy.example" {
		t.Fatalf("unexpected random billing request: country=%q proxy=%q", service.randomBillingCountry, service.randomBillingProxy)
	}

	generateRec := httptest.NewRecorder()
	generateReq := httptest.NewRequest(http.MethodPost, "/api/payment/generate-link", bytes.NewBufferString(`{
		"account_id": 7,
		"plan_type": "team",
		"workspace_name": "Ops",
		"price_interval": "month",
		"seat_quantity": 5,
		"country": "GB",
		"currency": "GBP"
	}`))
	generateReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(generateRec, generateReq)
	if generateRec.Code != http.StatusOK {
		t.Fatalf("expected generate-link 200, got %d", generateRec.Code)
	}
	if service.generateLinkReq.AccountID != 7 || service.generateLinkReq.PlanType != "team" || service.generateLinkReq.WorkspaceName != "Ops" {
		t.Fatalf("unexpected generate-link request: %+v", service.generateLinkReq)
	}
	var generatePayload map[string]any
	if err := json.Unmarshal(generateRec.Body.Bytes(), &generatePayload); err != nil {
		t.Fatalf("decode generate-link payload: %v", err)
	}
	if generatePayload["link"] != "https://chatgpt.com/pay/cs_test_123" || generatePayload["source"] != "openai_checkout" || generatePayload["checkout_session_id"] != "cs_test_123" {
		t.Fatalf("unexpected generate-link payload: %#v", generatePayload)
	}

	openRec := httptest.NewRecorder()
	openReq := httptest.NewRequest(http.MethodPost, "/api/payment/open-incognito", bytes.NewBufferString(`{"url":"https://chatgpt.com/pay/cs_test_123","account_id":7}`))
	openReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(openRec, openReq)
	if openRec.Code != http.StatusOK {
		t.Fatalf("expected open-incognito 200, got %d", openRec.Code)
	}
	if service.openIncognitoReq.URL != "https://chatgpt.com/pay/cs_test_123" || service.openIncognitoReq.AccountID != 7 {
		t.Fatalf("unexpected open-incognito request: %+v", service.openIncognitoReq)
	}

	invalidRec := httptest.NewRecorder()
	invalidReq := httptest.NewRequest(http.MethodPost, "/api/payment/open-incognito", bytes.NewBufferString(`{}`))
	invalidReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid open-incognito 400, got %d", invalidRec.Code)
	}
	assertDetailMessage(t, invalidRec.Body.Bytes(), "URL 不能为空")
}

func TestSessionHandlerCompatibilityRoutes(t *testing.T) {
	service := &fakePaymentService{
		sessionDiagnosticResp: paymentpkg.SessionDiagnosticResponse{
			Success: true,
			Diagnostic: paymentpkg.SessionDiagnosticPayload{
				AccountID: 9,
				Email:     "diag@example.com",
				TokenState: map[string]any{
					"has_access_token": true,
				},
				CookieState: map[string]any{
					"has_oai_did": true,
				},
				BootstrapCapability: map[string]any{
					"can_login_bootstrap": true,
				},
				Notes:          []string{"ok"},
				Recommendation: "直接执行",
				CheckedAt:      "2026-04-05T09:00:00Z",
			},
		},
		sessionBootstrapResp: paymentpkg.SessionBootstrapResponse{
			Success:         true,
			Message:         "会话补全成功",
			AccountID:       9,
			Email:           "diag@example.com",
			SessionTokenLen: 12,
		},
		saveSessionTokenResp: paymentpkg.SaveSessionTokenResponse{
			Success:         true,
			AccountID:       9,
			Email:           "diag@example.com",
			SessionTokenLen: 12,
			Message:         "session_token 已保存",
		},
	}
	router := newTestRouter(service)

	diagRec := httptest.NewRecorder()
	diagReq := httptest.NewRequest(http.MethodGet, "/api/payment/accounts/9/session-diagnostic?probe=1&proxy=http://proxy.example", nil)
	router.ServeHTTP(diagRec, diagReq)
	if diagRec.Code != http.StatusOK {
		t.Fatalf("expected session diagnostic 200, got %d", diagRec.Code)
	}
	if service.sessionDiagnosticAccountID != 9 || !service.sessionDiagnosticProbe || service.sessionDiagnosticProxy != "http://proxy.example" {
		t.Fatalf("unexpected diagnostic request capture: id=%d probe=%v proxy=%q", service.sessionDiagnosticAccountID, service.sessionDiagnosticProbe, service.sessionDiagnosticProxy)
	}

	bootstrapRec := httptest.NewRecorder()
	bootstrapReq := httptest.NewRequest(http.MethodPost, "/api/payment/accounts/9/session-bootstrap", nil)
	router.ServeHTTP(bootstrapRec, bootstrapReq)
	if bootstrapRec.Code != http.StatusOK {
		t.Fatalf("expected session bootstrap 200, got %d", bootstrapRec.Code)
	}
	if service.sessionBootstrapAccountID != 9 {
		t.Fatalf("unexpected bootstrap request capture: %d", service.sessionBootstrapAccountID)
	}

	saveRec := httptest.NewRecorder()
	saveReq := httptest.NewRequest(http.MethodPost, "/api/payment/accounts/9/session-token", bytes.NewBufferString(`{"session_token":"session-123","merge_cookie":true}`))
	saveReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("expected session token save 200, got %d", saveRec.Code)
	}
	if service.saveSessionTokenAccountID != 9 || service.saveSessionTokenReq.SessionToken != "session-123" || !service.saveSessionTokenReq.MergeCookie {
		t.Fatalf("unexpected session-token request: account=%d req=%+v", service.saveSessionTokenAccountID, service.saveSessionTokenReq)
	}

	failService := &fakePaymentService{
		sessionBootstrapErr: errors.New("boom"),
	}
	failRouter := newTestRouter(failService)
	failRec := httptest.NewRecorder()
	failReq := httptest.NewRequest(http.MethodPost, "/api/payment/accounts/9/session-bootstrap", nil)
	failRouter.ServeHTTP(failRec, failReq)
	if failRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected bootstrap error 500, got %d", failRec.Code)
	}
	assertDetailMessage(t, failRec.Body.Bytes(), "boom")
}

func TestBindCardTaskHandlerCompatibilityRoutes(t *testing.T) {
	service := &fakePaymentService{
		createTaskResp: paymentpkg.CreateBindCardTaskResponse{
			Success: true,
			Task: paymentpkg.BindCardTask{
				ID:           11,
				AccountID:    7,
				AccountEmail: "alpha@example.com",
				PlanType:     "team",
				CheckoutURL:  "https://chatgpt.com/pay/cs_bind_11",
				Status:       paymentpkg.StatusLinkReady,
			},
			Link:              "https://chatgpt.com/pay/cs_bind_11",
			Source:            "openai_checkout",
			CheckoutSessionID: "cs_bind_11",
			HasClientSecret:   true,
		},
		listTasksResp: paymentpkg.ListBindCardTasksResponse{
			Total: 1,
			Tasks: []paymentpkg.BindCardTask{
				{
					ID:           11,
					AccountID:    7,
					AccountEmail: "alpha@example.com",
					PlanType:     "team",
					CheckoutURL:  "https://chatgpt.com/pay/cs_bind_11",
					Status:       paymentpkg.StatusOpened,
				},
			},
		},
		openTaskResp: paymentpkg.BindCardTaskActionResponse{
			Success: true,
			Task: paymentpkg.BindCardTask{
				ID:      11,
				Status:  paymentpkg.StatusOpened,
				BindMode:"semi_auto",
			},
		},
		thirdPartyResp: paymentpkg.AutoBindResult{
			PaidConfirmed: true,
			Task: paymentpkg.BindCardTask{
				ID:     11,
				Status: paymentpkg.StatusPaidPendingSync,
			},
		},
		localResp: paymentpkg.AutoBindResult{
			Verified: true,
			Task: paymentpkg.BindCardTask{
				ID:     11,
				Status: paymentpkg.StatusCompleted,
			},
			SubscriptionType: "plus",
		},
		markUserActionResp: paymentpkg.SyncBindCardTaskResponse{
			Success:          true,
			Verified:         true,
			SubscriptionType: "team",
			Task: paymentpkg.BindCardTask{
				ID:     11,
				Status: paymentpkg.StatusCompleted,
			},
		},
		syncTaskResp: paymentpkg.SyncBindCardTaskResponse{
			Success:          true,
			SubscriptionType: "free",
			Detail: map[string]any{
				"source":     "chatgpt_web",
				"confidence": "low",
			},
			Task: paymentpkg.BindCardTask{
				ID:     11,
				Status: paymentpkg.StatusPaidPendingSync,
			},
		},
		deleteTaskResp: paymentpkg.DeleteBindCardTaskResponse{
			Success: true,
			TaskID:  11,
		},
	}
	router := newTestRouter(service)

	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/payment/bind-card/tasks", bytes.NewBufferString(`{
		"account_id": 7,
		"plan_type": "team",
		"workspace_name": "Ops",
		"price_interval": "month",
		"seat_quantity": 5,
		"country": "GB",
		"currency": "GBP",
		"bind_mode": "third_party"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected create bind-card task 200, got %d", createRec.Code)
	}
	if service.createTaskReq.PlanType != "team" || service.createTaskReq.BindMode != "third_party" || service.createTaskReq.SeatQuantity != 5 {
		t.Fatalf("unexpected create task request: %+v", service.createTaskReq)
	}

	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/payment/bind-card/tasks?page=2&page_size=30&status=opened&search=alpha", nil)
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list bind-card tasks 200, got %d", listRec.Code)
	}
	if service.listTasksReq.Page != 2 || service.listTasksReq.PageSize != 30 || service.listTasksReq.Status != "opened" || service.listTasksReq.Search != "alpha" {
		t.Fatalf("unexpected list task request: %+v", service.listTasksReq)
	}

	runJSON := func(path string, body string) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected %s 200, got %d (%s)", path, rec.Code, rec.Body.String())
		}
	}

	runJSON("/api/payment/bind-card/tasks/11/open", `{}`)
	runJSON("/api/payment/bind-card/tasks/11/auto-bind-third-party", `{"card":{"number":"4242","exp_month":"01","exp_year":"30","cvc":"123"},"profile":{"name":"Ada","country":"GB"}}`)
	runJSON("/api/payment/bind-card/tasks/11/auto-bind-local", `{"card":{"number":"4242","exp_month":"01","exp_year":"30","cvc":"123"},"profile":{"name":"Ada","country":"GB"}}`)
	runJSON("/api/payment/bind-card/tasks/11/mark-user-action", `{"timeout_seconds":180,"interval_seconds":10}`)
	runJSON("/api/payment/bind-card/tasks/11/sync-subscription", `{}`)

	deleteRec := httptest.NewRecorder()
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/payment/bind-card/tasks/11", nil)
	router.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete bind-card task 200, got %d", deleteRec.Code)
	}
	if service.deletedTaskID != 11 {
		t.Fatalf("expected delete task id=11, got %d", service.deletedTaskID)
	}
}

func TestSubscriptionHandlerCompatibilityRoutes(t *testing.T) {
	service := &fakePaymentService{
		markSubscriptionResp: paymentpkg.MarkSubscriptionResponse{
			Success:          true,
			SubscriptionType: "team",
		},
		batchCheckResp: paymentpkg.BatchCheckSubscriptionResponse{
			SuccessCount: 2,
			FailedCount:  1,
			Details: []paymentpkg.BatchCheckSubscriptionDetail{
				{ID: 7, Email: "alpha@example.com", Success: true, SubscriptionType: "team", Confidence: "high", Source: "chatgpt_web"},
			},
		},
	}
	router := newTestRouter(service)

	markRec := httptest.NewRecorder()
	markReq := httptest.NewRequest(http.MethodPost, "/api/payment/accounts/7/mark-subscription", bytes.NewBufferString(`{"subscription_type":"team"}`))
	markReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(markRec, markReq)
	if markRec.Code != http.StatusOK {
		t.Fatalf("expected mark-subscription 200, got %d", markRec.Code)
	}
	if service.markSubscriptionAccountID != 7 || service.markSubscriptionReq.SubscriptionType != "team" {
		t.Fatalf("unexpected mark-subscription request: id=%d req=%+v", service.markSubscriptionAccountID, service.markSubscriptionReq)
	}

	batchRec := httptest.NewRecorder()
	batchReq := httptest.NewRequest(http.MethodPost, "/api/payment/accounts/batch-check-subscription", bytes.NewBufferString(`{
		"ids": [7,8],
		"select_all": true,
		"status_filter": "active",
		"email_service_filter": "outlook",
		"search_filter": "alpha",
		"refresh_token_state_filter": "has"
	}`))
	batchReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusOK {
		t.Fatalf("expected batch-check-subscription 200, got %d", batchRec.Code)
	}
	if !service.batchCheckReq.SelectAll || len(service.batchCheckReq.IDs) != 2 || service.batchCheckReq.StatusFilter != "active" {
		t.Fatalf("unexpected batch-check request: %+v", service.batchCheckReq)
	}
}

func newTestRouter(service *fakePaymentService) http.Handler {
	router := chi.NewRouter()
	paymenthttp.NewHandler(service).RegisterRoutes(router)
	return router
}

func assertDetailMessage(t *testing.T, body []byte, want string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if payload["detail"] != want {
		t.Fatalf("expected detail %q, got %#v", want, payload["detail"])
	}
}

type fakePaymentService struct {
	randomBillingCountry string
	randomBillingProxy   string
	randomBillingResp    paymentpkg.RandomBillingResponse
	randomBillingErr     error

	generateLinkReq  paymentpkg.GenerateLinkRequest
	generateLinkResp paymentpkg.GenerateLinkResponse
	generateLinkErr  error

	openIncognitoReq  paymentpkg.OpenIncognitoRequest
	openIncognitoResp paymentpkg.OpenIncognitoResponse
	openIncognitoErr  error

	sessionDiagnosticAccountID int
	sessionDiagnosticProbe     bool
	sessionDiagnosticProxy     string
	sessionDiagnosticResp      paymentpkg.SessionDiagnosticResponse
	sessionDiagnosticErr       error

	sessionBootstrapAccountID int
	sessionBootstrapProxy     string
	sessionBootstrapResp      paymentpkg.SessionBootstrapResponse
	sessionBootstrapErr       error

	saveSessionTokenAccountID int
	saveSessionTokenReq       paymentpkg.SaveSessionTokenRequest
	saveSessionTokenResp      paymentpkg.SaveSessionTokenResponse
	saveSessionTokenErr       error

	createTaskReq  paymentpkg.CreateBindCardTaskRequest
	createTaskResp paymentpkg.CreateBindCardTaskResponse
	createTaskErr  error

	listTasksReq  paymentpkg.ListBindCardTasksRequest
	listTasksResp paymentpkg.ListBindCardTasksResponse
	listTasksErr  error

	openTaskID   int
	openTaskResp paymentpkg.BindCardTaskActionResponse
	openTaskErr  error

	thirdPartyTaskID int
	thirdPartyReq    paymentpkg.ThirdPartyAutoBindRequest
	thirdPartyResp   paymentpkg.AutoBindResult
	thirdPartyErr    error

	localTaskID int
	localReq    paymentpkg.LocalAutoBindRequest
	localResp   paymentpkg.AutoBindResult
	localErr    error

	markUserActionTaskID int
	markUserActionReq    paymentpkg.MarkUserActionRequest
	markUserActionResp   paymentpkg.SyncBindCardTaskResponse
	markUserActionErr    error

	syncTaskID   int
	syncTaskReq  paymentpkg.SyncBindCardTaskRequest
	syncTaskResp paymentpkg.SyncBindCardTaskResponse
	syncTaskErr  error

	deletedTaskID int
	deleteTaskResp paymentpkg.DeleteBindCardTaskResponse
	deleteTaskErr  error

	markSubscriptionAccountID int
	markSubscriptionReq       paymentpkg.MarkSubscriptionRequest
	markSubscriptionResp      paymentpkg.MarkSubscriptionResponse
	markSubscriptionErr       error

	batchCheckReq  paymentpkg.BatchCheckSubscriptionRequest
	batchCheckResp paymentpkg.BatchCheckSubscriptionResponse
	batchCheckErr  error
}

func (f *fakePaymentService) GetRandomBillingProfile(_ context.Context, country string, proxy string) (paymentpkg.RandomBillingResponse, error) {
	f.randomBillingCountry = country
	f.randomBillingProxy = proxy
	return f.randomBillingResp, f.randomBillingErr
}

func (f *fakePaymentService) GeneratePaymentLink(_ context.Context, req paymentpkg.GenerateLinkRequest) (paymentpkg.GenerateLinkResponse, error) {
	f.generateLinkReq = req
	return f.generateLinkResp, f.generateLinkErr
}

func (f *fakePaymentService) OpenBrowserIncognito(_ context.Context, req paymentpkg.OpenIncognitoRequest) (paymentpkg.OpenIncognitoResponse, error) {
	f.openIncognitoReq = req
	return f.openIncognitoResp, f.openIncognitoErr
}

func (f *fakePaymentService) GetAccountSessionDiagnostic(_ context.Context, accountID int, probe bool, proxy string) (paymentpkg.SessionDiagnosticResponse, error) {
	f.sessionDiagnosticAccountID = accountID
	f.sessionDiagnosticProbe = probe
	f.sessionDiagnosticProxy = proxy
	return f.sessionDiagnosticResp, f.sessionDiagnosticErr
}

func (f *fakePaymentService) BootstrapAccountSessionToken(_ context.Context, accountID int, proxy string) (paymentpkg.SessionBootstrapResponse, error) {
	f.sessionBootstrapAccountID = accountID
	f.sessionBootstrapProxy = proxy
	return f.sessionBootstrapResp, f.sessionBootstrapErr
}

func (f *fakePaymentService) SaveAccountSessionToken(_ context.Context, accountID int, req paymentpkg.SaveSessionTokenRequest) (paymentpkg.SaveSessionTokenResponse, error) {
	f.saveSessionTokenAccountID = accountID
	f.saveSessionTokenReq = req
	return f.saveSessionTokenResp, f.saveSessionTokenErr
}

func (f *fakePaymentService) CreateBindCardTask(_ context.Context, req paymentpkg.CreateBindCardTaskRequest) (paymentpkg.CreateBindCardTaskResponse, error) {
	f.createTaskReq = req
	return f.createTaskResp, f.createTaskErr
}

func (f *fakePaymentService) ListBindCardTasks(_ context.Context, req paymentpkg.ListBindCardTasksRequest) (paymentpkg.ListBindCardTasksResponse, error) {
	f.listTasksReq = req
	return f.listTasksResp, f.listTasksErr
}

func (f *fakePaymentService) OpenBindCardTask(_ context.Context, taskID int) (paymentpkg.BindCardTaskActionResponse, error) {
	f.openTaskID = taskID
	return f.openTaskResp, f.openTaskErr
}

func (f *fakePaymentService) AutoBindBindCardTaskThirdParty(_ context.Context, taskID int, req paymentpkg.ThirdPartyAutoBindRequest) (paymentpkg.AutoBindResult, error) {
	f.thirdPartyTaskID = taskID
	f.thirdPartyReq = req
	return f.thirdPartyResp, f.thirdPartyErr
}

func (f *fakePaymentService) AutoBindBindCardTaskLocal(_ context.Context, taskID int, req paymentpkg.LocalAutoBindRequest) (paymentpkg.AutoBindResult, error) {
	f.localTaskID = taskID
	f.localReq = req
	return f.localResp, f.localErr
}

func (f *fakePaymentService) MarkBindCardTaskUserAction(_ context.Context, taskID int, req paymentpkg.MarkUserActionRequest) (paymentpkg.SyncBindCardTaskResponse, error) {
	f.markUserActionTaskID = taskID
	f.markUserActionReq = req
	return f.markUserActionResp, f.markUserActionErr
}

func (f *fakePaymentService) SyncBindCardTaskSubscription(_ context.Context, taskID int, req paymentpkg.SyncBindCardTaskRequest) (paymentpkg.SyncBindCardTaskResponse, error) {
	f.syncTaskID = taskID
	f.syncTaskReq = req
	return f.syncTaskResp, f.syncTaskErr
}

func (f *fakePaymentService) DeleteBindCardTask(_ context.Context, taskID int) (paymentpkg.DeleteBindCardTaskResponse, error) {
	f.deletedTaskID = taskID
	return f.deleteTaskResp, f.deleteTaskErr
}

func (f *fakePaymentService) MarkSubscription(_ context.Context, accountID int, req paymentpkg.MarkSubscriptionRequest) (paymentpkg.MarkSubscriptionResponse, error) {
	f.markSubscriptionAccountID = accountID
	f.markSubscriptionReq = req
	return f.markSubscriptionResp, f.markSubscriptionErr
}

func (f *fakePaymentService) BatchCheckSubscription(_ context.Context, req paymentpkg.BatchCheckSubscriptionRequest) (paymentpkg.BatchCheckSubscriptionResponse, error) {
	f.batchCheckReq = req
	return f.batchCheckResp, f.batchCheckErr
}
