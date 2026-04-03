from .client import (
    TeamAccount,
    TeamAuthenticationError,
    TeamClient,
    TeamClientError,
    TeamInvite,
    TeamMember,
    TeamNotFoundError,
    TeamPermissionError,
    TeamRateLimitError,
    TeamResponseFormatError,
    TeamUser,
)
from .utils import DEFAULT_MAX_MEMBERS, pick_active_memberships, seats_available

_LAZY_EXPORTS = {
    "TeamSyncError": (".sync", "TeamSyncError"),
    "TeamSyncNotFoundError": (".sync", "TeamSyncNotFoundError"),
    "sync_team_memberships": (".sync", "sync_team_memberships"),
    "invite_account_ids": (".invite", "invite_account_ids"),
    "invite_manual_emails": (".invite", "invite_manual_emails"),
    "apply_membership_action": (".membership_actions", "apply_membership_action"),
}


def __getattr__(name: str):
    lazy_target = _LAZY_EXPORTS.get(name)
    if lazy_target is None:
        raise AttributeError(f"module {__name__!r} has no attribute {name!r}")

    from importlib import import_module

    module_name, attr_name = lazy_target
    value = getattr(import_module(module_name, __name__), attr_name)
    globals()[name] = value
    return value


def __dir__() -> list[str]:
    return sorted(set(globals()) | set(__all__))

__all__ = [
    "TeamAccount",
    "TeamAuthenticationError",
    "TeamClient",
    "TeamClientError",
    "TeamInvite",
    "TeamMember",
    "TeamNotFoundError",
    "TeamPermissionError",
    "TeamRateLimitError",
    "TeamResponseFormatError",
    "TeamSyncError",
    "TeamSyncNotFoundError",
    "TeamUser",
    "DEFAULT_MAX_MEMBERS",
    "apply_membership_action",
    "invite_account_ids",
    "invite_manual_emails",
    "pick_active_memberships",
    "seats_available",
    "sync_team_memberships",
]
