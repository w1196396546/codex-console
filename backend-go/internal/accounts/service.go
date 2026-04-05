package accounts

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var sessionTokenChunkPattern = regexp.MustCompile(`(?:^|;\s*)__Secure-next-auth\.session-token\.(\d+)=([^;]+)`)

type Repository interface {
	ListAccounts(ctx context.Context, req ListAccountsRequest) ([]Account, int, error)
	ListAccountsForOverview(ctx context.Context, req AccountOverviewCardsRequest) ([]Account, error)
	ListAccountsForSelectable(ctx context.Context, req AccountOverviewSelectableRequest) ([]Account, error)
	GetAccountByID(ctx context.Context, accountID int) (Account, error)
	GetCurrentAccountID(ctx context.Context) (*int, error)
	GetAccountsStatsSummary(ctx context.Context) (AccountsStatsSummary, error)
	GetAccountsOverviewStats(ctx context.Context) (AccountsOverviewStats, error)
	GetAccountByEmail(ctx context.Context, email string) (Account, bool, error)
	UpsertAccount(ctx context.Context, account Account) (Account, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
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

func buildTeamRelationCompatibility(account Account) ([]string, map[string]any, int) {
	if normalizeSubscriptionType(account.SubscriptionType) != "team" {
		return []string{}, nil, 0
	}
	return []string{"owner"}, map[string]any{
		"owner_count":      1,
		"member_count":     0,
		"has_owner_role":   true,
		"has_member_role":  false,
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
	return strings.TrimSpace(fmt.Sprintf("%v", data[key]))
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
