"""纯函数：处理 Team 邮箱与成员状态统计。"""

from __future__ import annotations

from collections.abc import Iterable, Mapping
import re

_STATUS_PRIORITY = {
    'failed': 0,
    'removed': 1,
    'revoked': 2,
    'invited': 3,
    'already_member': 4,
    'joined': 5,
}
_ACTIVE_STATUSES = {'joined', 'already_member', 'invited'}
_EMAIL_PATTERN = re.compile(r'([A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,})', re.IGNORECASE)
DEFAULT_MAX_MEMBERS = 6


def normalize_team_email(email: object) -> str:
    """规范化 Team 邮箱，便于比较与去重。"""
    if not isinstance(email, str):
        return ''

    normalized = email.strip().lower()
    match = _EMAIL_PATTERN.search(normalized)
    if match:
        return match.group(1)
    return normalized


def resolve_team_member_status(statuses: Iterable[object]) -> str:
    """按预定义优先级返回同一邮箱的最终状态。"""
    best_status = 'failed'
    best_rank = _STATUS_PRIORITY[best_status]

    for status in statuses:
        normalized_status = str(status or '').strip().lower()
        rank = _STATUS_PRIORITY.get(normalized_status, -1)
        if rank > best_rank:
            best_status = normalized_status
            best_rank = rank

    return best_status


def pick_active_memberships(members: Iterable[Mapping[str, object]]) -> list[dict[str, str]]:
    """按邮箱合并成员状态后，返回仍属于当前成员的唯一记录。"""
    statuses_by_email: dict[str, list[object]] = {}

    for member in members:
        email = normalize_team_email(member.get('member_email') or member.get('email'))
        if not email:
            continue
        status = member.get('membership_status') or member.get('status')
        statuses_by_email.setdefault(email, []).append(status)

    active_memberships: list[dict[str, str]] = []
    for email, statuses in statuses_by_email.items():
        resolved_status = resolve_team_member_status(statuses)
        if resolved_status in _ACTIVE_STATUSES:
            active_memberships.append(
                {'member_email': email, 'membership_status': resolved_status}
            )

    return active_memberships


def count_current_members(members: Iterable[Mapping[str, object]]) -> int:
    """统计去重后的活跃唯一邮箱数量。"""
    return len(pick_active_memberships(members))


def seats_available(*, current_members: object, max_members: object = None) -> int:
    """根据当前成员数与席位上限计算剩余席位，不返回负数。"""
    resolved_max_members = DEFAULT_MAX_MEMBERS if max_members in (None, '') else int(max_members)
    resolved_current_members = 0 if current_members in (None, '') else int(current_members)
    return max(resolved_max_members - resolved_current_members, 0)
