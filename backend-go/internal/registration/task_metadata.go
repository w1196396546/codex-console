package registration

import (
	"encoding/json"
	"strings"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

type TaskMetadata struct {
	Email        any
	EmailService any
}

func ResolveTaskMetadata(job jobs.Job) TaskMetadata {
	metadata := TaskMetadata{
		Email:        nil,
		EmailService: nil,
	}

	var request StartRequest
	if len(job.Payload) > 0 && json.Unmarshal(job.Payload, &request) == nil {
		if serviceType := strings.TrimSpace(request.EmailServiceType); serviceType != "" {
			metadata.EmailService = serviceType
		}
	}

	if len(job.Result) == 0 {
		return metadata
	}

	var result struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(job.Result, &result) != nil {
		return metadata
	}
	if email := strings.TrimSpace(result.Email); email != "" {
		metadata.Email = email
	}

	return metadata
}
