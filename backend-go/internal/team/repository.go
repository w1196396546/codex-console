package team

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("team: not found")

type Repository interface {
	ListTeams(ctx context.Context, req ListTeamsRequest) ([]TeamRecord, int, error)
	GetTeam(ctx context.Context, teamID int64) (TeamRecord, error)
	GetAccount(ctx context.Context, accountID int64) (AccountRecord, error)
	ListAccountsByIDs(ctx context.Context, accountIDs []int64) (map[int64]AccountRecord, error)
	ListMembershipsByTeam(ctx context.Context, teamID int64) ([]TeamMembershipRecord, error)
	GetMembership(ctx context.Context, membershipID int64) (TeamMembershipRecord, error)
	SaveMembership(ctx context.Context, membership TeamMembershipRecord) (TeamMembershipRecord, error)
	SaveTeam(ctx context.Context, team TeamRecord) (TeamRecord, error)
	ListTasks(ctx context.Context, req ListTasksRequest) ([]TeamTaskRecord, error)
	GetTaskByUUID(ctx context.Context, taskUUID string) (TeamTaskRecord, error)
	ListTaskItems(ctx context.Context, taskID int64) ([]TeamTaskItemRecord, error)
}
