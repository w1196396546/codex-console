package registration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/uploader"
)

type AutoUploadDispatchRequest struct {
	JobID        string
	StartRequest StartRequest
	Account      accounts.Account
}

type AutoUploadDispatcher interface {
	Dispatch(ctx context.Context, req AutoUploadDispatchRequest, logf func(level string, message string) error) (AutoUploadDispatchResult, error)
}

type AutoUploadDispatchResult struct {
	AccountUpdate accounts.UpsertAccountRequest
}

type autoUploadSenderFactory func(kind uploader.UploadKind) (uploader.Sender, error)

type defaultAutoUploadDispatcher struct {
	configs       uploader.ConfigRepository
	senderFactory autoUploadSenderFactory
}

type autoUploadDispatchPlan struct {
	enabled    bool
	label      string
	kind       uploader.UploadKind
	serviceIDs []int
	list       func(context.Context, []int) ([]uploader.ServiceConfig, error)
}

func NewAutoUploadDispatcher(configs uploader.ConfigRepository, doer uploader.HTTPDoer) AutoUploadDispatcher {
	return newAutoUploadDispatcher(configs, func(kind uploader.UploadKind) (uploader.Sender, error) {
		return uploader.NewSender(kind, doer)
	})
}

func newAutoUploadDispatcher(configs uploader.ConfigRepository, senderFactory autoUploadSenderFactory) AutoUploadDispatcher {
	if configs == nil || senderFactory == nil {
		return nil
	}
	return &defaultAutoUploadDispatcher{
		configs:       configs,
		senderFactory: senderFactory,
	}
}

func (d *defaultAutoUploadDispatcher) Dispatch(ctx context.Context, req AutoUploadDispatchRequest, logf func(level string, message string) error) (AutoUploadDispatchResult, error) {
	if d == nil || d.configs == nil || d.senderFactory == nil {
		return AutoUploadDispatchResult{}, nil
	}

	account := toUploadAccount(req.Account)
	var dispatchResult AutoUploadDispatchResult
	plans := []autoUploadDispatchPlan{
		{
			enabled:    req.StartRequest.AutoUploadCPA,
			label:      "CPA",
			kind:       uploader.UploadKindCPA,
			serviceIDs: append([]int(nil), req.StartRequest.CPAServiceIDs...),
			list:       d.configs.ListCPAServiceConfigs,
		},
		{
			enabled:    req.StartRequest.AutoUploadSub2API,
			label:      "Sub2API",
			kind:       uploader.UploadKindSub2API,
			serviceIDs: append([]int(nil), req.StartRequest.Sub2APIServiceIDs...),
			list:       d.configs.ListSub2APIServiceConfigs,
		},
		{
			enabled:    req.StartRequest.AutoUploadTM,
			label:      "TM",
			kind:       uploader.UploadKindTM,
			serviceIDs: append([]int(nil), req.StartRequest.TMServiceIDs...),
			list:       d.configs.ListTMServiceConfigs,
		},
	}

	for _, plan := range plans {
		if !plan.enabled {
			continue
		}
		if account.Email == "" {
			safeAutoUploadLog(logf, "warning", fmt.Sprintf("[%s] 账号邮箱为空，跳过上传", plan.label))
			continue
		}
		if account.AccessToken == "" {
			safeAutoUploadLog(logf, "info", fmt.Sprintf("[%s] 账号缺少 access_token，跳过上传", plan.label))
			continue
		}

		configs, err := plan.list(ctx, plan.serviceIDs)
		if err != nil {
			safeAutoUploadLog(logf, "warning", fmt.Sprintf("[%s] 加载服务配置失败: %v", plan.label, err))
			continue
		}
		if len(configs) == 0 {
			safeAutoUploadLog(logf, "info", fmt.Sprintf("[%s] 无可用服务，跳过上传", plan.label))
			continue
		}

		sender, err := d.senderFactory(plan.kind)
		if err != nil {
			safeAutoUploadLog(logf, "warning", fmt.Sprintf("[%s] 初始化上传器失败: %v", plan.label, err))
			continue
		}

		for _, service := range configs {
			results, err := sender.Send(ctx, uploader.SendRequest{
				Service:  service,
				Accounts: []uploader.UploadAccount{account},
			})
			if err != nil {
				safeAutoUploadLog(logf, "warning", fmt.Sprintf("[%s] 上传失败(%s): %v", plan.label, autoUploadServiceName(service), err))
				continue
			}
			if len(results) == 0 {
				safeAutoUploadLog(logf, "info", fmt.Sprintf("[%s] 未返回上传结果(%s)", plan.label, autoUploadServiceName(service)))
				continue
			}
			for _, result := range results {
				level := "info"
				status := "成功"
				if !result.Success {
					level = "warning"
					status = "失败"
				}
				message := strings.TrimSpace(result.Message)
				if message == "" {
					message = "上传完成"
				}
				safeAutoUploadLog(logf, level, fmt.Sprintf("[%s] %s(%s): %s", plan.label, status, autoUploadServiceName(service), message))
				if shouldWriteAutoUploadSuccess(plan.kind, result, req.Account.Email) {
					markAutoUploadSuccess(&dispatchResult.AccountUpdate, req.Account, plan.kind, time.Now().UTC())
				}
			}
		}
	}

	return dispatchResult, nil
}

func safeAutoUploadLog(logf func(level string, message string) error, level string, message string) {
	if logf == nil {
		return
	}
	_ = logf(level, message)
}

func autoUploadServiceName(service uploader.ServiceConfig) string {
	name := strings.TrimSpace(service.Name)
	if name != "" {
		return name
	}
	if service.ID > 0 {
		return fmt.Sprintf("service-%d", service.ID)
	}
	return "unknown-service"
}

func toUploadAccount(account accounts.Account) uploader.UploadAccount {
	return uploader.UploadAccount{
		ID:           account.ID,
		Email:        account.Email,
		AccessToken:  account.AccessToken,
		RefreshToken: account.RefreshToken,
		SessionToken: account.SessionToken,
		ClientID:     account.ClientID,
		AccountID:    account.AccountID,
		WorkspaceID:  account.WorkspaceID,
		IDToken:      account.IDToken,
	}
}

func shouldWriteAutoUploadSuccess(kind uploader.UploadKind, result uploader.UploadResult, accountEmail string) bool {
	if !result.Success {
		return false
	}
	switch kind {
	case uploader.UploadKindCPA, uploader.UploadKindSub2API:
	default:
		return false
	}

	resultEmail := strings.TrimSpace(result.AccountEmail)
	accountEmail = strings.TrimSpace(accountEmail)
	if resultEmail == "" || accountEmail == "" {
		return resultEmail == accountEmail
	}
	return strings.EqualFold(resultEmail, accountEmail)
}

func markAutoUploadSuccess(update *accounts.UpsertAccountRequest, account accounts.Account, kind uploader.UploadKind, now time.Time) {
	if update == nil {
		return
	}
	if strings.TrimSpace(update.Email) == "" {
		*update = buildAutoUploadAccountUpdate(account)
	}

	switch kind {
	case uploader.UploadKindCPA:
		update.CPAUploaded = boolPointer(true)
		update.CPAUploadedAt = cloneAutoUploadTime(now)
	case uploader.UploadKindSub2API:
		update.Sub2APIUploaded = boolPointer(true)
		update.Sub2APIUploadedAt = cloneAutoUploadTime(now)
	}
}

func buildAutoUploadAccountUpdate(account accounts.Account) accounts.UpsertAccountRequest {
	return accounts.UpsertAccountRequest{
		Email:          account.Email,
		Password:       account.Password,
		ClientID:       account.ClientID,
		SessionToken:   account.SessionToken,
		EmailService:   account.EmailService,
		EmailServiceID: account.EmailServiceID,
		AccountID:      account.AccountID,
		WorkspaceID:    account.WorkspaceID,
		AccessToken:    account.AccessToken,
		RefreshToken:   account.RefreshToken,
		IDToken:        account.IDToken,
		Cookies:        account.Cookies,
		ProxyUsed:      account.ProxyUsed,
		ExtraData:      cloneAutoUploadExtraData(account.ExtraData),
		Status:         account.Status,
		Source:         account.Source,
		RegisteredAt:   cloneAutoUploadTimePtr(account.RegisteredAt),
	}
}

func cloneAutoUploadExtraData(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneAutoUploadTime(now time.Time) *time.Time {
	cloned := now
	return &cloned
}

func cloneAutoUploadTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func boolPointer(value bool) *bool {
	return &value
}
