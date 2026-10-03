import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

// Browser-UI fixes ported from the Dispatcharr plugin review: CSP-safe image
// handlers, guide windowing, sports follow identity, admin gating.
const read = name => fs.readFileSync(new URL('../internal/plugin/ui/' + name, import.meta.url), 'utf8');
const app = read('app.js');
const page = read('page.html');
const lineup = read('lineup.js');
// Objects built inside the vm context have that realm's prototypes.
const plain = value => JSON.parse(JSON.stringify(value));

function fn(name) {
  const start = app.search(new RegExp('(?:async )?function ' + name + '\\('));
  assert.ok(start >= 0, name + ' must exist');
  return app.slice(start, app.indexOf('\n}\n', start) + 2);
}
function slice(startMarker, endMarker) {
  const start = app.indexOf(startMarker);
  assert.ok(start >= 0, startMarker + ' must exist');
  const end = app.indexOf(endMarker, start);
  assert.ok(end > start, endMarker + ' must follow ' + startMarker);
  return app.slice(start, end);
}

test('page ships a CSP that forbids inline script and has no inline handlers', () => {
  const csp = (page.match(/http-equiv="Content-Security-Policy" content="([^"]+)"/) || [])[1] || '';
  assert.match(csp, /script-src 'self'(;|$)/);
  assert.match(csp, /object-src 'none'/);
  assert.match(csp, /base-uri 'none'/);
  assert.match(csp, /media-src 'self' https: http: blob:/, 'HLS relay / provider streams must stay playable');
  assert.match(csp, /worker-src 'self' blob:/, 'hls.js / mpegts.js workers');
  assert.match(csp, /frame-src 'self' https: http:/, 'external channel manager iframe');
  assert.doesNotMatch(page, /<script(?![^>]*\bsrc=)[^>]*>/, 'no inline <script> blocks');
  for (const [name, source] of [['app.js', app], ['lineup.js', lineup], ['page.html', page]]) {
    assert.doesNotMatch(source, /\son[a-z]+=\\?["']/, name + ' must not render inline on*= handlers');
  }
});

test('every declarative image action used in markup is registered', () => {
  const ctx = vm.createContext({ document: { addEventListener() {} }, console });
  vm.runInContext(slice('const imageErrorActions', 'function byId('), ctx);
  const errors = vm.runInContext('Object.keys(imageErrorActions)', ctx);
  const loads = vm.runInContext('Object.keys(imageLoadActions)', ctx);
  const used = name => [...app.matchAll(new RegExp('data-img-' + name + '=\\\\?["\']([a-z-]+)', 'g'))].map(match => match[1]);
  assert.ok(used('error').length >= 13);
  for (const action of used('error')) assert.ok(errors.includes(action), 'missing error action ' + action);
  for (const action of used('load')) assert.ok(loads.includes(action), 'missing load action ' + action);
});

test('declarative image handler applies the fallback in the capture phase', () => {
  const listeners = {};
  const ctx = vm.createContext({ document: { addEventListener(type, handler, capture) { listeners[type] = { handler, capture }; } }, console });
  vm.runInContext(slice('const imageErrorActions', 'function byId('), ctx);
  assert.equal(listeners.error.capture, true);
  const fallback = { hidden: true };
  const image = { tagName: 'IMG', dataset: { imgError: 'logo-fallback' }, hidden: false, nextElementSibling: fallback };
  listeners.error.handler({ type: 'error', target: image });
  assert.equal(image.hidden, true);
  assert.equal(fallback.hidden, false);
  const unknown = { tagName: 'IMG', dataset: { imgError: 'constructor' }, hidden: false };
  listeners.error.handler({ type: 'error', target: unknown });
  assert.equal(unknown.hidden, false, 'only registered actions run');
});

test('escapeHTML is null-safe and external links require https', () => {
  const ctx = vm.createContext({});
  vm.runInContext(fn('escapeHTML') + fn('safeHTTPS') + fn('externalLinkAttrs'), ctx);
  assert.equal(ctx.escapeHTML(null), '');
  assert.equal(ctx.escapeHTML(undefined), '');
  assert.equal(ctx.escapeHTML(0), '0');
  assert.equal(ctx.escapeHTML('<a "x">'), '&lt;a &quot;x&quot;&gt;');
  assert.equal(ctx.externalLinkAttrs('javascript:alert(1)'), '');
  assert.equal(ctx.externalLinkAttrs('http://espn.com'), '');
  assert.equal(ctx.externalLinkAttrs('https://espn.com/game?id=1'), ' href="https://espn.com/game?id=1" target="_blank" rel="noopener noreferrer"');
});

function guideContext(responder) {
  const requests = [];
  const ctx = vm.createContext({
    Math, Date, Number, String, Promise, console: { warn() {} },
    state: { app: { programs: [] }, view: 'home' },
    items: value => Array.isArray(value) ? value : [],
    rebuildProgramIndex() {},
    guideWindow: () => ({ start: 0, end: 0 }),
    refreshVisibleGuideBlock() {}, updateSearchPageResults() {}, updateMyTVSearchSurface() {},
    async getJSON(url) { requests.push(url); return responder(url); }
  });
  vm.runInContext(slice('const guideLookbehindSeconds', 'async function refreshSupplementalData('), ctx);
  return { ctx, requests };
}

test('guide windows are requested with start/end, merged, and deduplicated', async () => {
  const now = Math.floor(Date.now() / 1000);
  const { ctx, requests } = guideContext(url => {
    const params = new URLSearchParams(url.split('?')[1]);
    const start = Number(params.get('start'));
    return { programs: [
      { id: 'p-' + start, channelId: 'c', startUnix: start, endUnix: start + 1800 },
      { id: 'shared', channelId: 'c', startUnix: start + 1800, endUnix: start + 3600, title: 'v' + start }
    ] };
  });
  await ctx.fetchGuideWindow(now - 3 * 3600, now + 24 * 3600);
  assert.equal(requests[0], '/dispatcharr/api/guide?start=' + (now - 3 * 3600) + '&end=' + (now + 24 * 3600));
  await ctx.ensureGuideCoverage(now + 7 * 24 * 3600);
  assert.equal(requests[1], '/dispatcharr/api/guide?start=' + (now + 24 * 3600) + '&end=' + (now + 7 * 24 * 3600), 'forward fetch starts at the covered end');
  const ids = plain(ctx.state.app.programs.map(program => program.id));
  assert.deepEqual(ids.slice().sort(), ['p-' + (now - 3 * 3600), 'p-' + (now + 24 * 3600), 'shared'].sort());
  assert.equal(await ctx.ensureGuideCoverage(now + 3600), false, 'already covered windows are not re-fetched');
  assert.equal(requests.length, 2);
});

test('a backend that ignores the window returns the full guide and is treated as full coverage', async () => {
  const now = Math.floor(Date.now() / 1000);
  const full = { programs: [
    { id: 'a', channelId: 'c', startUnix: now, endUnix: now + 1800 },
    { id: 'far', channelId: 'c', startUnix: now + 5 * 24 * 3600, endUnix: now + 5 * 24 * 3600 + 1800 }
  ] };
  const { ctx, requests } = guideContext(() => full);
  await ctx.fetchGuideWindow(now - 3 * 3600, now + 24 * 3600);
  assert.equal(vm.runInContext('guideCoverage.full', ctx), true);
  assert.equal(ctx.state.app.programs.length, 2);
  assert.equal(await ctx.ensureGuideCoverage(now + 7 * 24 * 3600), false, 'no window fetches after full fallback');
  await ctx.fetchGuideWindow(now - 3 * 3600, now + 24 * 3600);
  assert.equal(requests[1], '/dispatcharr/api/guide', 'later refreshes re-read the full guide');
});

function sportsContext(favorites) {
  const saves = [];
  const ctx = vm.createContext({
    state: { app: { preferences: { sportsFavoriteTeams: favorites } }, sports: { events: [] }, sportsLeagueTeams: {} },
    prefsSync: { loaded: true },
    items: value => Array.isArray(value) ? value : [],
    uniqueIDs: values => [...new Set((Array.isArray(values) ? values : []).map(v => String(v || '')).filter(Boolean))],
    lower: value => String(value || '').toLowerCase(),
    sportsFavoriteTeamMap() { return ctx.state.app.preferences.sportsFavoriteTeams; },
    savePrefs(options) { saves.push(options); }
  });
  vm.runInContext(['sportsGamePassSlug', 'sportsTeamHasName', 'sportsTeamIdentityIDs', 'sportsKnownTeamEntities', 'dropPhantomSportsFollows', 'sportsFavoriteTeamMatches'].map(fn).join('\n'), ctx);
  return { ctx, saves };
}

test('sports follows match legacy ids and aliases but never nameless teams', () => {
  const { ctx } = sportsContext({ 'old-id': true, 'gamepass:nfl:detroit-lions': true });
  assert.equal(ctx.sportsFavoriteTeamMatches({ id: 'new-id', name: 'Chicago Bears', legacyIds: ['old-id'] }), true);
  assert.equal(ctx.sportsFavoriteTeamMatches({ id: 'x', name: 'Chicago Bears', aliases: ['old-id'] }), true);
  assert.equal(ctx.sportsFavoriteTeamMatches({ id: 'old-id', name: '' }), false, 'nameless phantoms never match');
  assert.equal(ctx.sportsFavoriteTeamMatches({ id: '', name: 'Detroit Lions' }), true, 'game-pass follows match by name');
});

test('follows that only resolve to nameless teams are purged once sports and prefs are loaded', () => {
  const { ctx, saves } = sportsContext({ ghost: true, real: true, 'gamepass:mlb:ghost': true, unseen: true });
  ctx.state.sports = { incomplete: true, events: [
    { home: { id: 'ghost', name: '' }, away: { id: 'real', name: 'Real FC' } },
    { home: { id: 'gamepass:mlb:ghost', name: '' }, away: { id: 'other', name: 'Other' } }
  ] };
  assert.equal(ctx.dropPhantomSportsFollows(), true);
  assert.deepEqual(plain(Object.keys(ctx.state.app.preferences.sportsFavoriteTeams).sort()), ['gamepass:mlb:ghost', 'real', 'unseen']);
  assert.equal(saves.length, 1);
  ctx.prefsSync.loaded = false;
  ctx.state.app.preferences.sportsFavoriteTeams.ghost = true;
  assert.equal(ctx.dropPhantomSportsFollows(), false, 'never purge before real prefs load');
});

test('refresh controls and forced refreshes are admin-only', () => {
  const ctx = vm.createContext({ isAdminRoute: false, state: { app: { isAdmin: 'true' } }, toasts: [], showAppToast(message) { ctx.toasts.push(message); }, Number });
  vm.runInContext(fn('siloUserIsAdmin') + fn('adminOnlyDenied'), ctx);
  assert.equal(ctx.siloUserIsAdmin(), false, 'only an explicit boolean true grants admin');
  ctx.state.app.isAdmin = true;
  assert.equal(ctx.siloUserIsAdmin(), true);
  assert.equal(ctx.adminOnlyDenied({ status: 403 }, 'force a guide refresh'), true);
  assert.deepEqual(ctx.toasts, ['Only Silo admins can force a guide refresh.']);
  assert.equal(ctx.adminOnlyDenied({ status: 500 }, 'x'), false);
  assert.match(app, /"\/dispatcharr\/api\/sports" \+ \(force && !preparedOnly && siloUserIsAdmin\(\) \? "\?refresh=1"/);
  assert.match(app, /"\/dispatcharr\/api\/events" \+ \(force && siloUserIsAdmin\(\) \? "\?refresh=1"/);
  assert.match(fn('renderRail'), /\[data-guide-refresh\][\s\S]*button\.hidden = !siloUserIsAdmin\(\)/);
});

test('page unload stops sessions with an authenticated keepalive fetch', () => {
  assert.doesNotMatch(app, /navigator\.sendBeacon\(|addEventListener\("beforeunload"/);
  const unload = fn('stopWatchOnUnload');
  assert.match(unload, /coreRequestOptions\(\{/);
  assert.match(unload, /keepalive: true/);
  assert.match(app, /window\.addEventListener\("pagehide"/);
});

test('each video source owns one AbortController for its listeners', () => {
  const aborted = [];
  const ctx = vm.createContext({
    state: {}, Object,
    AbortController: class { constructor() { this.signal = { id: aborted.length + 1 }; } abort() { aborted.push(this.signal.id); } }
  });
  vm.runInContext(fn('videoSourceListenerOptions') + fn('videoSourceListenerSignal'), ctx);
  const first = ctx.videoSourceListenerOptions();
  assert.deepEqual(plain(first({ once: true })), { once: true, signal: { id: 1 } });
  ctx.videoSourceListenerOptions();
  assert.deepEqual(aborted, [1], 'switching sources aborts the previous listeners');
  assert.deepEqual(plain(ctx.videoSourceListenerSignal()), { id: 2 });
});
