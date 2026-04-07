package mail

import (
	stdmail "net/mail"
	"strings"
	"testing"
	"time"
)

func TestExtractMessageBodyDecodesQuotedPrintableHTMLCharset(t *testing.T) {
	t.Parallel()

	raw := strings.Join([]string{
		"From: OpenAI <noreply@openai.com>",
		"Subject: =?ISO-8859-1?Q?Votre_code_de_v=E9rification?=",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=ISO-8859-1",
		"Content-Transfer-Encoding: quoted-printable",
		"",
		"<html><body>Votre&nbsp;v=E9rification code is <strong>246810</strong>.</body></html>",
	}, "\r\n")

	message, err := stdmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("read raw message: %v", err)
	}

	body, err := extractMessageBody(message.Header, message.Body)
	if err != nil {
		t.Fatalf("extract message body: %v", err)
	}

	if body != "Votre vérification code is 246810." {
		t.Fatalf("expected decoded body, got %q", body)
	}

	subject := decodeMIMEHeader(message.Header.Get("Subject"))
	if subject != "Votre code de vérification" {
		t.Fatalf("expected decoded subject, got %q", subject)
	}
}

func TestParseTempmailMessageTimeSupportsUnixMilliseconds(t *testing.T) {
	t.Parallel()

	expected := time.Date(2025, 3, 3, 10, 0, 2, 0, time.UTC)

	got := parseTempmailMessageTime(expected.UnixMilli())
	if !got.Equal(expected) {
		t.Fatalf("expected %s from unix milliseconds, got %s", expected.Format(time.RFC3339), got.Format(time.RFC3339))
	}
}
