package mail

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"
)

const outlookOffice365IMAPAddress = "outlook.office365.com:993"
const outlookLiveIMAPAddress = "outlook.live.com:993"

type OutlookConfig struct {
	Email        string
	Password     string
	ClientID     string
	RefreshToken string
	ProxyURL     string
}

type Outlook struct {
	email               string
	password            string
	clientID            string
	refreshToken        string
	proxyURL            string
	imapAddress         string
	fallbackIMAPAddress string
	newIMAP             func(IMAPConfig) *IMAPMail
}

func NewOutlook(config OutlookConfig) *Outlook {
	return &Outlook{
		email:               strings.TrimSpace(config.Email),
		password:            strings.TrimSpace(config.Password),
		clientID:            strings.TrimSpace(config.ClientID),
		refreshToken:        strings.TrimSpace(config.RefreshToken),
		proxyURL:            strings.TrimSpace(config.ProxyURL),
		imapAddress:         outlookOffice365IMAPAddress,
		fallbackIMAPAddress: outlookLiveIMAPAddress,
		newIMAP:             NewIMAPMail,
	}
}

func (o *Outlook) Create(context.Context) (Inbox, error) {
	if o == nil {
		return Inbox{}, errors.New("outlook provider is required")
	}
	if o.email == "" {
		return Inbox{}, errors.New("outlook email is required")
	}
	if strings.TrimSpace(o.password) == "" && !o.hasOAuth() {
		return Inbox{}, errors.New("outlook password or oauth credentials are required")
	}

	return Inbox{
		Email: o.email,
		Token: o.password,
	}, nil
}

func (o *Outlook) WaitCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, error) {
	if o == nil {
		return "", errors.New("outlook provider is required")
	}

	var lastErr error
	for _, attempt := range o.imapAttempts() {
		imapProvider, err := o.imapProvider(attempt)
		if err != nil {
			lastErr = err
			continue
		}

		code, err := imapProvider.WaitCode(ctx, inbox, pattern)
		if err == nil {
			return code, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		lastErr = err
	}

	if lastErr == nil {
		lastErr = errors.New("outlook imap provider is unavailable")
	}
	return "", lastErr
}

func (o *Outlook) imapProvider(attempt outlookIMAPAttempt) (*IMAPMail, error) {
	host, portValue, err := net.SplitHostPort(strings.TrimSpace(attempt.address))
	if err != nil {
		return nil, err
	}

	port, err := strconv.Atoi(portValue)
	if err != nil {
		return nil, err
	}

	builder := o.newIMAP
	if builder == nil {
		builder = NewIMAPMail
	}

	config := IMAPConfig{
		Host:     strings.TrimSpace(host),
		Port:     port,
		Email:    o.email,
		Password: o.password,
		ProxyURL: o.proxyURL,
		UseSSL:   true,
	}
	if attempt.authMode == outlookIMAPAuthOAuth2 {
		config.OAuth2AccessTokenSource = newOutlookOAuth2AccessTokenSource(outlookOAuth2TokenConfig{
			Email:        o.email,
			ClientID:     o.clientID,
			RefreshToken: o.refreshToken,
			ProxyURL:     o.proxyURL,
			TokenURL:     attempt.oauthTokenURL,
			Scope:        attempt.oauthScope,
		})
	}

	return builder(config), nil
}

func (o *Outlook) imapAddresses() []string {
	primary := strings.TrimSpace(o.imapAddress)
	fallback := strings.TrimSpace(o.fallbackIMAPAddress)

	addresses := make([]string, 0, 2)
	if primary != "" {
		addresses = append(addresses, primary)
	}
	if fallback != "" && !strings.EqualFold(fallback, primary) {
		addresses = append(addresses, fallback)
	}
	return addresses
}

func (o *Outlook) hasOAuth() bool {
	return o != nil && o.clientID != "" && o.refreshToken != ""
}

func (o *Outlook) imapAttempts() []outlookIMAPAttempt {
	addresses := o.imapAddresses()
	attempts := make([]outlookIMAPAttempt, 0, len(addresses)*2)
	for _, address := range addresses {
		if o.hasOAuth() {
			tokenURL, scope := outlookOAuthSettingsForAddress(address)
			attempts = append(attempts, outlookIMAPAttempt{
				address:       address,
				authMode:      outlookIMAPAuthOAuth2,
				oauthTokenURL: tokenURL,
				oauthScope:    scope,
			})
			continue
		}
		if o.password != "" {
			attempts = append(attempts, outlookIMAPAttempt{
				address:  address,
				authMode: outlookIMAPAuthPassword,
			})
		}
	}
	return attempts
}

type outlookIMAPAuthMode string

const (
	outlookIMAPAuthPassword outlookIMAPAuthMode = "password"
	outlookIMAPAuthOAuth2   outlookIMAPAuthMode = "oauth2"
)

type outlookIMAPAttempt struct {
	address       string
	authMode      outlookIMAPAuthMode
	oauthTokenURL string
	oauthScope    string
}
