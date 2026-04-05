package mail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIMAPMailCreateReturnsConfiguredInbox(t *testing.T) {
	t.Parallel()

	provider := NewIMAPMail(IMAPConfig{
		Email: "native@example.com",
	})

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	if inbox.Email != "native@example.com" {
		t.Fatalf("expected configured email, got %q", inbox.Email)
	}
	if inbox.Token != "" {
		t.Fatalf("expected empty token, got %q", inbox.Token)
	}
}

func TestIMAPMailWaitCodePollsFetcherUntilCodeArrives(t *testing.T) {
	t.Parallel()

	var polls int
	provider := NewIMAPMail(IMAPConfig{
		Email:        "native@example.com",
		PollInterval: 10 * time.Millisecond,
		Fetcher: func(ctx context.Context, inbox Inbox) ([]IMAPMessage, error) {
			polls++
			if polls == 1 {
				return []IMAPMessage{
					{
						From:    "alerts@example.com",
						Subject: "Weekly stats 654321",
						Body:    "This should be ignored",
					},
				}, nil
			}

			return []IMAPMessage{
				{
					From:    "OpenAI <noreply@openai.com>",
					Subject: "OpenAI sign-in",
					Body:    "Your verification code is 123456.",
				},
			}, nil
		},
	})

	code, err := provider.WaitCode(context.Background(), Inbox{Email: "native@example.com"}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "123456" {
		t.Fatalf("expected code 123456, got %q", code)
	}
	if polls < 2 {
		t.Fatalf("expected at least 2 polls, got %d", polls)
	}
}

func TestIMAPMailWaitCodeReturnsFetcherError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("fetch failed")
	provider := NewIMAPMail(IMAPConfig{
		Email: "native@example.com",
		Fetcher: func(ctx context.Context, inbox Inbox) ([]IMAPMessage, error) {
			return nil, expectedErr
		},
	})

	_, err := provider.WaitCode(context.Background(), Inbox{Email: "native@example.com"}, DefaultCodePattern)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected fetcher error, got %v", err)
	}
}

func TestIMAPMailWaitCodeHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var polls int
	provider := NewIMAPMail(IMAPConfig{
		Email:        "native@example.com",
		PollInterval: 10 * time.Millisecond,
		Fetcher: func(ctx context.Context, inbox Inbox) ([]IMAPMessage, error) {
			polls++
			if polls == 1 {
				cancel()
			}
			return nil, nil
		},
	})

	_, err := provider.WaitCode(ctx, Inbox{Email: "native@example.com"}, DefaultCodePattern)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestDefaultIMAPExtractorRequiresOpenAIVerificationMail(t *testing.T) {
	t.Parallel()

	extractor := defaultIMAPCodeExtractor()
	code, found := extractor([]IMAPMessage{
		{
			From:    "OpenAI <noreply@openai.com>",
			Subject: "Welcome to OpenAI",
			Body:    "Your monthly summary is 654321.",
		},
		{
			From:    "alerts@example.com",
			Subject: "Verification code 123456",
			Body:    "Use 123456 to continue.",
		},
	}, Inbox{}, regexp.MustCompile(`\b(\d{6})\b`))

	if found {
		t.Fatalf("expected no code, got %q", code)
	}
}

func TestDefaultIMAPExtractorPrefersSubjectThenBody(t *testing.T) {
	t.Parallel()

	extractor := defaultIMAPCodeExtractor()
	code, found := extractor([]IMAPMessage{
		{
			From:    "OpenAI <noreply@openai.com>",
			Subject: "Your verification code is 654321",
			Body:    "Your verification code is 123456.",
		},
	}, Inbox{}, regexp.MustCompile(`\b(\d{6})\b`))

	if !found {
		t.Fatal("expected verification code to be found")
	}
	if code != "654321" {
		t.Fatalf("expected subject code 654321, got %q", code)
	}
}

func TestIMAPMailWaitCodeSkipsMessagesBeforeOTPSentAt(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC)
	provider := NewIMAPMail(IMAPConfig{
		Email: "native@example.com",
		Fetcher: func(ctx context.Context, inbox Inbox) ([]IMAPMessage, error) {
			return []IMAPMessage{
				{
					ID:         "older-message",
					From:       "OpenAI <noreply@openai.com>",
					Subject:    "Your verification code is 111111",
					Body:       "Your verification code is 111111.",
					ReceivedAt: sentAt.Add(-15 * time.Second),
				},
				{
					ID:         "fresh-message",
					From:       "OpenAI <noreply@openai.com>",
					Subject:    "Your verification code is 222222",
					Body:       "Your verification code is 222222.",
					ReceivedAt: sentAt.Add(2 * time.Second),
				},
			}, nil
		},
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email:     "native@example.com",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code with otp sent at: %v", err)
	}
	if code != "222222" {
		t.Fatalf("expected newer code 222222, got %q", code)
	}
}

func TestIMAPMailWaitCodeDeduplicatesFingerprintsAcrossCalls(t *testing.T) {
	t.Parallel()

	provider := NewIMAPMail(IMAPConfig{
		Email:        "native@example.com",
		PollInterval: 5 * time.Millisecond,
		Fetcher: func(ctx context.Context, inbox Inbox) ([]IMAPMessage, error) {
			return []IMAPMessage{
				{
					ID:         "<same-message-id@example.com>",
					From:       "OpenAI <noreply@openai.com>",
					Subject:    "Your verification code is 123456",
					Body:       "Your verification code is 123456.",
					ReceivedAt: time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC),
				},
			}, nil
		},
	})

	code, err := provider.WaitCode(context.Background(), Inbox{Email: "native@example.com"}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("first wait code: %v", err)
	}
	if code != "123456" {
		t.Fatalf("expected first code 123456, got %q", code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = provider.WaitCode(ctx, Inbox{Email: "native@example.com"}, DefaultCodePattern)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected duplicate code to be ignored until context deadline, got %v", err)
	}
}

func TestIMAPMailWaitCodeResetsFallbackCodeDedupeForNewOTPStage(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC)
	provider := NewIMAPMail(IMAPConfig{
		Email:        "native@example.com",
		PollInterval: 5 * time.Millisecond,
		Fetcher: func(ctx context.Context, inbox Inbox) ([]IMAPMessage, error) {
			return []IMAPMessage{
				{
					From:    "OpenAI <noreply@openai.com>",
					Subject: "Your verification code is 654321",
					Body:    "Your verification code is 654321.",
				},
			}, nil
		},
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email:     "native@example.com",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("first wait code for stage one: %v", err)
	}
	if code != "654321" {
		t.Fatalf("expected first stage code 654321, got %q", code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = provider.WaitCode(ctx, Inbox{
		Email:     "native@example.com",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected repeated stage to skip fallback duplicate, got %v", err)
	}

	code, err = provider.WaitCode(context.Background(), Inbox{
		Email:     "native@example.com",
		OTPSentAt: sentAt.Add(10 * time.Second),
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code for new otp stage: %v", err)
	}
	if code != "654321" {
		t.Fatalf("expected same fallback code to be accepted for a new stage, got %q", code)
	}
}

func TestParseFetchedMessageDecodesHeadersAndExtractsHTMLBody(t *testing.T) {
	t.Parallel()

	raw := strings.Join([]string{
		"From: =?UTF-8?Q?OpenAI_=E5=AE=89=E5=85=A8?= <noreply@openai.com>",
		"Subject: =?UTF-8?Q?Your_=E9=AA=8C=E8=AF=81_code?=",
		"Date: Mon, 03 Mar 2025 10:00:00 +0000",
		"Message-ID: <decoded-message-id@example.com>",
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=\"mail-boundary\"",
		"",
		"--mail-boundary",
		"Content-Type: text/html; charset=UTF-8",
		"Content-Transfer-Encoding: quoted-printable",
		"",
		"<html><body>Your&nbsp;verification <b>code</b> is <strong>246810</strong>.</body></html>",
		"--mail-boundary--",
		"",
	}, "\r\n")

	message, err := parseFetchedMessage("101", raw)
	if err != nil {
		t.Fatalf("parse fetched message: %v", err)
	}
	if message.ID != "<decoded-message-id@example.com>" {
		t.Fatalf("expected message-id to win over uid fallback, got %q", message.ID)
	}
	if message.Subject != "Your 验证 code" {
		t.Fatalf("expected decoded subject, got %q", message.Subject)
	}
	if message.From != "OpenAI 安全 <noreply@openai.com>" {
		t.Fatalf("expected decoded from header, got %q", message.From)
	}
	if strings.Contains(message.Body, "<html") || strings.Contains(message.Body, "<strong>") {
		t.Fatalf("expected html tags stripped from body, got %q", message.Body)
	}
	if !strings.Contains(message.Body, "Your verification code is 246810.") {
		t.Fatalf("expected extracted html body content, got %q", message.Body)
	}
}

func TestIMAPMailWaitCodeUsesDefaultFetcherAgainstLocalServer(t *testing.T) {
	t.Parallel()

	server := newFakeIMAPServer(t, fakeIMAPServerConfig{
		email:    "native@example.com",
		password: "secret-password",
		messages: []fakeIMAPServerMessage{
			{
				uid:     101,
				from:    "OpenAI <noreply@openai.com>",
				subject: "OpenAI verification",
				body:    "Your verification code is 246810.",
				date:    time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC),
			},
		},
	})

	provider := NewIMAPMail(IMAPConfig{
		Host:         server.host,
		Port:         server.port,
		Email:        "native@example.com",
		Password:     "secret-password",
		UseSSL:       false,
		PollInterval: time.Millisecond,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{Email: "native@example.com"}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code via default fetcher: %v", err)
	}
	if code != "246810" {
		t.Fatalf("expected code 246810, got %q", code)
	}
}

func TestIMAPMailWaitCodeUsesInboxCredentialsFallbackForDefaultFetcher(t *testing.T) {
	t.Parallel()

	server := newFakeIMAPServer(t, fakeIMAPServerConfig{
		email:    "native@example.com",
		password: "secret-password",
		messages: []fakeIMAPServerMessage{
			{
				uid:     102,
				from:    "OpenAI <noreply@openai.com>",
				subject: "OpenAI sign-in",
				body:    "Your verification code is 135790.",
				date:    time.Date(2025, 3, 2, 10, 0, 0, 0, time.UTC),
			},
		},
	})

	provider := NewIMAPMail(IMAPConfig{
		Host:         server.host,
		Port:         server.port,
		UseSSL:       false,
		PollInterval: time.Millisecond,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email: "native@example.com",
		Token: "secret-password",
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code with inbox fallback: %v", err)
	}
	if code != "135790" {
		t.Fatalf("expected code 135790, got %q", code)
	}
}

type fakeIMAPServerConfig struct {
	email    string
	password string
	messages []fakeIMAPServerMessage
}

type fakeIMAPServerMessage struct {
	uid     uint32
	from    string
	subject string
	body    string
	date    time.Time
}

type fakeIMAPServer struct {
	host string
	port int
}

func newFakeIMAPServer(t *testing.T, cfg fakeIMAPServerConfig) fakeIMAPServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake imap server: %v", err)
	}

	server := fakeIMAPServer{
		host: "127.0.0.1",
		port: listener.Addr().(*net.TCPAddr).Port,
	}

	var once sync.Once
	closeListener := func() {
		once.Do(func() {
			_ = listener.Close()
		})
	}

	t.Cleanup(closeListener)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		closeListener()
		serveFakeIMAPConnection(conn, cfg)
	}()

	return server
}

func serveFakeIMAPConnection(conn net.Conn, cfg fakeIMAPServerConfig) {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	writeLine := func(format string, args ...any) {
		_, _ = fmt.Fprintf(writer, format+"\r\n", args...)
		_ = writer.Flush()
	}

	readLiteral := func(size int) string {
		if size <= 0 {
			return ""
		}
		buf := make([]byte, size)
		_, _ = io.ReadFull(reader, buf)
		return string(buf)
	}

	writeLine("* OK Fake IMAP ready")

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}

		fields := strings.Split(line, " ")
		if len(fields) < 2 {
			continue
		}
		tag := fields[0]
		command := strings.ToUpper(fields[1])

		switch command {
		case "CAPABILITY":
			writeLine("* CAPABILITY IMAP4rev1 UIDPLUS")
			writeLine("%s OK CAPABILITY completed", tag)
		case "LOGIN":
			if len(fields) >= 4 && fields[2] == cfg.email && fields[3] == cfg.password {
				writeLine("%s OK LOGIN completed", tag)
				continue
			}
			writeLine("%s NO LOGIN failed", tag)
		case "SELECT":
			writeLine("* FLAGS (\\Seen)")
			writeLine("* %d EXISTS", len(cfg.messages))
			writeLine("* 0 RECENT")
			writeLine("%s OK [READ-WRITE] SELECT completed", tag)
		case "UID":
			if len(fields) < 3 {
				writeLine("%s BAD UID command", tag)
				continue
			}
			switch strings.ToUpper(fields[2]) {
			case "SEARCH":
				var uidParts []string
				for _, message := range cfg.messages {
					uidParts = append(uidParts, fmt.Sprintf("%d", message.uid))
				}
				writeLine("* SEARCH %s", strings.Join(uidParts, " "))
				writeLine("%s OK UID SEARCH completed", tag)
			case "FETCH":
				for index, message := range cfg.messages {
					body := fakeIMAPRawMessage(message)
					writeLine("* %d FETCH (UID %d RFC822 {%d}", index+1, message.uid, len(body))
					_, _ = writer.WriteString(body)
					_, _ = writer.WriteString("\r\n")
					_ = writer.Flush()
					writeLine(")")
				}
				writeLine("%s OK UID FETCH completed", tag)
			default:
				writeLine("%s BAD unsupported UID command", tag)
			}
		case "LOGOUT":
			writeLine("* BYE Fake IMAP signing off")
			writeLine("%s OK LOGOUT completed", tag)
			return
		case "ID":
			if strings.Contains(line, "{") {
				start := strings.LastIndex(line, "{")
				end := strings.LastIndex(line, "}")
				if start >= 0 && end > start {
					var size int
					_, _ = fmt.Sscanf(line[start:end+1], "{%d}", &size)
					writeLine("+ id literal")
					_ = readLiteral(size)
				}
			}
			writeLine("%s OK ID completed", tag)
		default:
			writeLine("%s BAD unsupported command", tag)
		}
	}
}

func fakeIMAPRawMessage(message fakeIMAPServerMessage) string {
	return fmt.Sprintf(
		"Date: %s\r\nFrom: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		message.date.Format(time.RFC1123Z),
		message.from,
		message.subject,
		message.body,
	)
}
