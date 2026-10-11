import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

// The preference sync engine lives in app.js; load it the way prefs-sync.test.mjs does.
const source = fs.readFileSync(new URL('../internal/plugin/ui/app.js', import.meta.url), 'utf8');
const fixture = JSON.parse(fs.readFileSync(new URL('../testdata/livetv-v1/fixtures/preferences_v1.json', import.meta.url), 'utf8'));

function slice(startMarker, endMarker) {
  const start = source.indexOf(startMarker);
  assert.ok(start >= 0, startMarker + ' must exist');
  const end = endMarker ? source.indexOf(endMarker, start) : source.length;
  assert.ok(end > start, endMarker + ' must follow ' + startMarker);
  return source.slice(start, end);
}

const clone = value => JSON.parse(JSON.stringify(value));

function setup(remoteValues, profileID) {
  const server = { values: clone(remoteValues), puts: [] };
  const ctx = vm.createContext({
    console, setTimeout, clearTimeout, JSON, Object, Array, String, Number, Math, Date, Promise,
    state: { app: { preferences: null, channels: [] }, view: 'home' },
    pluginInstallationID: '14', localCacheSuffix: '14', isAdminRoute: false,
    items: value => Array.isArray(value) ? value : [],
    uniqueIDs: values => [...new Set((Array.isArray(values) ? values : []).map(v => String(v || '')).filter(Boolean))],
    defaultPrefs: () => ({ favorites: {}, favoriteOrder: [], hiddenCategories: {}, recentChannels: [], keywordPasses: [], categoryBrowse: { sort: 'provider' } }),
    normalizeKeywordPasses: value => Array.isArray(value) ? value : [],
    normalizePreferences() {},
    readRecentSearches: () => [],
    coreStoredValue: key => key === 'profile_id' ? (ctx.viewerProfile || '') : '',
    showAppToast() {}, renderSettings() {}, render() {},
    document: { hidden: false, addEventListener() {} },
    window: { addEventListener() {} },
    localStorage: { setItem() {}, getItem() { return null; } },
    async loadPluginSettingsValues() { return clone(server.values); },
    async corePutNoContent(url, body) {
      server.puts.push(clone(body.values));
      server.values = clone(body.values);
    }
  });
  ctx.viewerProfile = profileID || '';
  vm.runInContext(slice('function mergePrefs(', 'function normalizePreferences('), ctx);
  vm.runInContext(slice('function readSiloPrefsValue(', 'function readAdminSettingsValue('), ctx);
  vm.runInContext(slice('let pluginSettingsWriteChain', 'function saveAdminCategorySettings('), ctx);
  ctx.sync = vm.runInContext('prefsSync', ctx);
  return { ctx, server };
}

// Pure helpers only need the functions themselves; the rest of the context is inert.
const { ctx: pure } = setup({});
// Results are copied out of the vm realm so deepStrictEqual sees plain objects.
const profileView = (remote, pid) => clone(pure.profileView(remote, pid));
const mergeProfileView = (remote, merged, pid) => clone(pure.mergeProfileView(remote, merged, pid));

// Loads this "tab" the way hydrateApp does: stored blob -> the viewer's profile view.
function load(ctx, server) {
  ctx.state.app.preferences = ctx.mergePrefs(ctx.viewerPrefsView(ctx.readSiloPrefsValue(server.values.preferences)));
  ctx.adoptLoadedPrefs(null);
}
const stored = server => JSON.parse(server.values.preferences);

test('profileView takes the scoped keys from the profile section', () => {
  const view = profileView(fixture, 'p2');
  assert.deepEqual(view.favorites, { 'xtream:42': true });
  assert.deepEqual(view.favoriteOrder, ['xtream:42']);
  assert.deepEqual(view.hiddenCategories, { '7': true });
  assert.deepEqual(view.playback, { volume: 0.5 }, 'non-scoped keys stay top-level');
  assert.equal('profiles' in view, false, 'the stored profiles map is never part of the diffed view');
});

test('profileView falls back to the legacy keys when the profile has no section', () => {
  const view = profileView(fixture, 'p1');
  assert.deepEqual(view.favorites, { 'm3u:news.hd': true });
  assert.deepEqual(view.favoriteOrder, ['m3u:news.hd']);
  assert.deepEqual(view.hiddenCategories, {});
  assert.equal('profiles' in view, false);
});

test('profileView with no profile id reads the legacy keys and does not mutate its input', () => {
  const before = clone(fixture);
  const view = profileView(fixture, '');
  assert.deepEqual(view.favorites, { 'm3u:news.hd': true });
  assert.deepEqual(fixture, before);
  view.favorites.extra = true;
  assert.deepEqual(fixture, before, 'the view is a copy');
});

test('mergeProfileView writes a toggled favorite into profiles[pid] and leaves the rest alone', () => {
  const merged = profileView(fixture, 'p1');
  merged.favorites['xtream:7'] = true;
  merged.favoriteOrder.push('xtream:7');
  merged.playback.volume = 0.9;
  const result = mergeProfileView(fixture, merged, 'p1');
  assert.deepEqual(result.profiles.p1.favorites, { 'm3u:news.hd': true, 'xtream:7': true });
  assert.deepEqual(result.profiles.p1.favoriteOrder, ['m3u:news.hd', 'xtream:7']);
  assert.deepEqual(result.profiles.p1.hiddenCategories, {});
  assert.deepEqual(result.favorites, fixture.favorites, 'legacy top-level favorites are unchanged');
  assert.deepEqual(result.favoriteOrder, fixture.favoriteOrder);
  assert.deepEqual(result.hiddenCategories, fixture.hiddenCategories);
  assert.deepEqual(result.profiles.p2, fixture.profiles.p2, 'other profiles are untouched');
  assert.deepEqual(result.playback, { volume: 0.9 }, 'non-scoped keys are written top-level');
});

test('mergeProfileView keeps profile keys outside the scoped set and the other profiles', () => {
  const remote = { favorites: { a: true }, profiles: { p1: { favorites: { b: true }, note: 'kept' }, p2: { favorites: { c: true } } } };
  const merged = profileView(remote, 'p1');
  merged.favorites = { b: true, d: true };
  const result = mergeProfileView(remote, merged, 'p1');
  assert.deepEqual(result.profiles.p1, { favorites: { b: true, d: true }, note: 'kept' });
  assert.deepEqual(result.profiles.p2, { favorites: { c: true } });
  assert.deepEqual(result.favorites, { a: true });
});

test('mergeProfileView applies removals of non-scoped keys', () => {
  const remote = { playback: { volume: 1 }, recentSearches: ['x'], profiles: {} };
  const merged = profileView(remote, 'p1');
  delete merged.recentSearches;
  const result = mergeProfileView(remote, merged, 'p1');
  assert.equal('recentSearches' in result, false);
});

test('with no profile id the output is exactly what is stored today', () => {
  const remote = { favorites: { a: true }, favoriteOrder: ['a'], hiddenCategories: {}, playback: { volume: 1 } };
  const merged = clone(remote);
  merged.favorites.b = true;
  merged.favoriteOrder.push('b');
  const result = mergeProfileView(remote, merged, '');
  assert.deepEqual(result, merged);
  assert.equal('profiles' in result, false, 'no profiles section is invented');
});

test('with no profile id an existing profiles map still survives the save', () => {
  const merged = profileView(fixture, '');
  merged.favorites['m3u:sports.hd'] = true;
  const result = mergeProfileView(fixture, merged, '');
  assert.deepEqual(result.profiles, fixture.profiles);
  assert.deepEqual(result.favorites, { 'm3u:news.hd': true, 'm3u:sports.hd': true });
});

test('the vendored fixture round-trips through view and merge unchanged', () => {
  for (const pid of ['', 'p1', 'p2']) {
    const result = mergeProfileView(fixture, profileView(fixture, pid), pid);
    if (pid === 'p1') {
      // p1 had no section, so its first write adopts the legacy values it was showing.
      assert.deepEqual(result.profiles.p1, { favorites: fixture.favorites, favoriteOrder: fixture.favoriteOrder, hiddenCategories: fixture.hiddenCategories });
      delete result.profiles.p1;
    }
    assert.deepEqual(result, fixture, 'profile ' + JSON.stringify(pid));
  }
});

test('mergePrefs keeps the stored profiles map so a save cannot drop it', () => {
  const { ctx } = setup({});
  assert.deepEqual(clone(ctx.mergePrefs(fixture).profiles), fixture.profiles);
  assert.equal('profiles' in ctx.mergePrefs({ favorites: {} }), false);
});

test('a profile tab loads its own view and saves into its own section', async () => {
  const { ctx, server } = setup({ preferences: JSON.stringify(fixture), other: 'kept' }, 'p1');
  load(ctx, server);
  assert.deepEqual(clone(ctx.state.app.preferences.favorites), { 'm3u:news.hd': true });
  assert.equal('profiles' in ctx.state.app.preferences, false);
  assert.equal(ctx.sync.profileID, 'p1', 'the tab records the profile its prefs were loaded for');
  ctx.state.app.preferences.favorites['xtream:7'] = true;
  ctx.state.app.preferences.favoriteOrder.push('xtream:7');
  ctx.savePrefs({ quiet: true });
  await ctx.flushPrefsSave();
  assert.equal(server.puts.length, 1);
  const saved = stored(server);
  assert.deepEqual(saved.profiles.p1.favorites, { 'm3u:news.hd': true, 'xtream:7': true });
  assert.deepEqual(saved.profiles.p2, fixture.profiles.p2);
  assert.deepEqual(saved.favorites, fixture.favorites);
  assert.deepEqual(saved.favoriteOrder, fixture.favoriteOrder);
  assert.equal(server.values.other, 'kept');
  assert.deepEqual(clone(ctx.state.app.preferences.favorites), { 'm3u:news.hd': true, 'xtream:7': true }, 'the tab keeps the profile view, not the stored shape');
  assert.equal('profiles' in ctx.state.app.preferences, false);
  assert.equal('profiles' in ctx.sync.baseline, false);
  assert.equal(ctx.sync.dirty, false);
});

test('hidden groups are per profile too, whatever the id they are keyed by', async () => {
  const { ctx, server } = setup({ preferences: JSON.stringify(fixture) }, 'p2');
  load(ctx, server);
  assert.deepEqual(clone(ctx.state.app.preferences.hiddenCategories), { '7': true });
  // The category settings page keys hidden groups by category.sourceID.
  ctx.state.app.preferences.hiddenCategories['src:9'] = true;
  delete ctx.state.app.preferences.hiddenCategories['7'];
  ctx.savePrefs({ quiet: true });
  await ctx.flushPrefsSave();
  const saved = stored(server);
  assert.deepEqual(saved.profiles.p2.hiddenCategories, { 'src:9': true });
  assert.deepEqual(saved.hiddenCategories, {});
});

test('a tab on another profile reloads its own favorites, never the saving profile\'s', async () => {
  const a = setup({ preferences: JSON.stringify(fixture) }, 'p1');
  load(a.ctx, a.server);
  a.ctx.state.app.preferences.favorites['xtream:7'] = true;
  a.ctx.state.app.preferences.favoriteOrder.push('xtream:7');
  a.ctx.savePrefs({ quiet: true });
  await a.ctx.flushPrefsSave();

  const b = setup({ preferences: a.server.values.preferences }, 'p2');
  load(b.ctx, b.server);
  assert.deepEqual(clone(b.ctx.state.app.preferences.favorites), { 'xtream:42': true });
  b.ctx.state.app.preferences = b.ctx.mergePrefs({ favorites: { stale: true } });
  await b.ctx.reloadPrefsFromRemote({ force: true });
  assert.deepEqual(clone(b.ctx.state.app.preferences.favorites), { 'xtream:42': true });
  assert.equal('profiles' in b.ctx.state.app.preferences, false);

  // The same profile in another tab does pick the change up.
  const c = setup({ preferences: a.server.values.preferences }, 'p1');
  c.ctx.state.app.preferences = c.ctx.mergePrefs({ favorites: { stale: true } });
  c.ctx.adoptLoadedPrefs(null);
  await c.ctx.reloadPrefsFromRemote({ force: true });
  assert.deepEqual(clone(c.ctx.state.app.preferences.favorites), { 'm3u:news.hd': true, 'xtream:7': true });
});

test('without a profile id a save behaves as before and keeps stored profiles', async () => {
  const { ctx, server } = setup({ preferences: JSON.stringify(fixture) }, '');
  load(ctx, server);
  assert.deepEqual(clone(ctx.state.app.preferences.favorites), { 'm3u:news.hd': true });
  ctx.state.app.preferences.favorites['m3u:sports.hd'] = true;
  ctx.state.app.preferences.favoriteOrder.push('m3u:sports.hd');
  ctx.savePrefs({ quiet: true });
  await ctx.flushPrefsSave();
  const saved = stored(server);
  assert.deepEqual(saved.favorites, { 'm3u:news.hd': true, 'm3u:sports.hd': true });
  assert.deepEqual(saved.profiles, fixture.profiles);
});

test('without a profile id and no stored profiles, no profiles key is written', async () => {
  const { ctx, server } = setup({ preferences: JSON.stringify({ favorites: { a: true }, favoriteOrder: ['a'] }) }, '');
  load(ctx, server);
  ctx.state.app.preferences.favorites.b = true;
  ctx.state.app.preferences.favoriteOrder.push('b');
  ctx.savePrefs({ quiet: true });
  await ctx.flushPrefsSave();
  assert.equal('profiles' in stored(server), false);
});

test('edits made as p1 are never written into p2 when the profile switches before the save', async () => {
  const { ctx, server } = setup({ preferences: JSON.stringify(fixture) }, 'p1');
  load(ctx, server);
  ctx.state.app.preferences.favorites['xtream:7'] = true;
  ctx.state.app.preferences.favoriteOrder.push('xtream:7');
  ctx.savePrefs({ quiet: true });
  ctx.viewerProfile = 'p2';
  await ctx.flushPrefsSave();
  assert.equal(server.puts.length, 0, 'nothing is written for the stale profile');
  assert.deepEqual(clone(ctx.state.app.preferences.favorites), { 'xtream:42': true }, 'p2 view is adopted');
  assert.deepEqual(clone(ctx.state.app.preferences.hiddenCategories), { '7': true });
  assert.equal('profiles' in ctx.state.app.preferences, false);
  assert.equal(ctx.sync.profileID, 'p2');
  assert.equal(ctx.sync.dirty, false, 'the pending diff is discarded');
  assert.deepEqual(clone(ctx.sync.baseline.favorites), { 'xtream:42': true });
  assert.deepEqual(JSON.parse(server.values.preferences), fixture, 'the stored prefs are untouched');
});

test('a profile switch while the save is reading settings still writes nothing', async () => {
  const { ctx, server } = setup({ preferences: JSON.stringify(fixture) }, 'p1');
  load(ctx, server);
  ctx.state.app.preferences.favorites['xtream:7'] = true;
  ctx.savePrefs({ quiet: true });
  const read = ctx.loadPluginSettingsValues;
  ctx.loadPluginSettingsValues = async function() {
    const values = await read();
    ctx.viewerProfile = 'p2';
    return values;
  };
  await ctx.flushPrefsSave();
  assert.equal(server.puts.length, 0);
  assert.deepEqual(clone(ctx.state.app.preferences.favorites), { 'xtream:42': true });
  assert.equal(ctx.sync.profileID, 'p2');
  assert.equal(ctx.sync.dirty, false);
});

test('a save for the same profile still writes normally and keeps the recorded profile', async () => {
  const { ctx, server } = setup({ preferences: JSON.stringify(fixture) }, 'p2');
  load(ctx, server);
  ctx.state.app.preferences.favorites['xtream:7'] = true;
  ctx.state.app.preferences.favoriteOrder.push('xtream:7');
  ctx.savePrefs({ quiet: true });
  await ctx.flushPrefsSave();
  assert.equal(server.puts.length, 1);
  assert.deepEqual(stored(server).profiles.p2.favorites, { 'xtream:42': true, 'xtream:7': true });
  assert.equal(ctx.sync.profileID, 'p2');
});

test('a reload adopts the new profile\'s view and drops edits pending for the old one', async () => {
  const { ctx, server } = setup({ preferences: JSON.stringify(fixture) }, 'p1');
  load(ctx, server);
  ctx.state.app.preferences.favorites['xtream:7'] = true;
  ctx.savePrefs({ quiet: true });
  ctx.viewerProfile = 'p2';
  // Not forced and inside the 5s recheck window: a profile change must still reload.
  await ctx.reloadPrefsFromRemote();
  assert.deepEqual(clone(ctx.state.app.preferences.favorites), { 'xtream:42': true });
  assert.equal(ctx.sync.profileID, 'p2');
  assert.equal(ctx.sync.dirty, false);
  assert.equal(ctx.sync.timer, 0, 'the debounced save for the old profile is cancelled');
  await ctx.flushPrefsSave();
  assert.equal(server.puts.length, 0);
});

test('a reload for the same profile still keeps edits that are pending', async () => {
  const { ctx, server } = setup({ preferences: JSON.stringify(fixture) }, 'p1');
  load(ctx, server);
  ctx.state.app.preferences.favorites['xtream:7'] = true;
  ctx.savePrefs({ quiet: true });
  await ctx.reloadPrefsFromRemote({ force: true });
  assert.deepEqual(clone(ctx.state.app.preferences.favorites), { 'm3u:news.hd': true, 'xtream:7': true });
  assert.equal(ctx.sync.dirty, true);
});
