package registration

type StartRequest struct {
	EmailServiceType   string         `json:"email_service_type"`
	Proxy              string         `json:"proxy,omitempty"`
	EmailServiceID     *int           `json:"email_service_id,omitempty"`
	EmailServiceConfig map[string]any `json:"email_service_config,omitempty"`
}

type TaskResponse struct {
	TaskUUID string `json:"task_uuid"`
	Status   string `json:"status"`
}
