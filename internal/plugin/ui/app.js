async function getJSON(url) {
  const response = await coreFetch(route(url));
  if (!response.ok) throw await requestError(response);
  return response.json();
}
async function postJSON(url, body) {
  const response = await coreFetch(route(url), { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
  if (!response.ok) throw await requestError(response);
  return response.json();
}
let coreAccessToken = "";
let coreRefreshPromise = null;
function coreStoredValue(key) {
  try { return localStorage.getItem(key) || ""; }
  catch (_) { return ""; }
}
function coreStoreValue(key, value) {
  try {
    if (value) localStorage.setItem(key, value);
  } catch (_) {}
}
function coreRequestOptions(options) {
  const next = Object.assign({}, options || {});
  const headers = Object.assign({}, next.headers || {});
  if (coreAccessToken) headers.Authorization = "Bearer " + coreAccessToken;
  const profileID = coreStoredValue("profile_id");
  const profileToken = coreStoredValue("profile_token");
  if (profileID) headers["X-Profile-Id"] = profileID;
  if (profileToken) headers["X-Profile-Token"] = profileToken;
  next.credentials = "include";
  next.headers = headers;
  return next;
}
async function refreshCoreSession() {
  const refreshToken = coreStoredValue("refresh_token");
  if (!refreshToken) return false;
  const response = await fetch("/api/v1/auth/refresh", {
    method: "POST",
    credentials: "include",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken })
  });
  if (!response.ok) return false;
  const payload = await response.json().catch(function() { return {}; });
  if (!payload.access_token) return false;
  coreAccessToken = String(payload.access_token);
  if (payload.refresh_token) coreStoreValue("refresh_token", String(payload.refresh_token));
  return true;
}
async function coreFetch(url, options) {
  if (!coreAccessToken && coreStoredValue("refresh_token")) {
    if (!coreRefreshPromise) coreRefreshPromise = refreshCoreSession().finally(function() { coreRefreshPromise = null; });
    await coreRefreshPromise;
  }
  let response = await fetch(url, coreRequestOptions(options));
  if (response.status !== 401) return response;
  if (!coreRefreshPromise) {
    coreRefreshPromise = refreshCoreSession().finally(function() { coreRefreshPromise = null; });
  }
  if (await coreRefreshPromise) response = await fetch(url, coreRequestOptions(options));
  return response;
}
async function coreGetJSON(url) {
  const response = await coreFetch(url);
  if (!response.ok) throw await requestError(response);
  return response.json();
}
async function corePutNoContent(url, body) {
  const response = await coreFetch(url, { method: "PUT", headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
  if (!response.ok) throw await requestError(response);
}
async function requestError(response) {
  const text = await response.text().catch(function() { return ""; });
  const detail = text ? ": " + text.slice(0, 240) : "";
  const error = new Error("request failed (" + response.status + ")" + detail);
  error.status = response.status;
  return error;
}
function readableError(error) {
  const status = Number(error && error.status || 0);
  const message = String(error && error.message ? error.message : error || "unknown error");
  if (status === 401 || /request failed \(401\)|unexpected status 401|unauthorized/i.test(message)) {
    return "Your Silo session expired. Refresh the page or sign in again.";
  }
  if (status === 403 || /request failed \(403\)|unexpected status 403|forbidden|permission/i.test(message)) {
    return message;
  }
  return message;
}
function channelByID(id) {
  const channel = rawChannelByID(id);
  return channel ? effectiveChannel(channel) : null;
}
function rawChannelByID(id) {
  return items(state.app.channels).find(function(channel) { return channel.id === id; }) || null;
}
function categoryStartsFeatured(name) {
  return String(name || "").trim().indexOf("*") === 0;
}
function categoryDisplayName(name) {
  name = String(name || "").trim();
  return categoryStartsFeatured(name) ? name.slice(1).trim() : name;
}
function effectiveChannel(channel) {
  if (!channel) return null;
  const copy = Object.assign({}, channel);
  const label = sourceCategoryLabel(channel);
  if (label) copy.categoryName = label;
  return copy;
}
function effectiveChannels(includeHidden) {
  return items(state.app.channels).filter(channelMatchesProfileSelection).map(function(channel, index) {
    const copy = effectiveChannel(channel);
    copy.sourceIndex = index;
    return copy;
  }).sort(function(left, right) {
    return (left.sourceIndex || 0) - (right.sourceIndex || 0);
  });
}
function sourceCategoryID(id) { return "source:" + String(id || ""); }
function customCategoryID(id) { return "custom:" + String(id || ""); }
function virtualCategoryID(path) { return "virtual:" + String(path || ""); }
function featuredCategoryID(path) { return "featured:" + String(path || ""); }
function virtualCategoryPath(id) { return String(id || "").indexOf("virtual:") === 0 ? String(id || "").slice("virtual:".length) : ""; }
function featuredCategoryPath(id) { return String(id || "").indexOf("featured:") === 0 ? String(id || "").slice("featured:".length) : ""; }
function categoryParsing() {
  const delimiterEnabled = prefs().groupCategoriesByPipe === true || (sourceMode() !== "xtream" && adminSettings().mode === "delimiter");
  return { enabled: delimiterEnabled, mode: delimiterEnabled ? "delimiter" : "off", delimiter: "pipe", regex: "", output: "" };
}
function customGroups() {
  return items(prefs().customGroups).slice().filter(function(group) {
    return group && group.id && group.name;
  }).sort(function(left, right) {
    return (Number(left.order || 0) - Number(right.order || 0)) || String(left.name || "").localeCompare(String(right.name || ""));
  });
}
function customMemberships(groupID) {
  return uniqueIDs(items((prefs().customGroupMemberships || {})[groupID]));
}
function sourceCategoryRawName(id) {
  const category = items(state.app.categories).find(function(item) { return item.id === id; });
  return category ? category.name : "";
}
function sourceCategoryName(id) {
  return renamedCategoryDisplayName(sourceCategoryRawName(id));
}
function sourceCategoryRawLabel(channel) {
  return sourceCategoryRawName(channel.categoryId) || channel.categoryName || "";
}
function sourceCategoryLabel(channel) {
  return renamedCategoryDisplayName(sourceCategoryRawLabel(channel));
}
function sourceCategoryOriginalLabel(channel) {
  return categoryDisplayName(sourceCategoryRawLabel(channel));
}
function normalizedGroupName(value) {
  return categoryDisplayName(value).toLowerCase();
}
function parsedDelimitedPath(name) {
  return window.DispatcharrLineup.parsedDelimitedPath(name, (adminSettings().delimiter || "pipe"));
}
function parsedCategoryPath(name) {
  const settings = categoryParsing();
  name = String(name || "").trim();
  if (!settings.enabled || !name) return [];
  let parts = [];
  if (settings.mode === "delimiter") {
    parts = parsedDelimitedPath(name);
  } else if (settings.mode === "regex" && settings.regex) {
    try {
      const pattern = new RegExp(settings.regex);
      const match = name.match(pattern);
      if (!match) return [];
      if (settings.output) parts = name.replace(pattern, settings.output).split("/");
      else parts = match.slice(1);
    } catch (_) {
      return [];
    }
  }
  parts = parts.map(function(part) { return String(part || "").trim(); }).filter(Boolean);
  return parts.length > 1 ? parts : [];
}
function categoryPathFromDisplayName(name) {
  const display = categoryDisplayName(name);
  const parts = parsedCategoryPath(display);
  return parts.length > 1 ? parts.join(" / ") : display;
}
function virtualPathForChannel(channel) {
  const paths = virtualPathsForChannel(channel);
  return paths.length ? paths[0] : "";
}
function sourceVirtualPathForChannel(channel) {
  return parsedCategoryPath(sourceCategoryLabel(channel)).join(" / ");
}
function featuredPathForSourceName(name) {
  if (!categoryStartsFeatured(name)) return "";
  return categoryPathFromDisplayName(renamedCategoryDisplayName(name));
}
function featuredPathsForChannel(channel) {
  const path = featuredPathForSourceName(sourceCategoryRawLabel(channel));
  return path ? [path] : [];
}
function configuredCategoryPath(value) {
  const display = categoryDisplayName(value);
  const parts = parsedCategoryPath(display);
  if (parts.length > 1) return parts.join(" / ");
  const slashParts = String(display || "").split(/\s*\/\s*/).map(function(part) { return String(part || "").trim(); }).filter(Boolean);
  return slashParts.length > 1 ? slashParts.join(" / ") : "";
}
function aliasVirtualPathsForSourcePath(sourcePath) {
  const normalizedSourcePath = configuredCategoryPath(sourcePath) || String(sourcePath || "").trim();
  if (!normalizedSourcePath) return [];
  const paths = [];
  const seen = {};
  categoryAliases().forEach(function(alias) {
    const fromPath = configuredCategoryPath(alias.sourcePath);
    const toPath = configuredCategoryPath(alias.aliasPath);
    if (!fromPath || !toPath) return;
    if (normalizedSourcePath !== fromPath && normalizedSourcePath.indexOf(fromPath + " / ") !== 0) return;
    const suffix = normalizedSourcePath === fromPath ? "" : normalizedSourcePath.slice(fromPath.length + 3);
    const remappedPath = suffix ? toPath + " / " + suffix : toPath;
    if (!seen[remappedPath]) {
      seen[remappedPath] = true;
      paths.push(remappedPath);
    }
  });
  return paths;
}
function channelNamePathParts(channel) {
  return window.DispatcharrLineup.channelNamePathParts(channel);
}
function channelNameFolderPathParts(channel) {
  const parts = channelNamePathParts(channel).map(normalizedNameToken).filter(Boolean);
  if (parts.length < 2) return [];
  return parts.slice(0, -1);
}
function appendUnduplicatedPathParts(basePath, extraParts) {
  return window.DispatcharrLineup.appendUnduplicatedPathParts(basePath, extraParts);
}
function appendVirtualPathParts(basePath, extraParts) {
  return window.DispatcharrLineup.appendVirtualPathParts(basePath, extraParts, adminSettings().collapseDuplicateVirtualGroups);
}
function normalizedNameToken(value) {
  return window.DispatcharrLineup.normalizedNameToken(value);
}
function looksLikeUSStateCode(value) {
  const code = normalizedNameToken(value).toUpperCase();
  const states = {
    AL: true, AK: true, AZ: true, AR: true, CA: true, CO: true, CT: true, DE: true, FL: true, GA: true,
    HI: true, ID: true, IL: true, IN: true, IA: true, KS: true, KY: true, LA: true, ME: true, MD: true,
    MA: true, MI: true, MN: true, MS: true, MO: true, MT: true, NE: true, NV: true, NH: true, NJ: true,
    NM: true, NY: true, NC: true, ND: true, OH: true, OK: true, OR: true, PA: true, RI: true, SC: true,
    SD: true, TN: true, TX: true, UT: true, VT: true, VA: true, WA: true, WV: true, WI: true, WY: true,
    DC: true
  };
  return states[code] === true;
}
function inferredInternationalRoot(channel, parts) {
  const text = (parts.join(" ") + " " + sourceCategoryLabel(channel || {}) + " " + ((channel && channel.categoryName) || "")).toLowerCase();
  if (/\b(sport|sports|team|teams|league|football|soccer|futbol|fútbol|mlb|nba|nfl|nhl|mls|f1)\b/.test(text)) return "International Sports";
  if (/\b(news|noticias|nouvelles)\b/.test(text)) return "International News";
  if (/\b(kids|children|cartoon|junior)\b/.test(text)) return "International Kids";
  return "International TV";
}
function inferredChannelNameGroupPaths(channel) {
  if (!useChannelNameVirtualPaths()) return [];
  const parts = channelNamePathParts(channel).map(normalizedNameToken).filter(Boolean);
  if (parts.length < 2) return [];
  const first = parts[0];
  const second = parts[1];
  const paths = [];
  if (looksLikeUSStateCode(first)) {
    const state = first.toUpperCase();
    paths.push("US TV / Locals / " + state);
    if (second) paths.push("US TV / Locals / " + state + " / " + second);
    return uniqueIDs(paths);
  }
  const root = inferredInternationalRoot(channel, parts);
  paths.push(root + " / " + first);
  if (second) paths.push(root + " / " + first + " / " + second);
  return uniqueIDs(paths);
}
function localMarketPathPartsForChannel(channel, sourceParts) {
  const sourceTail = normalizedNameToken(items(sourceParts).slice(-1)[0] || "").toLowerCase();
  if (sourceTail !== "local" && sourceTail !== "locals") return [];
  const parts = channelNamePathParts(channel).map(normalizedNameToken).filter(Boolean);
  if (parts.length < 2) return [];
  if (looksLikeUSStateCode(parts[0])) return parts[1] ? [parts[1]] : [];
  return [parts[0]];
}
function virtualPathsForChannel(channel) {
  const paths = [];
  if (useProfileGroupVirtualPaths()) {
    profileVirtualPathsForChannel(channel).forEach(function(path) { paths.push(path); });
    return uniqueIDs(paths);
  }
  const sourcePath = sourceVirtualPathForChannel(channel);
  if (sourcePath && useSourceGroupVirtualPaths()) {
    paths.push(sourcePath);
    aliasVirtualPathsForSourcePath(sourcePath).forEach(function(path) { paths.push(path); });
    if (virtualGroupSourceMode() === "group_channel") {
      const combinedPath = appendVirtualPathParts(sourcePath, channelNameFolderPathParts(channel));
      if (combinedPath) {
        paths.push(combinedPath);
        aliasVirtualPathsForSourcePath(combinedPath).forEach(function(path) { paths.push(path); });
      }
    }
  }
  if (useChannelNameVirtualPaths()) {
    inferredChannelNameGroupPaths(channel).forEach(function(path) { paths.push(path); });
  }
  return uniqueIDs(paths);
}
function profileVirtualPathsForChannel(channel) {
  const sourcePath = sourceGroupPathForChannel(channel);
  if (!sourcePath) return [];
  const paths = [];
  const sourceParts = String(sourcePath || "").split(" / ").map(normalizedNameToken).filter(Boolean);
  const groupPaths = [sourcePath].concat(aliasVirtualPathsForSourcePath(sourcePath));
  const marketParts = localMarketPathPartsForChannel(channel, sourceParts);
  profilePathsForChannel(channel).forEach(function(profilePath) {
    paths.push(profilePath);
    groupPaths.forEach(function(groupPath) {
      const groupParts = String(groupPath || "").split(" / ").map(normalizedNameToken).filter(Boolean);
      const combinedPath = appendVirtualPathParts(profilePath, groupParts);
      if (combinedPath) paths.push(combinedPath);
      const marketPath = appendVirtualPathParts(combinedPath || (profilePath + " / " + groupPath), marketParts);
      if (marketPath) paths.push(marketPath);
    });
  });
  return uniqueIDs(paths);
}
function sourceGroupPathForChannel(channel) {
  return sourceVirtualPathForChannel(channel) || categoryDisplayName(sourceCategoryLabel(channel));
}
function profilePathsForChannel(channel) {
  const profiles = profileMapByID();
  const selectedProfile = state.app && state.app.source && state.app.source.channelProfile;
  let profileIDs = selectedProfile && selectedProfile.id ? [selectedProfile.id] : items(channel && channel.profileIds);
  if (!profileSelectionIsAll()) {
    const selected = selectedProfileMap();
    profileIDs = profileIDs.filter(function(profileID) { return !!selected[profileID]; });
  }
  return profileIDs.map(function(profileID) {
    return profileVirtualPathForName((profiles[profileID] || {}).name || profileID);
  }).filter(Boolean);
}
function profileVirtualPathForName(name) {
  return String(name || "").split("|").map(function(part) {
    return part.trim();
  }).filter(Boolean).join(" / ");
}
function profileMapByID() {
  const map = {};
  items(state.app && state.app.source && state.app.source.profiles).forEach(function(profile) {
    if (profile && profile.id) map[profile.id] = profile;
  });
  return map;
}
function isRewindableChannel(channel) {
  if (!channel) return false;
  if (isWorldCupReplayGroup(sourceCategoryRawLabel(channel)) || isWorldCupReplayGroup(sourceCategoryLabel(channel))) return true;
  return virtualPathsForChannel(channel).some(function(path) {
    if (isWorldCupReplayGroup(path)) return true;
    return path.split(" / ").some(isWorldCupReplayGroup);
  });
}
function channelInSelectedCategory(channel, id) {
  if (!id) return true;
  if (id.indexOf("source:") === 0) return channel.categoryId === id.slice("source:".length);
  if (id.indexOf("custom:") === 0) return customMemberships(id.slice("custom:".length)).indexOf(channel.id) !== -1;
  if (id.indexOf("featured:") === 0) {
    const selected = featuredCategoryPath(id);
    return featuredPathsForChannel(channel).some(function(path) {
      return path === selected || path.indexOf(selected + " / ") === 0;
    });
  }
  if (id.indexOf("virtual:") === 0) {
    const selected = virtualCategoryPath(id);
    return virtualPathsForChannel(channel).some(function(path) {
      return path === selected || path.indexOf(selected + " / ") === 0;
    });
  }
  return channel.categoryId === id;
}
function visibleChannels(ignoreQuery) {
  const hidden = hiddenMap();
  const channels = effectiveChannels(false).filter(function(channel) {
    if (channel.categoryId && hidden[channel.categoryId]) return false;
    if (state.view !== "favorites" && state.category && !channelInSelectedCategory(channel, state.category)) return false;
    if (!ignoreQuery && state.query && !guideChannelMatchesQuery(channel)) return false;
    if (state.view === "favorites" && !favoriteMap()[channel.id] && !autoFavoriteMap()[channel.id]) return false;
    return true;
  });
  return state.view === "favorites" ? orderedFavoriteChannels(channels) : channels;
}
function orderedFavoriteChannels(channels) {
  const byID = {};
  items(channels || effectiveChannels(false)).forEach(function(channel) { byID[channel.id] = channel; });
  const ordered = uniqueIDs(items(prefs().favoriteOrder)).map(function(id) { return byID[id]; }).filter(Boolean);
  const missing = items(channels || effectiveChannels(false)).filter(function(channel) {
    return (favoriteMap()[channel.id] || autoFavoriteMap()[channel.id]) && ordered.indexOf(channel) === -1;
  });
  return ordered.concat(missing);
}
function moveFavorite(channelID, direction) {
  const favorites = orderedFavoriteChannels(visibleChannels(true)).filter(function(channel) { return !!favoriteMap()[channel.id]; });
  const order = favorites.map(function(channel) { return channel.id; });
  const index = order.indexOf(channelID);
  if (index === -1) return;
  const target = direction === "up" ? index - 1 : index + 1;
  if (target < 0 || target >= order.length) return;
  const value = order[index];
  order[index] = order[target];
  order[target] = value;
  state.app.preferences.favoriteOrder = order;
  savePrefs();
  render();
}
function setChannelFavorite(channelID, enabled) {
  const id = String(channelID || "");
  if (!id || !state.app || !state.app.preferences) return false;
  if (enabled) {
    state.app.preferences.favorites[id] = true;
    state.app.preferences.favoriteOrder = uniqueIDs(items(state.app.preferences.favoriteOrder).concat([id]));
  } else {
    delete state.app.preferences.favorites[id];
    delete state.app.preferences.autoFavorites[id];
    state.app.preferences.favoriteOrder = items(state.app.preferences.favoriteOrder).filter(function(item) { return item !== id; });
  }
  normalizePreferences();
  savePrefs();
  return !!favoriteMap()[id];
}
function channelMatchesQuery(channel) {
  if (!state.query) return true;
  return lower([channel.name, channel.categoryName, channel.number].join(" ")).indexOf(lower(state.query)) !== -1;
}
function programMatchesQuery(program) {
  if (!state.query) return true;
  return lower([program.title, program.description].join(" ")).indexOf(lower(state.query)) !== -1;
}
function rebuildProgramIndex() {
  const sorted = items(state.app && state.app.programs).slice().sort(function(a, b) {
    return (a.startUnix || 0) - (b.startUnix || 0);
  });
  const byChannel = {};
  sorted.forEach(function(program) {
    const id = String(program.channelId || "");
    if (!id) return;
    if (!byChannel[id]) byChannel[id] = [];
    byChannel[id].push(program);
  });
  state.sortedPrograms = sorted;
  state.programsByChannel = byChannel;
}
function timeLabel(unix) {
  if (!unix) return "";
  return new Date(unix * 1000).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}
function dateTimeLabel(unix) {
  if (!unix) return "Never";
  return new Date(unix * 1000).toLocaleString([], { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });
}
function relativeUpdatedLabel(unix) {
  unix = Number(unix || 0);
  if (!unix) return "Updated time unknown";
  const seconds = Math.max(0, Math.floor(Date.now() / 1000) - unix);
  if (seconds < 60) return "Updated just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return "Updated " + minutes + " minute" + (minutes === 1 ? "" : "s") + " ago";
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return "Updated " + hours + " hour" + (hours === 1 ? "" : "s") + " ago";
  const days = Math.floor(hours / 24);
  return "Updated " + days + " day" + (days === 1 ? "" : "s") + " ago";
}
function sourceModeLabel(mode) {
  mode = String(mode || sourceMode() || "");
  if (mode === "direct_login") return "Dispatcharr Direct";
  if (mode === "api_key") return "Dispatcharr API Key";
  if (mode === "xtream") return "Xtream Codes";
  if (mode === "m3u_xmltv") return "M3U + XMLTV";
  return mode || "Not configured";
}
function epgCellStyle(startUnix, endUnix, windowInfo) {
  const start = Math.max(startUnix || windowInfo.start, windowInfo.start);
  const end = Math.min(endUnix || start + 1800, windowInfo.end);
  const leftSlots = (start - windowInfo.start) / 1800;
  const widthSlots = Math.max((end - start) / 1800, 0);
  return "left: calc(" + leftSlots.toFixed(4) + " * var(--epg-slot)); width: calc(" + widthSlots.toFixed(4) + " * var(--epg-slot) - 0.0625rem);";
}
function stopPlayback() {
 if(state.videoAttachment){state.videoAttachment.destroy();state.videoAttachment=null;state.hls=null;state.tsPlayer=null;}
  const video = byId("player");
  if (state.hls) { state.hls.destroy(); state.hls = null; }
  if (state.tsPlayer) { state.tsPlayer.destroy(); state.tsPlayer = null; }
  if (state.playerChromeTimer) {
    clearTimeout(state.playerChromeTimer);
    state.playerChromeTimer = null;
  }
  state.playerChromeIdle = false;
  state.playerSportsOpen = false;
  stopPlayerSportsRefresh();
  if (video) {
    video.pause();
    video.removeAttribute("src");
    video.load();
  }
  stopTimeShiftSession();
}
function setView(view, options) {
  options = options || {};
  if (view === "search" && state.view !== "search" && state.view !== "player") {
    state.searchReturnView = state.view || "home";
  }
  if (view !== "player") {
    stopPlayback();
    if (state.view === "player") stopCurrentWatch("leave_player");
  }
  if (view !== "multiview" && state.view === "multiview") stopAllMultiview("leave_multiview");
  if (view !== state.view && !options.preserveBrowseState) state.folderQuery = "";
  state.view = view;
  if ((view === "favorites" || view === "onlater") && !options.preserveBrowseState) state.category = "";
  if ((view === "search" || view === "onlater") && dvrEnabled()) loadRecordings(false);
  if (view === "sports") {
    state.category = "";
    loadSports(false);
  }
  if (view === "events") {
    state.category = "";
    loadEvents(false);
  }
  render();
  ensureGuideCoverageForView(view);
}
function setCategory(id) {
  if ((id || "") !== state.category) state.folderQuery = "";
  state.category = id || "";
  state.view = id ? (nestedFolderChildren(id).length ? "live" : "guide") : "home";
  render();
  ensureGuideCoverageForView(state.view);
}
function selectedNestedFolder(id) {
  id = String(id || "");
  if (id.indexOf("featured:") === 0) return { featured: true, path: featuredCategoryPath(id) };
  if (id.indexOf("virtual:") === 0) return { featured: false, path: virtualCategoryPath(id) };
  return null;
}
function nestedFolderChildren(id) {
  const folder = selectedNestedFolder(id);
  if (!folder) return [];
  const hidden = hiddenMap();
  const includeChannel = function(channel) { return !(channel.categoryId && hidden[channel.categoryId]); };
  return folder.featured ? featuredChildCategories(folder.path, includeChannel) : virtualChildCategories(folder.path, includeChannel);
}
async function hydrateApp(payload, options) {
  options = options || {};
  const previousApp = state.app || {};
  payload = payload || {};
  payload.programs = Array.isArray(payload.programs) ? payload.programs : items(previousApp.programs);
  payload.vod = payload.vod || previousApp.vod || { available: false, categories: [], items: [] };
  payload.series = payload.series || previousApp.series || { available: false, categories: [], items: [] };
  // /api/refresh responses may omit isAdmin; keep the bootstrap value.
  if (typeof payload.isAdmin !== "boolean" && options.reuseSettings && typeof previousApp.isAdmin === "boolean") payload.isAdmin = previousApp.isAdmin;
  state.app = payload;
  const previousPrefs = previousApp.preferences || null;
  let prefsJustLoaded = false;
  let loadedProfileID;
  if (options.localCache) {
    state.app.preferences = previousPrefs || defaultPrefs();
    state.adminCategorySettings = defaultAdminCategorySettings();
  } else if (options.reuseSettings) {
    state.app.preferences = previousPrefs || defaultPrefs();
  } else {
    const loadFailed = {};
    const values = await loadPluginSettingsValues().catch(function(error) {
      try { console.warn("Xtream profile preference load failed", error); } catch (_) {}
      return loadFailed;
    });
    if (values === loadFailed) {
      // Keep whatever this tab already has; saves stay pending until a load succeeds.
      state.app.preferences = previousPrefs || mergePrefs(state.app.preferences);
    } else {
      loadedProfileID = activeViewerProfileID();
      const rawPrefs = readSiloPrefsValue(values && values.preferences ? values.preferences : "");
      state.app.preferences = mergePrefs((rawPrefs && profileView(rawPrefs, loadedProfileID)) || state.app.preferences);
      prefsJustLoaded = true;
    }
    state.adminCategorySettings = await loadAdminCategorySettings().catch(function() { return defaultAdminCategorySettings(); });
  }
  state.savedAdminCategorySettings = cloneAdminCategorySettings(state.adminCategorySettings);
  state.app.programs = items(state.app.programs);
  rebuildProgramIndex();
  normalizePreferences();
  if (prefsJustLoaded) adoptLoadedPrefs(previousPrefs, loadedProfileID);
  else if (!prefsSync.baseline) prefsSync.baseline = prefsSnapshot(state.app.preferences);
  if (!prefsSync.loaded && !options.localCache && !options.reuseSettings) schedulePrefsLoadRetry();
  state.recentSearches = readRecentSearches();
  normalizeAdminCategorySettings();
  state.savedAdminCategorySettings = cloneAdminCategorySettings(state.adminCategorySettings);
  if (!options.localCache) writeLocalAppCache(state.app);
}
async function refreshStatusData() {
  const status = await getJSON("/dispatcharr/api/status");
  if (state.app) {
    state.app.status = status || {};
    if (state.app.source && status && status.profileAccess) state.app.source.profileAccess = status.profileAccess;
  }
  return status || {};
}
// The guide is loaded in time windows (/api/guide?start=&end=) instead of as
// one full blob: the initial load covers now-3h..now+24h, and later windows are
// fetched on demand (guide window moving forward, search / My TV look-ahead).
// If the backend ignores the window params the full guide comes back; that is
// detected and treated as full coverage.
const guideLookbehindSeconds = 3 * 3600;
const guideInitialLookaheadSeconds = 24 * 3600;
const guideSearchLookaheadSeconds = 7 * 24 * 3600;
const guideCoverage = { start: 0, end: 0, full: false, pending: null };
function programOverlapsWindow(program, start, end) {
  const programStart = Number(program && program.startUnix || 0);
  const programEnd = Number(program && program.endUnix || 0) || programStart + 1800;
  return programEnd > start && programStart < end;
}
async function refreshSupplementalData(includeContent) {
  if (!state.app) return;
  const now = Math.floor(Date.now() / 1000);
  // Periodic refreshes only re-read the near window; farther windows fetched
  // on demand stay merged in.
  const requests = [fetchGuideWindow(now - guideLookbehindSeconds, now + guideInitialLookaheadSeconds).catch(function(error) {
    try { console.warn("Xtream guide load failed", error); } catch (_) {}
    return false;
  })];
  if (includeContent) {
    requests.push(getJSON("/dispatcharr/api/vod").catch(function() { return null; }));
    requests.push(getJSON("/dispatcharr/api/series").catch(function() { return null; }));
  }
  const payloads = await Promise.all(requests);
  if (includeContent && payloads[1]) state.app.vod = payloads[1];
  if (includeContent && payloads[2]) state.app.series = payloads[2];
  rebuildProgramIndex();
  writeLocalAppCache(state.app);
  if (state.view === "guide" || state.view === "search" || state.view === "mytv") ensureGuideCoverageForView(state.view);
}
async function loadApp() {
  const cached = readLocalAppCache();
  let renderedCachedApp = false;
  if (cached) {
    await hydrateApp(cached, { localCache: true });
    state.appLoadedFromCache = true;
    renderedCachedApp = true;
    render();
  }
  try {
    await hydrateApp(await getJSON("/dispatcharr/api/app"));
    state.appLoadedFromCache = false;
    await loadRecordingCapability();
    render();
    await refreshSupplementalData(true);
    render();
  } catch (error) {
    if (!renderedCachedApp) throw error;
    showAppToast("Showing saved guide. Refresh failed.");
    try { console.warn("Dispatcharr app refresh failed", error); } catch (_) {}
  }
}
function epgLastSuccessUnix() {
  const status = state.app && state.app.status ? state.app.status : {};
  return Number(status.epgLastSuccessUnix || 0);
}
function setSettingsMenuOpen(open) {
  const menu = byId("settings-menu");
  const button = byId("settings-menu-button");
  if (!menu || !button) return;
  const shell = menu.closest(".settings-menu");
  if (shell) shell.classList.toggle("open", !!open);
  button.setAttribute("aria-expanded", open ? "true" : "false");
}
function settingsMenuOpen() {
  const menu = byId("settings-menu");
  return !!(menu && menu.closest(".settings-menu") && menu.closest(".settings-menu").classList.contains("open"));
}
function renderRail() {
  const browseButton = byId("primary-browse-nav");
  if (browseButton && browseButton.dataset) {
    const showChannels = channelGroupsInSideMenu();
    browseButton.dataset.view = showChannels ? "channels" : "guide";
    browseButton.dataset.activeViews = showChannels ? "channels live" : "guide";
    const label = browseButton.querySelector("span");
    if (label) label.textContent = showChannels ? "Channels" : "Guide";
  }
  document.querySelectorAll("[data-view]").forEach(function(button) {
    const unavailable = (button.dataset.view === "recordings" && !dvrEnabled()) || (button.dataset.view === "sports" && !sportsNavAvailable()) || (button.dataset.view === "events" && !eventsNavAvailable());
    const activeViews = String(button.dataset.activeViews || button.dataset.view || "").split(/\s+/).filter(Boolean);
    button.hidden = unavailable;
    const active = !unavailable && activeViews.indexOf(state.view) !== -1;
    button.classList.toggle("active", active);
    if (active) button.setAttribute("aria-current", "page");
    else button.removeAttribute("aria-current");
  });
  document.querySelectorAll("[data-guide-refresh]").forEach(function(button) {
    button.hidden = !siloUserIsAdmin();
  });
  const favoriteCount = byId("favorite-count");
  if (favoriteCount) favoriteCount.textContent = Object.keys(favoriteMap()).length + Object.keys(autoFavoriteMap()).length;
}
function channelLogoFallback(channel) {
  const name = String((channel && channel.name) || "TV").trim();
  const region = name.match(/^\(([A-Za-z0-9]{2,4})\)/);
  if (region) return region[1].slice(0, 4);
  const parts = name.replace(/[^A-Za-z0-9]+/g, " ").trim().split(/\s+/).filter(Boolean);
  if (parts.length > 1) return parts.slice(0, 2).map(function(part) { return part.charAt(0); }).join("");
  return (parts[0] || name || "TV").slice(0, 5);
}
function logoHTML(channel) {
  const fallback = "<span class=\"logo logo-fallback\"" + (channel && channel.logoUrl ? " hidden" : "") + " aria-hidden=\"true\">" + escapeHTML(channelLogoFallback(channel)) + "</span>";
  if (channel && channel.logoUrl) return "<img class=\"logo\" src=\"" + escapeHTML(channel.logoUrl) + "\" alt=\"\" data-img-error=\"logo-fallback\">" + fallback;
  return fallback;
}
function render() {
  if (!state.app) return;
  if (state.view === "sports" && !sportsEnabled()) state.view = "home";
  if (state.view === "events") state.view = "home";
  if (state.view === "recordings" && !dvrEnabled()) state.view = "home";
  if (state.view === "admin" && !isAdminRoute) state.view = "home";
  document.querySelector(".shell").classList.toggle("is-player", state.view === "player");
  document.querySelector(".shell").classList.toggle("is-home", state.view === "home");
  document.querySelector(".shell").classList.toggle("is-guide", state.view === "guide");
  document.querySelector(".shell").classList.toggle("is-sports", state.view === "sports");
  document.querySelector(".shell").classList.toggle("is-events", state.view === "events");
  document.querySelector(".shell").classList.toggle("is-multiview", state.view === "multiview");
  document.querySelector(".shell").classList.toggle("is-search", state.view === "search");
  document.querySelector(".shell").classList.toggle("is-onlater", state.view === "onlater");
  document.querySelector(".shell").classList.toggle("is-mytv", state.view === "mytv");
  renderRail();
  renderSportsTopbarTabs();
  if (state.view === "guide") renderGuidePage();
  else if (state.view === "player") renderPlayerPage();
  else if (state.view === "multiview") renderMultiviewPage();
  else if (state.view === "live" || state.view === "favorites") renderLivePage();
  else if (state.view === "sports") renderSportsPage();
  else if (state.view === "events") renderEventsPage();
  else if (state.view === "onlater") renderOnLaterPage();
  else if (state.view === "mytv") renderMyTVPage();
  else if (state.view === "search") renderSearchPage();
  else if (state.view === "series-detail") renderSeriesDetailPage();
  else if (state.view === "recordings") renderRecordingsPage();
  else if (state.view === "admin") renderAdminPage();
  else if (state.view === "settings") renderSettings();
  else renderHome();
}
function renderHome() {
  const root = byId("view");
  const recent = recentChannels(5);
  const watched = recent;
  const favorites = homeFavoriteChannels();
  root.innerHTML = "<div class=\"home-page\">"
    + (watched.length ? homeSection("Recently watched", rowCards(watched), "") : "")
    + (favorites.length ? homeSection("Favorites", favoriteHomeCards(favorites), "") : "")
    + homeSection("TV Guide", renderHomeGuide(homeGuideChannels(watched, favorites), "No current guide data for recently watched or favorite channels.", { hideFreshness: true }), "", guideFreshnessHTML(), "home-guide-section")
    + categoryGrid("home")
    + "</div>";
}
function homeSection(title, content, eyebrow, actions, className) {
  const eyebrowHTML = eyebrow ? "<span>" + escapeHTML(eyebrow) + "</span>" : "";
  return "<section class=\"home-section " + escapeHTML(className || "") + "\"><header class=\"home-section-header\"><div>" + eyebrowHTML + "<h2>" + escapeHTML(title) + "</h2></div>" + (actions || "") + "</header>" + content + "</section>";
}
function emptyStateHTML(title, detail) {
  detail = String(detail || "").trim();
  return "<div class=\"empty\"><strong>" + escapeHTML(title) + "</strong>" + (detail ? "<div class=\"muted\">" + escapeHTML(detail) + "</div>" : "") + "</div>";
}
function catalogEmptyDetail() {
  if (userSourcesMissing()) return "Add your Xtream Codes provider in Settings to build your Live TV.";
  if (!state.app || !state.app.status) return "Check your connection in Live TV Admin or press Refresh.";
  const status = state.app.status;
  if (status.status === "error" && status.lastError) return status.lastError;
  if (!status.channelCount) return "No channels synced yet. Run a sync from Live TV Admin or press Refresh.";
  return "Try Refresh or open Live TV Admin to verify the connection.";
}
function sectionHeader(title) {
  return "<div class=\"section-title\"><span>" + escapeHTML(title) + "</span></div>";
}
function sectionHeaderWithActions(title, actions) {
  return "<div class=\"section-title\"><span>" + escapeHTML(title) + "</span>" + (actions || "") + "</div>";
}
function sectionActions(actions) {
  return "<div class=\"section-title actions-only\">" + (actions || "") + "</div>";
}
function rowCards(channels) {
  if (!channels.length) return emptyStateHTML("No channels yet.", catalogEmptyDetail());
  return "<div class=\"row-scroll recent-channel-row\">" + channels.map(function(channel) {
    const program = currentProgram(channel) || {};
    const channelName = channel.name || "Untitled";
    const programTitle = String(program.title || "").trim();
    const subtitle = programTitle && lower(programTitle) !== lower(channelName) ? programTitle : "Live channel";
    return "<button type=\"button\" class=\"continue-card recent-channel-card\" data-channel=\"" + escapeHTML(channel.id) + "\" aria-label=\"Watch " + escapeHTML(channelName + " - " + subtitle) + "\"><div class=\"poster-box\">" + (channel.logoUrl ? "<img src=\"" + escapeHTML(channel.logoUrl) + "\" alt=\"\">" : "<span>" + escapeHTML(channelName.slice(0, 5)) + "</span>") + "</div><span class=\"recent-channel-copy\"><strong>" + escapeHTML(channelName) + "</strong><span class=\"muted\" data-overflow-tooltip=\"" + escapeHTML(subtitle) + "\">" + escapeHTML(subtitle) + "</span></span></button>";
  }).join("") + "</div>";
}
function homeFavoriteChannels() {
  return orderedFavoriteChannels(visibleChannels(true)).filter(function(channel) {
    return !!(favoriteMap()[channel.id] || autoFavoriteMap()[channel.id]);
  }).slice(0, 10);
}
function favoriteHomeCards(channels) {
  return "<div class=\"row-scroll favorites-row\">" + channels.map(function(channel) {
    return "<button class=\"continue-card home-favorite-card\" data-channel=\"" + escapeHTML(channel.id) + "\"><div class=\"poster-box\">" + (channel.logoUrl ? "<img src=\"" + escapeHTML(channel.logoUrl) + "\" alt=\"\">" : "<span>" + escapeHTML((channel.name || "TV").slice(0, 5)) + "</span>") + "</div><strong>" + escapeHTML(channel.name || "Untitled") + "</strong><div class=\"muted\">" + escapeHTML(channel.categoryName || "Live TV") + "</div></button>";
  }).join("") + "</div>";
}
function searchNeedle() {
  return lower(state.searchQuery).trim();
}
function searchableChannels() {
  const hidden = hiddenMap();
  return dedupeChannels(effectiveChannels(false).filter(function(channel) {
    return !(channel.categoryId && hidden[channel.categoryId]);
  }));
}
function channelMatchesSearch(channel, query) {
  const haystack = [channel.name, channel.number, channel.categoryName, sourceCategoryLabel(channel), sourceCategoryRawLabel(channel)].join(" ");
  return lower(haystack).indexOf(query) !== -1;
}
function programMatchesSearch(program, query) {
  if (programIsGuidePlaceholder(program)) return false;
  const channel = channelByID(program.channelId) || {};
  const haystack = [program.title, program.summary, program.description, channel.name, channel.categoryName].join(" ");
  return lower(haystack).indexOf(query) !== -1;
}
function contentCategoryName(kind, item) {
  const payload = state.app && state.app[kind] ? state.app[kind] : {};
  const match = items(payload.categories).find(function(category) { return category.id === item.categoryId; });
  return (match && match.name) || "";
}
function contentMatchesSearch(kind, item, query) {
  const haystack = [item.name, item.title, item.description, item.rating, contentCategoryName(kind, item)].join(" ");
  return lower(haystack).indexOf(query) !== -1;
}
function allDiscoveryGroups() {
  const hidden = hiddenMap();
  const includeChannel = function(channel) { return !(channel.categoryId && hidden[channel.categoryId]); };
  return customGroupCategories()
    .concat(featuredCategoriesFromPaths("", includeChannel, true))
    .concat(virtualCategoriesFromPaths("", includeChannel, true))
    .concat(sourceCategoriesWithChannels(includeChannel))
    .sort(compareCategoryDisplayName);
}
function groupMatchesSearch(group, query) {
  return lower([group.name, group.id, group.kind].join(" ")).indexOf(query) !== -1;
}
function normalizeProgramTitle(title) {
  return lower(title).replace(/\s+/g, " ").replace(/\s+\(\d{4}\)$/, "").trim();
}
function programSearchText(program) {
  const channel = channelByID(program.channelId) || {};
  return [program.title, program.summary, program.description, channel.name, channel.categoryName].join(" ");
}
function recordingMatchesSearch(recording, query) {
  const haystack = [recordingTitle(recording), recordingChannelName(recording), recordingStatus(recording)].join(" ");
  return lower(haystack).indexOf(query) !== -1;
}
function programIsLive(program) {
  const now = Math.floor(Date.now() / 1000);
  return (program.startUnix || 0) <= now && (program.endUnix || 0) > now;
}
function programIsUpcoming(program) {
  return (program.startUnix || 0) > Math.floor(Date.now() / 1000);
}
function programLooksMovie(program) {
  if (programIsGuidePlaceholder(program)) return false;
  return /\b(movie|film|premiere|cinema|starring)\b/i.test(programSearchText(program));
}
function programLooksEvent(program) {
  if (programIsGuidePlaceholder(program)) return false;
  return /\b(awards|parade|special|ceremony|debate|concert|festival|live)\b/i.test(programSearchText(program));
}
function programOnLaterType(program) {
  if (programLooksSports(program)) return "sports";
  if (programLooksMovie(program)) return "movies";
  if (programLooksEvent(program)) return "events";
  return "other";
}
function programMatchesOnLaterType(program, type) {
  type = type || "all";
  if (type === "all") return true;
  if (type === "passes") {
    return keywordPasses().some(function(pass) { return lower(programSearchText(program)).indexOf(lower(pass.keyword)) !== -1; });
  }
  return programOnLaterType(program) === type;
}
function groupedUpcomingAirings(programs, query) {
  const groups = {};
  items(programs).filter(programIsUpcoming).forEach(function(program) {
    if (programIsGuidePlaceholder(program)) return;
    const searchKey = normalizeProgramTitle(program.title);
    if (!searchKey || (query && searchKey.indexOf(query) === -1 && lower(programSearchText(program)).indexOf(query) === -1)) return;
    // The guide payload has airing IDs, not a trustworthy series ID. Keep
    // same-title programs on different channels separate instead
    // of pretending they are one authoritative series.
    const groupID = [searchKey, String(program.channelId || "")].join("|");
    groups[groupID] = groups[groupID] || { id: groupID, key: searchKey, title: program.title || "Untitled", programs: [] };
    groups[groupID].programs.push(program);
  });
  return Object.keys(groups).map(function(key) {
    const group = groups[key];
    group.programs.sort(function(left, right) { return (left.startUnix || 0) - (right.startUnix || 0); });
    return group;
  }).sort(function(left, right) {
    return (left.programs[0].startUnix || 0) - (right.programs[0].startUnix || 0);
  });
}
function searchFilters() {
  const filters = [
    { id: "all", label: "All" },
    { id: "channels", label: "Live TV" },
    { id: "groups", label: "Groups" },
    { id: "guide", label: "Guide" },
    { id: "movies", label: "Movies" },
    { id: "shows", label: "Shows" }
  ];
  if (sportsEnabled()) filters.push({ id: "sports", label: "Sports" });
  return filters;
}
function firstSearchMatches(collection, predicate, limit) {
  const matches = [];
  const source = items(collection);
  for (let index = 0; index < source.length && matches.length < limit; index += 1) {
    if (predicate(source[index])) matches.push(source[index]);
  }
  return matches;
}
function searchResultSections(query) {
  const filter = state.searchType || "all";
  const include = function(id) { return filter === "all" || filter === id || (filter === "guide" && (id === "programs" || id === "airings")); };
  const sections = [];
  let programCatalog = null;
  const programs = function() {
    if (!programCatalog) programCatalog = programsFor("");
    return programCatalog;
  };
  if (include("channels")) {
    const channels = rankedSearchMatches(searchableChannels(), function(channel) {
      return searchMatchScore(channel.name, [channel.number, channel.categoryName, sourceCategoryLabel(channel), sourceCategoryRawLabel(channel)].join(" "), query);
    }, 18);
    sections.push({ id: "channels", title: "Channels", rows: channels.map(function(channel) {
      return {
        attrs: "data-search-channel=\"" + escapeHTML(channel.id) + "\"",
        favoriteChannelId: channel.id,
        art: logoHTML(channel),
        title: channel.name || "Untitled",
        meta: ["Channel", channel.categoryName || "Live TV"].filter(Boolean).join(" - "),
        action: "Watch"
      };
    }) });
  }
  if (include("groups")) {
    const groups = rankedSearchMatches(allDiscoveryGroups(), function(group) {
      return searchMatchScore(group.name, [group.id, group.kind].join(" "), query);
    }, 18);
    sections.push({ id: "groups", title: "Groups", rows: groups.map(function(group) {
      return {
        attrs: "data-search-category=\"" + escapeHTML(group.id) + "\"",
        art: "<span class=\"logo logo-fallback\">GRP</span>",
        title: group.name || "Untitled group",
        meta: [(group.kind === "custom" ? "My Group" : "Group"), group.count ? group.count + " channels" : ""].filter(Boolean).join(" - "),
        action: "Open"
      };
    }) });
  }
  if (include("programs")) {
    const matchingPrograms = rankedSearchMatches(programs().filter(function(program) {
      return !programIsGuidePlaceholder(program) && (!state.searchAiringChannel || String(program.channelId || "") === state.searchAiringChannel);
    }), function(program) {
      const channel = channelByID(program.channelId) || {};
      return searchMatchScore(program.title, [program.summary, program.description, channel.name, channel.categoryName].join(" "), query);
    }, 18);
    sections.push({ id: "programs", title: "Guide Programs", rows: matchingPrograms.map(function(program) {
      const channel = channelByID(program.channelId) || {};
      return {
        attrs: "data-search-program-channel=\"" + escapeHTML(program.channelId || "") + "\" data-search-program=\"" + escapeHTML(program.id || "") + "\"",
        art: logoHTML(channel),
        title: program.title || "Untitled program",
        meta: [(programIsLive(program) ? "Live now" : dateTimeLabel(program.startUnix)), channel.name || "Live TV"].filter(Boolean).join(" - "),
        action: "Details",
        recordable: recordingSchedulingEnabled() && programIsUpcoming(program),
        channelId: program.channelId,
        programId: program.id || ""
      };
    }) });
  }
  if (include("airings")) {
    const airings = groupedUpcomingAirings(programs(), query).slice(0, 12);
    sections.push({ id: "airings", title: "Upcoming Airings", rows: airings.map(function(group) {
      const first = group.programs[0] || {};
      const channel = channelByID(first.channelId) || {};
      return {
        attrs: "data-search-airing=\"" + escapeHTML(group.key) + "\" data-search-airing-channel=\"" + escapeHTML(first.channelId || "") + "\"",
        art: logoHTML(channel),
        title: group.title,
        meta: [group.programs.length + " airings", dateTimeLabel(first.startUnix), channel.name || ""].filter(Boolean).join(" - "),
        action: "Show"
      };
    }) });
  }
  if (sportsEnabled() && include("sports")) {
    const people = rankedSearchMatches(myTVSportsPeople(), function(team) {
      return searchMatchScore(team.name, [team.abbreviation, team.kind, team.leagueName].join(" "), query);
    }, 12);
    sections.push({ id: "sports-people", title: "Teams & Fighters", rows: people.map(function(team) {
      const followed = sportsFavoriteTeamMatches(team);
      return {
        attrs: "data-sports-favorite-team=\"" + escapeHTML(team.id || "") + "\" data-sports-favorite-enabled=\"" + (followed ? "false" : "true") + "\"",
        art: renderSportsTeamLogo(team, "logo"),
        title: team.name || "Team",
        meta: [team.kind || "Team", team.leagueName || ""].filter(Boolean).join(" - "),
        action: followed ? "Following" : "Follow"
      };
    }) });
    const leagues = rankedSearchMatches(items(state.sports && state.sports.leagues), function(league) {
      return searchMatchScore(league.name, [league.sportName, league.id].join(" "), query);
    }, 8);
    sections.push({ id: "sports-leagues", title: "Leagues", rows: leagues.map(function(league) {
      const followed = !!sportsFavoriteLeagueMap()[league.id];
      return {
        attrs: "data-sports-favorite-league=\"" + escapeHTML(league.id || "") + "\" data-sports-favorite-enabled=\"" + (followed ? "false" : "true") + "\"",
        art: renderSportsLeagueMark(league),
        title: league.name || "League",
        meta: [league.sportName || "Sports", "League"].join(" - "),
        action: followed ? "Following" : "Follow"
      };
    }) });
    const matchingPrograms = rankedSearchMatches(programs().filter(programLooksSports), function(program) {
      const channel = channelByID(program.channelId) || {};
      return searchMatchScore(program.title, [channel.name, channel.categoryName].join(" "), query);
    }, 12);
    sections.push({ id: "sports", title: "Sports From Guide", rows: matchingPrograms.map(function(program) {
      const channel = channelByID(program.channelId) || {};
      return {
        attrs: "data-search-program-channel=\"" + escapeHTML(program.channelId || "") + "\" data-search-program=\"" + escapeHTML(program.id || "") + "\"",
        art: logoHTML(channel),
        title: program.title || "Sports",
        meta: [(programIsLive(program) ? "Live now" : dateTimeLabel(program.startUnix)), channel.name || ""].filter(Boolean).join(" - "),
        action: "Details"
      };
    }) });
  }
  if (include("events")) {
    const trackedEvents = rankedSearchMatches(items(state.events && state.events.events), function(event) {
      return searchMatchScore(event.name || event.shortName, [event.categoryName, event.description, event.keyword].join(" "), query);
    }, 10);
    sections.push({ id: "tracked-events", title: "Events", rows: trackedEvents.map(function(event) {
      const followed = !!(featuredEventMap()[event.id] || adminFeaturedEventMap()[event.id]);
      return {
        attrs: "data-event-feature=\"" + escapeHTML(event.id || "") + "\"",
        disabled: !!adminFeaturedEventMap()[event.id],
        art: "<span class=\"logo logo-fallback\">EVT</span>",
        title: event.shortName || event.name || "Event",
        meta: [event.categoryName || "Event", eventStatusLabel(event)].filter(Boolean).join(" - "),
        action: followed ? "Following" : "Follow"
      };
    }) });
    const matchingPrograms = rankedSearchMatches(programs().filter(programLooksEvent), function(program) {
      const channel = channelByID(program.channelId) || {};
      return searchMatchScore(program.title, [channel.name, channel.categoryName].join(" "), query);
    }, 12);
    sections.push({ id: "events", title: "Events From Guide", rows: matchingPrograms.map(function(program) {
      const channel = channelByID(program.channelId) || {};
      return {
        attrs: "data-search-program-channel=\"" + escapeHTML(program.channelId || "") + "\" data-search-program=\"" + escapeHTML(program.id || "") + "\"",
        art: logoHTML(channel),
        title: program.title || "Event",
        meta: [(programIsLive(program) ? "Live now" : dateTimeLabel(program.startUnix)), channel.name || ""].filter(Boolean).join(" - "),
        action: "Details"
      };
    }) });
  }
  if (include("movies")) {
    const vodMovies = firstSearchMatches(items(state.app && state.app.vod && state.app.vod.items), function(item) { return contentMatchesSearch("vod", item, query); }, 8);
    const guideMovies = firstSearchMatches(programs(), function(program) { return programLooksMovie(program) && programMatchesSearch(program, query); }, 8);
    sections.push({ id: "movies", title: "Movies", rows: vodMovies.map(function(item) {
      return {
        attrs: "data-vod-playback=\"" + escapeHTML(item.id || "") + "\"",
        art: item.posterUrl ? "<img src=\"" + escapeHTML(item.posterUrl) + "\" alt=\"\">" : "<span class=\"logo logo-fallback\">VOD</span>",
        title: item.name || "Untitled movie",
        meta: ["Movie", contentCategoryName("vod", item), item.rating].filter(Boolean).join(" - "),
        action: "On Demand"
      };
    }).concat(guideMovies.map(function(program) {
      const channel = channelByID(program.channelId) || {};
      return {
        attrs: "data-search-program-channel=\"" + escapeHTML(program.channelId || "") + "\" data-search-program=\"" + escapeHTML(program.id || "") + "\"",
        art: logoHTML(channel),
        title: program.title || "Movie",
        meta: ["Guide", dateTimeLabel(program.startUnix), channel.name || ""].filter(Boolean).join(" - "),
        action: "Details"
      };
    })) });
  }
  if (include("shows")) {
    const shows = firstSearchMatches(items(state.app && state.app.series && state.app.series.items), function(item) { return contentMatchesSearch("series", item, query); }, 12);
    sections.push({ id: "shows", title: "Shows", rows: shows.map(function(item) {
      return {
        attrs: "data-series-open=\"" + escapeHTML(item.id || "") + "\"",
        art: item.posterUrl ? "<img src=\"" + escapeHTML(item.posterUrl) + "\" alt=\"\">" : "<span class=\"logo logo-fallback\">TV</span>",
        title: item.name || "Untitled show",
        meta: ["Show", contentCategoryName("series", item), item.releaseDate].filter(Boolean).join(" - "),
        action: "On Demand"
      };
    }) });
  }
  if (include("recordings") && state.recordings && state.recordings.available) {
    const recordings = firstSearchMatches(normalizeRecordings(state.recordings), function(recording) { return recordingMatchesSearch(recording, query); }, 12);
    sections.push({ id: "recordings", title: "Recordings", rows: recordings.map(function(recording) {
      const playbackURL = recordingPlaybackURL(recording);
      return {
        attrs: playbackURL ? "data-recording-playback=\"" + escapeHTML(playbackURL) + "\"" : "",
        disabled: !playbackURL,
        art: "<span class=\"logo logo-fallback\">REC</span>",
        title: recordingTitle(recording),
        meta: [recordingChannelName(recording), recordingWindow(recording), recordingStatus(recording)].filter(Boolean).join(" - "),
        action: playbackURL ? "Play" : "Saved"
      };
    }) });
  }
  return sections.filter(function(section) { return section.rows.length; });
}

function renderSeriesDetailPage() {
  const detail = state.seriesDetail || { episodes: [] };
  const episodes = items(detail.episodes);
  const back = "<button class=\"chip\" data-series-back=\"true\">Back to search</button>";
  const rows = episodes.length ? episodes.map(function(episode) {
    const label = "S" + String(episode.season || 0).padStart(2, "0") + " E" + String(episode.number || 0).padStart(2, "0");
    return "<button class=\"search-result\" data-episode-playback=\"" + escapeHTML(episode.id || "") + "\"><span class=\"search-result-art\"><span class=\"logo logo-fallback\">" + escapeHTML(label) + "</span></span><span class=\"search-result-main\"><strong>" + escapeHTML(episode.title || "Untitled episode") + "</strong><small>" + escapeHTML(label) + "</small></span><span class=\"search-result-action\">Play</span></button>";
  }).join("") : "<div class=\"empty\">No episodes are available.</div>";
  byId("view").innerHTML = "<section class=\"search-page\"><div class=\"search-page-head\">" + back + "<div><p class=\"eyebrow\">Series</p><h2>" + escapeHTML(detail.name || "Episodes") + "</h2></div></div><div class=\"search-result-list\">" + rows + "</div></section>";
}

async function openSeriesDetail(seriesID) {
  if (!seriesID) return;
  state.seriesDetail = { loading: true, seriesID: seriesID, episodes: [] };
  state.view = "series-detail";
  render();
  try {
    state.seriesDetail = await getJSON("/dispatcharr/api/series/info?series_id=" + encodeURIComponent(seriesID));
  } catch (error) {
    state.seriesDetail = { seriesID: seriesID, episodes: [], name: "Series unavailable" };
    showAppToast(readableError(error));
  }
  if (state.view === "series-detail") render();
}

async function playOnDemand(title, gatewayURL) {
  if (!gatewayURL) return;
  stopTimeShiftSession();
  state.currentChannel = { id: "ondemand:" + title, name: title || "On Demand", categoryName: "On Demand", streamFormat: "" };
  state.view = "player";
  render();
  try {
    await ensurePlayerLibraries();
    if (await showStreamBlockedIfNeeded(route(gatewayURL), "")) return;
    setVideoSource(route(gatewayURL), { format: "" });
  } catch (error) {
    showPlayerToast(readableError(error));
  }
}
function renderSearchResultRow(row) {
  const record = row.recordable ? "<button class=\"search-result-record\" type=\"button\" data-schedule-channel=\"" + escapeHTML(row.channelId || "") + "\" data-schedule-program=\"" + escapeHTML(row.programId || "") + "\">Record</button>" : "";
  return "<div class=\"search-result-row\"><button class=\"search-result\" type=\"button\" " + (row.attrs || "") + (row.disabled ? " disabled" : "") + "><span class=\"search-result-art\">" + row.art + "</span><span class=\"search-result-main\"><strong>" + escapeHTML(row.title) + "</strong><small>" + escapeHTML(row.meta || "") + "</small></span><span class=\"search-result-action\">" + escapeHTML(row.action || "") + "</span></button>" + record + (row.favoriteChannelId ? channelFavoriteButton(row.favoriteChannelId, row.title) : "") + "</div>";
}
function renderSearchResultCard(row) {
  const record = row.recordable ? "<button class=\"search-result-record\" type=\"button\" data-schedule-channel=\"" + escapeHTML(row.channelId || "") + "\" data-schedule-program=\"" + escapeHTML(row.programId || "") + "\">Record</button>" : "";
  return "<div class=\"search-result-card\"><button class=\"search-result-card-main\" type=\"button\" " + (row.attrs || "") + (row.disabled ? " disabled" : "") + "><span class=\"search-result-card-art\">" + row.art + "</span><span class=\"search-result-card-copy\"><strong>" + escapeHTML(row.title) + "</strong><small>" + escapeHTML(row.meta || "") + "</small></span><span class=\"search-result-card-action\">" + escapeHTML(row.action || "") + "</span></button>" + record + "</div>";
}
function renderSearchResults(query) {
  const sections = searchResultSections(query);
  const savePass = query && !keywordPasses().some(function(pass) { return lower(pass.keyword) === lower(query); }) ? "<button class=\"search-save-pass\" type=\"button\" data-keyword-pass-add=\"" + escapeHTML(query) + "\">Follow search</button>" : "";
  if (!sections.length) return "<div class=\"search-empty\"><span>" + icon("search") + "</span><strong>No current matches for &ldquo;" + escapeHTML(query) + "&rdquo;</strong><p>Add it to My TV and we’ll watch future guide listings.</p>" + savePass + "</div>";
  return (savePass ? "<div class=\"search-pass-action\"><span class=\"search-pass-copy\"><strong>Follow &ldquo;" + escapeHTML(query) + "&rdquo; in My TV</strong><small>See matching guide listings as they become available.</small></span>" + savePass + "</div>" : "") + "<div class=\"search-results\">" + sections.map(function(section) {
    return "<section class=\"search-result-section\"><header class=\"search-result-section-head\"><h3>" + escapeHTML(section.title) + "</h3><span>" + section.rows.length + (section.rows.length === 1 ? " result" : " results") + "</span></header><div class=\"search-result-list\">" + section.rows.map(renderSearchResultRow).join("") + "</div></section>";
  }).join("") + "</div>";
}
const SEARCH_RESULTS_DELAY_MS = 180;
const SEARCH_MIN_QUERY_LENGTH = 2;
let searchResultsTimer = null;
function clearSearchResultsTimer() {
  if (!searchResultsTimer) return;
  clearTimeout(searchResultsTimer);
  searchResultsTimer = null;
}
function renderSearchPageResults() {
  const query = searchNeedle();
  if (!query) return renderSearchStart();
  if (query.length < SEARCH_MIN_QUERY_LENGTH) return "<div class=\"search-start-empty search-query-hint\"><span>" + icon("search") + "</span><strong>Keep typing</strong><p>Enter at least two characters to search the full lineup.</p></div>";
  return renderSearchResults(query);
}
function updateSearchPageResults() {
  const root = byId("search-page-results");
  if (!root) {
    renderSearchPage();
    return;
  }
  root.innerHTML = renderSearchPageResults();
}
function scheduleSearchResultsUpdate() {
  clearSearchResultsTimer();
  searchResultsTimer = setTimeout(function() {
    searchResultsTimer = null;
    if (state.view === "search") updateSearchPageResults();
  }, SEARCH_RESULTS_DELAY_MS);
}
function updateLiveSearchFilter() {
  if (refreshGuideRowsForQuery()) return;
  render();
}
function renderSearchStart() {
  const recent = items(state.recentSearches);
  const recentHTML = recent.length ? sectionHeaderWithActions("Recent searches", "<button class=\"search-clear\" type=\"button\" data-search-clear=\"true\">Clear All</button>") + "<div class=\"search-chip-row\">" + recent.map(function(value) {
    return "<button class=\"search-chip\" type=\"button\" data-search-recent=\"" + escapeHTML(value) + "\">" + escapeHTML(value) + "</button>";
  }).join("") + "</div>" : "";
  const passes = keywordPasses();
  const passHTML = passes.length ? sectionHeader("Keyword Passes") + "<div class=\"search-chip-row\">" + passes.map(function(pass) {
    return "<button class=\"search-chip\" type=\"button\" data-search-recent=\"" + escapeHTML(pass.keyword) + "\">" + escapeHTML(pass.keyword) + "</button>";
  }).join("") + "</div>" : "";
  const browsed = recentChannels(10);
  const browsedHTML = browsed.length ? sectionHeader("Recently browsed") + rowCards(browsed) : "";
  if (!recentHTML && !passHTML && !browsedHTML) return "<div class=\"search-start-empty\"><span>" + icon("search") + "</span><strong>Search your entire lineup</strong><p>Find live channels, groups, guide programs, movies, and shows.</p></div>";
  return recentHTML + passHTML + browsedHTML;
}
function renderSearchPage() {
  const root = byId("view");
  const main = document.querySelector(".main");
  if (main) main.scrollLeft = 0;
  if (root) root.scrollLeft = 0;
  const query = state.searchQuery || "";
  const filter = state.searchType || "all";
  const filterHTML = "<div class=\"search-scope-row\" aria-label=\"Search scope\">" + searchFilters().map(function(item) {
    return "<button class=\"search-chip" + (filter === item.id ? " active" : "") + "\" type=\"button\" data-search-type=\"" + escapeHTML(item.id) + "\">" + escapeHTML(item.label) + "</button>";
  }).join("") + "</div>";
  const clear = query ? "<button class=\"search-query-clear\" type=\"button\" data-search-query-clear=\"true\" aria-label=\"Clear search\">" + icon("x") + "</button>" : "";
  root.innerHTML = "<div class=\"search-page\"><header class=\"search-commandbar\"><div class=\"search-commandbar-title\"><h2>Search</h2><span>Channels, programs, movies or shows</span></div><div class=\"search-form\"><label class=\"search-input-shell\"><span>" + icon("search") + "</span><input id=\"search-page-input\" class=\"search-field\" value=\"" + escapeHTML(query) + "\" placeholder=\"Search everything on TV\" autocomplete=\"off\" spellcheck=\"false\">" + clear + "</label><button class=\"search-cancel\" type=\"button\" data-search-cancel=\"true\">Done</button></div>" + filterHTML + "</header><div id=\"search-page-results\" class=\"search-page-results\" aria-live=\"polite\">" + renderSearchPageResults() + "</div></div>";
  const input = byId("search-page-input");
  if (input && document.activeElement !== input) {
    setTimeout(function() {
      const focused = byId("search-page-input");
      if (focused) {
        focused.focus();
        focused.setSelectionRange(focused.value.length, focused.value.length);
      }
    }, 0);
  }
}
function addKeywordPass(keyword) {
  keyword = String(keyword || "").trim();
  if (!keyword || !state.app || !state.app.preferences) return;
  state.app.preferences.keywordPasses = normalizeKeywordPasses(keywordPasses().concat([{ keyword: keyword, createdAt: Date.now() }]));
  savePrefs();
  showAppToast("Added to My TV.");
  if (state.view === "mytv") updateMyTVSearchSurface();
  else renderSearchPage();
}
function removeKeywordPass(id) {
  id = String(id || "");
  if (!state.app || !state.app.preferences) return;
  state.app.preferences.keywordPasses = keywordPasses().filter(function(pass) { return pass.id !== id; });
  savePrefs();
  if (state.view === "mytv") updateMyTVSearchSurface();
  else renderOnLaterPage();
}
function onLaterFilters() {
  return [
    { id: "all", label: "All" },
    { id: "live", label: "Live Now" },
    { id: "today", label: "Today" },
    { id: "sports", label: "Sports" },
    { id: "events", label: "Events" },
    { id: "movies", label: "Movies" },
    { id: "passes", label: "Passes" }
  ];
}
function onLaterPrograms(options) {
  options = options || {};
  const now = Math.floor(Date.now() / 1000);
  const endOfToday = new Date();
  endOfToday.setHours(23, 59, 59, 999);
  const todayEnd = Math.floor(endOfToday.getTime() / 1000);
  const time = state.onLaterTime || "all";
  const type = state.onLaterType || "all";
  return programsFor("").filter(function(program) {
    if (programIsGuidePlaceholder(program)) return false;
    if ((program.endUnix || 0) < now) return false;
    if (time === "live" && !programIsLive(program)) return false;
    if (time === "today" && ((program.startUnix || 0) < now - 3600 || (program.startUnix || 0) > todayEnd)) return false;
    if (!options.ignoreType && !programMatchesOnLaterType(program, type)) return false;
    return true;
  }).sort(function(left, right) {
    return (left.startUnix || 0) - (right.startUnix || 0);
  });
}
function renderProgramDiscoveryRow(program) {
  const channel = channelByID(program.channelId) || {};
  return renderSearchResultRow({
    attrs: "data-search-program-channel=\"" + escapeHTML(program.channelId || "") + "\" data-search-program=\"" + escapeHTML(program.id || "") + "\"",
    art: logoHTML(channel),
    title: program.title || "Untitled program",
    meta: [(programIsLive(program) ? "Live now" : dateTimeLabel(program.startUnix)), channel.name || ""].filter(Boolean).join(" - "),
    action: "Details",
    recordable: recordingSchedulingEnabled() && programIsUpcoming(program),
    channelId: program.channelId,
    programId: program.id || ""
  });
}
function renderProgramDiscoveryCard(program) {
  const channel = channelByID(program.channelId) || {};
  return renderSearchResultCard({
    attrs: "data-search-program-channel=\"" + escapeHTML(program.channelId || "") + "\" data-search-program=\"" + escapeHTML(program.id || "") + "\"",
    art: logoHTML(channel),
    title: program.title || "Untitled program",
    meta: [(programIsLive(program) ? "Live now" : dateTimeLabel(program.startUnix)), channel.name || ""].filter(Boolean).join(" - "),
    action: "Details",
    recordable: recordingSchedulingEnabled() && programIsUpcoming(program),
    channelId: program.channelId,
    programId: program.id || ""
  });
}
function renderOnLaterPage() {
  const root = byId("view");
  const time = state.onLaterTime || "all";
  const type = state.onLaterType || "all";
  const timePrograms = onLaterPrograms({ ignoreType: true });
  const typeCounts = { all: timePrograms.length, sports: 0, events: 0, movies: 0, passes: 0 };
  timePrograms.forEach(function(program) {
    const programType = programOnLaterType(program);
    if (Object.prototype.hasOwnProperty.call(typeCounts, programType)) typeCounts[programType] += 1;
    if (programMatchesOnLaterType(program, "passes")) typeCounts.passes += 1;
  });
  const filterButton = function(item, group, label) {
    const active = (group === "time" ? time : type) === item.id;
    const count = group === "type" ? "<span class=\"search-chip-count\">" + escapeHTML(String(typeCounts[item.id] || 0)) + "</span>" : "";
    return "<button class=\"search-chip" + (active ? " active" : "") + "\" type=\"button\" data-onlater-" + group + "=\"" + escapeHTML(item.id) + "\" aria-pressed=\"" + (active ? "true" : "false") + "\"><span>" + escapeHTML(label || item.label) + "</span>" + count + "</button>";
  };
  const byID = function(id) { return onLaterFilters().find(function(item) { return item.id === id; }); };
  const filters = '<div class="filter-sections"><div class="on-later-filter-group filter-section" data-on-later-filter-group="time"><span class="filter-section-label">Time</span><div class="search-chip-row">' + ["all", "live", "today"].map(function(id) { return filterButton(byID(id), "time"); }).join("") + '</div></div><div class="on-later-filter-group filter-section" data-on-later-filter-group="type"><span class="filter-section-label">Type</span><div class="search-chip-row">' + ["all", "sports", "events", "movies", "passes"].map(function(id) { return filterButton(byID(id), "type", id === "all" ? "All Types" : ""); }).join("") + "</div></div></div>";
  const programs = onLaterPrograms();
  const airings = groupedUpcomingAirings(programs, "").slice(0, 16);
  const passes = keywordPasses();
  const passHTML = passes.length ? sectionHeader("Keyword Passes") + "<div class=\"keyword-pass-list\">" + passes.map(function(pass) {
    return "<div class=\"keyword-pass\"><button type=\"button\" data-search-recent=\"" + escapeHTML(pass.keyword) + "\"><strong>" + escapeHTML(pass.keyword) + "</strong><small>" + escapeHTML(String(onLaterPrograms().filter(function(program) { return lower(programSearchText(program)).indexOf(lower(pass.keyword)) !== -1; }).length)) + " matches</small></button><button type=\"button\" data-keyword-pass-remove=\"" + escapeHTML(pass.id) + "\">Remove</button></div>";
  }).join("") + "</div>" : "";
  root.innerHTML = "<div class=\"search-page on-later-page\"><div class=\"search-hero\"><h2>On Later</h2><p>Upcoming guide content organized for watching and recording.</p></div>" + filters
    + (passHTML && (type === "all" || type === "passes") ? passHTML : "")
    + (airings.length && time !== "live" ? sectionHeader("Upcoming Airings") + "<div class=\"on-later-card-grid\">" + airings.map(function(group) {
      const first = group.programs[0] || {};
      const channel = channelByID(first.channelId) || {};
      return renderSearchResultCard({ attrs: "data-search-airing=\"" + escapeHTML(group.key) + "\"", art: logoHTML(channel), title: group.title, meta: [group.programs.length + " airings", dateTimeLabel(first.startUnix), channel.name || ""].filter(Boolean).join(" - "), action: "Show" });
    }).join("") + "</div>" : "")
    + sectionHeader(type !== "all" ? byID(type).label : (time !== "all" ? byID(time).label : "Guide Picks"))
    + (programs.length ? "<div class=\"on-later-card-grid\">" + programs.slice(0, 60).map(renderProgramDiscoveryCard).join("") + "</div>" : "<div class=\"empty\">No matching guide entries.</div>")
    + "</div>";
}
function uniqueEventChannels(channels) {
  const seen = {};
  return items(channels).filter(function(channel) {
    if (!channelMatchesProfileSelection(channel)) return false;
    const key = channel && channel.id ? "id:" + channel.id : "label:" + lower([channel && channel.name, channel && channel.categoryName, channel && channel.reason].join("|"));
    if (!key || seen[key]) return false;
    seen[key] = true;
    return true;
  });
}
function loadEvents(force) {
  if (state.eventsLoading) return Promise.resolve(state.events || { events: [], categories: [] });
  if (state.events && !force) return Promise.resolve(state.events);
  state.eventsLoading = true;
  return getJSON("/dispatcharr/api/events" + (force && siloUserIsAdmin() ? "?refresh=1" : "")).then(function(payload) {
    state.events = payload || { events: [], categories: [] };
    delete state.events.error;
    return state.events;
  }).catch(function(error) {
    if (!state.events) state.events = { events: [], categories: [], error: readableError(error) };
    else state.events.error = readableError(error);
    showAppToast("Could not refresh events.");
    return state.events;
  }).finally(function() {
    state.eventsLoading = false;
    if (state.view === "events") renderEventsPage();
  });
}
function eventTabLabel(tab) {
  return ({ live: "Live", upcoming: "Upcoming", all: "All" })[tab] || "Live";
}
function eventTabButtonsHTML() {
  return ["upcoming", "live", "all"].map(function(tab) {
    return "<button type=\"button\" data-event-tab=\"" + tab + "\" class=\"" + (state.eventsTab === tab ? "active" : "") + "\" aria-pressed=\"" + (state.eventsTab === tab ? "true" : "false") + "\">" + escapeHTML(eventTabLabel(tab)) + "</button>";
  }).join("");
}
function renderEventTabFilters() {
  return "<div class=\"sports-filter-row\"><div class=\"view-toggle\" aria-label=\"Events filter\">" + eventTabButtonsHTML() + "</div></div>";
}
function renderEventsPage() {
  const root = byId("view");
  if (!state.events && !state.eventsLoading) loadEvents(false);
  renderSportsTopbarTabs();
  const payload = state.events || { events: [], categories: [] };
  const events = filteredBroadcastEvents(payload);
  root.innerHTML = "<div class=\"sports-page\"><div class=\"sports-pinned\">" + renderEventTabFilters() + renderEventCategoryFilters(payload)
    + recoveryPanelHTML(payload.error, "events")
    + (state.eventsLoading && !events.length ? "<div class=\"empty\">Loading events...</div>" : "")
    + "</div><div class=\"sports-score-scroll\">"
    + (events.length ? "<div class=\"sports-board\">" + events.map(renderBroadcastEventCard).join("") + "</div>" : (!state.eventsLoading ? "<div class=\"empty\">No matching events.</div>" : ""))
    + "</div>"
    + "</div>";
}
function renderEventCategoryFilters(payload) {
  const categories = items(payload && payload.categories);
  if (!categories.length) return "";
  const chips = ["<button class=\"chip" + (!state.eventCategory ? " active" : "") + "\" data-event-category=\"\">All events</button>"].concat(categories.map(function(category) {
    const label = category.name || category.id || "Events";
    return "<button class=\"chip" + (state.eventCategory === category.id ? " active" : "") + "\" data-event-category=\"" + escapeHTML(category.id) + "\">" + escapeHTML(label) + "</button>";
  }));
  return "<div class=\"sports-leagues\">" + chips.join("") + "</div>";
}
function recoveryPanelHTML(error, kind) {
  if (!error) return "";
  const label = kind === "sports" ? "sports" : "events";
  return '<div class="recovery-panel"><span role="status" aria-live="polite">' + escapeHTML(error) + "</span><div class=\"recovery-actions\"><button type=\"button\" data-recovery-retry=\"" + label + "\" aria-label=\"Retry loading " + label + "\">Retry</button><button type=\"button\" data-recovery-reload=\"true\" aria-label=\"Reload Live TV\">Reload</button></div></div>";
}
function eventStatusLabel(event) {
  if (event.live) return "Live";
  if (event.completed) return "Ended";
  if (event.startUnix) return sportsDateLabel(event.startUnix);
  return "Time TBD";
}
function setEventTab(tab) {
  state.eventsTab = tab || "live";
  state.expandedEvents = {};
  renderEventsPage();
}
function setEventCategory(categoryID) {
  state.eventCategory = categoryID || "";
  state.expandedEvents = {};
  renderEventsPage();
}
function favoriteCards(channels, hasFavorites) {
  if (!channels.length) {
    const filtered = !!(hasFavorites && state.folderQuery);
    return "<div class=\"favorites-empty\"><div class=\"favorites-empty-icon\">" + icon(filtered ? "search" : "heart") + "</div><strong>" + (filtered ? "No matching favorites" : "No favorite channels yet") + "</strong><p>" + (filtered ? "Try a different channel or program name." : "Favorite a channel from Home or Guide and it will appear here.") + "</p>" + (filtered ? "" : "<button type=\"button\" class=\"favorites-empty-action\" data-favorites-action=\"guide\">" + icon("guide") + "<span>Open Guide</span></button>") + "</div>";
  }
  return "<div class=\"favorites-grid\">" + channels.map(function(channel, index) {
    const reorder = channels.length > 1 ? "<button type=\"button\" class=\"favorite-order-up\" data-favorite-move=\"up\" data-channel-id=\"" + escapeHTML(channel.id) + "\" aria-label=\"Move " + escapeHTML(channel.name || "channel") + " up\" title=\"Move up\"" + (index === 0 ? " disabled" : "") + ">" + icon("chevron-down") + "</button><button type=\"button\" data-favorite-move=\"down\" data-channel-id=\"" + escapeHTML(channel.id) + "\" aria-label=\"Move " + escapeHTML(channel.name || "channel") + " down\" title=\"Move down\"" + (index === channels.length - 1 ? " disabled" : "") + ">" + icon("chevron-down") + "</button>" : "";
    const controls = "<div class=\"favorite-card-actions\" aria-label=\"Manage " + escapeHTML(channel.name || "channel") + "\"><button type=\"button\" class=\"favorite-remove\" data-favorite-remove data-channel-id=\"" + escapeHTML(channel.id) + "\" aria-label=\"Remove " + escapeHTML(channel.name || "channel") + " from favorites\" title=\"Remove from favorites\">" + icon("heart-solid") + "</button>" + reorder + "</div>";
    return "<article class=\"favorite-card\"><button class=\"continue-card\" data-channel=\"" + escapeHTML(channel.id) + "\"><div class=\"poster-box\">" + (channel.logoUrl ? "<img src=\"" + escapeHTML(channel.logoUrl) + "\" alt=\"\">" : "<span>" + escapeHTML((channel.name || "TV").slice(0, 5)) + "</span>") + "</div><strong>" + escapeHTML(channel.name || "Untitled") + "</strong><div class=\"muted\">" + escapeHTML(channel.categoryName || "Live TV") + "</div></button>" + controls + "</article>";
  }).join("") + "</div>";
}
function favoritesPage(channels) {
  const filtered = channels.filter(channelMatchesFolderQuery).slice(0, 60);
  const count = channels.length;
  const countLabel = count + " " + (count === 1 ? "channel" : "channels");
  return "<section class=\"favorites-page\"><header class=\"favorites-header\"><h2>Favorite channels</h2><div class=\"favorites-count\">" + escapeHTML(countLabel) + "</div></header><label class=\"favorites-search\"><span>" + icon("search") + "</span><input id=\"folder-filter\" type=\"search\" placeholder=\"Filter favorites\" value=\"" + escapeHTML(state.folderQuery || "") + "\" autocomplete=\"off\" aria-label=\"Filter favorite channels\"></label>" + favoriteCards(filtered, count > 0) + "</section>";
}
function compareCategoryDisplayName(left, right) {
  const leftName = String((left && (left.name || left.id)) || "");
  const rightName = String((right && (right.name || right.id)) || "");
  return leftName.localeCompare(rightName, undefined, { numeric: true, sensitivity: "base" }) || leftName.localeCompare(rightName) || String((left && left.id) || "").localeCompare(String((right && right.id) || ""));
}
function categoryGrid(context) {
  const hidden = hiddenMap();
  const sourceCategories = sourceCategoriesWithChannels(function(channel) {
    return !(channel.categoryId && hidden[channel.categoryId]);
  });
  const custom = customGroupCategories();
  const listing = adminListingCategories("");
  const featured = sourceCategories.filter(function(category) { return !!category.featured; }).sort(compareCategoryDisplayName);
  const featuredSourceIDs = {};
  featured.forEach(function(category) { featuredSourceIDs[category.sourceID] = true; });
  const regularListing = listing.filter(function(category) {
    return !(category.kind === "source" && featuredSourceIDs[category.sourceID]);
  });
  const sections = [];
  if (featured.length) sections.push(categoryGridSection(featuredGroupLabel(), featured, context));
  if (custom.length) sections.push(categoryGridSection("My Groups", custom, context));
  if (regularListing.length) sections.push(categoryGridSection(adminListingTitle(), regularListing, context));
  if (!listing.length && sourceCategories.length) sections.push(categoryGridSection(adminListingTitle(), sourceCategories, context));
  return sections.length ? sections.join("") : "<div class=\"empty\">No groups yet.</div>";
}
function categoryGridSection(title, categories, context) {
  if (context === "home") return homeSection(title, "<div class=\"category-grid\">" + categories.map(categoryTileHTML).join("") + "</div>", "", "", "home-category-section");
  return sectionHeader(title) + "<div class=\"category-grid\">" + categories.map(categoryTileHTML).join("") + "</div>";
}
function categoryTileHTML(category) {
  const name = String((category && (category.name || category.id)) || "");
  const meta = String((category && category.count ? category.count + " channels" : (category && category.kind) || "") || "");
  return "<button class=\"tile" + (state.category === category.id ? " active" : "") + "\" data-category=\"" + escapeHTML(category.id) + "\" aria-label=\"" + escapeHTML(meta ? name + ", " + meta : name) + "\"><span class=\"tile-copy\"><strong data-overflow-tooltip=\"" + escapeHTML(name) + "\">" + escapeHTML(name) + "</strong><span>" + escapeHTML(meta) + "</span></span><span class=\"tile-disclosure\" aria-hidden=\"true\">" + icon("chevron-right") + "</span></button>";
}
function activeVirtualCategoryID(path, featured) {
  return featured ? featuredCategoryID(path) : virtualCategoryID(path);
}
function virtualFolderHeader(path, featured) {
  return "<div class=\"section-title virtual-folder-title\">" + virtualFolderBreadcrumbs(path, featured) + "<div class=\"virtual-folder-actions\">" + guideFreshnessHTML() + renderVirtualCategoryViewToggle() + "</div></div>";
}
function virtualFolderBreadcrumbs(path, featured) {
  const parts = path.split(" / ").filter(Boolean);
  const rootLabel = featured ? featuredGroupLabel() : virtualGroupLabel();
  const crumbs = ["<button data-category=\"\">" + escapeHTML(rootLabel) + "</button>"];
  parts.forEach(function(part, index) {
    const crumbPath = parts.slice(0, index + 1).join(" / ");
    crumbs.push("<span class=\"sep\">/</span><button data-category=\"" + escapeHTML(activeVirtualCategoryID(crumbPath, featured)) + "\">" + escapeHTML(part) + "</button>");
  });
  return "<div class=\"breadcrumbs\" aria-label=\"Virtual folder breadcrumbs\">" + crumbs.join("") + "</div>";
}
function sourceCategoriesWithChannels(includeChannel) {
  const categoryCounts = {};
  effectiveChannels(false).forEach(function(channel) {
    if (includeChannel && !includeChannel(channel)) return;
    if (channel.categoryId) categoryCounts[channel.categoryId] = (categoryCounts[channel.categoryId] || 0) + 1;
  });
  return items(state.app.categories).filter(function(category) {
    return !!categoryCounts[category.id];
  }).map(function(category) {
    const rawName = category.name || category.id;
    const name = renamedCategoryDisplayName(rawName);
    const featured = categoryStartsFeatured(rawName);
    const featuredPath = featured ? featuredPathForSourceName(rawName) : "";
    return { id: featuredPath ? featuredCategoryID(featuredPath) : sourceCategoryID(category.id), sourceID: category.id, name: name, featured: featured, kind: featuredPath ? "featured" : "source", count: categoryCounts[category.id] || 0 };
  });
}
function customGroupCategories() {
  return customGroups().map(function(group) {
    return { id: customCategoryID(group.id), name: group.name, kind: "custom", count: customMemberships(group.id).filter(function(id) { return channelMatchesProfileSelection(channelByID(id)); }).length };
  }).filter(function(group) {
    return group.count > 0;
  });
}
function virtualGroupCategories(includeChannel) {
  return categoriesFromChannelPaths("", includeChannel, virtualPathsForChannel, virtualCategoryID, "virtual", true);
}
function virtualCategoriesFromPaths(parentPath, includeChannel, includeAllDescendants) {
  return categoriesFromChannelPaths(parentPath, includeChannel, virtualPathsForChannel, virtualCategoryID, "virtual", includeAllDescendants);
}
function featuredCategoriesFromPaths(parentPath, includeChannel, includeAllDescendants) {
  return categoriesFromChannelPaths(parentPath, includeChannel, featuredPathsForChannel, featuredCategoryID, "featured", includeAllDescendants);
}
function categoriesFromChannelPaths(parentPath, includeChannel, pathsForChannel, categoryIDForPath, kind, includeAllDescendants) {
  parentPath = String(parentPath || "");
  const groups = {};
  effectiveChannels(false).forEach(function(channel) {
    if (includeChannel && !includeChannel(channel)) return;
    pathsForChannel(channel).forEach(function(path) {
      const parts = path.split(" / ").filter(Boolean);
      const parentParts = parentPath ? parentPath.split(" / ").filter(Boolean) : [];
      if (parts.length <= parentParts.length) return;
      for (let index = 0; index < parentParts.length; index++) {
        if (parts[index] !== parentParts[index]) return;
      }
      const limit = includeAllDescendants ? parts.length : parentParts.length + 1;
      for (let index = parentParts.length; index < limit; index++) {
        const childPath = parts.slice(0, index + 1).join(" / ");
        groups[childPath] = groups[childPath] || { id: categoryIDForPath(childPath), name: includeAllDescendants ? childPath : parts[index], kind: kind, count: 0, channelIDs: {} };
        groups[childPath].channelIDs[channel.id] = true;
      }
    });
  });
  return Object.keys(groups).sort().map(function(path) {
    const group = groups[path];
    group.count = Object.keys(group.channelIDs).length;
    delete group.channelIDs;
    return group;
  });
}
function sourceVirtualChildCategories(parentPath, includeChannel) {
  return childCategoriesFromChannelPaths(parentPath, includeChannel, virtualPathsForChannel, virtualCategoryID, "virtual");
}
function featuredChildCategories(parentPath, includeChannel) {
  return childCategoriesFromChannelPaths(parentPath, includeChannel, featuredPathsForChannel, featuredCategoryID, "featured");
}
function childCategoriesFromChannelPaths(parentPath, includeChannel, pathsForChannel, categoryIDForPath, kind) {
  parentPath = String(parentPath || "");
  const children = {};
  effectiveChannels(false).forEach(function(channel) {
    if (includeChannel && !includeChannel(channel)) return;
    pathsForChannel(channel).forEach(function(groupPath) {
      const parts = groupPath.split(" / ").filter(Boolean);
      for (let index = 0; index < parts.length; index++) {
        const path = parts.slice(0, index + 1).join(" / ");
        const parentParts = parentPath ? parentPath.split(" / ").filter(Boolean) : [];
        if (parts.length <= parentParts.length) return;
        for (let parentIndex = 0; parentIndex < parentParts.length; parentIndex++) {
          if (parts[parentIndex] !== parentParts[parentIndex]) return;
        }
        if (index !== parentParts.length) continue;
        children[path] = children[path] || { id: categoryIDForPath(path), name: parts[parentParts.length], kind: kind, count: 0, channelIDs: {} };
        children[path].channelIDs[channel.id] = true;
      }
    });
  });
  return Object.keys(children).sort().map(function(path) {
    const child = children[path];
    child.count = Object.keys(child.channelIDs || {}).length;
    delete child.channelIDs;
    return child;
  });
}
function virtualChildCategories(parentPath, includeChannel) {
  return sourceVirtualChildCategories(parentPath, includeChannel);
}
function allFilterCategories() {
  const hidden = hiddenMap();
  return customGroupCategories().concat(adminListingCategories("", function(channel) { return !(channel.categoryId && hidden[channel.categoryId]); }));
}
function organizationRootLabel() {
  return useProfileGroupVirtualPaths() ? "Channel Profiles" : "Channel Groups";
}
function configuredGroupLabel() {
  return virtualGroupLabelSuffix(adminSettings().virtualGroupLabel);
}
function virtualGroupLabel() {
  return configuredGroupLabel();
}
function featuredGroupLabel() {
  return "Featured " + (categoryParsing().enabled ? configuredGroupLabel() : "Channel Groups");
}
function allGroupLabel() {
  return "All " + adminListingTitle().toLowerCase();
}
function virtualGroupLabelSuffix(value) {
  value = String(value || "").trim().replace(/^virtual\s+/i, "").trim();
  return value || "Groups";
}
function virtualCategoriesActive() {
  const hidden = hiddenMap();
  return categoryParsing().enabled && virtualGroupCategories(function(channel) { return !(channel.categoryId && hidden[channel.categoryId]); }).length > 0;
}
function recentChannels(limit) {
  const seen = {};
  const channels = [];
  items(prefs().recentChannels).forEach(function(id) {
    if (seen[id]) return;
    const channel = channelByID(id);
    if (!channel || !channelMatchesProfileSelection(channel)) return;
    seen[id] = true;
    channels.push(channel);
  });
  return channels.slice(0, limit || channels.length);
}
function categoryBrowseSettings() {
  const value = Object.assign({}, defaultPrefs().categoryBrowse, prefs().categoryBrowse || {});
  value.sort = ["provider", "name", "recent"].indexOf(value.sort) === -1 ? "provider" : value.sort;
  value.layout = value.layout === "list" ? "list" : "grid";
  value.logoStyle = value.logoStyle === "greyscale" ? "greyscale" : "color";
  value.cleanNames = value.cleanNames === true;
  return value;
}
function updateCategoryBrowseSetting(key, value) {
  if (!state.app || !state.app.preferences) return;
  const settings = categoryBrowseSettings();
  settings[key] = value;
  state.app.preferences.categoryBrowse = settings;
  state.categoryBrowseView = settings.layout;
  state.categoryMenuOpen = false;
  savePrefs({ quiet: true });
  render();
}
function sortedCategoryChannels(channels) {
  const settings = categoryBrowseSettings();
  const result = items(channels).slice();
  if (settings.sort === "name") {
    return result.sort(function(left, right) { return String(left.name || "").localeCompare(String(right.name || ""), undefined, { numeric: true }); });
  }
  if (settings.sort === "recent") {
    const recent = {};
    items(prefs().recentChannels).forEach(function(id, index) { recent[id] = index; });
    return result.map(function(channel, index) { return { channel: channel, index: index }; }).sort(function(left, right) {
      const leftRecent = Object.prototype.hasOwnProperty.call(recent, left.channel.id) ? recent[left.channel.id] : Number.MAX_SAFE_INTEGER;
      const rightRecent = Object.prototype.hasOwnProperty.call(recent, right.channel.id) ? recent[right.channel.id] : Number.MAX_SAFE_INTEGER;
      return leftRecent - rightRecent || left.index - right.index;
    }).map(function(entry) { return entry.channel; });
  }
  return result;
}
function categoryChannelName(channel) {
  const name = String(channel && channel.name || "Untitled").trim();
  if (!categoryBrowseSettings().cleanNames) return name;
  return name
    .replace(/^\s*(?:\[[A-Za-z0-9]{2,4}\]|\([A-Za-z0-9]{2,4}\)|(?:USA?|UK|CA|CAN|AU|NZ))\s*(?:[|:\-]\s*)?/i, "")
    .replace(/\s*(?:\[(?:SD|HD|FHD|UHD|4K)\]|\((?:SD|HD|FHD|UHD|4K)\))\s*$/i, "")
    .trim() || name;
}
function virtualCategoryView() {
  return state.virtualCategoryView === "list" ? "list" : "guide";
}
function renderVirtualCategoryViewToggle() {
  const active = virtualCategoryView();
  return "<div class=\"view-toggle\" aria-label=\"Virtual category view\"><button type=\"button\" data-virtual-category-view=\"guide\" class=\"" + (active === "guide" ? "active" : "") + "\" aria-pressed=\"" + (active === "guide" ? "true" : "false") + "\">Guide</button><button type=\"button\" data-virtual-category-view=\"list\" class=\"" + (active === "list" ? "active" : "") + "\" aria-pressed=\"" + (active === "list" ? "true" : "false") + "\">List</button></div>";
}
function renderVirtualCategoryChannelList(channels) {
  if (!channels.length) return sectionHeader("Channels") + "<div class=\"empty\">No channels in this virtual group yet.</div>";
  return sectionHeader("Channels") + "<div class=\"channel-button-list\">" + channels.map(function(channel) {
    const subtitle = channelProgramSubtitle(channel);
    return "<button class=\"virtual-channel-button\" data-channel=\"" + escapeHTML(channel.id) + "\">" + logoHTML(channel) + "<span><strong>" + escapeHTML(channel.name || "Untitled") + "</strong><span>" + escapeHTML(subtitle) + "</span></span></button>";
  }).join("") + "</div>";
}
function renderVirtualCategoryContent(channels) {
  return virtualCategoryView() === "list" ? renderVirtualCategoryChannelList(channels) : renderVirtualCategoryGuide(channels);
}
function setVirtualCategoryView(view) {
  state.virtualCategoryView = view === "list" ? "list" : "guide";
  renderLivePage();
}
function folderSearchNeedle() {
  return lower(String(state.folderQuery || "").trim());
}
function channelMatchesFolderQuery(channel) {
  const query = folderSearchNeedle();
  if (!query) return true;
  const current = currentProgram(channel) || {};
  const next = nextProgram(channel) || {};
  return lower([
    channel && channel.name,
    channel && channel.categoryName,
    sourceCategoryLabel(channel || {}),
    channel && channel.number,
    channel && channel.id,
    current.title,
    current.description,
    next.title,
    next.description
  ].join(" ")).indexOf(query) !== -1;
}
function categoryMatchesFolderQuery(category) {
  const query = folderSearchNeedle();
  if (!query) return true;
  return lower([
    category && category.name,
    category && category.id,
    category && category.kind
  ].join(" ")).indexOf(query) !== -1;
}
function folderFilterHTML(placeholder, actionsHTML) {
  return "<div class=\"folder-filter\"><input id=\"folder-filter\" class=\"search\" type=\"search\" placeholder=\"" + escapeHTML(placeholder || "Filter visible channels") + "\" value=\"" + escapeHTML(state.folderQuery || "") + "\" autocomplete=\"off\">" + (actionsHTML ? "<div class=\"folder-filter-actions\">" + actionsHTML + "</div>" : "") + "</div>";
}
function renderLivePage() {
  const channels = visibleChannels(false);
  if (state.view === "favorites") {
    byId("view").innerHTML = favoritesPage(channels);
    return;
  }
  const filteredChannels = channels.filter(channelMatchesFolderQuery);
  if (state.category) {
    const folder = selectedNestedFolder(state.category);
    const children = nestedFolderChildren(state.category);
    if (folder && children.length) {
      const filteredChildren = children.filter(categoryMatchesFolderQuery);
      byId("view").innerHTML = "<div class=\"section-title virtual-folder-title\">" + virtualFolderBreadcrumbs(folder.path, folder.featured) + "</div>"
        + folderFilterHTML("Filter this folder")
        + (filteredChildren.length ? "<div class=\"category-grid\">" + filteredChildren.map(categoryTileHTML).join("") + "</div>" : emptyStateHTML("No matching folders.", "Try a different filter."));
      return;
    }
    renderGuidePage();
    return;
  } else {
    byId("view").innerHTML = categoryGrid() + sectionHeader("Channels") + folderFilterHTML("Filter visible channels") + rowCards(filteredChannels.slice(0, 24));
  }
}

function categoryBrowserHeader(title) {
  return "<div class=\"section-title category-browser-title\"><button class=\"category-browser-back\" data-category=\"\" aria-label=\"Back to Categories\">" + icon("arrow-left") + "</button><span>" + escapeHTML(title) + "</span>" + categoryOptionsMenuHTML() + "</div>";
}
function categoryMenuItemHTML(group, value, label, selected) {
  return "<button type=\"button\" role=\"menuitemradio\" data-category-option-group=\"" + escapeHTML(group) + "\" data-category-option-value=\"" + escapeHTML(value) + "\" aria-checked=\"" + (selected ? "true" : "false") + "\"><span class=\"category-menu-check\">" + (selected ? icon("check") : "") + "</span><span>" + escapeHTML(label) + "</span></button>";
}
function categoryOptionsMenuHTML() {
  const settings = categoryBrowseSettings();
  return "<div class=\"category-options" + (state.categoryMenuOpen ? " open" : "") + "\"><button type=\"button\" class=\"category-options-button\" data-category-options-toggle=\"true\" aria-label=\"Group display options\" aria-haspopup=\"menu\" aria-expanded=\"" + (state.categoryMenuOpen ? "true" : "false") + "\">" + icon("ellipsis") + "</button><div class=\"category-options-menu\" role=\"menu\" aria-label=\"Group display options\">"
    + "<div class=\"category-menu-label\">Sort by</div>"
    + categoryMenuItemHTML("sort", "provider", "Provider order", settings.sort === "provider")
    + categoryMenuItemHTML("sort", "name", "Name", settings.sort === "name")
    + categoryMenuItemHTML("sort", "recent", "Recently watched", settings.sort === "recent")
    + "<div class=\"category-menu-separator\"></div><div class=\"category-menu-label\">Layout</div>"
    + categoryMenuItemHTML("layout", "list", "List", settings.layout === "list")
    + categoryMenuItemHTML("layout", "grid", "Grid", settings.layout === "grid")
    + "<div class=\"category-menu-separator\"></div><div class=\"category-menu-label\">Style</div>"
    + categoryMenuItemHTML("logoStyle", "color", "Colorful", settings.logoStyle === "color")
    + categoryMenuItemHTML("logoStyle", "greyscale", "Greyscale", settings.logoStyle === "greyscale")
    + "<div class=\"category-menu-separator\"></div><div class=\"category-menu-label\">Content</div>"
    + categoryMenuItemHTML("cleanNames", settings.cleanNames ? "false" : "true", "Clean up names", settings.cleanNames)
    + "</div></div>";
}

function renderCategoryBrowserChannels(channels) {
  if (!channels.length) return emptyStateHTML("No channels in this category.", "Try a different filter or refresh the source.");
  const settings = categoryBrowseSettings();
  channels = sortedCategoryChannels(channels);
  const styleClass = settings.logoStyle === "greyscale" ? " category-logos-greyscale" : "";
  if (settings.layout === "list") return "<div class=\"channel-button-list" + styleClass + "\">" + channels.map(function(channel) {
    return "<button class=\"virtual-channel-button\" data-channel=\"" + escapeHTML(channel.id) + "\">" + logoHTML(channel) + "<span><strong>" + escapeHTML(categoryChannelName(channel)) + "</strong><span>" + escapeHTML(channelProgramSubtitle(channel)) + "</span></span></button>";
  }).join("") + "</div>";
  return "<div class=\"category-channel-grid" + styleClass + "\">" + channels.map(function(channel) {
    return "<button class=\"category-channel-card\" data-channel=\"" + escapeHTML(channel.id) + "\">" + logoHTML(channel) + "<strong>" + escapeHTML(categoryChannelName(channel)) + "</strong><span>" + escapeHTML(channelProgramSubtitle(channel)) + "</span></button>";
  }).join("") + "</div>";
}
function recordingCustom(recording) {
  return recording && recording.custom_properties && typeof recording.custom_properties === "object" ? recording.custom_properties : {};
}
function recordingProgram(recording) {
  const custom = recordingCustom(recording);
  return custom.program && typeof custom.program === "object" ? custom.program : {};
}
function recordingStatus(recording) {
  const custom = recordingCustom(recording);
  const now = Date.now();
  const start = Date.parse(recording.start_time || custom.start_time || "");
  const end = Date.parse(recording.end_time || custom.end_time || "");
  if (custom.status) return String(custom.status);
  if (!Number.isNaN(start) && start > now) return "upcoming";
  if (!Number.isNaN(start) && !Number.isNaN(end) && start <= now && end >= now) return "recording";
  return "completed";
}
function recordingTitle(recording) {
  const custom = recordingCustom(recording);
  const program = recordingProgram(recording);
  return custom.title || program.title || custom.file_name || "Untitled recording";
}
function recordingChannelName(recording) {
  const custom = recordingCustom(recording);
  const program = recordingProgram(recording);
  return custom.channel_name || program.channel || program.channel_name || "Dispatcharr";
}
function recordingTimeLabel(value) {
  const date = new Date(value || "");
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleString([], { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });
}
function recordingWindow(recording) {
  const start = recordingTimeLabel(recording.start_time);
  const end = recordingTimeLabel(recording.end_time);
  if (start && end) return start + " - " + end;
  return start || end || "Time unavailable";
}
function normalizeRecordings(payload) {
  if (!payload || !payload.available) return [];
  return items(payload.items).slice().sort(function(a, b) {
    const aTime = Date.parse(a.start_time || "");
    const bTime = Date.parse(b.start_time || "");
    return (Number.isNaN(bTime) ? 0 : bTime) - (Number.isNaN(aTime) ? 0 : aTime);
  });
}
function recordingPlaybackURL(recording) {
  const silo = recording && recording._silo ? recording._silo : {};
  return silo.playback_url || "";
}
function recordingMatchesQuery(recording) {
  if (!state.query) return true;
  const haystack = [recordingTitle(recording), recordingChannelName(recording), recordingStatus(recording)].join(" ").toLowerCase();
  return haystack.indexOf(lower(state.query)) !== -1;
}
function renderRecordingCard(recording) {
  const status = recordingStatus(recording).toLowerCase();
  const playbackURL = recordingPlaybackURL(recording);
  const action = playbackURL ? "<button class=\"recording-action\" data-recording-playback=\"" + escapeHTML(playbackURL) + "\">" + icon("play") + "<span>Playback</span></button>" : "";
  return "<div class=\"recording-card\"><span><strong>" + escapeHTML(recordingTitle(recording)) + "</strong><span class=\"recording-meta\">" + escapeHTML(recordingChannelName(recording) + " - " + recordingWindow(recording)) + "</span></span><div class=\"recording-actions\">" + action + "<span class=\"recording-badge " + escapeHTML(status) + "\">" + escapeHTML(status.split("_").join(" ")) + "</span></div></div>";
}
function renderRecordingSection(title, recordings) {
  if (!recordings.length) return "";
  return sectionHeader(title) + "<div class=\"recording-list\">" + recordings.map(renderRecordingCard).join("") + "</div>";
}
function loadRecordings(force) {
  if (!dvrEnabled()) {
    state.recordings = { available: false, reason: "Recordings require Dispatcharr Direct Connect.", items: [] };
    return;
  }
  if (state.recordingsLoading || (state.recordings && !force)) return;
  state.recordingsLoading = true;
  getJSON("/dispatcharr/api/recordings").then(function(payload) {
    state.recordings = payload;
  }).catch(function(error) {
    state.recordings = { available: false, reason: "Unable to load Dispatcharr recordings.", items: [] };
  }).finally(function() {
    state.recordingsLoading = false;
    if (state.view === "recordings" || state.view === "search" || state.view === "onlater") render();
  });
}
function programByID(channelID, programID) {
  return programsFor(channelID).find(function(program) { return String(program.id || "") === String(programID || ""); }) || null;
}
function scheduleProgram(channelID, programID, button) {
  if (!dvrEnabled()) {
    showAppToast(sourceMode() === "direct_login" ? "Recordings are turned off by the Live TV admin." : "Recordings require Dispatcharr Direct Connect.");
    return;
  }
  if (!recordingSchedulingEnabled()) {
    showAppToast(recordingScheduleReason());
    return;
  }
  const channel = channelByID(channelID);
  const program = programByID(channelID, programID);
  if (!channel || !program) {
    showAppToast("Could not find that guide entry.");
    return;
  }
  if (button) button.disabled = true;
  postJSON("/dispatcharr/api/recordings", {
    channelId: channel.id,
    programId: program.id || "",
    title: program.title || channel.name || "Recording",
    description: program.description || "",
    startUnix: program.startUnix || 0,
    endUnix: program.endUnix || 0
  }).then(function() {
    state.recordings = null;
    loadRecordings(true);
    showAppToast("Recording scheduled in Dispatcharr.");
  }).catch(function(error) {
    const rawMessage = String(error && error.message ? error.message : error || "");
    const lowerMessage = rawMessage.toLowerCase();
    const status = Number(error && error.status || 0);
    if (status === 403
      || lowerMessage.indexOf("admin account or api key") >= 0
      || lowerMessage.indexOf("request failed (403)") >= 0
      || lowerMessage.indexOf("unexpected status 403") >= 0
      || lowerMessage.indexOf("forbidden") >= 0
      || lowerMessage.indexOf("permission") >= 0) {
      state.recordingCapability = { available: true, canSchedule: false, reason: "Scheduling requires a Dispatcharr admin account or Admin API Key." };
      render();
      renderProgramDetailsModal();
      showAppToast(state.recordingCapability.reason);
      return;
    }
    const message = readableError(error);
    if (status === 401 || message.toLowerCase().indexOf("session expired") >= 0) {
      showAppToast(message);
      return;
    }
    showAppToast("Dispatcharr could not schedule that recording.");
  }).finally(function() {
    if (button) button.disabled = false;
  });
}
function programDetailTags(program, channel) {
  const tags = [];
  if (programLooksSports(program)) tags.push("Sports");
  else if (programLooksMovie(program)) tags.push("Movie");
  if (program.rating) tags.push(program.rating);
  if (channel && channel.categoryName) tags.push(categoryDisplayName(channel.categoryName));
  if (programIsLive(program)) tags.push("Live now");
  return uniqueIDs(tags).slice(0, 5);
}
function programDurationLabel(seconds) {
  const minutes = Math.max(1, Math.round(Number(seconds || 0) / 60));
  const hours = Math.floor(minutes / 60);
  const remainder = minutes % 60;
  if (hours && remainder) return hours + " hr " + remainder + " min";
  if (hours) return hours + (hours === 1 ? " hr" : " hrs");
  return minutes + " min";
}
function renderRecordingsPage() {
  const root = byId("view");
  if (!state.recordings) {
    root.innerHTML = sectionHeader("Recordings") + "<div class=\"empty\">Loading Dispatcharr recordings...</div>";
    loadRecordings(false);
    return;
  }
  const toolbar = "<div class=\"recording-toolbar\"><button class=\"recording-refresh\" data-recordings-refresh=\"true\">Refresh recordings</button></div>";
  if (!state.recordings.available) {
    root.innerHTML = toolbar + sectionHeader("Recordings") + "<div class=\"empty\">" + escapeHTML(state.recordings.reason || "Recordings are not available for this connection mode.") + "</div>";
    return;
  }
  const recordings = normalizeRecordings(state.recordings).filter(recordingMatchesQuery);
  const active = recordings.filter(function(recording) { return recordingStatus(recording).toLowerCase() === "recording"; });
  const upcoming = recordings.filter(function(recording) { return recordingStatus(recording).toLowerCase() === "upcoming"; });
  const completed = recordings.filter(function(recording) {
    const status = recordingStatus(recording).toLowerCase();
    return status !== "recording" && status !== "upcoming";
  });
  root.innerHTML = toolbar
    + renderRecordingSection("Recording now", active)
    + renderRecordingSection("Upcoming", upcoming)
    + renderRecordingSection("Completed", completed.slice(0, 80))
    + (!recordings.length ? "<div class=\"empty\">No Dispatcharr recordings found.</div>" : "");
}
function currentProgram(channel) {
  if (!channel) return null;
  const now = Math.floor(Date.now() / 1000);
  return programsFor(channel.id).find(function(program) {
    return (!program.startUnix || program.startUnix <= now + 600) && (!program.endUnix || program.endUnix >= now);
  }) || programsFor(channel.id)[0] || null;
}
function liveProgram(channel) {
  if (!channel) return null;
  const now = Math.floor(Date.now() / 1000);
  return programsFor(channel.id).find(function(program) {
    return (!program.startUnix || program.startUnix <= now) && (!program.endUnix || program.endUnix >= now);
  }) || null;
}
function nextProgram(channel) {
  if (!channel) return null;
  const now = Math.floor(Date.now() / 1000);
  return programsFor(channel.id).find(function(program) {
    return (program.startUnix || 0) > now;
  }) || null;
}
function boundedHLSBufferSeconds(value) {
  const seconds = Number(value) || 12;
  return Math.max(5, Math.min(60, Math.round(seconds)));
}
function liveHLSOptions(value) {
  const bufferSeconds = boundedHLSBufferSeconds(value);
  return {
    liveSyncDuration: bufferSeconds,
    liveMaxLatencyDuration: bufferSeconds + Math.max(10, bufferSeconds),
    maxBufferLength: Math.max(30, bufferSeconds * 2),
    maxMaxBufferLength: Math.max(60, bufferSeconds * 3),
    backBufferLength: 30,
    startFragPrefetch: true,
    maxLiveSyncPlaybackRate: 1.15
  };
}
function overflowTooltip() {
  let tooltip = byId("overflow-tooltip");
  if (tooltip) return tooltip;
  tooltip = document.createElement("div");
  tooltip.id = "overflow-tooltip";
  tooltip.className = "overflow-tooltip";
  tooltip.setAttribute("role", "tooltip");
  document.body.appendChild(tooltip);
  return tooltip;
}
function overflowTooltipTarget(event) {
  if (!event.target || !event.target.closest) return null;
  return event.target.closest("[data-overflow-tooltip], [data-overflow-description]");
}
function descriptionOverflows(target) {
  if (!target) return false;
  if (target.getAttribute("data-tooltip-always") === "true") return true;
  return target.scrollWidth > target.clientWidth + 1 || target.scrollHeight > target.clientHeight + 1;
}
function positionOverflowTooltip(tooltip, target, event) {
  const padding = 12;
  const gap = 8;
  const rect = target.getBoundingClientRect();
  const anchorX = event && typeof event.clientX === "number" ? event.clientX : rect.left + rect.width / 2;
  const width = tooltip.offsetWidth;
  const height = tooltip.offsetHeight;
  const maxLeft = Math.max(padding, window.innerWidth - width - padding);
  const left = Math.min(Math.max(anchorX - width / 2, padding), maxLeft);
  let top = rect.top - height - gap;
  if (top < padding) top = rect.bottom + gap;
  tooltip.style.left = left + "px";
  tooltip.style.top = Math.min(top, Math.max(padding, window.innerHeight - height - padding)) + "px";
}
function showOverflowTooltip(target, event) {
  if (!descriptionOverflows(target)) return;
  const description = target ? String(target.getAttribute("data-overflow-tooltip") || target.textContent || "").trim() : "";
  if (!description) return;
  const tooltip = overflowTooltip();
  tooltip.textContent = description;
  tooltip.classList.add("visible");
  positionOverflowTooltip(tooltip, target, event);
}
function hideOverflowTooltip() {
  const tooltip = byId("overflow-tooltip");
  if (tooltip) tooltip.classList.remove("visible");
}
function epgProgramTitleParts(title) {
  const marker = "\u1d38\u1da6\u1d5b\u1d49";
  const value = String(title || "");
  const trimmed = value.trimEnd();
  if (!trimmed.endsWith(marker)) return { title: value, live: false, marker: "" };
  return { title: trimmed.slice(0, -marker.length).trimEnd(), live: true, marker: marker };
}
function epgVisibleTime(startUnix, windowStart) {
  return timeLabel(Math.max(startUnix || windowStart, windowStart));
}
function categorySettingsBuckets(categories) {
  const buckets = [
    { id: "live", name: "Live, replay & 24/7", description: "Events, replay feeds, specialty channels, and always-on collections.", categories: [] },
    { id: "north_america", name: "United States & Canada", description: "US and Canadian channel groups.", categories: [] },
    { id: "international", name: "International", description: "Regional and country-specific channel groups.", categories: [] }
  ];
  items(categories).forEach(function(category) {
    const name = String(category.name || category.sourceID || "").trim();
    if (/^(USA|CA)\s*\|/i.test(name)) buckets[1].categories.push(category);
    else if (/^(Live\s*\||Replay\s*\||24\/7\b|EN✦|XXX\b)/i.test(name) || /(FIFA World Cup|4K\s*\/\s*UHD|Pay-Per View|Game Pass)/i.test(name)) buckets[0].categories.push(category);
    else buckets[2].categories.push(category);
  });
  return buckets.filter(function(bucket) { return bucket.categories.length > 0; });
}
function renderCategorySettings() {
  const root = byId("settings-list");
  if (!root) return;
  const query = lower(state.categorySettingsQuery);
  const buckets = categorySettingsBuckets(sourceCategoriesWithChannels());
  root.innerHTML = "<label class=\"category-settings-search\">" + icon("search") + "<input id=\"category-settings-filter\" type=\"search\" placeholder=\"Find a channel group\" value=\"" + escapeHTML(state.categorySettingsQuery) + "\" aria-label=\"Find a channel group\"></label>"
    + buckets.map(function(bucket) {
      const visible = bucket.categories.filter(function(category) { return !query || lower(category.name || category.sourceID).indexOf(query) !== -1; });
      if (query && !visible.length) return "";
      const hiddenCount = bucket.categories.filter(function(category) { return !!hiddenMap()[category.sourceID]; }).length;
      const expanded = !!query || !!state.categorySettingsOpen[bucket.id];
      const rows = visible.map(function(category) {
        return "<label class=\"category-settings-row\"><span>" + escapeHTML(category.name || category.sourceID) + "</span><input type=\"checkbox\" data-hide=\"" + escapeHTML(category.sourceID) + "\"" + (hiddenMap()[category.sourceID] ? " checked" : "") + "></label>";
      }).join("");
      return "<section class=\"category-settings-group" + (expanded ? " is-open" : "") + "\"><div class=\"category-settings-group-head\"><button type=\"button\" data-category-section=\"" + bucket.id + "\" aria-expanded=\"" + (expanded ? "true" : "false") + "\"><span class=\"category-settings-chevron\">" + icon("chevron-down") + "</span><span><strong>" + escapeHTML(bucket.name) + "</strong><small>" + escapeHTML(bucket.description) + "</small></span><em>" + hiddenCount + " hidden · " + bucket.categories.length + " total</em></button><label title=\"Hide every group in " + escapeHTML(bucket.name) + "\"><span>Hide all</span><input type=\"checkbox\" data-hide-bucket=\"" + bucket.id + "\"" + (hiddenCount === bucket.categories.length ? " checked" : "") + "></label></div><div class=\"category-settings-rows\">" + rows + "</div></section>";
    }).join("") || "<div class=\"empty\">No matching channel groups.</div>";
}
function renderSettings() {
  ensureSelectedCustomGroup();
  const showSourceCategorySettings = !virtualCategoriesActive();
  byId("view").innerHTML = "<div class=\"settings-stack\">"
    + renderMyProvidersCard()
    + (isDispatcharrDirectSource() ? "<div class=\"settings-card profile-settings-card\"><div class=\"settings-card-head\"><div><h2>Live TV profiles</h2><p>Choose which Dispatcharr profile lineups appear in your Live TV experience.</p></div><span id=\"profile-selection-summary\" class=\"profile-selection-summary\"></span></div><div id=\"profile-settings\"></div></div>" : "")
    + "<div class=\"settings-card custom-groups-card\"><h2>Custom groups</h2><div id=\"custom-group-settings\"></div></div>"
    + "<div class=\"settings-card\"><h2>Category organization</h2><label class=\"settings-row\"><span><strong>Group categories by <code>|</code></strong><small>Turn provider paths such as USA | Sports into browsable folders.</small></span><input type=\"checkbox\" data-category-grouping=\"pipe\"" + (prefs().groupCategoriesByPipe ? " checked" : "") + "></label></div>"
    + (showSourceCategorySettings ? "<div class=\"settings-card category-settings-card\"><div class=\"settings-card-head\"><div><h2>Hidden channel groups</h2><p>Search or expand a section. Checked groups stay out of Home, Favorites, and Guide.</p></div></div><div id=\"settings-list\" class=\"settings-list category-settings-list\"></div></div>" : "")
    + "</div>";
  renderProfileSettings();
  renderCustomGroupSettings();
  if (!showSourceCategorySettings) return;
  renderCategorySettings();
}
function renderProfileSettings() {
  const root = byId("profile-settings");
  if (!root) return;
  const profiles = availableChannelProfiles();
  const selection = profileSelection();
  const allProfiles = selection.mode !== "selected";
  const selected = selectedProfileMap();
  const summary = byId("profile-selection-summary");
  if (summary) summary.textContent = allProfiles ? "All " + profiles.length + " profiles" : selection.profileIds.length + " of " + profiles.length + " profiles";
  if (!profiles.length) {
    root.innerHTML = profileSaveStatusHTML() + "<div class=\"empty\">No Dispatcharr channel profiles are available.</div>";
    return;
  }
  const query = lower(state.profileSettingsQuery);
  const visibleProfiles = profiles.filter(function(profile) {
    return !query || lower([profile.name, profile.id].join(" ")).indexOf(query) !== -1;
  });
  const rows = visibleProfiles.map(function(profile) {
    const checked = allProfiles || !!selected[profile.id];
    const count = Number(profile.channelCount || 0);
    return "<label class=\"profile-selection-row\"><span><strong>" + escapeHTML(profile.name || profile.id) + "</strong><small>" + escapeHTML(count + " channel" + (count === 1 ? "" : "s")) + "</small></span><input type=\"checkbox\" data-profile-selection-id=\"" + escapeHTML(profile.id) + "\"" + (checked ? " checked" : "") + "></label>";
  }).join("");
  root.innerHTML = profileSaveStatusHTML()
    + "<div class=\"profile-settings-toolbar\"><label class=\"profile-settings-search\"><span>" + icon("search") + "</span><input id=\"profile-settings-filter\" placeholder=\"Filter profiles\" value=\"" + escapeHTML(state.profileSettingsQuery) + "\" autocomplete=\"off\"></label><button type=\"button\" data-profile-selection-action=\"all\"" + (allProfiles ? " disabled" : "") + ">Use all profiles</button></div>"
    + "<div class=\"profile-selection-list\">" + (rows || "<div class=\"empty\">No profiles match that filter.</div>") + "</div>";
}
function applyProfileSelection(selection) {
  if (!state.app || !state.app.preferences) return;
  state.app.preferences.profileSelection = normalizeProfileSelection(selection);
  invalidateProfileSelectionCache();
  state.guideChannels = [];
  state.guideWindowStart = -1;
  state.guideWindowEnd = -1;
  state.sportsExpandedEvents = {};
  state.expandedEvents = {};
  if (state.category && !categoryName(state.category)) state.category = "";
  savePrefs();
  render();
}
function useAllProfiles() {
  applyProfileSelection({ mode: "all", profileIds: [] });
}
function updateSelectedProfile(profileID, enabled) {
  profileID = String(profileID || "");
  const profiles = availableChannelProfiles();
  const allIDs = profiles.map(function(profile) { return profile.id; });
  let selectedIDs = profileSelectionIsAll() ? allIDs.slice() : profileSelection().profileIds.slice();
  if (enabled && selectedIDs.indexOf(profileID) === -1) selectedIDs.push(profileID);
  if (!enabled) selectedIDs = selectedIDs.filter(function(id) { return id !== profileID; });
  selectedIDs = uniqueIDs(selectedIDs).filter(function(id) { return allIDs.indexOf(id) !== -1; });
  if (!selectedIDs.length) {
    showAppToast("Select at least one Live TV profile.");
    renderProfileSettings();
    return;
  }
  applyProfileSelection(selectedIDs.length === allIDs.length ? { mode: "all", profileIds: [] } : { mode: "selected", profileIds: selectedIDs });
}
function categoryName(id) {
  if (String(id || "").indexOf("source:") === 0) return sourceCategoryName(String(id || "").slice("source:".length));
  const category = allFilterCategories().find(function(item) { return item.id === id; });
  return category ? category.name : "";
}
function profileSaveStatusHTML() {
  if (!state.profileSaveMessage) return "";
  const warning = state.profileSaveStatus === "error" || state.profileSaveStatus === "local";
  return "<div class=\"settings-note" + (warning ? " settings-warning" : "") + "\">" + escapeHTML(state.profileSaveMessage) + "</div>";
}
function updateCategoryParsingField(field, target) {
  const settings = state.adminCategorySettings || defaultAdminCategorySettings();
  settings[field] = target.type === "checkbox" ? !!target.checked : target.value;
  if (!settings.delimiter) settings.delimiter = "pipe";
  state.adminCategorySettings = settings;
  if (state.category.indexOf("virtual:") === 0 && !categoryName(state.category)) state.category = "";
  normalizeAdminCategorySettings();
  markAdminSettingsDraft();
  renderAdminPage();
}
function ensureSourceCatalogAccount(source) {
  source.accounts = items(source.accounts);
  let catalog = source.accounts.find(function(account) { return account.id === source.catalogAccountId || account.catalog; });
  if (!catalog && source.username) {
    catalog = { id: sourceAccountID(source.username), name: "Catalog", username: source.username, password: source.password || "", passwordConfigured: !!source.passwordConfigured, enabled: true, catalog: true, compatible: true, connectionLimit: 0 };
    source.accounts.unshift(catalog);
    source.catalogAccountId = catalog.id;
  }
  return catalog || null;
}
function byteSizeLabel(value) {
  const bytes = Number(value || 0);
  if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(bytes >= 10737418240 ? 0 : 1) + " GB";
  if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + " MB";
  return Math.max(0, Math.round(bytes / 1024)) + " KB";
}
function renderExternalChannelManager() {
  const managerURL = adminECMURL();
  return "<div class=\"external-manager-surface\"><iframe class=\"external-manager-frame\" src=\"" + escapeHTML(managerURL) + "\" title=\"Channel Manager\"></iframe></div>";
}
function organizationPreviewPath(settings) {
  const channel = effectiveChannels(false)[0] || {};
  const separator = settings.delimiter === "dash" ? " - " : " | ";
  const split = function(value, fallback) {
    const parts = String(value || fallback || "").split(separator).map(function(part) { return part.trim(); }).filter(Boolean);
    return parts.length ? parts : [fallback || "Unassigned"];
  };
  const profile = profilePathsForChannel(channel)[0] || "Profile";
  const group = sourceCategoryOriginalLabel(channel) || sourceCategoryLabel(channel) || "Group";
  const channelPath = channel.name || "Channel";
  const source = normalizeVirtualGroupSource(settings.virtualGroupSource, settings.inferChannelNameGroups === true);
  const stages = [];
  if (source === "profile_group") stages.push({ label: "Profile path", value: split(profile, "Profile").join(" / ") });
  if (source === "group" || source === "group_channel" || source === "profile_group") stages.push({ label: "Group path", value: split(group, "Group").join(" / ") });
  if (source === "channel" || source === "group_channel") stages.push({ label: "Channel path", value: split(channelPath, "Channel").join(" / ") });
  return stages;
}
function renderOrganizationPreview(settings) {
  const stages = organizationPreviewPath(settings);
  const finalPath = stages.map(function(stage) { return stage.value; }).join(" / ") || organizationRootLabel();
  const inputs = stages.map(function(stage) {
    return '<div class="organization-stage"><strong>' + escapeHTML(stage.label) + "</strong><span>" + escapeHTML(stage.value) + "</span></div>";
  }).join('<span class="organization-arrow" aria-hidden="true">→</span>');
  return '<div id="organization-preview" class="organization-preview" aria-live="polite">' + inputs + (inputs ? '<span class="organization-arrow" aria-hidden="true">→</span>' : "") + '<div class="organization-result"><strong>Browse path</strong><span>' + escapeHTML(finalPath) + "</span></div></div>";
}
function ensureSelectedCustomGroup() {
  const groups = customGroups();
  if (groups.some(function(group) { return group.id === state.selectedCustomGroup; })) return;
  state.selectedCustomGroup = groups.length ? groups[0].id : "";
}
function selectedCustomGroup() {
  ensureSelectedCustomGroup();
  return customGroups().find(function(group) { return group.id === state.selectedCustomGroup; }) || null;
}
function renderCustomGroupSettings() {
  const root = byId("custom-group-settings");
  if (!root) return;
  const groups = customGroups();
  const selected = selectedCustomGroup();
  const memberships = selected ? customMemberships(selected.id) : [];
  const query = lower(state.customGroupQuery);
  const availableChannels = effectiveChannels(false).filter(function(channel) {
    if (selected && memberships.indexOf(channel.id) !== -1) return false;
    if (!query) return true;
    return lower(channel.name || channel.id).indexOf(query) !== -1 || lower(sourceCategoryLabel(channel)).indexOf(query) !== -1;
  });
  if (!availableChannels.some(function(channel) { return channel.id === state.customGroupChannelID; })) state.customGroupChannelID = availableChannels.length ? availableChannels[0].id : "";
  const pickerChannels = availableChannels.slice(0, 24);
  const createControl = "<div class=\"custom-group-control\"><label for=\"custom-group-name\">New group</label><div class=\"custom-group-field\"><input id=\"custom-group-name\" placeholder=\"Spanish\"><button data-custom-group-action=\"create\">Create</button></div></div>";
  const manageControl = groups.length
    ? "<div class=\"custom-group-control\"><label for=\"custom-group-select\">Edit group</label><div class=\"custom-group-field\"><select id=\"custom-group-select\">" + groups.map(function(group) { return "<option value=\"" + escapeHTML(group.id) + "\"" + (selected && selected.id === group.id ? " selected" : "") + ">" + escapeHTML(group.name) + "</option>"; }).join("") + "</select><button data-custom-group-action=\"delete\">Delete</button></div></div>"
    : "";
  const searchStatus = availableChannels.length
    ? "Showing " + pickerChannels.length + " of " + availableChannels.length + " matching channels."
    : "No matching channels.";
  const resultRows = pickerChannels.length ? pickerChannels.map(function(channel) {
    const active = channel.id === state.customGroupChannelID;
    return "<div class=\"custom-channel-result" + (active ? " selected" : "") + "\">"
      + "<button class=\"custom-channel-option\" type=\"button\" role=\"option\" aria-selected=\"" + (active ? "true" : "false") + "\" data-custom-group-channel-option=\"" + escapeHTML(channel.id) + "\"><strong>" + escapeHTML(channel.name || channel.id) + "</strong><small>" + escapeHTML(sourceCategoryLabel(channel) || "Live TV") + "</small></button>"
      + "<button class=\"custom-channel-add\" type=\"button\" data-custom-group-add-channel=\"" + escapeHTML(channel.id) + "\">Add</button>"
      + "</div>";
  }).join("") : "<div class=\"custom-group-empty\">No channels match that search.</div>";
  const memberRows = memberships.length ? memberships.map(function(id) {
    const channel = channelByID(id);
    return "<div class=\"custom-member-row\"><div><strong>" + escapeHTML((channel && channel.name) || id) + "</strong><small>" + escapeHTML((channel && sourceCategoryLabel(channel)) || "Missing from current lineup") + "</small></div><button data-custom-group-action=\"remove-channel\" data-channel-id=\"" + escapeHTML(id) + "\">Remove</button></div>";
  }).join("") : "<div class=\"custom-group-empty\">No channels in this group yet.</div>";
  root.innerHTML = "<div class=\"custom-groups-panel\">"
    + "<div class=\"custom-groups-controls\">" + createControl + manageControl + "</div>"
    + (selected ? "<div class=\"custom-group-meta\"><strong>" + escapeHTML(selected.name) + "</strong><span>" + memberships.length + " channels</span></div>"
      + "<div class=\"custom-group-workspace\">"
      + "<section class=\"custom-group-browser\"><div class=\"custom-group-section-head\"><strong>Add channels</strong><span>" + escapeHTML(searchStatus) + "</span></div><input class=\"custom-channel-search\" id=\"custom-group-channel-search\" role=\"combobox\" aria-controls=\"custom-group-channel-options\" aria-expanded=\"true\" aria-autocomplete=\"list\" placeholder=\"Search channels or groups\" value=\"" + escapeHTML(state.customGroupQuery) + "\"><div id=\"custom-group-channel-options\" class=\"custom-channel-options\" role=\"listbox\">" + resultRows + "</div></section>"
      + "<section class=\"custom-group-members\"><div class=\"custom-group-section-head\"><strong>Group channels</strong><span>" + memberships.length + " saved</span></div><div class=\"custom-member-list\">" + memberRows + "</div></section>"
      + "</div>"
      : "<div class=\"custom-group-empty\"><strong>Create a group</strong><span>Build a personal channel lineup by adding channels from the current Live TV catalog.</span></div>")
    + "</div>";
}
function selectCustomGroupChannel(channelID) {
  state.customGroupChannelID = channelID || "";
  renderSettings();
}
function slug(value) {
  return String(value || "").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 48) || "group";
}
function createCustomGroup(name) {
  name = String(name || "").trim();
  if (!name) return;
  const base = "group:" + slug(name);
  let id = base;
  let index = 2;
  while (customGroups().some(function(group) { return group.id === id; })) id = base + "-" + index++;
  state.app.preferences.customGroups.push({ id: id, name: name, order: customGroups().length + 1 });
  state.app.preferences.customGroupMemberships[id] = [];
  state.selectedCustomGroup = id;
  savePrefs();
  render();
}
function deleteSelectedCustomGroup() {
  const selected = selectedCustomGroup();
  if (!selected) return;
  state.app.preferences.customGroups = customGroups().filter(function(group) { return group.id !== selected.id; });
  delete state.app.preferences.customGroupMemberships[selected.id];
  if (state.category === customCategoryID(selected.id)) state.category = "";
  state.selectedCustomGroup = "";
  savePrefs();
  render();
}
function addChannelToSelectedGroup(channelID) {
  const selected = selectedCustomGroup();
  if (!selected || !channelID) return;
  state.app.preferences.customGroupMemberships[selected.id] = uniqueIDs(customMemberships(selected.id).concat([channelID]));
  savePrefs();
  render();
}
function removeChannelFromSelectedGroup(channelID) {
  const selected = selectedCustomGroup();
  if (!selected || !channelID) return;
  state.app.preferences.customGroupMemberships[selected.id] = customMemberships(selected.id).filter(function(id) { return id !== channelID; });
  savePrefs();
  render();
}
function textTrackList() {
  const video = byId("player");
  if (!video || !video.textTracks || typeof video.textTracks.length !== "number") return [];
  const tracks = [];
  for (let index = 0; index < video.textTracks.length; index++) {
    const track = video.textTracks[index];
    if (!track || (track.kind && ["subtitles", "captions"].indexOf(track.kind) === -1)) continue;
    tracks.push(track);
  }
  return tracks;
}
function textTrackName(track, index) {
  return track && (track.label || track.language || track.kind || track.id) ? (track.label || track.language || track.kind || track.id) : "Subtitles " + (index + 1);
}
function updateAudioMenu() {
  const button = byId("player-audio-button");
  const languageButton = byId("player-language-button");
  const menu = byId("player-audio-menu");
  if (!button || !menu) return;
  const tracks = audioTrackList();
  const activeIndex = tracks.findIndex(function(track) { return !!track.enabled; });
  state.selectedAudioTrack = activeIndex >= 0 ? activeIndex : state.selectedAudioTrack;
  const activeLabel = tracks.length ? audioTrackName(tracks[state.selectedAudioTrack] || tracks[0], state.selectedAudioTrack || 0) : "Default audio";
  button.innerHTML = icon("language") + "<span>" + escapeHTML(activeLabel) + "</span>" + icon("chevron-down");
  button.setAttribute("aria-expanded", state.audioMenuOpen ? "true" : "false");
  if (languageButton) {
    languageButton.classList.toggle("active", state.audioMenuOpen && tracks.length > 1);
    languageButton.setAttribute("aria-expanded", state.audioMenuOpen && tracks.length > 1 ? "true" : "false");
    languageButton.setAttribute("aria-label", tracks.length > 1 ? "Audio language: " + activeLabel : "Audio language");
  }
  menu.classList.toggle("open", state.audioMenuOpen);
  updatePlayerChrome();
  menu.innerHTML = tracks.length ? tracks.map(function(track, index) {
    return "<button type=\"button\" role=\"menuitem\" data-player-action=\"audio-track\" data-audio-index=\"" + index + "\" class=\"" + (index === state.selectedAudioTrack ? "active" : "") + "\">" + escapeHTML(audioTrackName(track, index)) + "</button>";
  }).join("") : "<button type=\"button\" role=\"menuitem\" class=\"active\" data-player-action=\"audio-track\" data-audio-index=\"0\">Default audio</button>";
}
function toggleLanguageMenu() {
  const tracks = audioTrackList();
  if (tracks.length <= 1) {
    closePlayerPopovers();
    showPlayerToast("No alternate audio languages are available for this stream.");
    return;
  }
  state.audioMenuOpen = !state.audioMenuOpen;
  closePlayerPopovers("audio");
  updateAudioMenu();
}
function showAppToast(message) {
  let toast = byId("app-toast");
  if (!toast) {
    toast = document.createElement("div");
    toast.id = "app-toast";
    toast.className = "app-toast";
    toast.setAttribute("role", "status");
    document.body.appendChild(toast);
  }
  toast.textContent = message;
  toast.classList.add("show");
  clearTimeout(state.appToastTimer);
  state.appToastTimer = setTimeout(function() { toast.classList.remove("show"); }, 2600);
}
async function openCastPicker() {
  const video = byId("player");
  if (!video) return;
  closePlayerPopovers();
  try {
    if (typeof video.webkitShowPlaybackTargetPicker === "function") {
      video.webkitShowPlaybackTargetPicker();
      return;
    }
    if (video.remote && typeof video.remote.prompt === "function") {
      await video.remote.prompt();
      return;
    }
    showPlayerToast("AirPlay or Cast is not available in this browser.");
  } catch (error) {
    showPlayerToast("No playback target selected.");
  }
}
async function togglePictureInPicture() {
  const video = byId("player");
  if (!video) return;
  closePlayerPopovers();
  if (!document.pictureInPictureEnabled || typeof video.requestPictureInPicture !== "function") {
    showPlayerToast("Picture in Picture is not available in this browser.");
    return;
  }
  try {
    if (document.pictureInPictureElement) await document.exitPictureInPicture();
    else await video.requestPictureInPicture();
  } catch (error) {
    showPlayerToast("Picture in Picture could not be opened.");
  }
}
function updateCenterPlayButton() {
  const video = byId("player");
  const button = byId("player-center-button");
  if (!video) return;
  const paused = video.paused || video.ended;
  if (paused && state.timeShiftSession) state.timeShiftSession.rewound = true;
  const transport = byId("player-timeshift-play");
  if (transport) {
    transport.innerHTML = icon(paused ? "play" : "pause");
    transport.setAttribute("aria-label", paused ? "Play" : "Pause");
  }
  updateTimeShiftUI();
  if (!button) return;
  const loading = !!state.playerWaiting && !video.paused;
  const show = loading || video.paused;
  button.classList.toggle("hidden", !show);
  button.classList.toggle("loading", loading);
  button.innerHTML = loading ? icon("loader") : icon(video.paused ? "play" : "pause");
  button.setAttribute("aria-label", loading ? "Loading stream" : (video.paused ? "Play" : "Pause"));
  button.disabled = loading;
}
function togglePlayPause() {
  const video = byId("player");
  if (!video) return;
  closePlayerPopovers();
  if (video.paused) video.play().catch(function() { showPlayerToast("Playback could not be started."); });
  else video.pause();
  updateCenterPlayButton();
}
async function playChannel(channel) {
  if (state.view !== "player") {
    const main = document.querySelector(".main");
    const guideScroll = byId("guide-scroll");
    state.playerReturnContext = {
      view: state.view,
      category: state.category,
      query: state.query,
      folderQuery: state.folderQuery,
      scrollY: window.scrollY,
      mainScrollTop: main ? main.scrollTop : 0,
      guideScrollTop: guideScroll ? guideScroll.scrollTop : 0,
      guideScrollLeft: guideScroll ? guideScroll.scrollLeft : 0
    };
  }
  stopTimeShiftSession();
  const timeShiftAttempt = state.timeShiftAttempt;
  state.currentChannel = channel;
  state.view = "player";
  render();
  try {
    await ensurePlayerLibraries(liveRewindEnabled() && channel.streamFormat !== "hls" ? "hls" : channel.streamFormat);
  } catch (_) {
    showPlayerToast("Playback components could not be loaded.");
    return;
  }
  if (timeShiftAttempt !== state.timeShiftAttempt || !state.currentChannel || state.currentChannel.id !== channel.id) return;
  let watchSession = null;
  try {
    watchSession = await startWatch(channel);
  } catch (error) {
    if (error && error.superseded) return;
    showPlayerToast("Could not reserve a provider connection: " + readableError(error));
    return;
  }
  const streamURL = browserStreamURL(channel, watchSession && watchSession.id);
  if (await showStreamBlockedIfNeeded(streamURL, channel.streamFormat)) return;
  if (timeShiftAttempt !== state.timeShiftAttempt || !state.currentChannel || state.currentChannel.id !== channel.id) return;
  const managedRewind = liveRewindEnabled() && !isRewindableChannel(channel);
  setVideoSource(streamURL + (managedRewind ? "&media_mode=rewind" : ""), { rewindable: isRewindableChannel(channel), format: channel.streamFormat, hlsBufferSeconds: channel.hlsBufferSeconds });

  if (timeShiftAttempt !== state.timeShiftAttempt || !state.currentChannel || state.currentChannel.id !== channel.id) return;
  const guide = await getJSON("/dispatcharr/api/guide?channel_id=" + encodeURIComponent(channel.id)).catch(function() { return { programs: [] }; });
  if (!state.currentChannel || state.currentChannel.id !== channel.id) return;
  const nowGuide = byId("now-guide");
  if (nowGuide) nowGuide.innerHTML = items(guide.programs).slice(0, 6).map(function(program) { return "<div class=\"program\"><time>" + escapeHTML(timeLabel(program.startUnix)) + "</time><strong>" + escapeHTML(program.title || "Untitled") + "</strong></div>"; }).join("") || "<div class=\"empty\">No guide entries.</div>";
}
document.addEventListener("click", function(event) {
  const guideCategory = event.target.closest("[data-guide-category]");
  if (guideCategory) {
    event.preventDefault();
    const categoryID = guideCategory.getAttribute("data-guide-category") || "";
    state.guideCategoryPickerOpen = false;
    state.guideCategoryQuery = "";
    navigateGuideCategory(categoryID);
    return;
  }
  const guideCategoryToggle = event.target.closest("[data-guide-category-toggle]");
  if (guideCategoryToggle) {
    event.preventDefault();
    state.guideCategoryPickerOpen = !state.guideCategoryPickerOpen;
    renderGuidePage();
    if (state.guideCategoryPickerOpen) {
      const input = byId("guide-category-search");
      if (input) input.focus();
    }
    return;
  }
  if (state.guideCategoryPickerOpen && !event.target.closest(".guide-category-picker")) {
    state.guideCategoryPickerOpen = false;
    const picker = document.querySelector(".guide-category-picker");
    if (picker) picker.classList.remove("open");
  }
  const categoryOption = event.target.closest("[data-category-option-group]");
  if (categoryOption) {
    event.preventDefault();
    event.stopPropagation();
    const group = categoryOption.getAttribute("data-category-option-group");
    let value = categoryOption.getAttribute("data-category-option-value");
    if (group === "cleanNames") value = value === "true";
    updateCategoryBrowseSetting(group, value);
    return;
  }
  const categoryOptionsToggle = event.target.closest("[data-category-options-toggle]");
  if (categoryOptionsToggle) {
    event.preventDefault();
    event.stopPropagation();
    state.categoryMenuOpen = !state.categoryMenuOpen;
    const root = categoryOptionsToggle.closest(".category-options");
    if (root) root.classList.toggle("open", state.categoryMenuOpen);
    categoryOptionsToggle.setAttribute("aria-expanded", state.categoryMenuOpen ? "true" : "false");
    return;
  }
  if (state.categoryMenuOpen && !event.target.closest(".category-options")) {
    state.categoryMenuOpen = false;
    const categoryOptions = document.querySelector(".category-options");
    if (categoryOptions) categoryOptions.classList.remove("open");
    const categoryOptionsButton = document.querySelector("[data-category-options-toggle]");
    if (categoryOptionsButton) categoryOptionsButton.setAttribute("aria-expanded", "false");
  }
  const timeShiftAdminAction = event.target.closest("[data-timeshift-admin-action]");
  if (timeShiftAdminAction) {
    const action = timeShiftAdminAction.getAttribute("data-timeshift-admin-action");
    if (action === "refresh") refreshAdminTimeShiftStatus(false);
    if (action === "clear") clearAdminTimeShiftCache();
    return;
  }
  const settingsMenuButton = event.target.closest("#settings-menu-button");
  if (settingsMenuButton) {
    event.preventDefault();
    setSettingsMenuOpen(!settingsMenuOpen());
    return;
  }
  if (!event.target.closest(".settings-menu")) setSettingsMenuOpen(false);
  const profileSelectionAction = event.target.closest("[data-profile-selection-action]");
  if (profileSelectionAction) {
    event.preventDefault();
    if (profileSelectionAction.getAttribute("data-profile-selection-action") === "all") useAllProfiles();
    return;
  }
  const categorySection = event.target.closest("[data-category-section]");
  if (categorySection) {
    event.preventDefault();
    const sectionID = categorySection.getAttribute("data-category-section");
    state.categorySettingsOpen[sectionID] = !state.categorySettingsOpen[sectionID];
    renderCategorySettings();
    return;
  }
  const playerSportsChannel = event.target.closest("[data-player-sports-channel]");
  if (playerSportsChannel) {
    event.preventDefault();
    const channel = channelByID(playerSportsChannel.getAttribute("data-player-sports-channel"));
    if (channel) {
      state.playerSportsOpen = false;
      stopPlayerSportsRefresh();
      playChannel(channel);
    }
    return;
  }
  const playerTarget = event.target.closest("[data-player-action]");
  if (playerTarget) {
    event.preventDefault();
    handlePlayerAction(playerTarget.getAttribute("data-player-action"), playerTarget);
    return;
  }
  const recordingsRefresh = event.target.closest("[data-recordings-refresh]");
  if (recordingsRefresh) {
    event.preventDefault();
    state.recordings = null;
    loadRecordings(true);
    renderRecordingsPage();
    return;
  }
  const guideRefresh = event.target.closest("[data-guide-refresh]");
  if (guideRefresh) {
    event.preventDefault();
    setSettingsMenuOpen(false);
    if (state.view === "sports") {
      const buttons = Array.prototype.slice.call(document.querySelectorAll("[data-guide-refresh]"));
      buttons.forEach(function(button) {
        button.classList.add("is-loading");
        button.disabled = true;
      });
      loadSports(true).finally(function() {
        buttons.forEach(function(button) {
          button.classList.remove("is-loading");
          button.disabled = false;
        });
      });
      renderSportsPage();
      return;
    }
    if (state.view === "events") {
      const buttons = Array.prototype.slice.call(document.querySelectorAll("[data-guide-refresh]"));
      buttons.forEach(function(button) {
        button.classList.add("is-loading");
        button.disabled = true;
      });
      loadEvents(true).finally(function() {
        buttons.forEach(function(button) {
          button.classList.remove("is-loading");
          button.disabled = false;
        });
      });
      renderEventsPage();
      return;
    }
    refreshGuideBlockData();
    return;
  }
  const programDetailClose = event.target.closest("[data-program-modal-close]");
  if (programDetailClose) {
    event.preventDefault();
    closeProgramDetails();
    return;
  }
  const programDetailWatch = event.target.closest("[data-program-detail-watch]");
  if (programDetailWatch) {
    event.preventDefault();
    const channel = channelByID(programDetailWatch.getAttribute("data-program-detail-watch"));
    closeProgramDetails();
    if (channel) playChannel(channel);
    return;
  }
  const catchupTarget = event.target.closest("[data-catchup-channel]");
  if (catchupTarget) {
    event.preventDefault();
    const channelID = catchupTarget.getAttribute("data-catchup-channel");
    const start = catchupTarget.getAttribute("data-catchup-start");
    const end = catchupTarget.getAttribute("data-catchup-end");
    const channel = channelByID(channelID);
    closeProgramDetails();
    if (channel) playOnDemand(channel.name || "Replay", "/dispatcharr/catchup/stream?channel_id=" + encodeURIComponent(channelID || "") + "&start_unix=" + encodeURIComponent(start || "") + "&end_unix=" + encodeURIComponent(end || ""));
    return;
  }
  const programDetailSchedule = event.target.closest("[data-program-detail-schedule]");
  if (programDetailSchedule) {
    event.preventDefault();
    scheduleProgram(programDetailSchedule.getAttribute("data-program-detail-schedule"), programDetailSchedule.getAttribute("data-program-detail-program"), programDetailSchedule);
    return;
  }
  const programDetailTarget = event.target.closest("[data-program-detail-channel]");
  if (programDetailTarget) {
    event.preventDefault();
    openProgramDetails(programDetailTarget.getAttribute("data-program-detail-channel"), programDetailTarget.getAttribute("data-program-detail"));
    return;
  }
  const searchCancel = event.target.closest("[data-search-cancel]");
  if (searchCancel) {
    event.preventDefault();
    setView(state.searchReturnView || "home");
    return;
  }
  const searchClear = event.target.closest("[data-search-clear]");
  if (searchClear) {
    event.preventDefault();
    clearRecentSearches();
    renderSearchPage();
    return;
  }
  const searchQueryClear = event.target.closest("[data-search-query-clear]");
  if (searchQueryClear) {
    event.preventDefault();
    state.searchQuery = "";
    renderSearchPage();
    return;
  }
  const searchRecent = event.target.closest("[data-search-recent]");
  if (searchRecent) {
    event.preventDefault();
    state.searchQuery = searchRecent.getAttribute("data-search-recent") || "";
    rememberSearch(state.searchQuery);
    renderSearchPage();
    return;
  }
  const searchType = event.target.closest("[data-search-type]");
  if (searchType) {
    event.preventDefault();
    state.searchType = searchType.getAttribute("data-search-type") || "all";
    renderSearchPage();
    return;
  }
  const searchChannel = event.target.closest("[data-search-channel]");
  if (searchChannel) {
    event.preventDefault();
    rememberSearch(state.searchQuery);
    const channel = channelByID(searchChannel.getAttribute("data-search-channel"));
    if (channel) playChannel(channel);
    return;
  }
  const searchCategory = event.target.closest("[data-search-category]");
  if (searchCategory) {
    event.preventDefault();
    rememberSearch(state.searchQuery);
    setCategory(searchCategory.getAttribute("data-search-category"));
    return;
  }
  const searchProgram = event.target.closest("[data-search-program-channel]");
  if (searchProgram) {
    event.preventDefault();
    rememberSearch(state.searchQuery);
    const channelID = searchProgram.getAttribute("data-search-program-channel");
    const programID = searchProgram.getAttribute("data-search-program");
    openProgramDetails(channelID, programID);
    return;
  }
  const searchAiring = event.target.closest("[data-search-airing]");
  if (searchAiring) {
    event.preventDefault();
    rememberSearch(state.searchQuery);
    closeProgramDetails();
    state.searchQuery = searchAiring.getAttribute("data-search-airing") || state.searchQuery;
    state.searchType = "programs";
    setView("search");
    return;
  }
  const keywordPassAdd = event.target.closest("[data-keyword-pass-add]");
  if (keywordPassAdd) {
    event.preventDefault();
    addKeywordPass(keywordPassAdd.getAttribute("data-keyword-pass-add"));
    return;
  }
  const keywordPassRemove = event.target.closest("[data-keyword-pass-remove]");
  if (keywordPassRemove) {
    event.preventDefault();
    removeKeywordPass(keywordPassRemove.getAttribute("data-keyword-pass-remove"));
    return;
  }
  const onLaterType = event.target.closest("[data-onlater-type]");
  if (onLaterType) {
    event.preventDefault();
    state.onLaterType = onLaterType.getAttribute("data-onlater-type") || "all";
    renderOnLaterPage();
    return;
  }
  const onLaterTime = event.target.closest("[data-onlater-time]");
  if (onLaterTime) {
    event.preventDefault();
    state.onLaterTime = onLaterTime.getAttribute("data-onlater-time") || "all";
    renderOnLaterPage();
    return;
  }
  const sportsTab = event.target.closest("[data-sports-tab]");
  if (sportsTab) {
    event.preventDefault();
    setSportsTab(sportsTab.getAttribute("data-sports-tab"));
    return;
  }
  const sportsLeague = event.target.closest("[data-sports-league]");
  if (sportsLeague) {
    event.preventDefault();
    setSportsLeague(sportsLeague.getAttribute("data-sports-league"));
    return;
  }
  const sportsRefresh = event.target.closest("[data-sports-refresh]");
  if (sportsRefresh) {
    event.preventDefault();
    loadSports(true);
    renderSportsPage();
    return;
  }
  const recoveryRetry = event.target.closest("[data-recovery-retry]");
  if (recoveryRetry) {
    event.preventDefault();
    const kind = recoveryRetry.getAttribute("data-recovery-retry");
    if (kind === "sports") loadSports(true);
    else loadEvents(true);
    return;
  }
  const recoveryReload = event.target.closest("[data-recovery-reload]");
  if (recoveryReload) {
    event.preventDefault();
    window.location.reload();
    return;
  }
  const sportsExpand = event.target.closest("[data-sports-expand-event]");
  if (sportsExpand) {
    event.preventDefault();
    toggleSportsEventChannels(sportsExpand.getAttribute("data-sports-expand-event"));
    return;
  }
  const sportsFavorite = event.target.closest("[data-sports-favorite-team]");
  if (sportsFavorite) {
    event.preventDefault();
    toggleSportsTeamFavorite(sportsFavorite.getAttribute("data-sports-favorite-team"), sportsFavorite.getAttribute("data-sports-favorite-enabled") === "true");
    return;
  }
  const eventTab = event.target.closest("[data-event-tab]");
  if (eventTab) {
    event.preventDefault();
    setEventTab(eventTab.getAttribute("data-event-tab"));
    return;
  }
  const eventCategory = event.target.closest("[data-event-category]");
  if (eventCategory) {
    event.preventDefault();
    setEventCategory(eventCategory.getAttribute("data-event-category"));
    return;
  }
  const eventExpand = event.target.closest("[data-event-expand]");
  if (eventExpand) {
    event.preventDefault();
    toggleBroadcastEventChannels(eventExpand.getAttribute("data-event-expand"));
    return;
  }
  const recordingPlayback = event.target.closest("[data-recording-playback]");
  if (recordingPlayback) {
    event.preventDefault();
    const url = recordingPlayback.getAttribute("data-recording-playback");
    if (url) window.open(url, "_blank", "noopener");
    return;
  }
  const vodPlayback = event.target.closest("[data-vod-playback]");
  if (vodPlayback) {
    event.preventDefault();
    playOnDemand(vodPlayback.textContent || "On Demand", "/dispatcharr/vod/stream?item_id=" + encodeURIComponent(vodPlayback.getAttribute("data-vod-playback")));
    return;
  }
  const seriesOpen = event.target.closest("[data-series-open]");
  if (seriesOpen) {
    event.preventDefault();
    openSeriesDetail(seriesOpen.getAttribute("data-series-open"));
    return;
  }
  const seriesBack = event.target.closest("[data-series-back]");
  if (seriesBack) {
    event.preventDefault();
    state.view = "search";
    render();
    return;
  }
  const episodePlayback = event.target.closest("[data-episode-playback]");
  if (episodePlayback) {
    event.preventDefault();
    const detail = state.seriesDetail || {};
    const episodeID = episodePlayback.getAttribute("data-episode-playback");
    playOnDemand(episodePlayback.textContent || "Episode", "/dispatcharr/episode/stream?series_id=" + encodeURIComponent(detail.seriesID || "") + "&episode_id=" + encodeURIComponent(episodeID || ""));
    return;
  }
  const scheduleTarget = event.target.closest("[data-schedule-channel]");
  if (scheduleTarget) {
    event.preventDefault();
    event.stopPropagation();
    scheduleProgram(scheduleTarget.getAttribute("data-schedule-channel"), scheduleTarget.getAttribute("data-schedule-program"), scheduleTarget);
    return;
  }
  const customGroupAction = event.target.closest("[data-custom-group-action]");
  if (customGroupAction) {
    event.preventDefault();
    const action = customGroupAction.getAttribute("data-custom-group-action");
    if (action === "create") createCustomGroup((byId("custom-group-name") || {}).value || "");
    if (action === "delete") deleteSelectedCustomGroup();
    if (action === "add-channel") addChannelToSelectedGroup(state.customGroupChannelID || "");
    if (action === "remove-channel") removeChannelFromSelectedGroup(customGroupAction.getAttribute("data-channel-id"));
    return;
  }
  const customGroupAddChannel = event.target.closest("[data-custom-group-add-channel]");
  if (customGroupAddChannel) {
    event.preventDefault();
    addChannelToSelectedGroup(customGroupAddChannel.getAttribute("data-custom-group-add-channel"));
    return;
  }
  const customGroupChannelOption = event.target.closest("[data-custom-group-channel-option]");
  if (customGroupChannelOption) {
    event.preventDefault();
    selectCustomGroupChannel(customGroupChannelOption.getAttribute("data-custom-group-channel-option"));
    return;
  }
  const adminAliasAction = event.target.closest("[data-admin-alias-action]");
  if (adminAliasAction) {
    event.preventDefault();
    const action = adminAliasAction.getAttribute("data-admin-alias-action");
    if (action === "add") addAdminCategoryAlias();
    if (action === "remove") removeAdminCategoryAlias(Number(adminAliasAction.getAttribute("data-admin-alias-index")));
    return;
  }
  const adminSettingsAction = event.target.closest("[data-admin-settings-action]");
  if (adminSettingsAction) {
    event.preventDefault();
    const action = adminSettingsAction.getAttribute("data-admin-settings-action");
    if (action === "save") saveAdminCategorySettings();
    if (action === "discard") discardAdminCategorySettings();
    return;
  }
  const sourceAction = event.target.closest("[data-source-action]");
  if (sourceAction) {
    event.preventDefault();
    handleAdminSourceAction(sourceAction.getAttribute("data-source-action"), sourceAction.getAttribute("data-source-id") || "");
    return;
  }
  const accountAction = event.target.closest("[data-account-action]");
  if (accountAction) {
    event.preventDefault();
    handleSourceAccountAction(accountAction.getAttribute("data-account-action"), Number(accountAction.getAttribute("data-account-index")));
    return;
  }
  const sourceStep = event.target.closest("[data-source-step]");
  if (sourceStep) {
    event.preventDefault();
    state.adminSourceEditorStep = sourceStep.getAttribute("data-source-step") || "general";
    rerenderSourceManager();
    return;
  }
  const sourceFormat = event.target.closest("[data-source-format]");
  if (sourceFormat && state.adminSourceEditor) {
    event.preventDefault();
    state.adminSourceEditor.liveFormat = sourceFormat.getAttribute("data-source-format") === "ts" ? "ts" : "m3u8";
    rerenderSourceManager();
    return;
  }
  const adminTab = event.target.closest("[data-admin-tab]");
  if (adminTab) {
    event.preventDefault();
    setAdminTab(adminTab.getAttribute("data-admin-tab"));
    return;
  }
  const virtualCategoryViewTarget = event.target.closest("[data-virtual-category-view]");
  if (virtualCategoryViewTarget) {
    event.preventDefault();
    setVirtualCategoryView(virtualCategoryViewTarget.getAttribute("data-virtual-category-view"));
    return;
  }
  const categoryBrowseViewTarget = event.target.closest("[data-category-browse-view]");
  if (categoryBrowseViewTarget) {
    state.categoryBrowseView = categoryBrowseViewTarget.getAttribute("data-category-browse-view") === "list" ? "list" : "grid";
    updateCategoryBrowseSetting("layout", state.categoryBrowseView);
    return;
  }
  const favoritesAction = event.target.closest("[data-favorites-action]");
  if (favoritesAction) {
    event.preventDefault();
    setView(favoritesAction.getAttribute("data-favorites-action") || "guide");
    return;
  }
  const favoriteMove = event.target.closest("[data-favorite-move]");
  if (favoriteMove) {
    event.preventDefault();
    moveFavorite(favoriteMove.getAttribute("data-channel-id"), favoriteMove.getAttribute("data-favorite-move"));
    return;
  }
  const favoriteRemove = event.target.closest("[data-favorite-remove]");
  if (favoriteRemove) {
    event.preventDefault();
    setChannelFavorite(favoriteRemove.getAttribute("data-channel-id"), false);
    render();
    return;
  }
  const multiviewAction = event.target.closest("[data-multiview-action]");
  if (multiviewAction) {
    event.preventDefault();
    event.stopPropagation();
    handleMultiviewAction(multiviewAction.getAttribute("data-multiview-action"), multiviewAction.getAttribute("data-multiview-tile-id"));
    return;
  }
  const multiviewChannel = event.target.closest("[data-multiview-channel]");
  if (multiviewChannel) {
    event.preventDefault();
    event.stopPropagation();
    const channel = channelByID(multiviewChannel.getAttribute("data-multiview-channel"));
    if (channel) addChannelToMultiview(channel);
    return;
  }
  const playerGuideMultiview = event.target.closest("[data-player-guide-multiview]");
  if (playerGuideMultiview) {
    event.preventDefault();
    event.stopPropagation();
    const channel = channelByID(playerGuideMultiview.getAttribute("data-player-guide-multiview"));
    if (channel) addChannelToMultiview(channel);
    return;
  }
  const multiviewFocus = event.target.closest("[data-multiview-focus]");
  if (multiviewFocus) {
    event.preventDefault();
    focusMultiviewTile(multiviewFocus.getAttribute("data-multiview-focus"));
    return;
  }
  const channelTarget = event.target.closest("[data-channel]");
  if (channelTarget) {
    const channel = channelByID(channelTarget.getAttribute("data-channel"));
    if (channel) playChannel(channel);
  }
  const categoryTarget = event.target.closest("[data-category]");
  if (categoryTarget) setCategory(categoryTarget.getAttribute("data-category"));
});
document.addEventListener("mouseover", function(event) {
  const target = overflowTooltipTarget(event);
  if (!target || (event.relatedTarget && target.contains(event.relatedTarget))) return;
  showOverflowTooltip(target, event);
});
document.addEventListener("mousemove", function(event) {
  const target = overflowTooltipTarget(event);
  if (target) showOverflowTooltip(target, event);
}, { passive: true });
document.addEventListener("mouseout", function(event) {
  const target = overflowTooltipTarget(event);
  if (!target || (event.relatedTarget && target.contains(event.relatedTarget))) return;
  hideOverflowTooltip();
});
document.addEventListener("focusin", function(event) {
  const target = overflowTooltipTarget(event);
  if (target) showOverflowTooltip(target, event);
  if (state.view === "player") wakePlayerChrome();
});
document.addEventListener("focusout", function(event) {
  const target = overflowTooltipTarget(event);
  if (target) hideOverflowTooltip();
});
document.addEventListener("fullscreenchange", updateFullscreenButton);
document.addEventListener("webkitfullscreenchange", updateFullscreenButton);
document.addEventListener("keydown", function(event) {
  if (trapProgramModalFocus(event)) return;
  if (state.programDetails && event.key === "Escape") {
    event.preventDefault();
    closeProgramDetails();
    return;
  }
  if (event.target && event.target.id === "search-page-input" && event.key === "Enter") {
    event.preventDefault();
    clearSearchResultsTimer();
    updateSearchPageResults();
    rememberSearch(state.searchQuery);
    return;
  }
  if (state.view === "search" && event.key === "Escape") {
    event.preventDefault();
    setView(state.searchReturnView || "home");
    return;
  }
  if (state.view !== "player") return;
  const tag = event.target && event.target.tagName ? event.target.tagName.toLowerCase() : "";
  if (tag === "input" || tag === "textarea" || tag === "select") return;
  if (event.key === "Escape" && state.playerSportsOpen) {
    event.preventDefault();
    togglePlayerSports(false);
    return;
  }
  if (event.key === "ArrowDown" && sportsFirstPlayerEnabled() && !state.playerSportsOpen) {
    event.preventDefault();
    togglePlayerSports(true);
    return;
  }
  if (event.key === " " || event.key === "k" || event.key === "K") {
    event.preventDefault();
    togglePlayPause();
  }
  if (event.key === "f" || event.key === "F") {
    event.preventDefault();
    toggleFullscreen();
  }
});
["mousemove", "mousedown", "touchstart", "keydown"].forEach(function(eventName) {
  document.addEventListener(eventName, function(event) {
    if (state.view !== "player") return;
    if (eventName === "mousemove" && event.movementX === 0 && event.movementY === 0) return;
    wakePlayerChrome();
  }, { passive: true });
});
document.addEventListener("change", function(event) {
  const profileID = event.target.getAttribute("data-profile-selection-id");
  if (profileID) {
    updateSelectedProfile(profileID, !!event.target.checked);
    return;
  }
  const adminField = event.target.getAttribute("data-admin-category-field");
  if (adminField) {
    updateCategoryParsingField(adminField, event.target);
    return;
  }
  const adminECMField = event.target.getAttribute("data-admin-ecm-field");
  if (adminECMField) {
    updateAdminECMField(adminECMField, event.target);
    return;
  }
  const adminRecordingField = event.target.getAttribute("data-admin-recording-field");
  if (adminRecordingField) {
    updateAdminRecordingField(adminRecordingField, event.target);
    return;
  }
  const adminPlayerField = event.target.getAttribute("data-admin-player-field");
  if (adminPlayerField) {
    updateAdminPlayerField(adminPlayerField, event.target);
    return;
  }
  const adminTimeShiftField = event.target.getAttribute("data-admin-timeshift-field");
  if (adminTimeShiftField) {
    updateAdminTimeShiftField(adminTimeShiftField, event.target);
    return;
  }
  const adminAliasField = event.target.getAttribute("data-admin-alias-field");
  if (adminAliasField) {
    updateAdminCategoryAlias(Number(event.target.getAttribute("data-admin-alias-index")), adminAliasField, event.target.value || "");
    return;
  }
  if (event.target && event.target.id === "custom-group-select") {
    state.selectedCustomGroup = event.target.value;
    state.customGroupQuery = "";
    state.customGroupChannelID = "";
    renderSettings();
    return;
  }
  const categoryGrouping = event.target.getAttribute("data-category-grouping");
  if (categoryGrouping) {
    state.app.preferences.groupCategoriesByPipe = event.target.checked;
    state.category = "";
    savePrefs();
    render();
    return;
  }
  const hideBucket = event.target.getAttribute("data-hide-bucket");
  if (hideBucket) {
    const bucket = categorySettingsBuckets(sourceCategoriesWithChannels()).find(function(candidate) { return candidate.id === hideBucket; });
    if (!bucket) return;
    bucket.categories.forEach(function(category) {
      if (event.target.checked) state.app.preferences.hiddenCategories[category.sourceID] = true;
      else delete state.app.preferences.hiddenCategories[category.sourceID];
    });
    savePrefs();
    renderCategorySettings();
    return;
  }
  const id = event.target.getAttribute("data-hide");
  if (!id) return;
  if (event.target.checked) state.app.preferences.hiddenCategories[id] = true;
  else delete state.app.preferences.hiddenCategories[id];
  savePrefs();
  renderCategorySettings();
});
document.addEventListener("input", function(event) {
  if (event.target && event.target.hasAttribute("data-account-field") && state.adminSourceEditor) {
    const index = Number(event.target.getAttribute("data-account-index"));
    const account = items(state.adminSourceEditor.accounts)[index];
    if (!account) return;
    const field = event.target.getAttribute("data-account-field");
    account[field] = event.target.type === "checkbox" ? !!event.target.checked : (event.target.type === "number" ? Math.max(0, Number(event.target.value) || 0) : event.target.value);
    if (field === "username" && !account.id) account.id = String(account.username || "account").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
    if (field === "username" || field === "password") account.compatible = false;
    return;
  }
  const sourceFieldMap = { "source-name": "name", "source-url": "baseUrl", "source-username": "username", "source-password": "password", "source-enabled": "enabled", "source-alternate-epg-enabled": "alternateEpgEnabled", "source-alternate-epg-url": "alternateEpgUrl", "source-alternate-epg-policy": "alternateEpgPolicy", "source-hls-buffer-seconds": "hlsBufferSeconds" };
  if (event.target && sourceFieldMap[event.target.id] && state.adminSourceEditor) {
    const field = sourceFieldMap[event.target.id];
    state.adminSourceEditor[field] = event.target.type === "checkbox" ? !!event.target.checked : (event.target.type === "number" ? Number(event.target.value) || 0 : event.target.value);
    if (field.indexOf("alternateEpg") === 0) {
      state.adminSourceEPGResult = null;
      state.adminSourceEPGError = "";
      const result = document.querySelector(".source-epg-result");
      if (result) result.remove();
    }
    return;
  }
  if (event.target && event.target.id === "guide-category-search") {
    state.guideCategoryQuery = event.target.value || "";
    const options = document.querySelector(".guide-category-options");
    if (options) options.innerHTML = guideCategoryOptionsHTML(guideFilterCategories());
    return;
  }
  if (event.target && event.target.id === "category-settings-filter") {
    state.categorySettingsQuery = event.target.value || "";
    renderCategorySettings();
    const input = byId("category-settings-filter");
    if (input) {
      input.focus();
      input.setSelectionRange(input.value.length, input.value.length);
    }
    return;
  }
  if (event.target && event.target.id === "profile-settings-filter") {
    state.profileSettingsQuery = event.target.value || "";
    renderProfileSettings();
    const input = byId("profile-settings-filter");
    if (input) {
      input.focus();
      input.setSelectionRange(input.value.length, input.value.length);
    }
    return;
  }
  if (event.target && event.target.id === "player-volume-slider") {
    state.volume = Number(event.target.value || 0) / 100;
    applyVolumeToVideo();
    syncMultiviewAudio();
  }
  if (event.target && event.target.id === "player-timeshift-range") {
    const video = byId("player");
    if (video && video.seekable && video.seekable.length) {
      video.currentTime = video.seekable.start(0) + Number(event.target.value || 0);
      updateTimeShiftUI();
    }
    return;
  }
  if (event.target && event.target.id === "search-page-input") {
    state.searchQuery = event.target.value || "";
    scheduleSearchResultsUpdate();
    return;
  }
  if (event.target && event.target.id === "player-guide-search") {
    state.playerGuideQuery = event.target.value || "";
    renderPlayerGuidePanel();
    const input = byId("player-guide-search");
    if (input) {
      input.focus();
      input.setSelectionRange(input.value.length, input.value.length);
    }
    return;
  }
  if (event.target && event.target.id === "folder-filter") {
    state.folderQuery = event.target.value || "";
    renderLivePage();
    const input = byId("folder-filter");
    if (input) {
      input.focus();
      input.setSelectionRange(input.value.length, input.value.length);
    }
    return;
  }
  if (event.target && event.target.id === "multiview-search") {
    state.multiviewQuery = event.target.value || "";
    const picker = byId("multiview-picker");
    if (picker) picker.outerHTML = renderMultiviewPicker();
    const input = byId("multiview-search");
    if (input) {
      input.focus();
      input.setSelectionRange(input.value.length, input.value.length);
    }
    return;
  }
  const adminCategoryField = event.target.getAttribute("data-admin-category-field");
  if (adminCategoryField) {
    const settings = state.adminCategorySettings || defaultAdminCategorySettings();
    settings[adminCategoryField] = event.target.type === "checkbox" ? !!event.target.checked : event.target.value;
    state.adminCategorySettings = settings;
    normalizeAdminCategorySettings();
    const preview = byId("organization-preview");
    if (preview) preview.outerHTML = renderOrganizationPreview(adminSettings());
    return;
  }
  const adminEventKeywordIndex = event.target.getAttribute("data-admin-event-keyword-index");
  if (adminEventKeywordIndex !== null) {
    updateAdminEventKeywords(Number(adminEventKeywordIndex), event.target.getAttribute("data-admin-event-keyword-field") || "keywords", event.target.value || "");
    return;
  }
  if (event.target && event.target.id === "custom-group-channel-search") {
    state.customGroupQuery = event.target.value || "";
    state.customGroupChannelID = "";
    renderSettings();
    const input = byId("custom-group-channel-search");
    if (input) {
      input.focus();
      input.setSelectionRange(input.value.length, input.value.length);
    }
  }
});
document.addEventListener("keydown", function(event) {
  if (event.key === "Escape") setSettingsMenuOpen(false);
});
document.querySelectorAll("[data-view]").forEach(function(button) {
  button.onclick = function() {
    setSettingsMenuOpen(false);
    setView(button.dataset.view);
  };
});
const appSearchButton = byId("app-search-button");
if (appSearchButton) appSearchButton.onclick = function() { setView("search"); };
const globalSearch = byId("global-search");
if (globalSearch) {
  globalSearch.oninput = function(event) { state.query = event.target.value; updateLiveSearchFilter(); };
  globalSearch.onkeydown = function(event) {
    if (event.key !== "Enter") return;
    state.searchQuery = event.target.value || "";
    rememberSearch(state.searchQuery);
    setView("search");
  };
}
window.addEventListener("resize", function() {
  if (state.view === "guide") scheduleGuideWindowRender();
});
window.addEventListener("pagehide", function() {
  if (state.currentSession) stopWatchOnUnload(state.currentSession.id);
  items(state.multiviewTiles).forEach(function(tile) {
    if (tile.session) stopWatchOnUnload(tile.session.id);
  });
});
const sportsGameStatsState = { id: "", data: null, loading: false, fetchedAt: 0, timer: null };

function searchMatchScore(primary, secondary, query) {
  const needle = lower(query).trim();
  const title = lower(primary).replace(/\s+/g, " ").trim();
  const details = lower(secondary);
  if (!needle) return 0;
  if (title === needle) return 1000;
  if (title.indexOf(needle) === 0) return 800;
  if (title.indexOf(needle) !== -1) return 600;
  if (details.indexOf(needle) !== -1) return 300;
  return 0;
}

function myTVFollowedPeople() {
  const people = {};
  myTVBuiltInSportsPeople().forEach(function(team) { people[team.id] = team; });
  myTVSportsPeople().forEach(function(team) {
    people[team.id] = team;
    // Follows stored under a legacy id / alias still resolve to the named team.
    sportsTeamIdentityIDs(team).forEach(function(alias) {
      if (!people[alias]) people[alias] = Object.assign({}, team, { id: alias });
    });
  });
  return Object.keys(sportsFavoriteTeamMap()).filter(function(id) { return !!sportsFavoriteTeamMap()[id]; }).map(function(id) {
    return people[id] || { id: id, name: "Saved team", abbreviation: "TV", kind: "Team" };
  });
}

function myTVFeaturedEvents() {
  const followed = Object.assign({}, adminFeaturedEventMap(), featuredEventMap());
  const now = Math.floor(Date.now() / 1000);
  return items(state.events && state.events.events).filter(function(event) {
    return followed[event.id] && (!event.completed || Number(event.endUnix || 0) >= now - 3 * 3600);
  }).sort(function(left, right) { return Number(left.startUnix || 0) - Number(right.startUnix || 0); }).slice(0, 12);
}

function myTVFollowingCard(kind, entity) {
  const isLeague = kind === "league";
  const id = String(entity.id || "");
  const name = entity.name || (isLeague ? "Saved league" : "Saved team");
  const mark = isLeague ? renderSportsLeagueMark(entity) : renderSportsTeamLogo(entity, "my-tv-follow-logo");
  const attr = isLeague ? "data-sports-favorite-league" : "data-sports-favorite-team";
  return "<article class=\"my-tv-follow-card\"><span class=\"my-tv-follow-mark\">" + mark + "</span><span><strong>" + escapeHTML(name) + "</strong><small>" + escapeHTML(isLeague ? "League" : myTVSportsPassLabel(entity)) + "</small></span><button type=\"button\" " + attr + "=\"" + escapeHTML(id) + "\" data-sports-favorite-enabled=\"false\" aria-label=\"Remove " + escapeHTML(name) + " pass\">" + icon("x") + "</button></article>";
}

function myTVFollowingHTML() {
  const passes = keywordPasses();
  const people = sportsEnabled() ? myTVFollowedPeople() : [];
  const leagues = sportsEnabled() ? myTVFollowedLeagues() : [];
  if (!passes.length && !people.length && !leagues.length) return "";
  const searches = passes.map(function(pass) {
    return "<article class=\"my-tv-follow-card query\"><span class=\"my-tv-follow-mark\">" + icon("search") + "</span><span><strong>" + escapeHTML(pass.keyword) + "</strong><small>Title or event</small></span><button type=\"button\" data-keyword-pass-remove=\"" + escapeHTML(pass.id) + "\" aria-label=\"Stop tracking " + escapeHTML(pass.keyword) + "\">" + icon("x") + "</button></article>";
  });
  return "<section class=\"my-tv-section\" aria-label=\"Following\"><div class=\"my-tv-following\">" + searches.concat(people.map(function(team) { return myTVFollowingCard("team", team); }), leagues.map(function(league) { return myTVFollowingCard("league", league); })).join("") + "</div></section>";
}

function myTVSearchResults(query) {
  query = String(query || "").trim();
  if (query.length < 2) return "<p class=\"my-tv-search-note\">Keep typing to search channels, shows, teams, fighters, leagues, and events.</p>";
  const people = rankedSearchMatches(myTVSportsPeople(), function(team) {
    return searchMatchScore(team.name, [team.abbreviation, team.kind, team.leagueName].join(" "), query);
  }, 12);
  const leagues = rankedSearchMatches(items(state.sports && state.sports.leagues), function(league) {
    return searchMatchScore(league.name, [league.sportName, league.id].join(" "), query);
  }, 8);
  const guideTitles = rankedSearchMatches(distinctOnLaterPrograms(programsFor("").filter(function(program) {
    return !programIsGuidePlaceholder(program) && Number(program.endUnix || 0) >= Math.floor(Date.now() / 1000);
  })), function(program) { return searchMatchScore(program.title, programSearchText(program), query); }, 10);
  const events = rankedSearchMatches(items(state.events && state.events.events), function(event) {
    return searchMatchScore(event.name || event.shortName, [event.categoryName, event.description, event.keyword].join(" "), query);
  }, 8);
  const passSaved = keywordPasses().some(function(pass) { return lower(pass.keyword) === lower(query); });
  const trackQuery = people.length || leagues.length ? "" : "<button class=\"my-tv-track-query\" type=\"button\" data-keyword-pass-add=\"" + escapeHTML(query) + "\"" + (passSaved ? " disabled" : "") + ">" + icon(passSaved ? "check" : "plus") + "<span><strong>" + (passSaved ? "Tracking this search" : "Track “" + escapeHTML(query) + "”") + "</strong><small>Watch future guide listings for a match</small></span></button>";
  const personRows = people.map(function(team) {
    const followed = sportsFavoriteTeamMatches(team);
    const passLabel = myTVSportsPassLabel(team);
    return "<article class=\"my-tv-result\"><span class=\"my-tv-result-mark\">" + renderSportsTeamLogo(team, "my-tv-result-logo") + "</span><span><strong>" + escapeHTML(team.name || "Team") + "</strong><small>" + escapeHTML(passLabel) + "</small></span><button type=\"button\" data-sports-favorite-team=\"" + escapeHTML(team.id || "") + "\" data-sports-favorite-enabled=\"" + (followed ? "false" : "true") + "\">" + (followed ? "Game pass active" : "Create game pass") + "</button></article>";
  });
  const leagueRows = leagues.map(function(league) {
    const followed = !!sportsFavoriteLeagueMap()[league.id];
    return "<article class=\"my-tv-result\"><span class=\"my-tv-result-mark\">" + renderSportsLeagueMark(league) + "</span><span><strong>" + escapeHTML(league.name || "League") + "</strong><small>League</small></span><button type=\"button\" data-sports-favorite-league=\"" + escapeHTML(league.id || "") + "\" data-sports-favorite-enabled=\"" + (followed ? "false" : "true") + "\">" + (followed ? "Following" : "Follow") + "</button></article>";
  });
  const titleRows = guideTitles.map(function(program) {
    const saved = keywordPasses().some(function(pass) { return lower(pass.keyword) === lower(program.title); });
    const channel = channelByID(program.channelId) || {};
    return "<article class=\"my-tv-result\"><span class=\"my-tv-result-mark\">" + logoHTML(channel) + "</span><span><strong>" + escapeHTML(program.title || "Program") + "</strong><small>" + escapeHTML([dateTimeLabel(program.startUnix), channel.name].filter(Boolean).join(" · ")) + "</small></span><button type=\"button\" data-keyword-pass-add=\"" + escapeHTML(program.title || "") + "\"" + (saved ? " disabled" : "") + ">" + (saved ? "Tracking" : "Track") + "</button></article>";
  });
  const eventRows = events.map(function(event) {
    const followed = !!(featuredEventMap()[event.id] || adminFeaturedEventMap()[event.id]);
    return "<article class=\"my-tv-result\"><span class=\"my-tv-result-mark\">" + icon("calendar") + "</span><span><strong>" + escapeHTML(event.shortName || event.name || "Event") + "</strong><small>" + escapeHTML([event.categoryName, eventStatusLabel(event)].filter(Boolean).join(" · ")) + "</small></span><button type=\"button\" data-event-feature=\"" + escapeHTML(event.id || "") + "\"" + (adminFeaturedEventMap()[event.id] ? " disabled" : "") + ">" + (followed ? "Following" : "Follow") + "</button></article>";
  });
  const channelRows = rankedSearchMatches(searchableChannels(), function(channel) {
    return searchMatchScore(channel.name, [channel.number, channel.categoryName].join(" "), query);
  }, 12).map(myTVChannelRow);
  const groups = [["Channels", channelRows], ["Teams & fighters", personRows], ["Leagues", leagueRows], ["From the guide", titleRows], ["Events", eventRows]].filter(function(group) { return group[1].length; });
  const catalogNote = state.myTVTeamCatalogLoading && !people.length && !leagues.length ? "<p class=\"my-tv-search-note\">Searching team rosters…</p>" : "";
  return "<section class=\"my-tv-search-results\">" + trackQuery + catalogNote + (groups.length ? groups.map(function(group) { return "<div><h3>" + escapeHTML(group[0]) + "</h3><div class=\"my-tv-result-list\">" + group[1].join("") + "</div></div>"; }).join("") : "<p class=\"my-tv-search-note\">No current listings match yet. Track the search and My TV will watch future guide updates.</p>") + "</section>";
}

function myTVChannelRow(channel) {
  const program = currentProgram(channel) || {};
  return renderSearchResultRow({ attrs: 'data-channel="' + escapeHTML(channel.id) + '"', favoriteChannelId: channel.id, art: logoHTML(channel), title: channel.name || "Channel", meta: program.title || channel.categoryName || "Live channel", action: "Watch" });
}

function myTVFavoriteChannelsHTML() {
  const channels = orderedFavoriteChannels(searchableChannels()).filter(function(channel) { return !!favoriteMap()[channel.id]; });
  if (!channels.length) return "";
  return '<section class="my-tv-section my-tv-favorites" aria-label="Favorite channels"><header><h3>Favorite channels</h3><button class="search-cancel" type="button" data-view="search">Find channels</button></header>' + (channels.length ? '<div class="search-result-list">' + channels.map(myTVChannelRow).join('') + '</div>' : '<p class="my-tv-search-note">Save channels with the heart button and watch them here.</p>') + '</section>';
}

function myTVDashboardHTML() {
  const guidePrograms = myTVGuidePrograms();
  const sportsEvents = sportsEnabled() ? myTVFollowedSportsEvents() : [];
  const upcoming = (guidePrograms.length ? onLaterShelfHTML("From your guide", guidePrograms) : "")
    + (sportsEvents.length ? '<section class="my-tv-section" aria-label="Your sports"><div class="sports-event-grid my-tv-sports-grid">' + sportsEvents.map(renderSportsEventTile).join("") + '</div></section>' : '');
  const empty = !upcoming ? '<section class="my-tv-empty"><div><strong>Make this page yours</strong><p>Search for a channel or show to follow live and upcoming coverage.</p></div><div class="my-tv-empty-actions"><button type="button" data-view="guide">Browse guide</button>' + (sportsEnabled() ? '<button type="button" data-view="sports">Browse sports</button>' : '') + '</div></section>' : '';
  return myTVFavoriteChannelsHTML() + myTVFollowingHTML() + (upcoming ? '<section class="my-tv-up-next"><header><h2>Up next</h2></header>' + upcoming + '</section>' : '') + empty;
}

function updateMyTVSearchSurface() {
  const query = state.myTVQuery || "";
  const title = byId("my-tv-title");
  const description = byId("my-tv-description");
  const context = byId("my-tv-search-context");
  if (title) title.textContent = query ? "Find & follow" : "My TV";
  if (description) description.textContent = query ? "Save channels and follow shows, teams, or events." : "Your favorite channels, saved follows, and what’s coming up next.";
  if (context) context.hidden = !query;
  const results = byId("my-tv-search-results");
  const dashboard = byId("my-tv-dashboard");
  const clear = byId("my-tv-search-clear");
  if (results) {
    results.hidden = !query;
    const html = query ? myTVSearchResults(query) : "";
    if (results.myTVSearchHTML !== html) {
      results.innerHTML = html;
      results.myTVSearchHTML = html;
    }
  }
  if (dashboard) {
    dashboard.hidden = !!query;
    if (!query) dashboard.innerHTML = myTVDashboardHTML();
  }
  if (clear) clear.hidden = !query;
}

function renderMyTVPage() {
  const root = byId("view");
  if (!root) return;
  if (!state.sports && !state.sportsLoading) loadSports(false);
  const query = state.myTVQuery || "";
  root.innerHTML = "<div class=\"my-tv-page\"><header class=\"my-tv-header\"><div class=\"my-tv-heading\"><h2 id=\"my-tv-title\">My TV</h2><p id=\"my-tv-description\">Your favorite channels, saved follows, and what’s coming up next.</p></div><label class=\"my-tv-search\"><span>" + icon("search") + "</span><input id=\"my-tv-search\" type=\"search\" value=\"" + escapeHTML(query) + "\" placeholder=\"Find channels, shows, teams, or events\" aria-label=\"Find something to follow\" autocomplete=\"off\" spellcheck=\"false\"><button id=\"my-tv-search-clear\" type=\"button\" aria-label=\"Clear My TV search\" data-my-tv-search-clear=\"true\"" + (query ? "" : " hidden") + ">" + icon("x") + "</button></label></header>"
    + "<div id=\"my-tv-search-context\" class=\"my-tv-search-context\" hidden><h3>Search results</h3><button class=\"my-tv-back\" type=\"button\" data-my-tv-search-clear=\"true\">" + icon("arrow-left") + "Back to My TV</button></div>"
    + "<div id=\"my-tv-search-results\" class=\"my-tv-search-surface\" aria-live=\"polite\"" + (query ? "" : " hidden") + "></div><div id=\"my-tv-dashboard\"" + (query ? " hidden" : "") + "></div></div>";
  updateMyTVSearchSurface();
  ensureMyTVTeamCatalog(query);
}

function channelFavoriteButton(channelID, name) {
  const saved = !!favoriteMap()[channelID];
  const label = (saved ? "Remove " : "Save ") + (name || "channel") + (saved ? " from My TV favorites" : " to My TV favorites");
  return '<button class="search-channel-favorite' + (saved ? ' active' : '') + '" type="button" data-save-channel="' + escapeHTML(channelID) + '" aria-label="' + escapeHTML(label) + '" title="' + escapeHTML(label) + '" aria-pressed="' + saved + '">' + icon(saved ? "heart-solid" : "heart") + '</button>';
}

function rankedSearchMatches(collection, scorer, limit) {
  return items(collection).map(function(item, index) {
    return { item: item, index: index, score: scorer(item) };
  }).filter(function(result) {
    return result.score > 0;
  }).sort(function(left, right) {
    return right.score - left.score || left.index - right.index;
  }).slice(0, limit).map(function(result) {
    return result.item;
  });
}

function onLaterShelfHTML(title, programs, renderer) {
  const all = distinctOnLaterPrograms(programs);
  if (!all.length) return "";
  const key = onLaterShelfKey(title);
  const limit = Math.max(18, Number(state.onLaterShelfLimits[key] || 18));
  const visible = all.slice(0, limit);
  const remaining = all.length - visible.length;
  const more = remaining > 0 ? "<button class=\"on-later-more\" type=\"button\" data-onlater-more=\"" + escapeHTML(key) + "\"><strong>Show more</strong><span>" + escapeHTML(String(Math.min(18, remaining))) + " of " + escapeHTML(String(remaining)) + " remaining</span></button>" : "";
  const renderCard = renderer || renderProgramDiscoveryCard;
  return "<section class=\"on-later-shelf\" data-onlater-shelf=\"" + escapeHTML(key) + "\"><header><h3>" + escapeHTML(title) + "</h3><span>" + escapeHTML(String(all.length)) + "</span></header><div class=\"on-later-shelf-rail\">" + visible.map(renderCard).join("") + more + "</div></section>";
}

function dedupeChannels(channels) {
  const seen = {};
  return items(channels).filter(function(channel) {
    const key = channelLogicalKey(channel) || ("id:" + String(channel && channel.id || ""));
    if (seen[key]) return false;
    seen[key] = true;
    return true;
  });
}

function featuredEventMap() { return prefs().featuredEvents || {}; }

function distinctOnLaterPrograms(programs) {
  const seen = {};
  return items(programs).filter(function(program) {
    const key = onLaterProgramIdentity(program);
    if (!key || seen[key]) return false;
    seen[key] = true;
    return true;
  });
}

async function getJSONWithin(url, timeoutMs, timeoutMessage) {
  if (typeof AbortController === "undefined") return getJSON(url);
  const controller = new AbortController();
  const timer = setTimeout(function() { controller.abort(); }, Math.max(1000, Number(timeoutMs || 0)));
  try {
    const response = await coreFetch(route(url), { signal: controller.signal });
    if (!response.ok) throw await requestError(response);
    return response.json();
  } catch (error) {
    if (error && error.name === "AbortError") throw new Error(timeoutMessage || "The request took too long. Try again.");
    throw error;
  } finally {
    clearTimeout(timer);
  }
}



function appRouteHash(snapshot) {
  snapshot = snapshot || appRouteSnapshot();
  if (snapshot.view === "admin") return "#/admin/" + appRoutePart(snapshot.adminTab || "source");
  if (snapshot.view === "player" && snapshot.channelID) return "#/watch/" + appRoutePart(snapshot.channelID);
  if (snapshot.view === "sports") {
    let route = "#/sports/" + appRoutePart(snapshot.sportsTab || "live");
    if (snapshot.sportsEvent) route += "/event/" + appRoutePart(snapshot.sportsEvent);
    else if (snapshot.sportsLeague) route += "/league/" + appRoutePart(snapshot.sportsLeague);
    return route;
  }
  if (snapshot.view === "events") {
    let route = "#/events/" + appRoutePart(snapshot.eventsTab || "upcoming");
    if (snapshot.eventCategory) route += "/category/" + appRoutePart(snapshot.eventCategory);
    return route;
  }
  if (snapshot.view === "live" && snapshot.category) return "#/channels/" + appRoutePart(snapshot.category);
  const view = ["home", "channels", "live", "favorites", "guide", "mytv", "search", "recordings", "settings", "multiview"].indexOf(snapshot.view) !== -1 ? snapshot.view : "home";
  return "#/" + view;
}
// External links from provider data must be absolute https URLs; anything
// else (javascript:, data:, relative paths) is dropped.
function safeHTTPS(value) {
  value = String(value == null ? "" : value).trim();
  return /^https:\/\/[^\s"'<>]+$/i.test(value) ? value : "";
}
function externalLinkAttrs(url) {
  const safe = safeHTTPS(url);
  return safe ? " href=\"" + escapeHTML(safe) + "\" target=\"_blank\" rel=\"noopener noreferrer\"" : "";
}

function renderSaveChannelListButton(categoryID) {
  categoryID = String(categoryID || "");
  if (!categoryID) return "";
  const existing = savedLineupForCategory(categoryID);
  const label = existing ? "Edit Channel List" : "Save Channel List";
  return "<button type=\"button\" class=\"section-action save-channel-list\" data-saved-lineup-edit=\"" + escapeHTML(existing ? existing.id : "") + "\" data-saved-lineup-category=\"" + escapeHTML(categoryID) + "\">" + icon(existing ? "heart-solid" : "heart") + "<span>" + label + "</span></button>";
}

function onLaterShelfKey(title) {
  return lower(title).replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "guide";
}

function channelGroupsInSideMenu() { return adminSettings().sideMenuMode === "channels"; }

function eventsNavAvailable() {
  if (!state.events) return true;
  if (state.eventsLoading) return true;
  return items(state.events.events).some(function(event) {
    return uniqueEventChannels(event.channels).length > 0;
  });
}

function channelLogicalKey(channel) {
  if (!channel) return "";
  const guideID = lower(channel.guideId).trim();
  if (guideID) return "guide:" + guideID;
  const name = lower(channel.name).replace(/\s+/g, " ").trim();
  const category = lower(sourceCategoryLabel(channel)).replace(/\s+/g, " ").trim();
  const logo = lower(channel.logoUrl).trim();
  return "channel:" + [name, category, logo].join("|");
}

function onLaterProgramIdentity(program) {
  return String(program && program.id || "") || [program && program.title, program && program.channelId, program && program.startUnix, program && program.endUnix].join("|");
}

function appRouteSnapshot() {
  return {
    view: isAdminRoute ? "admin" : state.view,
    category: state.category || "",
    sportsTab: state.sportsTab || "live",
    sportsLeague: state.sportsLeague || "",
    sportsEvent: state.sportsSelectedEventID || "",
    eventsTab: state.eventsTab || "upcoming",
    eventCategory: state.eventCategory || "",
    adminTab: state.adminTab || "source",
    channelID: state.view === "player" && state.currentChannel ? state.currentChannel.id : ""
  };
}

function appRoutePart(value) {
  return encodeURIComponent(String(value || ""));
}

function savedLineupForCategory(categoryID) {
  return savedLineups().find(function(lineup) { return lineup.categoryId === String(categoryID || ""); }) || null;
}

function savedLineups() { return normalizeSavedLineups(prefs().savedLineups); }

function normalizeSavedLineups(value) {
  const seenIDs = {};
  const seenCategories = {};
  return items(value).map(function(lineup, index) {
    lineup = lineup || {};
    const categoryID = String(lineup.categoryId || "").trim();
    const name = String(lineup.name || "").trim();
    let id = String(lineup.id || "").trim();
    if (!categoryID || !name || seenCategories[categoryID]) return null;
    if (!id) id = "lineup-" + String(index + 1);
    if (seenIDs[id]) id += "-" + String(index + 1);
    seenIDs[id] = true;
    seenCategories[categoryID] = true;
    return { id: id, name: name, categoryId: categoryID, hideFinalGroups: lineup.hideFinalGroups === true };
  }).filter(Boolean).slice(0, 24);
}

document.addEventListener("click", function(event) {
if (!event.target.closest("[data-guide-now],[data-guide-search-channel],[data-save-channel],[data-my-tv-search-clear],[data-onlater-more],[data-sports-tab],[data-sports-open-event],[data-sports-open-league],[data-sports-back],[data-sports-favorite-team],[data-sports-favorite-league],[data-sports-preferred-channel],[data-sports-spoilers],[data-search-airing]")) return;
event.stopImmediatePropagation();
const guideNow = event.target.closest("[data-guide-now]");
if (guideNow) { event.preventDefault(); jumpGuideToNow(); return; }
const guideResult = event.target.closest("[data-guide-search-channel]");
if (guideResult) {
    event.preventDefault();
    jumpGuideToProgram(guideResult.getAttribute("data-guide-search-channel"), guideResult.getAttribute("data-guide-search-program"), Number(guideResult.getAttribute("data-guide-search-start")));
    return;
  }
const searchAiring = event.target.closest("[data-search-airing]");
if (searchAiring) {
    event.preventDefault();
    rememberSearch(state.searchQuery);
    closeProgramDetails();
    state.searchQuery = searchAiring.getAttribute("data-search-airing") || state.searchQuery;
    state.searchAiringChannel = searchAiring.getAttribute("data-search-airing-channel") || "";
    state.searchType = "programs";
    setView("search");
    return;
  }
const myTVSearchClear = event.target.closest("[data-my-tv-search-clear]");
if (myTVSearchClear) {
    event.preventDefault();
    state.myTVQuery = "";
    const input = byId("my-tv-search");
    if (input) {
      input.value = "";
      input.focus();
    }
    updateMyTVSearchSurface();
    return;
  }
const onLaterMore = event.target.closest("[data-onlater-more]");
if (onLaterMore) {
    event.preventDefault();
    const key = onLaterMore.getAttribute("data-onlater-more") || "guide";
    const previousLimit = Math.max(18, Number(state.onLaterShelfLimits[key] || 18));
    const previousRail = onLaterMore.closest(".on-later-shelf-rail");
    const previousScrollLeft = previousRail ? Number(previousRail.scrollLeft || 0) : 0;
    state.onLaterShelfLimits[key] = previousLimit + 18;
    if (state.view === "mytv") updateMyTVSearchSurface();
    else renderOnLaterPage();
    restoreOnLaterShelfPosition(key, previousLimit, previousScrollLeft);
    return;
  }
const sportsTab = event.target.closest("[data-sports-tab]");
if (sportsTab) {
    event.preventDefault();
    setSportsTab(sportsTab.getAttribute("data-sports-tab"));
    return;
  }
const sportsOpenEvent = event.target.closest("[data-sports-open-event]");
if (sportsOpenEvent) {
    event.preventDefault();
    openSportsEvent(sportsOpenEvent.getAttribute("data-sports-open-event"));
    return;
  }
const sportsOpenLeague = event.target.closest("[data-sports-open-league]");
if (sportsOpenLeague) {
    event.preventDefault();
    openSportsLeague(sportsOpenLeague.getAttribute("data-sports-open-league"));
    return;
  }
const sportsBack = event.target.closest("[data-sports-back]");
if (sportsBack) {
    event.preventDefault();
    backFromSportsDetail();
    return;
  }
const sportsFavorite = event.target.closest("[data-sports-favorite-team]");
if (sportsFavorite) {
    event.preventDefault();
    toggleSportsTeamFavorite(sportsFavorite.getAttribute("data-sports-favorite-team"), sportsFavorite.getAttribute("data-sports-favorite-enabled") === "true");
    return;
  }
const sportsFavoriteLeague = event.target.closest("[data-sports-favorite-league]");
if (sportsFavoriteLeague) {
    event.preventDefault();
    toggleSportsLeagueFavorite(sportsFavoriteLeague.getAttribute("data-sports-favorite-league"), sportsFavoriteLeague.getAttribute("data-sports-favorite-enabled") === "true");
    return;
  }
const sportsPreferredChannel = event.target.closest("[data-sports-preferred-channel]");
if (sportsPreferredChannel) {
    event.preventDefault();
    event.stopPropagation();
    setSportsPreferredChannel(sportsPreferredChannel.getAttribute("data-sports-preferred-league"), sportsPreferredChannel.getAttribute("data-sports-preferred-channel"));
    return;
  }
const sportsSpoilers = event.target.closest("[data-sports-spoilers]");
if (sportsSpoilers) {
    event.preventDefault();
    toggleSportsSpoilers(sportsSpoilers.getAttribute("data-sports-spoilers"));
    return;
  }
const saveChannel = event.target.closest("[data-save-channel]");
if (saveChannel) {
    event.preventDefault();
    const id = saveChannel.getAttribute("data-save-channel");
    const saved = setChannelFavorite(id, !favoriteMap()[id]);
    showAppToast(saved ? "Channel saved to My TV." : "Channel removed from My TV favorites.");
    if (state.view === "mytv") updateMyTVSearchSurface();
    else {
      const results = byId("search-page-results");
      if (results) results.innerHTML = renderSearchPageResults();
    }
    return;
  }
}, true);

function restoreOnLaterShelfPosition(key, previousLimit, previousScrollLeft) {
  const root = byId("view");
  if (!root || typeof root.querySelector !== "function") return false;
  const shelf = root.querySelector('[data-onlater-shelf="' + String(key || "guide").replace(/[^a-z0-9-]/gi, "") + '"]');
  const rail = shelf && shelf.querySelector(".on-later-shelf-rail");
  if (!rail) return false;
  const cards = typeof rail.querySelectorAll === "function" ? rail.querySelectorAll(".on-later-program-card") : [];
  const firstNewCard = cards && cards[previousLimit];
  rail.scrollLeft = Math.max(Number(previousScrollLeft || 0), firstNewCard ? Math.max(0, Number(firstNewCard.offsetLeft || 0) - 16) : 0);
  const focusTarget = firstNewCard && typeof firstNewCard.querySelector === "function" ? firstNewCard.querySelector("button") : null;
  if (focusTarget && typeof focusTarget.focus === "function") {
    try { focusTarget.focus({ preventScroll: true }); } catch (_) { focusTarget.focus(); }
  }
  return !!firstNewCard;
}
startGuideAutoRefresh();
startPrefsSync();
document.addEventListener("input", function(event) {
  if (event.target.id === "my-tv-search") {
    state.myTVQuery = event.target.value || "";
    updateMyTVSearchSurface();
    ensureMyTVTeamCatalog(state.myTVQuery);
  }
}, true);
document.addEventListener("keydown", function(event) {
  if (handleGuideKeyboard(event)) event.stopImmediatePropagation();
}, true);
document.addEventListener("focusin", function(event) {
  if (state.view === "guide") rememberGuideFocus(event.target);
});
(isAdminRoute ? loadAdminApp() : loadApp()).catch(function() {
  byId("view").innerHTML = emptyStateHTML(isAdminRoute ? "Unable to load Xtreme sources." : "Unable to load Live TV.", isAdminRoute ? "Confirm this plugin is enabled, then refresh the admin app." : "Check your Xtreme Codes sources in Live TV Admin, then refresh this page.");
});
