import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const app = fs.readFileSync(new URL('../internal/plugin/ui/app.js', import.meta.url), 'utf8');

function fn(source, name) {
  const start = source.indexOf('function ' + name + '(');
  assert.ok(start >= 0, name + ' must exist');
  // Function bodies end at the first top-level closing brace.
  return source.slice(start, source.indexOf('\n}\n', start) + 2);
}

function setup() {
  const calls = [];
  const pending = [];
  let next = 0;
  const ctx = vm.createContext({
    state: { currentSession: null, heartbeat: null, multiviewTiles: [] },
    setInterval: () => 1, clearInterval() {},
    items: value => Array.isArray(value) ? value : [],
    recordWatchPreference() {}, renderRail() {}, startMultiviewHeartbeat() {},
    postJSON(url, body) {
      calls.push({ url, body });
      if (url !== '/dispatcharr/api/watch/start') return Promise.resolve({});
      let resolve;
      const promise = new Promise(r => { resolve = r; });
      pending.push(() => resolve({ session: { id: 'session-' + (++next) } }));
      return promise;
    }
  });
  vm.runInContext(['startWatch', 'stopCurrentWatch', 'startMultiviewWatch', 'stopMultiviewWatch'].map(name => fn(app, name)).join('\n'), ctx);
  return { ctx, calls, pending };
}
const flush = () => new Promise(resolve => setImmediate(resolve));
const stops = calls => calls.filter(call => call.url === '/dispatcharr/api/watch/stop').map(call => call.body.sessionId);

test('a /watch/start that resolves after a newer switch is stopped immediately', async () => {
  const { ctx, calls, pending } = setup();
  const first = ctx.startWatch({ id: 'a', name: 'A' }).catch(error => error);
  const second = ctx.startWatch({ id: 'b', name: 'B' });
  pending[1]();
  await flush();
  pending[0]();
  await flush();
  assert.equal((await second).id, 'session-1');
  assert.equal(ctx.state.currentSession.id, 'session-1', 'B (resolved first) stays current');
  assert.equal((await first).superseded, true, 'the stale start rejects as superseded');
  assert.deepEqual(stops(calls), ['session-2'], 'the stale A session is stopped, not orphaned');
});

test('leaving the player while /watch/start is in flight stops the late session', async () => {
  const { ctx, calls, pending } = setup();
  const started = ctx.startWatch({ id: 'a', name: 'A' }).catch(error => error);
  ctx.stopCurrentWatch('leave_player');
  pending[0]();
  await flush();
  assert.equal(ctx.state.currentSession, null);
  assert.equal((await started).superseded, true);
  assert.deepEqual(stops(calls), ['session-1']);
});

test('a multiview tile removed while /watch/start is in flight stops the late session', async () => {
  const { ctx, calls, pending } = setup();
  const tile = { id: 't1', channel: { id: 'a', name: 'A' } };
  ctx.state.multiviewTiles = [tile];
  const started = ctx.startMultiviewWatch(tile).catch(error => error);
  ctx.stopMultiviewWatch(tile, 'remove_multiview_tile');
  ctx.state.multiviewTiles = [];
  pending[0]();
  await flush();
  assert.ok(!tile.session, 'the removed tile never adopts the late session');
  assert.equal((await started).superseded, true);
  assert.deepEqual(stops(calls), ['session-1']);
});

test('a live multiview tile keeps its session', async () => {
  const { ctx, calls, pending } = setup();
  const tile = { id: 't1', channel: { id: 'a', name: 'A' } };
  ctx.state.multiviewTiles = [tile];
  const started = ctx.startMultiviewWatch(tile);
  assert.equal(ctx.startMultiviewWatch(tile), started, 'concurrent starts share one request');
  pending[0]();
  assert.equal((await started).id, 'session-1');
  assert.equal(tile.session.id, 'session-1');
  assert.deepEqual(stops(calls), []);
});
