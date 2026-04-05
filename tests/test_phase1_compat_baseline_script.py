from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT_PATH = ROOT / "scripts" / "verify_phase1_compat_baseline.sh"
MANIFEST_PATH = ROOT / ".planning" / "phases" / "01-compatibility-baseline" / "01-compat-fixture-manifest.md"
RUNBOOK_PATH = ROOT / ".planning" / "phases" / "01-compatibility-baseline" / "01-ops-compat-runbook.md"


def test_phase1_verify_script_uses_stable_python_gate_and_uv_friendly_launcher():
    script = SCRIPT_PATH.read_text(encoding="utf-8")

    assert "uv run pytest tests/test_registration_routes.py tests/test_accounts_routes.py tests/test_settings_routes.py tests/test_payment_routes.py -q" in script
    assert "pytest tests/test_registration_routes.py tests/test_accounts_routes.py tests/test_settings_routes.py tests/test_payment_routes.py tests/test_team_routes.py tests/test_team_tasks_routes.py -q" not in script


def test_phase1_manifest_marks_team_python_suite_as_advisory_follow_up():
    manifest = MANIFEST_PATH.read_text(encoding="utf-8")

    assert "Team route family" in manifest
    assert "advisory, not part of the Phase 1 green gate" in manifest


def test_phase1_runbook_records_team_suite_as_later_phase_follow_up():
    runbook = RUNBOOK_PATH.read_text(encoding="utf-8")

    assert "Team route/task pytest remains advisory until Phase 4" in runbook
