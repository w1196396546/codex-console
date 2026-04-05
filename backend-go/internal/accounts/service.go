package accounts

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/mail"
	"github.com/dou-jiang/codex-console/backend-go/internal/uploader"
	"github.com/jackc/pgx/v5/pgxpool"
)

var sessionTokenChunkPattern = regexp.MustCompile(`(?:^|;\s*)__Secure-next-auth\.session-token\.(\d+)=([^;]+)`)

type overviewEndpoint struct {
	name     string
	url      string
	required bool
}

var overviewUsageEndpoints = []overviewEndpoint{
	{name: "me", url: "https://chatgpt.com/backend-api/me", required: true},
	{name: "wham_usage", url: "https://chatgpt.com/backend-api/wham/usage", required: true},
	{name: "codex_usage", url: "https://chatgpt.com/backend-api/codex/usage", required: false},
}

type overviewHTTPError struct {
	statusCode int
	message    string
}

func (e *overviewHTTPError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

type Repository interface {
	ListAccounts(ctx context.Context, req ListAccountsRequest) ([]Account, int, error)
	ListAccountsForOverview(ctx context.Context, req AccountOverviewCardsRequest) ([]Account, error)
	ListAccountsForSelectable(ctx context.Context, req AccountOverviewSelectableRequest) ([]Account, error)
	ListAccountsBySelection(ctx context.Context, req AccountSelectionRequest) ([]Account, error)
	GetAccountByID(ctx context.Context, accountID int) (Account, error)
	GetCurrentAccountID(ctx context.Context) (*int, error)
	GetAccountsStatsSummary(ctx context.Context) (AccountsStatsSummary, error)
	GetAccountsOverviewStats(ctx context.Context) (AccountsOverviewStats, error)
	GetAccountByEmail(ctx context.Context, email string) (Account, bool, error)
	UpsertAccount(ctx context.Context, account Account) (Account, error)
	DeleteAccount(ctx context.Context, accountID int) error
}

type Service struct {
	repository       Repository
	configRepository uploader.ConfigRepository
	httpDoer         uploader.HTTPDoer
	now              func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Service) ListAccounts(ctx context.Context, req ListAccountsRequest) (AccountListResponse, error) {
	normalized := req.Normalized()
	if s == nil || s.repository == nil {
		return AccountListResponse{
			Page:     normalized.Page,
			PageSize: normalized.PageSize,
			Total:    0,
			Accounts: make([]Account, 0),
		}, nil
	}

	accounts, total, err := s.repository.ListAccounts(ctx, normalized)
	if err != nil {
		return AccountListResponse{}, err
	}

	return AccountListResponse{
		Page:     normalized.Page,
		PageSize: normalized.PageSize,
		Total:    total,
		Accounts: projectCompatibilityAccounts(accounts),
	}, nil
}

func (s *Service) GetCurrentAccount(ctx context.Context) (CurrentAccountResponse, error) {
	if s == nil || s.repository == nil {
		return CurrentAccountResponse{}, nil
	}

	currentID, err := s.repository.GetCurrentAccountID(ctx)
	if err != nil {
		return CurrentAccountResponse{}, err
	}
	if currentID == nil {
		return CurrentAccountResponse{}, nil
	}

	account, err := s.repository.GetAccountByID(ctx, *currentID)
	if err != nil {
		if err == ErrAccountNotFound {
			return CurrentAccountResponse{}, nil
		}
		return CurrentAccountResponse{}, err
	}
	account = projectCompatibilityAccount(account)
	return CurrentAccountResponse{
		CurrentAccountID: currentID,
		Account: &CurrentAccountSummary{
			ID:           account.ID,
			Email:        account.Email,
			Status:       account.Status,
			EmailService: account.EmailService,
			PlanType:     normalizePlanType(account.SubscriptionType),
		},
	}, nil
}

func (s *Service) GetAccountsStatsSummary(ctx context.Context) (AccountsStatsSummary, error) {
	if s == nil || s.repository == nil {
		return AccountsStatsSummary{
			ByStatus:       map[string]int{},
			ByEmailService: map[string]int{},
		}, nil
	}

	resp, err := s.repository.GetAccountsStatsSummary(ctx)
	if err != nil {
		return AccountsStatsSummary{}, err
	}
	if resp.ByStatus == nil {
		resp.ByStatus = map[string]int{}
	}
	if resp.ByEmailService == nil {
		resp.ByEmailService = map[string]int{}
	}
	return resp, nil
}

func (s *Service) GetAccountsOverviewStats(ctx context.Context) (AccountsOverviewStats, error) {
	if s == nil || s.repository == nil {
		return AccountsOverviewStats{
			ByStatus:       map[string]int{},
			ByEmailService: map[string]int{},
			BySource:       map[string]int{},
			BySubscription: map[string]int{},
			RecentAccounts: make([]AccountOverviewRecentItem, 0),
		}, nil
	}

	resp, err := s.repository.GetAccountsOverviewStats(ctx)
	if err != nil {
		return AccountsOverviewStats{}, err
	}
	if resp.ByStatus == nil {
		resp.ByStatus = map[string]int{}
	}
	if resp.ByEmailService == nil {
		resp.ByEmailService = map[string]int{}
	}
	if resp.BySource == nil {
		resp.BySource = map[string]int{}
	}
	if resp.BySubscription == nil {
		resp.BySubscription = map[string]int{}
	}
	if resp.RecentAccounts == nil {
		resp.RecentAccounts = make([]AccountOverviewRecentItem, 0)
	}
	return resp, nil
}

func (s *Service) ListOverviewCards(ctx context.Context, req AccountOverviewCardsRequest) (AccountOverviewCardsResponse, error) {
	normalized := req.Normalized()
	now := time.Now().UTC().Format(time.RFC3339)
	if s == nil || s.repository == nil {
		return AccountOverviewCardsResponse{
			CacheTTLSeconds: OverviewCacheTTLSeconds,
			NetworkMode:     "cache_only",
			Proxy:           normalized.Proxy,
			Accounts:        make([]AccountOverviewCard, 0),
			RefreshedAt:     now,
		}, nil
	}

	accounts, err := s.repository.ListAccountsForOverview(ctx, normalized)
	if err != nil {
		return AccountOverviewCardsResponse{}, err
	}
	currentID, err := s.repository.GetCurrentAccountID(ctx)
	if err != nil {
		return AccountOverviewCardsResponse{}, err
	}

	rows := make([]AccountOverviewCard, 0, len(accounts))
	for _, rawAccount := range accounts {
		account := projectCompatibilityAccount(rawAccount)
		if !isPaidSubscription(account.SubscriptionType) || isOverviewCardRemoved(account.ExtraData) {
			continue
		}
		rows = append(rows, buildOverviewCard(account, currentID))
	}

	return AccountOverviewCardsResponse{
		Total:            len(rows),
		CurrentAccountID: currentID,
		CacheTTLSeconds:  OverviewCacheTTLSeconds,
		NetworkMode:      "cache_only",
		Proxy:            normalized.Proxy,
		Accounts:         rows,
		RefreshedAt:      now,
	}, nil
}

func (s *Service) ListOverviewSelectable(ctx context.Context, req AccountOverviewSelectableRequest) (AccountOverviewSelectableResponse, error) {
	normalized := req.Normalized()
	if s == nil || s.repository == nil {
		return AccountOverviewSelectableResponse{Accounts: make([]AccountOverviewSelectableItem, 0)}, nil
	}

	accounts, err := s.repository.ListAccountsForSelectable(ctx, normalized)
	if err != nil {
		return AccountOverviewSelectableResponse{}, err
	}

	rows := make([]AccountOverviewSelectableItem, 0, len(accounts))
	for _, rawAccount := range accounts {
		account := projectCompatibilityAccount(rawAccount)
		if !isPaidSubscription(account.SubscriptionType) || !isOverviewCardRemoved(account.ExtraData) {
			continue
		}
		rows = append(rows, AccountOverviewSelectableItem{
			ID:               account.ID,
			Email:            account.Email,
			Password:         account.Password,
			Status:           account.Status,
			EmailService:     account.EmailService,
			SubscriptionType: firstNonEmpty(strings.TrimSpace(account.SubscriptionType), "free"),
			ClientID:         account.ClientID,
			AccountID:        account.AccountID,
			WorkspaceID:      account.WorkspaceID,
			HasAccessToken:   strings.TrimSpace(account.AccessToken) != "",
			CreatedAt:        formatTime(account.CreatedAt),
		})
	}

	return AccountOverviewSelectableResponse{
		Total:    len(rows),
		Accounts: rows,
	}, nil
}

func (s *Service) GetAccount(ctx context.Context, accountID int) (Account, error) {
	if s == nil || s.repository == nil {
		return Account{}, ErrRepositoryNotConfigured
	}

	account, err := s.repository.GetAccountByID(ctx, accountID)
	if err != nil {
		return Account{}, err
	}
	return projectCompatibilityAccount(account), nil
}

func (s *Service) GetAccountTokens(ctx context.Context, accountID int) (AccountTokensResponse, error) {
	if s == nil || s.repository == nil {
		return AccountTokensResponse{}, ErrRepositoryNotConfigured
	}

	account, err := s.repository.GetAccountByID(ctx, accountID)
	if err != nil {
		return AccountTokensResponse{}, err
	}
	account = projectCompatibilityAccount(account)

	sessionToken, sessionSource := resolveSessionToken(account)
	return AccountTokensResponse{
		ID:                 account.ID,
		Email:              account.Email,
		AccessToken:        account.AccessToken,
		RefreshToken:       account.RefreshToken,
		IDToken:            account.IDToken,
		SessionToken:       sessionToken,
		SessionTokenSource: sessionSource,
		DeviceID:           account.DeviceID,
		HasTokens:          strings.TrimSpace(account.AccessToken) != "" && strings.TrimSpace(account.RefreshToken) != "",
	}, nil
}

func (s *Service) GetAccountCookies(ctx context.Context, accountID int) (AccountCookiesResponse, error) {
	if s == nil || s.repository == nil {
		return AccountCookiesResponse{}, ErrRepositoryNotConfigured
	}

	account, err := s.repository.GetAccountByID(ctx, accountID)
	if err != nil {
		return AccountCookiesResponse{}, err
	}
	return AccountCookiesResponse{
		AccountID: accountID,
		Cookies:   account.Cookies,
	}, nil
}

func (s *Service) CreateManualAccount(ctx context.Context, req ManualAccountCreateRequest) (Account, error) {
	normalized := req.Normalized()
	if normalized.Email == "" || !strings.Contains(normalized.Email, "@") {
		return Account{}, ErrAccountEmailRequired
	}
	if normalized.Password == "" {
		return Account{}, fmt.Errorf("accounts: password is required")
	}

	if s == nil || s.repository == nil {
		return Account{}, ErrRepositoryNotConfigured
	}
	if _, found, err := s.repository.GetAccountByEmail(ctx, normalized.Email); err != nil {
		return Account{}, err
	} else if found {
		return Account{}, ErrAccountAlreadyExists
	}

	saved, err := s.UpsertAccount(ctx, UpsertAccountRequest{
		Email:            normalized.Email,
		Password:         normalized.Password,
		ClientID:         normalized.ClientID,
		SessionToken:     normalized.SessionToken,
		EmailService:     normalized.EmailService,
		AccountID:        normalized.AccountID,
		WorkspaceID:      normalized.WorkspaceID,
		AccessToken:      normalized.AccessToken,
		RefreshToken:     normalized.RefreshToken,
		IDToken:          normalized.IDToken,
		Cookies:          normalized.Cookies,
		ProxyUsed:        normalized.ProxyUsed,
		ExtraData:        normalized.Metadata,
		Status:           normalized.Status,
		Source:           normalized.Source,
		SubscriptionType: normalized.SubscriptionType,
		SubscriptionAt:   optionalSubscriptionTimestamp(normalized.SubscriptionType, s.nowUTC()),
		RegisteredAt:     cloneTimePtr(s.nowPtr()),
	})
	if err != nil {
		return Account{}, err
	}
	return projectCompatibilityAccount(saved), nil
}

func (s *Service) ImportAccounts(ctx context.Context, req ImportAccountsRequest) (ImportAccountsResult, error) {
	result := ImportAccountsResult{
		Success: true,
		Total:   len(req.Accounts),
		Errors:  make([]map[string]any, 0),
	}
	if s == nil || s.repository == nil {
		return result, ErrRepositoryNotConfigured
	}

	for index, raw := range req.Accounts {
		createReq := parseImportAccount(raw)
		if createReq.Email == "" || !strings.Contains(createReq.Email, "@") {
			result.Failed++
			result.Errors = append(result.Errors, map[string]any{"index": index + 1, "email": raw["email"], "error": "邮箱格式不正确"})
			continue
		}

		_, found, err := s.repository.GetAccountByEmail(ctx, createReq.Email)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, map[string]any{"index": index + 1, "email": createReq.Email, "error": err.Error()})
			continue
		}
		if found && !req.Overwrite {
			result.Skipped++
			continue
		}

		_, err = s.UpsertAccount(ctx, UpsertAccountRequest{
			Email:            createReq.Email,
			Password:         createReq.Password,
			ClientID:         createReq.ClientID,
			SessionToken:     createReq.SessionToken,
			EmailService:     createReq.EmailService,
			AccountID:        createReq.AccountID,
			WorkspaceID:      createReq.WorkspaceID,
			AccessToken:      createReq.AccessToken,
			RefreshToken:     createReq.RefreshToken,
			IDToken:          createReq.IDToken,
			Cookies:          createReq.Cookies,
			ProxyUsed:        createReq.ProxyUsed,
			ExtraData:        createReq.Metadata,
			Status:           createReq.Status,
			Source:           firstNonEmpty(createReq.Source, "import"),
			SubscriptionType: createReq.SubscriptionType,
			SubscriptionAt:   optionalSubscriptionTimestamp(createReq.SubscriptionType, s.nowUTC()),
			LastRefresh:      cloneTimePtr(s.nowPtr()),
			RegisteredAt:     cloneTimePtr(s.nowPtr()),
		})
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, map[string]any{"index": index + 1, "email": createReq.Email, "error": err.Error()})
			continue
		}
		if found {
			result.Updated++
		} else {
			result.Created++
		}
	}

	if len(result.Errors) == 0 {
		result.Errors = nil
	}
	return result, nil
}

func (s *Service) UpdateAccount(ctx context.Context, accountID int, req AccountUpdateRequest) (Account, error) {
	if s == nil || s.repository == nil {
		return Account{}, ErrRepositoryNotConfigured
	}
	account, err := s.repository.GetAccountByID(ctx, accountID)
	if err != nil {
		return Account{}, err
	}

	normalized := req.Normalized()
	if normalized.Status != "" {
		account.Status = normalized.Status
	}
	if len(normalized.Metadata) > 0 {
		if account.ExtraData == nil {
			account.ExtraData = map[string]any{}
		}
		for key, value := range normalized.Metadata {
			account.ExtraData[key] = value
		}
	}
	if normalized.Cookies != nil {
		account.Cookies = *normalized.Cookies
	}
	if normalized.SessionToken != nil {
		account.SessionToken = *normalized.SessionToken
		account.LastRefresh = s.nowPtr()
	}

	saved, err := s.repository.UpsertAccount(ctx, buildUpsertAccount(account))
	if err != nil {
		return Account{}, err
	}
	return projectCompatibilityAccount(saved), nil
}

func (s *Service) DeleteAccount(ctx context.Context, accountID int) (ActionResponse, error) {
	if s == nil || s.repository == nil {
		return ActionResponse{}, ErrRepositoryNotConfigured
	}
	account, err := s.repository.GetAccountByID(ctx, accountID)
	if err != nil {
		return ActionResponse{}, err
	}
	if err := s.repository.DeleteAccount(ctx, accountID); err != nil {
		return ActionResponse{}, err
	}
	return ActionResponse{
		Success: true,
		Message: fmt.Sprintf("账号 %s 已删除", account.Email),
	}, nil
}

func (s *Service) BatchDeleteAccounts(ctx context.Context, req AccountSelectionRequest) (BatchDeleteResponse, error) {
	accounts, missingIDs, err := s.listSelectedAccounts(ctx, req)
	if err != nil {
		return BatchDeleteResponse{}, err
	}

	resp := BatchDeleteResponse{
		Success:    true,
		MissingIDs: missingIDs,
	}
	for _, account := range accounts {
		if err := s.repository.DeleteAccount(ctx, account.ID); err != nil {
			resp.Errors = append(resp.Errors, fmt.Sprintf("ID %d: %v", account.ID, err))
			continue
		}
		resp.DeletedCount++
	}
	if len(resp.Errors) == 0 {
		resp.Errors = nil
	}
	return resp, nil
}

func (s *Service) BatchUpdateAccounts(ctx context.Context, req BatchUpdateRequest) (BatchUpdateResponse, error) {
	normalized := req.Normalized()
	accounts, missingIDs, err := s.listSelectedAccounts(ctx, normalized.AccountSelectionRequest)
	if err != nil {
		return BatchUpdateResponse{}, err
	}

	resp := BatchUpdateResponse{
		Success:        true,
		RequestedCount: requestedSelectionCount(normalized.AccountSelectionRequest, accounts, missingIDs),
		SkippedCount:   len(missingIDs),
		MissingIDs:     missingIDs,
	}
	for _, account := range accounts {
		account.Status = normalized.Status
		if _, err := s.repository.UpsertAccount(ctx, buildUpsertAccount(account)); err != nil {
			resp.Errors = append(resp.Errors, fmt.Sprintf("ID %d: %v", account.ID, err))
			continue
		}
		resp.UpdatedCount++
	}
	if resp.SkippedCount > 0 {
		resp.Message = fmt.Sprintf("部分账号不存在，已跳过 %d 个", resp.SkippedCount)
	}
	if len(resp.Errors) == 0 {
		resp.Errors = nil
	}
	return resp, nil
}

func (s *Service) ExportAccounts(ctx context.Context, format string, req AccountSelectionRequest) (AccountExportResponse, error) {
	accounts, _, err := s.listSelectedAccounts(ctx, req)
	if err != nil {
		return AccountExportResponse{}, err
	}
	switch strings.TrimSpace(format) {
	case "json":
		return exportAccountsJSON(accounts)
	case "csv":
		return exportAccountsCSV(accounts)
	case "sub2api":
		return exportAccountsSub2API(accounts)
	case "codex":
		return exportAccountsCodex(accounts)
	case "cpa":
		return exportAccountsCPA(accounts)
	default:
		return AccountExportResponse{}, ErrUnsupportedExportFormat
	}
}

func (s *Service) BatchRefreshTokens(ctx context.Context, req BatchTokenRefreshRequest) (BatchRefreshResponse, error) {
	accounts, _, err := s.listSelectedAccounts(ctx, req.AccountSelectionRequest)
	if err != nil {
		return BatchRefreshResponse{}, err
	}
	resp := BatchRefreshResponse{Errors: make([]map[string]any, 0)}
	for _, account := range accounts {
		result, err := s.refreshAccount(ctx, account, req.Proxy)
		if err != nil || !result.Success {
			resp.FailedCount++
			resp.Errors = append(resp.Errors, map[string]any{"id": account.ID, "error": firstNonEmpty(result.Error, errorText(err))})
			continue
		}
		resp.SuccessCount++
	}
	if len(resp.Errors) == 0 {
		resp.Errors = nil
	}
	return resp, nil
}

func (s *Service) RefreshAccountToken(ctx context.Context, accountID int, req TokenRefreshRequest) (TokenRefreshActionResponse, error) {
	account, err := s.repository.GetAccountByID(ctx, accountID)
	if err != nil {
		return TokenRefreshActionResponse{}, err
	}
	return s.refreshAccount(ctx, account, req.Proxy)
}

func (s *Service) BatchValidateTokens(ctx context.Context, req BatchTokenValidateRequest) (BatchValidateResponse, error) {
	accounts, _, err := s.listSelectedAccounts(ctx, req.AccountSelectionRequest)
	if err != nil {
		return BatchValidateResponse{}, err
	}
	resp := BatchValidateResponse{Details: make([]TokenValidateResponse, 0, len(accounts))}
	for _, account := range accounts {
		result, err := s.validateAccount(ctx, account, req.Proxy)
		if err != nil {
			result.Error = firstNonEmpty(result.Error, err.Error())
		}
		resp.Details = append(resp.Details, result)
		if result.Valid {
			resp.ValidCount++
		} else {
			resp.InvalidCount++
		}
	}
	return resp, nil
}

func (s *Service) ValidateAccountToken(ctx context.Context, accountID int, req TokenValidateRequest) (TokenValidateResponse, error) {
	account, err := s.repository.GetAccountByID(ctx, accountID)
	if err != nil {
		return TokenValidateResponse{}, err
	}
	return s.validateAccount(ctx, account, req.Proxy)
}

func (s *Service) BatchUploadAccountsCPA(ctx context.Context, req BatchCPAUploadRequest) (BatchUploadResponse, error) {
	return s.batchUpload(ctx, uploader.UploadKindCPA, req.AccountSelectionRequest, req.CPAServiceID, req.Proxy, 0, 0)
}

func (s *Service) UploadAccountCPA(ctx context.Context, accountID int, req CPAUploadRequest) (ActionResponse, error) {
	return s.singleUpload(ctx, uploader.UploadKindCPA, accountID, req.CPAServiceID, req.Proxy, 0, 0)
}

func (s *Service) BatchUploadAccountsSub2API(ctx context.Context, req BatchSub2APIUploadRequest) (BatchUploadResponse, error) {
	normalized := req.Normalized()
	return s.batchUpload(ctx, uploader.UploadKindSub2API, normalized.AccountSelectionRequest, normalized.ServiceID, "", normalized.Concurrency, normalized.Priority)
}

func (s *Service) UploadAccountSub2API(ctx context.Context, accountID int, req Sub2APIUploadRequest) (ActionResponse, error) {
	normalized := req.Normalized()
	return s.singleUpload(ctx, uploader.UploadKindSub2API, accountID, normalized.ServiceID, "", normalized.Concurrency, normalized.Priority)
}

func (s *Service) BatchUploadAccountsTM(ctx context.Context, req BatchTMUploadRequest) (BatchUploadResponse, error) {
	normalized := req.Normalized()
	return s.batchUpload(ctx, uploader.UploadKindTM, normalized.AccountSelectionRequest, normalized.ServiceID, "", 0, 0)
}

func (s *Service) UploadAccountTM(ctx context.Context, accountID int, req TMUploadRequest) (ActionResponse, error) {
	normalized := req.Normalized()
	return s.singleUpload(ctx, uploader.UploadKindTM, accountID, normalized.ServiceID, "", 0, 0)
}

func (s *Service) RemoveOverviewCards(ctx context.Context, req OverviewCardDeleteRequest) (OverviewCardMutationResponse, error) {
	accounts, _, err := s.listSelectedAccounts(ctx, selectionFromOverviewDelete(req))
	if err != nil {
		return OverviewCardMutationResponse{}, err
	}
	resp := OverviewCardMutationResponse{Success: true, Total: len(accounts)}
	for _, account := range accounts {
		if isOverviewCardRemoved(account.ExtraData) {
			continue
		}
		if account.ExtraData == nil {
			account.ExtraData = map[string]any{}
		}
		account.ExtraData[OverviewCardRemovedKey] = true
		if _, err := s.repository.UpsertAccount(ctx, buildUpsertAccount(account)); err != nil {
			return OverviewCardMutationResponse{}, err
		}
		resp.RemovedCount++
	}
	return resp, nil
}

func (s *Service) RestoreOverviewCard(ctx context.Context, accountID int) (ActionResponse, error) {
	account, err := s.repository.GetAccountByID(ctx, accountID)
	if err != nil {
		return ActionResponse{}, err
	}
	delete(account.ExtraData, OverviewCardRemovedKey)
	if _, err := s.repository.UpsertAccount(ctx, buildUpsertAccount(account)); err != nil {
		return ActionResponse{}, err
	}
	return ActionResponse{Success: true, Message: "restored"}, nil
}

func (s *Service) AttachOverviewCard(ctx context.Context, accountID int) (OverviewAttachResponse, error) {
	account, err := s.repository.GetAccountByID(ctx, accountID)
	if err != nil {
		return OverviewAttachResponse{}, err
	}
	already := !isOverviewCardRemoved(account.ExtraData)
	delete(account.ExtraData, OverviewCardRemovedKey)
	if _, err := s.repository.UpsertAccount(ctx, buildUpsertAccount(account)); err != nil {
		return OverviewAttachResponse{}, err
	}
	return OverviewAttachResponse{Success: true, ID: account.ID, Email: account.Email, AlreadyInCards: already}, nil
}

func (s *Service) RefreshOverview(ctx context.Context, req OverviewRefreshRequest) (OverviewRefreshResponse, error) {
	accounts, _, err := s.listSelectedAccounts(ctx, selectionFromOverviewRefresh(req))
	if err != nil {
		return OverviewRefreshResponse{}, err
	}
	resp := OverviewRefreshResponse{Details: make([]OverviewRefreshDetail, 0, len(accounts))}
	for _, account := range accounts {
		detail := OverviewRefreshDetail{ID: account.ID, Email: account.Email}
		if !isPaidSubscription(account.SubscriptionType) || isOverviewCardRemoved(account.ExtraData) {
			detail.Success = false
			detail.Error = "账号不在 Codex 卡片范围内，已跳过"
			resp.Details = append(resp.Details, detail)
			continue
		}
		accountProxy := firstNonEmpty(strings.TrimSpace(account.ProxyUsed), strings.TrimSpace(req.Proxy))
		overview, updated := s.refreshAccountOverview(ctx, &account, req.Force, accountProxy)
		if updated {
			if _, err := s.repository.UpsertAccount(ctx, buildUpsertAccount(account)); err != nil {
				detail.Success = false
				detail.Error = sanitizeOverviewError(err, account)
				resp.FailedCount++
				resp.Details = append(resp.Details, detail)
				continue
			}
		}
		if overviewRefreshFailed(overview) {
			detail.Success = false
			detail.Error = firstNonEmpty(
				strings.TrimSpace(extractStringMapValue(overview, "error")),
				"未获取到配额数据",
			)
			resp.FailedCount++
		} else {
			detail.Success = true
			detail.PlanType = normalizePlanType(firstNonEmpty(extractStringMapValue(overview, "plan_type"), account.SubscriptionType))
			resp.SuccessCount++
		}
		resp.Details = append(resp.Details, detail)
	}
	return resp, nil
}

func (s *Service) GetInboxCode(ctx context.Context, accountID int) (InboxCodeResponse, error) {
	account, err := s.repository.GetAccountByID(ctx, accountID)
	if err != nil {
		return InboxCodeResponse{}, err
	}
	pool := s.pool()
	if pool == nil {
		return InboxCodeResponse{Success: false, Error: "未找到可用的邮箱服务配置"}, nil
	}

	services, err := listInboxServices(ctx, pool)
	if err != nil {
		return InboxCodeResponse{}, err
	}
	serviceType, config := selectInboxServiceConfig(account, services)
	if serviceType == "" || len(config) == 0 {
		return InboxCodeResponse{Success: false, Error: "未找到可用的邮箱服务配置"}, nil
	}

	provider, err := mail.NewProvider(serviceType, config)
	if err != nil {
		return InboxCodeResponse{Success: false, Error: err.Error()}, nil
	}
	inbox, err := buildInbox(provider, account)
	if err != nil {
		return InboxCodeResponse{Success: false, Error: err.Error()}, nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	code, err := provider.WaitCode(waitCtx, inbox, mail.DefaultCodePattern)
	if err != nil {
		return InboxCodeResponse{Success: false, Error: err.Error()}, nil
	}
	if strings.TrimSpace(code) == "" {
		return InboxCodeResponse{Success: false, Error: "未收到验证码邮件"}, nil
	}
	return InboxCodeResponse{Success: true, Code: code, Email: account.Email}, nil
}

func (s *Service) UpsertAccount(ctx context.Context, req UpsertAccountRequest) (Account, error) {
	normalized, err := req.Normalized(time.Now().UTC())
	if err != nil {
		return Account{}, err
	}
	if s == nil || s.repository == nil {
		return Account{}, ErrRepositoryNotConfigured
	}

	existing, found, err := s.repository.GetAccountByEmail(ctx, normalized.Email)
	if err != nil {
		return Account{}, fmt.Errorf("lookup account by email: %w", err)
	}
	if !found && normalized.EmailService == "" {
		return Account{}, ErrAccountEmailServiceRequired
	}

	account := normalized.ToAccount()
	if found {
		account = mergeAccount(existing, normalized)
	}

	saved, err := s.repository.UpsertAccount(ctx, account)
	if err != nil {
		return Account{}, fmt.Errorf("upsert account: %w", err)
	}

	return saved, nil
}

func projectCompatibilityAccounts(accounts []Account) []Account {
	projected := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		projected = append(projected, projectCompatibilityAccount(account))
	}
	return projected
}

func projectCompatibilityAccount(account Account) Account {
	projected := account
	projected.HasRefreshToken = strings.TrimSpace(projected.RefreshToken) != ""
	projected.DeviceID = resolveDeviceID(projected)
	roleBadges, summary, count := buildTeamRelationCompatibility(projected)
	projected.TeamRoleBadges = roleBadges
	projected.TeamRelationSummary = summary
	projected.TeamRelationCount = count
	return projected
}

func buildOverviewCard(account Account, currentID *int) AccountOverviewCard {
	overview := compatibilityOverview(account)
	current := currentID != nil && account.ID == *currentID

	planType := normalizePlanType(firstNonEmpty(strings.TrimSpace(account.SubscriptionType), extractStringMapValue(overview, "plan_type")))
	planSource := firstNonEmpty(
		mapPlanSource(account.SubscriptionType),
		extractStringMapValue(overview, "plan_source"),
		"default",
	)

	return AccountOverviewCard{
		ID:                account.ID,
		Email:             account.Email,
		Status:            account.Status,
		EmailService:      account.EmailService,
		CreatedAt:         formatTime(account.CreatedAt),
		LastRefresh:       formatTime(account.LastRefresh),
		Current:           current,
		HasAccessToken:    strings.TrimSpace(account.AccessToken) != "",
		PlanType:          planType,
		PlanSource:        planSource,
		HasPlusOrTeam:     isPaidSubscription(firstNonEmpty(strings.TrimSpace(account.SubscriptionType), normalizeSubscriptionType(extractStringMapValue(overview, "plan_type")))),
		HourlyQuota:       extractQuotaSnapshot(overview, "hourly_quota"),
		WeeklyQuota:       extractQuotaSnapshot(overview, "weekly_quota"),
		CodeReviewQuota:   extractQuotaSnapshot(overview, "code_review_quota"),
		OverviewFetchedAt: extractStringMapValue(overview, "fetched_at"),
		OverviewStale:     extractBoolMapValue(overview, "stale"),
		OverviewError:     overview["error"],
	}
}

func compatibilityOverview(account Account) map[string]any {
	overview := UnknownQuotaSnapshot()
	_ = overview
	raw := cloneExtraData(account.ExtraData)
	cached, ok := raw[OverviewExtraDataKey].(map[string]any)
	if !ok || len(cached) == 0 {
		return fallbackOverview(account)
	}

	cloned := cloneExtraData(cached)
	if _, ok := cloned["hourly_quota"].(map[string]any); !ok {
		cloned["hourly_quota"] = UnknownQuotaSnapshot()
	}
	if _, ok := cloned["weekly_quota"].(map[string]any); !ok {
		cloned["weekly_quota"] = UnknownQuotaSnapshot()
	}
	if _, ok := cloned["code_review_quota"].(map[string]any); !ok {
		cloned["code_review_quota"] = UnknownQuotaSnapshot()
	}
	if strings.TrimSpace(extractStringMapValue(cloned, "plan_type")) == "" {
		cloned["plan_type"] = normalizePlanType(account.SubscriptionType)
	}
	if strings.TrimSpace(extractStringMapValue(cloned, "plan_source")) == "" {
		cloned["plan_source"] = mapPlanSource(account.SubscriptionType)
	}
	return cloned
}

func fallbackOverview(account Account) map[string]any {
	return map[string]any{
		"plan_type":         normalizePlanType(account.SubscriptionType),
		"plan_source":       firstNonEmpty(mapPlanSource(account.SubscriptionType), "default"),
		"hourly_quota":      UnknownQuotaSnapshot(),
		"weekly_quota":      UnknownQuotaSnapshot(),
		"code_review_quota": UnknownQuotaSnapshot(),
		"fetched_at":        time.Now().UTC().Format(time.RFC3339),
		"stale":             true,
	}
}

func fallbackOverviewWithError(account Account, errorMessage string, stale bool) map[string]any {
	overview := fallbackOverview(account)
	if strings.TrimSpace(errorMessage) != "" {
		overview["error"] = strings.TrimSpace(errorMessage)
	}
	overview["stale"] = stale
	return overview
}

func staleOverviewFromCache(cached map[string]any, errorMessage string) map[string]any {
	stale := cloneExtraData(cached)
	if stale == nil {
		stale = map[string]any{}
	}
	stale["stale"] = true
	if strings.TrimSpace(errorMessage) != "" {
		stale["error"] = strings.TrimSpace(errorMessage)
	}
	return stale
}

func overviewRefreshFailed(overview map[string]any) bool {
	return extractStringMapValue(extractQuotaSnapshot(overview, "hourly_quota"), "status") == "unknown" &&
		extractStringMapValue(extractQuotaSnapshot(overview, "weekly_quota"), "status") == "unknown"
}

func (s *Service) refreshAccountOverview(ctx context.Context, account *Account, forceRefresh bool, proxy string) (map[string]any, bool) {
	if account == nil {
		return map[string]any{}, false
	}
	extraData := cloneExtraData(account.ExtraData)
	cached, _ := extraData[OverviewExtraDataKey].(map[string]any)
	cacheStale := isOverviewCacheStale(cached, s.nowUTC())

	if strings.TrimSpace(account.AccessToken) == "" {
		if len(cached) > 0 {
			return staleOverviewFromCache(cached, "missing_access_token"), false
		}
		return fallbackOverviewWithError(*account, "missing_access_token", true), false
	}

	if !forceRefresh && len(cached) > 0 && !cacheStale {
		return cloneExtraData(cached), false
	}

	overview, err := s.fetchCodexOverview(ctx, *account, proxy)
	if err != nil {
		errorMessage := sanitizeOverviewError(err, *account)
		if len(cached) > 0 {
			return staleOverviewFromCache(cached, errorMessage), false
		}
		return fallbackOverviewWithError(*account, errorMessage, true), false
	}

	if len(cached) > 0 && !forceRefresh {
		for _, key := range []string{"hourly_quota", "weekly_quota", "code_review_quota"} {
			if extractStringMapValue(extractQuotaSnapshot(overview, key), "status") == "unknown" &&
				extractStringMapValue(extractQuotaSnapshot(cached, key), "status") == "ok" {
				overview[key] = cloneExtraData(extractQuotaSnapshot(cached, key))
			}
		}
	}

	s.syncOverviewSubscription(account, overview)
	if extraData == nil {
		extraData = map[string]any{}
	}
	extraData[OverviewExtraDataKey] = cloneExtraData(overview)
	account.ExtraData = extraData
	return overview, true
}

func (s *Service) fetchCodexOverview(ctx context.Context, account Account, proxy string) (map[string]any, error) {
	headers := buildOverviewHeaders(account)
	payloads := make(map[string]map[string]any, len(overviewUsageEndpoints))
	errorsList := make([]string, 0, len(overviewUsageEndpoints))

	for _, endpoint := range overviewUsageEndpoints {
		payload, err := s.requestOverviewJSON(ctx, endpoint.url, headers, proxy)
		if err != nil {
			status := overviewErrorStatus(err)
			if !endpoint.required && (status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound) {
				continue
			}
			errorsList = append(errorsList, fmt.Sprintf("%s: %s", endpoint.name, sanitizeOverviewError(err, account)))
			continue
		}
		payloads[endpoint.name] = payload
	}

	if len(payloads) == 0 {
		return nil, fmt.Errorf("所有概览接口请求失败")
	}

	if len(errorsList) > 3 {
		errorsList = errorsList[:3]
	}
	planType, planSource := detectOverviewPlan(account, payloads)
	return map[string]any{
		"plan_type":         planType,
		"plan_source":       planSource,
		"hourly_quota":      extractOverviewQuota("hourly", payloads),
		"weekly_quota":      extractOverviewQuota("weekly", payloads),
		"code_review_quota": extractOverviewCodeReviewQuota(payloads),
		"sources":           overviewPayloadSources(payloads),
		"fetched_at":        s.nowUTC().Format(time.RFC3339),
		"errors":            errorsList,
	}, nil
}

func (s *Service) requestOverviewJSON(ctx context.Context, rawURL string, headers map[string]string, proxy string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}

	doer := s.resolveOverviewDoer(proxy)
	resp, err := doer.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && strings.Contains(strings.ToLower(string(body)), "deactivated") {
		return nil, &overviewHTTPError{
			statusCode: resp.StatusCode,
			message:    "account_deactivated",
		}
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, &overviewHTTPError{
			statusCode: resp.StatusCode,
			message:    fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	var payload map[string]any
	if len(bytes.TrimSpace(body)) == 0 {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func (s *Service) resolveOverviewDoer(proxy string) uploader.HTTPDoer {
	if s != nil && s.httpDoer != nil {
		return s.httpDoer
	}
	if strings.TrimSpace(proxy) != "" {
		return newHTTPClient(proxy)
	}
	return http.DefaultClient
}

func buildOverviewHeaders(account Account) map[string]string {
	headers := map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(account.AccessToken),
		"Content-Type":  "application/json",
	}
	if accountID := firstNonEmpty(strings.TrimSpace(account.AccountID), strings.TrimSpace(account.WorkspaceID)); accountID != "" {
		headers["ChatGPT-Account-Id"] = accountID
	}
	if strings.TrimSpace(account.Cookies) != "" {
		headers["cookie"] = strings.TrimSpace(account.Cookies)
		if oaiDID := extractCookieValue(account.Cookies, "oai-did"); oaiDID != "" {
			headers["oai-device-id"] = oaiDID
		}
	}
	return headers
}

func isOverviewCacheStale(cached map[string]any, now time.Time) bool {
	if len(cached) == 0 {
		return true
	}
	fetchedAt := parseTimeString(extractStringMapValue(cached, "fetched_at"))
	if fetchedAt == nil {
		return true
	}
	return now.UTC().Sub(fetchedAt.UTC()) > OverviewCacheTTLSeconds*time.Second
}

func (s *Service) syncOverviewSubscription(account *Account, overview map[string]any) {
	if account == nil {
		return
	}
	planSource := extractStringMapValue(overview, "plan_source")
	trusted := []string{"me.", "wham_usage.", "codex_usage.", "id_token.", "access_token."}
	isTrusted := false
	for _, prefix := range trusted {
		if strings.HasPrefix(planSource, prefix) {
			isTrusted = true
			break
		}
	}
	if !isTrusted {
		return
	}
	current := normalizeSubscriptionType(account.SubscriptionType)
	detected := normalizeSubscriptionType(extractStringMapValue(overview, "plan_type"))
	if detected != "" && current != detected {
		account.SubscriptionType = detected
		now := s.nowUTC()
		account.SubscriptionAt = &now
	}
}

func sanitizeOverviewError(err error, account Account) string {
	message := strings.TrimSpace(err.Error())
	replacements := []string{
		strings.TrimSpace(account.AccessToken),
		strings.TrimSpace(account.RefreshToken),
		strings.TrimSpace(account.SessionToken),
		strings.TrimSpace(account.IDToken),
		strings.TrimSpace(account.Cookies),
	}
	for _, secret := range replacements {
		if secret == "" {
			continue
		}
		message = strings.ReplaceAll(message, secret, "[redacted]")
	}
	return firstNonEmpty(message, "未获取到配额数据")
}

func overviewErrorStatus(err error) int {
	if httpErr, ok := err.(*overviewHTTPError); ok && httpErr != nil {
		return httpErr.statusCode
	}
	return 0
}

func overviewPayloadSources(payloads map[string]map[string]any) []string {
	sources := make([]string, 0, len(payloads))
	for key := range payloads {
		sources = append(sources, key)
	}
	sort.Strings(sources)
	return sources
}

func extractOverviewQuota(window string, payloads map[string]map[string]any) map[string]any {
	if strict := extractOverviewQuotaFromRateLimit(window, payloads); len(strict) > 0 {
		return strict
	}
	return UnknownQuotaSnapshot()
}

func extractOverviewQuotaFromRateLimit(window string, payloads map[string]map[string]any) map[string]any {
	for _, sourceName := range []string{"wham_usage", "codex_usage"} {
		payload := payloads[sourceName]
		if len(payload) == 0 {
			continue
		}
		for sourcePath, rateLimit := range iterOverviewRateLimitCandidates(payload) {
			selectedKey, selectedPayload, ok := selectOverviewRateLimitWindow(rateLimit, window)
			if !ok {
				continue
			}
			parsed := extractOverviewQuotaFromWindow(selectedPayload)
			if len(parsed) == 0 {
				continue
			}
			parsed["source"] = fmt.Sprintf("%s.%s.%s", sourceName, sourcePath, selectedKey)
			return parsed
		}

		topLevel := map[string]any{
			"primary_window":   payload["primary_window"],
			"secondary_window": payload["secondary_window"],
		}
		selectedKey, selectedPayload, ok := selectOverviewRateLimitWindow(topLevel, window)
		if !ok {
			continue
		}
		parsed := extractOverviewQuotaFromWindow(selectedPayload)
		if len(parsed) == 0 {
			continue
		}
		parsed["source"] = fmt.Sprintf("%s.%s", sourceName, selectedKey)
		return parsed
	}
	return nil
}

func iterOverviewRateLimitCandidates(payload map[string]any) map[string]map[string]any {
	candidates := map[string]map[string]any{}
	if rateLimit, ok := payload["rate_limit"].(map[string]any); ok {
		candidates["rate_limit"] = rateLimit
	}
	for _, parentKey := range []string{"usage", "data", "quota", "limits", "codex"} {
		parent, ok := payload[parentKey].(map[string]any)
		if !ok {
			continue
		}
		rateLimit, ok := parent["rate_limit"].(map[string]any)
		if ok {
			candidates[parentKey+".rate_limit"] = rateLimit
		}
	}
	return candidates
}

func selectOverviewRateLimitWindow(rateLimit map[string]any, targetWindow string) (string, map[string]any, bool) {
	type candidate struct {
		key       string
		payload   map[string]any
		inferred  string
		confident bool
	}
	candidates := make([]candidate, 0, 2)
	for _, key := range []string{"primary_window", "secondary_window"} {
		raw, ok := rateLimit[key].(map[string]any)
		if !ok {
			continue
		}
		inferred, confident := inferOverviewWindowType(raw, key)
		candidates = append(candidates, candidate{key: key, payload: raw, inferred: inferred, confident: confident})
	}
	for _, item := range candidates {
		if item.confident && item.inferred == targetWindow {
			return item.key, item.payload, true
		}
	}
	for _, item := range candidates {
		if item.inferred == targetWindow {
			return item.key, item.payload, true
		}
	}
	fallbackKey := "primary_window"
	if targetWindow == "weekly" {
		fallbackKey = "secondary_window"
	}
	for _, item := range candidates {
		if item.key == fallbackKey {
			return item.key, item.payload, true
		}
	}
	return "", nil, false
}

func inferOverviewWindowType(windowPayload map[string]any, windowKey string) (string, bool) {
	seconds := overviewFloatValue(windowPayload["limit_window_seconds"])
	if seconds == nil {
		seconds = overviewFloatValue(windowPayload["window_seconds"])
	}
	if seconds != nil && *seconds > 0 {
		if *seconds >= 5*24*60*60 {
			return "weekly", true
		}
		if *seconds <= 12*60*60 {
			return "hourly", true
		}
	}
	if windowKey == "primary_window" {
		return "hourly", false
	}
	return "weekly", false
}

func extractOverviewQuotaFromWindow(windowPayload map[string]any) map[string]any {
	if len(windowPayload) == 0 {
		return nil
	}
	usedPercent := overviewFloatValue(windowPayload["used_percent"])
	remainingPercent := overviewFloatValue(windowPayload["remaining_percent"])
	if usedPercent != nil && *usedPercent >= 0 && *usedPercent <= 1 {
		value := *usedPercent * 100
		usedPercent = &value
	}
	if remainingPercent != nil && *remainingPercent >= 0 && *remainingPercent <= 1 {
		value := *remainingPercent * 100
		remainingPercent = &value
	}
	if remainingPercent == nil && usedPercent != nil {
		value := 100 - *usedPercent
		remainingPercent = &value
	}
	total := firstOverviewFloat(windowPayload, "total", "limit", "max", "capacity")
	used := overviewFloatValue(windowPayload["used"])
	remaining := overviewFloatValue(windowPayload["remaining"])
	if total != nil && remaining == nil && remainingPercent != nil {
		value := *total * (*remainingPercent / 100)
		remaining = &value
	}
	if total != nil && used == nil && remaining != nil {
		value := maxFloat(*total-*remaining, 0)
		used = &value
	}
	if total != nil && remaining == nil && used != nil {
		value := maxFloat(*total-*used, 0)
		remaining = &value
	}

	resetAt := firstOverviewTime(windowPayload, "resets_at", "reset_at", "next_reset_at", "next_reset")
	resetIn := firstOverviewFloat(windowPayload, "resets_in_seconds", "remaining_seconds", "seconds_to_reset", "reset_in")
	if resetIn == nil && resetAt != nil {
		value := maxFloat(resetAt.Sub(time.Now().UTC()).Seconds(), 0)
		resetIn = &value
	}
	if resetAt == nil && resetIn != nil {
		value := time.Now().UTC().Add(time.Duration(int(*resetIn)) * time.Second)
		resetAt = &value
	}
	if total == nil && used == nil && remaining == nil && remainingPercent == nil && resetAt == nil && resetIn == nil {
		return nil
	}
	if remainingPercent == nil && total != nil && *total > 0 && remaining != nil {
		value := maxFloat(minFloat((*remaining / *total)*100, 100), 0)
		remainingPercent = &value
	}

	return map[string]any{
		"used":          overviewIntValue(used),
		"total":         overviewIntValue(total),
		"remaining":     overviewIntValue(remaining),
		"percentage":    overviewRounded(remainingPercent),
		"reset_at":      formatTime(resetAt),
		"reset_in_text": formatOverviewDuration(resetIn),
		"status":        "ok",
	}
}

func extractOverviewCodeReviewQuota(payloads map[string]map[string]any) map[string]any {
	payload := payloads["wham_usage"]
	if len(payload) == 0 {
		return UnknownQuotaSnapshot()
	}
	reviewRateLimit, ok := payload["code_review_rate_limit"].(map[string]any)
	if !ok {
		return UnknownQuotaSnapshot()
	}
	for _, key := range []string{"primary_window", "secondary_window"} {
		window, _ := reviewRateLimit[key].(map[string]any)
		if parsed := extractOverviewQuotaFromWindow(window); len(parsed) > 0 {
			return parsed
		}
	}
	return UnknownQuotaSnapshot()
}

func detectOverviewPlan(account Account, payloads map[string]map[string]any) (string, string) {
	if detected, ok := detectOverviewPlanFromMe(payloads["me"]); ok {
		return detected, "me.plan"
	}
	for _, sourceName := range []string{"wham_usage", "codex_usage"} {
		if detected, ok := detectOverviewPlanFromPayload(payloads[sourceName]); ok {
			return detected, sourceName + ".plan"
		}
	}
	if normalized := normalizePlanType(account.SubscriptionType); normalized != "" {
		return normalized, firstNonEmpty(mapPlanSource(account.SubscriptionType), "default")
	}
	return "Basic", "default"
}

func detectOverviewPlanFromMe(payload map[string]any) (string, bool) {
	if len(payload) == 0 {
		return "", false
	}
	for _, candidate := range overviewPlanCandidates(payload) {
		normalized := normalizePlanType(candidate)
		if normalized != "" {
			return normalized, true
		}
	}
	if accountBlock, ok := payload["account"].(map[string]any); ok {
		for _, candidate := range overviewPlanCandidates(accountBlock) {
			normalized := normalizePlanType(candidate)
			if normalized != "" {
				return normalized, true
			}
		}
	}
	if subscriptionBlock, ok := payload["subscription"].(map[string]any); ok {
		for _, candidate := range overviewPlanCandidates(subscriptionBlock) {
			normalized := normalizePlanType(candidate)
			if normalized != "" {
				return normalized, true
			}
		}
	}
	for _, key := range []string{"has_paid_subscription", "has_active_subscription", "is_paid", "is_subscribed"} {
		if value, ok := payload[key].(bool); ok && value {
			return "Plus", true
		}
	}
	return "", false
}

func detectOverviewPlanFromPayload(payload map[string]any) (string, bool) {
	if len(payload) == 0 {
		return "", false
	}
	for _, candidate := range overviewPlanCandidates(payload) {
		normalized := normalizePlanType(candidate)
		if normalized != "" {
			return normalized, true
		}
	}
	return "", false
}

func overviewPlanCandidates(payload map[string]any) []string {
	candidates := make([]string, 0, 8)
	for _, key := range []string{"plan_type", "plan", "subscription_plan", "account_plan", "subscription_tier", "chatgpt_plan_type", "tier", "planType", "product"} {
		if value := stringValue(payload[key]); value != "" {
			candidates = append(candidates, value)
		}
	}
	return candidates
}

func firstOverviewFloat(payload map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		if value := overviewFloatValue(payload[key]); value != nil {
			return value
		}
	}
	return nil
}

func firstOverviewTime(payload map[string]any, keys ...string) *time.Time {
	for _, key := range keys {
		if value := parseTimeString(stringValue(payload[key])); value != nil {
			return value
		}
	}
	return nil
}

func overviewFloatValue(value any) *float64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case float64:
		return &typed
	case float32:
		result := float64(typed)
		return &result
	case int:
		result := float64(typed)
		return &result
	case int64:
		result := float64(typed)
		return &result
	case int32:
		result := float64(typed)
		return &result
	case json.Number:
		result, err := typed.Float64()
		if err != nil {
			return nil
		}
		return &result
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		result, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil
		}
		return &result
	default:
		return nil
	}
}

func overviewIntValue(value *float64) any {
	if value == nil {
		return nil
	}
	return int(*value)
}

func overviewRounded(value *float64) any {
	if value == nil {
		return nil
	}
	return mathRound(*value, 2)
}

func formatOverviewDuration(seconds *float64) string {
	if seconds == nil {
		return "-"
	}
	remaining := int(maxFloat(*seconds, 0))
	if remaining < 60 {
		return "1分钟"
	}
	minutes := remaining / 60
	days := minutes / 1440
	hours := (minutes % 1440) / 60
	mins := minutes % 60
	if days > 0 {
		return fmt.Sprintf("%d天%d小时", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%d小时%d分钟", hours, mins)
	}
	return fmt.Sprintf("%d分钟", maxInt(mins, 1))
}

func mathRound(value float64, precision int) float64 {
	scale := mathPow10(precision)
	return float64(int(value*scale+0.5)) / scale
}

func mathPow10(power int) float64 {
	result := 1.0
	for i := 0; i < power; i++ {
		result *= 10
	}
	return result
}

func maxFloat(left float64, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func minFloat(left float64, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func buildTeamRelationCompatibility(account Account) ([]string, map[string]any, int) {
	if normalizeSubscriptionType(account.SubscriptionType) != "team" {
		return []string{}, nil, 0
	}
	return []string{"owner"}, map[string]any{
		"owner_count":     1,
		"member_count":    0,
		"has_owner_role":  true,
		"has_member_role": false,
	}, 1
}

func resolveDeviceID(account Account) string {
	if strings.TrimSpace(account.DeviceID) != "" {
		return strings.TrimSpace(account.DeviceID)
	}
	if fromCookie := extractCookieValue(account.Cookies, "oai-did"); fromCookie != "" {
		return fromCookie
	}
	for _, key := range []string{"device_id", "oai_did", "oai-device-id"} {
		if value := strings.TrimSpace(fmt.Sprintf("%v", account.ExtraData[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func resolveSessionToken(account Account) (string, string) {
	if token := strings.TrimSpace(account.SessionToken); token != "" {
		return token, "db"
	}

	if direct := extractCookieValue(account.Cookies, "__Secure-next-auth.session-token"); direct != "" {
		return direct, "cookies"
	}

	chunks := sessionTokenChunkPattern.FindAllStringSubmatch(account.Cookies, -1)
	if len(chunks) == 0 {
		return "", "none"
	}

	type chunk struct {
		index int
		value string
	}
	parts := make([]chunk, 0, len(chunks))
	for _, item := range chunks {
		index, err := strconv.Atoi(item[1])
		if err != nil {
			continue
		}
		parts = append(parts, chunk{index: index, value: strings.TrimSpace(item[2])})
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].index < parts[j].index })

	var builder strings.Builder
	for _, item := range parts {
		builder.WriteString(item.value)
	}
	if builder.Len() == 0 {
		return "", "none"
	}
	return builder.String(), "cookies"
}

func extractCookieValue(cookiesText string, cookieName string) string {
	text := strings.TrimSpace(cookiesText)
	if text == "" {
		return ""
	}

	prefixes := []string{cookieName + "=", "; " + cookieName + "="}
	for _, prefix := range prefixes {
		index := strings.Index(text, prefix)
		if index < 0 {
			continue
		}
		start := index + len(prefix)
		if strings.HasPrefix(prefix, "; ") {
			start = index + len(prefix)
		}
		value := text[start:]
		if end := strings.Index(value, ";"); end >= 0 {
			value = value[:end]
		}
		return strings.TrimSpace(value)
	}
	return ""
}

func extractQuotaSnapshot(overview map[string]any, key string) map[string]any {
	value, ok := overview[key].(map[string]any)
	if !ok || len(value) == 0 {
		return UnknownQuotaSnapshot()
	}
	cloned := cloneExtraData(value)
	if _, ok := cloned["status"]; !ok {
		cloned["status"] = "unknown"
	}
	return cloned
}

func extractStringMapValue(data map[string]any, key string) string {
	return stringValue(data[key])
}

func extractBoolMapValue(data map[string]any, key string) bool {
	value, _ := data[key].(bool)
	return value
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func mapPlanSource(subscriptionType string) string {
	if normalizeSubscriptionType(subscriptionType) == "" {
		return ""
	}
	return "db.subscription_type"
}

func normalizePlanType(rawPlan string) string {
	value := normalizeFilterText(rawPlan)
	switch {
	case strings.Contains(value, "team"), strings.Contains(value, "enterprise"):
		return "Team"
	case strings.Contains(value, "plus"):
		return "Plus"
	case strings.Contains(value, "pro"):
		return "Pro"
	case value == "", strings.Contains(value, "free"), strings.Contains(value, "basic"):
		return "Basic"
	default:
		return strings.Title(value)
	}
}

func normalizeSubscriptionType(raw string) string {
	value := normalizeFilterText(raw)
	switch {
	case value == "", value == "free", value == "basic", value == "none", value == "null":
		return ""
	case strings.Contains(value, "team"), strings.Contains(value, "enterprise"):
		return "team"
	case strings.Contains(value, "plus"), strings.Contains(value, "pro"):
		return "plus"
	default:
		return value
	}
}

func isPaidSubscription(raw string) bool {
	switch normalizeSubscriptionType(raw) {
	case "plus", "team":
		return true
	default:
		return false
	}
}

func isOverviewCardRemoved(extraData map[string]any) bool {
	value, _ := extraData[OverviewCardRemovedKey].(bool)
	return value
}

func (s *Service) listSelectedAccounts(ctx context.Context, selection AccountSelectionRequest) ([]Account, []int, error) {
	if s == nil || s.repository == nil {
		return nil, nil, ErrRepositoryNotConfigured
	}
	normalized := selection.Normalized()
	accounts, err := s.repository.ListAccountsBySelection(ctx, normalized)
	if err != nil {
		return nil, nil, err
	}
	missing := missingIDs(normalized.IDs, accounts)
	return accounts, missing, nil
}

func requestedSelectionCount(selection AccountSelectionRequest, accounts []Account, missing []int) int {
	if selection.SelectAll {
		return len(accounts)
	}
	return len(selection.IDs)
}

func missingIDs(requested []int, accounts []Account) []int {
	if len(requested) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(accounts))
	for _, account := range accounts {
		seen[account.ID] = struct{}{}
	}
	missing := make([]int, 0)
	for _, id := range requested {
		if _, ok := seen[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return missing
}

func buildUpsertAccount(account Account) Account {
	return Account{
		ID:                account.ID,
		Email:             account.Email,
		Password:          account.Password,
		ClientID:          account.ClientID,
		SessionToken:      account.SessionToken,
		EmailService:      account.EmailService,
		EmailServiceID:    account.EmailServiceID,
		AccountID:         account.AccountID,
		WorkspaceID:       account.WorkspaceID,
		AccessToken:       account.AccessToken,
		RefreshToken:      account.RefreshToken,
		IDToken:           account.IDToken,
		Cookies:           account.Cookies,
		ProxyUsed:         account.ProxyUsed,
		LastRefresh:       cloneTimePtr(account.LastRefresh),
		ExpiresAt:         cloneTimePtr(account.ExpiresAt),
		ExtraData:         cloneExtraData(account.ExtraData),
		CPAUploaded:       account.CPAUploaded,
		CPAUploadedAt:     cloneTimePtr(account.CPAUploadedAt),
		Sub2APIUploaded:   account.Sub2APIUploaded,
		Sub2APIUploadedAt: cloneTimePtr(account.Sub2APIUploadedAt),
		Status:            account.Status,
		Source:            account.Source,
		SubscriptionType:  account.SubscriptionType,
		SubscriptionAt:    cloneTimePtr(account.SubscriptionAt),
		RegisteredAt:      cloneTimePtr(account.RegisteredAt),
	}
}

func optionalSubscriptionTimestamp(subscriptionType string, now time.Time) *time.Time {
	if normalizeSubscriptionType(subscriptionType) == "" {
		return nil
	}
	cloned := now
	return &cloned
}

func (s *Service) nowUTC() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now().UTC()
}

func (s *Service) nowPtr() *time.Time {
	now := s.nowUTC()
	return &now
}

func parseImportAccount(raw map[string]any) ManualAccountCreateRequest {
	request := ManualAccountCreateRequest{
		Email:            stringValue(raw["email"]),
		Password:         stringValue(raw["password"]),
		EmailService:     firstNonEmpty(stringValue(raw["email_service"]), "manual"),
		Status:           firstNonEmpty(normalizeFilterText(stringValue(raw["status"])), DefaultAccountStatus),
		ClientID:         stringValue(raw["client_id"]),
		AccountID:        stringValue(raw["account_id"]),
		WorkspaceID:      firstNonEmpty(stringValue(raw["workspace_id"]), stringValue(raw["account_id"])),
		AccessToken:      stringValue(raw["access_token"]),
		RefreshToken:     stringValue(raw["refresh_token"]),
		IDToken:          stringValue(raw["id_token"]),
		SessionToken:     stringValue(raw["session_token"]),
		Cookies:          stringValue(raw["cookies"]),
		ProxyUsed:        stringValue(raw["proxy_used"]),
		Source:           firstNonEmpty(stringValue(raw["source"]), "import"),
		SubscriptionType: firstNonEmpty(stringValue(raw["subscription_type"]), stringValue(raw["plan_type"])),
		Metadata:         make(map[string]any),
	}
	for _, key := range []string{"auth_mode", "user_id", "organization_id", "account_name", "account_structure", "quota", "tags", "created_at", "last_used"} {
		if value, ok := raw[key]; ok && value != nil {
			request.Metadata[key] = value
		}
	}
	return request.Normalized()
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func exportAccountsJSON(accounts []Account) (AccountExportResponse, error) {
	items := make([]map[string]any, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, map[string]any{
			"email":             account.Email,
			"password":          account.Password,
			"client_id":         account.ClientID,
			"account_id":        account.AccountID,
			"workspace_id":      account.WorkspaceID,
			"access_token":      account.AccessToken,
			"refresh_token":     account.RefreshToken,
			"id_token":          account.IDToken,
			"session_token":     account.SessionToken,
			"email_service":     account.EmailService,
			"registered_at":     formatTime(account.RegisteredAt),
			"last_refresh":      formatTime(account.LastRefresh),
			"expires_at":        formatTime(account.ExpiresAt),
			"status":            account.Status,
			"subscription_type": firstNonEmpty(account.SubscriptionType, "free"),
			"source":            account.Source,
		})
	}
	content, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return AccountExportResponse{}, err
	}
	return AccountExportResponse{
		ContentType: "application/json",
		Filename:    "accounts_" + sTimestamp() + ".json",
		Content:     content,
	}, nil
}

func exportAccountsCSV(accounts []Account) (AccountExportResponse, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"ID", "Email", "Password", "Client ID", "Account ID", "Workspace ID", "Access Token", "Refresh Token", "ID Token", "Session Token", "Email Service", "Status", "Registered At", "Last Refresh", "Expires At"}); err != nil {
		return AccountExportResponse{}, err
	}
	for _, account := range accounts {
		if err := writer.Write([]string{
			strconv.Itoa(account.ID),
			account.Email,
			account.Password,
			account.ClientID,
			account.AccountID,
			account.WorkspaceID,
			account.AccessToken,
			account.RefreshToken,
			account.IDToken,
			account.SessionToken,
			account.EmailService,
			account.Status,
			formatTime(account.RegisteredAt),
			formatTime(account.LastRefresh),
			formatTime(account.ExpiresAt),
		}); err != nil {
			return AccountExportResponse{}, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return AccountExportResponse{}, err
	}
	return AccountExportResponse{
		ContentType: "text/csv",
		Filename:    "accounts_" + sTimestamp() + ".csv",
		Content:     buffer.Bytes(),
	}, nil
}

func exportAccountsSub2API(accounts []Account) (AccountExportResponse, error) {
	uploadAccounts := make([]uploader.UploadAccount, 0, len(accounts))
	for _, account := range accounts {
		uploadAccounts = append(uploadAccounts, toUploadAccount(account))
	}
	payload, err := uploader.BuildSub2APIBatchPayload(uploader.ServiceConfig{Kind: uploader.UploadKindSub2API}, uploadAccounts, uploader.Sub2APIBatchOptions{})
	if err != nil {
		return AccountExportResponse{}, err
	}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return AccountExportResponse{}, err
	}
	filename := "sub2api_tokens_" + sTimestamp() + ".json"
	if len(accounts) == 1 {
		filename = accounts[0].Email + "_sub2api.json"
	}
	return AccountExportResponse{ContentType: "application/json", Filename: filename, Content: content}, nil
}

func exportAccountsCodex(accounts []Account) (AccountExportResponse, error) {
	lines := make([]string, 0, len(accounts))
	for _, account := range accounts {
		line, err := json.Marshal(map[string]any{
			"email":         account.Email,
			"password":      account.Password,
			"client_id":     account.ClientID,
			"access_token":  account.AccessToken,
			"refresh_token": account.RefreshToken,
			"session_token": account.SessionToken,
			"account_id":    account.AccountID,
			"workspace_id":  account.WorkspaceID,
			"cookies":       account.Cookies,
			"type":          "codex",
			"source":        firstNonEmpty(account.Source, "manual"),
		})
		if err != nil {
			return AccountExportResponse{}, err
		}
		lines = append(lines, string(line))
	}
	return AccountExportResponse{
		ContentType: "application/x-ndjson",
		Filename:    "codex_accounts_" + sTimestamp() + ".jsonl",
		Content:     []byte(strings.Join(lines, "\n")),
	}, nil
}

func exportAccountsCPA(accounts []Account) (AccountExportResponse, error) {
	if len(accounts) == 1 {
		file, err := uploader.BuildCPAAuthFile(toUploadAccount(accounts[0]))
		if err != nil {
			return AccountExportResponse{}, err
		}
		return AccountExportResponse{
			ContentType: file.ContentType,
			Filename:    file.Filename,
			Content:     file.Content,
		}, nil
	}

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for _, account := range accounts {
		file, err := uploader.BuildCPAAuthFile(toUploadAccount(account))
		if err != nil {
			return AccountExportResponse{}, err
		}
		writer, err := archive.Create(file.Filename)
		if err != nil {
			return AccountExportResponse{}, err
		}
		if _, err := writer.Write(file.Content); err != nil {
			return AccountExportResponse{}, err
		}
	}
	if err := archive.Close(); err != nil {
		return AccountExportResponse{}, err
	}
	return AccountExportResponse{
		ContentType: "application/zip",
		Filename:    "cpa_tokens_" + sTimestamp() + ".zip",
		Content:     buffer.Bytes(),
	}, nil
}

func sTimestamp() string {
	return time.Now().UTC().Format("20060102_150405")
}

func (s *Service) refreshAccount(ctx context.Context, account Account, proxy string) (TokenRefreshActionResponse, error) {
	sessionToken, _ := resolveSessionToken(account)
	if sessionToken != "" {
		accessToken, expiresAt, err := refreshWithSessionToken(ctx, sessionToken, proxy)
		if err == nil {
			account.AccessToken = accessToken
			account.LastRefresh = s.nowPtr()
			account.ExpiresAt = cloneTimePtr(expiresAt)
			if account.RefreshToken != "" {
				account.Status = DefaultAccountStatus
			}
			if _, err := s.repository.UpsertAccount(ctx, buildUpsertAccount(account)); err != nil {
				return TokenRefreshActionResponse{}, err
			}
			return TokenRefreshActionResponse{Success: true, Message: "Token 刷新成功", ExpiresAt: formatTime(expiresAt)}, nil
		}
	}

	if strings.TrimSpace(account.RefreshToken) == "" {
		return TokenRefreshActionResponse{Success: false, Error: "账号没有可用的刷新方式（缺少 session_token 和 refresh_token）"}, nil
	}

	accessToken, refreshToken, expiresAt, err := refreshWithOAuthToken(ctx, account.RefreshToken, firstNonEmpty(account.ClientID, defaultOpenAIClientID), proxy)
	if err != nil {
		return TokenRefreshActionResponse{Success: false, Error: err.Error()}, nil
	}
	account.AccessToken = accessToken
	if refreshToken != "" {
		account.RefreshToken = refreshToken
	}
	account.LastRefresh = s.nowPtr()
	account.ExpiresAt = cloneTimePtr(expiresAt)
	account.Status = DefaultAccountStatus
	if _, err := s.repository.UpsertAccount(ctx, buildUpsertAccount(account)); err != nil {
		return TokenRefreshActionResponse{}, err
	}
	return TokenRefreshActionResponse{Success: true, Message: "Token 刷新成功", ExpiresAt: formatTime(expiresAt)}, nil
}

func (s *Service) validateAccount(ctx context.Context, account Account, proxy string) (TokenValidateResponse, error) {
	resp := TokenValidateResponse{ID: account.ID}
	if strings.TrimSpace(account.AccessToken) == "" {
		account.Status = "failed"
		_, _ = s.repository.UpsertAccount(ctx, buildUpsertAccount(account))
		resp.Error = "账号没有 access_token"
		return resp, nil
	}

	valid, errText := validateAccessToken(ctx, account.AccessToken, proxy)
	resp.Valid = valid
	resp.Error = errText
	nextStatus := account.Status
	if valid {
		nextStatus = resolvePartialAccountStatus(account)
	} else {
		lower := strings.ToLower(errText)
		switch {
		case strings.Contains(lower, "401"), strings.Contains(lower, "过期"), strings.Contains(lower, "invalid"):
			nextStatus = "expired"
		case strings.Contains(lower, "403"), strings.Contains(lower, "封禁"), strings.Contains(lower, "forbidden"):
			nextStatus = "banned"
		default:
			nextStatus = "failed"
		}
	}
	if normalizeFilterText(account.Status) != normalizeFilterText(nextStatus) {
		account.Status = nextStatus
		_, _ = s.repository.UpsertAccount(ctx, buildUpsertAccount(account))
	}
	return resp, nil
}

func (s *Service) batchUpload(ctx context.Context, kind uploader.UploadKind, selection AccountSelectionRequest, serviceID *int, proxy string, concurrency int, priority int) (BatchUploadResponse, error) {
	accounts, _, err := s.listSelectedAccounts(ctx, selection)
	if err != nil {
		return BatchUploadResponse{}, err
	}
	if len(accounts) == 0 {
		return BatchUploadResponse{}, nil
	}
	serviceConfig, err := s.selectServiceConfig(ctx, kind, serviceID)
	if err != nil {
		return BatchUploadResponse{}, err
	}
	sender, err := uploader.NewSender(kind, s.resolveHTTPDoer())
	if err != nil {
		return BatchUploadResponse{}, err
	}

	uploadAccounts := make([]uploader.UploadAccount, 0, len(accounts))
	details := make([]map[string]any, 0, len(accounts))
	skippedCount := 0
	for _, account := range accounts {
		if strings.TrimSpace(account.AccessToken) == "" {
			skippedCount++
			details = append(details, map[string]any{"id": account.ID, "email": account.Email, "success": false, "error": "账号缺少 Token，无法上传"})
			continue
		}
		uploadAccounts = append(uploadAccounts, toUploadAccount(account))
	}
	if len(uploadAccounts) == 0 {
		return BatchUploadResponse{SkippedCount: skippedCount, Details: details}, nil
	}

	results, err := sender.Send(ctx, uploader.SendRequest{
		Service:  serviceConfig,
		Accounts: uploadAccounts,
		Sub2API:  uploader.Sub2APIBatchOptions{Concurrency: concurrency, Priority: priority},
	})
	if err != nil {
		return BatchUploadResponse{}, err
	}

	resp := BatchUploadResponse{SkippedCount: skippedCount, Details: details}
	for _, result := range results {
		detail := map[string]any{"email": result.AccountEmail, "success": result.Success, "message": result.Message}
		resp.Details = append(resp.Details, detail)
		if result.Success {
			resp.SuccessCount++
			if kind == uploader.UploadKindCPA || kind == uploader.UploadKindSub2API {
				if account, ok := findAccountByEmail(accounts, result.AccountEmail); ok {
					markUploadFlags(kind, &account, s.nowUTC())
					_, _ = s.repository.UpsertAccount(ctx, buildUpsertAccount(account))
				}
			}
		} else {
			resp.FailedCount++
		}
	}
	if len(resp.Details) == 0 {
		resp.Details = nil
	}
	return resp, nil
}

func (s *Service) singleUpload(ctx context.Context, kind uploader.UploadKind, accountID int, serviceID *int, proxy string, concurrency int, priority int) (ActionResponse, error) {
	account, err := s.repository.GetAccountByID(ctx, accountID)
	if err != nil {
		return ActionResponse{}, err
	}
	if strings.TrimSpace(account.AccessToken) == "" {
		return ActionResponse{Success: false, Error: "账号缺少 Token，无法上传"}, nil
	}
	serviceConfig, err := s.selectServiceConfig(ctx, kind, serviceID)
	if err != nil {
		return ActionResponse{}, err
	}
	sender, err := uploader.NewSender(kind, s.resolveHTTPDoer())
	if err != nil {
		return ActionResponse{}, err
	}
	results, err := sender.Send(ctx, uploader.SendRequest{
		Service:  serviceConfig,
		Accounts: []uploader.UploadAccount{toUploadAccount(account)},
		Sub2API:  uploader.Sub2APIBatchOptions{Concurrency: concurrency, Priority: priority},
	})
	if err != nil {
		return ActionResponse{}, err
	}
	if len(results) == 0 {
		return ActionResponse{Success: false, Error: "上传未返回结果"}, nil
	}
	result := results[0]
	if result.Success && (kind == uploader.UploadKindCPA || kind == uploader.UploadKindSub2API) {
		markUploadFlags(kind, &account, s.nowUTC())
		_, _ = s.repository.UpsertAccount(ctx, buildUpsertAccount(account))
	}
	if result.Success {
		return ActionResponse{Success: true, Message: firstNonEmpty(strings.TrimSpace(result.Message), "上传成功")}, nil
	}
	return ActionResponse{Success: false, Error: result.Message}, nil
}

func (s *Service) selectServiceConfig(ctx context.Context, kind uploader.UploadKind, serviceID *int) (uploader.ServiceConfig, error) {
	repo := s.resolveConfigRepository()
	if repo == nil {
		return uploader.ServiceConfig{}, uploader.ErrConfigRepositoryNotConfigured
	}
	ids := []int(nil)
	if serviceID != nil && *serviceID > 0 {
		ids = []int{*serviceID}
	}
	switch kind {
	case uploader.UploadKindCPA:
		configs, err := repo.ListCPAServiceConfigs(ctx, ids)
		if err != nil {
			return uploader.ServiceConfig{}, err
		}
		if len(configs) == 0 {
			return uploader.ServiceConfig{}, uploader.ErrServiceConfigNotFound
		}
		return configs[0], nil
	case uploader.UploadKindSub2API:
		configs, err := repo.ListSub2APIServiceConfigs(ctx, ids)
		if err != nil {
			return uploader.ServiceConfig{}, err
		}
		if len(configs) == 0 {
			return uploader.ServiceConfig{}, uploader.ErrServiceConfigNotFound
		}
		return configs[0], nil
	case uploader.UploadKindTM:
		configs, err := repo.ListTMServiceConfigs(ctx, ids)
		if err != nil {
			return uploader.ServiceConfig{}, err
		}
		if len(configs) == 0 {
			return uploader.ServiceConfig{}, uploader.ErrServiceConfigNotFound
		}
		return configs[0], nil
	default:
		return uploader.ServiceConfig{}, uploader.ErrUploadKindInvalid
	}
}

func markUploadFlags(kind uploader.UploadKind, account *Account, now time.Time) {
	if account == nil {
		return
	}
	switch kind {
	case uploader.UploadKindCPA:
		account.CPAUploaded = true
		account.CPAUploadedAt = cloneTimePtr(&now)
	case uploader.UploadKindSub2API:
		account.Sub2APIUploaded = true
		account.Sub2APIUploadedAt = cloneTimePtr(&now)
	}
}

func findAccountByEmail(accounts []Account, email string) (Account, bool) {
	for _, account := range accounts {
		if strings.EqualFold(strings.TrimSpace(account.Email), strings.TrimSpace(email)) {
			return account, true
		}
	}
	return Account{}, false
}

func toUploadAccount(account Account) uploader.UploadAccount {
	return uploader.UploadAccount{
		ID:           account.ID,
		Email:        account.Email,
		AccessToken:  account.AccessToken,
		RefreshToken: account.RefreshToken,
		SessionToken: account.SessionToken,
		ClientID:     account.ClientID,
		AccountID:    account.AccountID,
		WorkspaceID:  account.WorkspaceID,
		IDToken:      account.IDToken,
		ExpiresAt:    cloneTimePtr(account.ExpiresAt),
		LastRefresh:  cloneTimePtr(account.LastRefresh),
	}
}

func selectionFromOverviewDelete(req OverviewCardDeleteRequest) AccountSelectionRequest {
	normalized := req.Normalized()
	return AccountSelectionRequest{
		IDs:                normalized.IDs,
		SelectAll:          normalized.SelectAll,
		StatusFilter:       normalized.StatusFilter,
		EmailServiceFilter: normalized.EmailServiceFilter,
		SearchFilter:       normalized.SearchFilter,
	}
}

func selectionFromOverviewRefresh(req OverviewRefreshRequest) AccountSelectionRequest {
	normalized := req.Normalized()
	return AccountSelectionRequest{
		IDs:                normalized.IDs,
		SelectAll:          normalized.SelectAll,
		StatusFilter:       normalized.StatusFilter,
		EmailServiceFilter: normalized.EmailServiceFilter,
		SearchFilter:       normalized.SearchFilter,
	}
}

func (s *Service) resolveConfigRepository() uploader.ConfigRepository {
	if s != nil && s.configRepository != nil {
		return s.configRepository
	}
	if pool := s.pool(); pool != nil {
		return uploader.NewPostgresConfigRepository(pool)
	}
	return nil
}

func (s *Service) resolveHTTPDoer() uploader.HTTPDoer {
	if s != nil && s.httpDoer != nil {
		return s.httpDoer
	}
	return http.DefaultClient
}

func (s *Service) pool() *pgxpool.Pool {
	if s == nil {
		return nil
	}
	repo, ok := s.repository.(*PostgresRepository)
	if !ok || repo == nil {
		return nil
	}
	pool, _ := repo.db.(*pgxpool.Pool)
	return pool
}

type inboxServiceRecord struct {
	ServiceType string
	Config      map[string]any
	Enabled     bool
	Priority    int
}

func listInboxServices(ctx context.Context, pool *pgxpool.Pool) ([]inboxServiceRecord, error) {
	if pool == nil {
		return nil, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT service_type, COALESCE(config::text, '{}'), enabled, priority
		FROM email_services
		WHERE enabled = TRUE
		ORDER BY priority ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query inbox services: %w", err)
	}
	defer rows.Close()

	services := make([]inboxServiceRecord, 0)
	for rows.Next() {
		var (
			record    inboxServiceRecord
			configRaw string
		)
		if err := rows.Scan(&record.ServiceType, &configRaw, &record.Enabled, &record.Priority); err != nil {
			return nil, fmt.Errorf("scan inbox service: %w", err)
		}
		record.Config = map[string]any{}
		if strings.TrimSpace(configRaw) != "" {
			if err := json.Unmarshal([]byte(configRaw), &record.Config); err != nil {
				return nil, fmt.Errorf("decode inbox service config: %w", err)
			}
		}
		services = append(services, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inbox services: %w", err)
	}
	return services, nil
}

func selectInboxServiceConfig(account Account, services []inboxServiceRecord) (string, map[string]any) {
	serviceType := normalizeFilterText(account.EmailService)
	if serviceType == "" {
		return "", nil
	}
	domain := ""
	if parts := strings.Split(account.Email, "@"); len(parts) == 2 {
		domain = strings.ToLower(strings.TrimSpace(parts[1]))
	}
	var firstMatch *inboxServiceRecord
	for i := range services {
		service := services[i]
		if !service.Enabled || normalizeFilterText(service.ServiceType) != serviceType {
			continue
		}
		if firstMatch == nil {
			firstMatch = &services[i]
		}
		if serviceType == "outlook" && strings.EqualFold(stringValue(service.Config["email"]), account.Email) {
			return service.ServiceType, cloneExtraData(service.Config)
		}
		if serviceType == "moe_mail" {
			cfgDomain := strings.ToLower(firstNonEmpty(stringValue(service.Config["default_domain"]), stringValue(service.Config["domain"])))
			if cfgDomain != "" && cfgDomain == domain {
				return service.ServiceType, cloneExtraData(service.Config)
			}
		}
	}
	if firstMatch == nil {
		return "", nil
	}
	return firstMatch.ServiceType, cloneExtraData(firstMatch.Config)
}

func buildInbox(provider mail.Provider, account Account) (mail.Inbox, error) {
	inbox, err := provider.Create(context.Background())
	if err != nil {
		return mail.Inbox{}, err
	}
	if strings.EqualFold(strings.TrimSpace(inbox.Email), strings.TrimSpace(account.Email)) || strings.TrimSpace(account.EmailService) == "outlook" {
		inbox.OTPSentAt = time.Now().UTC()
		return inbox, nil
	}
	if strings.TrimSpace(account.EmailServiceID) == "" {
		return mail.Inbox{}, fmt.Errorf("邮箱服务缺少 inbox token")
	}
	return mail.Inbox{
		Email:     account.Email,
		Token:     account.EmailServiceID,
		OTPSentAt: time.Now().UTC(),
	}, nil
}

const (
	defaultOpenAIClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultOpenAIRedirectURI = "http://localhost:1455/auth/callback"
)

func refreshWithSessionToken(ctx context.Context, sessionToken string, proxy string) (string, *time.Time, error) {
	client := newHTTPClient(proxy)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/api/auth/session", nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", defaultBrowserUA)
	req.Header.Set("cookie", "__Secure-next-auth.session-token="+sessionToken)
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	var payload map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("Session token 刷新失败: HTTP %d", resp.StatusCode)
	}
	accessToken := firstNonEmpty(stringValue(payload["accessToken"]), stringValue(payload["access_token"]))
	if accessToken == "" {
		return "", nil, fmt.Errorf("Session token 刷新失败: 未找到 accessToken")
	}
	expiresAt := parseTimeString(stringValue(payload["expires"]))
	return accessToken, expiresAt, nil
}

func refreshWithOAuthToken(ctx context.Context, refreshToken string, clientID string, proxy string) (string, string, *time.Time, error) {
	client := newHTTPClient(proxy)
	form := url.Values{
		"client_id":     []string{clientID},
		"grant_type":    []string{"refresh_token"},
		"refresh_token": []string{refreshToken},
		"redirect_uri":  []string{defaultOpenAIRedirectURI},
	}
	body := strings.NewReader(form.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://auth.openai.com/oauth/token", body)
	if err != nil {
		return "", "", nil, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.Header.Set("accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", nil, err
	}
	defer resp.Body.Close()
	var payload map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if resp.StatusCode != http.StatusOK {
		return "", "", nil, fmt.Errorf("OAuth token 刷新失败: HTTP %d", resp.StatusCode)
	}
	accessToken := stringValue(payload["access_token"])
	if accessToken == "" {
		return "", "", nil, fmt.Errorf("OAuth token 刷新失败: 未找到 access_token")
	}
	nextRefresh := firstNonEmpty(stringValue(payload["refresh_token"]), refreshToken)
	expiresIn, _ := strconv.Atoi(firstNonEmpty(stringValue(payload["expires_in"]), "3600"))
	expiresAt := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second)
	return accessToken, nextRefresh, &expiresAt, nil
}

func validateAccessToken(ctx context.Context, accessToken string, proxy string) (bool, string) {
	client := newHTTPClient(proxy)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/backend-api/me", nil)
	if err != nil {
		return false, err.Error()
	}
	req.Header.Set("authorization", "Bearer "+accessToken)
	req.Header.Set("accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, ""
	case http.StatusUnauthorized:
		return false, "Token 无效或已过期"
	case http.StatusForbidden:
		return false, "账号可能被封禁"
	default:
		return false, fmt.Sprintf("验证失败: HTTP %d", resp.StatusCode)
	}
}

const defaultBrowserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func newHTTPClient(proxy string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.TrimSpace(proxy) != "" {
		if proxyURL, err := url.Parse(proxy); err == nil && proxyURL != nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: transport}
}

func parseTimeString(value string) *time.Time {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func resolvePartialAccountStatus(account Account) string {
	if strings.TrimSpace(account.RefreshToken) != "" {
		return DefaultAccountStatus
	}
	if strings.TrimSpace(account.Source) == "login" && strings.TrimSpace(account.Password) == "" {
		return "login_incomplete"
	}
	if detected, _ := account.ExtraData["existing_account_detected"].(bool); detected && strings.TrimSpace(account.Password) == "" {
		return "login_incomplete"
	}
	return "token_pending"
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mergeAccount(existing Account, incoming UpsertAccountRequest) Account {
	merged := existing
	merged.Email = incoming.Email
	originalStatus := strings.TrimSpace(existing.Status)
	originalRefreshToken := strings.TrimSpace(existing.RefreshToken)

	mergeStringField(&merged.Password, incoming.Password)
	mergeStringField(&merged.ClientID, incoming.ClientID)
	mergeStringField(&merged.SessionToken, incoming.SessionToken)
	mergeStringField(&merged.EmailService, incoming.EmailService)
	mergeStringField(&merged.EmailServiceID, incoming.EmailServiceID)
	mergeStringField(&merged.AccountID, incoming.AccountID)
	mergeStringField(&merged.WorkspaceID, incoming.WorkspaceID)
	mergeStringField(&merged.AccessToken, incoming.AccessToken)
	mergeStringField(&merged.RefreshToken, incoming.RefreshToken)
	mergeStringField(&merged.IDToken, incoming.IDToken)
	mergeStringField(&merged.Cookies, incoming.Cookies)
	mergeStringField(&merged.ProxyUsed, incoming.ProxyUsed)
	mergeTimeField(&merged.LastRefresh, incoming.LastRefresh)
	mergeTimeField(&merged.ExpiresAt, incoming.ExpiresAt)
	mergeStringField(&merged.Status, incoming.Status)
	mergeStringField(&merged.Source, incoming.Source)
	mergeStringField(&merged.SubscriptionType, incoming.SubscriptionType)
	mergeTimeField(&merged.SubscriptionAt, incoming.SubscriptionAt)
	mergeBoolField(&merged.CPAUploaded, incoming.CPAUploaded)
	mergeTimeField(&merged.CPAUploadedAt, incoming.CPAUploadedAt)
	mergeBoolField(&merged.Sub2APIUploaded, incoming.Sub2APIUploaded)
	mergeTimeField(&merged.Sub2APIUploadedAt, incoming.Sub2APIUploadedAt)

	extraData := cloneExtraData(existing.ExtraData)
	if len(incoming.ExtraData) > 0 {
		for key, value := range incoming.ExtraData {
			extraData[key] = value
		}
	}
	merged.ExtraData = extraData

	if shouldPreserveAccountStatus(originalStatus, originalRefreshToken, incoming.Status, incoming.RefreshToken) {
		merged.Status = originalStatus
		removeTemporaryAccountExtraData(merged.ExtraData)
	}

	if incoming.RegisteredAt != nil && shouldRefreshRegisteredAt(existing.Status, incoming.Status, existing.RegisteredAt) {
		merged.RegisteredAt = cloneTimePtr(incoming.RegisteredAt)
	}

	return merged
}

func mergeStringField(target *string, incoming string) {
	if incoming == "" {
		return
	}
	*target = incoming
}

func mergeBoolField(target *bool, incoming *bool) {
	if incoming == nil {
		return
	}
	*target = *incoming
}

func mergeTimeField(target **time.Time, incoming *time.Time) {
	if incoming == nil {
		return
	}
	*target = cloneTimePtr(incoming)
}

func shouldPreserveAccountStatus(originalStatus string, originalRefreshToken string, incomingStatus string, incomingRefreshToken string) bool {
	if strings.TrimSpace(originalRefreshToken) == "" || strings.TrimSpace(incomingRefreshToken) != "" {
		return false
	}
	if strings.TrimSpace(originalStatus) == "" {
		return false
	}

	switch strings.TrimSpace(incomingStatus) {
	case "token_pending", "login_incomplete":
		return true
	default:
		return false
	}
}

func shouldRefreshRegisteredAt(existingStatus string, incomingStatus string, existingRegisteredAt *time.Time) bool {
	if existingRegisteredAt == nil {
		return true
	}

	return strings.TrimSpace(existingStatus) == "failed" && strings.TrimSpace(incomingStatus) == "active"
}

func removeTemporaryAccountExtraData(extraData map[string]any) {
	if len(extraData) == 0 {
		return
	}

	delete(extraData, "token_pending")
	delete(extraData, "login_incomplete")
	delete(extraData, "account_status_reason")
}
