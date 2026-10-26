import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import appSourceModule from './app-source.cjs';
const { appSource, topLevelFunction } = appSourceModule;

const app = appSource();

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


test('automatic live rewind uses managed playback with the existing watch lease even when direct credentials are disabled', async () => {
  const sources = [];
  const ctx = vm.createContext({
    URL, window: {location: {href: 'https://silo.example/api/v2/plugin-content/plugins/18/xtream'}},
    state: {view: 'player', timeShiftAttempt: 0},
    stopTimeShiftSession() { ctx.state.timeShiftAttempt++; },
    render() {}, async ensurePlayerLibraries() {},
    liveRewindEnabled: () => true, isRewindableChannel: () => false,
    async startWatch() { return {id: 'watch-1'}; },
    browserStreamURL: (channel, id) => '/xtream/stream?channel_id=' + channel.id + '&session_id=' + id,
    adminSettings: () => ({allowDirectProviderURLs: false}),
    coreFetch: async () => { throw new Error('live channels must not probe the legacy stream route'); },
    setVideoSource: (url, options) => sources.push({url, options}),
    async getJSON() { return {programs: []}; }, byId: () => null,
    showPlayerToast: message => assert.fail(message)
  });
  vm.runInContext(['playChannel', 'showStreamBlockedIfNeeded', 'streamBlockedMessage'].map(name => topLevelFunction(app, name)).join('\n'), ctx);
  await ctx.playChannel({id:'news', streamFormat:'ts'});
  assert.equal(sources.length, 1);
  const url = new URL(sources[0].url, 'https://silo.example');
  assert.equal(url.searchParams.get('session_id'), 'watch-1');
  assert.equal(url.searchParams.get('media_mode'), 'rewind');
});
