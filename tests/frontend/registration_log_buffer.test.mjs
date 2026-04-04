import test from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

function loadBufferModule() {
  const modulePath = require.resolve('../../static/js/registration_log_buffer.js');
  delete require.cache[modulePath];
  return require(modulePath);
}

test('日志缓冲器会把同一批次消息合并后再统一渲染', () => {
  const { createBufferedLogPump } = loadBufferModule();
  const batches = [];
  let scheduledFlush = null;

  const buffer = createBufferedLogPump({
    renderBatch: (entries) => {
      batches.push(entries.map((entry) => entry.message));
    },
    scheduleFlush: (flush) => {
      scheduledFlush = flush;
      return () => {
        scheduledFlush = null;
      };
    },
  });

  buffer.enqueue('info', 'first-log');
  buffer.enqueue('warning', 'second-log');

  assert.equal(batches.length, 0);
  assert.equal(typeof scheduledFlush, 'function');

  scheduledFlush();

  assert.deepEqual(batches, [['first-log', 'second-log']]);
});

test('日志缓冲器 reset 后会清空去重状态，允许同文案再次渲染', () => {
  const { createBufferedLogPump } = loadBufferModule();
  const batches = [];
  let flush = null;

  const buffer = createBufferedLogPump({
    renderBatch: (entries) => {
      batches.push(entries.map((entry) => `${entry.type}:${entry.message}`));
    },
    scheduleFlush: (scheduled) => {
      flush = scheduled;
      return () => {
        flush = null;
      };
    },
  });

  buffer.enqueue('info', 'same-message');
  flush();
  buffer.enqueue('info', 'same-message');
  assert.deepEqual(batches, [['info:same-message']]);

  buffer.reset();
  buffer.enqueue('info', 'same-message');
  flush();

  assert.deepEqual(batches, [
    ['info:same-message'],
    ['info:same-message'],
  ]);
});

test('即使 requestAnimationFrame 没有执行，默认调度也会通过超时兜底刷新日志', async () => {
  const originalRaf = global.requestAnimationFrame;
  const originalCancelRaf = global.cancelAnimationFrame;

  global.requestAnimationFrame = () => 1;
  global.cancelAnimationFrame = () => {};

  try {
    const { createBufferedLogPump } = loadBufferModule();
    const batches = [];
    const buffer = createBufferedLogPump({
      renderBatch: (entries) => {
        batches.push(entries.map((entry) => entry.message));
      },
    });

    buffer.enqueue('info', 'fallback-log');
    await new Promise((resolve) => setTimeout(resolve, 80));

    assert.deepEqual(batches, [['fallback-log']]);
  } finally {
    global.requestAnimationFrame = originalRaf;
    global.cancelAnimationFrame = originalCancelRaf;
  }
});
