"""Team 关系绑定与回填服务。"""

from __future__ import annotations

from sqlalchemy.orm import Session

from src.database.models import Account, AppLog
from src.database.team_models import TeamMembership

from .utils import normalize_team_email


def relink_account_memberships(session: Session, account_id: int, email: str) -> int:
    """按邮箱将尚未绑定的成员关系回填到本地账号。"""
    normalized_email = normalize_team_email(email)
    if not normalized_email:
        return 0

    memberships = (
        session.query(TeamMembership)
        .filter(
            TeamMembership.member_email == normalized_email,
            TeamMembership.local_account_id.is_(None),
        )
        .all()
    )

    updated = 0
    for membership in memberships:
        membership.local_account_id = account_id
        updated += 1

    if updated:
        session.flush()

    return updated


def bind_local_account(session: Session, membership_id: int, account_id: int) -> dict[str, object]:
    """手动绑定成员关系到本地账号，第一阶段仅允许同邮箱绑定。"""
    membership = session.query(TeamMembership).filter(TeamMembership.id == membership_id).first()
    if membership is None:
        return {"success": False, "error_code": 404, "message": "membership not found"}

    account = session.query(Account).filter(Account.id == account_id).first()
    if account is None:
        return {"success": False, "error_code": 404, "message": "account not found"}

    membership_email = normalize_team_email(membership.member_email)
    account_email = normalize_team_email(account.email)
    if not membership_email or membership_email != account_email:
        return {
            "success": False,
            "error_code": 400,
            "message": "cross-email binding requires explicit confirmation",
        }

    if membership.local_account_id not in (None, account.id):
        return {
            "success": False,
            "error_code": 409,
            "message": "membership already bound to another local account",
        }

    membership.local_account_id = account.id
    membership.source = "manual_bind"
    session.add(
        AppLog(
            level="INFO",
            logger="team.relation",
            module="team.relation",
            pathname=__file__,
            lineno=0,
            message=(
                f"Team membership manual bind success: membership_id={membership.id}, "
                f"account_id={account.id}, email={account_email}"
            ),
        )
    )
    session.flush()
    return {"success": True, "membership_id": membership.id, "account_id": account.id}
