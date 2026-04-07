package main

import (
	"github.com/dou-jiang/codex-console/backend-go/internal/team"
)

type apiTeamRuntime struct {
	Service           *team.Service
	TaskService       *team.TaskService
	MembershipGateway team.MembershipGateway
	Executor          team.TaskExecutor
}

type apiTeamRuntimeOptions struct {
	membershipTransport team.TransitionTransport
	executorHooks       team.TransitionExecutorHooks
}

type apiTeamRuntimeOption func(*apiTeamRuntimeOptions)

func withAPITeamMembershipTransport(transport team.TransitionTransport) apiTeamRuntimeOption {
	return func(options *apiTeamRuntimeOptions) {
		options.membershipTransport = transport
	}
}

func withAPITeamExecutorHooks(hooks team.TransitionExecutorHooks) apiTeamRuntimeOption {
	return func(options *apiTeamRuntimeOptions) {
		options.executorHooks = hooks
	}
}

func newAPITeamServices(repository team.TaskRepository, runtime team.TaskRuntime, opts ...apiTeamRuntimeOption) apiTeamRuntime {
	options := apiTeamRuntimeOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	gateway := team.NewTransitionMembershipGateway(options.membershipTransport)
	service := team.NewService(repository, gateway)
	executor := team.NewTransitionTaskExecutor(repository, options.executorHooks)
	taskService := team.NewTaskService(repository, service, runtime, executor)

	return apiTeamRuntime{
		Service:           service,
		TaskService:       taskService,
		MembershipGateway: gateway,
		Executor:          executor,
	}
}
