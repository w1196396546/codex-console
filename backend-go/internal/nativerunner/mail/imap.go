package mail

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	stdmail "net/mail"
	"net/textproto"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultIMAPPollInterval = 3 * time.Second
const defaultIMAPDialTimeout = 30 * time.Second
const defaultIMAPOTPTimeSkew = 5 * time.Second
const defaultIMAPOTPStageResetThreshold = 3 * time.Second

var defaultIMAPSemanticCodePattern = regexp.MustCompile(`(?i)(?:verification(?:\s+code)?|security\s+code|one[\s-]*time\s+code|otp|login|log\s+in|code|验证码)[^0-9]{0,32}(\d{6})`)
var imapLiteralPattern = regexp.MustCompile(`\{(\d+)\}$`)
var imapFetchUIDPattern = regexp.MustCompile(`(?i)\bUID\s+(\d+)\b`)
var imapHTMLTagPattern = regexp.MustCompile(`(?s)<[^>]+>`)
var imapWhitespacePattern = regexp.MustCompile(`\s+`)
var imapPunctuationSpacingPattern = regexp.MustCompile(`\s+([.,!?;:])`)

type IMAPFetcher func(ctx context.Context, inbox Inbox) ([]IMAPMessage, error)

type IMAPExtractor func(messages []IMAPMessage, inbox Inbox, pattern *regexp.Regexp) (string, bool)

type IMAPConfig struct {
	Host         string
	Port         int
	Email        string
	Username     string
	Password     string
	UseSSL       bool
	DialTimeout  time.Duration
	PollInterval time.Duration
	Fetcher      IMAPFetcher
	Extractor    IMAPExtractor
}

type IMAPMessage struct {
	ID         string
	From       string
	Subject    string
	Body       string
	ReceivedAt time.Time
}

type IMAPMail struct {
	host         string
	port         int
	email        string
	username     string
	password     string
	useSSL       bool
	dialTimeout  time.Duration
	pollInterval time.Duration
	fetcher      IMAPFetcher
	extractor    IMAPExtractor
	stateMu      sync.Mutex
	codeStates   map[string]*imapCodeState
}

type imapCodeState struct {
	fingerprints  map[string]struct{}
	fallbackCodes map[string]struct{}
	stageMarker   int64
}

func NewIMAPMail(config IMAPConfig) *IMAPMail {
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultIMAPPollInterval
	}

	provider := &IMAPMail{
		host:         strings.TrimSpace(config.Host),
		port:         config.Port,
		email:        strings.TrimSpace(config.Email),
		username:     strings.TrimSpace(config.Username),
		password:     config.Password,
		useSSL:       config.UseSSL,
		dialTimeout:  config.DialTimeout,
		pollInterval: pollInterval,
		extractor:    config.Extractor,
		codeStates:   make(map[string]*imapCodeState),
	}
	if provider.username == "" {
		provider.username = provider.email
	}
	if provider.extractor == nil {
		provider.extractor = provider.defaultCodeExtractor()
	}
	if config.Fetcher != nil {
		provider.fetcher = config.Fetcher
	} else {
		provider.fetcher = provider.fetchMessages
	}

	return provider
}

func (i *IMAPMail) Create(ctx context.Context) (Inbox, error) {
	_ = ctx

	if i == nil {
		return Inbox{}, errors.New("imap mail provider is required")
	}
	if i.email == "" {
		return Inbox{}, errors.New("imap mail email is required")
	}

	return Inbox{Email: i.email}, nil
}

func (i *IMAPMail) WaitCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, error) {
	if i == nil {
		return "", errors.New("imap mail provider is required")
	}
	if i.fetcher == nil {
		return "", errors.New("imap mail fetcher is required")
	}
	if i.extractor == nil {
		return "", errors.New("imap mail extractor is required")
	}
	if pattern == nil {
		pattern = DefaultCodePattern
	}

	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()

	for {
		messages, err := i.fetcher(ctx, inbox)
		if err != nil {
			return "", err
		}
		i.prepareCodeState(i.emailStateKey(inbox), inbox.OTPSentAt)
		if code, found := i.extractor(messages, inbox, pattern); found {
			return code, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func defaultIMAPCodeExtractor() IMAPExtractor {
	return func(messages []IMAPMessage, _ Inbox, pattern *regexp.Regexp) (string, bool) {
		if pattern == nil {
			pattern = DefaultCodePattern
		}

		for _, message := range messages {
			if !isOpenAIVerificationMessage(message) {
				continue
			}

			if code, ok := extractPatternCode(message.Subject, pattern); ok {
				return code, true
			}
			if code, ok := extractSemanticCode(message.Body); ok {
				return code, true
			}
			if code, ok := extractPatternCode(message.Body, pattern); ok {
				return code, true
			}
		}

		return "", false
	}
}

func (i *IMAPMail) defaultCodeExtractor() IMAPExtractor {
	return func(messages []IMAPMessage, inbox Inbox, pattern *regexp.Regexp) (string, bool) {
		if pattern == nil {
			pattern = DefaultCodePattern
		}

		minReceivedAt := time.Time{}
		if !inbox.OTPSentAt.IsZero() {
			minReceivedAt = inbox.OTPSentAt.Add(-defaultIMAPOTPTimeSkew)
		}

		emailKey := i.emailStateKey(inbox)
		for _, message := range messages {
			if !message.ReceivedAt.IsZero() && !minReceivedAt.IsZero() && message.ReceivedAt.Before(minReceivedAt) {
				continue
			}
			if !isOpenAIVerificationMessage(message) {
				continue
			}

			var code string
			var ok bool
			if code, ok = extractPatternCode(message.Subject, pattern); !ok {
				if code, ok = extractSemanticCode(message.Body); !ok {
					code, ok = extractPatternCode(message.Body, pattern)
				}
			}
			if !ok {
				continue
			}

			if i.hasSeenCode(emailKey, message, code) {
				continue
			}
			i.markCodeSeen(emailKey, message, code)
			return code, true
		}

		return "", false
	}
}

func (i *IMAPMail) fetchMessages(ctx context.Context, inbox Inbox) ([]IMAPMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	address, username, password, err := i.connectionSettings(inbox)
	if err != nil {
		return nil, err
	}

	client, err := newIMAPClient(ctx, address, strings.TrimSpace(i.host), i.useSSL, i.effectiveDialTimeout())
	if err != nil {
		return nil, err
	}
	defer client.close()

	if err := client.login(username, password); err != nil {
		return nil, err
	}
	defer client.logout()

	if err := client.selectInbox(); err != nil {
		return nil, err
	}

	uids, err := client.searchAllUIDs()
	if err != nil {
		return nil, err
	}
	if len(uids) == 0 {
		return nil, nil
	}

	messages, err := client.fetchMessagesByUIDs(uids)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(messages, func(left, right int) bool {
		if messages[left].ReceivedAt.Equal(messages[right].ReceivedAt) {
			return messages[left].ID > messages[right].ID
		}
		return messages[left].ReceivedAt.After(messages[right].ReceivedAt)
	})

	return messages, nil
}

func (i *IMAPMail) connectionSettings(inbox Inbox) (address string, username string, password string, err error) {
	if i.host == "" {
		return "", "", "", errors.New("imap mail host is required")
	}

	port := i.port
	if port <= 0 {
		if i.useSSL {
			port = 993
		} else {
			port = 143
		}
	}

	email := strings.TrimSpace(i.email)
	if email == "" {
		email = strings.TrimSpace(inbox.Email)
	}

	username = strings.TrimSpace(i.username)
	if username == "" {
		username = email
	}
	if username == "" {
		return "", "", "", errors.New("imap mail username is required")
	}

	password = i.password
	if strings.TrimSpace(password) == "" {
		password = inbox.Token
	}
	if strings.TrimSpace(password) == "" {
		return "", "", "", errors.New("imap mail password is required")
	}

	return net.JoinHostPort(strings.TrimSpace(i.host), strconv.Itoa(port)), username, password, nil
}

func (i *IMAPMail) effectiveDialTimeout() time.Duration {
	if i.dialTimeout > 0 {
		return i.dialTimeout
	}
	return defaultIMAPDialTimeout
}

type imapClient struct {
	conn    net.Conn
	reader  *bufio.Reader
	writer  *bufio.Writer
	nextTag int
}

func newIMAPClient(ctx context.Context, address string, serverName string, useSSL bool, timeout time.Duration) (*imapClient, error) {
	dialer := &net.Dialer{Timeout: timeout}

	var (
		conn net.Conn
		err  error
	)
	if useSSL {
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config: &tls.Config{
				MinVersion: tls.VersionTLS12,
				ServerName: serverName,
			},
		}
		conn, err = tlsDialer.DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return nil, fmt.Errorf("connect imap server %s: %w", address, err)
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}

	client := &imapClient{
		conn:    conn,
		reader:  bufio.NewReader(conn),
		writer:  bufio.NewWriter(conn),
		nextTag: 1,
	}

	line, err := client.readLine()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read imap greeting: %w", err)
	}
	if !strings.HasPrefix(line, "* OK") && !strings.HasPrefix(line, "* PREAUTH") {
		_ = conn.Close()
		return nil, fmt.Errorf("unexpected imap greeting: %s", line)
	}

	return client, nil
}

func (c *imapClient) close() {
	if c == nil || c.conn == nil {
		return
	}
	_ = c.conn.Close()
}

func (c *imapClient) login(username string, password string) error {
	_, err := c.runCommand("LOGIN " + formatIMAPAtom(username) + " " + formatIMAPAtom(password))
	if err != nil {
		return fmt.Errorf("login imap server: %w", err)
	}
	return nil
}

func (c *imapClient) selectInbox() error {
	_, err := c.runCommand("SELECT INBOX")
	if err != nil {
		return fmt.Errorf("select inbox: %w", err)
	}
	return nil
}

func (c *imapClient) searchAllUIDs() ([]uint32, error) {
	lines, err := c.runCommand("UID SEARCH ALL")
	if err != nil {
		return nil, fmt.Errorf("search inbox messages: %w", err)
	}

	for _, line := range lines {
		if !strings.HasPrefix(line, "* SEARCH") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) <= 2 {
			return nil, nil
		}

		uids := make([]uint32, 0, len(fields)-2)
		for _, field := range fields[2:] {
			value, convErr := strconv.ParseUint(field, 10, 32)
			if convErr != nil {
				return nil, fmt.Errorf("parse imap uid %q: %w", field, convErr)
			}
			uids = append(uids, uint32(value))
		}
		return uids, nil
	}

	return nil, nil
}

func (c *imapClient) fetchMessagesByUIDs(uids []uint32) ([]IMAPMessage, error) {
	if len(uids) == 0 {
		return nil, nil
	}

	parts := make([]string, 0, len(uids))
	for _, uid := range uids {
		parts = append(parts, strconv.FormatUint(uint64(uid), 10))
	}

	lines, err := c.runCommand("UID FETCH " + strings.Join(parts, ",") + " (UID RFC822)")
	if err != nil {
		return nil, fmt.Errorf("fetch imap messages: %w", err)
	}

	messages := make([]IMAPMessage, 0, len(uids))
	for idx := 0; idx < len(lines); idx++ {
		line := lines[idx]
		if !strings.Contains(line, " FETCH ") {
			continue
		}

		uid, ok := parseFetchedUID(line)
		if !ok || idx+1 >= len(lines) {
			continue
		}

		message, parseErr := parseFetchedMessage(uid, lines[idx+1])
		if parseErr != nil {
			return nil, parseErr
		}
		messages = append(messages, message)
		idx++
	}

	return messages, nil
}

func (c *imapClient) logout() {
	if c == nil {
		return
	}
	_, _ = c.runCommand("LOGOUT")
}

func (c *imapClient) runCommand(command string) ([]string, error) {
	tag := fmt.Sprintf("A%04d", c.nextTag)
	c.nextTag++

	if _, err := fmt.Fprintf(c.writer, "%s %s\r\n", tag, command); err != nil {
		return nil, err
	}
	if err := c.writer.Flush(); err != nil {
		return nil, err
	}

	var lines []string
	for {
		line, err := c.readLine()
		if err != nil {
			return nil, err
		}

		if strings.HasPrefix(line, tag+" ") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return nil, fmt.Errorf("unexpected imap tagged response: %s", line)
			}
			if strings.EqualFold(fields[1], "OK") {
				return lines, nil
			}
			return nil, fmt.Errorf("imap command %q failed: %s", command, line)
		}

		lines = append(lines, line)

		size, ok := parseIMAPLiteralSize(line)
		if !ok {
			continue
		}

		raw := make([]byte, size)
		if _, err := io.ReadFull(c.reader, raw); err != nil {
			return nil, err
		}
		lines = append(lines, string(raw))

		if prefix, err := c.reader.Peek(2); err == nil && string(prefix) == "\r\n" {
			_, _ = c.reader.Discard(2)
		}
	}
}

func (c *imapClient) readLine() (string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func parseIMAPLiteralSize(line string) (int, bool) {
	match := imapLiteralPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 2 {
		return 0, false
	}

	size, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return size, true
}

func parseFetchedUID(line string) (string, bool) {
	match := imapFetchUIDPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func parseFetchedMessage(uid string, raw string) (IMAPMessage, error) {
	result := IMAPMessage{
		ID:   uid,
		Body: strings.TrimSpace(raw),
	}

	message, err := stdmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return result, nil
	}

	result.From = decodeMIMEHeader(message.Header.Get("From"))
	result.Subject = decodeMIMEHeader(message.Header.Get("Subject"))
	if messageID := strings.TrimSpace(message.Header.Get("Message-ID")); messageID != "" {
		result.ID = messageID
	}
	if receivedAt, err := message.Header.Date(); err == nil {
		result.ReceivedAt = receivedAt
	}

	body, err := extractMessageBody(message.Header, message.Body)
	if err != nil {
		return IMAPMessage{}, fmt.Errorf("read fetched imap message %s body: %w", uid, err)
	}
	if body != "" {
		result.Body = body
	}
	return result, nil
}

func formatIMAPAtom(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " (){%*\"\\") {
		replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
		return `"` + replacer.Replace(value) + `"`
	}
	return value
}

func isOpenAIVerificationMessage(message IMAPMessage) bool {
	sender := strings.ToLower(strings.TrimSpace(message.From))
	subject := strings.ToLower(strings.TrimSpace(message.Subject))
	body := strings.ToLower(strings.TrimSpace(message.Body))
	blob := sender + "\n" + subject + "\n" + body

	if !strings.Contains(blob, "openai") {
		return false
	}

	for _, keyword := range []string{
		"verification",
		"verification code",
		"verify",
		"one-time code",
		"one time code",
		"otp",
		"log in",
		"login",
		"security code",
		"验证码",
	} {
		if strings.Contains(blob, keyword) {
			return true
		}
	}

	return false
}

func extractSemanticCode(content string) (string, bool) {
	match := defaultIMAPSemanticCodePattern.FindStringSubmatch(content)
	if len(match) >= 2 {
		return match[1], true
	}
	return "", false
}

func extractPatternCode(content string, pattern *regexp.Regexp) (string, bool) {
	match := pattern.FindStringSubmatch(content)
	if len(match) >= 2 {
		return match[1], true
	}
	if len(match) == 1 {
		return match[0], true
	}
	return "", false
}

func (i *IMAPMail) emailStateKey(inbox Inbox) string {
	email := strings.TrimSpace(inbox.Email)
	if email == "" {
		email = i.email
	}
	return strings.ToLower(strings.TrimSpace(email))
}

func (i *IMAPMail) prepareCodeState(emailKey string, sentAt time.Time) {
	if emailKey == "" {
		return
	}

	i.stateMu.Lock()
	defer i.stateMu.Unlock()

	state := i.ensureCodeStateLocked(emailKey)
	if sentAt.IsZero() {
		return
	}

	stageMarker := sentAt.Unix()
	if state.stageMarker == 0 || absInt64(stageMarker-state.stageMarker) > int64(defaultIMAPOTPStageResetThreshold/time.Second) {
		clear(state.fingerprints)
		clear(state.fallbackCodes)
		state.stageMarker = stageMarker
	}
}

func (i *IMAPMail) hasSeenCode(emailKey string, message IMAPMessage, code string) bool {
	if emailKey == "" {
		return false
	}

	i.stateMu.Lock()
	defer i.stateMu.Unlock()

	state := i.ensureCodeStateLocked(emailKey)
	fingerprint, useFallback := codeFingerprint(message, code)
	if _, ok := state.fingerprints[fingerprint]; ok {
		return true
	}
	if useFallback {
		_, ok := state.fallbackCodes[code]
		return ok
	}
	return false
}

func (i *IMAPMail) markCodeSeen(emailKey string, message IMAPMessage, code string) {
	if emailKey == "" {
		return
	}

	i.stateMu.Lock()
	defer i.stateMu.Unlock()

	state := i.ensureCodeStateLocked(emailKey)
	fingerprint, useFallback := codeFingerprint(message, code)
	state.fingerprints[fingerprint] = struct{}{}
	if useFallback {
		state.fallbackCodes[code] = struct{}{}
	}
}

func (i *IMAPMail) ensureCodeStateLocked(emailKey string) *imapCodeState {
	state := i.codeStates[emailKey]
	if state == nil {
		state = &imapCodeState{
			fingerprints:  make(map[string]struct{}),
			fallbackCodes: make(map[string]struct{}),
		}
		i.codeStates[emailKey] = state
	}
	return state
}

func codeFingerprint(message IMAPMessage, code string) (string, bool) {
	mailID := strings.TrimSpace(message.ID)
	if mailID == "" {
		mailID = "-"
	}
	mailTS := int64(0)
	if !message.ReceivedAt.IsZero() {
		mailTS = message.ReceivedAt.Unix()
	}
	return fmt.Sprintf("%d|%s|%s", mailTS, mailID, code), mailID == "-" && mailTS <= 0
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func decodeMIMEHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	decoded, err := (&mime.WordDecoder{}).DecodeHeader(value)
	if err != nil {
		return value
	}
	return strings.TrimSpace(decoded)
}

func extractMessageBody(header stdmail.Header, body io.Reader) (string, error) {
	contentType := strings.TrimSpace(header.Get("Content-Type"))
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType == "" {
		payload, readErr := io.ReadAll(body)
		if readErr != nil {
			return "", readErr
		}
		return normalizeExtractedBody(string(payload), contentType), nil
	}

	if strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		return extractMultipartBody(body, params["boundary"])
	}

	payload, err := decodeTransferBody(header, body)
	if err != nil {
		return "", err
	}
	return normalizeExtractedBody(string(payload), mediaType), nil
}

func extractMultipartBody(body io.Reader, boundary string) (string, error) {
	if strings.TrimSpace(boundary) == "" {
		payload, err := io.ReadAll(body)
		if err != nil {
			return "", err
		}
		return normalizeExtractedBody(string(payload), ""), nil
	}

	reader := multipart.NewReader(body, boundary)
	var texts []string
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}

		text, err := extractPartBody(part.Header, part)
		part.Close()
		if err != nil {
			return "", err
		}
		if text != "" {
			texts = append(texts, text)
		}
	}

	return strings.TrimSpace(strings.Join(texts, " ")), nil
}

func extractPartBody(header textproto.MIMEHeader, body io.Reader) (string, error) {
	mediaType, _, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		mediaType = strings.TrimSpace(header.Get("Content-Type"))
	}

	switch strings.ToLower(mediaType) {
	case "text/plain", "text/html":
	default:
		return "", nil
	}

	payload, err := decodeTransferBody(stdmail.Header(header), body)
	if err != nil {
		return "", err
	}
	return normalizeExtractedBody(string(payload), mediaType), nil
}

func decodeTransferBody(header stdmail.Header, body io.Reader) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "base64":
		decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, body))
		if err != nil {
			return nil, err
		}
		return decoded, nil
	case "quoted-printable":
		decoded, err := io.ReadAll(quotedprintable.NewReader(body))
		if err != nil {
			return nil, err
		}
		return decoded, nil
	default:
		return io.ReadAll(body)
	}
}

func normalizeExtractedBody(content string, contentType string) string {
	content = html.UnescapeString(content)
	content = strings.ReplaceAll(content, "\u00a0", " ")
	if strings.Contains(strings.ToLower(contentType), "html") || strings.Contains(strings.ToLower(content), "<html") {
		content = imapHTMLTagPattern.ReplaceAllString(content, " ")
	}
	content = imapWhitespacePattern.ReplaceAllString(content, " ")
	content = imapPunctuationSpacingPattern.ReplaceAllString(content, "$1")
	return strings.TrimSpace(content)
}
