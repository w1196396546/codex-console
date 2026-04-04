from __future__ import annotations

from datetime import datetime
from typing import Sequence

from sqlalchemy.orm import Session

from src.config.constants import AccountStatus
from src.database.models import Account
from src.database.team_crud import upsert_team
from src.services.team.client import TeamClient


def _parse_upstream_datetime(value: str | None) -> datetime | None:
    if not value:
        return None
    normalized = value.replace("Z", "+00:00")
    return datetime.fromisoformat(normalized)


async def discover_teams_from_local_accounts(
    db: Session,
    *,
    owner_account_ids: Sequence[int] | None = None,
    client: TeamClient | None = None,
) -> dict[str, int]:
    """从本地 accounts 表发现 Team 母号并同步到 teams。"""
    team_client = client or TeamClient()
    query = db.query(Account)
    if owner_account_ids is not None:
        normalized_owner_ids = [
            owner_account_id
            for owner_account_id in owner_account_ids
            if isinstance(owner_account_id, int) and owner_account_id > 0
        ]
        if not normalized_owner_ids:
            return {
                "accounts_scanned": 0,
                "teams_found": 0,
                "teams_persisted": 0,
            }
        query = query.filter(Account.id.in_(normalized_owner_ids))

    raw_accounts = query.order_by(Account.id.asc()).all()

    accounts_scanned = 0
    teams_found = 0
    teams_persisted = 0
    seen_account_ids: set[str] = set()

    for account in raw_accounts:
        access_token = str(account.access_token or "").strip()
        account_id = str(account.account_id or "").strip()
        if not access_token or not account_id:
            continue
        if str(account.status or "").strip().lower() in {
            AccountStatus.FAILED.value,
            AccountStatus.EXPIRED.value,
            AccountStatus.BANNED.value,
        }:
            continue
        if account_id in seen_account_ids:
            continue
        seen_account_ids.add(account_id)
        accounts_scanned += 1

        payload = await team_client.get_team_accounts(account.access_token)
        parsed_accounts = team_client.parse_team_accounts(payload)

        team_accounts = [
            item
            for item in parsed_accounts
            if item.get("plan_type") == "team"
        ]
        owner_teams = [
            item
            for item in team_accounts
            if item.get("account_role_snapshot") == "account-owner"
        ]

        teams_found += len(team_accounts)
        for item in owner_teams:
            upsert_team(
                db,
                owner_account_id=account.id,
                upstream_account_id=item["upstream_account_id"],
                team_name=item["team_name"],
                plan_type=item["plan_type"],
                subscription_plan=item.get("subscription_plan"),
                account_role_snapshot=item.get("account_role_snapshot"),
                expires_at=_parse_upstream_datetime(item.get("expires_at")),
                status=account.status or "active",
                last_sync_at=datetime.utcnow(),
                sync_status="synced",
                sync_error=None,
            )
            teams_persisted += 1

    return {
        "accounts_scanned": accounts_scanned,
        "teams_found": teams_found,
        "teams_persisted": teams_persisted,
    }
