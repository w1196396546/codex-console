package mail

import (
	"encoding/json"
	"fmt"
	"math"
	stdmail "net/mail"
	"strconv"
	"strings"
	"time"
)

const defaultUnknownTimestampGrace = 15 * time.Second

var apiMessageTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	time.RFC1123Z,
	time.RFC1123,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
}

type rawMessageContent struct {
	From    string
	Subject string
	Body    string
}

func firstParsedMessageTime(values ...any) time.Time {
	for _, value := range values {
		if parsed := parseMessageTimeValue(value); !parsed.IsZero() {
			return parsed
		}
	}
	return time.Time{}
}

func parseMessageTimeValue(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return typed.UTC()
	case json.Number:
		return parseMessageTimeString(typed.String())
	case float64:
		return unixTimeFromFloat(typed)
	case float32:
		return unixTimeFromFloat(float64(typed))
	case int:
		return unixTimeFromNumeric(int64(typed))
	case int32:
		return unixTimeFromNumeric(int64(typed))
	case int64:
		return unixTimeFromNumeric(typed)
	case uint:
		return unixTimeFromNumeric(int64(typed))
	case uint32:
		return unixTimeFromNumeric(int64(typed))
	case uint64:
		if typed > math.MaxInt64 {
			return time.Time{}
		}
		return unixTimeFromNumeric(int64(typed))
	case string:
		return parseMessageTimeString(typed)
	case []byte:
		return parseMessageTimeString(string(typed))
	default:
		return time.Time{}
	}
}

func parseMessageTimeString(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}
	}

	if numeric, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return unixTimeFromFloat(numeric)
	}

	for _, layout := range apiMessageTimeLayouts {
		var (
			parsed time.Time
			err    error
		)
		switch layout {
		case time.RFC3339Nano, time.RFC3339, time.RFC1123Z, time.RFC1123:
			parsed, err = time.Parse(layout, trimmed)
		default:
			parsed, err = time.ParseInLocation(layout, trimmed, time.UTC)
		}
		if err == nil {
			return parsed.UTC()
		}
	}

	return time.Time{}
}

func unixTimeFromFloat(value float64) time.Time {
	if value <= 0 {
		return time.Time{}
	}

	if value >= 1_000_000_000_000 {
		return time.UnixMilli(int64(value)).UTC()
	}

	seconds, fraction := math.Modf(value)
	nanos := int64(math.Round(fraction * float64(time.Second)))
	return time.Unix(int64(seconds), nanos).UTC()
}

func flattenMessageText(value any) string {
	return flattenMessageValue(value, "text/plain")
}

func flattenMessageHTML(value any) string {
	return flattenMessageValue(value, "text/html")
}

func flattenMessageValue(value any, contentType string) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return normalizeExtractedBody(typed, contentType)
	case []string:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := normalizeExtractedBody(item, contentType); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := flattenMessageValue(item, contentType); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return normalizeExtractedBody(fmt.Sprint(typed), contentType)
	}
}

func extractRawMessageContent(value any) rawMessageContent {
	raw := strings.TrimSpace(rawMessageString(value))
	if raw == "" {
		return rawMessageContent{}
	}

	message, err := stdmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return rawMessageContent{Body: normalizeExtractedBody(raw, "")}
	}

	body, err := extractMessageBody(message.Header, message.Body)
	if err != nil {
		body = normalizeExtractedBody(raw, "")
	}

	return rawMessageContent{
		From:    decodeMIMEHeader(message.Header.Get("From")),
		Subject: decodeMIMEHeader(message.Header.Get("Subject")),
		Body:    body,
	}
}

func firstRawMessageContent(values ...any) rawMessageContent {
	for _, value := range values {
		if content := extractRawMessageContent(value); content != (rawMessageContent{}) {
			return content
		}
	}
	return rawMessageContent{}
}

func rawMessageString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func buildSearchText(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return strings.Join(filtered, "\n")
}

func shouldWaitForTimestampedMessage(sentAt, now time.Time) bool {
	if sentAt.IsZero() {
		return false
	}
	return now.UTC().Before(sentAt.UTC().Add(defaultUnknownTimestampGrace))
}
