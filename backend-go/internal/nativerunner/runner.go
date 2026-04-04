package nativerunner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/mail"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
)

type Flow interface {
	Run(ctx context.Context, input FlowRequest) (registration.NativeRunnerResult, error)
}

type FlowFunc func(ctx context.Context, input FlowRequest) (registration.NativeRunnerResult, error)

func (f FlowFunc) Run(ctx context.Context, input FlowRequest) (registration.NativeRunnerResult, error) {
	return f(ctx, input)
}

type FlowRequest struct {
	RunnerRequest registration.RunnerRequest
	MailProvider  mail.Provider
	Inbox         mail.Inbox
	Logf          func(level string, message string) error
	runtime       *flowRuntime
}

type ProviderFactory interface {
	NewProvider(serviceType string, config map[string]any) (mail.Provider, error)
}

type providerFactoryFunc func(serviceType string, config map[string]any) (mail.Provider, error)

func (f providerFactoryFunc) NewProvider(serviceType string, config map[string]any) (mail.Provider, error) {
	return f(serviceType, config)
}

type Options struct {
	Flow            Flow
	ProviderFactory ProviderFactory
}

type Runner struct {
	flow            Flow
	providerFactory ProviderFactory
}

var _ registration.NativeRunner = (*Runner)(nil)

func New(options Options) *Runner {
	factory := options.ProviderFactory
	if factory == nil {
		factory = providerFactoryFunc(mail.NewProvider)
	}

	return &Runner{
		flow:            options.Flow,
		providerFactory: factory,
	}
}

func (r *Runner) RunNative(ctx context.Context, req registration.RunnerRequest, logf func(level string, message string) error) (registration.NativeRunnerResult, error) {
	if r == nil {
		return registration.NativeRunnerResult{}, errors.New("native runner is required")
	}
	if r.flow == nil {
		return registration.NativeRunnerResult{}, errors.New("native flow is required")
	}
	if r.providerFactory == nil {
		return registration.NativeRunnerResult{}, errors.New("native provider factory is required")
	}
	if logf == nil {
		logf = func(string, string) error { return nil }
	}

	serviceType, config := resolveMailProvider(req)
	provider, err := r.providerFactory.NewProvider(serviceType, config)
	if err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("create native mail provider: %w", err)
	}

	inbox, err := provider.Create(ctx)
	if err != nil {
		return registration.NativeRunnerResult{}, err
	}

	return r.flow.Run(ctx, FlowRequest{
		RunnerRequest: req,
		MailProvider:  provider,
		Inbox:         inbox,
		Logf:          logf,
	})
}

func resolveMailProvider(req registration.RunnerRequest) (string, map[string]any) {
	serviceType := strings.TrimSpace(req.Plan.EmailService.Type)
	if serviceType == "" {
		serviceType = strings.TrimSpace(req.StartRequest.EmailServiceType)
	}
	if serviceType == "" {
		serviceType = "tempmail"
	}

	config := req.Plan.EmailService.Config
	if len(config) == 0 {
		config = req.StartRequest.EmailServiceConfig
	}

	return serviceType, config
}
