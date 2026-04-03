"""Team 成员关系动作服务。"""

from __future__ import annotations

from datetime import datetime

from sqlalchemy.orm import Session

from src.database.models import Account
from src.database.team_models import Team, TeamMembership

from .client import TeamClient, TeamClientError
from .relation import bind_local_account
from .utils import count_current_members, seats_available as calculate_seats_available


def _utcnow() -> datetime:
    return datetime.utcnow()


def _build_result(
    *,
    success: bool,
    message: str,
    team_id: int | None,
    membership_id: int,
    next_status: str | None,
    refresh_required: bool,
    error_code: int | None = None,
    **extra,
) -> dict[str, object]:
    result: dict[str, object] = {
        "success": success,
        "message": message,
        "team_id": team_id,
        "membership_id": membership_id,
        "next_status": next_status,
        "refresh_required": refresh_required,
    }
    if error_code is not None:
        result["error_code"] = error_code
    result.update(extra)
    return result


def _load_membership(session: Session, membership_id: int) -> TeamMembership | None:
    return session.query(TeamMembership).filter(TeamMembership.id == membership_id).first()


def _load_team(session: Session, team_id: int) -> Team | None:
    return session.query(Team).filter(Team.id == team_id).first()


def _load_owner_account(session: Session, owner_account_id: int) -> Account | None:
    return session.query(Account).filter(Account.id == owner_account_id).first()


def _apply_removed_status(membership: TeamMembership, *, next_status: str) -> None:
    membership.membership_status = next_status
    membership.removed_at = _utcnow()
    membership.sync_error = None


def _recalculate_team_aggregate(session: Session, team: Team) -> None:
    memberships = (
        session.query(TeamMembership)
        .filter(TeamMembership.team_id == team.id)
        .all()
    )
    active_member_count = count_current_members(
        {
            "member_email": membership.member_email,
            "membership_status": membership.membership_status,
        }
        for membership in memberships
    )
    team.current_members = active_member_count
    team.seats_available = calculate_seats_available(
        current_members=active_member_count,
        max_members=team.max_members,
    )


def _commit_membership_changes(
    session: Session,
    *,
    membership: TeamMembership,
    team: Team | None = None,
    refresh_team_aggregate: bool = False,
) -> TeamMembership:
    session.add(membership)
    if team is not None:
        if refresh_team_aggregate:
            _recalculate_team_aggregate(session, team)
        session.add(team)
    session.commit()
    session.refresh(membership)
    if team is not None:
        session.refresh(team)
    return membership


async def _revoke_invite(
    *,
    membership: TeamMembership,
    team: Team,
    owner_account: Account,
    client: TeamClient,
) -> None:
    await client._call_transport(
        "DELETE",
        f"/backend-api/accounts/{team.upstream_account_id}/invites",
        access_token=str(owner_account.access_token or "").strip(),
        json={"email_address": membership.member_email},
    )


async def _remove_member(
    *,
    membership: TeamMembership,
    team: Team,
    owner_account: Account,
    client: TeamClient,
) -> None:
    await client._call_transport(
        "DELETE",
        f"/backend-api/accounts/{team.upstream_account_id}/users/{membership.upstream_user_id}",
        access_token=str(owner_account.access_token or "").strip(),
    )


async def apply_membership_action(
    db: Session,
    *,
    membership_id: int,
    action: str,
    client: TeamClient | None = None,
    account_id: int | None = None,
) -> dict[str, object]:
    """对单条 TeamMembership 执行动作。"""
    membership = _load_membership(db, membership_id)
    if membership is None:
        return _build_result(
            success=False,
            message="membership not found",
            team_id=None,
            membership_id=membership_id,
            next_status=None,
            refresh_required=False,
            error_code=404,
        )

    normalized_action = str(action or "").strip().lower()
    current_status = str(membership.membership_status or "").strip().lower()

    if normalized_action == "bind-local-account":
        if account_id is None:
            return _build_result(
                success=False,
                message="account_id is required for bind-local-account",
                team_id=membership.team_id,
                membership_id=membership.id,
                next_status=current_status or membership.membership_status,
                refresh_required=False,
                error_code=400,
            )

        bind_result = bind_local_account(db, membership.id, account_id)
        refreshed_membership = _load_membership(db, membership.id) or membership
        if bind_result.get("success"):
            bound_team = _load_team(db, refreshed_membership.team_id)
            refreshed_membership = _commit_membership_changes(
                db,
                membership=refreshed_membership,
                team=bound_team,
                refresh_team_aggregate=True,
            )
        return _build_result(
            success=bool(bind_result.get("success")),
            message=str(bind_result.get("message") or ("local account bound" if bind_result.get("success") else "bind local account failed")),
            team_id=refreshed_membership.team_id,
            membership_id=refreshed_membership.id,
            next_status=refreshed_membership.membership_status,
            refresh_required=bool(bind_result.get("success")),
            error_code=bind_result.get("error_code"),
            account_id=bind_result.get("account_id", account_id),
        )

    if normalized_action not in {"revoke", "remove"}:
        return _build_result(
            success=False,
            message=f"unsupported membership action: {normalized_action or action}",
            team_id=membership.team_id,
            membership_id=membership.id,
            next_status=current_status or membership.membership_status,
            refresh_required=False,
            error_code=400,
        )

    team = _load_team(db, membership.team_id)
    if team is None:
        return _build_result(
            success=False,
            message="team not found",
            team_id=membership.team_id,
            membership_id=membership.id,
            next_status=current_status or membership.membership_status,
            refresh_required=False,
            error_code=404,
        )

    owner_account = _load_owner_account(db, team.owner_account_id)
    if owner_account is None:
        return _build_result(
            success=False,
            message="owner account not found",
            team_id=membership.team_id,
            membership_id=membership.id,
            next_status=current_status or membership.membership_status,
            refresh_required=False,
            error_code=404,
        )

    access_token = str(owner_account.access_token or "").strip()
    if not access_token:
        return _build_result(
            success=False,
            message="owner access token missing",
            team_id=membership.team_id,
            membership_id=membership.id,
            next_status=current_status or membership.membership_status,
            refresh_required=False,
            error_code=400,
        )

    team_client = client or TeamClient()

    if normalized_action == "revoke":
        if current_status != "invited":
            return _build_result(
                success=False,
                message="revoke is only allowed for invited memberships",
                team_id=membership.team_id,
                membership_id=membership.id,
                next_status=current_status or membership.membership_status,
                refresh_required=False,
                error_code=400,
            )

        try:
            await _revoke_invite(
                membership=membership,
                team=team,
                owner_account=owner_account,
                client=team_client,
            )
        except TeamClientError as exc:
            membership.sync_error = str(exc)
            _commit_membership_changes(db, membership=membership)
            return _build_result(
                success=False,
                message=str(exc),
                team_id=membership.team_id,
                membership_id=membership.id,
                next_status=current_status or membership.membership_status,
                refresh_required=False,
                error_code=502,
            )

        _apply_removed_status(membership, next_status="revoked")
        membership = _commit_membership_changes(
            db,
            membership=membership,
            team=team,
            refresh_team_aggregate=True,
        )
        return _build_result(
            success=True,
            message="membership invite revoked",
            team_id=membership.team_id,
            membership_id=membership.id,
            next_status="revoked",
            refresh_required=True,
        )

    if current_status not in {"joined", "already_member"}:
        return _build_result(
            success=False,
            message="remove is only allowed for joined/already_member memberships",
            team_id=membership.team_id,
            membership_id=membership.id,
            next_status=current_status or membership.membership_status,
            refresh_required=False,
            error_code=400,
        )

    if not str(membership.upstream_user_id or "").strip():
        return _build_result(
            success=False,
            message="upstream_user_id is required for remove",
            team_id=membership.team_id,
            membership_id=membership.id,
            next_status=current_status or membership.membership_status,
            refresh_required=False,
            error_code=400,
        )

    try:
        await _remove_member(
            membership=membership,
            team=team,
            owner_account=owner_account,
            client=team_client,
        )
    except TeamClientError as exc:
        membership.sync_error = str(exc)
        _commit_membership_changes(db, membership=membership)
        return _build_result(
            success=False,
            message=str(exc),
            team_id=membership.team_id,
            membership_id=membership.id,
            next_status=current_status or membership.membership_status,
            refresh_required=False,
            error_code=502,
        )

    _apply_removed_status(membership, next_status="removed")
    membership = _commit_membership_changes(
        db,
        membership=membership,
        team=team,
        refresh_team_aggregate=True,
    )
    return _build_result(
        success=True,
        message="membership removed",
        team_id=membership.team_id,
        membership_id=membership.id,
        next_status="removed",
        refresh_required=True,
    )
