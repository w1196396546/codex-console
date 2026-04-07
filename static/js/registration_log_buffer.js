(function (root, factory) {
    const api = factory();
    if (typeof module !== 'undefined' && module.exports) {
        module.exports = api;
    }
    root.RegistrationLogBuffer = api;
})(typeof window !== 'undefined' ? window : globalThis, function () {
    function defaultScheduleFlush(flush) {
        let settled = false;
        let rafId = null;
        let timeoutId = null;

        function run() {
            if (settled) {
                return;
            }
            settled = true;
            if (rafId !== null && typeof cancelAnimationFrame === 'function') {
                cancelAnimationFrame(rafId);
            }
            if (timeoutId !== null) {
                clearTimeout(timeoutId);
            }
            flush();
        }

        if (typeof requestAnimationFrame === 'function') {
            rafId = requestAnimationFrame(run);
        }
        // 某些桌面容器/后台标签页里 requestAnimationFrame 可能被节流甚至不触发，
        // 这里补一个超时兜底，避免日志一直卡在队列里不显示。
        timeoutId = setTimeout(run, 32);

        return () => {
            settled = true;
            if (rafId !== null && typeof cancelAnimationFrame === 'function') {
                cancelAnimationFrame(rafId);
            }
            if (timeoutId !== null) {
                clearTimeout(timeoutId);
            }
        };
    }

    function createBufferedLogPump(options) {
        const renderBatch = options && typeof options.renderBatch === 'function'
            ? options.renderBatch
            : () => {};
        const scheduleFlush = options && typeof options.scheduleFlush === 'function'
            ? options.scheduleFlush
            : defaultScheduleFlush;
        const dedupeLimit = Math.max(1, Number(options && options.dedupeLimit) || 1000);

        let queue = [];
        let displayedKeys = new Set();
        let cancelScheduledFlush = null;

        function trimDisplayedKeys() {
            if (displayedKeys.size <= dedupeLimit) {
                return;
            }
            const keys = Array.from(displayedKeys);
            const deleteCount = Math.max(1, Math.floor(dedupeLimit / 2));
            keys.slice(0, deleteCount).forEach((key) => displayedKeys.delete(key));
        }

        function flushNow() {
            if (cancelScheduledFlush) {
                cancelScheduledFlush();
                cancelScheduledFlush = null;
            }
            if (!queue.length) {
                return;
            }
            const batch = queue;
            queue = [];
            renderBatch(batch);
        }

        function ensureScheduled() {
            if (cancelScheduledFlush) {
                return;
            }
            cancelScheduledFlush = scheduleFlush(() => {
                cancelScheduledFlush = null;
                flushNow();
            });
        }

        function enqueue(type, message) {
            const logKey = `${type}:${message}`;
            if (displayedKeys.has(logKey)) {
                return false;
            }
            displayedKeys.add(logKey);
            trimDisplayedKeys();
            queue.push({ type, message });
            ensureScheduled();
            return true;
        }

        function enqueueMany(entries) {
            const normalizedEntries = Array.isArray(entries) ? entries : [];
            let appended = 0;

            normalizedEntries.forEach((entry) => {
                if (!entry || typeof entry.message !== 'string') {
                    return;
                }
                const type = typeof entry.type === 'string' ? entry.type : 'info';
                const logKey = `${type}:${entry.message}`;
                if (displayedKeys.has(logKey)) {
                    return;
                }
                displayedKeys.add(logKey);
                queue.push({ type, message: entry.message });
                appended += 1;
            });

            if (!appended) {
                return 0;
            }

            trimDisplayedKeys();
            ensureScheduled();
            return appended;
        }

        function reset() {
            queue = [];
            displayedKeys = new Set();
            if (cancelScheduledFlush) {
                cancelScheduledFlush();
                cancelScheduledFlush = null;
            }
        }

        return {
            enqueue,
            enqueueMany,
            flushNow,
            reset,
        };
    }

    return {
        createBufferedLogPump,
    };
});
