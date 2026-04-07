package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	sentinelReqURL           = "https://sentinel.openai.com/backend-api/sentinel/req"
	defaultSecCHUA           = `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`
	maxSentinelTries         = 500000
	defaultSentinelUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

var (
	sentinelNavigatorProps = []string{
		"vendorSub", "productSub", "vendor", "maxTouchPoints",
		"scheduling", "userActivation", "doNotTrack", "geolocation",
		"connection", "plugins", "mimeTypes", "pdfViewerEnabled",
		"webkitTemporaryStorage", "webkitPersistentStorage",
		"hardwareConcurrency", "cookieEnabled", "credentials",
		"mediaDevices", "permissions", "locks", "ink",
	}
	sentinelDocKeys = []string{"location", "implementation", "URL", "documentURI", "compatMode"}
	sentinelWinKeys = []string{"Object", "Function", "Array", "Number", "parseFloat", "undefined"}
)

type sentinelChallengeResponse struct {
	Token       string `json:"token"`
	ProofOfWork struct {
		Required   bool   `json:"required"`
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
}

type sentinelTokenGenerator struct {
	deviceID         string
	userAgent        string
	requirementsSeed string
	sessionID        string
}

func newSentinelTokenGenerator(deviceID string, userAgent string) *sentinelTokenGenerator {
	return &sentinelTokenGenerator{
		deviceID:         strings.TrimSpace(deviceID),
		userAgent:        firstNonEmpty(strings.TrimSpace(userAgent), defaultSentinelUserAgent),
		requirementsSeed: fmt.Sprintf("%f", rand.Float64()),
		sessionID:        fmt.Sprintf("%d", time.Now().UnixNano()),
	}
}

func (g *sentinelTokenGenerator) buildConfig() []any {
	now := time.Now().UTC()
	dateText := now.Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")
	perfNow := 1000 + rand.Float64()*49000
	timeOrigin := float64(time.Now().UnixNano())/1e6 - perfNow

	return []any{
		"1920x1080",
		dateText,
		4294705152,
		0,
		g.userAgent,
		"https://sentinel.openai.com/sentinel/20260124ceb8/sdk.js",
		nil,
		nil,
		"en-US",
		"en-US,en",
		rand.Float64(),
		sentinelNavigatorProps[rand.Intn(len(sentinelNavigatorProps))] + "−undefined",
		sentinelDocKeys[rand.Intn(len(sentinelDocKeys))],
		sentinelWinKeys[rand.Intn(len(sentinelWinKeys))],
		perfNow,
		g.sessionID,
		"",
		[]int{4, 8, 12, 16}[rand.Intn(4)],
		timeOrigin,
	}
}

func (g *sentinelTokenGenerator) base64Encode(value any) string {
	raw, _ := json.Marshal(value)
	return base64.StdEncoding.EncodeToString(raw)
}

func fnv1a32Hex(text string) string {
	var h uint32 = 2166136261
	for _, ch := range text {
		h ^= uint32(ch)
		h *= 16777619
	}

	h ^= h >> 16
	h *= 2246822507
	h ^= h >> 13
	h *= 3266489909
	h ^= h >> 16
	return fmt.Sprintf("%08x", h)
}

func (g *sentinelTokenGenerator) generateRequirementsToken() string {
	config := g.buildConfig()
	config[3] = 1
	config[9] = int(math.Round(5 + rand.Float64()*45))
	return "gAAAAAC" + g.base64Encode(config)
}

func (g *sentinelTokenGenerator) generateProofToken(seed string, difficulty string) string {
	if strings.TrimSpace(seed) == "" {
		return ""
	}
	if strings.TrimSpace(difficulty) == "" {
		difficulty = "0"
	}

	startedAt := time.Now()
	config := g.buildConfig()
	diffLen := len(difficulty)

	for nonce := 0; nonce < maxSentinelTries; nonce++ {
		config[3] = nonce
		config[9] = int(math.Round(float64(time.Since(startedAt).Milliseconds())))
		data := g.base64Encode(config)
		hashHex := fnv1a32Hex(seed + data)
		if diffLen <= len(hashHex) && hashHex[:diffLen] <= difficulty {
			return "gAAAAAB" + data + "~S"
		}
	}

	return ""
}

func (c *Client) shouldUseSentinel(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	switch {
	case host == "chatgpt.com", host == "auth.openai.com", host == "openai.com":
		return true
	case strings.HasSuffix(host, ".chatgpt.com"), strings.HasSuffix(host, ".openai.com"):
		return true
	default:
		return false
	}
}

func (c *Client) sentinelHeaderToken(ctx context.Context, flow string, rawURL string) string {
	if c == nil || !c.shouldUseSentinel(rawURL) {
		return ""
	}

	deviceID := strings.TrimSpace(c.deviceID)
	if deviceID == "" {
		return ""
	}

	generator := newSentinelTokenGenerator(deviceID, c.userAgent)
	requirementsToken := generator.generateRequirementsToken()

	requestBody, err := json.Marshal(map[string]string{
		"p":    requirementsToken,
		"id":   deviceID,
		"flow": strings.TrimSpace(flow),
	})
	if err != nil {
		return ""
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, sentinelReqURL, bytes.NewReader(requestBody))
	if err != nil {
		return ""
	}
	request.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	request.Header.Set("Referer", "https://sentinel.openai.com/backend-api/sentinel/frame.html")
	request.Header.Set("Origin", "https://sentinel.openai.com")
	request.Header.Set("User-Agent", firstNonEmpty(strings.TrimSpace(c.userAgent), defaultSentinelUserAgent))
	request.Header.Set("sec-ch-ua", defaultSecCHUA)
	request.Header.Set("sec-ch-ua-mobile", "?0")
	request.Header.Set("sec-ch-ua-platform", `"Windows"`)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return ""
	}

	var challenge sentinelChallengeResponse
	if err := json.Unmarshal(body, &challenge); err != nil {
		return ""
	}
	if strings.TrimSpace(challenge.Token) == "" {
		return ""
	}

	pValue := requirementsToken
	if challenge.ProofOfWork.Required && strings.TrimSpace(challenge.ProofOfWork.Seed) != "" {
		proofToken := generator.generateProofToken(challenge.ProofOfWork.Seed, challenge.ProofOfWork.Difficulty)
		if proofToken == "" {
			return ""
		}
		pValue = proofToken
	}

	headerValue, err := json.Marshal(map[string]string{
		"p":    pValue,
		"t":    "",
		"c":    strings.TrimSpace(challenge.Token),
		"id":   deviceID,
		"flow": strings.TrimSpace(flow),
	})
	if err != nil {
		return ""
	}
	return string(headerValue)
}
