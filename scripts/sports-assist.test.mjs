import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
const source = fs.readFileSync(new URL('../internal/plugin/ui/sports_assist.js', import.meta.url), 'utf8');
function engine() {
  const context = vm.createContext({document: {addEventListener() {}}, Date});
  vm.runInContext(source + '\nglobalThis.Engine = SportsSwitchEngine;', context);
  return new context.Engine();
}
const candidates = [
  {channelID: 'news', eventID: 'a', label: 'Other game', live: true, followed: false},
  {channelID: 'knicks', eventID: 'b', label: 'Knicks at Celtics', live: true, followed: true}
];
const input = now => ({now, current: 'news', mode: 'auto', candidates, active: true, fresh: true});
test('auto switching gives five full seconds of visible notice before selecting a followed live game', () => {
  const e = engine();
  assert.equal(e.tick(input(1000)).pending.channelID, 'knicks');
  for (let now = 2000; now < 6000; now += 1000) assert.equal(e.tick(input(now)).switchTo, undefined);
  assert.equal(e.tick(input(6000)).switchTo, 'knicks');
});

test('cancel snoozes recommendations and a stay-here lock survives fresh data', () => {
  const e = engine(); e.tick(input(1000)); e.cancel(2000);
  assert.equal(e.tick(input(3000)).pending, undefined);
  assert.equal(e.tick(input(122001)).pending.channelID, 'knicks');
  e.lock('news');
  assert.equal(e.tick(input(123001)).pending, undefined);
  assert.equal(e.tick(input(124001)).switchTo, undefined);
  e.unlock(124001);
  assert.equal(e.tick(input(125001)).pending, undefined);
});
test('manual channel changes and suspended timers cannot complete an old countdown', () => {
  const e = engine(); e.tick(input(1000));
  assert.equal(e.tick(input(6000)).switchTo, undefined, 'a sleeping/background tab needs a new visible countdown');
  assert.equal(e.tick({...input(7000), current: 'other'}).pending, undefined);
  assert.equal(e.tick({...input(9000), current: 'other'}).switchTo, undefined);
});
test('stale, paused, or withdrawn games cancel a countdown; suggest-only never tunes', () => {
  for (const blocked of [{fresh: false}, {active: false}, {candidates: []}]) {
    const e = engine(); e.tick(input(1000));
    assert.equal(e.tick({...input(2000), ...blocked}).pending, undefined);
    assert.equal(e.tick(input(3000)).switchTo, undefined);
  }
  const e = engine();
  assert.equal(e.tick({...input(1000), mode: 'suggest'}).pending.channelID, 'knicks');
  assert.equal(e.tick({...input(9000), mode: 'suggest'}).switchTo, undefined);
  assert.equal(e.tick({...input(10000), mode: 'auto'}).switchTo, undefined, 'enabling automatic switching starts a new countdown');
});
test('an already-followed live game stays selected and equal candidates keep a stable order', () => {
  const e = engine();
  assert.equal(e.tick({...input(1000), current: 'knicks'}).pending, undefined);
  const other = {channelID: 'mets', eventID: 'c', label: 'Mets', live: true, followed: true};
  const another = engine();
  assert.equal(another.tick({...input(1000), candidates: [other, ...candidates]}).pending.channelID, 'knicks');
});

function player() {
  let now = 1000, profile = 'p1'; const requests = [], played = [], roots = new Map();
  const video = {paused: false, ended: false, seeking: false, readyState: 4};
  const settings = {sportsSwitchMode: 'auto'};
  const element = () => ({hidden: false, querySelector: () => ({textContent: ''}), remove() {roots.delete('sports-assist-banner');}, set innerHTML(v) {this.html = v;}});
  const context = vm.createContext({
    document: {hidden: false, addEventListener() {}, createElement: element, querySelector: () => ({appendChild: e => roots.set(e.id, e)})},
    state: {view: 'player', currentChannel: {id: 'news'}, app: {channels: [{id: 'news'}, {id: 'knicks'}]}},
    byId: id => id === 'player' ? video : roots.get(id), prefs: () => settings,
    performance: {now: () => now}, Date: {now: () => 1800000000000},
    sportsCompanionViewer: () => profile, sportsCompanionDelay: () => 0,
    sportsFavoriteTeamMatches: team => !!team?.followed,
    playerSportsEventChannel: event => event.channels[0], sportsTeamName: team => team?.name || 'Team',
    items: value => Array.isArray(value) ? value : [], escapeHTML: String,
    playChannel: channel => played.push(channel.id), route: value => value,
    AbortController, setTimeout: () => 1, clearTimeout() {},
    coreFetch: (url, options) => new Promise(resolve => requests.push({url, options, resolve})),
  });
  vm.runInContext(source, context);
  const run = code => vm.runInContext(code, context);
  const payload = {updatedAtUnix: 1800000000, events: [
    {id: 'followed', live: true, home: {name: 'Knicks', followed: true}, away: {name: 'Celtics'}, channels: [{id: 'knicks', score: 90}]}
  ]};
  return {context, requests, played, video, settings, run, tick: () => run('sportsAssistTick()'), advance: () => {now += 1000;}, profile: value => {profile = value;}, payload};
}
const settle = () => new Promise(setImmediate);
test('the live adapter tunes the authorized channel after notice and stops when playback is paused', async () => {
  const p = player(); p.tick(); await settle();
  assert.equal(p.requests.length, 1);
  p.requests[0].resolve({ok: true, json: async () => p.payload}); await settle();
  p.advance(); p.tick();
  for (let i = 0; i < 5; i++) {p.advance(); p.tick();}
  assert.deepEqual(p.played, ['knicks']);
  const paused = player(); paused.tick();
  paused.requests[0].resolve({ok: true, json: async () => paused.payload}); await settle();
  paused.advance(); paused.tick(); paused.video.paused = true;
  for (let i = 0; i < 8; i++) {paused.advance(); paused.tick();}
  assert.deepEqual(paused.played, []);
});
test('leaving the player or changing profile aborts pending data and cannot publish an old suggestion', async () => {
  const p = player(); p.tick(); p.profile('p2'); p.advance(); p.tick();
  assert.equal(p.requests[0].options.signal.aborted, true);
  p.requests[0].resolve({ok: true, json: async () => p.payload}); await settle();
  assert.equal(p.run('sportsAssist.payload'), null);
  p.context.state.view = 'sports'; p.tick();
  assert.equal(p.requests[1].options.signal.aborted, true);
  assert.deepEqual(p.played, []);
});

test('the player adapter refuses missing channel confidence and buffering or rewound playback', async () => {
  for (const condition of ['confidence', 'buffering', 'rewound']) {
    const p = player();
    if (condition === 'confidence') delete p.payload.events[0].channels[0].score;
    if (condition === 'buffering') p.video.readyState = 2;
    if (condition === 'rewound') p.context.state.timeShiftSession = {rewound: true};
    p.tick(); p.requests[0].resolve({ok: true, json: async () => p.payload}); await settle();
    for (let i=0; i<8; i++) {p.advance(); p.tick();}
    assert.deepEqual(p.played, [], condition);
  }
});

test('switching is off by default and starts no additional sports requests', () => {
  const p = player(); delete p.settings.sportsSwitchMode; p.tick();
  assert.equal(p.requests.length, 0); assert.deepEqual(p.played, []);
});

test('practice follows the same countdown, cancel and lock rules using only simulated channel identities', () => {
  const context = vm.createContext({document: {addEventListener() {}}, Date});
  vm.runInContext(source + '\nglobalThis.Practice = SportsSwitchPractice;', context);
  const p = new context.Practice(1000);
  assert.equal(p.tick(1000).pending.channelID, 'practice:harbor');
  for (let now=2000; now<=6000; now+=1000) p.tick(now);
  assert.equal(p.current, 'practice:harbor');
  p.next(7000); const next = p.tick(7000).pending.channelID;
  assert.match(next, /^practice:/);
  p.engine.cancel(7500);
  for (let now=8000; now<=16000; now+=1000) p.tick(now);
  assert.equal(p.current, 'practice:harbor');
  p.next(17000); p.engine.lock(p.current);
  for (let now=17000; now<=25000; now+=1000) assert.equal(p.tick(now).pending, undefined);
  p.restart(26000);
  assert.equal(p.tick(26000).pending.channelID, 'practice:harbor');
  assert.equal(p.current, 'practice:waiting');
});

test('a followed delayed game is held until its buffered final arrives even if the live schedule has dropped it', async () => {
  const p = player(); const data = {live: true, completed: false};
  p.context.sportsCompanionDelay = () => 60;
  p.context.sportsEventStateID = event => event.id;
  p.context.sportsCompanion = {viewer: 'p1', event: {id: 'ended-ahead', home: {name: 'Followed', followed: true}, away: {name: 'Other'}, channels: [{id: 'news', score: 90}]}, buffer: {read: () => ({data})}};
  p.tick(); p.requests[0].resolve({ok: true, json: async () => p.payload}); await settle();
  for (let i=0; i<8; i++) {p.advance(); p.tick();}
  assert.deepEqual(p.played, [], 'the schedule cannot cut off a delayed followed game');
  data.completed = true;
  for (let i=0; i<6; i++) {p.advance(); p.tick();}
  assert.deepEqual(p.played, ['knicks']);
});
test('a different event on the same suggested channel receives a new full countdown', () => {
  const e = engine(); e.tick(input(1000));
  const changed = candidates.map(row => row.channelID === 'knicks' ? {...row, eventID: 'new-game', label: 'A new matchup'} : row);
  e.tick({...input(2000), candidates: changed});
  for (let now=3000; now<7000; now+=1000) assert.equal(e.tick({...input(now), candidates: changed}).switchTo, undefined);
  assert.equal(e.tick({...input(7000), candidates: changed}).switchTo, 'knicks');
});
