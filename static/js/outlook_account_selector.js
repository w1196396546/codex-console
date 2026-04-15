(function (global, factory) {
    const api = factory();

    if (typeof module !== 'undefined' && module.exports) {
        module.exports = api;
    }

    if (global) {
        global.OutlookAccountSelector = api;
    }
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
    const EMAIL_PATTERN = /[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/i;

    function toNumericId(value) {
        const numeric = Number(value);
        return Number.isFinite(numeric) ? numeric : null;
    }

    function cloneIdSet(values) {
        const next = new Set();
        for (const value of values || []) {
            const numeric = toNumericId(value);
            if (numeric !== null) {
                next.add(numeric);
            }
        }
        return next;
    }

    function normalizeKeyword(value) {
        return String(value || '').trim().toLowerCase();
    }

    function extractNormalizedEmailCandidate(value) {
        const text = String(value || '').trim();
        if (!text) {
            return '';
        }

        if (text.includes('----')) {
            const firstPart = text.split('----', 1)[0];
            const firstMatch = firstPart.match(EMAIL_PATTERN);
            if (firstMatch) {
                return normalizeKeyword(firstMatch[0]);
            }
        }

        const match = text.match(EMAIL_PATTERN);
        if (match) {
            return normalizeKeyword(match[0]);
        }

        return normalizeKeyword(text);
    }

    function hasStructuredKeywordInput(value) {
        if (Array.isArray(value)) {
            return value.length > 1;
        }

        const text = String(value || '');
        return text.includes('----') || text.includes('\n') || text.includes('\r');
    }

    function collectNormalizedEmails(value) {
        const rawValues = Array.isArray(value)
            ? value
            : String(value || '').split(/\r?\n/);
        const seen = new Set();
        const emails = [];

        for (const rawValue of rawValues) {
            const normalized = extractNormalizedEmailCandidate(rawValue);
            if (!normalized || seen.has(normalized)) {
                continue;
            }
            seen.add(normalized);
            emails.push(normalized);
        }

        return emails;
    }

    function hasRegistrationCompleteFlag(account) {
        return Boolean(account) && account.is_registration_complete === true;
    }

    // 当前注册执行状态只看账号是否存在、以及 refresh token 是否完整；
    // 不引入 account.status 的额外业务语义。
    function mapExecutionState(account) {
        if (hasRegistrationCompleteFlag(account)) {
            return 'registered_complete';
        }
        if (account && Boolean(account.is_registered)) {
            return 'registered_needs_token_refresh';
        }
        return 'unregistered';
    }

    function isExecutableExecutionState(executionState) {
        return executionState === 'unregistered' || executionState === 'registered_needs_token_refresh';
    }

    function isExecutableAccount(account) {
        return isExecutableExecutionState(mapExecutionState(account));
    }

    function getExecutionStateLabel(executionState) {
        if (executionState === 'registered_needs_token_refresh') {
            return '已注册，待补 Token';
        }
        if (executionState === 'registered_complete') {
            return '注册已完成';
        }
        return '未注册';
    }

    function matchesExecutionState(account, executionState) {
        if (!executionState || executionState === 'all') {
            return true;
        }
        return mapExecutionState(account) === executionState;
    }

    function matchesKeyword(account, keyword) {
        if (!keyword) {
            return true;
        }

        const exactEmails = collectNormalizedEmails(keyword);
        if (exactEmails.length > 0 && (exactEmails.length > 1 || hasStructuredKeywordInput(keyword))) {
            return exactEmails.includes(normalizeKeyword(account && account.email));
        }

        const haystacks = [
            account.email,
            account.name,
        ];

        return haystacks.some((value) => String(value || '').toLowerCase().includes(keyword));
    }

    function filterAccounts(accounts, filters) {
        const keyword = normalizeKeyword(filters && filters.keyword);
        const executionState = (filters && (filters.executionState || filters.status)) || 'all';

        return (accounts || []).filter((account) => (
            matchesExecutionState(account, executionState) && matchesKeyword(account, keyword)
        ));
    }

    function createInitialSelectedIds(accounts) {
        const selected = new Set();

        for (const account of accounts || []) {
            const numericId = toNumericId(account.id);
            if (numericId !== null && isExecutableAccount(account)) {
                selected.add(numericId);
            }
        }

        return selected;
    }

    function countExecutableAccounts(accounts) {
        let count = 0;
        for (const account of accounts || []) {
            if (isExecutableAccount(account)) {
                count += 1;
            }
        }
        return count;
    }

    function selectVisibleAccounts(selectedIds, visibleAccounts) {
        const next = cloneIdSet(selectedIds);

        for (const account of visibleAccounts || []) {
            const numericId = toNumericId(account.id);
            if (numericId !== null) {
                next.add(numericId);
            }
        }

        return next;
    }

    function selectVisibleExecutableAccounts(selectedIds, visibleAccounts) {
        const next = cloneIdSet(selectedIds);

        for (const account of visibleAccounts || []) {
            const numericId = toNumericId(account.id);
            if (numericId !== null && isExecutableAccount(account)) {
                next.add(numericId);
            }
        }

        return next;
    }

    const selectExecutableVisibleAccounts = selectVisibleExecutableAccounts;
    const selectVisibleUnregisteredAccounts = selectVisibleExecutableAccounts;

    function deselectVisibleAccounts(selectedIds, visibleAccounts) {
        const next = cloneIdSet(selectedIds);

        for (const account of visibleAccounts || []) {
            const numericId = toNumericId(account.id);
            if (numericId !== null) {
                next.delete(numericId);
            }
        }

        return next;
    }

    function getVisibleSelectedIds(selectedIds, visibleAccounts) {
        const selected = cloneIdSet(selectedIds);
        const visibleSelected = new Set();

        for (const account of visibleAccounts || []) {
            const numericId = toNumericId(account.id);
            if (numericId !== null && selected.has(numericId)) {
                visibleSelected.add(numericId);
            }
        }

        return visibleSelected;
    }

    function buildSelectionSummary(options) {
        const totalCount = Number(options && options.totalCount) || 0;
        const filteredCount = Number(options && options.filteredCount) || 0;
        const selectedIds = cloneIdSet(options && options.selectedIds);
        const visibleSelectedIds = cloneIdSet(options && options.visibleSelectedIds);
        const hiddenSelectedCount = Math.max(0, selectedIds.size - visibleSelectedIds.size);

        let summary = `已选 ${selectedIds.size} / ${totalCount} 个账户，当前显示 ${filteredCount} 个`;
        if (hiddenSelectedCount > 0) {
            summary += `，其中 ${hiddenSelectedCount} 个已选项已被筛选隐藏`;
        }
        return summary;
    }

    function resolveSelectedIdsByEmails(accounts, emails) {
        const targets = new Set(collectNormalizedEmails(emails));
        const selected = new Set();

        if (targets.size === 0) {
            return selected;
        }

        for (const account of accounts || []) {
            const numericId = toNumericId(account && account.id);
            if (numericId === null) {
                continue;
            }

            if (targets.has(normalizeKeyword(account && account.email))) {
                selected.add(numericId);
            }
        }

        return selected;
    }

    return {
        buildSelectionSummary,
        collectNormalizedEmails,
        countExecutableAccounts,
        createInitialSelectedIds,
        deselectVisibleAccounts,
        filterAccounts,
        getExecutionStateLabel,
        getVisibleSelectedIds,
        isExecutableAccount,
        mapExecutionState,
        resolveSelectedIdsByEmails,
        selectExecutableVisibleAccounts,
        selectVisibleAccounts,
        selectVisibleExecutableAccounts,
        selectVisibleUnregisteredAccounts,
    };
});
