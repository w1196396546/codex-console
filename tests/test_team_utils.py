import importlib

import pytest


MODULE_NAME = 'src.services.team.utils'


def _load_team_utils_module():
    try:
        return importlib.import_module(MODULE_NAME)
    except ModuleNotFoundError as exc:
        pytest.fail(f'missing module {MODULE_NAME}: {exc}')


def test_normalize_team_email_trims_lowercases_and_extracts_from_wrapped_text():
    team_utils = _load_team_utils_module()

    assert team_utils.normalize_team_email('  Alice+Team@Example.COM  ') == 'alice+team@example.com'
    assert team_utils.normalize_team_email(' Name <Foo@Example.com> ') == 'foo@example.com'
    assert team_utils.normalize_team_email(None) == ''


def test_resolve_team_member_status_prefers_highest_priority_status():
    team_utils = _load_team_utils_module()

    assert team_utils.resolve_team_member_status(['invited', 'joined']) == 'joined'
    assert team_utils.resolve_team_member_status(['failed', 'removed', 'already_member']) == 'already_member'
    assert team_utils.resolve_team_member_status(['revoked', 'failed']) == 'revoked'


def test_count_current_members_counts_invited_members_from_spec_fields():
    team_utils = _load_team_utils_module()

    members = [
        {'member_email': ' Invitee@Example.com ', 'membership_status': 'invited'},
        {'member_email': 'Removed@example.com', 'membership_status': 'removed'},
    ]

    assert team_utils.count_current_members(members) == 1


def test_count_current_members_deduplicates_same_email_across_joined_and_invited():
    team_utils = _load_team_utils_module()

    members = [
        {'member_email': ' Name <Alice@Example.com> ', 'membership_status': 'invited'},
        {'member_email': 'alice@example.com', 'membership_status': 'joined'},
        {'member_email': 'BOB@example.com', 'membership_status': 'already_member'},
        {'member_email': 'bob@example.com ', 'membership_status': 'failed'},
        {'member_email': 'carol@example.com', 'membership_status': 'removed'},
        {'member_email': 'dave@example.com', 'membership_status': 'joined'},
        {'member_email': 'dave@example.com', 'membership_status': 'revoked'},
        {'member_email': None, 'membership_status': 'joined'},
    ]

    assert team_utils.count_current_members(members) == 3


def test_count_current_members_keeps_legacy_field_names_as_fallback():
    team_utils = _load_team_utils_module()

    members = [
        {'email': 'legacy@example.com', 'status': 'invited'},
        {'email': 'legacy@example.com', 'status': 'joined'},
    ]

    assert team_utils.count_current_members(members) == 1


def test_pick_active_memberships_merges_by_email_before_filtering_active_statuses():
    team_utils = _load_team_utils_module()

    members = [
        {'member_email': ' Name <Alice@Example.com> ', 'membership_status': 'invited'},
        {'member_email': 'alice@example.com', 'membership_status': 'joined'},
        {'member_email': 'BOB@example.com', 'membership_status': 'failed'},
        {'member_email': 'bob@example.com ', 'membership_status': 'already_member'},
        {'member_email': 'carol@example.com', 'membership_status': 'removed'},
        {'member_email': 'dave@example.com', 'membership_status': 'joined'},
        {'member_email': 'dave@example.com', 'membership_status': 'revoked'},
        {'member_email': None, 'membership_status': 'joined'},
    ]

    assert team_utils.pick_active_memberships(members) == [
        {'member_email': 'alice@example.com', 'membership_status': 'joined'},
        {'member_email': 'bob@example.com', 'membership_status': 'already_member'},
        {'member_email': 'dave@example.com', 'membership_status': 'joined'},
    ]


def test_pick_active_memberships_ignores_non_spec_active_status_values():
    team_utils = _load_team_utils_module()

    members = [
        {'member_email': 'spec@example.com', 'membership_status': 'invited'},
        {'member_email': 'non-spec@example.com', 'membership_status': 'active'},
        {'member_email': 'legacy-non-spec@example.com', 'status': 'active'},
    ]

    assert team_utils.pick_active_memberships(members) == [
        {'member_email': 'spec@example.com', 'membership_status': 'invited'},
    ]
    assert team_utils.count_current_members(members) == 1


def test_team_package_exports_pick_active_memberships():
    team_package = importlib.import_module('src.services.team')

    assert team_package.pick_active_memberships is not None


def test_seats_available_uses_default_max_members_when_value_is_missing():
    team_utils = _load_team_utils_module()

    assert team_utils.DEFAULT_MAX_MEMBERS == 6
    assert team_utils.seats_available(current_members=1) == 5


def test_seats_available_never_drops_below_zero():
    team_utils = _load_team_utils_module()

    assert team_utils.seats_available(current_members=5, max_members=3) == 0
