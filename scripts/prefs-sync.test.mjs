import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

// Ported from the Dispatcharr plugin; the preference sync engine lives in app.js.
const source = fs.readFileSync(new URL('../internal/plugin/ui/app.js', import.meta.url), 'utf8');

function slice(startMarker, endMarker) {
  const start = source.indexOf(startMarker);
  assert.ok(start >= 0, startMarker + ' must exist');
  const end = endMarker ? source.indexOf(endMarker, start) : source.length;
  assert.ok(end > start, endMarker + ' must follow ' + startMarker);
  return source.slice(start, end);
}

const clone = value => JSON.parse(JSON.stringify(value));
// Objects built inside the vm context have that realm's prototypes.
const plain = clone;

function setup(remoteValues, options = {}) {
  const server = { values: clone(remoteValues), reads: 0, puts: [], urls: [], failReads: 0, failPuts: 0 };
  const ctx = vm.createContext({
    console, setTimeout, clearTimeout, JSON, Object, Array, String, Number, Math, Date, Promise,
    state: { app: { preferences: null, channels: [] }, view: 'home' },
    pluginInstallationID: '14', localCacheSuffix: '14', isAdminRoute: false,
    toasts: [], renders: 0,
    items: value => Array.isArray(value) ? value : [],
    uniqueIDs: values => [...new Set((Array.isArray(values) ? values : []).map(v => String(v || '')).filter(Boolean))],
    defaultPrefs: () => ({ favorites: {}, favoriteOrder: [], hiddenCategories: {}, recentChannels: [], keywordPasses: [], categoryBrowse: { sort: 'provider' } }),
    normalizeKeywordPasses: value => Array.isArray(value) ? value : [],
    normalizePreferences() {},
    readRecentSearches: () => [],
    showAppToast(message) { ctx.toasts.push(message); },
    renderSettings() {}, render() { ctx.renders += 1; },
    document: { hidden: false, addEventListener() {} },
    window: { addEventListener() {} },
    localStorage: { setItem() {}, getItem() { return null; } },
    async loadPluginSettingsValues() {
      server.reads += 1;
      if (server.failReads > 0) { server.failReads -= 1; throw Object.assign(new Error('read failed'), { status: 500 }); }
      return clone(server.values);
    },
    async corePutNoContent(url, body) {
      if (server.failPuts > 0) { server.failPuts -= 1; throw new Error('put failed'); }
      server.urls.push(url);
      server.puts.push(clone(body.values));
      server.values = clone(body.values);
    }
  });
  vm.runInContext(slice('function mergePrefs(', 'function normalizePreferences('), ctx);
  vm.runInContext(slice('function readSiloPrefsValue(', 'function readAdminSettingsValue('), ctx);
  vm.runInContext(slice('let pluginSettingsWriteChain', 'function saveAdminCategorySettings('), ctx);
  ctx.sync = vm.runInContext('prefsSync', ctx);
  if (options.loaded !== false) {
    ctx.state.app.preferences = ctx.remotePrefsFromValues(server.values);
    ctx.adoptLoadedPrefs(null);
  }
  return { ctx, server };
}
const remotePrefs = server => JSON.parse(server.values.preferences);

test('a failed settings read never writes (other keys are not wiped)', async () => {
  const { ctx, server } = setup({ preferences: '{}', adminCategorySettings: '{"keep":true}' });
  server.failReads = 1;
  await assert.rejects(ctx.savePluginSettingValue('recentSearches', '[]'));
  assert.equal(server.puts.length, 0);
  await ctx.savePluginSettingValue('recentSearches', '["x"]');
  assert.equal(server.puts.length, 1);
  assert.equal(server.urls[0], '/api/v1/settings/plugins/14');
  assert.equal(server.values.adminCategorySettings, '{"keep":true}');
});

test('prefs are not persisted until the real prefs load, then pending edits merge onto them', async () => {
  const stored = { favorites: { espn: true }, favoriteOrder: ['espn'], hiddenCategories: { kids: true } };
  const { ctx, server } = setup({ preferences: JSON.stringify(stored) }, { loaded: false });
  ctx.state.app.preferences = ctx.defaultPrefs();
  ctx.sync.baseline = clone(ctx.state.app.preferences);
  ctx.state.app.preferences.favorites.cnn = true;
  ctx.state.app.preferences.favoriteOrder.push('cnn');
  ctx.savePrefs({ quiet: true });
  await new Promise(resolve => setTimeout(resolve, 450));
  assert.equal(server.puts.length, 0, 'defaults must never be written over unloaded prefs');
  const local = ctx.state.app.preferences;
  ctx.state.app.preferences = ctx.remotePrefsFromValues(server.values);
  ctx.adoptLoadedPrefs(local);
  await ctx.flushPrefsSave();
  assert.equal(server.puts.length, 1);
  const saved = remotePrefs(server);
  assert.deepEqual(saved.favorites, { espn: true, cnn: true });
  assert.deepEqual(saved.favoriteOrder, ['espn', 'cnn']);
  assert.deepEqual(saved.hiddenCategories, { kids: true });
  ctx.sync.retryTimer && clearTimeout(ctx.sync.retryTimer);
});

test('saves are debounced into one write and re-read so other tabs\' changes survive', async () => {
  const { ctx, server } = setup({ preferences: JSON.stringify({ favorites: { a: true }, favoriteOrder: ['a'] }), other: 'kept' });
  // Another tab adds "b" and hides a category after this tab loaded.
  server.values.preferences = JSON.stringify({ favorites: { a: true, b: true }, favoriteOrder: ['a', 'b'], hiddenCategories: { news: true } });
  ctx.state.app.preferences.favorites.c = true;
  ctx.state.app.preferences.favoriteOrder.push('c');
  ctx.savePrefs();
  delete ctx.state.app.preferences.favorites.a;
  ctx.state.app.preferences.favoriteOrder = ['c'];
  ctx.savePrefs();
  await new Promise(resolve => setTimeout(resolve, 450));
  await ctx.flushPrefsSave();
  assert.equal(server.puts.length, 1, 'rapid saves collapse into a single write');
  const saved = remotePrefs(server);
  assert.deepEqual(saved.favorites, { b: true, c: true });
  assert.deepEqual(saved.favoriteOrder, ['b', 'c'], 'additions append to the remote order; removals apply');
  assert.deepEqual(saved.hiddenCategories, { news: true });
  assert.equal(server.values.other, 'kept');
  assert.deepEqual(plain(ctx.state.app.preferences.favorites), { b: true, c: true }, 'this tab adopts the merged result');
});

test('recent channels prepend this tab\'s additions ahead of the remote list', async () => {
  const { ctx, server } = setup({ preferences: JSON.stringify({ recentChannels: ['a', 'b'] }) });
  server.values.preferences = JSON.stringify({ recentChannels: ['x', 'a', 'b'] });
  ctx.state.app.preferences.recentChannels = ['c', 'a', 'b'];
  ctx.savePrefs({ quiet: true });
  await ctx.flushPrefsSave();
  assert.deepEqual(remotePrefs(server).recentChannels, ['c', 'a', 'b', 'x']);
});

test('keys outside the normalized set (sports follows, profile selection) survive a load and save', async () => {
  const stored = { favorites: {}, sportsFavoriteLeagues: { nfl: true }, profileSelection: { mode: 'selected', profileIds: ['p1'] }, sportsSpoilersHidden: true };
  const { ctx, server } = setup({ preferences: JSON.stringify(stored) });
  assert.deepEqual(plain(ctx.state.app.preferences.sportsFavoriteLeagues), { nfl: true });
  ctx.state.app.preferences.favorites.z = true;
  ctx.savePrefs({ quiet: true });
  await ctx.flushPrefsSave();
  const saved = remotePrefs(server);
  assert.deepEqual(saved.sportsFavoriteLeagues, { nfl: true });
  assert.deepEqual(saved.profileSelection, { mode: 'selected', profileIds: ['p1'] });
  assert.equal(saved.sportsSpoilersHidden, true);
  assert.deepEqual(saved.favorites, { z: true });
});

test('the known phantom follow is removed on load and persisted through the save queue', async () => {
  const stored = { sportsFavoriteTeams: { 'sports-team:cbe5cfdf7c2118a9': true, lions: true } };
  const { ctx, server } = setup({ preferences: JSON.stringify(stored) });
  assert.deepEqual(plain(ctx.state.app.preferences.sportsFavoriteTeams), { lions: true });
  assert.equal(server.puts.length, 0, 'nothing is written synchronously');
  await ctx.flushPrefsSave();
  assert.deepEqual(remotePrefs(server).sportsFavoriteTeams, { lions: true });
});

test('a failed save keeps the edit pending for retry', async () => {
  const { ctx, server } = setup({ preferences: '{}' });
  server.failPuts = 1;
  ctx.state.app.preferences.favorites.x = true;
  ctx.savePrefs({ quiet: true });
  await ctx.flushPrefsSave();
  assert.equal(ctx.sync.dirty, true);
  assert.equal(ctx.state.profileSaveStatus, 'error');
  if (ctx.sync.retryTimer) { clearTimeout(ctx.sync.retryTimer); ctx.sync.retryTimer = 0; }
  await ctx.flushPrefsSave();
  assert.deepEqual(remotePrefs(server).favorites, { x: true });
  assert.equal(ctx.sync.dirty, false);
});
