package registration_test

import (
	"context"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
)

func TestBuildAvailableServices(t *testing.T) {
	response := registration.BuildAvailableServices(
		map[string]string{
			"tempmail_enabled":         "true",
			"yyds_mail_enabled":        "true",
			"yyds_mail_api_key":        "secret",
			"yyds_mail_default_domain": "mail.example.com",
			"custom_domain_base_url":   "https://custom.example.com",
			"custom_domain_api_key":    "custom-secret",
		},
		[]registration.EmailServiceRecord{
			{
				ID:          11,
				ServiceType: "outlook",
				Name:        "Outlook A",
				Priority:    1,
				Config: map[string]any{
					"client_id":     "client-1",
					"refresh_token": "refresh-1",
				},
			},
			{
				ID:          12,
				ServiceType: "temp_mail",
				Name:        "Temp Worker",
				Priority:    2,
				Config: map[string]any{
					"domain": "temp.example.com",
				},
			},
			{
				ID:          13,
				ServiceType: "luckmail",
				Name:        "Luck Worker",
				Priority:    3,
				Config: map[string]any{
					"preferred_domain": "luck.example.com",
				},
			},
		},
	)

	tempmail := response["tempmail"]
	if !tempmail.Available || tempmail.Count != 1 || len(tempmail.Services) != 1 {
		t.Fatalf("unexpected tempmail group: %+v", tempmail)
	}

	yyds := response["yyds_mail"]
	if !yyds.Available || yyds.Count != 1 || len(yyds.Services) != 1 {
		t.Fatalf("unexpected yyds_mail group: %+v", yyds)
	}
	if yyds.Services[0]["default_domain"] != "mail.example.com" {
		t.Fatalf("expected yyds default_domain, got %#v", yyds.Services[0]["default_domain"])
	}

	outlook := response["outlook"]
	if !outlook.Available || outlook.Count != 1 || len(outlook.Services) != 1 {
		t.Fatalf("unexpected outlook group: %+v", outlook)
	}
	if outlook.Services[0]["id"] != 11 {
		t.Fatalf("expected outlook id=11, got %#v", outlook.Services[0]["id"])
	}
	if outlook.Services[0]["has_oauth"] != true {
		t.Fatalf("expected outlook has_oauth=true, got %#v", outlook.Services[0]["has_oauth"])
	}

	moeMail := response["moe_mail"]
	if !moeMail.Available || moeMail.Count != 1 || len(moeMail.Services) != 1 {
		t.Fatalf("expected moe_mail settings fallback, got %+v", moeMail)
	}
	if moeMail.Services[0]["from_settings"] != true {
		t.Fatalf("expected moe_mail from_settings=true, got %#v", moeMail.Services[0]["from_settings"])
	}

	tempMail := response["temp_mail"]
	if !tempMail.Available || tempMail.Count != 1 || len(tempMail.Services) != 1 {
		t.Fatalf("unexpected temp_mail group: %+v", tempMail)
	}
	if tempMail.Services[0]["domain"] != "temp.example.com" {
		t.Fatalf("expected temp_mail domain, got %#v", tempMail.Services[0]["domain"])
	}

	luckmail := response["luckmail"]
	if !luckmail.Available || luckmail.Count != 1 || len(luckmail.Services) != 1 {
		t.Fatalf("unexpected luckmail group: %+v", luckmail)
	}
	if luckmail.Services[0]["preferred_domain"] != "luck.example.com" {
		t.Fatalf("expected luckmail preferred_domain, got %#v", luckmail.Services[0]["preferred_domain"])
	}

	if response["duck_mail"].Available || response["duck_mail"].Count != 0 || len(response["duck_mail"].Services) != 0 {
		t.Fatalf("expected duck_mail empty group, got %+v", response["duck_mail"])
	}
}

func TestListAvailableServices(t *testing.T) {
	service := registration.NewAvailableServicesService(availableServicesFakeRepository{
		settings: map[string]string{
			"tempmail_enabled": "false",
		},
		services: []registration.EmailServiceRecord{
			{
				ID:          22,
				ServiceType: "outlook",
				Name:        "Outlook B",
				Config:      map[string]any{},
			},
		},
	})

	response, err := service.ListAvailableServices(context.Background())
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}

	if response["tempmail"].Available {
		t.Fatalf("expected tempmail unavailable, got %+v", response["tempmail"])
	}
	if !response["outlook"].Available || response["outlook"].Count != 1 {
		t.Fatalf("expected one outlook service, got %+v", response["outlook"])
	}
}

type availableServicesFakeRepository struct {
	settings map[string]string
	services []registration.EmailServiceRecord
}

func (f availableServicesFakeRepository) GetSettings(_ context.Context, _ []string) (map[string]string, error) {
	return f.settings, nil
}

func (f availableServicesFakeRepository) ListEmailServices(_ context.Context) ([]registration.EmailServiceRecord, error) {
	return f.services, nil
}
