(function (global, factory) {
    const api = factory();

    if (typeof module !== 'undefined' && module.exports) {
        module.exports = api;
    }

    if (global) {
        global.AutoTeamPage = api;
    }
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
    const FINISHED_TASK_STATUSES = new Set(['completed', 'failed', 'cancelled']);

    function normalizeText(value) {
        return String(value || '').trim();
    }

    function normalizePositiveInt(value) {
        const numeric = Number(value);
        if (!Number.isFinite(numeric) || numeric <= 0) {
            return null;
        }
        return Math.trunc(numeric);
    }

    function formatDateTime(value) {
        const text = normalizeText(value);
        if (!text) {
            return '-';
        }

        const date = new Date(text);
        if (Number.isNaN(date.getTime())) {
            return text;
        }
        return date.toLocaleString('zh-CN', {
            hour12: false,
        });
    }

    function buildTeamsListPath(filters = {}) {
        const params = new URLSearchParams();
        if (normalizeText(filters.status)) {
            params.set('status', normalizeText(filters.status));
        }
        if (normalizeText(filters.search)) {
            params.set('search', normalizeText(filters.search));
        }
        const ownerAccountId = normalizePositiveInt(filters.ownerAccountId);
        if (ownerAccountId !== null) {
            params.set('owner_account_id', String(ownerAccountId));
        }

        const query = params.toString();
        return query ? `/api/team/teams?${query}` : '/api/team/teams';
    }

    function buildMembershipsPath(teamId, membershipStatus) {
        const normalizedTeamId = normalizePositiveInt(teamId);
        if (normalizedTeamId === null) {
            throw new Error('team id is required');
        }

        const params = new URLSearchParams();
        const status = normalizeText(membershipStatus);
        if (status) {
            params.set('status', status);
        }

        const query = params.toString();
        const basePath = `/api/team/teams/${normalizedTeamId}/memberships`;
        return query ? `${basePath}?${query}` : basePath;
    }

    function deriveInitialTeamState(query) {
        const source = typeof query === 'string' ? query.replace(/^\?/, '') : '';
        const params = new URLSearchParams(source);

        return {
            teams: [],
            taskItems: [],
            selectedTeamId: normalizePositiveInt(params.get('team_id')),
            activeTaskUuid: '',
            filters: {
                ownerAccountId: normalizePositiveInt(params.get('owner_account_id')),
                status: normalizeText(params.get('status')),
                search: normalizeText(params.get('search')),
            },
            loading: {
                teams: false,
                detail: false,
                tasks: false,
            },
        };
    }

    function buildMembershipActionRequest({ id, action }) {
        const membershipId = normalizePositiveInt(id);
        if (membershipId === null) {
            throw new Error('membership id is required');
        }

        const normalizedAction = normalizeText(action);
        if (!normalizedAction) {
            throw new Error('membership action is required');
        }

        return {
            membershipId,
            action: normalizedAction,
        };
    }

    async function afterSuccessfulMembershipAction(teamId, membershipStatus, {
        refreshTeamDetail,
        refreshMemberships,
        refreshTasks,
    } = {}) {
        const normalizedTeamId = normalizePositiveInt(teamId);
        if (normalizedTeamId === null) {
            throw new Error('team id is required');
        }

        const detailPath = `/api/team/teams/${normalizedTeamId}`;
        const membershipsPath = buildMembershipsPath(normalizedTeamId, membershipStatus);
        const tasksPath = `/api/team/tasks?team_id=${normalizedTeamId}`;

        if (typeof refreshTeamDetail === 'function') {
            await refreshTeamDetail(detailPath);
        }
        if (typeof refreshMemberships === 'function') {
            await refreshMemberships(membershipsPath);
        }
        if (typeof refreshTasks === 'function') {
            await refreshTasks(tasksPath);
        }

        return {
            detailPath,
            membershipsPath,
            tasksPath,
        };
    }

    function resolveInviteAvailability({ status, syncStatus } = {}) {
        const normalizedStatus = normalizeText(status).toLowerCase();
        const normalizedSyncStatus = normalizeText(syncStatus).toLowerCase();

        if (normalizedSyncStatus && normalizedSyncStatus !== 'success') {
            return {
                disabled: true,
                tone: 'danger',
                reason: '同步状态异常，请先完成一次成功同步再继续邀请。',
            };
        }

        if (normalizedStatus === 'full') {
            return {
                disabled: true,
                tone: 'warning',
                reason: '当前 Team 已满，无法继续批量邀请。',
            };
        }

        return {
            disabled: false,
            tone: 'ready',
            reason: '',
        };
    }

    function createAcceptedTaskFlow({
        refreshTeams,
        refreshTeamDetail,
        refreshTasks,
        createWebSocket,
        onStatusChange,
        onError,
    }) {
        const socketFactory = typeof createWebSocket === 'function'
            ? createWebSocket
            : (path) => new WebSocket(resolveWsUrl(path));

        async function refreshAfterSuccess(acceptedPayload) {
            if (typeof refreshTeams === 'function') {
                await refreshTeams('/api/team/teams');
            }
            if (acceptedPayload && acceptedPayload.team_id && typeof refreshTeamDetail === 'function') {
                await refreshTeamDetail(`/api/team/teams/${acceptedPayload.team_id}`);
            }
            if (acceptedPayload && acceptedPayload.team_id && typeof refreshTasks === 'function') {
                await refreshTasks(`/api/team/tasks?team_id=${acceptedPayload.team_id}`);
            }
        }

        async function start(acceptedPayload) {
            const wsChannel = normalizeText(acceptedPayload && acceptedPayload.ws_channel);
            if (!wsChannel) {
                return null;
            }

            const socket = socketFactory(wsChannel);
            const notify = (taskStatus, payload) => {
                if (typeof onStatusChange === 'function') {
                    onStatusChange(taskStatus, payload);
                }
            };

            socket.addEventListener('message', async (event) => {
                let payload;
                try {
                    payload = JSON.parse(event.data);
                } catch (error) {
                    if (typeof onError === 'function') {
                        onError(error);
                    }
                    return;
                }

                const taskStatus = normalizeText(payload && payload.status) || 'pending';
                notify(taskStatus, payload);

                if (!FINISHED_TASK_STATUSES.has(taskStatus)) {
                    return;
                }

                socket.close();
                if (taskStatus === 'completed') {
                    await refreshAfterSuccess(acceptedPayload);
                }
            });

            socket.addEventListener('error', (error) => {
                if (typeof onError === 'function') {
                    onError(error);
                }
            });

            return socket;
        }

        return { start };
    }

    function resolveWsUrl(path) {
        const origin = global.location && global.location.origin ? global.location.origin : 'http://localhost';
        const base = origin.replace(/^http/, 'ws');
        return `${base}${path}`;
    }

    async function fetchJson(path, options) {
        const response = await fetch(path, {
            credentials: 'same-origin',
            ...options,
            headers: {
                'Content-Type': 'application/json',
                ...(options && options.headers ? options.headers : {}),
            },
        });

        if (!response.ok) {
            const detail = await response.text();
            throw new Error(detail || `请求失败: ${response.status}`);
        }
        return response.json();
    }

    function renderTeams(state, elements) {
        const list = elements.teamList;
        const items = Array.isArray(state.teams) ? state.teams : [];
        elements.teamsStatus.textContent = state.loading.teams ? '加载中' : `已加载 ${items.length} 个`;
        elements.metricTotalTeams.textContent = String(items.length);
        elements.metricTotalSeats.textContent = String(items.reduce((sum, item) => sum + (Number(item.seats_available) || 0), 0));
        elements.metricActiveTasks.textContent = String(items.reduce((sum, item) => sum + (Number(item.active_task_count) || 0), 0));

        if (!items.length) {
            list.innerHTML = '<div class="team-empty-state">没有匹配的 Team，试试更换筛选或先执行“发现母号”。</div>';
            return;
        }

        list.innerHTML = items.map((team) => `
            <button
                type="button"
                class="team-list-item${team.id === state.selectedTeamId ? ' is-active' : ''}"
                data-team-id="${team.id}"
            >
                <div class="team-list-item-top">
                    <strong>${escapeHtml(team.team_name || `Team #${team.id}`)}</strong>
                    <span>${escapeHtml(team.status || 'unknown')}</span>
                </div>
                <div class="team-list-item-meta">
                    <span>Owner: ${escapeHtml(team.owner_email || '-')}</span>
                    <span>席位: ${Number(team.seats_available) || 0}/${Number(team.max_members) || 0}</span>
                </div>
            </button>
        `).join('');
    }

    function renderTeamDetail(detail, elements) {
        if (!detail) {
            elements.teamDetailTitle.textContent = '请选择一个 Team';
            elements.teamDetailStatus.textContent = '未选择';
            elements.teamDetailStatus.classList.add('muted');
            elements.detailOwnerEmail.textContent = '-';
            elements.detailOwnerId.textContent = '-';
            elements.detailMembers.textContent = '-';
            elements.detailSyncStatus.textContent = '-';
            elements.detailLastSync.textContent = '-';
            elements.detailSeats.textContent = '-';
            elements.teamDetailCallout.textContent = '当前还没有选中的 Team。左侧列表会展示 owner、席位和同步健康度，点击任意 Team 可在这里查看详情。';
            return;
        }

        elements.teamDetailTitle.textContent = detail.team_name || `Team #${detail.id}`;
        elements.teamDetailStatus.textContent = detail.status || 'unknown';
        elements.teamDetailStatus.classList.remove('muted');
        elements.detailOwnerEmail.textContent = detail.owner_email || '-';
        elements.detailOwnerId.textContent = detail.owner_account_id || '-';
        elements.detailMembers.textContent = `${Number(detail.joined_count) || 0} joined / ${Number(detail.invited_count) || 0} invited`;
        elements.detailSyncStatus.textContent = detail.sync_status || '-';
        elements.detailLastSync.textContent = formatDateTime(detail.last_sync_at);
        elements.detailSeats.textContent = `${Number(detail.seats_available) || 0} / ${Number(detail.max_members) || 0}`;
        elements.teamDetailCallout.textContent = detail.last_sync_error
            ? `最近同步异常：${detail.last_sync_error}`
            : '最近一次同步没有暴露错误，可以继续执行批量同步或邀请动作。';
    }

    function renderTasks(tasks, state, elements) {
        const items = Array.isArray(tasks) ? tasks : [];
        elements.taskCurrentUuid.textContent = state.activeTaskUuid || '暂无运行中的任务';
        elements.taskLiveStatus.textContent = state.activeTaskUuid ? '监听中' : '空闲';
        elements.taskCurrentSummary.textContent = state.activeTaskUuid
            ? '任务已被 accepted，正在等待 WebSocket 推送完成态。'
            : '当 discover / sync-batch 被 accepted 后，这里会监听 WebSocket 并在完成后刷新 Team 列表。';

        if (!items.length) {
            elements.taskList.innerHTML = '<div class="team-empty-state">还没有 Team 任务记录。</div>';
            return;
        }

        elements.taskList.innerHTML = items.map((item) => `
            <div class="task-list-item">
                <div class="task-list-item-top">
                    <strong>${escapeHtml(item.task_type || 'unknown')}</strong>
                    <span>${escapeHtml(item.status || 'pending')}</span>
                </div>
                <div class="task-list-item-meta">
                    <span>${escapeHtml(item.task_uuid || '-')}</span>
                    <span>${escapeHtml(formatDateTime(item.created_at))}</span>
                </div>
            </div>
        `).join('');
    }

    function escapeHtml(value) {
        return String(value || '')
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    function collectElements(root) {
        return {
            ownerFilter: root.querySelector('[data-role="owner-filter"]'),
            statusFilter: root.querySelector('[data-role="status-filter"]'),
            searchFilter: root.querySelector('[data-role="search-filter"]'),
            teamList: root.querySelector('[data-role="team-list"]'),
            teamsStatus: root.querySelector('[data-role="teams-status"]'),
            metricTotalTeams: root.querySelector('[data-role="metric-total-teams"]'),
            metricTotalSeats: root.querySelector('[data-role="metric-total-seats"]'),
            metricActiveTasks: root.querySelector('[data-role="metric-active-tasks"]'),
            teamDetailTitle: root.querySelector('[data-role="team-detail-title"]'),
            teamDetailStatus: root.querySelector('[data-role="team-detail-status"]'),
            detailOwnerEmail: root.querySelector('[data-role="detail-owner-email"]'),
            detailOwnerId: root.querySelector('[data-role="detail-owner-id"]'),
            detailMembers: root.querySelector('[data-role="detail-members"]'),
            detailSyncStatus: root.querySelector('[data-role="detail-sync-status"]'),
            detailLastSync: root.querySelector('[data-role="detail-last-sync"]'),
            detailSeats: root.querySelector('[data-role="detail-seats"]'),
            teamDetailCallout: root.querySelector('[data-role="team-detail-callout"]'),
            taskLiveStatus: root.querySelector('[data-role="task-live-status"]'),
            taskCurrentUuid: root.querySelector('[data-role="task-current-uuid"]'),
            taskCurrentSummary: root.querySelector('[data-role="task-current-summary"]'),
            taskList: root.querySelector('[data-role="task-list"]'),
        };
    }

    async function initPage(root) {
        const state = deriveInitialTeamState(global.location && global.location.search);
        const elements = collectElements(root);

        elements.ownerFilter.value = state.filters.ownerAccountId || '';
        elements.statusFilter.value = state.filters.status || '';
        elements.searchFilter.value = state.filters.search || '';

        async function loadTeams(path) {
            state.loading.teams = true;
            renderTeams(state, elements);
            const payload = await fetchJson(path || buildTeamsListPath(state.filters));
            state.teams = Array.isArray(payload.items) ? payload.items : [];
            if (!state.selectedTeamId && state.teams.length > 0) {
                state.selectedTeamId = state.teams[0].id;
            }
            state.loading.teams = false;
            renderTeams(state, elements);
            await loadSelectedTeam();
            return payload;
        }

        async function loadSelectedTeam(path) {
            if (!state.selectedTeamId) {
                renderTeamDetail(null, elements);
                renderTasks([], state, elements);
                return null;
            }
            state.loading.detail = true;
            const detail = await fetchJson(path || `/api/team/teams/${state.selectedTeamId}`);
            state.loading.detail = false;
            renderTeamDetail(detail, elements);
            return detail;
        }

        async function loadTasks(path) {
            if (!state.selectedTeamId) {
                renderTasks([], state, elements);
                return null;
            }
            state.loading.tasks = true;
            const payload = await fetchJson(path || `/api/team/tasks?team_id=${state.selectedTeamId}`);
            state.taskItems = Array.isArray(payload.items) ? payload.items : [];
            state.loading.tasks = false;
            renderTasks(state.taskItems, state, elements);
            return payload;
        }

        const acceptedTaskFlow = createAcceptedTaskFlow({
            refreshTeams: loadTeams,
            refreshTeamDetail: loadSelectedTeam,
            refreshTasks: loadTasks,
            onStatusChange: (status, payload) => {
                state.activeTaskUuid = FINISHED_TASK_STATUSES.has(status) ? '' : (payload.task_uuid || state.activeTaskUuid);
                elements.taskLiveStatus.textContent = status;
                renderTasks(state.taskItems, state, elements);
            },
            onError: (error) => {
                elements.taskLiveStatus.textContent = '监听失败';
                elements.taskCurrentSummary.textContent = error && error.message ? error.message : '任务监听失败';
            },
        });

        async function triggerAcceptedTask(path, body) {
            const acceptedPayload = await fetchJson(path, {
                method: 'POST',
                body: JSON.stringify(body),
            });
            state.activeTaskUuid = acceptedPayload.task_uuid || '';
            renderTasks(state.taskItems, state, elements);
            await acceptedTaskFlow.start(acceptedPayload);
        }

        root.addEventListener('click', async (event) => {
            const action = event.target.closest('[data-action]');
            if (action) {
                const actionName = action.getAttribute('data-action');
                if (actionName === 'refresh') {
                    await loadTeams();
                    await loadTasks();
                    return;
                }
                if (actionName === 'discover-owner') {
                    const ownerAccountId = normalizePositiveInt(elements.ownerFilter.value)
                        || normalizePositiveInt((state.teams.find((item) => item.id === state.selectedTeamId) || {}).owner_account_id);
                    if (!ownerAccountId) {
                        global.alert('请先输入母号 ID，或先从已有 Team 中选择一个母号。');
                        return;
                    }
                    await triggerAcceptedTask('/api/team/discovery/run', { ids: [ownerAccountId] });
                    return;
                }
                if (actionName === 'sync-batch') {
                    const selectedIds = state.selectedTeamId ? [state.selectedTeamId] : state.teams.slice(0, 5).map((item) => item.id);
                    if (!selectedIds.length) {
                        global.alert('当前没有可同步的 Team。');
                        return;
                    }
                    await triggerAcceptedTask('/api/team/teams/sync-batch', { ids: selectedIds });
                    return;
                }
            }

            const teamButton = event.target.closest('[data-team-id]');
            if (!teamButton) {
                return;
            }

            state.selectedTeamId = normalizePositiveInt(teamButton.getAttribute('data-team-id'));
            renderTeams(state, elements);
            await loadSelectedTeam();
            await loadTasks();
        });

        root.addEventListener('change', async (event) => {
            const target = event.target;
            if (target === elements.ownerFilter) {
                state.filters.ownerAccountId = normalizePositiveInt(target.value);
                await loadTeams();
                await loadTasks();
            } else if (target === elements.statusFilter) {
                state.filters.status = normalizeText(target.value);
                await loadTeams();
                await loadTasks();
            }
        });

        root.addEventListener('input', async (event) => {
            if (event.target !== elements.searchFilter) {
                return;
            }
            state.filters.search = normalizeText(event.target.value);
            await loadTeams();
            await loadTasks();
        });

        renderTeams(state, elements);
        renderTeamDetail(null, elements);
        renderTasks([], state, elements);
        await loadTeams();
        await loadTasks();
    }

    if (typeof document !== 'undefined') {
        document.addEventListener('DOMContentLoaded', () => {
            const root = document.querySelector('[data-team-shell]');
            if (root) {
                initPage(root).catch((error) => {
                    console.error('初始化 Team 页面失败', error);
                });
            }
        });
    }

    return {
        afterSuccessfulMembershipAction,
        buildMembershipActionRequest,
        buildTeamsListPath,
        createAcceptedTaskFlow,
        deriveInitialTeamState,
        initPage,
        resolveInviteAvailability,
    };
});
