import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
const source = fs.readFileSync(new URL('../internal/plugin/ui/sports_companion.js', import.meta.url), 'utf8');
function buffer() {
  const context = vm.createContext({document: {addEventListener() {}}, Date, setTimeout, clearTimeout, setInterval, clearInterval});
  vm.runInContext(source + '\nglobalThis.buffer = new SportsCompanionBuffer()', context);
  return context.buffer;
}
test('delay withholds fresh results, matures snapshots, and forgets another game', () => {
  const b = buffer();
  b.push('knicks', {homeScore: '8'}, 1000);
  assert.equal(b.read('knicks', 60, 60999), null);
  b.push('knicks', {homeScore: '10'}, 31000);
  assert.equal(b.read('knicks', 60, 61000).data.homeScore, '8');
  assert.equal(b.read('knicks', 60, 91000).data.homeScore, '10');
  b.push('mets', {homeScore: '2'}, 92000);
  assert.equal(b.read('knicks', 0, 92000), null);
  assert.equal(b.read('mets', 60, 92000), null);
});

function lifecycle() {
  const root = new URL('../internal/plugin/ui/', import.meta.url);
  const core = fs.readFileSync(new URL('core.js', root), 'utf8');
  const app = fs.readFileSync(new URL('app.js', root), 'utf8');
  const transport = core.includes('async function coreFetch(') ? core : app;
  function declaration(text, name) {
    const start = text.indexOf(name);
    assert.notEqual(start, -1, name);
    return text.slice(start, text.indexOf('\n}', start) + 2);
  }
  let tick = null, now = 1000;
  const requests = [];
  const context = vm.createContext({
    document: {hidden: false, addEventListener() {}},
    state: {view: 'player', currentChannel: {id: 'channel'}, playerSportsOpen: true},
    performance: {now: () => now}, AbortController,
    setTimeout: () => 1, clearTimeout() {},
    setInterval: callback => { tick = callback; return 1; },
    clearInterval: () => { tick = null; },
    byId: id => id === 'player-sports-drawer' ? {} : null,
    prefs: () => ({sportsStatsDelaySeconds: 0}),
    sportsFirstPlayerActive: () => true,
    sportsEventStateID: e => e?.id || '',
    playerSportsEvents: () => [context.game],
    playerSportsCurrentEvent: events => events[0],
    playerSportsChannelMatches: () => true,
    game: {id: 'game-a'},
    route: path => path,
    coreAccessToken: 'fixture-access', coreRefreshPromise: null,
    coreStoredValue: key => ({profile_id: 'profile-7', profile_token: 'fixture-profile'})[key] || '',
    fetch: (url, options) => new Promise((resolve, reject) => requests.push({url, options, resolve, reject})),
    stopCompatibilitySession() {}, stopPlayerSportsRefresh() {},
    resetPlayerSportsDrawerLayout() {}, stopTimeShiftSession() {},
  });
  vm.runInContext(declaration(transport, 'function coreRequestOptions(') + '\n' + declaration(transport, 'async function coreFetch('), context);
  vm.runInContext(source, context);
  // Rendering has its own browser test; these cases exercise real transport,
  // generation admission and the real player's teardown entry point.
  vm.runInContext('renderSportsCompanion = function() {}', context);
  vm.runInContext(declaration(app, 'function stopPlayback('), context);
  const run = code => vm.runInContext(code, context);
  return {context, requests, run, tick: () => tick?.(), advance: ms => { now += ms; }};
}
const settle = () => new Promise(setImmediate);
const success = score => ({ok: true, status: 200, json: async () => ({available: true, homeScore: score, awayScore: '0'})});

test('companion polling carries the authenticated account and profile', async () => {
  const h = lifecycle();
  h.run('startSportsCompanion()');
  await settle();
  assert.equal(h.requests.length, 1);
  const {url, options} = h.requests[0];
  assert.match(url, /\/api\/sports\?game_stats=game-a$/);
  assert.equal(options.headers?.Authorization, 'Bearer fixture-access');
  assert.equal(options.headers?.['X-Profile-Id'], 'profile-7');
  assert.equal(options.headers?.['X-Profile-Token'], 'fixture-profile');
  h.requests[0].resolve(success('7'));
  await settle();
  assert.equal(h.run('sportsCompanion.buffer.read("game-a",0,performance.now()).data.homeScore'), '7');
  h.run('stopSportsCompanion()');
});

test('player teardown aborts polling immediately and a late result cannot publish', async () => {
  const h = lifecycle();
  h.run('startSportsCompanion()');
  await settle();
  h.run('stopPlayback()');
  assert.equal(h.requests[0].options.signal.aborted, true);
  assert.equal(h.run('sportsCompanion.timer'), null);
  h.requests[0].resolve(success('99'));
  await settle();
  assert.equal(h.run('sportsCompanion.buffer.read("game-a",0,performance.now())'), null);
});

test('a new game clears failure state and ignores the previous generation', async () => {
  const h = lifecycle();
  h.run('startSportsCompanion()');
  await settle();
  h.requests[0].reject(new Error('feed down'));
  await settle();
  assert.equal(h.run('sportsCompanion.stale'), true);
  h.context.game = {id: 'game-b'};
  h.tick();
  await settle();
  assert.equal(h.run('sportsCompanion.stale'), false);
  h.run('startSportsCompanion()');
  await settle();
  assert.equal(h.requests[1].options.signal.aborted, true);
  h.requests[2].resolve(success('12'));
  h.requests[1].resolve(success('99'));
  await settle();
  assert.equal(h.run('sportsCompanion.buffer.read("game-b",0,performance.now()).data.homeScore'), '12');
  h.run('stopSportsCompanion()');
});
