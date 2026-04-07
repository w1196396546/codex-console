package team

import (
	"context"
	"errors"
	"testing"
)

func TestTeamTransitionGatewayRevokeInviteUsesUpstreamDelete(t *testing.T) {
	var got TransitionRequest

	gateway := NewTransitionMembershipGateway(func(_ context.Context, req TransitionRequest) error {
		got = req
		return nil
	})

	err := gateway.RevokeInvite(context.Background(), MembershipGatewayRevokeInviteParams{
		TeamUpstreamAccountID: "acct_101",
		OwnerAccessToken:      "owner-token",
		MemberEmail:           "invitee@example.com",
	})
	if err != nil {
		t.Fatalf("RevokeInvite returned error: %v", err)
	}

	if got.Method != "DELETE" {
		t.Fatalf("expected DELETE method, got %#v", got.Method)
	}
	if got.Path != "/backend-api/accounts/acct_101/invites" {
		t.Fatalf("expected invites path, got %#v", got.Path)
	}
	if got.AccessToken != "owner-token" {
		t.Fatalf("expected owner access token to be forwarded, got %#v", got.AccessToken)
	}
	if email, _ := got.JSON["email_address"].(string); email != "invitee@example.com" {
		t.Fatalf("expected email_address payload, got %#v", got.JSON)
	}
}

func TestTeamTransitionGatewayRemoveMemberPropagatesUpstreamError(t *testing.T) {
	boom := errors.New("upstream remove failed")
	gateway := NewTransitionMembershipGateway(func(_ context.Context, req TransitionRequest) error {
		if req.Path != "/backend-api/accounts/acct_101/users/user_123" {
			t.Fatalf("unexpected path: %#v", req.Path)
		}
		return boom
	})

	err := gateway.RemoveMember(context.Background(), MembershipGatewayRemoveMemberParams{
		TeamUpstreamAccountID: "acct_101",
		OwnerAccessToken:      "owner-token",
		UpstreamUserID:        "user_123",
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected upstream error to propagate, got %v", err)
	}
}
