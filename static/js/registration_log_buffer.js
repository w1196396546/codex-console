(function (root, factory) {
    const api = factory();
    if (typeof module !== 'undefined' && module.exports) {
        module.exports = api;
    }
    root.RegistrationLogBuffer = api;
})(typeof window !== 'undefined' ? window : globalThis, function () {
    function defaultScheduleFlush(flush) {
        if (typeof requestAnimationFrame === 'function') {
            const id = requestAnimationFrame(flush);
            return () => cancelAnimationFrame(id);
        }
        const id = setTimeout(flush, 16);
        return () => clearTimeout(id);
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
            flushNow,
            reset,
        };
    }

    return {
        createBufferedLogPump,
    };
});
