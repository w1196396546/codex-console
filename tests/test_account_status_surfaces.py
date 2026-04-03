from pathlib import Path


def test_accounts_template_exposes_partial_account_status_filters():
    template = Path("templates/accounts.html").read_text(encoding="utf-8")

    assert '<option value="token_pending">待补 RT</option>' in template
    assert '<option value="login_incomplete">登录待补全</option>' in template
    assert 'data-status="token_pending"' in template
    assert 'data-status="login_incomplete"' in template


def test_accounts_overview_template_allows_new_partial_statuses():
    template = Path("templates/accounts_overview.html").read_text(encoding="utf-8")

    assert '<option value="token_pending">token_pending</option>' in template
    assert '<option value="login_incomplete">login_incomplete</option>' in template


def test_frontend_status_maps_cover_partial_account_states():
    utils_js = Path("static/js/utils.js").read_text(encoding="utf-8")
    accounts_js = Path("static/js/accounts.js").read_text(encoding="utf-8")
    overview_js = Path("static/js/accounts_overview.js").read_text(encoding="utf-8")

    assert "token_pending: { text: '待补 RT', class: 'warning' }" in utils_js
    assert "login_incomplete: { text: '登录待补全', class: 'warning' }" in utils_js
    assert "token_pending: { icon: '🟠', title: '待补 RT' }" in utils_js
    assert "login_incomplete: { icon: '🟣', title: '登录待补全' }" in utils_js
    assert "['active', 'token_pending', 'login_incomplete', 'expired', 'banned', 'failed']" in accounts_js
    assert "if (value === 'token_pending') return '#f97316';" in overview_js
    assert "if (value === 'login_incomplete') return '#8b5cf6';" in overview_js


def test_accounts_list_exposes_sub2api_upload_column_and_renderer():
    template = Path("templates/accounts.html").read_text(encoding="utf-8")
    accounts_js = Path("static/js/accounts.js").read_text(encoding="utf-8")

    assert ">Sub2API</th>" in template
    assert "account.sub2api_uploaded" in accounts_js
    assert "format.date(account.sub2api_uploaded_at)" in accounts_js
