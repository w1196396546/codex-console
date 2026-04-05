package payment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
)

var sessionTokenChunkPattern = regexp.MustCompile(`(?:^|;\s*)__Secure-next-auth\.session-token\.(\d+)=([^;]+)`)

type Service struct {
	repository              Repository
	accounts                AccountsRepository
	checkoutLinkGenerator   CheckoutLinkGenerator
	billingProfileGenerator BillingProfileGenerator
	browserOpener           BrowserOpener
	sessionAdapter          SessionAdapter
	subscriptionChecker     SubscriptionChecker
	autoBinder              AutoBinder
	now                     func() time.Time
}

type Option func(*Service)

func NewService(repository Repository, accountsRepository AccountsRepository, opts ...Option) *Service {
	service := &Service{
		repository: repository,
		accounts:   accountsRepository,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithCheckoutLinkGenerator(generator CheckoutLinkGenerator) Option {
	return func(s *Service) { s.checkoutLinkGenerator = generator }
}

func WithBillingProfileGenerator(generator BillingProfileGenerator) Option {
	return func(s *Service) { s.billingProfileGenerator = generator }
}

func WithBrowserOpener(opener BrowserOpener) Option {
	return func(s *Service) { s.browserOpener = opener }
}

func WithSessionAdapter(adapter SessionAdapter) Option {
	return func(s *Service) { s.sessionAdapter = adapter }
}

func WithSubscriptionChecker(checker SubscriptionChecker) Option {
	return func(s *Service) { s.subscriptionChecker = checker }
}

func WithAutoBinder(binder AutoBinder) Option {
	return func(s *Service) { s.autoBinder = binder }
}

func (s *Service) GetRandomBillingProfile(ctx context.Context, country string, proxy string) (RandomBillingResponse, error) {
	if s == nil || s.billingProfileGenerator == nil {
		return RandomBillingResponse{}, ErrBillingProfileGeneratorMissing
	}
	profile, err := s.billingProfileGenerator.GenerateRandomBillingProfile(ctx, firstNonEmpty(strings.ToUpper(strings.TrimSpace(country)), "US"), strings.TrimSpace(proxy))
	if err != nil {
		return RandomBillingResponse{}, newStatusError(http.StatusInternalServerError, fmt.Sprintf("随机账单资料生成失败: %v", err))
	}
	return RandomBillingResponse{
		Success: true,
		Profile: profile,
	}, nil
}

func (s *Service) GetAccountSessionDiagnostic(ctx context.Context, accountID int, probe bool, proxy string) (SessionDiagnosticResponse, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return SessionDiagnosticResponse{}, err
	}

	accessToken := strings.TrimSpace(account.AccessToken)
	refreshToken := strings.TrimSpace(account.RefreshToken)
	sessionTokenDB := strings.TrimSpace(account.SessionToken)
	cookiesText := strings.TrimSpace(account.Cookies)
	deviceID := resolveDeviceID(account)
	sessionTokenCookie := extractSessionTokenFromCookieText(cookiesText)
	sessionChunkIndices := extractSessionTokenChunkIndices(cookiesText)
	resolvedSessionToken := firstNonEmpty(sessionTokenDB, sessionTokenCookie)

	notes := make([]string, 0, 4)
	if accessToken == "" {
		notes = append(notes, "缺少 access_token（无法走 auth/session 探测授权头）")
	}
	if resolvedSessionToken == "" {
		notes = append(notes, "未发现 session_token（DB 与 cookies 都为空）")
	}
	if len(sessionChunkIndices) > 0 && sessionTokenCookie == "" {
		notes = append(notes, "发现 session 分片但未能拼接，请检查 cookies 原文完整性")
	}
	if deviceID == "" {
		notes = append(notes, "缺少 oai-did（会话建立成功率会下降）")
	}

	var probeResult *SessionProbeResult
	if probe && s.sessionAdapter != nil {
		result, probeErr := s.sessionAdapter.ProbeSession(ctx, account, strings.TrimSpace(proxy))
		if probeErr != nil {
			probeResult = &SessionProbeResult{OK: false, Error: probeErr.Error()}
			notes = append(notes, "实时探测未通过："+probeErr.Error())
		} else {
			probeResult = result
			if probeResult != nil && !probeResult.OK {
				notes = append(notes, "实时探测未通过："+firstNonEmpty(strings.TrimSpace(probeResult.Error), fmt.Sprintf("http_status=%d", probeResult.HTTPStatus)))
			}
		}
	}

	canLoginBootstrap := strings.TrimSpace(account.Password) != "" && strings.TrimSpace(account.EmailService) != ""
	recommendation := "会话完整，可直接执行全自动绑卡"
	switch {
	case resolvedSessionToken == "" && accessToken != "":
		recommendation = "建议先用 access_token 预热 /api/auth/session，再执行全自动"
	case accessToken == "" && resolvedSessionToken == "":
		recommendation = "账号会话信息不足，建议重新登录一次并回写 cookies/session_token"
	case probeResult != nil && !probeResult.SessionTokenFound:
		recommendation = "建议检查代理线路与账号登录态，必要时切直连重试"
	}
	if resolvedSessionToken == "" && canLoginBootstrap {
		recommendation = "可尝试后端自动登录补会话（账号密码+邮箱验证码）后再执行全自动"
	}

	return SessionDiagnosticResponse{
		Success: true,
		Diagnostic: SessionDiagnosticPayload{
			AccountID: account.ID,
			Email:     account.Email,
			TokenState: map[string]any{
				"has_access_token":               accessToken != "",
				"access_token_len":               len(accessToken),
				"access_token_preview":           maskSecret(accessToken),
				"has_refresh_token":              refreshToken != "",
				"refresh_token_len":              len(refreshToken),
				"has_session_token_db":           sessionTokenDB != "",
				"session_token_db_len":           len(sessionTokenDB),
				"session_token_db_preview":       maskSecret(sessionTokenDB),
				"has_session_token_cookie":       sessionTokenCookie != "",
				"session_token_cookie_len":       len(sessionTokenCookie),
				"session_token_cookie_preview":   maskSecret(sessionTokenCookie),
				"resolved_session_token_len":     len(resolvedSessionToken),
				"resolved_session_token_preview": maskSecret(resolvedSessionToken),
			},
			CookieState: map[string]any{
				"has_cookies":           cookiesText != "",
				"cookies_len":           len(cookiesText),
				"has_oai_did":           extractCookieValue(cookiesText, "oai-did") != "",
				"resolved_oai_did":      maskSecret(deviceID),
				"session_chunk_count":   len(sessionChunkIndices),
				"session_chunk_indices": sessionChunkIndices,
			},
			BootstrapCapability: map[string]any{
				"can_login_bootstrap":      canLoginBootstrap,
				"has_password":             strings.TrimSpace(account.Password) != "",
				"email_service_type":       account.EmailService,
				"email_service_mailbox_id": account.EmailServiceID,
			},
			Probe:          probeResult,
			Notes:          notes,
			Recommendation: recommendation,
			CheckedAt:      s.now().Format(time.RFC3339),
		},
	}, nil
}

func (s *Service) BootstrapAccountSessionToken(ctx context.Context, accountID int, proxy string) (SessionBootstrapResponse, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return SessionBootstrapResponse{}, err
	}
	if s == nil || s.sessionAdapter == nil {
		return SessionBootstrapResponse{
			Success:   false,
			Message:   "会话补全未配置",
			AccountID: account.ID,
			Email:     account.Email,
		}, nil
	}

	result, err := s.sessionAdapter.BootstrapSessionToken(ctx, account, strings.TrimSpace(proxy))
	if err != nil {
		return SessionBootstrapResponse{}, newStatusError(http.StatusInternalServerError, fmt.Sprintf("会话补全失败: %v", err))
	}
	token := strings.TrimSpace(result.SessionToken)
	if token == "" {
		return SessionBootstrapResponse{
			Success:   false,
			Message:   "会话补全未命中 session_token",
			AccountID: account.ID,
			Email:     account.Email,
		}, nil
	}

	account.SessionToken = token
	if strings.TrimSpace(result.AccessToken) != "" {
		account.AccessToken = strings.TrimSpace(result.AccessToken)
	}
	if strings.TrimSpace(result.Cookies) != "" {
		account.Cookies = strings.TrimSpace(result.Cookies)
	} else {
		account.Cookies = upsertCookie(account.Cookies, "__Secure-next-auth.session-token", token)
	}
	lastRefresh := s.now()
	account.LastRefresh = &lastRefresh
	if _, err := s.accounts.UpsertAccount(ctx, account); err != nil {
		return SessionBootstrapResponse{}, fmt.Errorf("persist bootstrap session token: %w", err)
	}

	return SessionBootstrapResponse{
		Success:             true,
		Message:             "会话补全成功",
		AccountID:           account.ID,
		Email:               account.Email,
		SessionTokenLen:     len(token),
		SessionTokenPreview: maskSecret(token),
	}, nil
}

func (s *Service) SaveAccountSessionToken(ctx context.Context, accountID int, req SaveSessionTokenRequest) (SaveSessionTokenResponse, error) {
	token := strings.TrimSpace(req.SessionToken)
	if token == "" {
		return SaveSessionTokenResponse{}, newStatusError(http.StatusBadRequest, "session_token 不能为空")
	}
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return SaveSessionTokenResponse{}, err
	}
	account.SessionToken = token
	if req.MergeCookie {
		account.Cookies = upsertCookie(account.Cookies, "__Secure-next-auth.session-token", token)
	}
	now := s.now()
	account.LastRefresh = &now
	if _, err := s.accounts.UpsertAccount(ctx, account); err != nil {
		return SaveSessionTokenResponse{}, fmt.Errorf("persist session token: %w", err)
	}
	return SaveSessionTokenResponse{
		Success:             true,
		AccountID:           account.ID,
		Email:               account.Email,
		SessionTokenLen:     len(token),
		SessionTokenPreview: maskSecret(token),
		Message:             "session_token 已保存",
	}, nil
}

func (s *Service) GeneratePaymentLink(ctx context.Context, req GenerateLinkRequest) (GenerateLinkResponse, error) {
	account, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return GenerateLinkResponse{}, err
	}
	if s == nil || s.checkoutLinkGenerator == nil {
		return GenerateLinkResponse{}, ErrCheckoutLinkGeneratorMissing
	}
	result, err := s.checkoutLinkGenerator.GenerateCheckoutLink(ctx, account, req, strings.TrimSpace(req.Proxy))
	if err != nil {
		return GenerateLinkResponse{}, newStatusError(http.StatusInternalServerError, fmt.Sprintf("生成链接失败: %v", err))
	}
	opened := false
	if req.AutoOpen && result.Link != "" && s.browserOpener != nil {
		opened, _ = s.browserOpener.OpenIncognito(ctx, result.Link, account.Cookies)
	}
	return GenerateLinkResponse{
		Success:            true,
		Link:               result.Link,
		IsOfficialCheckout: isOfficialCheckoutLink(result.Link),
		PlanType:           req.PlanType,
		Country:            firstNonEmpty(strings.ToUpper(strings.TrimSpace(req.Country)), "US"),
		Currency:           firstNonEmpty(strings.ToUpper(strings.TrimSpace(req.Currency)), "USD"),
		AutoOpened:         opened,
		Source:             result.Source,
		FallbackReason:     result.FallbackReason,
		CheckoutSessionID:  result.CheckoutSessionID,
		PublishableKey:     result.PublishableKey,
		HasClientSecret:    strings.TrimSpace(result.ClientSecret) != "",
	}, nil
}

func (s *Service) OpenBrowserIncognito(ctx context.Context, req OpenIncognitoRequest) (OpenIncognitoResponse, error) {
	url := strings.TrimSpace(req.URL)
	if url == "" {
		return OpenIncognitoResponse{}, newStatusError(http.StatusBadRequest, "URL 不能为空")
	}
	cookies := ""
	if req.AccountID != 0 {
		account, err := s.getAccount(ctx, req.AccountID)
		if err != nil {
			return OpenIncognitoResponse{}, err
		}
		cookies = account.Cookies
	}
	if s == nil || s.browserOpener == nil {
		return OpenIncognitoResponse{Success: false, Message: "未找到可用浏览器，请手动复制链接"}, nil
	}
	opened, err := s.browserOpener.OpenIncognito(ctx, url, cookies)
	if err != nil {
		return OpenIncognitoResponse{}, newStatusError(http.StatusInternalServerError, fmt.Sprintf("请求失败: %v", err))
	}
	if opened {
		return OpenIncognitoResponse{Success: true, Message: "已在无痕模式打开浏览器"}, nil
	}
	return OpenIncognitoResponse{Success: false, Message: "未找到可用浏览器，请手动复制链接"}, nil
}

func (s *Service) CreateBindCardTask(ctx context.Context, req CreateBindCardTaskRequest) (CreateBindCardTaskResponse, error) {
	account, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return CreateBindCardTaskResponse{}, err
	}
	bindMode := normalizeBindMode(req.BindMode)
	if bindMode != "semi_auto" && bindMode != "third_party" && bindMode != "local_auto" {
		return CreateBindCardTaskResponse{}, newStatusError(http.StatusBadRequest, "bind_mode 必须为 semi_auto / third_party / local_auto")
	}
	if s == nil || s.checkoutLinkGenerator == nil {
		return CreateBindCardTaskResponse{}, ErrCheckoutLinkGeneratorMissing
	}
	link, err := s.checkoutLinkGenerator.GenerateCheckoutLink(ctx, account, GenerateLinkRequest{CheckoutRequestBase: req.CheckoutRequestBase}, strings.TrimSpace(req.Proxy))
	if err != nil {
		return CreateBindCardTaskResponse{}, newStatusError(http.StatusInternalServerError, fmt.Sprintf("创建绑卡任务失败: %v", err))
	}
	task, err := s.repository.CreateBindCardTask(ctx, CreateBindCardTaskParams{
		AccountID:         account.ID,
		PlanType:          strings.TrimSpace(req.PlanType),
		WorkspaceName:     req.WorkspaceName,
		PriceInterval:     req.PriceInterval,
		SeatQuantity:      req.SeatQuantity,
		Country:           firstNonEmpty(strings.ToUpper(strings.TrimSpace(req.Country)), "US"),
		Currency:          firstNonEmpty(strings.ToUpper(strings.TrimSpace(req.Currency)), "USD"),
		CheckoutURL:       link.Link,
		CheckoutSessionID: link.CheckoutSessionID,
		PublishableKey:    link.PublishableKey,
		ClientSecret:      link.ClientSecret,
		CheckoutSource:    link.Source,
		BindMode:          bindMode,
		Status:            StatusLinkReady,
	})
	if err != nil {
		return CreateBindCardTaskResponse{}, fmt.Errorf("create bind_card_task: %w", err)
	}
	autoOpened := false
	if req.AutoOpen && bindMode == "semi_auto" && s.browserOpener != nil {
		opened, openErr := s.browserOpener.OpenIncognito(ctx, task.CheckoutURL, account.Cookies)
		if openErr == nil && opened {
			now := s.now()
			task.Status = StatusOpened
			task.OpenedAt = &now
			task, _ = s.repository.UpdateBindCardTask(ctx, task)
			autoOpened = true
		}
	}
	return CreateBindCardTaskResponse{
		Success:            true,
		Task:               task,
		Link:               link.Link,
		IsOfficialCheckout: isOfficialCheckoutLink(link.Link),
		Source:             link.Source,
		FallbackReason:     link.FallbackReason,
		AutoOpened:         autoOpened,
		CheckoutSessionID:  link.CheckoutSessionID,
		PublishableKey:     link.PublishableKey,
		HasClientSecret:    strings.TrimSpace(link.ClientSecret) != "",
	}, nil
}

func (s *Service) ListBindCardTasks(ctx context.Context, req ListBindCardTasksRequest) (ListBindCardTasksResponse, error) {
	resp, err := s.repository.ListBindCardTasks(ctx, req)
	if err != nil {
		return ListBindCardTasksResponse{}, err
	}
	for idx := range resp.Tasks {
		account, accountErr := s.accounts.GetAccountByID(ctx, resp.Tasks[idx].AccountID)
		if accountErr != nil {
			continue
		}
		if resp.Tasks[idx].AccountEmail == "" {
			resp.Tasks[idx].AccountEmail = account.Email
		}
		if isPaidSubscription(account.SubscriptionType) && resp.Tasks[idx].Status != StatusCompleted {
			now := s.now()
			resp.Tasks[idx].Status = StatusCompleted
			if account.SubscriptionAt != nil {
				resp.Tasks[idx].CompletedAt = account.SubscriptionAt
			} else {
				resp.Tasks[idx].CompletedAt = &now
			}
			resp.Tasks[idx].LastCheckedAt = &now
			resp.Tasks[idx].LastError = ""
			updated, updateErr := s.repository.UpdateBindCardTask(ctx, resp.Tasks[idx])
			if updateErr == nil {
				resp.Tasks[idx] = updated
			}
		}
	}
	return resp, nil
}

func (s *Service) OpenBindCardTask(ctx context.Context, taskID int) (BindCardTaskActionResponse, error) {
	task, err := s.repository.GetBindCardTask(ctx, taskID)
	if err != nil {
		return BindCardTaskActionResponse{}, toStatusError(err, http.StatusNotFound, "绑卡任务不存在")
	}
	if strings.TrimSpace(task.CheckoutURL) == "" {
		return BindCardTaskActionResponse{}, newStatusError(http.StatusBadRequest, "任务缺少 checkout 链接")
	}
	if s.browserOpener == nil {
		return BindCardTaskActionResponse{}, newStatusError(http.StatusInternalServerError, "未找到可用的浏览器，请手动复制链接")
	}
	account, err := s.getAccount(ctx, task.AccountID)
	if err != nil {
		return BindCardTaskActionResponse{}, err
	}
	opened, openErr := s.browserOpener.OpenIncognito(ctx, task.CheckoutURL, account.Cookies)
	if openErr != nil || !opened {
		task.LastError = "未找到可用的浏览器"
		_, _ = s.repository.UpdateBindCardTask(ctx, task)
		return BindCardTaskActionResponse{}, newStatusError(http.StatusInternalServerError, "未找到可用的浏览器，请手动复制链接")
	}
	now := s.now()
	if task.Status != StatusPaidPendingSync && task.Status != StatusCompleted {
		task.Status = StatusOpened
	}
	task.OpenedAt = &now
	task.LastError = ""
	task, err = s.repository.UpdateBindCardTask(ctx, task)
	if err != nil {
		return BindCardTaskActionResponse{}, err
	}
	return BindCardTaskActionResponse{Success: true, Task: task}, nil
}

func (s *Service) AutoBindBindCardTaskThirdParty(ctx context.Context, taskID int, req ThirdPartyAutoBindRequest) (AutoBindResult, error) {
	return s.autoBind(ctx, taskID, func(task BindCardTask, account accounts.Account) (AutoBindResult, error) {
		if s.autoBinder == nil {
			return AutoBindResult{}, ErrAutoBinderMissing
		}
		return s.autoBinder.AutoBindThirdParty(ctx, task, account, req)
	})
}

func (s *Service) AutoBindBindCardTaskLocal(ctx context.Context, taskID int, req LocalAutoBindRequest) (AutoBindResult, error) {
	return s.autoBind(ctx, taskID, func(task BindCardTask, account accounts.Account) (AutoBindResult, error) {
		if s.autoBinder == nil {
			return AutoBindResult{}, ErrAutoBinderMissing
		}
		return s.autoBinder.AutoBindLocal(ctx, task, account, req)
	})
}

func (s *Service) autoBind(ctx context.Context, taskID int, call func(task BindCardTask, account accounts.Account) (AutoBindResult, error)) (AutoBindResult, error) {
	task, account, err := s.loadTaskAndAccount(ctx, taskID)
	if err != nil {
		return AutoBindResult{}, err
	}
	result, err := call(task, account)
	if err != nil {
		now := s.now()
		task.Status = StatusFailed
		task.LastError = err.Error()
		task.LastCheckedAt = &now
		_, _ = s.repository.UpdateBindCardTask(ctx, task)
		return AutoBindResult{}, newStatusError(http.StatusInternalServerError, err.Error())
	}
	now := s.now()
	switch {
	case isPaidSubscription(result.SubscriptionType):
		account.SubscriptionType = normalizeSubscriptionType(result.SubscriptionType)
		account.SubscriptionAt = &now
		_, _ = s.accounts.UpsertAccount(ctx, account)
		task.Status = StatusCompleted
		task.CompletedAt = &now
		task.LastError = ""
		task.LastCheckedAt = &now
	case result.PaidConfirmed:
		task.Status = StatusPaidPendingSync
		task.LastCheckedAt = &now
		task.LastError = "已确认支付，等待订阅同步"
	case result.Pending || result.NeedUserAction:
		task.Status = StatusWaitingUserAction
		task.LastCheckedAt = &now
	}
	updated, updateErr := s.repository.UpdateBindCardTask(ctx, task)
	if updateErr == nil {
		result.Task = updated
	}
	result.AccountID = account.ID
	result.AccountEmail = account.Email
	result.PaidConfirmed = result.PaidConfirmed || task.Status == StatusPaidPendingSync
	return result, nil
}

func (s *Service) SyncBindCardTaskSubscription(ctx context.Context, taskID int, req SyncBindCardTaskRequest) (SyncBindCardTaskResponse, error) {
	task, account, err := s.loadTaskAndAccount(ctx, taskID)
	if err != nil {
		return SyncBindCardTaskResponse{}, err
	}
	detail, err := s.checkSubscription(ctx, account, strings.TrimSpace(req.Proxy), true)
	if err != nil {
		now := s.now()
		task.Status = StatusFailed
		task.LastError = err.Error()
		task.LastCheckedAt = &now
		_, _ = s.repository.UpdateBindCardTask(ctx, task)
		return SyncBindCardTaskResponse{}, newStatusError(http.StatusInternalServerError, fmt.Sprintf("订阅检测失败: %v", err))
	}
	task, account = s.applySubscriptionResult(task, account, detail, s.now())
	if _, err := s.accounts.UpsertAccount(ctx, account); err != nil {
		return SyncBindCardTaskResponse{}, fmt.Errorf("persist account subscription: %w", err)
	}
	task, err = s.repository.UpdateBindCardTask(ctx, task)
	if err != nil {
		return SyncBindCardTaskResponse{}, err
	}
	return SyncBindCardTaskResponse{
		Success:          true,
		SubscriptionType: normalizeSubscriptionType(detail.Status),
		Detail:           detail.Map(),
		Task:             task,
		AccountID:        account.ID,
		AccountEmail:     account.Email,
	}, nil
}

func (s *Service) MarkBindCardTaskUserAction(ctx context.Context, taskID int, req MarkUserActionRequest) (SyncBindCardTaskResponse, error) {
	task, account, err := s.loadTaskAndAccount(ctx, taskID)
	if err != nil {
		return SyncBindCardTaskResponse{}, err
	}
	now := s.now()
	task.Status = StatusVerifying
	task.LastError = ""
	task.LastCheckedAt = &now
	task, _ = s.repository.UpdateBindCardTask(ctx, task)

	detail, err := s.checkSubscription(ctx, account, strings.TrimSpace(req.Proxy), true)
	if err != nil {
		task.Status = StatusFailed
		task.LastError = err.Error()
		task.LastCheckedAt = &now
		_, _ = s.repository.UpdateBindCardTask(ctx, task)
		return SyncBindCardTaskResponse{}, newStatusError(http.StatusInternalServerError, fmt.Sprintf("订阅检测失败: %v", err))
	}
	task, account = s.applySubscriptionResult(task, account, detail, now)
	if _, err := s.accounts.UpsertAccount(ctx, account); err != nil {
		return SyncBindCardTaskResponse{}, fmt.Errorf("persist account subscription: %w", err)
	}
	task, err = s.repository.UpdateBindCardTask(ctx, task)
	if err != nil {
		return SyncBindCardTaskResponse{}, err
	}
	subscriptionType := normalizeSubscriptionType(detail.Status)
	return SyncBindCardTaskResponse{
		Success:          true,
		Verified:         isPaidSubscription(subscriptionType),
		Checks:           1,
		SubscriptionType: subscriptionType,
		Detail:           detail.Map(),
		TokenRefreshUsed: detail.RefreshedToken,
		Task:             task,
		AccountID:        account.ID,
		AccountEmail:     account.Email,
	}, nil
}

func (s *Service) DeleteBindCardTask(ctx context.Context, taskID int) (DeleteBindCardTaskResponse, error) {
	if err := s.repository.DeleteBindCardTask(ctx, taskID); err != nil {
		return DeleteBindCardTaskResponse{}, toStatusError(err, http.StatusNotFound, "绑卡任务不存在")
	}
	return DeleteBindCardTaskResponse{Success: true, TaskID: taskID}, nil
}

func (s *Service) BatchCheckSubscription(ctx context.Context, req BatchCheckSubscriptionRequest) (BatchCheckSubscriptionResponse, error) {
	items, err := s.selectAccounts(ctx, req)
	if err != nil {
		return BatchCheckSubscriptionResponse{}, err
	}
	resp := BatchCheckSubscriptionResponse{
		Details: make([]BatchCheckSubscriptionDetail, 0, len(items)),
	}
	for _, account := range items {
		detail, checkErr := s.checkSubscription(ctx, account, strings.TrimSpace(req.Proxy), true)
		if checkErr != nil {
			resp.FailedCount++
			resp.Details = append(resp.Details, BatchCheckSubscriptionDetail{
				ID:      account.ID,
				Email:   account.Email,
				Success: false,
				Error:   checkErr.Error(),
			})
			continue
		}
		_, updatedAccount := s.applySubscriptionResult(BindCardTask{}, account, detail, s.now())
		if _, err := s.accounts.UpsertAccount(ctx, updatedAccount); err != nil {
			resp.FailedCount++
			resp.Details = append(resp.Details, BatchCheckSubscriptionDetail{
				ID:      account.ID,
				Email:   account.Email,
				Success: false,
				Error:   err.Error(),
			})
			continue
		}
		resp.SuccessCount++
		resp.Details = append(resp.Details, BatchCheckSubscriptionDetail{
			ID:               account.ID,
			Email:            account.Email,
			Success:          true,
			SubscriptionType: normalizeSubscriptionType(detail.Status),
			Confidence:       strings.TrimSpace(detail.Confidence),
			Source:           strings.TrimSpace(detail.Source),
			TokenRefreshed:   detail.RefreshedToken,
		})
	}
	return resp, nil
}

func (s *Service) MarkSubscription(ctx context.Context, accountID int, req MarkSubscriptionRequest) (MarkSubscriptionResponse, error) {
	value := normalizeSubscriptionType(req.SubscriptionType)
	if err := ensureSupportedSubscriptionType(value); err != nil {
		return MarkSubscriptionResponse{}, newStatusError(http.StatusBadRequest, "subscription_type 必须为 [free plus team]")
	}
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return MarkSubscriptionResponse{}, err
	}
	now := s.now()
	if value == "free" {
		account.SubscriptionType = ""
		account.SubscriptionAt = nil
	} else {
		account.SubscriptionType = value
		account.SubscriptionAt = &now
	}
	if _, err := s.accounts.UpsertAccount(ctx, account); err != nil {
		return MarkSubscriptionResponse{}, fmt.Errorf("persist subscription mark: %w", err)
	}
	return MarkSubscriptionResponse{Success: true, SubscriptionType: value}, nil
}

func (s *Service) getAccount(ctx context.Context, accountID int) (accounts.Account, error) {
	if s == nil || s.accounts == nil {
		return accounts.Account{}, ErrAccountsRepositoryNotConfigured
	}
	account, err := s.accounts.GetAccountByID(ctx, accountID)
	if errors.Is(err, accounts.ErrAccountNotFound) {
		return accounts.Account{}, newStatusError(http.StatusNotFound, "账号不存在")
	}
	if err != nil {
		return accounts.Account{}, fmt.Errorf("get account: %w", err)
	}
	return account, nil
}

func (s *Service) loadTaskAndAccount(ctx context.Context, taskID int) (BindCardTask, accounts.Account, error) {
	task, err := s.repository.GetBindCardTask(ctx, taskID)
	if err != nil {
		return BindCardTask{}, accounts.Account{}, toStatusError(err, http.StatusNotFound, "绑卡任务不存在")
	}
	account, err := s.getAccount(ctx, task.AccountID)
	if err != nil {
		return BindCardTask{}, accounts.Account{}, err
	}
	if task.AccountEmail == "" {
		task.AccountEmail = account.Email
	}
	return task, account, nil
}

func (s *Service) checkSubscription(ctx context.Context, account accounts.Account, proxy string, allowRefresh bool) (SubscriptionCheckDetail, error) {
	if s == nil || s.subscriptionChecker == nil {
		return SubscriptionCheckDetail{}, ErrSubscriptionCheckerMissing
	}
	return s.subscriptionChecker.CheckSubscription(ctx, account, proxy, allowRefresh)
}

func (s *Service) applySubscriptionResult(task BindCardTask, account accounts.Account, detail SubscriptionCheckDetail, now time.Time) (BindCardTask, accounts.Account) {
	status := normalizeSubscriptionType(detail.Status)
	confidence := strings.ToLower(strings.TrimSpace(detail.Confidence))
	if status == "free" && confidence == "" {
		confidence = "low"
	}
	if status == "plus" || status == "team" {
		account.SubscriptionType = status
		account.SubscriptionAt = &now
		task.Status = StatusCompleted
		task.CompletedAt = &now
		task.LastError = ""
		task.LastCheckedAt = &now
		return task, account
	}

	if status == "free" && confidence == "high" {
		account.SubscriptionType = ""
		account.SubscriptionAt = nil
		task.Status = StatusWaitingUserAction
		task.CompletedAt = nil
		task.LastError = ""
		task.LastCheckedAt = &now
		return task, account
	}

	task.CompletedAt = nil
	task.LastCheckedAt = &now
	if shouldKeepPaidPending(task, detail) {
		task.Status = StatusPaidPendingSync
		task.LastError = fmt.Sprintf(
			"已确认支付，订阅暂未同步（当前: %s, source=%s, confidence=%s%s）。可稍后再次点击“同步订阅”。",
			firstNonEmpty(status, "free"),
			firstNonEmpty(detail.Source, "unknown"),
			firstNonEmpty(detail.Confidence, "unknown"),
			noteSuffix(detail.Note),
		)
	} else {
		task.Status = StatusWaitingUserAction
		task.LastError = ""
		if confidence != "high" {
			task.LastError = fmt.Sprintf(
				"订阅判定低置信度（source=%s, confidence=%s%s），请稍后重试。",
				firstNonEmpty(detail.Source, "unknown"),
				firstNonEmpty(detail.Confidence, "unknown"),
				noteSuffix(detail.Note),
			)
		}
	}
	return task, account
}

func (s *Service) selectAccounts(ctx context.Context, req BatchCheckSubscriptionRequest) ([]accounts.Account, error) {
	if req.SelectAll {
		return s.accounts.ListAccountsBySelection(ctx, accounts.AccountSelectionRequest{
			IDs:                     append([]int(nil), req.IDs...),
			SelectAll:               req.SelectAll,
			StatusFilter:            strings.TrimSpace(req.StatusFilter),
			EmailServiceFilter:      strings.TrimSpace(req.EmailServiceFilter),
			SearchFilter:            strings.TrimSpace(req.SearchFilter),
			RefreshTokenStateFilter: strings.TrimSpace(req.RefreshTokenStateFilter),
		})
	}
	items := make([]accounts.Account, 0, len(req.IDs))
	seen := map[int]struct{}{}
	for _, id := range req.IDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		account, err := s.getAccount(ctx, id)
		if err != nil {
			continue
		}
		items = append(items, account)
	}
	return items, nil
}

func shouldKeepPaidPending(task BindCardTask, detail SubscriptionCheckDetail) bool {
	confidence := strings.ToLower(strings.TrimSpace(detail.Confidence))
	status := normalizeSubscriptionType(detail.Status)
	if task.Status == StatusPaidPendingSync {
		return true
	}
	hasCheckoutContext := strings.TrimSpace(task.CheckoutSessionID) != "" || strings.TrimSpace(task.CheckoutURL) != ""
	return status == "free" && confidence != "high" && hasCheckoutContext
}

func noteSuffix(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	return ", note=" + note
}

func toStatusError(err error, status int, detail string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrBindCardTaskNotFound):
		return newStatusError(status, detail)
	default:
		return err
	}
}

func normalizeSubscriptionType(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "plus":
		return "plus"
	case "team":
		return "team"
	case "free":
		return "free"
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
}

func isPaidSubscription(value string) bool {
	switch normalizeSubscriptionType(value) {
	case "plus", "team":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func ensureMap(value map[string]any) map[string]any {
	if value != nil {
		return value
	}
	return map[string]any{}
}

func extractCookieValue(cookiesText string, cookieName string) string {
	text := strings.TrimSpace(cookiesText)
	if text == "" || cookieName == "" {
		return ""
	}
	items := strings.Split(text, ";")
	for _, item := range items {
		part := strings.TrimSpace(item)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(name) == cookieName {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func extractSessionTokenFromCookieText(cookiesText string) string {
	if direct := extractCookieValue(cookiesText, "__Secure-next-auth.session-token"); direct != "" {
		return direct
	}
	chunks := sessionTokenChunkPattern.FindAllStringSubmatch(strings.TrimSpace(cookiesText), -1)
	if len(chunks) == 0 {
		return ""
	}
	parts := make(map[int]string, len(chunks))
	indexes := make([]int, 0, len(chunks))
	for _, chunk := range chunks {
		if len(chunk) != 3 {
			continue
		}
		index := parsePositiveInt(chunk[1])
		if index < 0 {
			continue
		}
		parts[index] = strings.TrimSpace(chunk[2])
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	builder := strings.Builder{}
	for _, index := range indexes {
		builder.WriteString(parts[index])
	}
	return builder.String()
}

func extractSessionTokenChunkIndices(cookiesText string) []int {
	chunks := sessionTokenChunkPattern.FindAllStringSubmatch(strings.TrimSpace(cookiesText), -1)
	indexes := make([]int, 0, len(chunks))
	for _, chunk := range chunks {
		if len(chunk) != 3 {
			continue
		}
		index := parsePositiveInt(chunk[1])
		if index >= 0 {
			indexes = append(indexes, index)
		}
	}
	sort.Ints(indexes)
	return indexes
}

func parsePositiveInt(value string) int {
	total := 0
	if value == "" {
		return -1
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return -1
		}
		total = total*10 + int(char-'0')
	}
	return total
}

func upsertCookie(cookiesText string, cookieName string, cookieValue string) string {
	cookieName = strings.TrimSpace(cookieName)
	cookieValue = strings.TrimSpace(cookieValue)
	if cookieName == "" {
		return strings.TrimSpace(cookiesText)
	}
	items := strings.Split(strings.TrimSpace(cookiesText), ";")
	output := make([]string, 0, len(items)+1)
	replaced := false
	for _, item := range items {
		part := strings.TrimSpace(item)
		if part == "" {
			continue
		}
		name, _, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(name) == cookieName {
			if cookieValue != "" {
				output = append(output, cookieName+"="+cookieValue)
			}
			replaced = true
			continue
		}
		output = append(output, part)
	}
	if !replaced && cookieValue != "" {
		output = append(output, cookieName+"="+cookieValue)
	}
	return strings.Join(output, "; ")
}

func resolveDeviceID(account accounts.Account) string {
	if strings.TrimSpace(account.DeviceID) != "" {
		return strings.TrimSpace(account.DeviceID)
	}
	if value := extractCookieValue(account.Cookies, "oai-did"); value != "" {
		return value
	}
	for _, key := range []string{"device_id", "oai_did", "oai-device-id"} {
		if raw, ok := account.ExtraData[key]; ok {
			if value := strings.TrimSpace(fmt.Sprint(raw)); value != "" && value != "<nil>" {
				return value
			}
		}
	}
	return ""
}

func maskSecret(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 8 {
		return strings.Repeat("*", len(trimmed))
	}
	return trimmed[:4] + strings.Repeat("*", len(trimmed)-8) + trimmed[len(trimmed)-4:]
}

func isOfficialCheckoutLink(link string) bool {
	normalized := strings.TrimSpace(strings.ToLower(link))
	return strings.Contains(normalized, "checkout") && (strings.Contains(normalized, "openai.com") || strings.Contains(normalized, "chatgpt.com"))
}
