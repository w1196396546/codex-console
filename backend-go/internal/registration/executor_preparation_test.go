package registration

func executorPreparationDependencies() PreparationDependencies {
	return PreparationDependencies{
		Settings: fakePreparationSettings{
			settings: map[string]string{
				"tempmail.enabled":         "true",
				"tempmail.base_url":        "https://api.tempmail.example/v2",
				"tempmail.timeout":         "45",
				"tempmail.max_retries":     "7",
				"yyds_mail.enabled":        "true",
				"yyds_mail.base_url":       "https://maliapi.example/v1",
				"yyds_mail.api_key":        "secret-key",
				"yyds_mail.default_domain": "mail.example.com",
				"custom_domain.base_url":   "https://settings.moe.example/api",
				"custom_domain.api_key":    "settings-key",
			},
		},
		Outlook: fakeOutlookPreparationReader{
			services: []EmailServiceRecord{
				{
					ID:          42,
					ServiceType: "outlook",
					Name:        "Prepared Outlook",
					Config: map[string]any{
						"email":         "prepared@example.com",
						"password":      "outlook-secret",
						"client_id":     "client-1",
						"refresh_token": "refresh-1",
					},
				},
			},
		},
	}
}
