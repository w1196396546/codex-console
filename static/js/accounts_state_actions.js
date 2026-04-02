(function (global, factory) {
    const api = factory();

    if (typeof module !== 'undefined' && module.exports) {
        module.exports = api;
    }

    if (global) {
        global.AccountsStateActions = api;
    }
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
    function normalizeText(value) {
        return String(value || '').trim();
    }

    function cloneIdSet(values) {
        return new Set(Array.from(values || []).map((value) => Number(value)).filter(Number.isFinite));
    }

    function serializeFilters(filters = {}) {
        return {
            status: normalizeText(filters.status),
            email_service: normalizeText(filters.email_service),
            refresh_token_state: normalizeText(filters.refresh_token_state),
            search: normalizeText(filters.search),
        };
    }

    function buildAccountsQuery({ page, pageSize, filters = {} }) {
        const normalized = serializeFilters(filters);
        const params = new URLSearchParams();
        params.set('page', String(page));
        params.set('page_size', String(pageSize));

        if (normalized.status) {
            params.set('status', normalized.status);
        }
        if (normalized.email_service) {
            params.set('email_service', normalized.email_service);
        }
        if (normalized.refresh_token_state) {
            params.set('refresh_token_state', normalized.refresh_token_state);
        }
        if (normalized.search) {
            params.set('search', normalized.search);
        }

        return params.toString();
    }

    function deriveFilterChangeState({
        previousFilters = {},
        nextFilters = {},
        currentPage = 1,
        selectedIds = new Set(),
        selectAllPages = false,
    }) {
        const previous = serializeFilters(previousFilters);
        const next = serializeFilters(nextFilters);
        const changed = Object.keys(previous).some((key) => previous[key] !== next[key]);

        if (!changed) {
            return {
                changed: false,
                currentPage,
                selectedIds: cloneIdSet(selectedIds),
                selectAllPages: Boolean(selectAllPages),
                filters: next,
            };
        }

        return {
            changed: true,
            currentPage: 1,
            selectedIds: new Set(),
            selectAllPages: false,
            filters: next,
        };
    }

    function buildSingleStateRequest({ accountId, status }) {
        return {
            path: `/accounts/${Number(accountId)}`,
            method: 'PATCH',
            body: { status: normalizeText(status) },
        };
    }

    function buildBatchOperationPayload({
        selectedIds = new Set(),
        selectAllPages = false,
        filters = {},
        extraFields = {},
    }) {
        const normalized = serializeFilters(filters);
        if (selectAllPages) {
            return {
                ids: [],
                select_all: true,
                status_filter: normalized.status || null,
                email_service_filter: normalized.email_service || null,
                refresh_token_state_filter: normalized.refresh_token_state || null,
                search_filter: normalized.search || null,
                ...extraFields,
            };
        }

        return {
            ids: Array.from(cloneIdSet(selectedIds)),
            ...extraFields,
        };
    }

    function buildBatchStatePayload({
        status,
        selectedIds = new Set(),
        selectAllPages = false,
        filters = {},
    }) {
        return buildBatchOperationPayload({
            selectedIds,
            selectAllPages,
            filters,
            extraFields: {
                status: normalizeText(status),
            },
        });
    }

    function getBatchStateControlState({
        selectedCount = 0,
        selectAllPages = false,
        totalAccounts = 0,
    }) {
        const count = selectAllPages ? Number(totalAccounts) || 0 : Number(selectedCount) || 0;
        return {
            disabled: count === 0,
            count,
            label: count > 0 ? `批量改状态 (${count})` : '批量改状态',
        };
    }

    function summarizeBatchStateResult({
        requestedCount = 0,
        updatedCount = 0,
        skippedCount = 0,
        missingIds = [],
        errors = [],
    }) {
        const errorCount = Array.isArray(errors) ? errors.length : 0;
        const missingCount = Number(skippedCount) || (Array.isArray(missingIds) ? missingIds.length : 0);
        const countChanged = Number(requestedCount) !== Number(updatedCount);

        if (errorCount === 0 && missingCount === 0 && !countChanged) {
            return {
                level: 'success',
                message: `已成功更新 ${updatedCount} 个账号状态`,
            };
        }

        let message = missingCount > 0
            ? `部分成功：已更新 ${updatedCount} 个账号，跳过 ${missingCount} 个不存在账号。`
            : `部分成功：已更新 ${updatedCount} 个账号，${errorCount} 个失败。`;
        if (countChanged && missingCount === 0) {
            message += '筛选结果在提交期间发生变化。';
        }
        return {
            level: 'warning',
            message,
        };
    }

    function createLatestRequestOrchestrator({
        fetcher,
        applyResult,
        applyError,
    }) {
        let inFlight = false;
        let latestRequestId = 0;
        let pendingArgs = null;

        async function request(args) {
            latestRequestId += 1;
            pendingArgs = args;

            if (inFlight) {
                return;
            }

            inFlight = true;
            while (pendingArgs) {
                const currentArgs = pendingArgs;
                pendingArgs = null;
                const requestId = latestRequestId;

                try {
                    const result = await fetcher(currentArgs);
                    if (requestId === latestRequestId && pendingArgs === null && typeof applyResult === 'function') {
                        await applyResult(result, currentArgs);
                    }
                } catch (error) {
                    if (requestId === latestRequestId && pendingArgs === null && typeof applyError === 'function') {
                        await applyError(error, currentArgs);
                    }
                }
            }
            inFlight = false;
        }

        return { request };
    }

    return {
        buildAccountsQuery,
        buildBatchOperationPayload,
        buildBatchStatePayload,
        createLatestRequestOrchestrator,
        buildSingleStateRequest,
        deriveFilterChangeState,
        getBatchStateControlState,
        summarizeBatchStateResult,
    };
});
