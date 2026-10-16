async function loadPluginSettingsValues() {
  if (!pluginInstallationID) return null;
  const payload = await coreGetJSON("/api/v1/settings/plugins/" + encodeURIComponent(pluginInstallationID));
  return payload && payload.values ? payload.values : {};
}
async function loadUserPrefs() {
  const values = await loadPluginSettingsValues();
  return viewerPrefsView(readSiloPrefsValue(values ? values.preferences : ""));
}
async function loadAdminCategorySettings() {
  return readAdminSettingsValue(await getJSON(adminSettingsURL()));
}
function adminSettingsURL() {
  return "/dispatcharr/api/admin-settings";
}
// Every plugin-settings write goes through one queue. A write always starts
// from a fresh, successful read so keys owned by other features (or other
// tabs) are never dropped; a failed read fails the write instead.
let pluginSettingsWriteChain = Promise.resolve();
function queuePluginSettingsWrite(task) {
  const run = pluginSettingsWriteChain.then(task, task);
  pluginSettingsWriteChain = run.catch(function() {});
  return run;
}
async function writePluginSettingsValues(update) {
  if (!pluginInstallationID) throw new Error("plugin installation settings unavailable");
  const loaded = await loadPluginSettingsValues();
  const values = Object.assign({}, loaded || {});
  const next = update(values) || values;
  await corePutNoContent("/api/v1/settings/plugins/" + encodeURIComponent(pluginInstallationID), { values: next });
  return next;
}
function savePluginSettingValue(key, value) {
  return queuePluginSettingsWrite(function() {
    return writePluginSettingsValues(function(values) {
      values[key] = value;
      return values;
    });
  });
}
async function persistAdminCategorySettingsInSilo(settings) {
  if (!pluginInstallationID) throw new Error("plugin installation settings unavailable");
  await corePutNoContent("/api/v1/admin/plugins/installations/" + encodeURIComponent(pluginInstallationID) + "/config", {
    key: "category_settings",
    value: settings
  });
}
// Preference sync. `prefsSync.baseline` is the last remote state this tab
// knows about (normalized). Local edits are the diff between it and
// state.app.preferences; every save re-reads the remote blob and applies only
// that diff, so concurrent tabs/devices do not overwrite each other.
const prefsSaveDebounceMs = 400;
const prefsMapKeys = ["favorites", "autoFavorites", "hiddenCategories", "sportsFavoriteTeams", "sportsFavoriteTeamLabels", "sportsFavoriteLeagues", "sportsPreferredChannels", "sportsPreferredNetworks", "featuredEvents", "continueWatching", "customGroupMemberships", "playback", "categoryBrowse"];
const prefsUnionListKeys = { favoriteOrder: 0, recentSearches: 12, recentChannels: 24 };
// `profileID` is the Silo profile state.app.preferences and the baseline were
// loaded for; a save only ever writes into that profile's section.
const prefsSync = { loaded: false, profileID: "", baseline: null, dirty: false, quiet: true, timer: 0, retryTimer: 0, loading: null, lastRemoteCheck: 0, channel: null, tabID: Math.random().toString(36).slice(2) };
function prefsSnapshot(value) { return JSON.parse(JSON.stringify(value == null ? {} : value)); }
function prefsValueEqual(left, right) { return JSON.stringify(left === undefined ? null : left) === JSON.stringify(right === undefined ? null : right); }
function prefsPlainObject(value) { return !!value && typeof value === "object" && !Array.isArray(value); }
// Ordered ID lists: apply this tab's additions/removals to the remote list.
// favoriteOrder appends additions (unless this tab reordered, then its order
// wins); recent* lists are most-recent-first, so additions go in front.
function mergePrefsList(key, remote, baseline, local) {
  remote = uniqueIDs(remote);
  baseline = uniqueIDs(baseline);
  local = uniqueIDs(local);
  const inLocal = {};
  const inBaseline = {};
  local.forEach(function(value) { inLocal[value] = true; });
  baseline.forEach(function(value) { inBaseline[value] = true; });
  const added = local.filter(function(value) { return !inBaseline[value]; });
  const kept = remote.filter(function(value) { return !inBaseline[value] || inLocal[value]; });
  const sharedLocal = local.filter(function(value) { return inBaseline[value]; });
  const sharedBaseline = baseline.filter(function(value) { return inLocal[value]; });
  let merged;
  if (key !== "favoriteOrder") merged = uniqueIDs(added.concat(sharedLocal, kept));
  else if (!prefsValueEqual(sharedLocal, sharedBaseline)) merged = uniqueIDs(local.concat(kept));
  else merged = uniqueIDs(kept.concat(added));
  const limit = prefsUnionListKeys[key];
  return limit ? merged.slice(0, limit) : merged;
}
function applyPrefsDiff(remote, baseline, local) {
  const result = prefsSnapshot(remote);
  baseline = baseline || {};
  local = local || {};
  const keys = uniqueIDs(Object.keys(baseline).concat(Object.keys(local)));
  keys.forEach(function(key) {
    if (prefsValueEqual(local[key], baseline[key])) return;
    if (prefsMapKeys.indexOf(key) !== -1 && prefsPlainObject(local[key])) {
      const before = prefsPlainObject(baseline[key]) ? baseline[key] : {};
      const target = prefsPlainObject(result[key]) ? result[key] : {};
      uniqueIDs(Object.keys(before).concat(Object.keys(local[key]))).forEach(function(subKey) {
        if (prefsValueEqual(local[key][subKey], before[subKey])) return;
        if (Object.prototype.hasOwnProperty.call(local[key], subKey)) target[subKey] = prefsSnapshot(local[key][subKey]);
        else delete target[subKey];
      });
      result[key] = target;
      return;
    }
    if (Object.prototype.hasOwnProperty.call(prefsUnionListKeys, key) && Array.isArray(local[key])) {
      result[key] = mergePrefsList(key, items(result[key]), items(baseline[key]), local[key]);
      return;
    }
    if (local[key] === undefined) delete result[key];
    else result[key] = prefsSnapshot(local[key]);
  });
  if (result.favorites && Array.isArray(result.favoriteOrder)) {
    result.favoriteOrder = result.favoriteOrder.filter(function(id) { return !!result.favorites[id]; });
  }
  return result;
}
// Raw stored shape: legacy top-level keys plus the per-profile `profiles` map.
// Use viewerPrefsView/profileView for what a tab shows and diffs against.
function remotePrefsFromValues(values) {
  return mergePrefs(readSiloPrefsValue(values && values.preferences ? values.preferences : "") || defaultPrefs());
}
// Favorites, their order and hidden groups are kept per Silo profile under
// preferences.profiles[profileID]. Everything else stays top-level. A profile
// with no section yet shows the legacy top-level values, and so does a viewer
// with no profile id.
const profileScopedPrefKeys = ["favorites", "favoriteOrder", "hiddenCategories"];
function activeViewerProfileID() { return coreStoredValue("profile_id") || ""; }
function prefsProfileSection(remote, profileID) {
  const section = profileID && remote && prefsPlainObject(remote.profiles) ? remote.profiles[profileID] : null;
  return prefsPlainObject(section) ? section : null;
}
// A copy of the stored prefs as the given profile sees them. `profiles` is
// removed so the per-profile map is never diffed or edited by the tab.
function profileView(remote, profileID) {
  const view = prefsSnapshot(remote);
  delete view.profiles;
  const section = prefsProfileSection(remote, profileID);
  if (!section) return view;
  profileScopedPrefKeys.forEach(function(key) {
    if (Object.prototype.hasOwnProperty.call(section, key)) view[key] = prefsSnapshot(section[key]);
    else delete view[key];
  });
  return view;
}
function viewerPrefsView(remote) {
  return remote ? profileView(remote, activeViewerProfileID()) : null;
}
// Inverse of profileView: `merged` is the viewer's view after a save diff and
// `remote` the raw stored prefs it was merged against. Scoped keys go into
// profiles[profileID] (the legacy top-level ones keep the stored values), every
// other key comes from `merged`, and the other profiles are carried over.
function mergeProfileView(remote, merged, profileID) {
  const result = prefsSnapshot(merged);
  delete result.profiles;
  if (remote && prefsPlainObject(remote.profiles)) result.profiles = prefsSnapshot(remote.profiles);
  if (!profileID) return result;
  const section = Object.assign({}, prefsProfileSection(remote, profileID));
  profileScopedPrefKeys.forEach(function(key) {
    if (remote && Object.prototype.hasOwnProperty.call(remote, key)) result[key] = prefsSnapshot(remote[key]);
    else delete result[key];
    if (Object.prototype.hasOwnProperty.call(merged, key)) section[key] = prefsSnapshot(merged[key]);
    else delete section[key];
  });
  result.profiles = Object.assign({}, result.profiles);
  result.profiles[profileID] = section;
  return result;
}
// Silo switches profiles inside its SPA without reloading the page, so the
// viewer can change while this tab still holds the previous profile's view.
function prefsProfileSwitched() {
  return prefsSync.loaded && prefsSync.profileID !== activeViewerProfileID();
}
// Edits pending for a profile the viewer has left must not be saved anywhere.
function discardPendingPrefsEdits() {
  if (prefsSync.timer) { clearTimeout(prefsSync.timer); prefsSync.timer = 0; }
  prefsSync.dirty = false;
  prefsSync.quiet = true;
}
// Called after state.app.preferences has been replaced by freshly loaded,
// normalized remote prefs. Re-applies edits made while prefs were not loaded.
// `profileID` is the profile that view was built for (default: the current one).
function adoptLoadedPrefs(previousLocal, profileID) {
  const nextProfileID = profileID === undefined ? activeViewerProfileID() : profileID;
  if (prefsSync.loaded && prefsSync.profileID !== nextProfileID) discardPendingPrefsEdits();
  const previousBaseline = prefsSync.baseline;
  const hadPendingEdits = prefsSync.dirty && previousBaseline && previousLocal;
  prefsSync.loaded = true;
  prefsSync.profileID = nextProfileID;
  prefsSync.baseline = prefsSnapshot(state.app.preferences);
  prefsSync.lastRemoteCheck = Date.now();
  if (prefsSync.retryTimer) { clearTimeout(prefsSync.retryTimer); prefsSync.retryTimer = 0; }
  const droppedKnown = dropKnownPhantomSportsFollows();
  if (typeof dropPhantomSportsFollows === "function") dropPhantomSportsFollows();
  if (hadPendingEdits) {
    state.app.preferences = applyPrefsDiff(state.app.preferences, previousBaseline, previousLocal);
    normalizePreferences();
    dropKnownPhantomSportsFollows();
    schedulePrefsFlush();
  } else if (droppedKnown) {
    schedulePrefsFlush();
  }
}
// Follow IDs a past backend build emitted for a nameless phantom team. They
// are removed on every successful prefs load (baseline is already set, so the
// removal is a diff and persists through the normal save queue).
const knownPhantomSportsFollowIDs = ["sports-team:cbe5cfdf7c2118a9"];
function dropKnownPhantomSportsFollows() {
  const preferences = state.app && state.app.preferences;
  if (!preferences) return false;
  let dropped = false;
  ["sportsFavoriteTeams", "sportsFavoriteTeamLabels"].forEach(function(key) {
    const map = preferences[key];
    if (!map || typeof map !== "object") return;
    knownPhantomSportsFollowIDs.forEach(function(id) {
      if (!Object.prototype.hasOwnProperty.call(map, id)) return;
      delete map[id];
      dropped = true;
    });
  });
  return dropped;
}
function schedulePrefsLoadRetry() {
  if ((prefsSync.loaded && !prefsProfileSwitched()) || prefsSync.retryTimer || !pluginInstallationID) return;
  prefsSync.retryTimer = setTimeout(function() {
    prefsSync.retryTimer = 0;
    reloadPrefsFromRemote({ force: true }).catch(function() { schedulePrefsLoadRetry(); });
  }, 15000);
}
async function reloadPrefsFromRemote(options) {
  options = options || {};
  if (!state.app || !pluginInstallationID || isAdminRoute) return;
  if (!options.force && !prefsProfileSwitched() && Date.now() - prefsSync.lastRemoteCheck < 5000) return;
  if (prefsSync.loading) return prefsSync.loading;
  prefsSync.loading = (async function() {
    const values = await loadPluginSettingsValues();
    const profileID = activeViewerProfileID();
    const switched = prefsProfileSwitched();
    const before = prefsSnapshot(state.app.preferences);
    const local = state.app.preferences;
    if (switched) {
      // The pending edits belong to the previous profile; show the new one's view.
      discardPendingPrefsEdits();
    } else if (prefsSync.loaded && prefsSync.dirty) {
      // A save is pending; it re-reads and merges remote state itself.
      prefsSync.lastRemoteCheck = Date.now();
      return;
    }
    state.app.preferences = profileView(remotePrefsFromValues(values), profileID);
    normalizePreferences();
    adoptLoadedPrefs(switched ? null : local, profileID);
    state.recentSearches = readRecentSearches();
    if (!prefsValueEqual(before, state.app.preferences)) renderAfterPrefsSync();
  })().finally(function() { prefsSync.loading = null; });
  return prefsSync.loading;
}
function renderAfterPrefsSync() {
  if (!state.app || state.view === "player" || state.view === "multiview" || state.programDetails) return;
  try { render(); } catch (error) {
    try { console.warn("Xtream render after preference sync failed", error); } catch (_) {}
  }
}
function prefsSyncChannelKey() { return "silo.ramindex.xtream.prefsSync.v1." + localCacheSuffix; }
function announcePrefsSaved() {
  const message = { type: "prefs-saved", tab: prefsSync.tabID, at: Date.now() };
  try { if (prefsSync.channel) { prefsSync.channel.postMessage(message); return; } } catch (_) {}
  try { localStorage.setItem(prefsSyncChannelKey(), JSON.stringify(message)); } catch (_) {}
}
function handlePrefsSyncMessage(message) {
  if (!message || message.type !== "prefs-saved" || message.tab === prefsSync.tabID) return;
  reloadPrefsFromRemote({ force: true }).catch(function(error) {
    try { console.warn("Xtream cross-tab preference reload failed", error); } catch (_) {}
  });
}
function startPrefsSync() {
  if (isAdminRoute || prefsSync.started) return;
  prefsSync.started = true;
  try {
    if (typeof BroadcastChannel === "function") {
      prefsSync.channel = new BroadcastChannel(prefsSyncChannelKey());
      prefsSync.channel.onmessage = function(event) { handlePrefsSyncMessage(event && event.data); };
    }
  } catch (_) { prefsSync.channel = null; }
  if (!prefsSync.channel) {
    window.addEventListener("storage", function(event) {
      if (!event || event.key !== prefsSyncChannelKey() || !event.newValue) return;
      try { handlePrefsSyncMessage(JSON.parse(event.newValue)); } catch (_) {}
    });
  }
  document.addEventListener("visibilitychange", function() {
    if (document.hidden) {
      if (prefsSync.dirty && prefsSync.loaded) flushPrefsSave();
      return;
    }
    reloadPrefsFromRemote().catch(function() { schedulePrefsLoadRetry(); });
  });
}
function schedulePrefsFlush() {
  prefsSync.dirty = true;
  if (prefsSync.timer) clearTimeout(prefsSync.timer);
  prefsSync.timer = setTimeout(flushPrefsSave, prefsSaveDebounceMs);
}
function flushPrefsSave() {
  if (prefsSync.timer) { clearTimeout(prefsSync.timer); prefsSync.timer = 0; }
  return queuePluginSettingsWrite(runPrefsSave);
}
const profileSwitchedDuringSave = new Error("viewer profile changed during save");
function dropEditsForSwitchedProfile() {
  discardPendingPrefsEdits();
  return reloadPrefsFromRemote({ force: true }).catch(function() { schedulePrefsLoadRetry(); });
}
async function runPrefsSave() {
  if (!state.app || !prefsSync.dirty || !prefsSync.loaded) return;
  if (prefsProfileSwitched()) return dropEditsForSwitchedProfile();
  const quiet = prefsSync.quiet;
  prefsSync.dirty = false;
  prefsSync.quiet = true;
  const local = prefsSnapshot(state.app.preferences);
  const baseline = prefsSync.baseline;
  // Always the profile the edits were made for, never whoever is viewing now.
  const profileID = prefsSync.profileID;
  let merged = null;
  try {
    await writePluginSettingsValues(function(values) {
      // The profile may have switched while the settings were being read.
      if (activeViewerProfileID() !== profileID) throw profileSwitchedDuringSave;
      const remoteRaw = remotePrefsFromValues(values);
      merged = applyPrefsDiff(profileView(remoteRaw, profileID), baseline, local);
      values.preferences = JSON.stringify(mergeProfileView(remoteRaw, merged, profileID));
      return values;
    });
  } catch (error) {
    if (error === profileSwitchedDuringSave) return dropEditsForSwitchedProfile();
    prefsSync.dirty = true;
    state.profileSaveStatus = "error";
    state.profileSaveMessage = "Could not save to your Silo profile.";
    if (!quiet) showAppToast(state.profileSaveMessage);
    if (state.view === "settings") renderSettings();
    try { console.warn("Xtream profile preference save failed", error); } catch (_) {}
    if (!prefsSync.retryTimer) {
      prefsSync.retryTimer = setTimeout(function() {
        prefsSync.retryTimer = 0;
        if (prefsSync.dirty) flushPrefsSave();
      }, 15000);
    }
    return;
  }
  const inFlightEdits = state.app.preferences;
  state.app.preferences = merged;
  normalizePreferences();
  prefsSync.baseline = prefsSnapshot(state.app.preferences);
  prefsSync.lastRemoteCheck = Date.now();
  state.app.preferences = applyPrefsDiff(state.app.preferences, local, inFlightEdits);
  state.profileSaveStatus = prefsSync.dirty ? "saving" : "saved";
  state.profileSaveMessage = prefsSync.dirty ? "" : "Saved to your Silo profile.";
  announcePrefsSaved();
  if (!prefsValueEqual(local, merged)) renderAfterPrefsSync();
  else if (state.view === "settings") renderSettings();
}
function savePrefs(options) {
  if (!state.app || !state.app.preferences) return;
  options = options || {};
  if (!pluginInstallationID) {
    state.profileSaveStatus = "error";
    state.profileSaveMessage = "Could not save to your Silo profile.";
    if (!options.quiet) showAppToast(state.profileSaveMessage);
    return;
  }
  if (!options.quiet) prefsSync.quiet = false;
  state.profileSaveStatus = "saving";
  state.profileSaveMessage = "";
  if (!prefsSync.loaded) {
    // Real prefs have not loaded yet; never write defaults over them. Keep the
    // edit pending and flush it once a load succeeds.
    prefsSync.dirty = true;
    schedulePrefsLoadRetry();
    return;
  }
  schedulePrefsFlush();
}
