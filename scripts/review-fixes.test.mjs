import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import appSourceModule from './app-source.cjs';
const { appSource, topLevelFunction } = appSourceModule;

// Browser-UI fixes ported from the Dispatcharr plugin review: CSP-safe image
// handlers, guide windowing, sports follow identity, admin gating.
const read = name => fs.readFileSync(new URL('../internal/plugin/ui/' + name, import.meta.url), 'utf8');
const app = appSource();
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
  // The guide constants live in app.js and the window functions in guide.js.
  const guideUnit = ['guideLookbehindSeconds', 'guideInitialLookaheadSeconds', 'guideSearchLookaheadSeconds', 'guideCoverage']
    .map(name => (app.match(new RegExp('^const ' + name + ' = .*$', 'm')) || [])[0]).join('\n') + '\n'
    + ['guideProgramKey', 'programOverlapsWindow', 'mergeGuideWindowPrograms', 'guideWindowIgnoredByBackend', 'fetchGuideWindow', 'ensureGuideCoverage', 'ensureGuideCoverageForView']
      .map(name => topLevelFunction(app, name)).join('');
  vm.runInContext(guideUnit, ctx);
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

// UX review fixes: friendly errors, inline delete confirmation, role-aware
// empty states, and player Escape handling.
test('readable errors strip wrappers, map provider failures, and fall back to ""', () => {
  const ctx = vm.createContext({ Number, String });
  // The shared readableError (core.js) consults this plugin's providerErrorRules.
  const rulesStart = app.indexOf('const providerErrorPrefix');
  const rules = app.slice(rulesStart, app.indexOf('\n];\n', rulesStart) + 4);
  vm.runInContext(rules + ['rawErrorDetail', 'readableErrorText', 'readableError', 'friendlyError', 'errorSentence', 'adminErrorDetail'].map(name => topLevelFunction(app, name)).join('\n'), ctx);
  const err = (status, message) => ({ status, message });
  assert.equal(ctx.readableError(err(502, 'request failed (502): connection test failed: check account authentication: provider rejected credentials')), 'The provider rejected this username or password. Check them and try again.');
  assert.equal(ctx.readableError(err(502, 'request failed (502): connection test failed: provider rejected credentials (account status Expired)')), 'The provider says this account is expired. Check the account with your provider.');
  assert.equal(ctx.readableError(err(409, 'request failed (409): provider "Main": all playback accounts are at their connection limit')), 'All provider connections are in use. Stop another stream, then try again.');
  assert.equal(ctx.readableError(err(409, 'request failed (409): provider "Main" has no compatible playback accounts')), "This provider has no playback account that can play this channel. Check the provider's playback accounts.", 'a 409 that is not a connection limit gets its own copy');
  assert.equal(ctx.readableError(err(409, 'request failed (409): watch already started')), '', 'an unknown 409 falls back to the caller copy');
  assert.match(ctx.readableError(err(502, 'request failed (502): connection test failed: check account authentication: execute request: dial tcp: no such host')), /could not be reached/);
  assert.equal(ctx.readableError(err(401, 'request failed (401): a signed-in Silo user is required')), 'Your Silo session expired. Refresh the page or sign in again.');
  assert.equal(ctx.readableError(err(400, 'request failed (400): source requires base URL')), 'Source requires base URL.');
  assert.equal(ctx.readableError(err(403, 'request failed (403): admin required')), 'Only Silo admins can do that.');
  assert.equal(ctx.readableError(err(500, 'request failed (500): could not load source registry')), '', 'nothing friendly to add');
  assert.equal(ctx.readableError(Object.assign(new Error('Sports data took too long to respond. Try again.'), { timeout: true })), 'Sports data took too long to respond. Try again.');
  assert.equal(ctx.friendlyError(err(500, 'request failed (500)'), 'Fallback.'), 'Fallback.');
  assert.equal(ctx.errorSentence('Could not save your provider.', err(500, 'x')), 'Could not save your provider.');
  assert.doesNotMatch(ctx.errorSentence('Connection test failed.', err(502, 'request failed (502): connection test failed: provider rejected credentials')), /request failed|connection test failed:/);
  assert.equal(ctx.adminErrorDetail(err(500, 'request failed (500): could not load source registry')), 'request failed (500): could not load source registry', 'admins see the raw detail');
});

// A minimal DOM for the shared inline confirmation (openInlineConfirm, core.js).
function inlineConfirmDOM() {
  const focused = [];
  const timers = [];
  function node(tag) {
    const listeners = {};
    const element = {
      tagName: tag, className: '', hidden: false, parentNode: null, children: [], attributes: {}, innerHTML: '',
      setAttribute(name, value) { this.attributes[name] = String(value); },
      getAttribute(name) { return this.attributes[name] ?? null; },
      addEventListener(type, handler) { listeners[type] = handler; },
      dispatch(type, event) { if (listeners[type]) listeners[type](Object.assign({ preventDefault() {}, stopPropagation() {} }, event)); },
      insertBefore(child, before) { child.parentNode = this; const index = this.children.indexOf(before); this.children.splice(index < 0 ? this.children.length : index, 0, child); },
      removeChild(child) { this.children.splice(this.children.indexOf(child), 1); child.parentNode = null; },
      contains(other) { return other === this || this.children.includes(other); },
      focus() { focused.push(this.name || tag); dom.document.activeElement = this; },
      querySelector(selector) { return selector === '.inline-confirm-no' ? this.cancel : null; },
      closest() { return null; }
    };
    return element;
  }
  const dom = {
    focused, timers, node,
    document: {
      activeElement: null,
      createElement(tag) { const group = node(tag); group.cancel = node('button'); group.cancel.name = 'cancel'; return group; },
      contains(element) { return !!element.parentNode; }
    },
    setTimeout(callback) { timers.push(callback); return timers.length; },
    clearTimeout() {}
  };
  return dom;
}

test('delete confirms inline through the shared helper, and personal providers warn about channels', () => {
  const dom = inlineConfirmDOM();
  const ctx = vm.createContext({ document: dom.document, setTimeout: dom.setTimeout, clearTimeout: dom.clearTimeout, String, escapeHTML: value => String(value) });
  const confirmStart = app.indexOf('const inlineConfirmIdleMs');
  vm.runInContext(app.slice(confirmStart, app.indexOf('\nfunction ', confirmStart)) + ['closeInlineConfirm', 'openInlineConfirm'].map(name => topLevelFunction(app, name)).join('\n'), ctx);
  const row = dom.node('div');
  const trigger = dom.node('button'); trigger.name = 'delete';
  row.insertBefore(trigger, null);
  const confirmed = [];
  ctx.openInlineConfirm(trigger, { title: 'Main IPTV', message: 'Your channels and guide from this provider will be removed.', onConfirm: target => confirmed.push(target) });
  const group = row.children[1];
  assert.equal(trigger.hidden, true, 'the trigger is replaced in place');
  assert.equal(group.attributes['aria-label'], 'Delete Main IPTV?');
  assert.match(group.innerHTML, /<strong>Delete Main IPTV\?<\/strong><small>Your channels and guide from this provider will be removed\.<\/small>/);
  assert.match(group.innerHTML, /class="inline-confirm-yes" aria-label="Confirm delete Main IPTV">Confirm</);
  assert.match(group.innerHTML, /class="inline-confirm-no" aria-label="Cancel deleting Main IPTV">Cancel</);
  assert.deepEqual(dom.focused, ['cancel'], 'focus moves to Cancel');
  group.dispatch('keydown', { key: 'Escape' });
  assert.equal(trigger.hidden, false, 'Escape cancels');
  assert.equal(row.children.length, 1);
  assert.deepEqual(dom.focused, ['cancel', 'delete'], 'and returns focus to Delete');
  ctx.openInlineConfirm(trigger, { title: 'Main IPTV', onConfirm: target => confirmed.push(target) });
  assert.doesNotMatch(row.children[1].innerHTML, /<small>/, 'the message is optional');
  row.children[1].dispatch('click', { target: { closest: selector => (selector === '.inline-confirm-yes' ? {} : null) } });
  assert.deepEqual(confirmed, [trigger], 'Confirm runs onConfirm with the trigger');
  ctx.openInlineConfirm(trigger, { title: 'Main IPTV' });
  dom.timers[dom.timers.length - 1]();
  assert.equal(trigger.hidden, false, 'the prompt reverts after inlineConfirmIdleMs');
  // XC wiring: sources and providers call the shared helper.
  assert.match(fn('handleAdminSourceAction'), /openInlineConfirm\(trigger \|\| document\.querySelector/);
  assert.match(fn('handleAdminSourceAction'), /Your channels and guide from this provider will be removed\./);
  assert.match(app, /const inlineConfirmIdleMs = 6000;/);
  assert.doesNotMatch(app, /data-source-action=\\"delete-confirm\\"/, 'no separate state-driven confirm markup');
});

test('empty catalog copy only points admins at Live TV Admin', () => {
  const ctx = vm.createContext({
    state: { app: { status: { channelCount: 0 } } }, personal: false, missing: false, admin: false,
    userSourcesActive() { return ctx.personal; }, userSourcesMissing() { return ctx.missing; }, siloUserIsAdmin() { return ctx.admin; },
    readableError() { return ''; }, Number, String
  });
  vm.runInContext(['catalogEmptyDetail', 'catalogEmptyActions'].map(name => topLevelFunction(app, name)).join('\n'), ctx);
  assert.doesNotMatch(ctx.catalogEmptyDetail(), /Live TV Admin/);
  ctx.admin = true;
  assert.match(ctx.catalogEmptyDetail(), /Live TV Admin/);
  ctx.admin = false; ctx.personal = true; ctx.missing = true;
  assert.match(ctx.catalogEmptyDetail(), /Settings/);
  assert.match(ctx.catalogEmptyActions(), /data-open-view="settings"/);
});

test('Escape closes the topmost player overlay first and returns focus to its opener', () => {
  const focused = [];
  const opener = { focus() { focused.push('opener'); } };
  const ctx = vm.createContext({
    state: { moreMenuOpen: true, playerGuideOpen: true },
    playerOverlayOpener: opener,
    document: { contains: () => true, querySelector: () => null },
    byId: id => ({ focus() { focused.push(id); } }),
    closePlayerPopovers() { ctx.state.moreMenuOpen = false; },
    renderPlayerGuidePanel() {}, wakePlayerChrome() {},
    togglePlayerSports() { ctx.state.playerSportsOpen = false; }
  });
  vm.runInContext(['focusPlayerOverlayOpener', 'closeTopPlayerOverlay'].map(name => topLevelFunction(app, name)).join('\n'), ctx);
  assert.equal(ctx.closeTopPlayerOverlay(), true);
  assert.equal(ctx.state.moreMenuOpen, false);
  assert.equal(ctx.state.playerGuideOpen, true, 'only the topmost overlay closes');
  assert.deepEqual(focused, ['opener']);
  assert.equal(ctx.closeTopPlayerOverlay(), true);
  assert.deepEqual(focused, ['opener', 'player-guide-button']);
  assert.equal(ctx.closeTopPlayerOverlay(), false, 'nothing open: the caller leaves the player');
  assert.match(app, /if \(!closeTopPlayerOverlay\(\)\) returnFromPlayer\(\);/);
});
