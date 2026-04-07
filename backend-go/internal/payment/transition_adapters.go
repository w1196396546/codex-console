package payment

import (
	"context"
	"fmt"
	"strings"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
)

type TransitionAdapters struct {
	CheckoutLinkGenerator   CheckoutLinkGenerator
	BillingProfileGenerator BillingProfileGenerator
	BrowserOpener           BrowserOpener
	SessionAdapter          SessionAdapter
	SubscriptionChecker     SubscriptionChecker
	AutoBinder              AutoBinder
}

func NewTransitionAdapters() TransitionAdapters {
	return TransitionAdapters{
		CheckoutLinkGenerator:   transitionCheckoutLinkGenerator{},
		BillingProfileGenerator: transitionBillingProfileGenerator{},
		BrowserOpener:           transitionBrowserOpener{},
		SessionAdapter:          transitionSessionAdapter{},
		SubscriptionChecker:     transitionSubscriptionChecker{},
		AutoBinder:              transitionAutoBinder{},
	}
}

type transitionCheckoutLinkGenerator struct{}

func (transitionCheckoutLinkGenerator) GenerateCheckoutLink(_ context.Context, account accounts.Account, req GenerateLinkRequest, _ string) (CheckoutLinkResult, error) {
	planType, err := normalizeTransitionPlanType(req.PlanType)
	if err != nil {
		return CheckoutLinkResult{}, err
	}
	sessionID := buildTransitionCheckoutSessionID(account.ID, planType, req.WorkspaceName)
	source := "openai_checkout"
	if planType == "business_trial" {
		source = "openai_checkout_business_trial"
	}
	return CheckoutLinkResult{
		Link:              fmt.Sprintf("https://chatgpt.com/checkout/openai_llc/%s", sessionID),
		Source:            source,
		CheckoutSessionID: sessionID,
	}, nil
}

type transitionBillingProfileGenerator struct{}

func (transitionBillingProfileGenerator) GenerateRandomBillingProfile(_ context.Context, country string, _ string) (map[string]any, error) {
	code := normalizeTransitionCountry(country)
	profile := transitionBillingProfiles[code]
	if len(profile) == 0 {
		profile = transitionBillingProfiles["US"]
	}
	result := make(map[string]any, len(profile)+2)
	for key, value := range profile {
		result[key] = value
	}
	result["country"] = code
	result["currency"] = transitionCountryCurrency[code]
	return result, nil
}

type transitionBrowserOpener struct{}

func (transitionBrowserOpener) OpenIncognito(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
}

type transitionSessionAdapter struct{}

func (transitionSessionAdapter) BootstrapSessionToken(_ context.Context, account accounts.Account, _ string) (SessionBootstrapResult, error) {
	token := resolveTransitionSessionToken(account)
	if token == "" {
		return SessionBootstrapResult{}, nil
	}
	cookies := account.Cookies
	cookies = upsertCookie(cookies, "__Secure-next-auth.session-token", token)
	deviceID := strings.TrimSpace(resolveDeviceID(account))
	if deviceID != "" {
		cookies = upsertCookie(cookies, "oai-did", deviceID)
	}
	return SessionBootstrapResult{
		SessionToken: token,
		AccessToken:  strings.TrimSpace(account.AccessToken),
		Cookies:      cookies,
	}, nil
}

func (transitionSessionAdapter) ProbeSession(_ context.Context, account accounts.Account, _ string) (*SessionProbeResult, error) {
	token := resolveTransitionSessionToken(account)
	accessToken := strings.TrimSpace(account.AccessToken)
	ok := token != "" || accessToken != "" || strings.TrimSpace(account.RefreshToken) != ""
	result := &SessionProbeResult{
		OK:                       ok,
		HTTPStatus:               200,
		SessionTokenFound:        token != "",
		AccessTokenInSessionJSON: accessToken != "",
		SessionTokenPreview:      maskSecret(token),
		AccessTokenPreview:       maskSecret(accessToken),
	}
	if !ok {
		result.OK = false
		result.HTTPStatus = 401
		result.Error = "transition probe: session context unavailable"
	}
	return result, nil
}

type transitionSubscriptionChecker struct{}

func (transitionSubscriptionChecker) CheckSubscription(_ context.Context, account accounts.Account, _ string, _ bool) (SubscriptionCheckDetail, error) {
	if hinted := resolveTransitionSubscriptionHint(account); hinted != nil {
		return *hinted, nil
	}
	if subscriptionType := normalizeSubscriptionType(account.SubscriptionType); isPaidSubscription(subscriptionType) {
		return SubscriptionCheckDetail{
			Status:     subscriptionType,
			Source:     "accounts_truth",
			Confidence: "high",
		}, nil
	}
	if strings.TrimSpace(account.AccessToken) != "" || resolveTransitionSessionToken(account) != "" {
		return SubscriptionCheckDetail{
			Status:     "free",
			Source:     "transition.account_tokens",
			Confidence: "low",
			Note:       "缺少上游订阅实检，保守视为 free",
		}, nil
	}
	return SubscriptionCheckDetail{
		Status:     "free",
		Source:     "transition.account_state",
		Confidence: "low",
		Note:       "缺少 access_token/session_token",
	}, nil
}

type transitionAutoBinder struct{}

func (transitionAutoBinder) AutoBindThirdParty(_ context.Context, task BindCardTask, account accounts.Account, req ThirdPartyAutoBindRequest) (AutoBindResult, error) {
	if strings.TrimSpace(task.CheckoutURL) == "" {
		return AutoBindResult{}, fmt.Errorf("任务缺少 checkout 链接")
	}
	subscriptionType := normalizeSubscriptionType(account.SubscriptionType)
	if subscriptionType == "" {
		subscriptionType = "free"
	}
	return AutoBindResult{
		Pending:          true,
		NeedUserAction:   true,
		SubscriptionType: subscriptionType,
		Detail: map[string]any{
			"mode":    "third_party",
			"message": "第三方自动绑卡已受理，等待支付最终状态或稍后同步订阅",
		},
		ThirdParty: map[string]any{
			"submitted":  true,
			"transition": true,
			"api_url":    strings.TrimSpace(req.APIURL),
			"assessment": map[string]any{
				"state":  "pending",
				"reason": "transition_adapter",
			},
		},
	}, nil
}

func (transitionAutoBinder) AutoBindLocal(_ context.Context, task BindCardTask, account accounts.Account, _ LocalAutoBindRequest) (AutoBindResult, error) {
	if strings.TrimSpace(task.CheckoutURL) == "" {
		return AutoBindResult{}, fmt.Errorf("任务缺少 checkout 链接")
	}
	subscriptionType := normalizeSubscriptionType(account.SubscriptionType)
	if subscriptionType == "" {
		subscriptionType = "free"
	}
	return AutoBindResult{
		Pending:          true,
		NeedUserAction:   true,
		SubscriptionType: subscriptionType,
		Detail: map[string]any{
			"mode":    "local_auto",
			"message": "本地自动绑卡需人工继续完成 challenge 或稍后同步订阅",
		},
		LocalAuto: map[string]any{
			"success":          true,
			"transition":       true,
			"pending":          true,
			"need_user_action": true,
			"stage":            "transition_pending",
		},
	}, nil
}

var transitionCountryCurrency = map[string]string{
	"US": "USD",
	"GB": "GBP",
	"CA": "CAD",
	"AU": "AUD",
	"SG": "SGD",
	"HK": "HKD",
	"JP": "JPY",
	"DE": "EUR",
	"FR": "EUR",
	"IT": "EUR",
	"ES": "EUR",
}

var transitionBillingProfiles = map[string]map[string]any{
	"US": {
		"first_name":  "Olivia",
		"last_name":   "Smith",
		"name":        "Olivia Smith",
		"line1":       "125 Market Street",
		"city":        "San Jose",
		"state":       "CA",
		"postal_code": "95112",
		"source":      "local_template",
	},
	"GB": {
		"first_name":  "Amelia",
		"last_name":   "Jones",
		"name":        "Amelia Jones",
		"line1":       "10 Bridge Street",
		"city":        "London",
		"state":       "London",
		"postal_code": "SW1A 1AA",
		"source":      "local_template",
	},
	"CA": {
		"first_name":  "Noah",
		"last_name":   "Brown",
		"name":        "Noah Brown",
		"line1":       "88 King Street W",
		"city":        "Toronto",
		"state":       "ON",
		"postal_code": "M5V 2T6",
		"source":      "local_template",
	},
	"AU": {
		"first_name":  "Charlotte",
		"last_name":   "Wilson",
		"name":        "Charlotte Wilson",
		"line1":       "101 George Street",
		"city":        "Sydney",
		"state":       "NSW",
		"postal_code": "2000",
		"source":      "local_template",
	},
	"SG": {
		"first_name":  "Lucas",
		"last_name":   "Tan",
		"name":        "Lucas Tan",
		"line1":       "8 Marina View",
		"city":        "Singapore",
		"state":       "SG",
		"postal_code": "018956",
		"source":      "local_template",
	},
	"HK": {
		"first_name":  "Chloe",
		"last_name":   "Chan",
		"name":        "Chloe Chan",
		"line1":       "1 Harbour Road",
		"city":        "Hong Kong",
		"state":       "HK",
		"postal_code": "000000",
		"source":      "local_template",
	},
	"JP": {
		"first_name":  "Haruto",
		"last_name":   "Sato",
		"name":        "Haruto Sato",
		"line1":       "1 Chiyoda",
		"city":        "Tokyo",
		"state":       "Tokyo",
		"postal_code": "100-0001",
		"source":      "local_template",
	},
	"DE": {
		"first_name":  "Emma",
		"last_name":   "Muller",
		"name":        "Emma Muller",
		"line1":       "12 Friedrichstrasse",
		"city":        "Berlin",
		"state":       "BE",
		"postal_code": "10115",
		"source":      "local_template",
	},
	"FR": {
		"first_name":  "Louis",
		"last_name":   "Martin",
		"name":        "Louis Martin",
		"line1":       "5 Rue de Rivoli",
		"city":        "Paris",
		"state":       "IDF",
		"postal_code": "75001",
		"source":      "local_template",
	},
	"IT": {
		"first_name":  "Sofia",
		"last_name":   "Rossi",
		"name":        "Sofia Rossi",
		"line1":       "18 Via del Corso",
		"city":        "Rome",
		"state":       "RM",
		"postal_code": "00118",
		"source":      "local_template",
	},
	"ES": {
		"first_name":  "Mateo",
		"last_name":   "Garcia",
		"name":        "Mateo Garcia",
		"line1":       "22 Calle de Alcala",
		"city":        "Madrid",
		"state":       "MD",
		"postal_code": "28001",
		"source":      "local_template",
	},
}

func normalizeTransitionPlanType(value string) (string, error) {
	planType := strings.ToLower(strings.TrimSpace(value))
	switch planType {
	case "plus", "team", "business_trial":
		return planType, nil
	default:
		return "", fmt.Errorf("plan_type 必须为 plus / team / business_trial")
	}
}

func buildTransitionCheckoutSessionID(accountID int, planType string, workspaceName string) string {
	suffix := normalizeTransitionWorkspaceSlug(workspaceName)
	if suffix == "" {
		return fmt.Sprintf("cs_transition_%s_%d", planType, accountID)
	}
	return fmt.Sprintf("cs_transition_%s_%d_%s", planType, accountID, suffix)
}

func normalizeTransitionWorkspaceSlug(workspaceName string) string {
	normalized := strings.ToLower(strings.TrimSpace(workspaceName))
	if normalized == "" {
		return ""
	}
	parts := make([]string, 0, len(normalized))
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		parts = append(parts, current.String())
		current.Reset()
	}
	appendToken := func(token string) {
		flush()
		if token != "" {
			parts = append(parts, token)
		}
	}

	for _, r := range normalized {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			current.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			flush()
		case r == '/':
			appendToken("slash")
		case r == '?':
			appendToken("q")
		case r == '#':
			appendToken("hash")
		case r == '%':
			appendToken("pct")
		case r <= 0x7f:
			appendToken(fmt.Sprintf("x%02x", r))
		default:
			appendToken(fmt.Sprintf("u%x", r))
		}
	}
	flush()
	return strings.Join(parts, "_")
}

func normalizeTransitionCountry(country string) string {
	code := strings.ToUpper(strings.TrimSpace(country))
	if _, ok := transitionCountryCurrency[code]; ok {
		return code
	}
	return "US"
}

func canTransitionBootstrapSession(account accounts.Account) bool {
	return strings.TrimSpace(account.AccessToken) != "" ||
		strings.TrimSpace(account.RefreshToken) != "" ||
		(strings.TrimSpace(account.Password) != "" && strings.TrimSpace(account.EmailService) != "")
}

func resolveTransitionSessionToken(account accounts.Account) string {
	return firstNonEmpty(strings.TrimSpace(account.SessionToken), extractSessionTokenFromCookieText(account.Cookies))
}

func resolveTransitionSubscriptionHint(account accounts.Account) *SubscriptionCheckDetail {
	if account.ExtraData == nil {
		return nil
	}
	status := normalizeSubscriptionType(stringValue(account.ExtraData["payment_subscription_status"]))
	confidence := strings.ToLower(strings.TrimSpace(stringValue(account.ExtraData["payment_subscription_confidence"])))
	if status == "" {
		return nil
	}
	if confidence == "" {
		confidence = "low"
	}
	return &SubscriptionCheckDetail{
		Status:         status,
		Source:         firstNonEmpty(strings.TrimSpace(stringValue(account.ExtraData["payment_subscription_source"])), "transition.extra_data"),
		Confidence:     confidence,
		Note:           strings.TrimSpace(stringValue(account.ExtraData["payment_subscription_note"])),
		RefreshedToken: boolValue(account.ExtraData["payment_subscription_refreshed"]),
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", value)
	}
}

func boolValue(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}
