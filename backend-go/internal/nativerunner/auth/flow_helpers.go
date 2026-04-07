package auth

import (
	"encoding/base64"
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var interactiveContinuationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)"callback_url"\s*:\s*"([^"]+)"`),
	regexp.MustCompile(`(?i)"continue_url"\s*:\s*"([^"]+)"`),
	regexp.MustCompile(`(?i)(?:href|action)\s*=\s*["']([^"'<>]+)["']`),
	regexp.MustCompile(`(?i)location(?:\.href)?\s*=\s*["']([^"'<>]+)["']`),
}

var (
	interactiveFormPattern          = regexp.MustCompile(`(?is)<form\b([^>]*)>(.*?)</form>`)
	interactiveElementPattern       = regexp.MustCompile(`(?is)<([a-zA-Z][\w:-]*)\b([^>]*)>`)
	interactiveInputPattern         = regexp.MustCompile(`(?is)<input\b([^>]*)>`)
	interactiveButtonPattern        = regexp.MustCompile(`(?is)<button\b([^>]*)>(.*?)</button>`)
	interactiveHTMLAttributePattern = regexp.MustCompile("(?is)([a-zA-Z_:][\\w:.-]*)\\s*=\\s*(\"([^\"]*)\"|'([^']*)'|([^\\s\"'=<>`]+))")
)

type interactiveContinuationRequest struct {
	Method  string
	URL     string
	Form    url.Values
	Body    []byte
	Headers Headers
}

func inferPageTypeFromURL(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	path := raw
	if err == nil {
		path = parsed.Path
	}

	path = strings.ToLower(path)
	switch {
	case strings.Contains(path, "/create-account/password"):
		return "create_account_password"
	case strings.Contains(path, "/log-in/password"):
		return "login_password"
	case strings.Contains(path, "/email-verification"), strings.Contains(path, "/email-otp"):
		return "email_otp_verification"
	case strings.Contains(path, "/about-you"):
		return "about_you"
	case strings.Contains(path, "/add-phone"):
		return "add_phone"
	case strings.Contains(path, "/u/continue"):
		return "continue"
	case strings.Contains(path, "/api/auth/callback/openai"):
		return "callback"
	case strings.Contains(path, "/workspace"), strings.Contains(path, "/organization"), strings.Contains(path, "/consent"):
		return "workspace_selection"
	default:
		return ""
	}
}

func extractString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func extractObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func (c *Client) normalizeFlowURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	resolved, err := c.resolveURL(raw)
	if err != nil {
		return raw
	}
	return resolved.String()
}

func urlPath(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return parsed.Path, nil
}

func extractURLQueryValue(raw string, key string) string {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(key) == "" {
		return ""
	}

	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get(key))
}

func urlOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func resolveURLWithBase(baseRaw string, targetRaw string) string {
	baseRaw = strings.TrimSpace(baseRaw)
	targetRaw = strings.TrimSpace(targetRaw)
	if targetRaw == "" {
		return ""
	}

	targetURL, err := url.Parse(targetRaw)
	if err != nil {
		return ""
	}
	if targetURL.IsAbs() {
		return targetURL.String()
	}

	if baseRaw == "" {
		return targetRaw
	}

	baseURL, err := url.Parse(baseRaw)
	if err != nil {
		return targetRaw
	}

	return baseURL.ResolveReference(targetURL).String()
}

func (c *Client) flowRequestURL(referer string, path string) string {
	if resolved := resolveURLWithBase(referer, path); resolved != "" {
		return resolved
	}
	return c.normalizeFlowURL(path)
}

func (c *Client) flowOrigin(referer string) string {
	if origin := urlOrigin(referer); origin != "" {
		return origin
	}
	return c.origin()
}

func extractContinueURL(c *Client, payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}

	continueURL := c.normalizeFlowURL(extractString(payload["continue_url"]))
	if continueURL != "" {
		return continueURL
	}

	page := extractObject(payload["page"])
	for _, key := range []string{"url", "href", "external_url"} {
		if pageURL := c.normalizeFlowURL(extractString(page[key])); pageURL != "" {
			return pageURL
		}
	}

	return ""
}

func extractCallbackURL(c *Client, payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}

	callbackURL := c.normalizeFlowURL(extractString(payload["callback_url"]))
	if callbackURL != "" {
		return callbackURL
	}

	return callbackURLFromValue(extractContinueURL(c, payload))
}

func extractPayloadPageType(payload map[string]any, urls ...string) string {
	if len(payload) != 0 {
		for _, key := range []string{"page_type", "pageType", "type"} {
			if pageType := extractString(payload[key]); pageType != "" {
				return pageType
			}
		}

		page := extractObject(payload["page"])
		for _, key := range []string{"page_type", "pageType", "type", "name"} {
			if pageType := extractString(page[key]); pageType != "" {
				return pageType
			}
		}
	}

	for _, raw := range urls {
		if pageType := inferPageTypeFromURL(raw); pageType != "" {
			return pageType
		}
	}

	return ""
}

func (c *Client) extractContinueNavigationTarget(raw string, currentURL string) string {
	if c == nil || strings.TrimSpace(raw) == "" {
		return ""
	}

	normalized := html.UnescapeString(raw)
	normalized = strings.ReplaceAll(normalized, `\/`, `/`)
	normalized = strings.ReplaceAll(normalized, `\u002F`, `/`)
	currentURL = c.normalizeFlowURL(currentURL)

	for _, pattern := range interactiveContinuationPatterns {
		for _, match := range pattern.FindAllStringSubmatch(normalized, -1) {
			if len(match) < 2 {
				continue
			}
			candidate := c.normalizeFlowURL(match[1])
			if candidate == "" || candidate == currentURL {
				continue
			}
			pageType := inferPageTypeFromURL(candidate)
			switch pageType {
			case "create_account_password", "login_password", "email_otp_verification", "about_you", "add_phone":
				return candidate
			}
		}
	}

	return ""
}

func isInteractiveAuthPageType(pageType string) bool {
	switch strings.TrimSpace(pageType) {
	case "create_account_password", "email_otp_verification", "about_you", "add_phone", "login_password":
		return true
	default:
		return false
	}
}

func canAutoContinueAuthPageType(pageType string) bool {
	switch strings.TrimSpace(pageType) {
	case "add_phone":
		return true
	default:
		return false
	}
}

func (c *Client) extractInteractiveContinuation(pageType string, raw string, currentURL string) interactiveContinuationRequest {
	if c == nil || !canAutoContinueAuthPageType(pageType) || strings.TrimSpace(raw) == "" {
		return interactiveContinuationRequest{}
	}

	normalized := html.UnescapeString(raw)
	normalized = strings.ReplaceAll(normalized, `\/`, `/`)
	normalized = strings.ReplaceAll(normalized, `\u002F`, `/`)
	currentURL = c.normalizeFlowURL(currentURL)

	if continuation := c.extractInteractiveFormContinuation(normalized, currentURL); continuation.URL != "" {
		return continuation
	}

	if continuation := c.extractInteractiveDataActionContinuation(normalized, currentURL); continuation.URL != "" {
		return continuation
	}

	if continuation := c.extractInteractiveFetchContinuation(normalized, currentURL); continuation.URL != "" {
		return continuation
	}

	for _, pattern := range interactiveContinuationPatterns {
		for _, match := range pattern.FindAllStringSubmatch(normalized, -1) {
			if len(match) < 2 {
				continue
			}
			candidate := c.normalizeFlowURL(match[1])
			if candidate == "" || candidate == currentURL {
				continue
			}
			if isContinuableAuthURL(candidate) {
				return interactiveContinuationRequest{
					Method: "GET",
					URL:    candidate,
				}
			}
		}
	}

	return interactiveContinuationRequest{}
}

func (c *Client) extractInteractiveDataActionContinuation(raw string, currentURL string) interactiveContinuationRequest {
	if c == nil || strings.TrimSpace(raw) == "" {
		return interactiveContinuationRequest{}
	}

	var (
		best      interactiveContinuationRequest
		bestScore int
	)

	for _, match := range interactiveElementPattern.FindAllStringSubmatch(raw, -1) {
		if len(match) < 3 {
			continue
		}

		tagName := strings.ToLower(strings.TrimSpace(match[1]))
		attrs := extractInteractiveHTMLAttributes(match[2])
		action := c.normalizeFlowURL(firstNonEmpty(
			attrs["data-action"],
			attrs["data-endpoint"],
			attrs["data-url"],
			attrs["data-href"],
		))
		if action == "" || action == currentURL {
			continue
		}

		method := strings.ToUpper(strings.TrimSpace(firstNonEmpty(attrs["data-method"], attrs["method"])))
		payload := extractInteractiveDataPayload(attrs)
		if method == "" {
			if len(payload) != 0 {
				method = httpMethodPost
			} else {
				method = httpMethodGet
			}
		}
		if method != httpMethodGet && method != httpMethodPost {
			continue
		}
		if method == httpMethodGet && !isContinuableAuthURL(action) {
			continue
		}

		candidateText := strings.ToLower(strings.Join([]string{
			tagName,
			match[0],
			action,
		}, " "))
		score := 1
		if method == httpMethodPost {
			score += 4
		}
		if len(payload) != 0 {
			score += 2
		}
		if strings.Contains(candidateText, "skip") {
			score += 3
		}
		if strings.Contains(candidateText, "continue") {
			score++
		}
		if strings.Contains(candidateText, "verify") || strings.Contains(candidateText, "submit_phone") || strings.Contains(candidateText, "verify_phone") {
			score -= 2
		}
		if score > bestScore {
			bestScore = score
			best = interactiveContinuationRequest{
				Method: method,
				URL:    action,
				Form:   payload,
			}
		}
	}

	return best
}

func (c *Client) extractInteractiveFetchContinuation(raw string, currentURL string) interactiveContinuationRequest {
	if c == nil || strings.TrimSpace(raw) == "" {
		return interactiveContinuationRequest{}
	}

	var (
		best      interactiveContinuationRequest
		bestScore int
	)

	for _, match := range extractStaticFetchCalls(raw) {
		action := c.normalizeFlowURL(match.url)
		if action == "" || action == currentURL {
			continue
		}

		method := strings.ToUpper(strings.TrimSpace(match.method))
		if method == "" {
			if len(match.body) != 0 {
				method = httpMethodPost
			} else {
				method = httpMethodGet
			}
		}
		if method != httpMethodGet && method != httpMethodPost {
			continue
		}
		if method == httpMethodGet && !isContinuableAuthURL(action) {
			continue
		}

		candidateText := strings.ToLower(strings.Join([]string{
			match.raw,
			action,
			string(match.body),
		}, " "))
		score := 1
		if method == httpMethodPost {
			score += 4
		}
		if len(match.body) != 0 || len(match.form) != 0 {
			score += 2
		}
		if strings.Contains(candidateText, "skip") {
			score += 3
		}
		if strings.Contains(candidateText, "continue") {
			score++
		}
		if strings.Contains(candidateText, "verify") || strings.Contains(candidateText, "submit_phone") || strings.Contains(candidateText, "verify_phone") {
			score -= 2
		}
		if score > bestScore {
			bestScore = score
			best = interactiveContinuationRequest{
				Method:  method,
				URL:     action,
				Form:    match.form,
				Body:    append([]byte(nil), match.body...),
				Headers: cloneHeaders(match.headers),
			}
		}
	}

	return best
}

func (c *Client) extractInteractiveFormContinuation(raw string, currentURL string) interactiveContinuationRequest {
	if c == nil || strings.TrimSpace(raw) == "" {
		return interactiveContinuationRequest{}
	}

	var (
		best      interactiveContinuationRequest
		bestScore int
	)

	for _, match := range interactiveFormPattern.FindAllStringSubmatch(raw, -1) {
		if len(match) < 3 {
			continue
		}

		attrs := extractInteractiveHTMLAttributes(match[1])
		action := c.normalizeFlowURL(attrs["action"])
		if action == "" || action == currentURL {
			continue
		}

		method := strings.ToUpper(strings.TrimSpace(attrs["method"]))
		if method == "" {
			method = "GET"
		}
		if method != "GET" && method != "POST" {
			continue
		}
		if method == "GET" && !isContinuableAuthURL(action) {
			continue
		}

		formValues := extractInteractiveFormValues(match[2])
		score := 1
		if method == "POST" {
			score += 4
		}
		if len(formValues) != 0 {
			score += 2
		}
		lowerForm := strings.ToLower(match[0])
		if strings.Contains(lowerForm, "skip") {
			score += 3
		}
		if strings.Contains(lowerForm, "continue") {
			score++
		}
		if strings.Contains(lowerForm, "verify") || strings.Contains(lowerForm, "submit_phone") || strings.Contains(lowerForm, "verify_phone") {
			score -= 2
		}
		if score > bestScore {
			bestScore = score
			best = interactiveContinuationRequest{
				Method: method,
				URL:    action,
				Form:   formValues,
			}
		}
	}

	return best
}

func extractInteractiveHTMLAttributes(raw string) map[string]string {
	attrs := make(map[string]string)
	for _, match := range interactiveHTMLAttributePattern.FindAllStringSubmatch(raw, -1) {
		if len(match) < 6 {
			continue
		}

		value := firstNonEmpty(match[3], match[4], match[5])
		attrs[strings.ToLower(strings.TrimSpace(match[1]))] = html.UnescapeString(value)
	}
	return attrs
}

func extractInteractiveFormValues(raw string) url.Values {
	values := url.Values{}
	for _, match := range interactiveInputPattern.FindAllStringSubmatch(raw, -1) {
		if len(match) < 2 {
			continue
		}

		attrs := extractInteractiveHTMLAttributes(match[1])
		name := strings.TrimSpace(attrs["name"])
		if name == "" {
			continue
		}

		inputType := strings.ToLower(strings.TrimSpace(attrs["type"]))
		if inputType != "" && inputType != "hidden" {
			continue
		}
		values.Add(name, attrs["value"])
	}

	for _, match := range interactiveButtonPattern.FindAllStringSubmatch(raw, -1) {
		if len(match) < 2 {
			continue
		}

		attrs := extractInteractiveHTMLAttributes(match[1])
		name := strings.TrimSpace(attrs["name"])
		if name == "" {
			continue
		}
		values.Add(name, attrs["value"])
	}

	return values
}

const (
	httpMethodGet  = "GET"
	httpMethodPost = "POST"
)

func extractInteractiveDataPayload(attrs map[string]string) url.Values {
	values := url.Values{}
	if len(attrs) == 0 {
		return values
	}

	for _, key := range []string{"data-payload", "data-params", "data-body", "data-state"} {
		mergeInteractivePayloadValues(values, attrs[key])
	}

	return values
}

func mergeInteractivePayloadValues(values url.Values, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		for key, value := range payload {
			mergeInteractiveScalarValue(values, key, value)
		}
	}
}

func mergeInteractiveScalarValue(values url.Values, key string, value any) {
	key = strings.TrimSpace(key)
	if key == "" || value == nil {
		return
	}

	switch typed := value.(type) {
	case string:
		values.Add(key, typed)
	case bool:
		if typed {
			values.Add(key, "true")
		} else {
			values.Add(key, "false")
		}
	case float64:
		values.Add(key, strconv.FormatFloat(typed, 'f', -1, 64))
	}
}

type staticFetchCall struct {
	raw     string
	url     string
	method  string
	body    []byte
	form    url.Values
	headers Headers
}

func extractStaticFetchCalls(raw string) []staticFetchCall {
	var calls []staticFetchCall
	searchStart := 0
	for searchStart < len(raw) {
		fetchStart, openParen, ok := findFetchCallStart(raw, searchStart)
		if !ok {
			break
		}

		arguments, closeParen, ok := extractJSEnclosed(raw, openParen, '(', ')')
		if !ok {
			searchStart = openParen + 1
			continue
		}

		callRaw := raw[fetchStart : closeParen+1]
		call, ok := parseStaticFetchCall(callRaw, arguments)
		if ok {
			calls = append(calls, call)
		}
		searchStart = closeParen + 1
	}
	return calls
}

func findFetchCallStart(raw string, start int) (int, int, bool) {
	for index := start; index < len(raw); index++ {
		if !strings.HasPrefix(raw[index:], "fetch") {
			continue
		}
		if index > 0 {
			prev := raw[index-1]
			if isJSIdentifierRune(prev) {
				continue
			}
		}

		cursor := index + len("fetch")
		for cursor < len(raw) && (raw[cursor] == ' ' || raw[cursor] == '\n' || raw[cursor] == '\r' || raw[cursor] == '\t') {
			cursor++
		}
		if cursor >= len(raw) || raw[cursor] != '(' {
			continue
		}
		return index, cursor, true
	}
	return 0, 0, false
}

func isJSIdentifierRune(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '$'
}

func extractJSEnclosed(raw string, openIndex int, open byte, close byte) (string, int, bool) {
	if openIndex < 0 || openIndex >= len(raw) || raw[openIndex] != open {
		return "", 0, false
	}

	depth := 0
	inString := byte(0)
	escapeNext := false
	for index := openIndex; index < len(raw); index++ {
		ch := raw[index]
		if inString != 0 {
			if escapeNext {
				escapeNext = false
				continue
			}
			if ch == '\\' {
				escapeNext = true
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}

		switch ch {
		case '\'', '"':
			inString = ch
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return raw[openIndex+1 : index], index, true
			}
		}
	}

	return "", 0, false
}

func splitJSTopLevel(raw string, separator byte) []string {
	var (
		parts        []string
		start        int
		parenDepth   int
		braceDepth   int
		bracketDepth int
		inString     byte
		escapeNext   bool
	)

	for index := 0; index < len(raw); index++ {
		ch := raw[index]
		if inString != 0 {
			if escapeNext {
				escapeNext = false
				continue
			}
			if ch == '\\' {
				escapeNext = true
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}

		switch ch {
		case '\'', '"':
			inString = ch
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case separator:
			if parenDepth == 0 && braceDepth == 0 && bracketDepth == 0 {
				parts = append(parts, raw[start:index])
				start = index + 1
			}
		}
	}

	parts = append(parts, raw[start:])
	return parts
}

func parseStaticFetchCall(raw string, arguments string) (staticFetchCall, bool) {
	parts := splitJSTopLevel(arguments, ',')
	if len(parts) < 2 {
		return staticFetchCall{}, false
	}

	requestURL, ok := parseJSStringLiteral(parts[0])
	if !ok {
		return staticFetchCall{}, false
	}

	options, ok := parseJSObjectProperties(parts[1])
	if !ok {
		return staticFetchCall{}, false
	}

	method, _ := parseJSStringLiteral(options["method"])
	headers := parseJSHeaderObject(options["headers"])
	body, form, bodyHeaders, ok := parseStaticFetchBody(options["body"])
	if !ok {
		return staticFetchCall{}, false
	}
	if len(bodyHeaders) != 0 {
		if headers == nil {
			headers = Headers{}
		}
		for key, value := range bodyHeaders {
			if strings.TrimSpace(headers[key]) == "" {
				headers[key] = value
			}
		}
	}

	return staticFetchCall{
		raw:     raw,
		url:     requestURL,
		method:  method,
		body:    body,
		form:    form,
		headers: headers,
	}, true
}

func parseJSObjectProperties(raw string) (map[string]string, bool) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return nil, false
	}

	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" {
		return map[string]string{}, true
	}

	properties := map[string]string{}
	for _, entry := range splitJSTopLevel(inner, ',') {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		pair := splitJSTopLevel(entry, ':')
		if len(pair) < 2 {
			return nil, false
		}

		key, ok := parseJSObjectKey(pair[0])
		if !ok {
			return nil, false
		}
		properties[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(strings.Join(pair[1:], ":"))
	}

	return properties, true
}

func parseJSObjectKey(raw string) (string, bool) {
	if value, ok := parseJSStringLiteral(raw); ok {
		return value, true
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	for index := 0; index < len(trimmed); index++ {
		if !isJSIdentifierRune(trimmed[index]) {
			return "", false
		}
	}
	return trimmed, true
}

func parseJSHeaderObject(raw string) Headers {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	properties, ok := parseJSObjectProperties(raw)
	if !ok {
		return nil
	}

	headers := Headers{}
	for key, value := range properties {
		parsed, ok := parseJSStringLiteral(value)
		if !ok {
			return nil
		}
		headers[key] = parsed
	}
	return headers
}

func parseStaticFetchBody(raw string) ([]byte, url.Values, Headers, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil, nil, true
	}

	if strings.HasPrefix(trimmed, "JSON.stringify") {
		arguments, _, ok := extractJSEnclosed(trimmed, len("JSON.stringify"), '(', ')')
		if !ok {
			return nil, nil, nil, false
		}
		payload, ok := parseJSScalarObject(arguments)
		if !ok {
			return nil, nil, nil, false
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, nil, false
		}
		return body, nil, Headers{"Content-Type": "application/json"}, true
	}

	if strings.HasPrefix(trimmed, "new URLSearchParams") {
		arguments, _, ok := extractJSEnclosed(trimmed, len("new URLSearchParams"), '(', ')')
		if !ok {
			return nil, nil, nil, false
		}
		payload, ok := parseJSScalarObject(arguments)
		if !ok {
			return nil, nil, nil, false
		}
		values := url.Values{}
		for key, value := range payload {
			mergeInteractiveScalarValue(values, key, value)
		}
		return []byte(values.Encode()), values, Headers{"Content-Type": "application/x-www-form-urlencoded"}, true
	}

	if body, ok := parseJSStringLiteral(trimmed); ok {
		return []byte(body), nil, nil, true
	}

	return nil, nil, nil, false
}

func parseJSScalarObject(raw string) (map[string]any, bool) {
	properties, ok := parseJSObjectProperties(raw)
	if !ok {
		return nil, false
	}

	payload := make(map[string]any, len(properties))
	for key, value := range properties {
		parsed, ok := parseJSScalarLiteral(value)
		if !ok {
			return nil, false
		}
		payload[key] = parsed
	}
	return payload, true
}

func parseJSScalarLiteral(raw string) (any, bool) {
	if value, ok := parseJSStringLiteral(raw); ok {
		return value, true
	}

	switch strings.TrimSpace(raw) {
	case "true":
		return true, true
	case "false":
		return false, true
	case "null":
		return nil, true
	}

	number, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err == nil {
		return number, true
	}
	return nil, false
}

func parseJSStringLiteral(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) < 2 {
		return "", false
	}

	quote := trimmed[0]
	if (quote != '\'' && quote != '"') || trimmed[len(trimmed)-1] != quote {
		return "", false
	}

	var builder strings.Builder
	for index := 1; index < len(trimmed)-1; index++ {
		ch := trimmed[index]
		if ch != '\\' {
			builder.WriteByte(ch)
			continue
		}

		if index+1 >= len(trimmed)-1 {
			return "", false
		}
		index++
		switch trimmed[index] {
		case '\\', '\'', '"', '/':
			builder.WriteByte(trimmed[index])
		case 'b':
			builder.WriteByte('\b')
		case 'f':
			builder.WriteByte('\f')
		case 'n':
			builder.WriteByte('\n')
		case 'r':
			builder.WriteByte('\r')
		case 't':
			builder.WriteByte('\t')
		default:
			return "", false
		}
	}

	return builder.String(), true
}

func isContinuableAuthURL(raw string) bool {
	switch inferPageTypeFromURL(raw) {
	case "continue", "callback", "workspace_selection":
		return true
	default:
		return false
	}
}

func decodeJWTAuthClaims(token string) map[string]any {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return nil
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil
	}

	claims, _ := payload["https://api.openai.com/auth"].(map[string]any)
	return claims
}
