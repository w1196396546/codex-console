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

type OutlookConfig struct {
	Email    string
	Password string
}

type Outlook struct {
	email       string
	password    string
	imapAddress string
	newIMAP     func(IMAPConfig) *IMAPMail
}

func NewOutlook(config OutlookConfig) *Outlook {
	return &Outlook{
		email:       strings.TrimSpace(config.Email),
		password:    strings.TrimSpace(config.Password),
		imapAddress: outlookOffice365IMAPAddress,
		newIMAP:     NewIMAPMail,
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

	imapProvider, err := o.imapProvider()
	if err != nil {
		return "", err
	}

	return imapProvider.WaitCode(ctx, inbox, pattern)
}

func (o *Outlook) imapProvider() (*IMAPMail, error) {
	host, portValue, err := net.SplitHostPort(strings.TrimSpace(o.imapAddress))
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
