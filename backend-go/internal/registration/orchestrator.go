package registration

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const PythonFallbackStageExecute = "python_execute_registration"

var (
	errPreparationSettingsLookup = errors.New("registration preparation settings lookup failed")
	errEmailServiceLookup        = errors.New("registration email service lookup failed")
	errOutlookLookup             = errors.New("registration outlook lookup failed")
	errOutlookReservation        = errors.New("registration outlook reservation failed")
)

type PreparationDependencies struct {
	Settings      SettingsProvider
	EmailServices EmailServiceCatalog
	Outlook       OutlookPreparationReader
	Proxies       ProxySelector
	Reservations  OutlookReservationStore
}

type SettingsProvider interface {
	GetSettings(ctx context.Context, keys []string) (map[string]string, error)
}

type EmailServiceCatalog interface {
	ListEmailServices(ctx context.Context) ([]EmailServiceRecord, error)
}

type OutlookPreparationReader interface {
	ListOutlookServices(ctx context.Context) ([]EmailServiceRecord, error)
	ListAccountsByEmails(ctx context.Context, emails []string) ([]RegisteredAccountRecord, error)
}

type ProxySelector interface {
	SelectProxy(ctx context.Context, req StartRequest) (ProxySelection, error)
}

type OutlookReservationStore interface {
	ListClaimedOutlookServiceIDs(ctx context.Context, excludeTaskUUID string) ([]int, error)
	ReserveOutlookService(ctx context.Context, taskUUID string, serviceID int) error
}

type PreparationResult struct {
	Request StartRequest
	Plan    ExecutionPlan
}

type ExecutionPlan struct {
	Stage        string               `json:"stage"`
	Task         ExecutionTaskContext `json:"task"`
	EmailService PreparedEmailService `json:"email_service"`
	Proxy        ProxySelection       `json:"proxy"`
	Outlook      *PreparedOutlook     `json:"outlook,omitempty"`
}

type ExecutionTaskContext struct {
	TaskUUID string `json:"task_uuid"`
}

type PreparedEmailService struct {
	Prepared       bool           `json:"prepared"`
	Type           string         `json:"type"`
	ServiceID      *int           `json:"service_id,omitempty"`
	Config         map[string]any `json:"config,omitempty"`
	Source         string         `json:"source,omitempty"`
	FallbackReason string         `json:"fallback_reason,omitempty"`
}

type ProxySelection struct {
	Requested string `json:"requested,omitempty"`
	Selected  string `json:"selected,omitempty"`
	Source    string `json:"source,omitempty"`
	Note      string `json:"note,omitempty"`
}

type PreparedOutlook struct {
	ServiceID         int    `json:"service_id"`
	Email             string `json:"email,omitempty"`
	Name              string `json:"name,omitempty"`
	HasOAuth          bool   `json:"has_oauth"`
	RegistrationState string `json:"registration_state,omitempty"`
	ReservationStatus string `json:"reservation_status,omitempty"`
	ReservationReason string `json:"reservation_reason,omitempty"`
}

type requestPreparer interface {
	Prepare(ctx context.Context, taskUUID string, req StartRequest) (PreparationResult, error)
}

type orchestrator struct {
	deps PreparationDependencies
}

func newOrchestrator(deps PreparationDependencies) *orchestrator {
	if deps.Proxies == nil {
		deps.Proxies = noopProxySelector{}
	}
	return &orchestrator{deps: deps}
}

func (o *orchestrator) Prepare(ctx context.Context, taskUUID string, req StartRequest) (PreparationResult, error) {
	normalized := normalizeStartRequest(req)
	proxy, err := o.deps.Proxies.SelectProxy(ctx, normalized)
	if err != nil {
		return PreparationResult{}, fmt.Errorf("select proxy: %w", err)
	}

	result := PreparationResult{
		Request: normalized,
		Plan: ExecutionPlan{
			Stage: PythonFallbackStageExecute,
			Task: ExecutionTaskContext{
				TaskUUID: strings.TrimSpace(taskUUID),
			},
			Proxy: proxy,
			EmailService: PreparedEmailService{
				Type: canonicalNativeEmailServiceType(normalized.EmailServiceType),
			},
		},
	}

	if prepared, outlookPlan, err := o.prepareByServiceID(ctx, taskUUID, normalized, proxy); err != nil {
		return PreparationResult{}, err
	} else if prepared != nil {
		result.Plan.EmailService = *prepared
		result.Plan.Outlook = outlookPlan
		return result, nil
	}

	if len(normalized.EmailServiceConfig) > 0 {
		result.Plan.EmailService = PreparedEmailService{
			Prepared:  true,
			Type:      canonicalNativeEmailServiceType(normalized.EmailServiceType),
			ServiceID: cloneIntPointer(normalized.EmailServiceID),
			Config:    normalizeEmailServiceConfig(canonicalNativeEmailServiceType(normalized.EmailServiceType), normalized.EmailServiceConfig, proxy.Selected),
			Source:    "inline_config",
		}
		if normalized.EmailServiceType == "outlook" {
			result.Plan.Outlook = prepareInlineOutlook(result.Plan.EmailService, normalized)
		}
		return result, nil
	}

	switch normalized.EmailServiceType {
	case "tempmail":
		prepared, err := o.prepareTempmail(ctx, normalized, proxy)
		if err != nil {
			return PreparationResult{}, err
		}
		result.Plan.EmailService = prepared
	case "yyds_mail":
		prepared, err := o.prepareYYDSMail(ctx, normalized, proxy)
		if err != nil {
			return PreparationResult{}, err
		}
		result.Plan.EmailService = prepared
	case "duck_mail", "freemail", "temp_mail", "luckmail", "luck_mail", "imap_mail", "imap":
		prepared, err := o.prepareConfiguredEmailServiceByType(ctx, normalized, proxy)
		if err != nil {
			return PreparationResult{}, err
		}
		result.Plan.EmailService = prepared
	case "moe_mail":
		prepared, err := o.prepareMoeMail(ctx, normalized, proxy)
		if err != nil {
			return PreparationResult{}, err
		}
		result.Plan.EmailService = prepared
	case "outlook":
		prepared, outlookPlan, err := o.prepareOutlook(ctx, taskUUID, normalized, proxy)
		if err != nil {
			return PreparationResult{}, err
		}
		result.Plan.EmailService = prepared
		result.Plan.Outlook = outlookPlan
	default:
		result.Plan.EmailService = PreparedEmailService{
			Prepared:       false,
			Type:           canonicalNativeEmailServiceType(normalized.EmailServiceType),
			ServiceID:      cloneIntPointer(normalized.EmailServiceID),
			Source:         "python_fallback",
			FallbackReason: "go preparation does not yet resolve this email service",
		}
	}

	return result, nil
}

type noopProxySelector struct{}

func (noopProxySelector) SelectProxy(_ context.Context, req StartRequest) (ProxySelection, error) {
	requested := strings.TrimSpace(req.Proxy)
	if requested == "" {
		return ProxySelection{
			Source: "unassigned",
			Note:   "proxy selector not configured",
		}, nil
	}
	return ProxySelection{
		Requested: requested,
		Selected:  requested,
		Source:    "request",
		Note:      "proxy selector not configured; using request proxy directly",
	}, nil
}

func normalizeStartRequest(req StartRequest) StartRequest {
	normalized := req
	normalized.EmailServiceType = strings.ToLower(strings.TrimSpace(req.EmailServiceType))
	if normalized.EmailServiceType == "" {
		normalized.EmailServiceType = "tempmail"
	}
	normalized.Proxy = strings.TrimSpace(req.Proxy)
	normalized.EmailServiceConfig = cloneMap(req.EmailServiceConfig)
	normalized.CPAServiceIDs = append([]int(nil), req.CPAServiceIDs...)
	normalized.Sub2APIServiceIDs = append([]int(nil), req.Sub2APIServiceIDs...)
	normalized.TMServiceIDs = append([]int(nil), req.TMServiceIDs...)
	return normalized
}

func (o *orchestrator) prepareByServiceID(
	ctx context.Context,
	taskUUID string,
	req StartRequest,
	proxy ProxySelection,
) (*PreparedEmailService, *PreparedOutlook, error) {
	if req.EmailServiceID == nil {
		return nil, nil, nil
	}
	if o.deps.EmailServices == nil {
		return &PreparedEmailService{
			Prepared:       false,
			Type:           canonicalNativeEmailServiceType(req.EmailServiceType),
			ServiceID:      cloneIntPointer(req.EmailServiceID),
			Source:         "python_fallback",
			FallbackReason: "email service catalog not configured",
		}, nil, nil
	}

	services, err := o.deps.EmailServices.ListEmailServices(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", errEmailServiceLookup, err)
	}

	service, ok := findEmailServiceByID(services, *req.EmailServiceID)
	if !ok {
		return nil, nil, fmt.Errorf("email service not found or disabled: %d", *req.EmailServiceID)
	}

	resolvedType := strings.TrimSpace(service.ServiceType)
	if resolvedType == "" {
		resolvedType = req.EmailServiceType
	}
	resolvedType = canonicalNativeEmailServiceType(resolvedType)
	prepared := &PreparedEmailService{
		Prepared:  true,
		Type:      resolvedType,
		ServiceID: cloneIntPointer(req.EmailServiceID),
		Config:    normalizeEmailServiceConfig(resolvedType, service.Config, proxy.Selected),
		Source:    "email_service_id",
	}
	if resolvedType != "outlook" {
		return prepared, nil, nil
	}

	outlookPlan, err := o.buildOutlookPlan(ctx, taskUUID, service, proxy)
	if err != nil {
		return nil, nil, err
	}
	return prepared, outlookPlan, nil
}

func (o *orchestrator) prepareTempmail(ctx context.Context, req StartRequest, proxy ProxySelection) (PreparedEmailService, error) {
	if o.deps.Settings == nil {
		return PreparedEmailService{
			Prepared:       false,
			Type:           canonicalNativeEmailServiceType(req.EmailServiceType),
			Source:         "python_fallback",
			FallbackReason: "settings provider not configured",
		}, nil
	}

	settings, err := o.deps.Settings.GetSettings(ctx, []string{
		"tempmail.enabled",
		"tempmail.base_url",
		"tempmail.timeout",
		"tempmail.max_retries",
	})
	if err != nil {
		return PreparedEmailService{}, fmt.Errorf("%w: %v", errPreparationSettingsLookup, err)
	}
	if !parseBoolSetting(settings["tempmail.enabled"]) {
		return PreparedEmailService{}, errors.New("tempmail service is disabled")
	}

	config := map[string]any{
		"base_url": settings["tempmail.base_url"],
	}
	if timeout, ok := parseOptionalInt(settings["tempmail.timeout"]); ok {
		config["timeout"] = timeout
	}
	if retries, ok := parseOptionalInt(settings["tempmail.max_retries"]); ok {
		config["max_retries"] = retries
	}
	config = normalizeEmailServiceConfig("tempmail", config, proxy.Selected)

	return PreparedEmailService{
		Prepared: true,
		Type:     canonicalNativeEmailServiceType(req.EmailServiceType),
		Config:   config,
		Source:   "settings.tempmail",
	}, nil
}

func (o *orchestrator) prepareYYDSMail(ctx context.Context, req StartRequest, proxy ProxySelection) (PreparedEmailService, error) {
	if o.deps.Settings == nil {
		return PreparedEmailService{
			Prepared:       false,
			Type:           canonicalNativeEmailServiceType(req.EmailServiceType),
			Source:         "python_fallback",
			FallbackReason: "settings provider not configured",
		}, nil
	}

	settings, err := o.deps.Settings.GetSettings(ctx, []string{
		"yyds_mail.enabled",
		"yyds_mail.base_url",
		"yyds_mail.api_key",
		"yyds_mail.default_domain",
		"yyds_mail.timeout",
		"yyds_mail.max_retries",
	})
	if err != nil {
		return PreparedEmailService{}, fmt.Errorf("%w: %v", errPreparationSettingsLookup, err)
	}
	if !parseBoolSetting(settings["yyds_mail.enabled"]) {
		return PreparedEmailService{
			Prepared:       false,
			Type:           canonicalNativeEmailServiceType(req.EmailServiceType),
			Source:         "python_fallback",
			FallbackReason: "yyds_mail service is disabled",
		}, nil
	}
	if strings.TrimSpace(settings["yyds_mail.api_key"]) == "" {
		return PreparedEmailService{
			Prepared:       false,
			Type:           canonicalNativeEmailServiceType(req.EmailServiceType),
			Source:         "python_fallback",
			FallbackReason: "yyds_mail api_key is not configured",
		}, nil
	}

	config := map[string]any{
		"base_url":       settings["yyds_mail.base_url"],
		"api_key":        settings["yyds_mail.api_key"],
		"default_domain": settings["yyds_mail.default_domain"],
	}
	if timeout, ok := parseOptionalInt(settings["yyds_mail.timeout"]); ok {
		config["timeout"] = timeout
	}
	if retries, ok := parseOptionalInt(settings["yyds_mail.max_retries"]); ok {
		config["max_retries"] = retries
	}

	return PreparedEmailService{
		Prepared: true,
		Type:     canonicalNativeEmailServiceType(req.EmailServiceType),
		Config:   normalizeEmailServiceConfig(canonicalNativeEmailServiceType(req.EmailServiceType), config, proxy.Selected),
		Source:   "settings.yyds_mail",
	}, nil
}

func (o *orchestrator) prepareConfiguredEmailServiceByType(
	ctx context.Context,
	req StartRequest,
	proxy ProxySelection,
) (PreparedEmailService, error) {
	if o.deps.EmailServices == nil {
		return PreparedEmailService{
			Prepared:       false,
			Type:           canonicalNativeEmailServiceType(req.EmailServiceType),
			Source:         "python_fallback",
			FallbackReason: "email service catalog not configured",
		}, nil
	}

	services, err := o.deps.EmailServices.ListEmailServices(ctx)
	if err != nil {
		return PreparedEmailService{}, fmt.Errorf("%w: %v", errEmailServiceLookup, err)
	}

	service, ok := findEmailServiceByType(services, req.EmailServiceType)
	if !ok {
		return PreparedEmailService{
			Prepared:       false,
			Type:           canonicalNativeEmailServiceType(req.EmailServiceType),
			Source:         "python_fallback",
			FallbackReason: "no enabled email service config found for this type",
		}, nil
	}

	resolvedType := strings.TrimSpace(service.ServiceType)
	if resolvedType == "" {
		resolvedType = req.EmailServiceType
	}
	resolvedType = canonicalNativeEmailServiceType(resolvedType)

	return PreparedEmailService{
		Prepared:  true,
		Type:      resolvedType,
		ServiceID: intPointer(service.ID),
		Config:    normalizeEmailServiceConfig(resolvedType, service.Config, proxy.Selected),
		Source:    "email_service_type." + req.EmailServiceType,
	}, nil
}

func (o *orchestrator) prepareMoeMail(ctx context.Context, req StartRequest, proxy ProxySelection) (PreparedEmailService, error) {
	prepared, err := o.prepareConfiguredEmailServiceByType(ctx, req, proxy)
	if err != nil {
		return PreparedEmailService{}, err
	}
	if prepared.Prepared {
		return prepared, nil
	}

	if o.deps.Settings == nil {
		return prepared, nil
	}

	settings, err := o.deps.Settings.GetSettings(ctx, []string{
		"custom_domain.base_url",
		"custom_domain.api_key",
	})
	if err != nil {
		return PreparedEmailService{}, fmt.Errorf("%w: %v", errPreparationSettingsLookup, err)
	}
	if strings.TrimSpace(settings["custom_domain.base_url"]) == "" || strings.TrimSpace(settings["custom_domain.api_key"]) == "" {
		return prepared, nil
	}

	return PreparedEmailService{
		Prepared: true,
		Type:     canonicalNativeEmailServiceType(req.EmailServiceType),
		Config: normalizeEmailServiceConfig(canonicalNativeEmailServiceType(req.EmailServiceType), map[string]any{
			"base_url": settings["custom_domain.base_url"],
			"api_key":  settings["custom_domain.api_key"],
		}, proxy.Selected),
		Source: "settings.custom_domain",
	}, nil
}

func (o *orchestrator) prepareOutlook(
	ctx context.Context,
	taskUUID string,
	req StartRequest,
	proxy ProxySelection,
) (PreparedEmailService, *PreparedOutlook, error) {
	if o.deps.Outlook == nil {
		return PreparedEmailService{
				Prepared:       false,
				Type:           req.EmailServiceType,
				Source:         "python_fallback",
				FallbackReason: "outlook preparation repository not configured",
			}, &PreparedOutlook{
				ReservationStatus: "reservation_not_configured",
				ReservationReason: "outlook preparation repository not configured",
			}, nil
	}

	services, err := o.deps.Outlook.ListOutlookServices(ctx)
	if err != nil {
		return PreparedEmailService{}, nil, fmt.Errorf("%w: %v", errOutlookLookup, err)
	}
	claimed := make(map[int]struct{})
	if o.deps.Reservations != nil {
		claimedIDs, err := o.deps.Reservations.ListClaimedOutlookServiceIDs(ctx, taskUUID)
		if err != nil {
			return PreparedEmailService{}, nil, fmt.Errorf("%w: %v", errOutlookReservation, err)
		}
		for _, id := range claimedIDs {
			claimed[id] = struct{}{}
		}
	}

	service, state, err := o.selectOutlookService(ctx, services, claimed)
	if err != nil {
		return PreparedEmailService{}, nil, err
	}
	outlookPlan, err := o.buildOutlookPlan(ctx, taskUUID, service, proxy)
	if err != nil {
		return PreparedEmailService{}, nil, err
	}
	outlookPlan.RegistrationState = state

	prepared := PreparedEmailService{
		Prepared:  true,
		Type:      "outlook",
		ServiceID: intPointer(service.ID),
		Config:    normalizeEmailServiceConfig("outlook", service.Config, proxy.Selected),
		Source:    "outlook_selection",
	}

	return prepared, outlookPlan, nil
}

func (o *orchestrator) selectOutlookService(
	ctx context.Context,
	services []EmailServiceRecord,
	claimed map[int]struct{},
) (EmailServiceRecord, string, error) {
	emails := make([]string, 0, len(services))
	for _, service := range services {
		email := resolveOutlookServiceEmail(service)
		if email == "" {
			continue
		}
		emails = append(emails, email)
	}

	registered := make(map[string]RegisteredAccountRecord)
	if len(emails) > 0 {
		accountRows, err := o.deps.Outlook.ListAccountsByEmails(ctx, emails)
		if err != nil {
			return EmailServiceRecord{}, "", fmt.Errorf("%w: %v", errOutlookLookup, err)
		}
		for _, account := range accountRows {
			registered[strings.TrimSpace(account.Email)] = account
		}
	}

	for _, service := range services {
		if _, ok := claimed[service.ID]; ok {
			continue
		}
		email := resolveOutlookServiceEmail(service)
		if email == "" {
			continue
		}
		if _, ok := registered[email]; ok {
			continue
		}
		return service, "unregistered", nil
	}

	return EmailServiceRecord{}, "", errors.New("no available outlook account remains for registration")
}

func (o *orchestrator) buildOutlookPlan(
	ctx context.Context,
	taskUUID string,
	service EmailServiceRecord,
	proxy ProxySelection,
) (*PreparedOutlook, error) {
	email := resolveOutlookServiceEmail(service)
	outlookPlan := &PreparedOutlook{
		ServiceID:         service.ID,
		Email:             email,
		Name:              strings.TrimSpace(service.Name),
		HasOAuth:          hasOutlookOAuth(service.Config),
		RegistrationState: "unregistered",
		ReservationStatus: "reservation_not_configured",
		ReservationReason: "outlook reservation store not configured",
	}

	if o.deps.Outlook != nil && email != "" {
		accounts, err := o.deps.Outlook.ListAccountsByEmails(ctx, []string{email})
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errOutlookLookup, err)
		}
		if len(accounts) > 0 {
			if strings.TrimSpace(accounts[0].RefreshToken) != "" {
				outlookPlan.RegistrationState = "registered_complete"
			} else {
				outlookPlan.RegistrationState = "registered_needs_token_refresh"
			}
		}
	}

	if o.deps.Reservations != nil {
		if err := o.deps.Reservations.ReserveOutlookService(ctx, taskUUID, service.ID); err != nil {
			return nil, fmt.Errorf("%w: %v", errOutlookReservation, err)
		}
		outlookPlan.ReservationStatus = "reserved"
		outlookPlan.ReservationReason = ""
	}

	_ = proxy
	return outlookPlan, nil
}

func prepareInlineOutlook(emailPlan PreparedEmailService, req StartRequest) *PreparedOutlook {
	return &PreparedOutlook{
		ServiceID:         derefInt(req.EmailServiceID),
		Email:             stringConfig(emailPlan.Config, "email"),
		Name:              "",
		HasOAuth:          hasOutlookOAuth(emailPlan.Config),
		RegistrationState: "unknown",
		ReservationStatus: "inline_config",
	}
}

func normalizeEmailServiceConfig(serviceType string, config map[string]any, proxyURL string) map[string]any {
	normalized := cloneMap(config)

	if value, ok := normalized["api_url"]; ok {
		if _, exists := normalized["base_url"]; !exists {
			normalized["base_url"] = value
		}
		delete(normalized, "api_url")
	}

	switch serviceType {
	case "moe_mail", "yyds_mail", "duck_mail":
		if value, ok := normalized["domain"]; ok {
			if _, exists := normalized["default_domain"]; !exists {
				normalized["default_domain"] = value
			}
			delete(normalized, "domain")
		}
	case "tempmail", "temp_mail", "freemail":
		if value, ok := normalized["default_domain"]; ok {
			if _, exists := normalized["domain"]; !exists {
				normalized["domain"] = value
			}
			delete(normalized, "default_domain")
		}
	case "luckmail":
		if value, ok := normalized["domain"]; ok {
			if _, exists := normalized["preferred_domain"]; !exists {
				normalized["preferred_domain"] = value
			}
			delete(normalized, "domain")
		}
	}

	if proxyURL != "" {
		if _, exists := normalized["proxy_url"]; !exists {
			normalized["proxy_url"] = proxyURL
		}
	}

	return normalized
}

func findEmailServiceByID(services []EmailServiceRecord, id int) (EmailServiceRecord, bool) {
	for _, service := range services {
		if service.ID == id {
			return service, true
		}
	}
	return EmailServiceRecord{}, false
}

func findEmailServiceByType(services []EmailServiceRecord, serviceType string) (EmailServiceRecord, bool) {
	wantType := canonicalNativeEmailServiceType(serviceType)
	for _, service := range services {
		if canonicalNativeEmailServiceType(service.ServiceType) == wantType {
			return service, true
		}
	}
	return EmailServiceRecord{}, false
}

func canonicalNativeEmailServiceType(serviceType string) string {
	switch strings.ToLower(strings.TrimSpace(serviceType)) {
	case "temp_mail":
		return "tempmail"
	case "yydsmail":
		return "yyds_mail"
	case "duckmail":
		return "duck_mail"
	case "luck_mail":
		return "luckmail"
	case "imap":
		return "imap_mail"
	default:
		return strings.ToLower(strings.TrimSpace(serviceType))
	}
}

func parseOptionalInt(raw string) (int, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneIntPointer(src *int) *int {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func intPointer(value int) *int {
	return &value
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
