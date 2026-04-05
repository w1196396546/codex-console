package registration

type StartRequest struct {
	EmailServiceType   string         `json:"email_service_type"`
	Proxy              string         `json:"proxy,omitempty"`
	EmailServiceID     *int           `json:"email_service_id,omitempty"`
	EmailServiceConfig map[string]any `json:"email_service_config,omitempty"`
	IntervalMin        int            `json:"interval_min,omitempty"`
	IntervalMax        int            `json:"interval_max,omitempty"`
	Concurrency        int            `json:"concurrency,omitempty"`
	Mode               string         `json:"mode,omitempty"`
	AutoUploadCPA      bool           `json:"auto_upload_cpa,omitempty"`
	CPAServiceIDs      []int          `json:"cpa_service_ids,omitempty"`
	AutoUploadSub2API  bool           `json:"auto_upload_sub2api,omitempty"`
	Sub2APIServiceIDs  []int          `json:"sub2api_service_ids,omitempty"`
	AutoUploadTM       bool           `json:"auto_upload_tm,omitempty"`
	TMServiceIDs       []int          `json:"tm_service_ids,omitempty"`
}

type TaskResponse struct {
	TaskUUID     string `json:"task_uuid"`
	Status       string `json:"status"`
	Email        any    `json:"email"`
	EmailService any    `json:"email_service"`
}

type TaskListResponse struct {
	Total int            `json:"total"`
	Tasks []TaskResponse `json:"tasks"`
}
