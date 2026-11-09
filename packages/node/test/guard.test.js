const { test } = require('node:test');
const assert = require('node:assert/strict');
const { LoadGuard, loadGuardMiddleware, getDefaultGuard, OverloadedError } = require('../dist/index');

test('LoadGuard: in-flight tracking and queue handoff', async () => {
  const guard = new LoadGuard({ maxConcurrency: 2, maxQueueSize: 5 });
  const rel1 = await guard.acquire();
  const rel2 = await guard.acquire();

  assert.equal(guard.getStats().currentInFlight, 2);

  let queuedResolved = false;
  guard.acquire().then((rel3) => {
    queuedResolved = true;
    rel3();
  });

  assert.equal(guard.getStats().currentQueueSize, 1);
  rel1();
  await new Promise((r) => setTimeout(r, 10));
  assert.equal(queuedResolved, true);
  rel2();
});

test('loadGuardMiddleware: zero-leak on destroyed socket', async () => {
  const mw = loadGuardMiddleware({ maxConcurrency: 10 });
  const guard = getDefaultGuard();
  assert.ok(guard);

  const mockReq = { destroyed: true };
  let called = false;
  await mw(mockReq, {}, () => { called = true; });

  assert.equal(called, false, 'destroyed request must be aborted immediately');
  assert.equal(guard.getStats().currentInFlight, 0, 'no in-flight slot leaked');
});
