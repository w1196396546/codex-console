(function (global, factory) {
    const api = factory();

    if (typeof module !== 'undefined' && module.exports) {
        module.exports = api;
    }

    if (global) {
        global.OutlookAccountSelector = api;
    }
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
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

    function matchesStatus(account, status) {
        if (status === 'registered') {
            return Boolean(account.is_registered);
        }
        if (status === 'unregistered') {
            return !account.is_registered;
        }
        return true;
    }

    function matchesKeyword(account, keyword) {
        if (!keyword) {
            return true;
        }

        const haystacks = [
            account.email,
            account.name,
        ];

        return haystacks.some((value) => String(value || '').toLowerCase().includes(keyword));
    }

    function filterAccounts(accounts, filters) {
        const keyword = normalizeKeyword(filters && filters.keyword);
        const status = (filters && filters.status) || 'all';

        return (accounts || []).filter((account) => (
            matchesStatus(account, status) && matchesKeyword(account, keyword)
        ));
    }

    function createInitialSelectedIds(accounts) {
        const selected = new Set();

        for (const account of accounts || []) {
            const numericId = toNumericId(account.id);
            if (numericId !== null && !account.is_registered) {
                selected.add(numericId);
            }
        }

        return selected;
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

    function selectVisibleUnregisteredAccounts(selectedIds, visibleAccounts) {
        const next = cloneIdSet(selectedIds);

        for (const account of visibleAccounts || []) {
            const numericId = toNumericId(account.id);
            if (numericId !== null && !account.is_registered) {
                next.add(numericId);
            }
        }

        return next;
    }

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

    return {
        buildSelectionSummary,
        createInitialSelectedIds,
        deselectVisibleAccounts,
        filterAccounts,
        getVisibleSelectedIds,
        selectVisibleAccounts,
        selectVisibleUnregisteredAccounts,
    };
});
