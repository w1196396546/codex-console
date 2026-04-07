package mail

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type otpCodeTracker struct {
	mu     sync.Mutex
	states map[string]*otpCodeState
}

type otpCodeState struct {
	fingerprints  map[string]struct{}
	fallbackCodes map[string]struct{}
	stageMarker   int64
}

func newOTPCodeTracker() *otpCodeTracker {
	return &otpCodeTracker{
		states: make(map[string]*otpCodeState),
	}
}

func (t *otpCodeTracker) prepare(inboxKey string, sentAt time.Time) {
	if t == nil || inboxKey == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.ensureStateLocked(inboxKey)
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

func (t *otpCodeTracker) hasSeen(inboxKey, fingerprint, fallbackCode string) bool {
	if t == nil || inboxKey == "" {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.ensureStateLocked(inboxKey)
	if fingerprint != "" {
		if _, ok := state.fingerprints[fingerprint]; ok {
			return true
		}
	}
	if fallbackCode != "" {
		_, ok := state.fallbackCodes[fallbackCode]
		return ok
	}
	return false
}

func (t *otpCodeTracker) markSeen(inboxKey, fingerprint, fallbackCode string) {
	if t == nil || inboxKey == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.ensureStateLocked(inboxKey)
	if fingerprint != "" {
		state.fingerprints[fingerprint] = struct{}{}
	}
	if fallbackCode != "" {
		state.fallbackCodes[fallbackCode] = struct{}{}
	}
}

func (t *otpCodeTracker) ensureStateLocked(inboxKey string) *otpCodeState {
	state := t.states[inboxKey]
	if state == nil {
		state = &otpCodeState{
			fingerprints:  make(map[string]struct{}),
			fallbackCodes: make(map[string]struct{}),
		}
		t.states[inboxKey] = state
	}
	return state
}

func otpInboxStateKey(inbox Inbox) string {
	if email := strings.ToLower(strings.TrimSpace(inbox.Email)); email != "" {
		return email
	}
	return strings.ToLower(strings.TrimSpace(inbox.Token))
}

func otpCodeFingerprint(messageID string, receivedAt time.Time, content, code string) (string, string) {
	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" {
		return "", ""
	}

	normalizedID := strings.TrimSpace(messageID)
	if normalizedID == "" {
		normalizedID = "-"
	}

	mailTS := int64(0)
	if !receivedAt.IsZero() {
		mailTS = receivedAt.Unix()
	}

	fingerprintTarget := normalizedID
	if fingerprintTarget == "-" {
		fingerprintTarget = strings.TrimSpace(content)
	}

	fingerprint := fmt.Sprintf("%d|%s|%s", mailTS, fingerprintTarget, trimmedCode)
	if normalizedID == "-" && mailTS <= 0 {
		return fingerprint, trimmedCode
	}
	return fingerprint, ""
}
