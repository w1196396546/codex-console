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
	Email    string
	Password string
}

type Outlook struct {
	email               string
	password            string
	imapAddress         string
	fallbackIMAPAddress string
	newIMAP             func(IMAPConfig) *IMAPMail
}

func NewOutlook(config OutlookConfig) *Outlook {
	return &Outlook{
		email:               strings.TrimSpace(config.Email),
		password:            strings.TrimSpace(config.Password),
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
	if o.password == "" {
		return Inbox{}, errors.New("outlook password is required")
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
	for _, address := range o.imapAddresses() {
		imapProvider, err := o.imapProvider(address)
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

func (o *Outlook) imapProvider(address string) (*IMAPMail, error) {
	host, portValue, err := net.SplitHostPort(strings.TrimSpace(address))
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

	return builder(IMAPConfig{
		Host:     strings.TrimSpace(host),
		Port:     port,
		Email:    o.email,
		Password: o.password,
		UseSSL:   true,
	}), nil
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
