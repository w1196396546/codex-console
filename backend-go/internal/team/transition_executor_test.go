package team

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

func TestTeamTransitionExecutorDiscoveryUsesRequestedOwners(t *testing.T) {
	executor := NewTransitionTaskExecutor(nil, TransitionExecutorHooks{
		Discover: func(_ context.Context, ownerAccountIDs []int64) (TaskExecutionResult, error) {
			if len(ownerAccountIDs) != 2 || ownerAccountIDs[0] != 7 || ownerAccountIDs[1] != 9 {
				t.Fatalf("expected normalized owner ids, got %#v", ownerAccountIDs)
			}
			return TaskExecutionResult{
				Status: jobs.StatusCompleted,
				Summary: map[string]any{
					"accounts_scanned": 2,
					"teams_found":      1,
					"teams_persisted":  1,
				},
				Logs: []string{"发现 1 个 Team"},
			}, nil
		},
	})

	result, err := executor.Execute(context.Background(), TaskExecutionRequest{
		TaskUUID:       "task-discovery",
		TaskType:       "discover_owner_teams",
		OwnerAccountID: ptrInt64(7),
		RequestPayload: map[string]any{"ids": []int64{7, 9, 7}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed discovery status, got %#v", result.Status)
	}
	if got, _ := result.Summary["teams_persisted"].(int); got != 1 {
		t.Fatalf("expected teams_persisted=1, got %#v", result.Summary)
	}
}

func TestTeamTransitionExecutorInvitePersistsFailedExecutionResult(t *testing.T) {
	boom := errors.New("invite transport failed")
	executor := NewTransitionTaskExecutor(nil, TransitionExecutorHooks{
		Invite: func(_ context.Context, teamID int64, taskType string, payload map[string]any) (TaskExecutionResult, error) {
			if teamID != 101 {
				t.Fatalf("expected team_id=101, got %d", teamID)
			}
			if taskType != "invite_emails" {
				t.Fatalf("expected invite_emails task type, got %#v", taskType)
			}
			if _, ok := payload["emails"]; !ok {
				t.Fatalf("expected invite payload emails, got %#v", payload)
			}
			return TaskExecutionResult{}, boom
		},
	})

	_, err := executor.Execute(context.Background(), TaskExecutionRequest{
		TaskUUID: "task-invite",
		TaskType: "invite_emails",
		TeamID:   ptrInt64(101),
		RequestPayload: map[string]any{
			"emails": []string{"invitee@example.com"},
		},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected invite error to propagate, got %v", err)
	}
}

func TestTeamTransitionGatewayAppliesDefaultTimeoutWithoutCallerDeadline(t *testing.T) {
	previousTimeout := transitionHTTPRequestTimeout
	transitionHTTPRequestTimeout = 200 * time.Millisecond
	t.Cleanup(func() {
		transitionHTTPRequestTimeout = previousTimeout
	})

	var observedDeadline time.Time
	var sawDeadline bool
	startedAt := time.Now()
	installTransitionHTTPStub(t, func(req *http.Request) (*http.Response, error) {
		observedDeadline, sawDeadline = req.Context().Deadline()
		return jsonHTTPResponse(http.StatusOK, map[string]any{"ok": true})
	})

	gateway := NewTransitionMembershipGateway(nil)
	err := gateway.RevokeInvite(context.Background(), MembershipGatewayRevokeInviteParams{
		TeamUpstreamAccountID: "acct_101",
		OwnerAccessToken:      "owner-token",
		MemberEmail:           "member@example.com",
	})
	if err != nil {
		t.Fatalf("RevokeInvite returned error: %v", err)
	}
	if !sawDeadline {
		t.Fatalf("expected default transition timeout to attach a deadline")
	}
	timeoutWindow := observedDeadline.Sub(startedAt)
	if timeoutWindow < 150*time.Millisecond || timeoutWindow > 250*time.Millisecond {
		t.Fatalf("expected default deadline near %s, got %s", transitionHTTPRequestTimeout, timeoutWindow)
	}
}

func TestTeamTransitionGatewayHonorsCallerDeadline(t *testing.T) {
	previousTimeout := transitionHTTPRequestTimeout
	transitionHTTPRequestTimeout = 250 * time.Millisecond
	t.Cleanup(func() {
		transitionHTTPRequestTimeout = previousTimeout
	})

	callerCtx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	expectedDeadline, ok := callerCtx.Deadline()
	if !ok {
		t.Fatal("expected caller context deadline")
	}

	var observedDeadline time.Time
	var sawDeadline bool
	installTransitionHTTPStub(t, func(req *http.Request) (*http.Response, error) {
		observedDeadline, sawDeadline = req.Context().Deadline()
		return jsonHTTPResponse(http.StatusOK, map[string]any{"ok": true})
	})

	gateway := NewTransitionMembershipGateway(nil)
	err := gateway.RemoveMember(callerCtx, MembershipGatewayRemoveMemberParams{
		TeamUpstreamAccountID: "acct_101",
		OwnerAccessToken:      "owner-token",
		UpstreamUserID:        "user-9",
	})
	if err != nil {
		t.Fatalf("RemoveMember returned error: %v", err)
	}
	if !sawDeadline {
		t.Fatalf("expected caller deadline to be preserved")
	}
	if delta := observedDeadline.Sub(expectedDeadline); delta < -15*time.Millisecond || delta > 15*time.Millisecond {
		t.Fatalf("expected observed deadline %s to stay near caller deadline %s", observedDeadline, expectedDeadline)
	}
}

func TestTeamTransitionExecutorDefaultDiscoveryPersistsUpstreamTeams(t *testing.T) {
	installTransitionHTTPStub(t, func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("expected discovery GET request, got %s", req.Method)
		}
		if req.URL.Path != "/backend-api/accounts/check/v4-2023-04-27" {
			t.Fatalf("unexpected discovery path: %s", req.URL.Path)
		}
		if auth := req.Header.Get("Authorization"); auth != "Bearer owner-token" {
			t.Fatalf("unexpected discovery authorization header: %q", auth)
		}
		return jsonHTTPResponse(http.StatusOK, map[string]any{
			"accounts": map[string]any{
				"acct_101": map[string]any{
					"account": map[string]any{
						"plan_type":         "team",
						"name":              "Alpha Team",
						"account_user_role": "account-owner",
					},
					"entitlement": map[string]any{
						"subscription_plan": "chatgpt-team",
						"expires_at":        "2026-12-31T00:00:00Z",
					},
				},
				"acct_admin": map[string]any{
					"account": map[string]any{
						"plan_type":         "team",
						"name":              "Admin Team",
						"account_user_role": "admin",
					},
					"entitlement": map[string]any{
						"subscription_plan": "chatgpt-team",
					},
				},
			},
		})
	})

	repo := &fakeRepository{
		accounts: map[int64]AccountRecord{
			7: {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
		},
		teams:       map[int64]TeamRecord{},
		memberships: map[int64]TeamMembershipRecord{},
		tasks:       map[string]TeamTaskRecord{},
		taskItems:   map[int64][]TeamTaskItemRecord{},
	}
	executor := NewTransitionTaskExecutor(repo, TransitionExecutorHooks{})

	discoveryResult, err := executor.Execute(context.Background(), TaskExecutionRequest{
		TaskUUID:       "task-discovery-default",
		TaskType:       "discover_owner_teams",
		OwnerAccountID: ptrInt64(7),
		RequestPayload: map[string]any{"ids": []int64{7, 7}},
	})
	if err != nil {
		t.Fatalf("default discovery returned error: %v", err)
	}
	if discoveryResult.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed discovery status, got %#v", discoveryResult.Status)
	}
	if got, _ := discoveryResult.Summary["teams_found"].(int); got != 2 {
		t.Fatalf("expected teams_found=2, got %#v", discoveryResult.Summary)
	}
	if got, _ := discoveryResult.Summary["teams_persisted"].(int); got != 1 {
		t.Fatalf("expected teams_persisted=1, got %#v", discoveryResult.Summary)
	}
	if len(repo.teams) != 1 {
		t.Fatalf("expected discovery to persist one owner team, got %#v", repo.teams)
	}
	persisted := repo.teams[1]
	if persisted.OwnerAccountID != 7 || persisted.UpstreamAccountID != "acct_101" {
		t.Fatalf("unexpected persisted discovery record: %#v", persisted)
	}
	if persisted.TeamName != "Alpha Team" || persisted.AccountRoleSnapshot != "account-owner" {
		t.Fatalf("expected persisted team metadata, got %#v", persisted)
	}
	if persisted.SubscriptionPlan != "chatgpt-team" || persisted.SyncStatus != "synced" || persisted.LastSyncAt == nil {
		t.Fatalf("expected discovery to mark sync metadata, got %#v", persisted)
	}
}

func TestTeamTransitionExecutorDefaultSyncPersistsUpstreamMembershipSnapshot(t *testing.T) {
	now := time.Date(2026, 4, 6, 11, 0, 0, 0, time.UTC)
	installTransitionHTTPStub(t, func(req *http.Request) (*http.Response, error) {
		if auth := req.Header.Get("Authorization"); auth != "Bearer owner-token" {
			t.Fatalf("unexpected sync authorization header: %q", auth)
		}
		switch req.URL.Path {
		case "/backend-api/accounts/check/v4-2023-04-27":
			return jsonHTTPResponse(http.StatusOK, map[string]any{"accounts": map[string]any{}})
		case "/backend-api/accounts/acct_101/users":
			return jsonHTTPResponse(http.StatusOK, map[string]any{
				"items": []any{
					map[string]any{
						"id":           "user-9",
						"email":        " Member@Example.com ",
						"role":         "member",
						"created_time": "2026-04-03T00:00:00Z",
					},
				},
				"total": 1,
				"limit": 100,
			})
		case "/backend-api/accounts/acct_101/invites":
			return jsonHTTPResponse(http.StatusOK, map[string]any{
				"items": []any{
					map[string]any{
						"email_address": "invitee@example.com",
						"role":          "member",
						"created_time":  "2026-04-04T00:00:00Z",
					},
				},
			})
		default:
			t.Fatalf("unexpected sync path: %s", req.URL.Path)
			return nil, nil
		}
	})

	repo := &fakeRepository{
		accounts: map[int64]AccountRecord{
			7: {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
			9: {ID: 9, Email: "member@example.com", Status: "active"},
		},
		teams: map[int64]TeamRecord{
			101: {
				ID:                101,
				OwnerAccountID:    7,
				UpstreamAccountID: "acct_101",
				TeamName:          "Alpha Team",
				Status:            "active",
				MaxMembers:        intPtr(5),
				UpdatedAt:         now,
			},
		},
		memberships: map[int64]TeamMembershipRecord{
			201: {ID: 201, TeamID: 101, MemberEmail: "stale@example.com", MembershipStatus: "already_member", Source: "sync"},
		},
		tasks:     map[string]TeamTaskRecord{},
		taskItems: map[int64][]TeamTaskItemRecord{},
	}
	executor := NewTransitionTaskExecutor(repo, TransitionExecutorHooks{})

	syncResult, err := executor.Execute(context.Background(), TaskExecutionRequest{
		TaskUUID: "task-sync-default",
		TaskType: "sync_team",
		TeamID:   ptrInt64(101),
		RequestPayload: map[string]any{
			"team_id": 101,
		},
	})
	if err != nil {
		t.Fatalf("default sync returned error: %v", err)
	}
	if syncResult.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed sync status, got %#v", syncResult.Status)
	}
	if got, _ := syncResult.Summary["processed_count"].(int); got != 1 {
		t.Fatalf("expected processed_count=1, got %#v", syncResult.Summary)
	}
	if got, _ := syncResult.Summary["failed_count"].(int); got != 0 {
		t.Fatalf("expected failed_count=0, got %#v", syncResult.Summary)
	}
	if repo.teams[101].SyncStatus != "synced" || repo.teams[101].LastSyncAt == nil {
		t.Fatalf("expected sync transition to persist team sync markers, got %#v", repo.teams[101])
	}
	if repo.teams[101].CurrentMembers != 2 || repo.teams[101].SeatsAvailable == nil || *repo.teams[101].SeatsAvailable != 3 {
		t.Fatalf("expected sync transition to refresh team counts, got %#v", repo.teams[101])
	}
	member := findMembershipByEmail(repo.memberships, "member@example.com")
	if member.MembershipStatus != "joined" || member.UpstreamUserID != "user-9" || member.LocalAccountID == nil || *member.LocalAccountID != 9 {
		t.Fatalf("expected remote member to persist joined membership, got %#v", member)
	}
	invite := findMembershipByEmail(repo.memberships, "invitee@example.com")
	if invite.MembershipStatus != "invited" {
		t.Fatalf("expected remote invite to persist invited membership, got %#v", invite)
	}
	stale := findMembershipByEmail(repo.memberships, "stale@example.com")
	if stale.MembershipStatus != "removed" || stale.RemovedAt == nil {
		t.Fatalf("expected missing prior member to be marked removed, got %#v", stale)
	}
}

func TestTeamTransitionExecutorDefaultSyncTimesOutStalledUpstreamTransport(t *testing.T) {
	previousTimeout := transitionHTTPRequestTimeout
	transitionHTTPRequestTimeout = 40 * time.Millisecond
	t.Cleanup(func() {
		transitionHTTPRequestTimeout = previousTimeout
	})

	installTransitionHTTPStub(t, func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})

	repo := &fakeRepository{
		accounts: map[int64]AccountRecord{
			7: {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
		},
		teams: map[int64]TeamRecord{
			101: {ID: 101, OwnerAccountID: 7, UpstreamAccountID: "acct_101", TeamName: "Alpha Team", Status: "active"},
		},
		memberships: map[int64]TeamMembershipRecord{},
		tasks:       map[string]TeamTaskRecord{},
		taskItems:   map[int64][]TeamTaskItemRecord{},
	}
	executor := NewTransitionTaskExecutor(repo, TransitionExecutorHooks{})

	startedAt := time.Now()
	_, err := executor.Execute(context.Background(), TaskExecutionRequest{
		TaskUUID: "task-sync-timeout",
		TaskType: "sync_team",
		TeamID:   ptrInt64(101),
		RequestPayload: map[string]any{
			"team_id": 101,
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded error, got %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("expected stalled sync to fail within bounded timeout, took %s", elapsed)
	}
}

func TestTeamTransitionExecutorDefaultSyncBatchPersistsEveryRequestedTeam(t *testing.T) {
	installTransitionHTTPStub(t, func(req *http.Request) (*http.Response, error) {
		if auth := req.Header.Get("Authorization"); auth != "Bearer owner-token" {
			t.Fatalf("unexpected sync authorization header: %q", auth)
		}
		switch req.URL.Path {
		case "/backend-api/accounts/acct_101/users":
			return jsonHTTPResponse(http.StatusOK, map[string]any{
				"items": []any{
					map[string]any{
						"id":           "user-9",
						"email":        "alpha.member@example.com",
						"role":         "member",
						"created_time": "2026-04-03T00:00:00Z",
					},
				},
				"total": 1,
				"limit": 100,
			})
		case "/backend-api/accounts/acct_101/invites":
			return jsonHTTPResponse(http.StatusOK, map[string]any{"items": []any{}})
		case "/backend-api/accounts/acct_202/users":
			return jsonHTTPResponse(http.StatusOK, map[string]any{
				"items": []any{
					map[string]any{
						"id":           "user-10",
						"email":        "beta.member@example.com",
						"role":         "member",
						"created_time": "2026-04-04T00:00:00Z",
					},
				},
				"total": 1,
				"limit": 100,
			})
		case "/backend-api/accounts/acct_202/invites":
			return jsonHTTPResponse(http.StatusOK, map[string]any{"items": []any{}})
		default:
			t.Fatalf("unexpected sync-batch path: %s", req.URL.Path)
			return nil, nil
		}
	})

	repo := &fakeRepository{
		accounts: map[int64]AccountRecord{
			7:  {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
			9:  {ID: 9, Email: "alpha.member@example.com", Status: "active"},
			10: {ID: 10, Email: "beta.member@example.com", Status: "active"},
		},
		teams: map[int64]TeamRecord{
			101: {ID: 101, OwnerAccountID: 7, UpstreamAccountID: "acct_101", TeamName: "Alpha Team", Status: "active", MaxMembers: intPtr(5)},
			202: {ID: 202, OwnerAccountID: 7, UpstreamAccountID: "acct_202", TeamName: "Beta Team", Status: "active", MaxMembers: intPtr(5)},
		},
		memberships: map[int64]TeamMembershipRecord{},
		tasks:       map[string]TeamTaskRecord{},
		taskItems:   map[int64][]TeamTaskItemRecord{},
	}
	executor := NewTransitionTaskExecutor(repo, TransitionExecutorHooks{})

	result, err := executor.Execute(context.Background(), TaskExecutionRequest{
		TaskUUID: "task-sync-batch-default",
		TaskType: "sync_all_teams",
		TeamID:   ptrInt64(101),
		RequestPayload: map[string]any{
			"ids": []int64{101, 202},
		},
	})
	if err != nil {
		t.Fatalf("default sync-batch returned error: %v", err)
	}
	if result.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed sync-batch status, got %#v", result.Status)
	}
	if got, _ := result.Summary["requested_count"].(int); got != 2 {
		t.Fatalf("expected requested_count=2, got %#v", result.Summary)
	}
	if got, _ := result.Summary["processed_count"].(int); got != 2 {
		t.Fatalf("expected processed_count=2, got %#v", result.Summary)
	}
	alpha := findMembershipByEmail(repo.memberships, "alpha.member@example.com")
	if alpha.TeamID != 101 || alpha.LocalAccountID == nil || *alpha.LocalAccountID != 9 || alpha.MembershipStatus != "joined" {
		t.Fatalf("expected first team membership to persist, got %#v", alpha)
	}
	beta := findMembershipByEmail(repo.memberships, "beta.member@example.com")
	if beta.TeamID != 202 || beta.LocalAccountID == nil || *beta.LocalAccountID != 10 || beta.MembershipStatus != "joined" {
		t.Fatalf("expected second team membership to persist, got %#v", beta)
	}
	if repo.teams[101].SyncStatus != "synced" || repo.teams[101].LastSyncAt == nil {
		t.Fatalf("expected first team sync markers, got %#v", repo.teams[101])
	}
	if repo.teams[202].SyncStatus != "synced" || repo.teams[202].LastSyncAt == nil {
		t.Fatalf("expected second team sync markers, got %#v", repo.teams[202])
	}
}

func TestTeamTransitionExecutorDefaultInvitePersistsMembershipResults(t *testing.T) {
	installTransitionHTTPStub(t, func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected invite POST request, got %s", req.Method)
		}
		if req.URL.Path != "/backend-api/accounts/acct_101/invites" {
			t.Fatalf("unexpected invite path: %s", req.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode invite request body: %v", err)
		}
		if got := payload["email_addresses"]; len(got.([]any)) != 1 || got.([]any)[0] != "invitee@example.com" {
			t.Fatalf("unexpected invite payload: %#v", payload)
		}
		return jsonHTTPResponse(http.StatusOK, map[string]any{"ok": true})
	})

	repo := &fakeRepository{
		accounts: map[int64]AccountRecord{
			7:  {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
			21: {ID: 21, Email: "Invitee@example.com", Status: "active"},
		},
		teams: map[int64]TeamRecord{
			101: {ID: 101, OwnerAccountID: 7, UpstreamAccountID: "acct_101", TeamName: "Alpha Team", Status: "active", CurrentMembers: 1, MaxMembers: intPtr(5), SeatsAvailable: intPtr(4)},
		},
		memberships: map[int64]TeamMembershipRecord{
			201: {ID: 201, TeamID: 101, MemberEmail: "member@example.com", MembershipStatus: "joined", Source: "sync"},
		},
		tasks:     map[string]TeamTaskRecord{},
		taskItems: map[int64][]TeamTaskItemRecord{},
	}
	executor := NewTransitionTaskExecutor(repo, TransitionExecutorHooks{})

	result, err := executor.Execute(context.Background(), TaskExecutionRequest{
		TaskUUID: "task-invite-default",
		TaskType: "invite_accounts",
		TeamID:   ptrInt64(101),
		RequestPayload: map[string]any{
			"ids": []int64{21},
		},
	})
	if err != nil {
		t.Fatalf("default invite returned error: %v", err)
	}
	if result.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed invite status, got %#v", result.Status)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected one transition invite item, got %#v", result.Items)
	}
	if result.Items[0].TargetEmail != "invitee@example.com" || lookupString(result.Items[0].After, "membership_status") != "invited" {
		t.Fatalf("expected invited transition item, got %#v", result.Items[0])
	}
	membership := findMembershipByEmail(repo.memberships, "invitee@example.com")
	if membership.LocalAccountID == nil || *membership.LocalAccountID != 21 || membership.MembershipStatus != "invited" || membership.Source != "invite" {
		t.Fatalf("expected invite to persist membership state, got %#v", membership)
	}
	if repo.teams[101].CurrentMembers != 2 || repo.teams[101].SeatsAvailable == nil || *repo.teams[101].SeatsAvailable != 3 {
		t.Fatalf("expected invite to refresh team summary, got %#v", repo.teams[101])
	}
	if result.Summary["success"] != true {
		t.Fatalf("expected invite success summary, got %#v", result.Summary)
	}
}

func (r *fakeRepository) UpsertTeam(_ context.Context, team TeamRecord) (TeamRecord, error) {
	for id, existing := range r.teams {
		if existing.OwnerAccountID == team.OwnerAccountID && normalizeEmail(existing.UpstreamAccountID) == normalizeEmail(team.UpstreamAccountID) {
			team.ID = id
			if team.CreatedAt.IsZero() {
				team.CreatedAt = existing.CreatedAt
			}
			if team.UpdatedAt.IsZero() {
				team.UpdatedAt = time.Now().UTC()
			}
			r.teams[id] = team
			return team, nil
		}
	}
	if team.ID == 0 {
		team.ID = nextTeamRecordID(r.teams)
	}
	if team.CreatedAt.IsZero() {
		team.CreatedAt = time.Now().UTC()
	}
	if team.UpdatedAt.IsZero() {
		team.UpdatedAt = team.CreatedAt
	}
	r.teams[team.ID] = team
	return team, nil
}

func (r *fakeRepository) UpsertMembership(_ context.Context, membership TeamMembershipRecord) (TeamMembershipRecord, error) {
	for id, existing := range r.memberships {
		if existing.TeamID == membership.TeamID && normalizeEmail(existing.MemberEmail) == normalizeEmail(membership.MemberEmail) {
			membership.ID = id
			if membership.CreatedAt.IsZero() {
				membership.CreatedAt = existing.CreatedAt
			}
			if membership.UpdatedAt.IsZero() {
				membership.UpdatedAt = time.Now().UTC()
			}
			r.memberships[id] = membership
			return membership, nil
		}
	}
	if membership.ID == 0 {
		membership.ID = nextMembershipRecordID(r.memberships)
	}
	if membership.CreatedAt.IsZero() {
		membership.CreatedAt = time.Now().UTC()
	}
	if membership.UpdatedAt.IsZero() {
		membership.UpdatedAt = membership.CreatedAt
	}
	r.memberships[membership.ID] = membership
	return membership, nil
}

func (r *fakeRepository) ListAccountsByEmails(_ context.Context, emails []string) (map[string]AccountRecord, error) {
	result := make(map[string]AccountRecord, len(emails))
	for _, email := range emails {
		normalized := normalizeEmail(email)
		for _, account := range r.accounts {
			if normalizeEmail(account.Email) == normalized {
				result[normalized] = account
				break
			}
		}
	}
	return result, nil
}

func installTransitionHTTPStub(t *testing.T, fn func(req *http.Request) (*http.Response, error)) {
	t.Helper()

	previous := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(fn)
	t.Cleanup(func() {
		http.DefaultTransport = previous
	})
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonHTTPResponse(status int, payload map[string]any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func findMembershipByEmail(memberships map[int64]TeamMembershipRecord, email string) TeamMembershipRecord {
	normalized := normalizeEmail(email)
	for _, membership := range memberships {
		if normalizeEmail(membership.MemberEmail) == normalized {
			return membership
		}
	}
	return TeamMembershipRecord{}
}

func nextTeamRecordID(records map[int64]TeamRecord) int64 {
	var maxID int64
	for id := range records {
		if id > maxID {
			maxID = id
		}
	}
	return maxID + 1
}

func nextMembershipRecordID(records map[int64]TeamMembershipRecord) int64 {
	var maxID int64
	for id := range records {
		if id > maxID {
			maxID = id
		}
	}
	return maxID + 1
}
