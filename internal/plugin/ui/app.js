const path = window.location.pathname;
const base = path.endsWith("/dispatcharr/player") ? path.slice(0, -"/dispatcharr/player".length) : (path.endsWith("/dispatcharr/admin") ? path.slice(0, -"/dispatcharr/admin".length) : (path.endsWith("/dispatcharr") ? path.slice(0, -"/dispatcharr".length) : ""));
const isAdminRoute = path.endsWith("/dispatcharr/admin");
const adminSettingsKey = "adminCategorySettings";
const pluginInstallationID = (base.match(/\/api\/v1\/plugins\/(\d+)/) || [])[1] || "";
const localCacheSuffix = pluginInstallationID || "default";
const appCacheKey = "silo.ramindex.dispatcharr.appSnapshot.v1." + localCacheSuffix;
const assetVersionMeta = document.querySelector('meta[name="dispatcharr-asset-version"]');
const assetVersion = assetVersionMeta ? String(assetVersionMeta.content || "") : "";
const assetPrefix = path.endsWith("/dispatcharr") ? "dispatcharr/assets" : "assets";
const state = { app: null, appLoadedFromCache: false, programsByChannel: {}, sortedPrograms: [], view: isAdminRoute ? "admin" : "home", category: "", query: "", folderQuery: "", searchQuery: "", searchType: "all", searchReturnView: "home", recentSearches: [], onLaterTime: "all", onLaterType: "all", hls: null, tsPlayer: null, currentChannel: null, currentSession: null, heartbeat: null, muted: false, volume: 1, volumeMenuOpen: false, audioMenuOpen: false, moreMenuOpen: false, playerGuideOpen: false, playerGuideQuery: "", playerSportsOpen: false, playerSportsTimer: null, playerReturnContext: null, selectedAudioTrack: 0, selectedTextTrack: -1, aspectMode: "fill", playerChromeIdle: false, playerChromeTimer: null, playerWaiting: false, multiviewTiles: [], multiviewActiveTileID: "", multiviewQuery: "", multiviewHeartbeat: null, recordings: null, recordingsLoading: false, recordingCapability: null, sports: null, sportsLoading: false, sportsLeague: "", sportsExpandedEvents: {}, events: null, eventsLoading: false, eventsTab: "upcoming", eventCategory: "", expandedEvents: {}, guideChannels: [], guideRendered: 0, guideLoading: false, guideWindowStart: -1, guideWindowEnd: -1, guideRenderFrame: 0, guideWarmPings: {}, guideAutoTimer: null, guideLastSlotStart: 0, guideLastAutoFetchAt: 0, guideAutoFetching: false, programDetails: null, refreshing: false, virtualCategoryView: "guide", selectedCustomGroup: "", customGroupQuery: "", customGroupChannelID: "", profileSettingsQuery: "", profileSelectionIDMap: null, profileChannelFilterMap: null, adminTab: "settings", adminCategorySettings: null, savedAdminCategorySettings: null, profileSaveStatus: "idle", profileSaveMessage: "", adminSaveStatus: "idle", adminSaveMessage: "", adminStatusRefreshing: false, adminProfileRefreshing: false, timeShiftSession: null, timeShiftHeartbeat: null, timeShiftTimelineTimer: null, timeShiftAttempt: 0, timeShiftAdminStatus: null, timeShiftAdminLoading: false };

function applySiloTheme() {
  const params = new URLSearchParams(window.location.search);
  const theme = String(params.get("theme") || document.documentElement.dataset.siloTheme || "").trim().toLowerCase().replace(/[^a-z0-9_-]/g, "");
  if (theme) document.documentElement.dataset.siloTheme = theme;
}

applySiloTheme();

function route(url) { return base + url; }
function assetURL(filename) {
  return assetPrefix + "/" + filename + (assetVersion ? "?v=" + encodeURIComponent(assetVersion) : "");
}
const playerLibraryPromises = {};
let programModalReturnFocus = null;
function loadPlayerLibrary(filename, globalName) {
  if (window[globalName]) return Promise.resolve();
  if (playerLibraryPromises[globalName]) return playerLibraryPromises[globalName];
  playerLibraryPromises[globalName] = new Promise(function(resolve, reject) {
    const script = document.createElement("script");
    script.src = assetURL(filename);
    script.async = true;
    script.onload = function() {
      if (window[globalName]) resolve();
      else reject(new Error(globalName + " did not initialize"));
    };
    script.onerror = function() { reject(new Error("could not load " + filename)); };
    document.head.appendChild(script);
  }).catch(function(error) {
    delete playerLibraryPromises[globalName];
    throw error;
  });
  return playerLibraryPromises[globalName];
}
function ensurePlayerLibraries(format) {
  if (format === "hls") return loadPlayerLibrary("hls.min.js", "Hls");
  if (format === "mpegts") return loadPlayerLibrary("mpegts.min.js", "mpegts");
  return Promise.all([
    loadPlayerLibrary("hls.min.js", "Hls"),
    loadPlayerLibrary("mpegts.min.js", "mpegts")
  ]);
}
function byId(id) { return document.getElementById(id); }
function items(value) { return Array.isArray(value) ? value : []; }
function lower(value) { return String(value || "").toLowerCase(); }
function uniqueIDs(values) {
  const seen = {};
  const result = [];
  items(values).forEach(function(value) {
    value = String(value || "");
    if (!value || seen[value]) return;
    seen[value] = true;
    result.push(value);
  });
  return result;
}
function escapeHTML(value) {
  return String(value || "").replace(/[&<>"']/g, function(ch) {
    return ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;" })[ch];
  });
}
function cssEscape(value) {
  if (window.CSS && CSS.escape) return CSS.escape(String(value || ""));
  return String(value || "").replace(/\\/g, "\\\\").replace(/"/g, "\\\"");
}
function icon(name) {
  const icons = {
    "arrow-left": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M15.75 19.5 8.25 12l7.5-7.5'/></svg>",
    "chevron-down": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='m6 9 6 6 6-6'/></svg>",
    "ellipsis": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M6.75 12a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0Zm6 0a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0Zm6 0a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0Z'/></svg>",
    "play": "<svg viewBox='0 0 24 24' fill='currentColor' aria-hidden='true'><path d='M8 5.6v12.8c0 .55.6.9 1.08.62l10.1-6.4a.73.73 0 0 0 0-1.24L9.08 4.98A.72.72 0 0 0 8 5.6Z'/></svg>",
    "record": "<svg viewBox='0 0 24 24' fill='currentColor' aria-hidden='true'><path d='M12 20.25a8.25 8.25 0 1 0 0-16.5 8.25 8.25 0 0 0 0 16.5Zm0-4a4.25 4.25 0 1 1 0-8.5 4.25 4.25 0 0 1 0 8.5Z'/></svg>",
    "pause": "<svg viewBox='0 0 24 24' fill='currentColor' aria-hidden='true'><path d='M7.25 5.25h3.25v13.5H7.25zM13.5 5.25h3.25v13.5H13.5z'/></svg>",
    "loader": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' d='M12 3a9 9 0 1 1-8.3 5.5'/></svg>",
    "speaker": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M19.1 8.9a7 7 0 0 1 0 6.2M16.2 10.9a3 3 0 0 1 0 2.2M4.5 14.25h3l4.25 3.25V6.5L7.5 9.75h-3v4.5Z'/></svg>",
    "speaker-off": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='m4.5 4.5 15 15M5 14.25h2.5l4.25 3.25v-5.75M11.75 8.7V6.5L8.8 8.75M16 10.8a3 3 0 0 1 .2 2.2'/></svg>",
    "airplay": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M6.75 17.25h-1.5A2.25 2.25 0 0 1 3 15V6.75A2.25 2.25 0 0 1 5.25 4.5h13.5A2.25 2.25 0 0 1 21 6.75V15a2.25 2.25 0 0 1-2.25 2.25h-1.5M8.25 21h7.5L12 16.5 8.25 21Z'/></svg>",
    "guide": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M4.5 6.75h15M4.5 12h15M4.5 17.25h15M8.25 4.5v15M15.75 4.5v15'/></svg>",
    "clock": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z'/><path stroke-linecap='round' stroke-linejoin='round' d='M12 7.5V12l3 2.25'/></svg>",
    "multiview": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M4.5 5.75h6.25v5.75H4.5zM13.25 5.75h6.25v5.75h-6.25zM4.5 14h6.25v4.25H4.5zM13.25 14h6.25v4.25h-6.25z'/></svg>",
    "trophy": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M8 4.5h8v4.25a4 4 0 0 1-8 0V4.5ZM9.5 14.5h5M12 12.75V18M8.5 20h7'/><path stroke-linecap='round' stroke-linejoin='round' d='M8 6H5.25v1.5A3.5 3.5 0 0 0 8.5 11M16 6h2.75v1.5A3.5 3.5 0 0 1 15.5 11'/></svg>",
    "settings": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M12 8.25a3.75 3.75 0 1 1 0 7.5 3.75 3.75 0 0 1 0-7.5Z'/><path stroke-linecap='round' stroke-linejoin='round' d='M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.6 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 8.92 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9c.23.64.84 1 1.51 1H21a2 2 0 0 1 0 4h-.09A1.65 1.65 0 0 0 19.4 15Z'/></svg>",
    "fullscreen": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M8.25 4.5H4.5v3.75M15.75 4.5h3.75v3.75M19.5 15.75v3.75h-3.75M4.5 15.75v3.75h3.75M9 9 4.5 4.5M15 9l4.5-4.5M15 15l4.5 4.5M9 15l-4.5 4.5'/></svg>",
    "fullscreen-exit": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M4.5 9h4.25V4.75M15.25 4.75V9h4.25M19.5 15h-4.25v4.25M8.75 19.25V15H4.5M8.75 9 4.5 4.75M15.25 9l4.25-4.25M15.25 15l4.25 4.25M8.75 15 4.5 19.25'/></svg>",
    "heart": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M21 8.25c0 6.25-9 11.25-9 11.25s-9-5-9-11.25A4.75 4.75 0 0 1 11.25 5 4.75 4.75 0 0 1 21 8.25Z'/></svg>",
    "heart-solid": "<svg viewBox='0 0 24 24' fill='currentColor' aria-hidden='true'><path d='M12 21s-9-5.1-9-12.25A5.45 5.45 0 0 1 12 4.7a5.45 5.45 0 0 1 9 4.05C21 15.9 12 21 12 21Z'/></svg>",
    "pip": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M4.5 6.75A2.25 2.25 0 0 1 6.75 4.5h10.5a2.25 2.25 0 0 1 2.25 2.25v10.5a2.25 2.25 0 0 1-2.25 2.25H6.75a2.25 2.25 0 0 1-2.25-2.25V6.75Z'/><path stroke-linecap='round' stroke-linejoin='round' d='M13.25 13.25h4.25v3.25h-4.25z'/></svg>",
    "captions": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M4.5 7.5A2.5 2.5 0 0 1 7 5h10a2.5 2.5 0 0 1 2.5 2.5v9A2.5 2.5 0 0 1 17 19H7a2.5 2.5 0 0 1-2.5-2.5v-9Z'/><path stroke-linecap='round' stroke-linejoin='round' d='M8.25 10.5h3M8.25 14h2.25M13.5 14h2.25'/></svg>",
    "language": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z'/><path stroke-linecap='round' stroke-linejoin='round' d='M3.75 9h16.5M3.75 15h16.5M12 3c2.25 2.35 3.25 5.25 3.25 9S14.25 18.65 12 21c-2.25-2.35-3.25-5.25-3.25-9S9.75 5.35 12 3Z'/></svg>",
    "aspect": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M4.5 7.25A2.75 2.75 0 0 1 7.25 4.5h9.5a2.75 2.75 0 0 1 2.75 2.75v9.5a2.75 2.75 0 0 1-2.75 2.75h-9.5a2.75 2.75 0 0 1-2.75-2.75v-9.5Z'/><path stroke-linecap='round' stroke-linejoin='round' d='M8 8h3M8 8v3M16 16h-3M16 16v-3'/></svg>",
    "rewind": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M8 8H4V4M4.75 8.5A8 8 0 1 1 4 14'/><path stroke-linecap='round' stroke-linejoin='round' d='M9 10.5v5M9 10.5H7.75M13 11.25a1.75 1.75 0 0 1 3.5 0v3a1.75 1.75 0 0 1-3.5 0v-3Z'/></svg>",
    "forward": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M16 8h4V4M19.25 8.5A8 8 0 1 0 20 14'/><path stroke-linecap='round' stroke-linejoin='round' d='M8 10.5v5M8 10.5H6.75M12 11.25a1.75 1.75 0 0 1 3.5 0v3a1.75 1.75 0 0 1-3.5 0v-3Z'/></svg>",
    "search": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='m20 20-4.5-4.5M10.5 18a7.5 7.5 0 1 1 0-15 7.5 7.5 0 0 1 0 15Z'/></svg>",
    "copy": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M8 8h9.25A1.75 1.75 0 0 1 19 9.75v9.5A1.75 1.75 0 0 1 17.25 21h-9.5A1.75 1.75 0 0 1 6 19.25V10'/><path stroke-linecap='round' stroke-linejoin='round' d='M5.75 16H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v.75'/></svg>",
    "external": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M13.5 4.5H19.5V10.5M19.25 4.75 11 13M10.5 6H6.75A2.25 2.25 0 0 0 4.5 8.25v9A2.25 2.25 0 0 0 6.75 19.5h9A2.25 2.25 0 0 0 18 17.25V13.5'/></svg>",
    "integrations": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M8.25 8.25h7.5v7.5h-7.5zM4.75 12h3.5M15.75 12h3.5M12 4.75v3.5M12 15.75v3.5'/><path stroke-linecap='round' stroke-linejoin='round' d='M6.25 6.25 8.5 8.5M17.75 6.25 15.5 8.5M6.25 17.75 8.5 15.5M17.75 17.75 15.5 15.5'/></svg>",
    "x": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M6 6l12 12M18 6 6 18'/></svg>"
  };
  return icons[name] || "";
}
function menuIcon(name) { return "<span class=\"menu-icon\">" + icon(name) + "</span>"; }
function defaultPrefs() {
  return { favorites: {}, favoriteOrder: [], autoFavorites: {}, hiddenCategories: {}, sportsFavoriteTeams: {}, keywordPasses: [], recentSearches: [], recentChannels: [], continueWatching: {}, playback: { backendProxySupported: false, streamMode: "redirect", outputFormat: "ts" }, categoryParsing: { enabled: false, mode: "off", delimiter: "pipe", regex: "", output: "" }, profileSelection: { mode: "all", profileIds: [] }, customGroups: [], customGroupMemberships: {} };
}
function prefs() { return state.app && state.app.preferences ? state.app.preferences : defaultPrefs(); }
function availableChannelProfiles() {
  return items(state.app && state.app.source && state.app.source.profiles).filter(function(profile) {
    return profile && profile.id && profile.name;
  }).slice().sort(function(left, right) {
    return String(left.name || left.id).localeCompare(String(right.name || right.id));
  });
}
function normalizeProfileSelection(value) {
  value = value || {};
  let mode = value.mode === "selected" ? "selected" : "all";
  let profileIDs = uniqueIDs(items(value.profileIds).map(function(id) { return String(id || "").trim(); }).filter(Boolean));
  const profiles = availableChannelProfiles();
  if (profiles.length) {
    const valid = {};
    profiles.forEach(function(profile) { valid[profile.id] = true; });
    profileIDs = profileIDs.filter(function(id) { return !!valid[id]; });
  }
  if (mode === "selected" && !profileIDs.length) mode = "all";
  return { mode: mode, profileIds: mode === "all" ? [] : profileIDs };
}
function profileSelection() { return normalizeProfileSelection(prefs().profileSelection); }
function selectedProfileMap() {
  if (state.profileSelectionIDMap !== null) return state.profileSelectionIDMap || null;
  const selection = profileSelection();
  if (selection.mode !== "selected") {
    state.profileSelectionIDMap = false;
    return null;
  }
  const selected = {};
  selection.profileIds.forEach(function(id) { selected[id] = true; });
  state.profileSelectionIDMap = selected;
  return selected;
}
function profileSelectionIsAll() { return !selectedProfileMap(); }
function invalidateProfileSelectionCache() {
  state.profileSelectionIDMap = null;
  state.profileChannelFilterMap = null;
}
function selectedProfileChannelMap() {
  if (state.profileChannelFilterMap !== null) return state.profileChannelFilterMap || null;
  const selected = selectedProfileMap();
  if (!selected) {
    state.profileChannelFilterMap = false;
    return null;
  }
  const channels = {};
  items(state.app && state.app.channels).forEach(function(channel) {
    if (items(channel && channel.profileIds).some(function(id) { return !!selected[id]; })) channels[channel.id] = true;
  });
  state.profileChannelFilterMap = channels;
  return channels;
}
function channelMatchesProfileSelection(channel) {
  const allowed = selectedProfileChannelMap();
  if (!allowed) return true;
  if (!channel) return false;
  if (channel.id && allowed[channel.id]) return true;
  const selected = selectedProfileMap();
  return !channel.id && items(channel.profileIds).some(function(id) { return !!selected[id]; });
}
function defaultEventKeywordRules() {
  return [
    { categoryId: "awards", categoryName: "Awards", keywords: ["Academy Awards", "The Oscars", "Oscars", "Tony Awards", "The Tonys", "Golden Globes", "Grammy Awards", "Grammys", "Emmy Awards", "Emmys", "CMA Awards", "ACM Awards", "Billboard Music Awards", "American Music Awards", "BET Awards", "MTV Video Music Awards", "Critics Choice Awards", "SAG Awards"] },
    { categoryId: "civic", categoryName: "Civic", keywords: ["State of the Union", "Presidential Address", "Joint Session", "Inauguration", "Election Night", "Presidential Debate"] },
    { categoryId: "parades", categoryName: "Parades", keywords: ["Thanksgiving Day Parade", "Macy's Thanksgiving Day Parade", "Rose Parade", "Christmas Parade"] },
    { categoryId: "entertainment", categoryName: "Entertainment", keywords: ["Live Special", "Special Presentation", "Red Carpet", "Ceremony", "Tribute Concert", "Benefit Concert", "Festival"] },
    { categoryId: "golf", categoryName: "Golf", keywords: ["PGA Tour", "LPGA Tour", "DP World Tour", "The Masters", "U.S. Open Golf", "The Open Championship", "Ryder Cup"], excludeKeywords: ["Golf Central", "highlights", "replay", "preview", "recap", "best of"], eventSeries: true, groupWindowMinutes: 60 },
    { categoryId: "motor-racing", categoryName: "Motor Racing", keywords: ["Formula 1", "F1 Grand Prix", "Grand Prix"], excludeKeywords: ["highlights", "replay", "practice recap", "post race", "pre race"], eventSeries: true, groupWindowMinutes: 60 },
    { categoryId: "combat-sports", categoryName: "Combat Sports", keywords: ["UFC", "Ultimate Fighting Championship", "MMA"], excludeKeywords: ["highlights", "replay", "countdown", "weigh-in", "preview", "recap"], eventSeries: true, groupWindowMinutes: 60 },
    { categoryId: "tennis", categoryName: "Tennis", keywords: ["ATP Tour", "WTA Tour", "Wimbledon", "US Open Tennis", "French Open Tennis", "Australian Open Tennis"], excludeKeywords: ["highlights", "replay", "preview", "recap", "best of"], eventSeries: true, groupWindowMinutes: 60 }
  ];
}
function defaultAdminCategorySettings() {
  return { mode: "normal", delimiter: "pipe", virtualGroupLabel: "Groups", virtualGroupSource: "group", collapseDuplicateVirtualGroups: true, allowRecordingsByDefault: true, sportsFirstPlayerEnabled: false, liveRewindEnabled: false, liveRewindCacheGB: 5, liveRewindWindowMinutes: 30, liveRewindMinFreeGB: 2, liveRewindMaxChannels: 20, inferChannelNameGroups: false, ecmEnabled: false, ecmURL: "", categoryRenames: [], categoryAliases: [], eventKeywords: defaultEventKeywordRules() };
}
function cloneAdminCategorySettings(settings) {
  try { return JSON.parse(JSON.stringify(Object.assign(defaultAdminCategorySettings(), settings || {}))); }
  catch (_) { return defaultAdminCategorySettings(); }
}
function adminSettingsSignature(settings) {
  return JSON.stringify(cloneAdminCategorySettings(settings));
}
function adminSettingsDirty() {
  return adminSettingsSignature(state.adminCategorySettings) !== adminSettingsSignature(state.savedAdminCategorySettings);
}
function markAdminSettingsDraft() {
  if (state.adminSaveStatus !== "saving") {
    if (adminSettingsDirty()) {
      state.adminSaveStatus = "dirty";
      state.adminSaveMessage = "Unsaved changes.";
    } else {
      state.adminSaveStatus = "idle";
      state.adminSaveMessage = "";
    }
  }
}
function adminSettings() {
  return Object.assign(defaultAdminCategorySettings(), state.adminCategorySettings || {});
}
function sourceMode() { return state.app && state.app.source ? String(state.app.source.mode || "") : ""; }
function isDispatcharrDirectSource() {
  const mode = sourceMode();
  return mode === "direct_login" || mode === "api_key";
}
function liveRewindEnabled() {
  return !!(isDispatcharrDirectSource() && adminSettings().liveRewindEnabled === true);
}
function sportsFirstPlayerEnabled() {
  return adminSettings().sportsFirstPlayerEnabled === true;
}
function dvrEnabled() {
  return !!(state.app && state.app.capabilities && state.app.capabilities.recordings && isDispatcharrDirectSource() && adminSettings().allowRecordingsByDefault !== false);
}
function recordingSchedulingEnabled() {
  return !!(dvrEnabled() && state.recordingCapability && state.recordingCapability.canSchedule);
}
function recordingScheduleReason() {
  const capability = state.recordingCapability || {};
  return capability.reason || "Scheduling requires a Dispatcharr admin account or Admin API Key.";
}
async function loadRecordingCapability() {
  if (!dvrEnabled()) {
    state.recordingCapability = { available: false, canSchedule: false, reason: "Recordings require Dispatcharr Direct Connect." };
    return state.recordingCapability;
  }
  state.recordingCapability = await getJSON("/dispatcharr/api/recordings/capability").catch(function() {
    return { available: true, canSchedule: false, reason: "Unable to verify Dispatcharr recording permissions." };
  });
  return state.recordingCapability;
}
function favoriteMap() { return prefs().favorites || {}; }
function autoFavoriteMap() { return prefs().autoFavorites || {}; }
function hiddenMap() { return prefs().hiddenCategories || {}; }
function sportsFavoriteTeamMap() { return prefs().sportsFavoriteTeams || {}; }
function normalizeKeywordPasses(value) {
  const seen = {};
  return items(value).map(function(pass) {
    pass = pass || {};
    const keyword = String(pass.keyword || pass.name || "").trim();
    if (!keyword) return null;
    const id = String(pass.id || ("keyword:" + lower(keyword).replace(/[^a-z0-9]+/g, "-"))).replace(/^-+|-+$/g, "");
    const key = lower(keyword);
    if (!id || seen[key]) return null;
    seen[key] = true;
    return { id: id, keyword: keyword, createdAt: Number(pass.createdAt || Date.now()) };
  }).filter(Boolean).slice(0, 24);
}
function keywordPasses() { return normalizeKeywordPasses(prefs().keywordPasses); }
function mergePrefs(remote) {
  remote = Object.assign(defaultPrefs(), remote || {});
  return {
    favorites: Object.assign({}, remote.favorites),
    favoriteOrder: uniqueIDs(items(remote.favoriteOrder)),
    autoFavorites: Object.assign({}, remote.autoFavorites),
    hiddenCategories: Object.assign({}, remote.hiddenCategories),
    sportsFavoriteTeams: Object.assign({}, remote.sportsFavoriteTeams),
    keywordPasses: normalizeKeywordPasses(remote.keywordPasses),
    recentSearches: uniqueIDs(items(remote.recentSearches).map(function(value) { return String(value || "").trim(); }).filter(Boolean)).slice(0, 12),
    recentChannels: uniqueIDs(items(remote.recentChannels)).slice(0, 24),
    continueWatching: Object.assign({}, remote.continueWatching),
    playback: Object.assign({}, remote.playback),
    categoryParsing: Object.assign({}, remote.categoryParsing),
    profileSelection: normalizeProfileSelection(remote.profileSelection),
    customGroups: items(remote.customGroups),
    customGroupMemberships: Object.assign({}, remote.customGroupMemberships)
  };
}
function normalizePreferences() {
  if (!state.app || !state.app.preferences) return;
  state.app.preferences = Object.assign(defaultPrefs(), state.app.preferences || {});
  state.app.preferences.categoryParsing = Object.assign(defaultPrefs().categoryParsing, state.app.preferences.categoryParsing || {});
  state.app.preferences.profileSelection = normalizeProfileSelection(state.app.preferences.profileSelection);
  state.app.preferences.sportsFavoriteTeams = state.app.preferences.sportsFavoriteTeams || {};
  state.app.preferences.keywordPasses = normalizeKeywordPasses(state.app.preferences.keywordPasses);
  state.app.preferences.recentSearches = uniqueIDs(items(state.app.preferences.recentSearches).map(function(value) { return String(value || "").trim(); }).filter(Boolean)).slice(0, 12);
  state.app.preferences.customGroups = items(state.app.preferences.customGroups);
  state.app.preferences.customGroupMemberships = state.app.preferences.customGroupMemberships || {};
  const valid = {};
  items(state.app.channels).forEach(function(channel) { valid[channel.id] = true; });
  const explicitFavorites = Object.keys(state.app.preferences.favorites || {}).filter(function(id) { return !!state.app.preferences.favorites[id] && !!valid[id]; });
  state.app.preferences.favoriteOrder = uniqueIDs(items(state.app.preferences.favoriteOrder).filter(function(id) { return !!state.app.preferences.favorites[id] && !!valid[id]; }).concat(explicitFavorites));
  const recent = uniqueIDs(items(state.app.preferences.recentChannels).filter(function(id) { return !!valid[id]; }));
  const watched = Object.keys(state.app.preferences.continueWatching || {}).sort(function(left, right) {
    const leftPlayed = Number((state.app.preferences.continueWatching[left] || {}).playedAt || 0);
    const rightPlayed = Number((state.app.preferences.continueWatching[right] || {}).playedAt || 0);
    return rightPlayed - leftPlayed;
  }).filter(function(id) { return !!valid[id]; });
  state.app.preferences.recentChannels = uniqueIDs(recent.concat(watched)).slice(0, 24);
  Object.keys(state.app.preferences.customGroupMemberships).forEach(function(groupID) {
    state.app.preferences.customGroupMemberships[groupID] = uniqueIDs(items(state.app.preferences.customGroupMemberships[groupID]).filter(function(id) { return !!valid[id]; }));
  });
  invalidateProfileSelectionCache();
}
function normalizeAdminCategorySettings() {
  state.adminCategorySettings = Object.assign(defaultAdminCategorySettings(), state.adminCategorySettings || {});
  if (state.adminCategorySettings.mode === "custom" || state.adminCategorySettings.mode === "admin_delimiter") state.adminCategorySettings.mode = "delimiter";
  if (["normal", "delimiter"].indexOf(state.adminCategorySettings.mode) === -1) state.adminCategorySettings.mode = "normal";
  if (!state.adminCategorySettings.delimiter) state.adminCategorySettings.delimiter = "pipe";
  if (state.adminCategorySettings.delimiter !== "pipe" && state.adminCategorySettings.delimiter !== "dash") state.adminCategorySettings.delimiter = "pipe";
  state.adminCategorySettings.virtualGroupLabel = virtualGroupLabelSuffix(state.adminCategorySettings.virtualGroupLabel);
  state.adminCategorySettings.allowRecordingsByDefault = state.adminCategorySettings.allowRecordingsByDefault !== false;
  state.adminCategorySettings.sportsFirstPlayerEnabled = state.adminCategorySettings.sportsFirstPlayerEnabled === true;
  state.adminCategorySettings.liveRewindEnabled = state.adminCategorySettings.liveRewindEnabled === true;
  state.adminCategorySettings.liveRewindCacheGB = Math.max(1, Math.min(500, Number(state.adminCategorySettings.liveRewindCacheGB) || 5));
  state.adminCategorySettings.liveRewindWindowMinutes = [15, 30, 60, 90, 120].indexOf(Number(state.adminCategorySettings.liveRewindWindowMinutes)) !== -1 ? Number(state.adminCategorySettings.liveRewindWindowMinutes) : 30;
  state.adminCategorySettings.liveRewindMinFreeGB = Math.max(1, Math.min(100, Number(state.adminCategorySettings.liveRewindMinFreeGB) || 2));
  state.adminCategorySettings.liveRewindMaxChannels = Math.max(1, Math.min(100, Math.round(Number(state.adminCategorySettings.liveRewindMaxChannels) || 20)));
  if (typeof state.adminCategorySettings.collapseDuplicateVirtualGroups === "undefined" && typeof state.adminCategorySettings.collapseDuplicateProfileGroups !== "undefined") {
    state.adminCategorySettings.collapseDuplicateVirtualGroups = state.adminCategorySettings.collapseDuplicateProfileGroups;
  }
  state.adminCategorySettings.collapseDuplicateVirtualGroups = state.adminCategorySettings.collapseDuplicateVirtualGroups !== false;
  delete state.adminCategorySettings.collapseDuplicateProfileGroups;
  state.adminCategorySettings.virtualGroupSource = normalizeVirtualGroupSource(state.adminCategorySettings.virtualGroupSource, state.adminCategorySettings.inferChannelNameGroups === true);
  if (state.adminCategorySettings.virtualGroupSource === "profile_group") state.adminCategorySettings.mode = "delimiter";
  state.adminCategorySettings.inferChannelNameGroups = state.adminCategorySettings.virtualGroupSource !== "group";
  state.adminCategorySettings.ecmURL = normalizeAdminECMURL(state.adminCategorySettings.ecmURL);
  state.adminCategorySettings.ecmEnabled = !!state.adminCategorySettings.ecmURL;
  state.adminCategorySettings.categoryRenames = [];
  state.adminCategorySettings.categoryAliases = normalizeCategoryAliases(state.adminCategorySettings.categoryAliases);
  state.adminCategorySettings.eventKeywords = normalizeEventKeywordRows(state.adminCategorySettings.eventKeywords);
  delete state.adminCategorySettings.groupAliases;
  delete state.adminCategorySettings.adminGroups;
  delete state.adminCategorySettings.adminGroupMemberships;
  delete state.adminCategorySettings.presentationOverrides;
}
function normalizeVirtualGroupSource(value, inferLegacy) {
  const mode = String(value || "").trim();
  if (mode === "group" || mode === "group_channel" || mode === "channel" || mode === "profile_group") return mode;
  return inferLegacy ? "group_channel" : "group";
}
function virtualGroupSourceMode() {
  return normalizeVirtualGroupSource(adminSettings().virtualGroupSource, adminSettings().inferChannelNameGroups === true);
}
function useSourceGroupVirtualPaths() {
  return virtualGroupSourceMode() !== "channel";
}
function useChannelNameVirtualPaths() {
  return virtualGroupSourceMode() !== "group";
}
function useProfileGroupVirtualPaths() {
  return virtualGroupSourceMode() === "profile_group";
}
function normalizeCategoryRenames(value) {
  const seen = {};
  return items(value).map(function(rename) {
    return {
      sourcePath: String((rename && rename.sourcePath) || "").trim(),
      displayName: String((rename && (rename.displayName || rename.aliasPath)) || "").trim()
    };
  }).filter(function(rename) {
    const key = lower(rename.sourcePath);
    if (!rename.sourcePath || !rename.displayName || seen[key]) return false;
    seen[key] = true;
    return true;
  });
}
function normalizeCategoryAliases(value) {
  const seen = {};
  return items(value).map(function(alias) {
    return {
      sourcePath: String((alias && alias.sourcePath) || "").trim(),
      aliasPath: String((alias && alias.aliasPath) || "").trim()
    };
  }).filter(function(alias) {
    if (!alias.sourcePath || !alias.aliasPath) return false;
    const key = alias.sourcePath + "\u0000" + alias.aliasPath;
    if (seen[key]) return false;
    seen[key] = true;
    return true;
  });
}
function normalizeEventKeywordRows(value) {
  const defaults = defaultEventKeywordRules();
  const rows = items(value).map(function(row) {
    row = row || {};
    const categoryId = normalizeEventCategoryId(row.categoryId || row.categoryName || "");
    const categoryName = String(row.categoryName || eventCategoryName(categoryId)).trim();
    const keywords = normalizeKeywordList(row.keywords);
    const excludeKeywords = normalizeKeywordList(row.excludeKeywords);
    const eventSeries = row.eventSeries === true;
    const groupWindowMinutes = eventSeries ? Math.max(15, Math.min(360, Number(row.groupWindowMinutes) || 60)) : 0;
    return { categoryId: categoryId, categoryName: categoryName, keywords: keywords, excludeKeywords: excludeKeywords, eventSeries: eventSeries, groupWindowMinutes: groupWindowMinutes };
  }).filter(function(row) { return row.categoryId && row.keywords.length; });
  const byID = {};
  defaults.concat(rows).forEach(function(row) {
    const id = normalizeEventCategoryId(row.categoryId || row.categoryName);
    if (!id) return;
    const existing = byID[id] || { categoryId: id, categoryName: row.categoryName || eventCategoryName(id), keywords: [], excludeKeywords: [], eventSeries: false, groupWindowMinutes: 0 };
    existing.keywords = normalizeKeywordList(existing.keywords.concat(row.keywords || []));
    existing.excludeKeywords = normalizeKeywordList(existing.excludeKeywords.concat(row.excludeKeywords || []));
    existing.eventSeries = existing.eventSeries || row.eventSeries === true;
    existing.groupWindowMinutes = existing.eventSeries ? Math.max(15, Math.min(360, Number(row.groupWindowMinutes || existing.groupWindowMinutes) || 60)) : 0;
    byID[id] = existing;
  });
  return Object.keys(byID).sort(function(left, right) {
    return eventCategoryName(left).localeCompare(eventCategoryName(right));
  }).map(function(id) {
    const row = byID[id];
    if (!row.eventSeries) delete row.groupWindowMinutes;
    return row;
  });
}
function normalizeKeywordList(value) {
  const rows = Array.isArray(value)
    ? value.reduce(function(list, item) { return list.concat(String(item || "").split(/\\n|[\n,]+/)); }, [])
    : String(value || "").split(/\\n|[\n,]+/);
  const seen = {};
  return rows.map(function(item) { return String(item || "").trim(); }).filter(function(item) {
    const key = lower(item);
    if (!key || seen[key]) return false;
    seen[key] = true;
    return true;
  });
}
function normalizeEventCategoryId(value) {
  value = lower(String(value || "").replace(/[^a-z0-9]+/gi, " ")).trim();
  if (value === "award") return "awards";
  if (value === "politics" || value === "political") return "civic";
  if (value === "parade") return "parades";
  if (value === "special" || value === "specials") return "entertainment";
  return value.replace(/\s+/g, "-");
}
function eventCategoryName(categoryId) {
  return ({ awards: "Awards", civic: "Civic", parades: "Parades", entertainment: "Entertainment", golf: "Golf", "motor-racing": "Motor Racing", "combat-sports": "Combat Sports", tennis: "Tennis" })[categoryId] || String(categoryId || "Events");
}
function categoryAliases() {
  return normalizeCategoryAliases(adminSettings().categoryAliases);
}
function renamedCategoryDisplayName(rawName) {
  return categoryDisplayName(rawName);
}
function normalizeAdminECMURL(value) {
  const fallback = "";
  const trimmed = String(value || "").trim();
  const lower = trimmed.toLowerCase();
  if (lower.indexOf("https://") === 0 || lower.indexOf("http://") === 0) return trimmed;
  return fallback;
}
function adminECMEnabled() {
  return !!adminECMURL();
}
function adminECMURL() {
  return normalizeAdminECMURL(adminSettings().ecmURL);
}
function recordWatchPreference(channel) {
  if (!state.app || !state.app.preferences || !channel) return;
  const id = String(channel.id || "");
  if (!id) return;
  const now = Math.floor(Date.now() / 1000);
  const existing = state.app.preferences.continueWatching[id] || {};
  const plays = Number(existing.plays || 0) + 1;
  state.app.preferences.recentChannels = uniqueIDs([id].concat(items(state.app.preferences.recentChannels))).slice(0, 24);
  state.app.preferences.continueWatching[id] = {
    itemKind: "channel",
    itemId: id,
    itemName: channel.name || id,
    playedAt: now,
    plays: plays
  };
  if (plays >= 3 && !favoriteMap()[id]) state.app.preferences.autoFavorites[id] = true;
  normalizePreferences();
  savePrefs();
}
function readRecentSearches() {
  return uniqueIDs(items(prefs().recentSearches).map(function(value) { return String(value || "").trim(); }).filter(Boolean)).slice(0, 12);
}
function writeRecentSearches(searches) {
  state.recentSearches = uniqueIDs(items(searches).map(function(value) { return String(value || "").trim(); }).filter(Boolean)).slice(0, 12);
  if (state.app && state.app.preferences) {
    state.app.preferences.recentSearches = state.recentSearches;
    savePrefs({ quiet: true });
  }
}
function rememberSearch(value) {
  value = String(value || "").trim();
  if (!value) return;
  writeRecentSearches([value].concat(state.recentSearches));
}
function clearRecentSearches() {
  writeRecentSearches([]);
}
function cacheProgramWindow() {
  const now = Math.floor(Date.now() / 1000);
  return { start: now - (2 * 3600), end: now + (30 * 3600) };
}
function compactProgramsForCache(programs, stripSummary) {
  const windowInfo = cacheProgramWindow();
  return items(programs).filter(function(program) {
    const start = Number(program.startUnix || 0);
    const end = Number(program.endUnix || 0);
    return (!end || end >= windowInfo.start) && (!start || start <= windowInfo.end);
  }).map(function(program) {
    const compact = {
      id: program.id,
      channelId: program.channelId,
      title: program.title,
      startUnix: program.startUnix,
      endUnix: program.endUnix
    };
    if (!stripSummary && program.summary) compact.summary = program.summary;
    return compact;
  });
}
function compactAppPayloadForCache(payload, stripSummary, channelsOnly) {
  if (!payload || !items(payload.channels).length) return null;
  return {
    cachedAtUnix: Math.floor(Date.now() / 1000),
    status: payload.status || {},
    source: payload.source || {},
    channels: items(payload.channels),
    categories: items(payload.categories),
    programs: channelsOnly ? [] : compactProgramsForCache(payload.programs, stripSummary),
    vod: { available: !!(payload.vod && payload.vod.available), categories: [], items: [] },
    series: { available: !!(payload.series && payload.series.available), categories: [], items: [] },
    preferences: defaultPrefs(),
    sessions: [],
    capabilities: payload.capabilities || {}
  };
}
function writeLocalAppCache(payload) {
  const variants = [
    compactAppPayloadForCache(payload, false, false),
    compactAppPayloadForCache(payload, true, false),
    compactAppPayloadForCache(payload, true, true)
  ].filter(Boolean);
  for (let index = 0; index < variants.length; index++) {
    try {
      localStorage.removeItem(appCacheKey);
      localStorage.setItem(appCacheKey, JSON.stringify(variants[index]));
      return;
    } catch (_) {}
  }
}
function readLocalAppCache() {
  try {
    const cached = JSON.parse(localStorage.getItem(appCacheKey) || "null");
    if (!cached || !items(cached.channels).length) return null;
    const age = Math.floor(Date.now() / 1000) - Number(cached.cachedAtUnix || 0);
    if (age < 0 || age > 72 * 3600) return null;
    cached.preferences = defaultPrefs();
    cached.sessions = [];
    cached.programs = items(cached.programs);
    cached.categories = items(cached.categories);
    cached.channels = items(cached.channels);
    cached.capabilities = cached.capabilities || {};
    return cached;
  } catch (_) {
    return null;
  }
}
function readSiloPrefsValue(value) {
  if (!value) return null;
  try { return Object.assign(defaultPrefs(), JSON.parse(value)); }
  catch (_) { return null; }
}
function readAdminSettingsValue(value) {
  if (!value) return defaultAdminCategorySettings();
  try {
    if (typeof value === "string") return Object.assign(defaultAdminCategorySettings(), JSON.parse(value));
    if (typeof value === "object") return Object.assign(defaultAdminCategorySettings(), value);
  }
  catch (_) { return defaultAdminCategorySettings(); }
  return defaultAdminCategorySettings();
}
async function loadPluginSettingsValues() {
  if (!pluginInstallationID) return null;
  const payload = await coreGetJSON("/api/v1/settings/plugins/" + encodeURIComponent(pluginInstallationID));
  return payload && payload.values ? payload.values : {};
}
async function loadUserPrefs() {
  const values = await loadPluginSettingsValues();
  return readSiloPrefsValue(values ? values.preferences : "");
}
async function loadAdminCategorySettings() {
  return readAdminSettingsValue(await getJSON(adminSettingsURL()));
}
function adminSettingsURL() {
  return "/dispatcharr/api/admin-settings";
}
async function savePluginSettingValue(key, value) {
  if (!pluginInstallationID) throw new Error("plugin installation settings unavailable");
  const values = await loadPluginSettingsValues().catch(function() { return {}; }) || {};
  values[key] = value;
  await corePutNoContent("/api/v1/settings/plugins/" + encodeURIComponent(pluginInstallationID), { values: values });
}
async function persistAdminCategorySettingsInSilo(settings) {
  if (!pluginInstallationID) throw new Error("plugin installation settings unavailable");
  await corePutNoContent("/api/v1/admin/plugins/installations/" + encodeURIComponent(pluginInstallationID) + "/config", {
    key: "category_settings",
    value: settings
  });
}
function savePrefs(options) {
  if (!state.app || !state.app.preferences) return;
  options = options || {};
  if (pluginInstallationID) {
    state.profileSaveStatus = "saving";
    state.profileSaveMessage = "";
    savePluginSettingValue("preferences", JSON.stringify(state.app.preferences)).then(function() {
      state.profileSaveStatus = "saved";
      state.profileSaveMessage = "Saved to your Silo profile.";
      if (state.view === "settings") renderSettings();
    }).catch(function(error) {
      state.profileSaveStatus = "error";
      state.profileSaveMessage = "Could not save to your Silo profile.";
      if (!options.quiet) showAppToast(state.profileSaveMessage);
      if (state.view === "settings") renderSettings();
      try { console.warn("Dispatcharr profile preference save failed", error); } catch (_) {}
    });
  } else {
    state.profileSaveStatus = "error";
    state.profileSaveMessage = "Could not save to your Silo profile.";
    if (!options.quiet) showAppToast(state.profileSaveMessage);
  }
}
function saveAdminCategorySettings() {
  state.adminCategorySettings = Object.assign(defaultAdminCategorySettings(), state.adminCategorySettings || {});
  normalizeAdminCategorySettings();
  state.adminSaveStatus = "saving";
  state.adminSaveMessage = "Saving...";
  if (state.view === "admin") renderAdminPage();
  postJSON(adminSettingsURL(), state.adminCategorySettings).then(function(saved) {
    state.adminCategorySettings = readAdminSettingsValue(saved);
    normalizeAdminCategorySettings();
    return persistAdminCategorySettingsInSilo(state.adminCategorySettings);
  }).then(function() {
    state.savedAdminCategorySettings = cloneAdminCategorySettings(state.adminCategorySettings);
    state.adminSaveStatus = "saved";
    state.adminSaveMessage = "Saved plugin settings.";
    if (state.view === "admin") renderAdminPage();
  }).catch(function(error) {
    state.adminSaveStatus = "error";
    state.adminSaveMessage = "Could not save plugin settings: " + readableError(error);
    if (state.view === "admin") renderAdminPage();
    try { console.warn("Dispatcharr admin plugin settings save failed", error); } catch (_) {}
  });
}
function discardAdminCategorySettings() {
  state.adminCategorySettings = cloneAdminCategorySettings(state.savedAdminCategorySettings);
  state.adminSaveStatus = "idle";
  state.adminSaveMessage = "";
  if (state.category.indexOf("virtual:") === 0 && !categoryName(state.category)) state.category = "";
  renderAdminPage();
}
async function getJSON(url) {
  const response = await fetch(route(url), { credentials: "include" });
  if (!response.ok) throw await requestError(response);
  return response.json();
}
async function postJSON(url, body) {
  const response = await fetch(route(url), { method: "POST", credentials: "include", headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
  if (!response.ok) throw await requestError(response);
  return response.json();
}
async function coreGetJSON(url) {
  const response = await fetch(url, { credentials: "include" });
  if (!response.ok) throw await requestError(response);
  return response.json();
}
async function corePutNoContent(url, body) {
  const response = await fetch(url, { method: "PUT", credentials: "include", headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
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
  const settings = adminSettings();
  const delimiterEnabled = settings.mode === "delimiter";
  return { enabled: delimiterEnabled, mode: delimiterEnabled ? "delimiter" : "off", delimiter: settings.delimiter || "pipe", regex: "", output: "" };
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
function isWorldCupReplayGroup(value) {
  return normalizedGroupName(value) === "world cup replays";
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
function guideChannelMatchesQuery(channel) {
  if (!state.query || channelMatchesQuery(channel)) return true;
  return programsFor(channel.id).some(programMatchesQuery);
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
function programsFor(channelID) {
  const now = Math.floor(Date.now() / 1000);
  const allowed = selectedProfileChannelMap();
  if (channelID && allowed && !allowed[channelID]) return [];
  const source = channelID ? items(state.programsByChannel[channelID]) : items(state.sortedPrograms);
  return source.filter(function(program) {
    return (!allowed || !!allowed[program.channelId]) && (!channelID || program.channelId === channelID) && (!program.endUnix || program.endUnix >= now - 3600);
  });
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
function guideSlotStart() {
  const now = Math.floor(Date.now() / 1000);
  return Math.floor(now / 1800) * 1800;
}
function guideSlots() {
  const start = guideSlotStart();
  const slots = [];
  for (let index = 0; index < 50; index++) slots.push(start + index * 1800);
  return slots;
}
function guideTimelineStyle(slots) {
  return "--epg-slots: " + slots.length + "; --epg-width: " + (slots.length * 11.25) + "rem;";
}
function guideWindow() {
  const start = guideSlotStart();
  return { start: start, end: start + (25 * 3600), slotCount: 50 };
}
function epgCellStyle(startUnix, endUnix, windowInfo) {
  const start = Math.max(startUnix || windowInfo.start, windowInfo.start);
  const end = Math.min(endUnix || start + 1800, windowInfo.end);
  const leftSlots = (start - windowInfo.start) / 1800;
  const widthSlots = Math.max((end - start) / 1800, 0);
  return "left: calc(" + leftSlots.toFixed(4) + " * var(--epg-slot)); width: calc(" + widthSlots.toFixed(4) + " * var(--epg-slot) - 0.0625rem);";
}
function stopPlayback() {
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
function stopCurrentWatch(reason) {
  if (!state.currentSession) return;
  postJSON("/dispatcharr/api/watch/stop", { sessionId: state.currentSession.id, reason: reason || "stop" }).catch(function() {});
  state.currentSession = null;
  if (state.heartbeat) {
    clearInterval(state.heartbeat);
    state.heartbeat = null;
  }
}
function multiviewTileKey(channelID) {
  return "mv-" + String(channelID || "").replace(/[^A-Za-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 48) + "-" + Math.random().toString(36).slice(2, 8);
}
function multiviewTileByID(tileID) {
  return items(state.multiviewTiles).find(function(tile) { return tile.id === tileID; }) || null;
}
function destroyMultiviewMedia(tile) {
  if (!tile) return;
  if (tile.hls) { tile.hls.destroy(); tile.hls = null; }
  if (tile.tsPlayer) { tile.tsPlayer.destroy(); tile.tsPlayer = null; }
  tile.attached = false;
}
function resetMultiviewMedia() {
  items(state.multiviewTiles).forEach(destroyMultiviewMedia);
}
function syncMultiviewAudio() {
  if (!items(state.multiviewTiles).length) state.multiviewActiveTileID = "";
  if (!state.multiviewActiveTileID && state.multiviewTiles[0]) state.multiviewActiveTileID = state.multiviewTiles[0].id;
  items(state.multiviewTiles).forEach(function(tile) {
    const video = byId(tile.videoID);
    const active = tile.id === state.multiviewActiveTileID;
    if (video) {
      video.muted = !active;
      video.volume = active ? state.volume : 0;
    }
    const root = document.querySelector("[data-multiview-tile=\"" + cssEscape(tile.id) + "\"]");
    if (root) root.classList.toggle("active", active);
  });
}
function startMultiviewHeartbeat() {
  if (state.multiviewHeartbeat) clearInterval(state.multiviewHeartbeat);
  state.multiviewHeartbeat = setInterval(function() {
    items(state.multiviewTiles).forEach(function(tile) {
      if (tile.session) postJSON("/dispatcharr/api/watch/heartbeat", { sessionId: tile.session.id }).catch(function() {});
    });
  }, 30000);
}
function startMultiviewWatch(tile) {
  if (!tile || tile.session || !tile.channel) return;
  recordWatchPreference(tile.channel);
  postJSON("/dispatcharr/api/watch/start", { itemKind: "channel", itemId: tile.channel.id, itemName: tile.channel.name }).then(function(payload) {
    tile.session = payload.session;
    startMultiviewHeartbeat();
    renderRail();
  }).catch(function() {});
}
function stopMultiviewWatch(tile, reason) {
  if (!tile || !tile.session) return;
  postJSON("/dispatcharr/api/watch/stop", { sessionId: tile.session.id, reason: reason || "stop" }).catch(function() {});
  tile.session = null;
}
function stopAllMultiview(reason) {
  items(state.multiviewTiles).forEach(function(tile) {
    destroyMultiviewMedia(tile);
    stopMultiviewWatch(tile, reason || "stop_multiview");
  });
  state.multiviewTiles = [];
  state.multiviewActiveTileID = "";
  if (state.multiviewHeartbeat) {
    clearInterval(state.multiviewHeartbeat);
    state.multiviewHeartbeat = null;
  }
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
}
function setCategory(id) {
  if ((id || "") !== state.category) state.folderQuery = "";
  state.category = id || "";
  state.view = id ? "live" : "home";
  render();
}
async function hydrateApp(payload, options) {
  options = options || {};
  const previousApp = state.app || {};
  payload = payload || {};
  payload.programs = Array.isArray(payload.programs) ? payload.programs : items(previousApp.programs);
  payload.vod = payload.vod || previousApp.vod || { available: false, categories: [], items: [] };
  payload.series = payload.series || previousApp.series || { available: false, categories: [], items: [] };
  state.app = payload;
  if (options.localCache) {
    state.app.preferences = defaultPrefs();
    state.adminCategorySettings = defaultAdminCategorySettings();
  } else if (options.reuseSettings) {
    state.app.preferences = previousApp.preferences || defaultPrefs();
  } else {
    const values = await loadPluginSettingsValues().catch(function() { return null; });
    const siloPrefs = readSiloPrefsValue(values && values.preferences ? values.preferences : "");
    state.app.preferences = mergePrefs(siloPrefs || state.app.preferences);
    const savedAdminSettings = values && values[adminSettingsKey] ? readAdminSettingsValue(values[adminSettingsKey]) : null;
    state.adminCategorySettings = await loadAdminCategorySettings().catch(function() {
      return savedAdminSettings || defaultAdminCategorySettings();
    });
  }
  state.savedAdminCategorySettings = cloneAdminCategorySettings(state.adminCategorySettings);
  state.app.programs = items(state.app.programs);
  state.recentSearches = readRecentSearches();
  rebuildProgramIndex();
  normalizePreferences();
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
async function refreshSupplementalData(includeContent) {
  if (!state.app) return;
  const requests = [getJSON("/dispatcharr/api/guide").catch(function(error) {
    try { console.warn("Dispatcharr guide load failed", error); } catch (_) {}
    return null;
  })];
  if (includeContent) {
    requests.push(getJSON("/dispatcharr/api/vod").catch(function() { return null; }));
    requests.push(getJSON("/dispatcharr/api/series").catch(function() { return null; }));
  }
  const payloads = await Promise.all(requests);
  if (payloads[0]) state.app.programs = items(payloads[0].programs);
  if (includeContent && payloads[1]) state.app.vod = payloads[1];
  if (includeContent && payloads[2]) state.app.series = payloads[2];
  rebuildProgramIndex();
  writeLocalAppCache(state.app);
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
function guideHasPrograms() {
  return items(state.app && state.app.programs).length > 0;
}
function epgLastSuccessUnix() {
  const status = state.app && state.app.status ? state.app.status : {};
  return Number(status.epgLastSuccessUnix || 0);
}
function guideUpdatedUnix() {
  const status = state.app && state.app.status ? state.app.status : {};
  return epgLastSuccessUnix() || Number(status.lastSuccessUnix || 0);
}
function guideFreshnessHTML() {
  if (state.refreshing) {
    return "<span class=\"guide-freshness is-refreshing\" role=\"status\" aria-live=\"polite\" aria-atomic=\"true\">Refreshing guide...</span>";
  }
  const unix = guideUpdatedUnix();
  const title = unix ? dateTimeLabel(unix) : "Guide has not synced yet";
  const stale = !unix || Math.floor(Date.now() / 1000) - unix > 6 * 3600;
  return "<span class=\"guide-freshness" + (stale ? " is-stale" : "") + "\" title=\"" + escapeHTML(title) + "\">" + escapeHTML(relativeUpdatedLabel(unix)) + "</span>";
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
function guideRefreshAdvanced(previousEPGSuccess) {
  const previous = Number(previousEPGSuccess || 0);
  return epgLastSuccessUnix() > previous || (!previous && guideHasPrograms());
}
function guideNeedsFollowupRefresh(previousEPGSuccess) {
  const status = state.app && state.app.status ? state.app.status : {};
  const epgStatus = String(status.epgStatus || "").toLowerCase();
  const refreshState = String((status.refresh || {}).state || "").toLowerCase();
  return refreshState === "queued" || refreshState === "running" || epgStatus === "loading" || !guideRefreshAdvanced(previousEPGSuccess);
}
async function pollGuideRefresh(previousEPGSuccess) {
  for (let attempt = 0; attempt < 300 && guideNeedsFollowupRefresh(previousEPGSuccess); attempt++) {
    await new Promise(function(resolve) { setTimeout(resolve, 2000); });
    await refreshStatusData();
    const status = state.app && state.app.status ? state.app.status : {};
    const refresh = status.refresh || {};
    const refreshState = String(refresh.state || "").toLowerCase();
    if (refreshState === "failed" || refreshState === "canceled") throw new Error(refresh.error || "guide refresh did not complete");
    if (refreshState === "succeeded") return;
    if (String(status.epgStatus || "").toLowerCase() === "failed") break;
  }
  if (guideNeedsFollowupRefresh(previousEPGSuccess)) throw new Error("guide refresh timed out");
}
function updateGuideFreshnessBlock() {
  const freshness = document.querySelector(".guide-freshness");
  if (freshness) freshness.outerHTML = guideFreshnessHTML();
}
function refreshVisibleGuideBlock() {
  updateGuideFreshnessBlock();
  if (state.view === "guide") {
    const guideScroll = byId("guide-scroll");
    const scrollLeft = guideScroll ? guideScroll.scrollLeft : 0;
    const scrollTop = guideScroll ? guideScroll.scrollTop : 0;
    resetGuideRows();
    renderEPG();
    if (guideScroll) {
      guideScroll.scrollLeft = scrollLeft;
      guideScroll.scrollTop = scrollTop;
    }
    return;
  }
  render();
}
function setGuideRefreshButtonsLoading(loading) {
  Array.prototype.slice.call(document.querySelectorAll("[data-guide-refresh]")).forEach(function(button) {
    button.classList.toggle("is-loading", !!loading);
    button.disabled = !!loading;
    button.setAttribute("aria-busy", loading ? "true" : "false");
  });
}
async function refreshGuideBlockData() {
  if (state.refreshing) return;
  const previousEPGSuccess = epgLastSuccessUnix();
  state.refreshing = true;
  setGuideRefreshButtonsLoading(true);
  refreshVisibleGuideBlock();
  showAppToast("Refreshing guide...");
  try {
    await hydrateApp(await postJSON("/dispatcharr/api/refresh", {}), { reuseSettings: true });
    refreshVisibleGuideBlock();
    if (guideNeedsFollowupRefresh(previousEPGSuccess)) {
      await pollGuideRefresh(previousEPGSuccess);
    }
    await refreshSupplementalData(true);
    state.recordings = null;
    refreshVisibleGuideBlock();
    showAppToast(guideRefreshAdvanced(previousEPGSuccess) ? "Guide refreshed from Dispatcharr." : "Guide refresh finished without newer EPG data.");
  } catch (error) {
    showAppToast("Dispatcharr refresh failed.");
  } finally {
    state.refreshing = false;
    setGuideRefreshButtonsLoading(false);
    refreshVisibleGuideBlock();
  }
}
function renderRail() {
  document.querySelectorAll("[data-view]").forEach(function(button) {
    const unavailable = button.dataset.view === "recordings" && !dvrEnabled();
    const activeViews = String(button.dataset.activeViews || button.dataset.view || "").split(/\s+/).filter(Boolean);
    button.hidden = unavailable;
    const active = !unavailable && activeViews.indexOf(state.view) !== -1;
    button.classList.toggle("active", active);
    if (active) button.setAttribute("aria-current", "page");
    else button.removeAttribute("aria-current");
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
  if (channel && channel.logoUrl) return "<img class=\"logo\" src=\"" + escapeHTML(channel.logoUrl) + "\" alt=\"\" onerror=\"this.hidden = true; this.nextElementSibling.hidden = false;\">" + fallback;
  return fallback;
}
function renderGuideChannelButton(channel) {
  const channelName = channel.name || "Untitled";
  return "<button class=\"epg-channel\" data-channel=\"" + escapeHTML(channel.id) + "\" data-channel-name=\"" + escapeHTML(channelName) + "\" aria-label=\"" + escapeHTML(channelName) + "\" title=\"" + escapeHTML(channelName) + "\">" + logoHTML(channel) + "<span class=\"epg-channel-title\">" + escapeHTML(channelName) + "</span></button>";
}
function render() {
  if (!state.app) return;
  if (state.view === "recordings" && !dvrEnabled()) state.view = "home";
  if (state.view === "admin" && !isAdminRoute) state.view = "home";
  document.querySelector(".shell").classList.toggle("is-player", state.view === "player");
  document.querySelector(".shell").classList.toggle("is-guide", state.view === "guide");
  document.querySelector(".shell").classList.toggle("is-sports", state.view === "sports");
  document.querySelector(".shell").classList.toggle("is-events", state.view === "events");
  document.querySelector(".shell").classList.toggle("is-multiview", state.view === "multiview");
  document.querySelector(".shell").classList.toggle("is-search", state.view === "search");
  document.querySelector(".shell").classList.toggle("is-onlater", state.view === "onlater");
  renderRail();
  renderSportsTopbarTabs();
  if (state.view === "guide") renderGuidePage();
  else if (state.view === "player") renderPlayerPage();
  else if (state.view === "multiview") renderMultiviewPage();
  else if (state.view === "live" || state.view === "favorites") renderLivePage();
  else if (state.view === "sports") renderSportsPage();
  else if (state.view === "events") renderEventsPage();
  else if (state.view === "onlater") renderOnLaterPage();
  else if (state.view === "search") renderSearchPage();
  else if (state.view === "recordings") renderRecordingsPage();
  else if (state.view === "admin") renderAdminPage();
  else if (state.view === "settings") renderSettings();
  else renderHome();
}
function guideSearchFocused() {
  return document.activeElement && document.activeElement.id === "guide-search";
}
function startGuideAutoRefresh() {
  if (isAdminRoute || state.guideAutoTimer) return;
  state.guideAutoTimer = setInterval(tickGuideAutoRefresh, 60000);
  document.addEventListener("visibilitychange", function() {
    if (!document.hidden) tickGuideAutoRefresh();
  });
}
async function tickGuideAutoRefresh() {
  if (!state.app || state.view !== "guide" || document.hidden) return;
  const slotStart = guideSlotStart();
  if (!state.guideLastSlotStart) state.guideLastSlotStart = slotStart;
  if (slotStart !== state.guideLastSlotStart && !guideSearchFocused()) {
    state.guideLastSlotStart = slotStart;
    renderGuidePage();
  }

  const now = Date.now();
  if (state.guideAutoFetching || state.refreshing || now - state.guideLastAutoFetchAt < 5 * 60 * 1000) return;
  state.guideAutoFetching = true;
  state.guideLastAutoFetchAt = now;
  try {
    await refreshStatusData();
    await refreshSupplementalData(false);
    if (state.view !== "guide") return;
    if (guideSearchFocused()) {
      resetGuideRows();
      renderEPG();
    } else {
      renderGuidePage();
    }
  } catch (error) {
    try { console.warn("Dispatcharr guide auto-refresh failed", error); } catch (_) {}
  } finally {
    state.guideAutoFetching = false;
  }
}
function renderHome() {
  const root = byId("view");
  const recent = recentChannels(5);
  const watched = recent.length ? recent : visibleChannels(false).slice(0, 5);
  const favorites = homeFavoriteChannels();
  root.innerHTML = sectionHeader("Recently watched")
    + rowCards(watched)
    + (favorites.length ? sectionHeader("Favorites") + favoriteHomeCards(favorites) : "")
    + sectionHeaderWithActions("TV Guide", guideFreshnessHTML())
    + renderHomeGuide(homeGuideChannels(watched), "No current guide data for recently watched channels.", { hideFreshness: true })
    + categoryGrid();
}
function emptyStateHTML(title, detail) {
  detail = String(detail || "").trim();
  return "<div class=\"empty\"><strong>" + escapeHTML(title) + "</strong>" + (detail ? "<div class=\"muted\">" + escapeHTML(detail) + "</div>" : "") + "</div>";
}
function catalogEmptyDetail() {
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
  return effectiveChannels(false).filter(function(channel) {
    return !(channel.categoryId && hidden[channel.categoryId]);
  });
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
function guideUnavailableLabel() {
  return "No Guide Data Available";
}
function programIsGuidePlaceholder(program) {
  const title = normalizeProgramTitle(program && program.title);
  return /^(no games? today|data not available|no (guide )?(data|information|programming)( available)?|programming unavailable|information not available)$/.test(title);
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
function programLooksSports(program) {
  if (programIsGuidePlaceholder(program)) return false;
  return /\b(vs\.?|@|game|match|cup|league|racing|football|baseball|basketball|soccer|hockey|f1|formula|mlb|nba|nfl|nhl|mls|wnba|premier)\b/i.test(programSearchText(program));
}
function programLooksMovie(program) {
  if (programIsGuidePlaceholder(program)) return false;
  return /\b(movie|film|premiere|cinema|starring)\b/i.test(programSearchText(program));
}
function programLooksEvent(program) {
  if (programIsGuidePlaceholder(program)) return false;
  return /\b(awards|parade|special|ceremony|debate|concert|festival|live)\b/i.test(programSearchText(program));
}
function groupedUpcomingAirings(programs, query) {
  const groups = {};
  items(programs).filter(programIsUpcoming).forEach(function(program) {
    if (programIsGuidePlaceholder(program)) return;
    const key = normalizeProgramTitle(program.title);
    if (!key || (query && key.indexOf(query) === -1 && lower(programSearchText(program)).indexOf(query) === -1)) return;
    groups[key] = groups[key] || { key: key, title: program.title || "Untitled", programs: [] };
    groups[key].programs.push(program);
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
  return [
    { id: "all", label: "All" },
    { id: "channels", label: "Channels" },
    { id: "groups", label: "Groups" },
    { id: "programs", label: "Programs" },
    { id: "airings", label: "Upcoming Airings" },
    { id: "sports", label: "Sports" },
    { id: "events", label: "Events" },
    { id: "movies", label: "Movies" },
    { id: "shows", label: "Shows" },
    { id: "recordings", label: "Recordings" }
  ];
}
function searchResultSections(query) {
  const filter = state.searchType || "all";
  const include = function(id) { return filter === "all" || filter === id; };
  const sections = [];
  if (include("channels")) {
    const channels = searchableChannels().filter(function(channel) { return channelMatchesSearch(channel, query); }).slice(0, 18);
    sections.push({ id: "channels", title: "Channels", rows: channels.map(function(channel) {
      return {
        attrs: "data-search-channel=\"" + escapeHTML(channel.id) + "\"",
        art: logoHTML(channel),
        title: channel.name || "Untitled",
        meta: ["Channel", channel.categoryName || "Live TV"].filter(Boolean).join(" - "),
        action: "Watch"
      };
    }) });
  }
  if (include("groups")) {
    const groups = allDiscoveryGroups().filter(function(group) { return groupMatchesSearch(group, query); }).slice(0, 18);
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
    const programs = programsFor("").filter(function(program) { return programMatchesSearch(program, query); }).slice(0, 18);
    sections.push({ id: "programs", title: "Guide Programs", rows: programs.map(function(program) {
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
    const airings = groupedUpcomingAirings(programsFor(""), query).slice(0, 12);
    sections.push({ id: "airings", title: "Upcoming Airings", rows: airings.map(function(group) {
      const first = group.programs[0] || {};
      const channel = channelByID(first.channelId) || {};
      return {
        attrs: "data-search-airing=\"" + escapeHTML(group.key) + "\"",
        art: logoHTML(channel),
        title: group.title,
        meta: [group.programs.length + " airings", dateTimeLabel(first.startUnix), channel.name || ""].filter(Boolean).join(" - "),
        action: "Show"
      };
    }) });
  }
  if (include("sports")) {
    const programs = programsFor("").filter(function(program) { return programLooksSports(program) && programMatchesSearch(program, query); }).slice(0, 12);
    sections.push({ id: "sports", title: "Sports From Guide", rows: programs.map(function(program) {
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
    const programs = programsFor("").filter(function(program) { return programLooksEvent(program) && programMatchesSearch(program, query); }).slice(0, 12);
    sections.push({ id: "events", title: "Events From Guide", rows: programs.map(function(program) {
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
    const vodMovies = items(state.app && state.app.vod && state.app.vod.items).filter(function(item) { return contentMatchesSearch("vod", item, query); }).slice(0, 8);
    const guideMovies = programsFor("").filter(function(program) { return programLooksMovie(program) && programMatchesSearch(program, query); }).slice(0, 8);
    sections.push({ id: "movies", title: "Movies", rows: vodMovies.map(function(item) {
      return {
        attrs: "",
        disabled: true,
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
    const shows = items(state.app && state.app.series && state.app.series.items).filter(function(item) { return contentMatchesSearch("series", item, query); }).slice(0, 12);
    sections.push({ id: "shows", title: "Shows", rows: shows.map(function(item) {
      return {
        attrs: "",
        disabled: true,
        art: item.posterUrl ? "<img src=\"" + escapeHTML(item.posterUrl) + "\" alt=\"\">" : "<span class=\"logo logo-fallback\">TV</span>",
        title: item.name || "Untitled show",
        meta: ["Show", contentCategoryName("series", item), item.releaseDate].filter(Boolean).join(" - "),
        action: "On Demand"
      };
    }) });
  }
  if (include("recordings") && state.recordings && state.recordings.available) {
    const recordings = normalizeRecordings(state.recordings).filter(function(recording) { return recordingMatchesSearch(recording, query); }).slice(0, 12);
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
function renderSearchResultRow(row) {
  const record = row.recordable ? "<button class=\"search-result-record\" type=\"button\" data-schedule-channel=\"" + escapeHTML(row.channelId || "") + "\" data-schedule-program=\"" + escapeHTML(row.programId || "") + "\">Record</button>" : "";
  return "<div class=\"search-result-row\"><button class=\"search-result\" type=\"button\" " + (row.attrs || "") + (row.disabled ? " disabled" : "") + "><span class=\"search-result-art\">" + row.art + "</span><span class=\"search-result-main\"><strong>" + escapeHTML(row.title) + "</strong><small>" + escapeHTML(row.meta || "") + "</small></span><span class=\"search-result-action\">" + escapeHTML(row.action || "") + "</span></button>" + record + "</div>";
}
function renderSearchResultCard(row) {
  const record = row.recordable ? "<button class=\"search-result-record\" type=\"button\" data-schedule-channel=\"" + escapeHTML(row.channelId || "") + "\" data-schedule-program=\"" + escapeHTML(row.programId || "") + "\">Record</button>" : "";
  return "<div class=\"search-result-card\"><button class=\"search-result-card-main\" type=\"button\" " + (row.attrs || "") + (row.disabled ? " disabled" : "") + "><span class=\"search-result-card-art\">" + row.art + "</span><span class=\"search-result-card-copy\"><strong>" + escapeHTML(row.title) + "</strong><small>" + escapeHTML(row.meta || "") + "</small></span><span class=\"search-result-card-action\">" + escapeHTML(row.action || "") + "</span></button>" + record + "</div>";
}
function renderSearchResults(query) {
  const sections = searchResultSections(query);
  const savePass = query && !keywordPasses().some(function(pass) { return lower(pass.keyword) === lower(query); }) ? "<button class=\"search-save-pass\" type=\"button\" data-keyword-pass-add=\"" + escapeHTML(query) + "\">Save Keyword Pass</button>" : "";
  if (!sections.length) return "<div class=\"search-empty\">No matches found." + savePass + "</div>";
  return (savePass ? "<div class=\"search-pass-action\">" + savePass + "</div>" : "") + "<div class=\"search-results\">" + sections.map(function(section) {
    return sectionHeader(section.title) + "<div class=\"search-result-list\">" + section.rows.map(renderSearchResultRow).join("") + "</div>";
  }).join("") + "</div>";
}
const SEARCH_RESULTS_DELAY_MS = 180;
let searchResultsTimer = null;
function clearSearchResultsTimer() {
  if (!searchResultsTimer) return;
  clearTimeout(searchResultsTimer);
  searchResultsTimer = null;
}
function renderSearchPageResults() {
  return searchNeedle() ? renderSearchResults(searchNeedle()) : renderSearchStart();
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
function refreshGuideRowsForQuery() {
  if (state.view !== "guide" || !byId("epg")) return false;
  resetGuideRows();
  renderEPG();
  return true;
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
  const categoryHTML = sectionHeader("Categories") + "<div class=\"search-category-grid\">" + [
    { id: "channels", label: "Channels", icon: "guide" },
    { id: "groups", label: "Groups", icon: "multiview" },
    { id: "programs", label: "Programs", icon: "search" },
    { id: "airings", label: "Upcoming Airings", icon: "guide" },
    { id: "sports", label: "Sports", icon: "search" },
    { id: "events", label: "Events", icon: "guide" },
    { id: "movies", label: "Movies", icon: "play" },
    { id: "shows", label: "Shows", icon: "multiview" }
  ].map(function(item) {
    return "<button class=\"search-category-tile\" type=\"button\" data-search-type=\"" + escapeHTML(item.id) + "\">" + icon(item.icon) + "<strong>" + escapeHTML(item.label) + "</strong></button>";
  }).join("") + "</div>";
  const browsed = recentChannels(10);
  const browsedHTML = browsed.length ? sectionHeader("Recently browsed") + rowCards(browsed) : "";
  return recentHTML + passHTML + categoryHTML + browsedHTML;
}
function renderSearchPage() {
  const root = byId("view");
  const query = state.searchQuery || "";
  const filter = state.searchType || "all";
  const filterHTML = "<div class=\"search-chip-row\">" + searchFilters().map(function(item) {
    return "<button class=\"search-chip" + (filter === item.id ? " active" : "") + "\" type=\"button\" data-search-type=\"" + escapeHTML(item.id) + "\">" + escapeHTML(item.label) + "</button>";
  }).join("") + "</div>";
  root.innerHTML = "<div class=\"search-page\"><div class=\"search-hero\"><h2>Search</h2><div class=\"search-form\"><input id=\"search-page-input\" class=\"search-field\" value=\"" + escapeHTML(query) + "\" placeholder=\"Search movies, tv shows, channels and more\" autocomplete=\"off\"><button class=\"search-cancel\" type=\"button\" data-search-cancel=\"true\">Cancel</button></div></div>" + filterHTML + "<div id=\"search-page-results\" class=\"search-page-results\">" + renderSearchPageResults() + "</div></div>";
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
  showAppToast("Keyword Pass saved.");
  renderSearchPage();
}
function removeKeywordPass(id) {
  id = String(id || "");
  if (!state.app || !state.app.preferences) return;
  state.app.preferences.keywordPasses = keywordPasses().filter(function(pass) { return pass.id !== id; });
  savePrefs();
  renderOnLaterPage();
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
function onLaterPrograms() {
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
    if (type === "sports" && !programLooksSports(program)) return false;
    if (type === "events" && !programLooksEvent(program)) return false;
    if (type === "movies" && !programLooksMovie(program)) return false;
    if (type === "passes" && !keywordPasses().some(function(pass) { return lower(programSearchText(program)).indexOf(lower(pass.keyword)) !== -1; })) return false;
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
  const filterButton = function(item, group, label) {
    const active = (group === "time" ? time : type) === item.id;
    return "<button class=\"search-chip" + (active ? " active" : "") + "\" type=\"button\" data-onlater-" + group + "=\"" + escapeHTML(item.id) + "\" aria-pressed=\"" + (active ? "true" : "false") + "\">" + escapeHTML(label || item.label) + "</button>";
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
function loadSports(force) {
  if (state.sportsLoading) return Promise.resolve(state.sports || { events: [], leagues: [] });
  if (state.sports && !force) return Promise.resolve(state.sports);
  state.sportsLoading = true;
  return getJSON("/dispatcharr/api/sports" + (force ? "?refresh=1" : "")).then(function(payload) {
    state.sports = payload || { events: [], leagues: [] };
    delete state.sports.error;
    applySportsFavoritesToPayload();
    return state.sports;
  }).catch(function(error) {
    if (!state.sports) state.sports = { events: [], leagues: [], error: readableError(error) };
    else state.sports.error = readableError(error);
    showAppToast("Could not refresh sports.");
    return state.sports;
  }).finally(function() {
    state.sportsLoading = false;
    if (state.view === "sports") renderSportsPage();
    if (state.view === "player" && state.playerSportsOpen) renderPlayerSportsDrawer();
  });
}
function applySportsFavoritesToPayload() {
  const favorites = sportsFavoriteTeamMap();
  items(state.sports && state.sports.events).forEach(function(event) {
    if (event.home) event.home.favorite = !!favorites[event.home.id];
    if (event.away) event.away.favorite = !!favorites[event.away.id];
  });
}
function sportsTabLabel(tab) {
  return ({ live: "Live", upcoming: "Upcoming", favorites: "Favorites", all: "All" })[tab] || "Live";
}
function sportsTabButtonsHTML() {
  return ["live", "upcoming", "favorites", "all"].map(function(tab) {
    return "<button type=\"button\" data-sports-tab=\"" + tab + "\" class=\"" + (state.sportsTab === tab ? "active" : "") + "\" aria-pressed=\"" + (state.sportsTab === tab ? "true" : "false") + "\">" + escapeHTML(sportsTabLabel(tab)) + "</button>";
  }).join("");
}
function renderSportsTopbarTabs() {
  const root = byId("sports-topbar-tabs");
  if (!root) return;
  root.innerHTML = "";
}
function renderSportsTabFilters() {
  const refreshClass = "sports-refresh" + (state.sportsLoading ? " is-loading" : "");
  const refreshDisabled = state.sportsLoading ? " disabled aria-busy=\"true\"" : "";
  return "<div class=\"sports-filter-row\"><div class=\"view-toggle\" aria-label=\"Sports filter\">" + sportsTabButtonsHTML() + "</div>"
    + "<button type=\"button\" class=\"" + refreshClass + "\" data-sports-refresh=\"true\"" + refreshDisabled + ">" + icon("loader") + "<span>Refresh scores</span></button></div>";
}
function renderSportsPage() {
  const root = byId("view");
  if (!state.sports && !state.sportsLoading) loadSports(false);
  renderSportsTopbarTabs();
  const payload = state.sports || { events: [], leagues: [] };
  const events = filteredSportsEvents(payload);
  root.innerHTML = "<div class=\"sports-page\"><div class=\"sports-pinned\">" + renderSportsTabFilters() + renderSportsLeagueFilters(payload)
    + recoveryPanelHTML(payload.error, "sports")
    + (state.sportsLoading && !events.length ? "<div class=\"empty\">Loading sports...</div>" : "")
    + "</div><div class=\"sports-score-scroll\">"
    + (events.length ? "<div class=\"sports-board\">" + events.map(renderSportsEventCard).join("") + "</div>" : (!state.sportsLoading ? "<div class=\"empty\">No sports matches.</div>" : ""))
    + "</div>"
    + "</div>";
}
function renderSportsLeagueFilters(payload) {
  const leagues = items(payload && payload.leagues);
  if (!leagues.length) return "";
  const chips = ["<button class=\"chip" + (!state.sportsLeague ? " active" : "") + "\" data-sports-league=\"\">All leagues</button>"].concat(leagues.map(function(league) {
    const label = league.name || league.id || "League";
    return "<button class=\"chip" + (state.sportsLeague === league.id ? " active" : "") + "\" data-sports-league=\"" + escapeHTML(league.id) + "\">" + escapeHTML(label) + "</button>";
  }));
  return "<div class=\"sports-leagues\">" + chips.join("") + "</div>";
}
function filteredSportsEvents(payload) {
  const now = Math.floor(Date.now() / 1000);
  return items(payload && payload.events).filter(function(event) {
    if (!profileSelectionIsAll() && !uniqueEventChannels(event.channels).length) return false;
    if (state.sportsLeague && event.leagueId !== state.sportsLeague) return false;
    if (state.sportsTab === "live" && !event.live) return false;
    const startUnix = Number(event.startUnix || 0);
    if (state.sportsTab === "upcoming" && (event.completed || event.live || (startUnix > 0 && startUnix < now - 3600))) return false;
    if (state.sportsTab === "favorites" && !sportsEventHasFavoriteTeam(event)) return false;
    return true;
  }).sort(compareSportsEventsForTab);
}
function compareSportsEventsForTab(left, right) {
  const tab = state.sportsTab || "live";
  if (left.live !== right.live) return left.live ? -1 : 1;
  if (tab === "upcoming") {
    const leftStart = sportsEventStartSort(left, 1);
    const rightStart = sportsEventStartSort(right, 1);
    if (leftStart !== rightStart) return leftStart - rightStart;
  } else {
    const leftRecent = sportsEventStartSort(left, 0);
    const rightRecent = sportsEventStartSort(right, 0);
    if (leftRecent !== rightRecent) return rightRecent - leftRecent;
  }
  return String(left.name || left.shortName || "").localeCompare(String(right.name || right.shortName || ""));
}
function sportsEventStartSort(event, fallback) {
  const start = Number(event && event.startUnix || 0);
  return start > 0 ? start : fallback;
}
function sportsEventHasFavoriteTeam(event) {
  const favorites = sportsFavoriteTeamMap();
  return !!(favorites[(event.home || {}).id] || favorites[(event.away || {}).id]);
}
function sportsEventMatchesQuery(event) {
  const channels = items(event.channels).map(function(channel) { return [channel.name, channel.categoryName, channel.reason].join(" "); }).join(" ");
  const text = [event.name, event.shortName, event.leagueName, event.statusText, (event.home || {}).name, (event.home || {}).abbreviation, (event.away || {}).name, (event.away || {}).abbreviation, channels].join(" ");
  return lower(text).indexOf(lower(state.query)) !== -1;
}
function renderSportsEventCard(event) {
  const status = sportsStatusLabel(event);
  return "<article class=\"sports-card" + (event.live ? " live" : "") + "\"><div class=\"sports-card-head sports-card-head-compact sports-card-head-status\"><div class=\"sports-status\">" + escapeHTML(status) + "</div></div>"
    + renderSportsMatchup(event, status)
    + renderSportsChannels(event)
    + "</article>";
}
function sportsStatusLabel(event) {
  if (event.live) return event.statusText || "Live";
  if (event.completed) return event.statusText || "Final";
  if (event.startUnix) return sportsDateLabel(event.startUnix);
  return event.statusText || "Time TBD";
}
function sportsDateLabel(unix) {
  const date = new Date(Number(unix || 0) * 1000);
  return date.toLocaleDateString([], { weekday: "short", month: "short", day: "numeric" }) + " " + date.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}
function sportsTeamName(team) {
  return (team && (team.name || team.abbreviation)) || "Team";
}
function sportsTeamAbbreviation(team) {
  const name = sportsTeamName(team);
  return (team && team.abbreviation) || name.split(/\s+/).map(function(part) { return part.slice(0, 1); }).join("").slice(0, 3);
}
function renderSportsTeamLogo(team, className) {
  const name = sportsTeamName(team);
  const label = sportsTeamAbbreviation(team).slice(0, 3);
  if (team && team.logoUrl) return "<img class=\"" + className + "\" src=\"" + escapeHTML(team.logoUrl) + "\" alt=\"\" onerror=\"this.hidden = true; this.nextElementSibling.hidden = false;\"><span class=\"" + className + " logo-fallback\" hidden>" + escapeHTML(label) + "</span>";
  return "<span class=\"" + className + " logo-fallback\">" + escapeHTML(label) + "</span>";
}
function renderSportsMatchup(event, status) {
  const center = event.leagueName || (event.live ? "Live" : (event.completed ? "Final" : "VS"));
  const detail = event.live || event.completed ? status : "";
  const showScore = !!(event.live || event.completed);
  return "<div class=\"sports-matchup\">" + renderSportsMatchTeam(event.away || {}, event.awayScore, showScore) + "<div class=\"sports-versus\"><strong>" + escapeHTML(center) + "</strong>" + (detail ? "<span>" + escapeHTML(detail) + "</span>" : "") + "</div>" + renderSportsMatchTeam(event.home || {}, event.homeScore, showScore) + "</div>";
}
function renderSportsMatchTeam(team, score, showScore) {
  const name = team.name || team.abbreviation || "Team";
  const favorite = !!sportsFavoriteTeamMap()[team.id];
  const logo = renderSportsTeamLogo(team, "sports-match-team-logo");
  const favoriteControl = team.id ? "<button class=\"sports-team-favorite" + (favorite ? " active" : "") + "\" type=\"button\" data-sports-favorite-team=\"" + escapeHTML(team.id || "") + "\" data-sports-favorite-enabled=\"" + (favorite ? "false" : "true") + "\" aria-label=\"" + escapeHTML(favorite ? "Unfollow " + name : "Follow " + name) + "\" aria-pressed=\"" + (favorite ? "true" : "false") + "\">" + icon(favorite ? "heart-solid" : "heart") + "<span>" + (favorite ? "Following" : "Follow") + "</span></button>" : "<span class=\"sports-team-favorite placeholder\" aria-hidden=\"true\"></span>";
  const scoreHTML = showScore ? "<span class=\"sports-match-team-score\">" + escapeHTML(score || "0") + "</span>" : "";
  return "<div class=\"sports-match-team\">" + logo + "<strong data-overflow-tooltip=\"" + escapeHTML(name) + "\">" + escapeHTML(name) + "</strong>" + scoreHTML + favoriteControl + "</div>";
}
function renderSportsChannels(event) {
  const channels = uniqueEventChannels(event.channels);
  if (!channels.length) return "<div class=\"sports-channel-empty muted\">No matching channels.</div>";
  const expanded = !!state.sportsExpandedEvents[event.id];
  const visible = expanded ? channels : channels.slice(0, 3);
  const hiddenCount = channels.length - visible.length;
  const more = hiddenCount > 0 ? "<button class=\"sports-channel-more\" type=\"button\" data-sports-expand-event=\"" + escapeHTML(event.id || "") + "\">+" + hiddenCount + " more</button>" : (expanded && channels.length > 3 ? "<button class=\"sports-channel-more\" type=\"button\" data-sports-expand-event=\"" + escapeHTML(event.id || "") + "\">Show less</button>" : "");
  return "<div class=\"sports-channels\">" + visible.map(renderSportsChannelChip).join("") + more + "</div>";
}
function renderSportsChannelChip(channel) {
  const meta = channel.categoryName || channel.reason || "Live TV";
  const name = channel.name || "Channel";
  return "<div class=\"sports-channel-wrap\"><button class=\"sports-channel\" type=\"button\" data-channel=\"" + escapeHTML(channel.id) + "\" title=\"" + escapeHTML(channel.reason || meta) + "\"><span class=\"sports-channel-logo\">" + logoHTML(channel) + "</span><span class=\"sports-channel-copy\"><strong data-overflow-tooltip=\"" + escapeHTML(name) + "\">" + escapeHTML(name) + "</strong><small>" + escapeHTML(meta) + "</small></span></button></div>";
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
function playerSportsChannelMatches(event, channelID) {
  return uniqueEventChannels(event && event.channels).some(function(channel) {
    return String(channel.id || "") === String(channelID || "") && Number(channel.score || 0) >= 60;
  });
}
function playerSportsEvents() {
  const now = Math.floor(Date.now() / 1000);
  const currentID = state.currentChannel && state.currentChannel.id;
  return items(state.sports && state.sports.events).filter(function(event) {
    const channels = uniqueEventChannels(event.channels).filter(function(channel) { return Number(channel.score || 0) >= 60; });
    const startsSoon = Number(event.startUnix || 0) <= now + 12 * 3600;
    return channels.length && !event.completed && (event.live || startsSoon || playerSportsChannelMatches(event, currentID));
  }).sort(function(left, right) {
    const leftCurrent = playerSportsChannelMatches(left, currentID) ? 1 : 0;
    const rightCurrent = playerSportsChannelMatches(right, currentID) ? 1 : 0;
    if (leftCurrent !== rightCurrent) return rightCurrent - leftCurrent;
    const leftFavorite = sportsEventHasFavoriteTeam(left) ? 1 : 0;
    const rightFavorite = sportsEventHasFavoriteTeam(right) ? 1 : 0;
    if (leftFavorite !== rightFavorite) return rightFavorite - leftFavorite;
    if (left.live !== right.live) return left.live ? -1 : 1;
    return sportsEventStartSort(left, Number.MAX_SAFE_INTEGER) - sportsEventStartSort(right, Number.MAX_SAFE_INTEGER);
  }).slice(0, 12);
}
function playerSportsEventChannel(event) {
  const currentID = state.currentChannel && state.currentChannel.id;
  const channels = uniqueEventChannels(event && event.channels).filter(function(channel) { return Number(channel.score || 0) >= 60; });
  return channels.find(function(channel) { return String(channel.id || "") === String(currentID || ""); }) || channels[0] || null;
}
function renderPlayerSportsEvent(event) {
  const channel = playerSportsEventChannel(event);
  const away = event.away || {};
  const home = event.home || {};
  const scored = event.live || event.completed;
  const current = playerSportsChannelMatches(event, state.currentChannel && state.currentChannel.id);
  return "<button class=\"player-sports-event" + (event.live ? " live" : "") + (current ? " current" : "") + "\" type=\"button\" data-player-sports-channel=\"" + escapeHTML(channel && channel.id) + "\"><span class=\"player-sports-event-top\"><span class=\"player-sports-league\">" + escapeHTML(event.leagueName || event.leagueId || "Sports") + "</span><span class=\"player-sports-status\">" + escapeHTML(sportsStatusLabel(event)) + "</span></span><span class=\"player-sports-team\"><span>" + escapeHTML(sportsTeamAbbreviation(away)) + "</span><strong>" + (scored ? escapeHTML(event.awayScore || "0") : "") + "</strong></span><span class=\"player-sports-team\"><span>" + escapeHTML(sportsTeamAbbreviation(home)) + "</span><strong>" + (scored ? escapeHTML(event.homeScore || "0") : "") + "</strong></span><small>" + escapeHTML((channel && channel.name) || event.shortName || event.name || "Sports") + "</small></button>";
}
function playerSportsChannels(events) {
  const seen = {};
  const channels = [];
  items(events).forEach(function(event) {
    uniqueEventChannels(event.channels).filter(function(channel) { return Number(channel.score || 0) >= 60; }).forEach(function(match) {
      if (!match.id || seen[match.id]) return;
      const channel = channelByID(match.id) || match;
      seen[match.id] = true;
      channels.push(channel);
    });
  });
  return channels.slice(0, 12);
}
function renderPlayerSportsChannel(channel) {
  const program = currentProgram(channel) || {};
  return "<button class=\"player-sports-channel\" type=\"button\" data-player-sports-channel=\"" + escapeHTML(channel.id) + "\"><span>" + logoHTML(channel) + "</span><span><strong>" + escapeHTML(channel.name || "Sports channel") + "</strong><small>" + escapeHTML(program.title || channel.categoryName || "Live TV") + "</small></span></button>";
}
function renderPlayerSportsDrawer() {
  const root = byId("player-sports-drawer");
  const shell = document.querySelector(".playback-shell");
  const button = byId("player-sports-button");
  if (shell) shell.classList.toggle("sports-open", !!state.playerSportsOpen);
  if (button) {
    button.classList.toggle("active", !!state.playerSportsOpen);
    button.setAttribute("aria-expanded", state.playerSportsOpen ? "true" : "false");
  }
  if (!root) return;
  root.classList.toggle("open", !!state.playerSportsOpen);
  if (!state.playerSportsOpen) {
    root.innerHTML = "";
    return;
  }
  const events = playerSportsEvents();
  const channels = playerSportsChannels(events);
  const loading = state.sportsLoading && !state.sports;
  root.innerHTML = "<div class=\"player-sports-head\"><div><strong>Live Sports</strong><span>Scores and matched channels</span></div><button type=\"button\" data-player-action=\"sports-close\" aria-label=\"Close live sports\">" + icon("x") + "</button></div>"
    + (loading ? "<div class=\"player-sports-loading\"><span></span><span></span><span></span></div>" : "")
    + (!loading && !events.length ? "<div class=\"player-sports-empty\">No live or upcoming events have a confident channel match.</div>" : "")
    + (events.length ? "<div class=\"player-sports-section\"><div class=\"player-sports-section-title\">Live &amp; upcoming</div><div class=\"player-sports-rail\">" + events.map(renderPlayerSportsEvent).join("") + "</div></div>" : "")
    + (channels.length ? "<div class=\"player-sports-section\"><div class=\"player-sports-section-title\">Sports channels</div><div class=\"player-sports-channel-rail\">" + channels.map(renderPlayerSportsChannel).join("") + "</div></div>" : "");
}
function stopPlayerSportsRefresh() {
  if (state.playerSportsTimer) clearInterval(state.playerSportsTimer);
  state.playerSportsTimer = null;
}
function startPlayerSportsRefresh() {
  stopPlayerSportsRefresh();
  if (!state.playerSportsOpen) return;
  state.playerSportsTimer = setInterval(function() {
    if (state.view === "player" && state.playerSportsOpen) loadSports(true);
    else stopPlayerSportsRefresh();
  }, 30000);
}
function togglePlayerSports(open) {
  if (!sportsFirstPlayerEnabled()) return;
  state.playerSportsOpen = typeof open === "boolean" ? open : !state.playerSportsOpen;
  state.playerGuideOpen = false;
  closePlayerPopovers();
  renderPlayerGuidePanel();
  renderPlayerSportsDrawer();
  if (state.playerSportsOpen) {
    loadSports(false).then(renderPlayerSportsDrawer);
    startPlayerSportsRefresh();
  } else stopPlayerSportsRefresh();
}
function setSportsTab(tab) {
  state.sportsTab = tab || "live";
  state.sportsExpandedEvents = {};
  renderSportsPage();
}
function setSportsLeague(leagueID) {
  state.sportsLeague = leagueID || "";
  state.sportsExpandedEvents = {};
  renderSportsPage();
}
function toggleSportsEventChannels(eventID) {
  eventID = String(eventID || "");
  if (!eventID) return;
  if (state.sportsExpandedEvents[eventID]) delete state.sportsExpandedEvents[eventID];
  else state.sportsExpandedEvents[eventID] = true;
  renderSportsPage();
}
function toggleSportsTeamFavorite(teamID, enabled) {
  teamID = String(teamID || "");
  if (!teamID) return;
  if (enabled) state.app.preferences.sportsFavoriteTeams[teamID] = true;
  else delete state.app.preferences.sportsFavoriteTeams[teamID];
  applySportsFavoritesToPayload();
  savePrefs();
  renderSportsPage();
}
function loadEvents(force) {
  if (state.eventsLoading) return Promise.resolve(state.events || { events: [], categories: [] });
  if (state.events && !force) return Promise.resolve(state.events);
  state.eventsLoading = true;
  return getJSON("/dispatcharr/api/events" + (force ? "?refresh=1" : "")).then(function(payload) {
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
function filteredBroadcastEvents(payload) {
  const now = Math.floor(Date.now() / 1000);
  return items(payload && payload.events).filter(function(event) {
    if (!profileSelectionIsAll() && !uniqueEventChannels(event.channels).length) return false;
    if (state.eventCategory && event.categoryId !== state.eventCategory) return false;
    if (state.eventsTab === "live" && !event.live) return false;
    const startUnix = Number(event.startUnix || 0);
    if (state.eventsTab === "upcoming" && (event.completed || event.live || (startUnix > 0 && startUnix < now - 3600))) return false;
    return true;
  });
}
function broadcastEventMatchesQuery(event) {
  const channels = items(event.channels).map(function(channel) { return [channel.name, channel.categoryName, channel.reason].join(" "); }).join(" ");
  const text = [event.name, event.shortName, event.categoryName, event.keyword, event.description, channels].join(" ");
  return lower(text).indexOf(lower(state.query)) !== -1;
}
function renderBroadcastEventCard(event) {
  const status = eventStatusLabel(event);
  const title = event.shortName || event.name || "Event";
  const artwork = event.artworkUrl || event.imageUrl || event.posterUrl || event.thumbnailUrl || "";
  const poster = artwork ? "<div class=\"event-poster\"><img src=\"" + escapeHTML(artwork) + "\" alt=\"\" onerror=\"this.closest('.event-poster').remove();\"></div>" : "";
  const cardClass = artwork ? 'class="event-card sports-card' : 'class="event-card no-art sports-card';
  const uniqueChannels = uniqueEventChannels(event.channels);
  const windows = items(event.windows);
  const meta = [event.keyword || "", windows.length > 1 ? windows.length + " coverage windows" : "", uniqueChannels.length ? uniqueChannels.length + " channel" + (uniqueChannels.length === 1 ? "" : "s") : ""].filter(Boolean).map(function(value, index) { return "<span" + (index === 0 && event.keyword ? " class=\"event-keyword\"" : "") + ">" + escapeHTML(value) + "</span>"; }).join("");
  return "<article " + cardClass + (event.live ? " live" : "") + '"><div class="sports-card-head"><div class="sports-card-title"><span class="sports-league-pill">' + escapeHTML(event.categoryName || "Events") + "</span><strong data-overflow-tooltip=\"" + escapeHTML(event.name || title) + "\">" + escapeHTML(title) + "</strong></div><div class=\"sports-status\">" + escapeHTML(status) + "</div></div>"
    + "<div class=\"event-card-body" + (artwork ? "" : " no-art") + "\">" + poster + "<div class=\"event-details\"><p data-overflow-description=\"true\">" + escapeHTML(event.description || "No event details available.") + "</p><div class=\"event-meta\">" + meta + "</div>" + renderEventBroadcastWindows(event) + "</div></div>"
    + renderBroadcastEventChannels(event)
    + "</article>";
}
function renderEventBroadcastWindows(event) {
  const windows = items(event && event.windows);
  if (windows.length < 2) return "";
  const visible = windows.slice(0, 4);
  const rows = visible.map(function(windowInfo, index) {
    const channels = uniqueEventChannels(windowInfo.channels);
    const end = windowInfo.endUnix ? " to " + timeLabel(windowInfo.endUnix) : "";
    return "<div class=\"event-broadcast-window\"><strong>" + escapeHTML(sportsDateLabel(windowInfo.startUnix)) + escapeHTML(end) + "</strong><small>" + escapeHTML(channels.length + " channel" + (channels.length === 1 ? "" : "s")) + "</small></div>";
  }).join("");
  const more = windows.length > visible.length ? "<div class=\"event-broadcast-more\">+" + (windows.length - visible.length) + " later</div>" : "";
  return "<div class=\"event-broadcast-windows\" aria-label=\"Broadcast coverage windows\">" + rows + more + "</div>";
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
function renderBroadcastEventChannels(event) {
  const channels = uniqueEventChannels(event.channels);
  if (!channels.length) return "<div class=\"sports-channel-empty muted\">No matching channels.</div>";
  const expanded = !!state.expandedEvents[event.id];
  const visible = expanded ? channels : channels.slice(0, 3);
  const hiddenCount = channels.length - visible.length;
  const more = hiddenCount > 0 ? "<button class=\"sports-channel-more\" type=\"button\" data-event-expand=\"" + escapeHTML(event.id || "") + "\">+" + hiddenCount + " more</button>" : (expanded && channels.length > 3 ? "<button class=\"sports-channel-more\" type=\"button\" data-event-expand=\"" + escapeHTML(event.id || "") + "\">Show less</button>" : "");
  return "<div class=\"sports-channels\">" + visible.map(renderSportsChannelChip).join("") + more + "</div>";
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
function toggleBroadcastEventChannels(eventID) {
  eventID = String(eventID || "");
  if (!eventID) return;
  if (state.expandedEvents[eventID]) delete state.expandedEvents[eventID];
  else state.expandedEvents[eventID] = true;
  renderEventsPage();
}
function favoriteCards(channels) {
  if (!channels.length) return "<div class=\"empty\">No favorite channels yet.</div>";
  return "<div class=\"row-scroll\">" + channels.map(function(channel, index) {
    const controls = favoriteMap()[channel.id] ? "<div class=\"settings-actions\"><button data-favorite-move=\"up\" data-channel-id=\"" + escapeHTML(channel.id) + "\"" + (index === 0 ? " disabled" : "") + ">Move up</button><button data-favorite-move=\"down\" data-channel-id=\"" + escapeHTML(channel.id) + "\"" + (index === channels.length - 1 ? " disabled" : "") + ">Move down</button></div>" : "";
    return "<div class=\"favorite-card\"><button class=\"continue-card\" data-channel=\"" + escapeHTML(channel.id) + "\"><div class=\"poster-box\">" + (channel.logoUrl ? "<img src=\"" + escapeHTML(channel.logoUrl) + "\" alt=\"\">" : "<span>" + escapeHTML((channel.name || "TV").slice(0, 5)) + "</span>") + "</div><strong>" + escapeHTML(channel.name || "Untitled") + "</strong><div class=\"muted\">" + escapeHTML(channel.categoryName || "Live TV") + "</div></button>" + controls + "</div>";
  }).join("") + "</div>";
}
function compareCategoryDisplayName(left, right) {
  const leftName = String((left && (left.name || left.id)) || "");
  const rightName = String((right && (right.name || right.id)) || "");
  return leftName.localeCompare(rightName, undefined, { numeric: true, sensitivity: "base" }) || leftName.localeCompare(rightName) || String((left && left.id) || "").localeCompare(String((right && right.id) || ""));
}
function categoryGrid() {
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
  if (featured.length) sections.push(categoryGridSection(featuredGroupLabel(), featured));
  if (custom.length) sections.push(categoryGridSection("My Groups", custom));
  if (regularListing.length) sections.push(categoryGridSection(adminListingTitle(), regularListing));
  if (!listing.length && sourceCategories.length) sections.push(categoryGridSection(adminListingTitle(), sourceCategories));
  return sections.length ? sections.join("") : "<div class=\"empty\">No groups yet.</div>";
}
function categoryGridSection(title, categories) {
  return sectionHeader(title) + "<div class=\"category-grid\">" + categories.map(categoryTileHTML).join("") + "</div>";
}
function categoryTileHTML(category) {
  const name = String((category && (category.name || category.id)) || "");
  const meta = String((category && category.count ? category.count + " channels" : (category && category.kind) || "") || "");
  return "<button class=\"tile" + (state.category === category.id ? " active" : "") + "\" data-category=\"" + escapeHTML(category.id) + "\" aria-label=\"" + escapeHTML(meta ? name + ", " + meta : name) + "\"><strong data-overflow-tooltip=\"" + escapeHTML(name) + "\">" + escapeHTML(name) + "</strong><span>" + escapeHTML(meta) + "</span></button>";
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
function guideFilterCategories() {
  const hidden = hiddenMap();
  const includeChannel = function(channel) { return !(channel.categoryId && hidden[channel.categoryId]); };
  const mode = adminSettings().mode || "normal";
  const custom = customGroupCategories();
  if (mode !== "delimiter") return custom.concat(sourceCategoriesWithChannels(includeChannel));
  return custom
    .concat(featuredCategoriesFromPaths("", includeChannel, true))
    .concat(virtualCategoriesFromPaths("", includeChannel, true))
    .sort(compareCategoryDisplayName);
}
function adminListingTitle() {
  const mode = adminSettings().mode || "normal";
  if (mode === "delimiter") return configuredGroupLabel();
  return "Channel Groups";
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
  const mode = adminSettings().mode || "normal";
  return "Featured " + (mode === "delimiter" ? configuredGroupLabel() : "Channel Groups");
}
function allGroupLabel() {
  return "All " + adminListingTitle().toLowerCase();
}
function virtualGroupLabelSuffix(value) {
  value = String(value || "").trim().replace(/^virtual\s+/i, "").trim();
  return value || "Groups";
}
function adminListingCategories(parentPath, includeChannel) {
  const hidden = hiddenMap();
  includeChannel = includeChannel || function(channel) { return !(channel.categoryId && hidden[channel.categoryId]); };
  const mode = adminSettings().mode || "normal";
  if (mode === "delimiter") return parentPath ? sourceVirtualChildCategories(parentPath, includeChannel) : sourceVirtualChildCategories("", includeChannel);
  return sourceCategoriesWithChannels(includeChannel);
}
function virtualCategoriesActive() {
  const hidden = hiddenMap();
  return adminSettings().mode === "delimiter" && virtualGroupCategories(function(channel) { return !(channel.categoryId && hidden[channel.categoryId]); }).length > 0;
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
function channelHasCurrentGuide(channel) {
  if (!channel) return false;
  const now = Math.floor(Date.now() / 1000);
  return programsFor(channel.id).some(function(program) {
    const start = program.startUnix || 0;
    const end = program.endUnix || start + 1800;
    return start <= now + 600 && end >= now;
  });
}
function channelHasNearGuide(channel) {
  if (!channel) return false;
  const now = Math.floor(Date.now() / 1000);
  return programsFor(channel.id).some(function(program) {
    const start = program.startUnix || 0;
    const end = program.endUnix || start + 1800;
    return start <= now + 1800 && end >= now - 300;
  });
}
function maybeWarmGuideForChannels(channels, key) {
  if (!state.app || state.appLoadedFromCache || !items(channels).length) return;
  if (items(channels).every(channelHasNearGuide)) return;
  const channelIds = items(channels).map(function(channel) { return channel && channel.id; }).filter(Boolean);
  if (!channelIds.length) return;
  const warmKey = String(key || channelIds.slice(0, 20).join("|"));
  const now = Date.now();
  if (state.guideWarmPings[warmKey] && now - state.guideWarmPings[warmKey] < 5 * 60 * 1000) return;
  state.guideWarmPings[warmKey] = now;
  postJSON("/dispatcharr/api/guide/ping", { channelIds: channelIds }).then(function(result) {
    if (result && result.refreshing) {
      setTimeout(function() {
        refreshStatusData().then(function() { return refreshSupplementalData(false); }).then(render).catch(function() {});
      }, 12000);
    }
  }).catch(function(error) {
    try { console.warn("Dispatcharr guide warm ping failed", error); } catch (_) {}
  });
}
function homeGuideChannels(watched) {
  const seen = {};
  const pool = [];
  items(watched).concat(visibleChannels(false).slice(0, 120)).forEach(function(channel) {
    if (!channel || seen[channel.id]) return;
    seen[channel.id] = true;
    pool.push(channel);
  });
  return pool.filter(channelHasCurrentGuide).slice(0, 5);
}
function renderHomeGuide(channels, emptyMessage, options) {
  const meta = options && options.hideFreshness ? "" : "<div class=\"guide-meta-row\">" + guideFreshnessHTML() + "</div>";
  if (!channels.length) return meta + "<div class=\"empty\">" + escapeHTML(emptyMessage || "No recently watched channels yet.") + "</div>";
  const slots = guideSlots();
  return meta + "<div class=\"home-guide guide-scroll\"><div class=\"guide-page guide-timeline\" style=\"" + guideTimelineStyle(slots) + "\"><div class=\"time-head\"><span>Today</span>" + slots.map(function(slot) { return "<span>" + escapeHTML(timeLabel(slot)) + "</span>"; }).join("") + "</div>" + channels.map(function(channel, channelIndex) {
    return "<div class=\"epg-row\">" + renderGuideChannelButton(channel) + "<div class=\"epg-programs\">" + renderEPGCells(channel, channelIndex) + "</div></div>";
  }).join("") + "</div></div>";
}
function renderVirtualCategoryGuide(channels) {
  return renderHomeGuide(channels, "No channels in this virtual group yet.", { hideFreshness: true });
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
    const program = currentProgram(channel) || {};
    const subtitle = program.title || channel.categoryName || "Live TV";
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
    const filtered = channels.filter(channelMatchesFolderQuery);
    byId("view").innerHTML = sectionHeader("Favorite channels") + folderFilterHTML("Filter favorites") + favoriteCards(filtered.slice(0, 60));
    return;
  }
  if (state.category.indexOf("virtual:") === 0 || state.category.indexOf("featured:") === 0) {
    const featured = state.category.indexOf("featured:") === 0;
    const path = featured ? featuredCategoryPath(state.category) : virtualCategoryPath(state.category);
    const hidden = hiddenMap();
    const children = (featured ? featuredChildCategories : virtualChildCategories)(path, function(channel) {
      return !(channel.categoryId && hidden[channel.categoryId]);
    });
    const filteredChildren = children.filter(categoryMatchesFolderQuery);
    const filteredChannels = channels.filter(channelMatchesFolderQuery);
    byId("view").innerHTML = virtualFolderHeader(path, featured)
      + folderFilterHTML("Filter this folder", "")
      + (filteredChildren.length ? "<div class=\"category-grid\">" + filteredChildren.map(categoryTileHTML).join("") + "</div>" : "")
      + renderVirtualCategoryContent(filteredChannels);
    maybeWarmGuideForChannels(filteredChannels, state.category);
    return;
  }
  const filteredChannels = channels.filter(channelMatchesFolderQuery);
  byId("view").innerHTML = categoryGrid() + sectionHeader(categoryName(state.category) || "Channels") + folderFilterHTML("Filter visible channels") + rowCards(filteredChannels.slice(0, 24));
  if (state.category) maybeWarmGuideForChannels(filteredChannels, state.category);
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
function programDetailsState() {
  if (!state.programDetails) return null;
  const channel = channelByID(state.programDetails.channelID);
  const program = programByID(state.programDetails.channelID, state.programDetails.programID) || state.programDetails.fallbackProgram;
  if (!channel || !program) return null;
  return { channel: channel, program: program };
}
function openProgramDetails(channelID, programID) {
  const program = programByID(channelID, programID);
  const channel = channelByID(channelID);
  if (!channel) return;
  programModalReturnFocus = document.activeElement;
  if (!program || programIsGuidePlaceholder(program)) {
    state.programDetails = { channelID: channelID, programID: programID, fallbackProgram: { id: "", title: "Program details unavailable", description: "Program details are not available for this guide entry. You can still watch the channel.", startUnix: 0, endUnix: 0 } };
    renderProgramDetailsModal();
    return;
  }
  state.programDetails = { channelID: channelID, programID: programID };
  renderProgramDetailsModal();
}
function closeProgramDetails() {
  state.programDetails = null;
  renderProgramDetailsModal();
  const returnFocus = programModalReturnFocus;
  programModalReturnFocus = null;
  if (returnFocus && typeof returnFocus.focus === "function" && document.contains(returnFocus)) returnFocus.focus();
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
function programDetailsModalHTML(details) {
  const channel = details.channel;
  const program = details.program;
  const title = program.title || guideUnavailableLabel();
  const description = String(program.summary || program.description || "").trim();
  const start = program.startUnix || 0;
  const end = program.endUnix || 0;
  const duration = start && end ? Math.max(1, Math.round((end - start) / 60)) : 0;
  const timeText = [start ? dateTimeLabel(start) : "", duration ? programDurationLabel(duration * 60) : ""].filter(Boolean).join(", ");
  const channelName = channel.name || channel.categoryName || "Live TV";
  const tags = programDetailTags(program, channel);
  const canSchedule = recordingSchedulingEnabled() && end > Math.floor(Date.now() / 1000);
  return "<div class=\"program-modal-backdrop\" data-program-modal-close=\"true\"></div><section class=\"program-modal\" role=\"dialog\" aria-modal=\"true\" aria-labelledby=\"program-modal-title\" aria-describedby=\"program-modal-description\">"
    + "<button class=\"program-modal-close\" type=\"button\" data-program-modal-close=\"true\" aria-label=\"Close\">" + icon("x") + "</button>"
    + "<div class=\"program-modal-head\"><div class=\"program-modal-art\">" + logoHTML(channel) + "</div><div class=\"program-modal-title\"><h2 id=\"program-modal-title\">" + escapeHTML(title) + "</h2><p>" + escapeHTML(channelName) + "</p><div class=\"program-modal-time\">" + icon("clock") + "<span>" + escapeHTML(timeText || (programIsLive(program) ? "Live now" : "Guide airing")) + "</span></div><div class=\"program-modal-tags\">" + tags.map(function(tag) { return "<span" + (tag === "Live now" ? " class=\"is-live\"" : "") + ">" + escapeHTML(tag) + "</span>"; }).join("") + "</div></div></div>"
    + "<div id=\"program-modal-description\" class=\"program-modal-body\">" + (description ? escapeHTML(description) : "No additional details are available for this airing.") + "</div>"
    + "<div class=\"program-modal-actions\"><button type=\"button\" data-search-airing=\"" + escapeHTML(title) + "\">" + icon("search") + "<span>More Airings</span></button><button type=\"button\" data-program-detail-watch=\"" + escapeHTML(channel.id) + "\">" + icon("play") + "<span>Watch Now</span></button>" + (canSchedule ? "<button type=\"button\" data-program-detail-schedule=\"" + escapeHTML(channel.id) + "\" data-program-detail-program=\"" + escapeHTML(program.id || "") + "\">" + icon("record") + "<span>Record</span></button>" : "") + "</div>"
    + "</section>";
}
function renderProgramDetailsModal() {
  let root = byId("program-details-root");
  if (!root) {
    root = document.createElement("div");
    root.id = "program-details-root";
    document.body.appendChild(root);
  }
  const details = programDetailsState();
  root.innerHTML = details ? programDetailsModalHTML(details) : "";
  const shell = document.querySelector(".shell");
  if (shell) {
    if (details) shell.setAttribute("inert", "");
    else shell.removeAttribute("inert");
  }
  document.body.classList.toggle("program-modal-open", !!details);
  if (details) {
    const closeButton = root.querySelector(".program-modal-close");
    if (closeButton) closeButton.focus();
  }
}
function trapProgramModalFocus(event) {
  if (!state.programDetails || event.key !== "Tab") return false;
  const modal = document.querySelector(".program-modal");
  if (!modal) return false;
  const focusable = Array.prototype.slice.call(modal.querySelectorAll("button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])"));
  if (!focusable.length) return false;
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
    return true;
  }
  if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
    return true;
  }
  return false;
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
function playerGuideProgramLines(channel) {
  const current = liveProgram(channel);
  const next = nextProgram(channel);
  const currentTitle = current && current.title ? current.title : "";
  const nextTitle = next && next.title ? next.title : "";
  if (currentTitle) {
    return {
      primary: (timeLabel(current.startUnix) || "Live") + " - " + currentTitle,
      secondary: nextTitle ? "Next " + (timeLabel(next.startUnix) || "Soon") + " - " + nextTitle : ""
    };
  }
  if (nextTitle) {
    return { primary: "Next " + (timeLabel(next.startUnix) || "Soon") + " - " + nextTitle, secondary: "" };
  }
  return { primary: guideUnavailableLabel(), secondary: "" };
}
function playerGuideMatches(channel, query) {
  query = lower(query).trim();
  if (!query) return true;
  const current = liveProgram(channel) || {};
  const next = nextProgram(channel) || {};
  const lines = playerGuideProgramLines(channel);
  const haystack = [
    channel && channel.name,
    channel && channel.categoryName,
    sourceCategoryLabel(channel || {}),
    current.title,
    current.description,
    next.title,
    next.description,
    lines.primary,
    lines.secondary
  ].map(lower).join(" ");
  return haystack.indexOf(query) !== -1;
}
function playerLogoHTML(channel) {
  if (channel && channel.logoUrl) return "<img class=\"player-logo\" src=\"" + escapeHTML(channel.logoUrl) + "\" alt=\"\">";
  return "<div class=\"player-logo player-logo-fallback\">" + escapeHTML(((channel && channel.name) || "TV").slice(0, 5)) + "</div>";
}
function playerFavoriteButtonHTML(channel) {
  const isFavorite = !!(channel && favoriteMap()[channel.id]);
  return "<button id=\"player-favorite-button\" class=\"player-icon favorite" + (isFavorite ? " active" : "") + "\" data-player-action=\"favorite\" aria-label=\"" + (isFavorite ? "Remove channel from favorites" : "Favorite channel") + "\" aria-pressed=\"" + (isFavorite ? "true" : "false") + "\">" + icon(isFavorite ? "heart-solid" : "heart") + "</button>";
}
function renderPlayerPage() {
  const channel = state.currentChannel || visibleChannels(false)[0] || null;
  const program = currentProgram(channel) || {};
  const channelName = channel ? channel.name || "Untitled channel" : "Choose a channel";
  const categoryNameText = channel ? channel.categoryName || "Live TV" : "Live TV";
  const replayMode = isRewindableChannel(channel);
  const title = program.title || channelName;
  const description = program.description || categoryNameText;
  const start = timeLabel(program.startUnix) || "LIVE";
  const end = timeLabel(program.endUnix) || "Now";
  const playbackShellClass = (replayMode ? "playback-shell is-replay" : "playback-shell") + (sportsFirstPlayerEnabled() ? " sports-enabled" : "");
  const videoAttributes = replayMode ? " autoplay playsinline controls" : " autoplay playsinline";
  const modeTag = replayMode ? "Replay" : "AV";
  const timelineEnd = replayMode ? escapeHTML(end) : "<span class=\"live-dot\"></span>LIVE&nbsp;&nbsp;" + escapeHTML(end);
  const timeShiftControls = "<div id=\"player-timeshift-controls\" class=\"player-timeshift-controls hidden\"><button class=\"player-icon\" data-player-action=\"rewind-30\" aria-label=\"Rewind 30 seconds\">" + icon("rewind") + "</button><button class=\"player-icon\" data-player-action=\"play-toggle\" aria-label=\"Play or pause\">" + icon("play") + "</button><button class=\"player-icon\" data-player-action=\"forward-30\" aria-label=\"Forward 30 seconds\">" + icon("forward") + "</button><input id=\"player-timeshift-range\" type=\"range\" min=\"0\" max=\"1\" step=\"0.25\" value=\"1\" aria-label=\"Live Rewind position\"><button class=\"player-live-button\" data-player-action=\"go-live\"><span class=\"live-dot\"></span><span id=\"player-timeshift-label\">LIVE</span></button></div>";
  const sportsButton = sportsFirstPlayerEnabled() ? "<button id=\"player-sports-button\" class=\"player-icon\" data-player-action=\"sports\" aria-label=\"Sports center\" aria-haspopup=\"true\" aria-expanded=\"false\">" + icon("trophy") + "</button>" : "";
  byId("view").innerHTML = "<section class=\"" + playbackShellClass + "\"><div class=\"playback-stage\"><video id=\"player\" class=\"playback-video\"" + videoAttributes + "></video><div class=\"playback-scrim\"></div><button id=\"player-center-button\" class=\"player-center-button hidden\" data-player-action=\"play-toggle\" aria-label=\"Play\">" + icon("play") + "</button><div class=\"player-top\"><button class=\"player-exit\" data-player-action=\"back\" aria-label=\"Back to Live TV browse\"><span class=\"player-icon\">" + icon("x") + "</span><span>Exit</span></button><div class=\"player-top-actions\"><div class=\"player-audio\"><button id=\"player-audio-button\" class=\"player-chip\" data-player-action=\"audio-menu\" aria-haspopup=\"true\" aria-expanded=\"false\">" + icon("language") + "<span>Audio</span>" + icon("chevron-down") + "</button><div id=\"player-audio-menu\" class=\"player-menu\" role=\"menu\"></div></div><div class=\"player-volume\"><button id=\"player-volume-button\" class=\"player-icon\" data-player-action=\"volume-menu\" aria-label=\"Volume\" aria-haspopup=\"true\" aria-expanded=\"false\">" + icon("speaker") + "</button><div id=\"player-volume-popover\" class=\"volume-popover\"><span>VOL</span><input id=\"player-volume-slider\" type=\"range\" min=\"0\" max=\"100\" step=\"1\" value=\"" + Math.round(state.volume * 100) + "\" aria-label=\"Volume\"><span id=\"player-volume-value\" class=\"volume-value\"></span></div></div><button class=\"player-icon\" data-player-action=\"cast\" aria-label=\"AirPlay or Cast\">" + icon("airplay") + "</button><button id=\"player-guide-button\" class=\"player-icon player-guide-button\" data-player-action=\"guide\" aria-label=\"Guide\" aria-haspopup=\"true\" aria-expanded=\"false\">" + icon("guide") + "</button>" + sportsButton + "<button class=\"player-icon\" data-player-action=\"add-multiview\" aria-label=\"Add current channel to multiview\">" + icon("multiview") + "</button><button id=\"player-fullscreen-button\" class=\"player-icon\" data-player-action=\"fullscreen\" aria-label=\"Fullscreen\" aria-pressed=\"false\">" + icon("fullscreen") + "</button><div class=\"player-more\"><button id=\"player-more-button\" class=\"player-icon\" data-player-action=\"more\" aria-label=\"More\" aria-haspopup=\"true\" aria-expanded=\"false\">" + icon("ellipsis") + "</button><div id=\"player-more-menu\" class=\"player-more-menu\"></div></div></div></div><div id=\"player-toast\" class=\"player-toast\" role=\"status\"></div><div id=\"player-guide-panel\" class=\"player-guide-panel\"></div><div id=\"player-sports-drawer\" class=\"player-sports-drawer\" aria-live=\"polite\"></div><div class=\"player-bottom\"><div class=\"player-bottom-row\"><div class=\"player-meta\">" + playerLogoHTML(channel) + "<div class=\"player-kicker\">" + escapeHTML(channelName) + "</div><h2 class=\"player-title\">" + escapeHTML(title) + "</h2><p class=\"player-description\" data-overflow-description=\"true\">" + escapeHTML(description) + "</p><div class=\"player-tags\"><span class=\"player-tag\">" + escapeHTML(categoryNameText) + "</span><span id=\"player-mode-tag\" class=\"player-tag\">" + escapeHTML(modeTag) + "</span></div></div><div class=\"player-bottom-actions\">" + playerFavoriteButtonHTML(channel) + "<button class=\"player-icon\" data-player-action=\"pip\" aria-label=\"Picture in Picture\">" + icon("pip") + "</button><button id=\"player-subtitles-button\" class=\"player-icon\" data-player-action=\"subtitles\" aria-label=\"Subtitles\" aria-pressed=\"false\">" + icon("captions") + "</button><button id=\"player-language-button\" class=\"player-icon\" data-player-action=\"language-menu\" aria-label=\"Audio language\" aria-haspopup=\"true\" aria-expanded=\"false\">" + icon("language") + "</button></div></div>" + timeShiftControls + "<div class=\"timeline\"><span>" + escapeHTML(start) + "</span><div class=\"timeline-bar\"><div class=\"timeline-fill\"></div><div class=\"timeline-knob\"></div></div><span>" + timelineEnd + "</span></div></div></div></section>";
  updateAudioMenu();
  updateVolumeMenu();
  renderPlayerGuidePanel();
  renderPlayerSportsDrawer();
  renderPlayerMoreMenu();
  updateFullscreenButton();
  wakePlayerChrome(1800);
}
function renderMultiviewPage() {
  resetMultiviewMedia();
  const tiles = items(state.multiviewTiles).filter(function(tile) {
    tile.channel = channelByID((tile.channel || {}).id || tile.channelID) || tile.channel;
    return tile.channel;
  }).slice(0, 4);
  state.multiviewTiles = tiles;
  if (!state.multiviewActiveTileID && tiles[0]) state.multiviewActiveTileID = tiles[0].id;
  if (state.multiviewActiveTileID && !tiles.some(function(tile) { return tile.id === state.multiviewActiveTileID; })) state.multiviewActiveTileID = tiles[0] ? tiles[0].id : "";
  const countClass = "count-" + Math.max(tiles.length, 1);
  const title = tiles.length ? tiles.length + " channel" + (tiles.length === 1 ? "" : "s") : "Choose channels";
  byId("view").innerHTML = "<section class=\"multiview-page\"><div class=\"multiview-toolbar\"><div><h2>Multiview</h2><p>" + escapeHTML(title) + " · focused tile owns audio</p></div><div class=\"multiview-actions\"><span class=\"multiview-count\">" + escapeHTML(String(tiles.length)) + "/4</span>" + (tiles.length ? "<button class=\"chip\" type=\"button\" data-multiview-action=\"clear\">Clear</button>" : "") + "</div></div>"
    + (tiles.length ? "<div class=\"multiview-grid " + countClass + "\">" + tiles.map(renderMultiviewTile).join("") + "</div>" : renderMultiviewEmpty())
    + (tiles.length && tiles.length < 4 ? renderMultiviewPicker() : "")
    + "</section>";
  attachMultiviewPlayers();
}
function renderMultiviewTile(tile) {
  const channel = tile.channel || {};
  const active = tile.id === state.multiviewActiveTileID;
  const program = currentProgram(channel) || {};
  const title = program.title || channel.name || "Live TV";
  const subtitle = channel.categoryName || "Live TV";
  const muted = active ? "Audio" : "Muted";
  return "<article class=\"multiview-tile" + (active ? " active" : "") + "\" data-multiview-tile=\"" + escapeHTML(tile.id) + "\" data-multiview-focus=\"" + escapeHTML(tile.id) + "\"><video id=\"" + escapeHTML(tile.videoID) + "\" class=\"multiview-video\" autoplay playsinline" + (active ? "" : " muted") + "></video><div class=\"multiview-tile-controls\"><button type=\"button\" data-multiview-action=\"focus\" data-multiview-tile-id=\"" + escapeHTML(tile.id) + "\" aria-label=\"Use audio from this tile\">" + icon("speaker") + "</button><button type=\"button\" data-multiview-action=\"single\" data-multiview-tile-id=\"" + escapeHTML(tile.id) + "\" aria-label=\"Open channel player\">" + icon("external") + "</button><button type=\"button\" data-multiview-action=\"remove\" data-multiview-tile-id=\"" + escapeHTML(tile.id) + "\" aria-label=\"Remove from multiview\">" + icon("x") + "</button></div><div class=\"multiview-tile-meta\"><div><strong data-overflow-tooltip=\"" + escapeHTML(title) + "\">" + escapeHTML(title) + "</strong><small data-overflow-tooltip=\"" + escapeHTML(channel.name || subtitle) + "\">" + escapeHTML(channel.name || subtitle) + "</small></div><span class=\"multiview-audio-badge\">" + escapeHTML(muted) + "</span></div></article>";
}
function renderMultiviewEmpty() {
  return "<div class=\"multiview-empty\"><div class=\"empty\">Add up to four live channels. The active tile is the only one with audio.</div>" + renderMultiviewPicker() + "</div>";
}
function multiviewChannelMatchesQuery(channel, query) {
  if (!query) return true;
  const program = currentProgram(channel) || {};
  return lower(channel.name).indexOf(query) !== -1
    || lower(channel.categoryName).indexOf(query) !== -1
    || lower(program.title).indexOf(query) !== -1;
}
function multiviewCandidateChannels(limit) {
  const selected = {};
  items(state.multiviewTiles).forEach(function(tile) {
    const id = (tile.channel || {}).id || tile.channelID;
    if (id) selected[id] = true;
  });
  const query = lower(state.multiviewQuery);
  const picks = recentChannels(12).concat(orderedFavoriteChannels(effectiveChannels(false))).concat(visibleChannels(false)).filter(Boolean);
  const unique = [];
  const seen = {};
  picks.forEach(function(channel) {
    if (!channel || seen[channel.id] || selected[channel.id]) return;
    if (!multiviewChannelMatchesQuery(channel, query)) return;
    seen[channel.id] = true;
    unique.push(channel);
  });
  return unique.slice(0, limit || 12);
}
function renderMultiviewPicker() {
  const unique = multiviewCandidateChannels(12);
  const summary = state.multiviewQuery ? unique.length + " matching channel" + (unique.length === 1 ? "" : "s") : "Recent and favorite channels";
  return "<div id=\"multiview-picker\" class=\"multiview-picker\"><div class=\"multiview-picker-head\"><strong>Add channel</strong><span>" + escapeHTML(summary) + "</span></div><label class=\"multiview-search\"><span>" + icon("search") + "</span><input id=\"multiview-search\" placeholder=\"Search channels or programs\" value=\"" + escapeHTML(state.multiviewQuery) + "\" autocomplete=\"off\"></label><div class=\"multiview-channel-grid\">" + (unique.length ? unique.map(function(channel) {
    return "<button class=\"multiview-channel-add\" type=\"button\" data-multiview-channel=\"" + escapeHTML(channel.id) + "\">" + logoHTML(channel) + "<span><strong>" + escapeHTML(channel.name || "Untitled") + "</strong><small>" + escapeHTML(channel.categoryName || "Live TV") + "</small></span></button>";
  }).join("") : "<div class=\"multiview-no-results\">No matching channels.</div>") + "</div></div>";
}
function attachMultiviewPlayers() {
  const missingFormats = items(state.multiviewTiles).map(function(tile) { return tile.channel && tile.channel.streamFormat; }).filter(function(format) {
    return (format === "hls" && !window.Hls) || (format === "mpegts" && !window.mpegts) || (!format && (!window.Hls || !window.mpegts));
  });
  if (missingFormats.length) {
    Promise.all(missingFormats.map(ensurePlayerLibraries)).then(attachMultiviewPlayers).catch(function() {
      showAppToast("Playback components could not be loaded.");
    });
    return;
  }
  items(state.multiviewTiles).forEach(function(tile) {
    const video = byId(tile.videoID);
    if (!video || tile.attached || !tile.channel) return;
    const attachment = attachVideoSource(video, browserStreamURL(tile.channel), { rewindable: isRewindableChannel(tile.channel), format: tile.channel.streamFormat });
    tile.hls = attachment.hls;
    tile.tsPlayer = attachment.tsPlayer;
    tile.attached = true;
    video.addEventListener("click", function() { focusMultiviewTile(tile.id); });
    video.addEventListener("dblclick", function() { openMultiviewTileSingle(tile.id); });
    video.play().catch(function() {});
    startMultiviewWatch(tile);
  });
  syncMultiviewAudio();
}
function addChannelToMultiview(channel) {
  if (!channel) return;
  if (state.view === "player") {
    stopPlayback();
    stopCurrentWatch("open_multiview");
  }
  const existing = state.multiviewTiles.find(function(tile) { return tile.channel && tile.channel.id === channel.id; });
  if (existing) {
    state.multiviewActiveTileID = existing.id;
    state.view = "multiview";
    render();
    return;
  }
  if (state.multiviewTiles.length >= 4) {
    showAppToast("Multiview supports up to four channels.");
    state.view = "multiview";
    render();
    return;
  }
  const tile = { id: multiviewTileKey(channel.id), channelID: channel.id, channel: channel, videoID: multiviewTileKey("video-" + channel.id), hls: null, tsPlayer: null, session: null, attached: false };
  state.multiviewTiles.push(tile);
  state.multiviewActiveTileID = tile.id;
  state.view = "multiview";
  render();
}
function focusMultiviewTile(tileID) {
  if (!multiviewTileByID(tileID)) return;
  state.multiviewActiveTileID = tileID;
  syncMultiviewAudio();
}
function removeMultiviewTile(tileID) {
  const tile = multiviewTileByID(tileID);
  if (!tile) return;
  destroyMultiviewMedia(tile);
  stopMultiviewWatch(tile, "remove_multiview_tile");
  state.multiviewTiles = state.multiviewTiles.filter(function(item) { return item.id !== tileID; });
  if (state.multiviewActiveTileID === tileID) state.multiviewActiveTileID = state.multiviewTiles[0] ? state.multiviewTiles[0].id : "";
  renderMultiviewPage();
}
function openMultiviewTileSingle(tileID) {
  const tile = multiviewTileByID(tileID);
  if (!tile || !tile.channel) return;
  stopAllMultiview("open_single_player");
  playChannel(tile.channel);
}
function handleMultiviewAction(action, tileID) {
  if (action === "clear") {
    stopAllMultiview("clear_multiview");
    renderMultiviewPage();
    return;
  }
  if (action === "focus") focusMultiviewTile(tileID);
  if (action === "remove") removeMultiviewTile(tileID);
  if (action === "single") openMultiviewTileSingle(tileID);
}
function hasOpenPlayerOverlay() {
  return state.audioMenuOpen || state.volumeMenuOpen || state.moreMenuOpen || state.playerGuideOpen;
}
function playerChromeHasFocus() {
  const active = document.activeElement;
  return !!(active && active.closest && active.closest(".player-top, .player-bottom, .player-guide-panel"));
}
function updatePlayerChrome() {
  const shell = document.querySelector(".playback-shell");
  if (!shell) return;
  shell.classList.toggle("is-idle", state.playerChromeIdle && !hasOpenPlayerOverlay() && !playerChromeHasFocus());
}
function wakePlayerChrome(delay) {
  if (state.view !== "player") return;
  state.playerChromeIdle = false;
  updatePlayerChrome();
  if (state.playerChromeTimer) clearTimeout(state.playerChromeTimer);
  state.playerChromeTimer = setTimeout(function() {
    if (playerChromeHasFocus()) {
      wakePlayerChrome();
      return;
    }
    state.playerChromeIdle = true;
    updatePlayerChrome();
  }, delay || 2400);
}
function renderPlayerGuidePanel() {
  const panel = byId("player-guide-panel");
  const button = byId("player-guide-button");
  if (!panel) return;
  const query = state.playerGuideQuery || "";
  const channels = visibleChannels(true).filter(function(channel) { return playerGuideMatches(channel, query); }).slice(0, 60);
  panel.classList.toggle("open", state.playerGuideOpen);
  if (button) {
    button.classList.toggle("active", state.playerGuideOpen);
    button.setAttribute("aria-expanded", state.playerGuideOpen ? "true" : "false");
  }
  updatePlayerChrome();
  if (!state.playerGuideOpen) return;
  panel.innerHTML = "<div class=\"player-guide-head\"><div class=\"player-guide-title\"><strong>Channel Guide</strong><span>" + escapeHTML(categoryName(state.category) || "Live TV") + "</span></div><button class=\"player-icon\" data-player-action=\"guide-close\" aria-label=\"Close guide\">" + icon("x") + "</button><label class=\"player-guide-search\"><span>" + icon("search") + "</span><input id=\"player-guide-search\" value=\"" + escapeHTML(query) + "\" placeholder=\"Search channels or programs\" autocomplete=\"off\" aria-label=\"Search channel guide\"></label></div><div class=\"player-guide-list\">" + (channels.length ? channels.map(function(channel) {
    const lines = playerGuideProgramLines(channel);
    return "<div class=\"player-guide-row" + (state.currentChannel && state.currentChannel.id === channel.id ? " active" : "") + "\"><button class=\"player-guide-select\" type=\"button\" data-channel=\"" + escapeHTML(channel.id) + "\">" + logoHTML(channel) + "<span><strong>" + escapeHTML(channel.name || "Untitled") + "</strong><small>" + escapeHTML(lines.primary) + "</small>" + (lines.secondary ? "<small>" + escapeHTML(lines.secondary) + "</small>" : "") + "</span></button><button class=\"player-guide-add\" type=\"button\" data-player-guide-multiview=\"" + escapeHTML(channel.id) + "\" aria-label=\"Add " + escapeHTML(channel.name || "channel") + " to multiview\">" + icon("multiview") + "</button></div>";
  }).join("") : "<div class=\"player-guide-empty\">No matching channels.</div>") + "</div>";
}
function currentStreamURL() {
  return state.currentChannel ? route("/dispatcharr/stream?channel_id=" + encodeURIComponent(state.currentChannel.id)) : "";
}
function browserStreamURL(channel) {
  return route("/dispatcharr/stream?channel_id=" + encodeURIComponent(channel.id) + "&output_profile=2");
}
function stopTimeShiftSession() {
  state.timeShiftAttempt += 1;
  const session = state.timeShiftSession;
  state.timeShiftSession = null;
  if (state.timeShiftHeartbeat) {
    clearInterval(state.timeShiftHeartbeat);
    state.timeShiftHeartbeat = null;
  }
  if (state.timeShiftTimelineTimer) {
    clearInterval(state.timeShiftTimelineTimer);
    state.timeShiftTimelineTimer = null;
  }
  if (session && session.leaseId) postJSON("/dispatcharr/api/timeshift/stop", { leaseId: session.leaseId }).catch(function() {});
  updateTimeShiftUI();
}
async function prepareTimeShift(channel) {
  if (!liveRewindEnabled() || !channel || channel.streamFormat === "hls") return null;
  const attemptID = state.timeShiftAttempt;
  const session = await postJSON("/dispatcharr/api/timeshift/start", { channelId: channel.id });
  if (attemptID !== state.timeShiftAttempt || state.view !== "player" || !state.currentChannel || state.currentChannel.id !== channel.id) {
    postJSON("/dispatcharr/api/timeshift/stop", { leaseId: session.leaseId }).catch(function() {});
    const stale = new Error("rewind attempt superseded");
    stale.superseded = true;
    throw stale;
  }
  state.timeShiftSession = session;
  state.timeShiftHeartbeat = setInterval(function() {
    if (state.timeShiftSession && state.timeShiftSession.leaseId) postJSON("/dispatcharr/api/timeshift/heartbeat", { leaseId: state.timeShiftSession.leaseId }).catch(function() {});
  }, 30000);
  for (let pollAttempt = 0; pollAttempt < 30; pollAttempt++) {
    await new Promise(function(resolve) { setTimeout(resolve, 500); });
    if (attemptID !== state.timeShiftAttempt || state.view !== "player" || !state.timeShiftSession || state.timeShiftSession.leaseId !== session.leaseId) {
      const stale = new Error("rewind attempt superseded");
      stale.superseded = true;
      throw stale;
    }
    const status = await getJSON("/dispatcharr/api/timeshift/status?lease_id=" + encodeURIComponent(session.leaseId));
    if (status.state === "failed") throw new Error(status.error || "rewind buffer failed");
    if (status.segmentCount >= 2) {
      session.status = status;
      session.ready = true;
      return route(session.manifestPath);
    }
  }
  throw new Error("rewind buffer startup timed out");
}
function fallbackFromTimeShift(channel, message) {
  if (state.view !== "player" || !channel || !state.currentChannel || channel.id !== state.currentChannel.id) return;
  stopTimeShiftSession();
  setVideoSource(browserStreamURL(channel), { rewindable: isRewindableChannel(channel), format: channel.streamFormat });
  if (message) showPlayerToast(message);
}
function timeShiftSeek(delta) {
  const video = byId("player");
  if (!video || !video.seekable || !video.seekable.length) return;
  const start = video.seekable.start(0);
  const end = video.seekable.end(video.seekable.length - 1);
  video.currentTime = Math.max(start, Math.min(end - 0.25, video.currentTime + delta));
  updateTimeShiftUI();
}
function timeShiftGoLive() {
  const video = byId("player");
  if (!video || !video.seekable || !video.seekable.length) return;
  video.currentTime = Math.max(video.seekable.start(0), video.seekable.end(video.seekable.length - 1) - 0.5);
  video.play().catch(function() {});
  updateTimeShiftUI();
}
function updateTimeShiftUI() {
  const controls = byId("player-timeshift-controls");
  const range = byId("player-timeshift-range");
  const label = byId("player-timeshift-label");
  const tag = byId("player-mode-tag");
  const video = byId("player");
  const active = !!(state.timeShiftSession && state.timeShiftSession.ready && video && video.seekable && video.seekable.length);
  if (controls) controls.classList.toggle("hidden", !active);
  if (tag) tag.textContent = active ? "Live Rewind" : (isRewindableChannel(state.currentChannel) ? "Replay" : "AV");
  if (!active) return;
  const start = video.seekable.start(0);
  const end = video.seekable.end(video.seekable.length - 1);
  const position = Math.max(start, Math.min(end, video.currentTime || end));
  const windowSeconds = Math.max(0, end - start);
  const behind = Math.max(0, end - position);
  if (range) {
    range.max = String(windowSeconds);
    range.value = String(Math.max(0, position - start));
  }
  if (label) label.textContent = behind < 3 ? "LIVE" : "-" + Math.floor(behind / 60) + ":" + String(Math.floor(behind % 60)).padStart(2, "0");
}
function applyAspectMode() {
  const video = byId("player");
  if (video) video.style.objectFit = state.aspectMode === "fit" ? "contain" : "cover";
}
function attachVideoSource(video, url, options) {
  const rewindable = !!(options && options.rewindable);
  const attachment = {
    hls: null,
    tsPlayer: null,
    destroy: function() {
      if (attachment.hls) { attachment.hls.destroy(); attachment.hls = null; }
      if (attachment.tsPlayer) { attachment.tsPlayer.destroy(); attachment.tsPlayer = null; }
      if (video) {
        video.pause();
        video.removeAttribute("src");
        video.load();
      }
    }
  };
  const isHLS = (options && options.format === "hls") || url.indexOf(".m3u8") !== -1;
  if (window.Hls && Hls.isSupported() && isHLS) {
    attachment.hls = new Hls(rewindable ? { liveSyncDurationCount: 1, liveMaxLatencyDurationCount: 5, maxBufferLength: 60 } : {});
    if (options && typeof options.onFatal === "function") {
      let fatalHandled = false;
      attachment.hls.on(Hls.Events.ERROR, function(_, data) {
        if (!fatalHandled && data && data.fatal) {
          fatalHandled = true;
          options.onFatal(data);
        }
      });
    }
    attachment.hls.loadSource(url);
    attachment.hls.attachMedia(video);
  } else if (window.mpegts && mpegts.isSupported() && !isHLS) {
    attachment.tsPlayer = mpegts.createPlayer({ type: "mpegts", isLive: !rewindable, url: url });
    attachment.tsPlayer.attachMediaElement(video);
    attachment.tsPlayer.load();
  } else {
    video.src = url;
  }
  return attachment;
}
function renderPlayerMoreMenu() {
  const button = byId("player-more-button");
  const menu = byId("player-more-menu");
  if (!menu) return;
  if (button) button.setAttribute("aria-expanded", state.moreMenuOpen ? "true" : "false");
  menu.classList.toggle("open", state.moreMenuOpen);
  updatePlayerChrome();
  if (!state.moreMenuOpen) return;
  const recent = items(prefs().recentChannels).map(channelByID).filter(Boolean).filter(function(channel) {
    return !state.currentChannel || channel.id !== state.currentChannel.id;
  }).slice(0, 3);
  menu.innerHTML = "<div class=\"player-more-kicker\">Video settings & controls</div>"
    + "<button data-player-action=\"aspect\">" + menuIcon("aspect") + "<span>Aspect ratio<small>" + (state.aspectMode === "fit" ? "Fit to screen" : "Fill screen") + "</small></span></button>"
    + "<button data-player-action=\"fullscreen\">" + menuIcon(document.fullscreenElement ? "fullscreen-exit" : "fullscreen") + "<span>Fullscreen<small>" + (document.fullscreenElement ? "Exit player fullscreen" : "Fill the display") + "</small></span></button>"
    + "<button data-player-action=\"guide\">" + menuIcon("guide") + "<span>Channel guide<small>Browse channels without leaving playback</small></span></button>"
    + "<button data-player-action=\"add-multiview\">" + menuIcon("multiview") + "<span>Add to multiview<small>Tile this channel with up to three more</small></span></button>"
    + "<button data-player-action=\"search-channel\">" + menuIcon("search") + "<span>Search channel<small>Jump to the channel list search</small></span></button>"
    + (recent.length ? "<div class=\"player-more-separator\"></div><div class=\"player-more-kicker\">Channels history</div>" + recent.map(function(channel) { return "<button data-channel=\"" + escapeHTML(channel.id) + "\">" + logoHTML(channel) + "<span>" + escapeHTML(channel.name || "Untitled") + "<small>" + escapeHTML(channel.categoryName || "Live TV") + "</small></span></button>"; }).join("") : "")
    + "<div class=\"player-more-separator\"></div><div class=\"player-more-kicker\">Video & audio casting</div>"
    + "<button data-player-action=\"cast\">" + menuIcon("airplay") + "<span>AirPlay or Cast<small>Use browser playback target picker</small></span></button>"
    + "<button data-player-action=\"copy-stream\">" + menuIcon("copy") + "<span>Copy stream URL<small>For an external player</small></span></button>"
    + "<button data-player-action=\"open-stream\">" + menuIcon("external") + "<span>Use external video player<small>Open the stream route in a new tab</small></span></button>";
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
function renderGuidePage() {
  const categories = guideFilterCategories();
  const slots = guideSlots();
  state.guideLastSlotStart = guideSlotStart();
  byId("view").innerHTML = '<div class="guide-page">' + sectionHeaderWithActions("TV Guide", guideFreshnessHTML()) + "<div class=\"guide-tools\"><label class=\"guide-category-filter\"><span>" + icon("search") + "</span><input id=\"category-select\" list=\"guide-category-options\" placeholder=\"Filter categories\" value=\"" + escapeHTML(guideCategoryInputValue(categories)) + "\" autocomplete=\"off\" aria-label=\"Filter categories\"><datalist id=\"guide-category-options\"><option value=\"" + escapeHTML(allGroupLabel()) + "\"></option>" + categories.map(function(category) { return "<option value=\"" + escapeHTML(category.name || category.id) + "\"></option>"; }).join("") + "</datalist></label><input id=\"guide-search\" class=\"search\" placeholder=\"Search by program or channel\" value=\"" + escapeHTML(state.query) + "\"></div><div id=\"guide-scroll\" class=\"guide-scroll\"><div class=\"guide-timeline\" style=\"" + guideTimelineStyle(slots) + "\"><div class=\"time-head\"><span>Today</span>" + slots.map(function(slot) { return "<span>" + escapeHTML(timeLabel(slot)) + "</span>"; }).join("") + '</div><div id="epg" class="guide-window-spacer" style="height:0px"><div class="guide-window" style="transform:translateY(0px)"></div></div></div></div></div>';
  byId("category-select").oninput = function(event) { updateGuideCategoryFilter(event.target.value, categories, false); };
  byId("category-select").onchange = function(event) { updateGuideCategoryFilter(event.target.value, categories, true); };
  byId("category-select").onblur = function(event) { event.target.value = guideCategoryInputValue(categories); };
  byId("guide-search").oninput = function(event) { state.query = event.target.value; resetGuideRows(); renderEPG(); };
  const guideScroll = byId("guide-scroll");
  if (guideScroll) guideScroll.onscroll = scheduleGuideWindowRender;
  resetGuideRows();
  maybeWarmGuideForChannels(state.guideChannels.slice(0, guideWindowOverscan() * 2), "guide:" + (state.category || "all"));
  renderEPG();
}
function guideCategoryInputValue(categories) {
  if (!state.category) return allGroupLabel();
  const category = items(categories).find(function(item) { return item.id === state.category; });
  return category ? category.name || category.id : "";
}
function guideCategoryFromInput(value, categories) {
  const query = lower(String(value || "").trim());
  if (!query || query === lower(allGroupLabel())) return "";
  const exact = items(categories).find(function(category) {
    return lower(category.name || category.id) === query || lower(category.id) === query;
  });
  return exact ? exact.id : null;
}
function updateGuideCategoryFilter(value, categories, forceReset) {
  const next = guideCategoryFromInput(value, categories);
  if (next === "" && !forceReset) return;
  if (next === null) {
    if (forceReset) renderGuidePage();
    return;
  }
  if (state.category === next) return;
  state.category = next;
  renderGuidePage();
}
function guideWindowOverscan() { return 8; }
function guideRowHeight() {
  const scroll = byId("guide-scroll");
  const value = scroll ? getComputedStyle(scroll).getPropertyValue("--epg-row-h").trim() : "";
  const number = parseFloat(value);
  if (!number) return 70;
  return value.indexOf("rem") !== -1 ? number * parseFloat(getComputedStyle(document.documentElement).fontSize || "16") : number;
}
function resetGuideRows() {
  state.guideChannels = visibleChannels(true).filter(guideChannelMatchesQuery);
  state.guideRendered = 0;
  state.guideLoading = false;
  state.guideWindowStart = -1;
  state.guideWindowEnd = -1;
  if (state.guideRenderFrame) cancelAnimationFrame(state.guideRenderFrame);
  state.guideRenderFrame = 0;
}
function renderEPGCells(channel, channelIndex) {
  const windowInfo = guideWindow();
  const windowStart = windowInfo.start;
  const windowEnd = windowInfo.end;
  const now = Math.floor(Date.now() / 1000);
  const channelMatched = channelMatchesQuery(channel);
  const programs = programsFor(channel.id).map(function(program) {
    const rawStart = program.startUnix || windowStart;
    const rawEnd = program.endUnix || rawStart + 1800;
    return {
      program: program,
      start: Math.max(rawStart, windowStart),
      end: Math.min(rawEnd, windowEnd),
      matchesQuery: channelMatched || programMatchesQuery(program)
    };
  }).filter(function(entry) {
    return entry.matchesQuery && entry.end > windowStart && entry.start < windowEnd;
  }).sort(function(a, b) {
    return a.start - b.start || a.end - b.end;
  });
  if (!programs.length) {
    return renderEPGGapCell(channel, windowStart, windowEnd, windowInfo);
  }
  const cells = [];
  let cursor = windowStart;
  programs.forEach(function(entry) {
    const program = entry.program;
    const start = Math.max(entry.start, cursor);
    const end = entry.end;
    if (end <= start) return;
    if (start > cursor) cells.push(renderEPGGapCell(channel, cursor, start, windowInfo));
    const canSchedule = recordingSchedulingEnabled() && (program.endUnix || 0) > now;
    const isLive = start <= now && end > now;
    const programTitle = programIsGuidePlaceholder(program) ? guideUnavailableLabel() : program.title || guideUnavailableLabel();
    const titleParts = epgProgramTitleParts(programTitle);
    const accessibleTitle = titleParts.live ? titleParts.title + " Live" : titleParts.title;
    const programTime = epgVisibleTime(start, windowStart);
    cells.push("<div class=\"epg-cell program" + (isLive ? " live" : "") + "\" style=\"" + epgCellStyle(start, end, windowInfo) + "\"><button class=\"epg-play\" data-program-detail-channel=\"" + escapeHTML(channel.id) + "\" data-program-detail=\"" + escapeHTML(program.id || "") + "\" aria-label=\"" + escapeHTML(programTime + " " + accessibleTitle) + "\"><time>" + escapeHTML(programTime) + "</time><strong>" + escapeHTML(titleParts.title) + (titleParts.live ? "<span class=\"epg-live-marker\" aria-hidden=\"true\">" + escapeHTML(titleParts.marker) + "</span>" : "") + "</strong></button>" + (canSchedule ? "<button class=\"epg-schedule\" data-schedule-channel=\"" + escapeHTML(channel.id) + "\" data-schedule-program=\"" + escapeHTML(program.id || "") + "\" aria-label=\"Schedule recording\">" + icon("record") + "</button>" : "") + "</div>");
    cursor = end;
  });
  if (cursor < windowEnd) cells.push(renderEPGGapCell(channel, cursor, windowEnd, windowInfo));
  return cells.join("");
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
function renderEPGGapCell(channel, startUnix, endUnix, windowInfo) {
  if (endUnix <= startUnix) return "";
  const emptyTitle = guideUnavailableLabel();
  const emptyTime = timeLabel(startUnix);
  return "<button class=\"epg-cell program epg-gap\" data-channel=\"" + escapeHTML(channel.id) + "\" aria-label=\"" + escapeHTML(emptyTime + " " + emptyTitle) + "\" style=\"" + epgCellStyle(startUnix, endUnix, windowInfo) + "\"><time>" + escapeHTML(emptyTime) + "</time><strong>" + escapeHTML(emptyTitle) + "</strong></button>";
}
function renderEPGRow(channel, channelIndex) {
  return "<div class=\"epg-row\">" + renderGuideChannelButton(channel) + "<div class=\"epg-programs\">" + renderEPGCells(channel, channelIndex) + "</div></div>";
}
function renderEPG() {
  renderGuideWindow(true);
}
function scheduleGuideWindowRender() {
  if (state.guideRenderFrame) return;
  state.guideRenderFrame = requestAnimationFrame(function() {
    state.guideRenderFrame = 0;
    renderGuideWindow(false);
  });
}
function guideVisibleRange(totalRows, scrollTop, viewportHeight, rowHeight, headerHeight) {
  const visibleRows = Math.max(1, Math.ceil(Math.max(0, viewportHeight) / rowHeight));
  const overscan = guideWindowOverscan();
  const rowsScrollTop = Math.max(0, scrollTop - headerHeight);
  const start = Math.max(0, Math.floor(rowsScrollTop / rowHeight) - overscan);
  const end = Math.min(totalRows, start + Math.min(60, visibleRows + overscan * 2));
  return { start: start, end: end };
}
function renderGuideWindow(force) {
  if (state.view !== "guide" || state.guideLoading) return;
  const root = byId("epg");
  if (!root) return;
  if (!state.guideChannels.length) {
    state.guideWindowStart = -1;
    state.guideWindowEnd = -1;
    root.style.height = "auto";
    root.innerHTML = "<div class=\"guide-window\" style=\"transform:translateY(0px)\"><div class=\"empty\">No guide matches.</div></div>";
    return;
  }
  const guideScroll = byId("guide-scroll");
  const rowHeight = guideRowHeight();
  const timeHead = guideScroll ? guideScroll.querySelector(".time-head") : null;
  const range = guideVisibleRange(state.guideChannels.length, guideScroll ? guideScroll.scrollTop : 0, guideScroll ? guideScroll.clientHeight : window.innerHeight, rowHeight, timeHead ? timeHead.offsetHeight : 0);
  const start = range.start;
  const end = range.end;
  if (!force && start === state.guideWindowStart && end === state.guideWindowEnd) return;
  state.guideLoading = true;
  const rows = state.guideChannels.slice(start, end).map(function(channel, offset) {
    return renderEPGRow(channel, start + offset);
  }).join("");
  state.guideRendered = end;
  state.guideWindowStart = start;
  state.guideWindowEnd = end;
  root.style.height = (state.guideChannels.length * rowHeight) + "px";
  root.innerHTML = "<div class=\"guide-window\" style=\"transform:translateY(" + (start * rowHeight) + "px)\">" + rows + "</div>";
  state.guideLoading = false;
}
function renderSettings() {
  ensureSelectedCustomGroup();
  const showSourceCategorySettings = !virtualCategoriesActive();
  byId("view").innerHTML = "<div class=\"settings-stack\">"
    + (isDispatcharrDirectSource() ? "<div class=\"settings-card profile-settings-card\"><div class=\"settings-card-head\"><div><h2>Live TV profiles</h2><p>Choose which Dispatcharr profile lineups appear in your Live TV experience.</p></div><span id=\"profile-selection-summary\" class=\"profile-selection-summary\"></span></div><div id=\"profile-settings\"></div></div>" : "")
    + "<div class=\"settings-card custom-groups-card\"><h2>Custom groups</h2><div id=\"custom-group-settings\"></div></div>"
    + (showSourceCategorySettings ? "<div class=\"settings-card\"><h2>Hidden channel groups</h2><div id=\"settings-list\" class=\"settings-list\"></div></div>" : "")
    + "</div>";
  renderProfileSettings();
  renderCustomGroupSettings();
  if (!showSourceCategorySettings) return;
  const root = byId("settings-list");
  const categories = sourceCategoriesWithChannels();
  root.innerHTML = categories.map(function(category) {
    return "<label><span>" + escapeHTML(category.name || category.sourceID) + "</span><input type=\"checkbox\" data-hide=\"" + escapeHTML(category.sourceID) + "\"" + (hiddenMap()[category.sourceID] ? " checked" : "") + "></label>";
  }).join("") || "<div class=\"empty\">No channel groups available for this connection.</div>";
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
function adminSaveStatusHTML() {
  if (!state.adminSaveMessage) return "";
  const warning = state.adminSaveStatus === "error" || state.adminSaveStatus === "dirty";
  return "<div class=\"settings-note" + (warning ? " settings-warning" : "") + "\">" + escapeHTML(state.adminSaveMessage) + "</div>";
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
function updateAdminECMField(field, target) {
  const settings = state.adminCategorySettings || defaultAdminCategorySettings();
  if (field === "url") settings.ecmURL = target.value;
  state.adminCategorySettings = settings;
  normalizeAdminCategorySettings();
  if (!adminECMEnabled() && state.adminTab === "manager") state.adminTab = "integrations";
  markAdminSettingsDraft();
  renderAdminPage();
}
function updateAdminRecordingField(field, target) {
  const settings = state.adminCategorySettings || defaultAdminCategorySettings();
  if (field === "default") settings.allowRecordingsByDefault = !!target.checked;
  state.adminCategorySettings = settings;
  normalizeAdminCategorySettings();
  markAdminSettingsDraft();
  renderAdminPage();
}
function updateAdminPlayerField(field, target) {
  const settings = state.adminCategorySettings || defaultAdminCategorySettings();
  if (field === "sports") settings.sportsFirstPlayerEnabled = !!target.checked;
  state.adminCategorySettings = settings;
  normalizeAdminCategorySettings();
  markAdminSettingsDraft();
  renderAdminPage();
}
function updateAdminTimeShiftField(field, target) {
  const settings = state.adminCategorySettings || defaultAdminCategorySettings();
  if (field === "enabled") settings.liveRewindEnabled = !!target.checked;
  if (field === "cache") settings.liveRewindCacheGB = Number(target.value || 5);
  if (field === "window") settings.liveRewindWindowMinutes = Number(target.value || 30);
  if (field === "free") settings.liveRewindMinFreeGB = Number(target.value || 2);
  if (field === "channels") settings.liveRewindMaxChannels = Number(target.value || 20);
  state.adminCategorySettings = settings;
  normalizeAdminCategorySettings();
  markAdminSettingsDraft();
  renderAdminPage();
}
function renderAdminPage() {
  normalizeAdminCategorySettings();
  if (!adminECMEnabled() && state.adminTab === "manager") state.adminTab = "integrations";
  renderAdminTopbarTabs();
  renderAdminTopbarActions();
  const shell = document.querySelector(".shell");
  if (shell) shell.classList.toggle("is-admin-manager", state.adminTab === "manager");
  byId("view").innerHTML = state.adminTab === "manager" ? renderExternalChannelManager() : "<div class=\"settings-stack\">" + (state.adminTab === "integrations" ? renderAdminIntegrationsTab() : renderAdminSettingsTab()) + "</div>";
  if (state.adminTab === "settings") {
    renderAdminRecordingSettings();
    renderAdminPlayerSettings();
    renderAdminTimeShiftSettings();
    renderAdminCategorySettings();
    renderAdminCategoryAliasSettings();
    renderAdminEventKeywordSettings();
  }
  if (state.adminTab === "integrations") {
    renderAdminECMSettings();
  }
}
function renderAdminTopbarTabs() {
  const root = byId("admin-tabs");
  if (!root) return;
  root.innerHTML = "<button type=\"button\" data-admin-tab=\"settings\" class=\"" + (state.adminTab === "settings" ? "active" : "") + "\">" + icon("settings") + "<span>Settings</span></button>"
    + "<button type=\"button\" data-admin-tab=\"integrations\" class=\"" + (state.adminTab === "integrations" ? "active" : "") + "\">" + icon("integrations") + "<span>Integrations</span></button>"
    + (adminECMEnabled() ? "<button type=\"button\" data-admin-tab=\"manager\" class=\"" + (state.adminTab === "manager" ? "active" : "") + "\">" + icon("external") + "<span>Channel Manager</span></button>" : "");
}
function renderAdminTopbarActions() {
  const root = byId("admin-actions");
  if (!root) return;
  if (state.adminTab === "manager") {
    root.innerHTML = "";
    return;
  }
  const dirty = adminSettingsDirty();
  const saving = state.adminSaveStatus === "saving";
  root.innerHTML = "<button class=\"admin-save\" data-admin-settings-action=\"save\"" + ((!dirty || saving) ? " disabled" : "") + ">Save</button><button class=\"admin-discard\" data-admin-settings-action=\"discard\"" + ((!dirty || saving) ? " disabled" : "") + ">Discard</button>";
}
function setAdminTab(tab) {
  if (tab === "manager" && adminECMEnabled()) state.adminTab = "manager";
  else if (tab === "integrations") state.adminTab = "integrations";
  else state.adminTab = "settings";
  renderAdminPage();
}
function renderAdminSettingsTab() {
  return ""
    + adminStatusPanel()
    + "<div class=\"settings-card settings-card-compact\"><h2>Recordings</h2><div id=\"admin-recording-settings\" class=\"settings-list\"></div></div>"
    + "<div class=\"settings-card settings-card-compact\"><h2>Player</h2><div id=\"admin-player-settings\" class=\"settings-list\"></div></div>"
    + "<div class=\"settings-card\"><div class=\"settings-card-head\"><div><h2>Live Rewind</h2><p>Bounded shared channel buffers for pause and rewind.</p></div></div><div id=\"admin-timeshift-settings\" class=\"settings-list\"></div></div>"
    + "<div class=\"settings-card settings-card-compact\"><h2>Group method</h2><div id=\"admin-category-settings\" class=\"settings-list\"></div></div>"
    + "<div class=\"settings-card\"><div class=\"settings-card-head\"><div><h2>Presentation Overrides</h2><p>Add alternate virtual group paths without changing the original Dispatcharr groups.</p></div></div><div id=\"admin-category-alias-settings\" class=\"settings-list\"></div></div>"
    + "<div class=\"settings-card\"><div class=\"settings-card-head\"><div><h2>Event Keywords</h2><p>Events are detected from the Dispatcharr guide. One keyword per line or comma-separated.</p></div></div><div id=\"admin-event-keyword-settings\" class=\"settings-list event-keyword-list\"></div></div>"
    + "";
}
function renderAdminRecordingSettings() {
  const root = byId("admin-recording-settings");
  if (!root) return;
  const settings = adminSettings();
  const available = !!(state.app && state.app.capabilities && state.app.capabilities.recordings && isDispatcharrDirectSource());
  const canSchedule = recordingSchedulingEnabled();
  const description = !available ? "Recordings require Dispatcharr Direct Connect." : (canSchedule ? "Show recording controls for Dispatcharr Direct users." : recordingScheduleReason());
  root.innerHTML = "<label class=\"settings-row compact-row\"><span><strong>Allow recordings by default</strong><small>" + escapeHTML(description) + "</small></span><input type=\"checkbox\" data-admin-recording-field=\"default\"" + (settings.allowRecordingsByDefault !== false ? " checked" : "") + (canSchedule ? "" : " disabled") + "></label>";
}
function renderAdminPlayerSettings() {
  const root = byId("admin-player-settings");
  if (!root) return;
  const settings = adminSettings();
  root.innerHTML = "<label class=\"settings-row compact-row\"><span><strong>Sports-first player</strong><small>Add a live score and matched-channel drawer to the video player.</small></span><input type=\"checkbox\" data-admin-player-field=\"sports\"" + (settings.sportsFirstPlayerEnabled ? " checked" : "") + "></label>";
}
function byteSizeLabel(value) {
  const bytes = Number(value || 0);
  if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(bytes >= 10737418240 ? 0 : 1) + " GB";
  if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + " MB";
  return Math.max(0, Math.round(bytes / 1024)) + " KB";
}
function renderAdminTimeShiftSettings() {
  const root = byId("admin-timeshift-settings");
  if (!root) return;
  const settings = adminSettings();
  const status = state.timeShiftAdminStatus || {};
  const direct = isDispatcharrDirectSource();
  const usage = state.timeShiftAdminLoading ? "Loading cache usage..." : (status.unavailable ? "Cache status is unavailable." : (state.timeShiftAdminStatus ? byteSizeLabel(status.bytes) + " of " + byteSizeLabel(status.maxBytes) + " · " + String(status.activeBuffers || 0) + " buffered channels · " + String(status.activeLeases || 0) + " viewers" : "Usage has not been checked yet."));
  const windows = [15, 30, 60, 90, 120].map(function(minutes) { return "<option value=\"" + minutes + "\"" + (Number(settings.liveRewindWindowMinutes) === minutes ? " selected" : "") + ">" + minutes + " minutes</option>"; }).join("");
  root.innerHTML = adminSaveStatusHTML()
    + "<label class=\"settings-row compact-row\"><span><strong>Enable Live Rewind</strong><small>Dispatcharr Direct MPEG-TS only. Unsupported channels fall back to normal playback.</small></span><input type=\"checkbox\" data-admin-timeshift-field=\"enabled\"" + (settings.liveRewindEnabled ? " checked" : "") + (direct ? "" : " disabled") + "></label>"
    + "<div class=\"settings-row settings-form-row\"><span class=\"settings-field-copy\"><strong>Total cache budget</strong><small>Shared across every user and buffered channel.</small></span><div class=\"settings-number-unit\"><input type=\"number\" min=\"1\" max=\"500\" step=\"1\" data-admin-timeshift-field=\"cache\" value=\"" + escapeHTML(String(settings.liveRewindCacheGB)) + "\"><span>GB</span></div></div>"
    + "<div class=\"settings-row settings-form-row\"><span class=\"settings-field-copy\"><strong>Maximum rewind window</strong><small>Oldest segments are removed first.</small></span><select data-admin-timeshift-field=\"window\">" + windows + "</select></div>"
    + "<div class=\"settings-row settings-form-row\"><span class=\"settings-field-copy\"><strong>Minimum free disk space</strong><small>Eviction starts before the server reaches this reserve.</small></span><div class=\"settings-number-unit\"><input type=\"number\" min=\"1\" max=\"100\" step=\"1\" data-admin-timeshift-field=\"free\" value=\"" + escapeHTML(String(settings.liveRewindMinFreeGB)) + "\"><span>GB</span></div></div>"
    + "<div class=\"settings-row settings-form-row\"><span class=\"settings-field-copy\"><strong>Maximum buffered channels</strong><small>Additional channels continue with normal live playback.</small></span><input type=\"number\" min=\"1\" max=\"100\" step=\"1\" data-admin-timeshift-field=\"channels\" value=\"" + escapeHTML(String(settings.liveRewindMaxChannels)) + "\"></div>"
    + "<div class=\"settings-row compact-row timeshift-usage-row\"><span><strong>Cache usage</strong><small>" + escapeHTML(usage) + "</small></span><div class=\"settings-inline-actions\"><button type=\"button\" data-timeshift-admin-action=\"refresh\">Refresh</button><button type=\"button\" data-timeshift-admin-action=\"clear\" class=\"danger\">Clear cache</button></div></div>";
  if (!state.timeShiftAdminStatus && !state.timeShiftAdminLoading) refreshAdminTimeShiftStatus(true);
}
async function refreshAdminTimeShiftStatus(quiet) {
  if (state.timeShiftAdminLoading) return;
  state.timeShiftAdminLoading = true;
  if (!quiet) renderAdminPage();
  try {
    state.timeShiftAdminStatus = await getJSON("/dispatcharr/api/timeshift/admin-status");
  } catch (_) {
    state.timeShiftAdminStatus = { unavailable: true };
    if (!quiet) showAppToast("Live Rewind cache status is unavailable.");
  } finally {
    state.timeShiftAdminLoading = false;
    if (state.view === "admin" && state.adminTab === "settings") renderAdminPage();
  }
}
async function clearAdminTimeShiftCache() {
  try {
    await postJSON("/dispatcharr/api/timeshift/clear", {});
    state.timeShiftAdminStatus = null;
    showAppToast("Live Rewind cache cleared.");
    refreshAdminTimeShiftStatus(true);
  } catch (_) {
    showAppToast("Live Rewind cache could not be cleared.");
  }
}
function renderAdminIntegrationsTab() {
  return ""
    + "<div class=\"settings-card integrations-card\"><div class=\"settings-card-head\"><div><h2>ECM</h2><p>Embed Enhanced Channel Manager for Dispatcharr channel work.</p></div></div><div id=\"admin-ecm-settings\" class=\"settings-list\"></div></div>"
    + "";
}
function adminStatusPill(status) {
  status = String(status || "ok").toLowerCase();
  const label = status === "failed" ? "Error" : (status === "loading" ? "Updating" : (status === "error" ? "Error" : (status === "unavailable" ? "Unavailable" : (status === "all_access" ? "All access" : (status === "empty" ? "Not assigned" : (status === "available" ? "Available" : "Healthy"))))));
  const cls = status === "loading" ? " loading" : ((status === "error" || status === "failed" || status === "unavailable") ? " error" : ((status === "empty" || status === "all_access") ? " warning" : ""));
  return "<span class=\"admin-status-pill" + cls + "\">" + escapeHTML(label) + "</span>";
}
function plainTextFromHTML(value) {
  return String(value || "").replace(/<[^>]*>/g, "").replace(/\s+/g, " ").trim();
}
function adminStatusItem(label, value, detail) {
  return "<div class=\"admin-status-item\" title=\"" + escapeHTML((label ? label + ": " : "") + (detail || plainTextFromHTML(value))) + "\"><span>" + escapeHTML(label) + "</span><strong>" + value + "</strong>" + (detail ? "<small>" + escapeHTML(detail) + "</small>" : "") + "</div>";
}
function adminStatusPanel() {
  const status = state.app && state.app.status ? state.app.status : {};
  const source = state.app && state.app.source ? state.app.source : {};
  const profileAccess = source.profileAccess || {};
  const guideStatus = String(status.epgStatus || (status.epgProgramCount ? "ok" : "unknown"));
  const error = status.lastError || status.epgLastError || "";
  const profileStatus = String(profileAccess.status || (items(source.profiles).length ? "available" : "empty"));
  const profileCount = Number(profileAccess.profileCount || items(source.profiles).length || 0);
  const membershipCount = Number(profileAccess.channelMembershipCount || 0);
  const profileValue = profileStatus === "available" ? escapeHTML(String(profileCount)) : adminStatusPill(profileStatus);
  const profileDetail = profileStatus === "available" ? String(membershipCount) + " channel memberships" : (profileAccess.message || "No profile information returned by Dispatcharr.");
  const refreshClass = "admin-status-refresh" + (state.adminStatusRefreshing ? " is-loading" : "");
  const refreshLabel = state.adminStatusRefreshing ? "Refreshing..." : "Refresh";
  return "<div class=\"admin-status-strip\" aria-label=\"Connection Status\"><div class=\"settings-card-head\"><div><h2>Connection Status</h2></div><button type=\"button\" class=\"" + refreshClass + "\" data-admin-status-refresh=\"true\" aria-label=\"Refresh connection status\"" + (state.adminStatusRefreshing ? " disabled aria-busy=\"true\"" : "") + ">" + icon("loader") + "<span>" + refreshLabel + "</span></button></div><div class=\"admin-status-grid\">"
    + adminStatusItem("Connection", adminStatusPill(status.status || "ok"), sourceModeLabel(source.mode))
    + adminStatusItem("Channels", escapeHTML(String(status.channelCount || items(state.app.channels).length || 0)), "Last catalog sync: " + dateTimeLabel(status.lastSuccessUnix))
    + adminStatusItem("Guide", adminStatusPill(guideStatus), String(status.epgProgramCount || items(state.app.programs).length || 0) + " programs · " + dateTimeLabel(status.epgLastSuccessUnix))
    + (isDispatcharrDirectSource() ? adminStatusItem("Profiles", profileValue, profileDetail) : "")
    + "</div>"
    + (error ? "<div class=\"settings-note settings-warning admin-status-note\">" + escapeHTML(error) + "</div>" : "")
    + (isDispatcharrDirectSource() && profileStatus !== "available" ? "<div class=\"settings-note settings-warning admin-status-note is-actionable\"><span>" + escapeHTML(profileDetail) + "</span><button type=\"button\" data-admin-profile-refresh=\"true\"" + (state.adminProfileRefreshing ? " disabled" : "") + ">" + (state.adminProfileRefreshing ? "Refreshing..." : "Retry profiles") + "</button></div>" : "")
    + "</div>";
}

async function refreshAdminStatus() {
  if (state.adminStatusRefreshing) return;
  state.adminStatusRefreshing = true;
  renderAdminPage();
  try {
    await refreshStatusData();
    showAppToast("Connection status refreshed.");
  } catch (error) {
    showAppToast("Could not refresh connection status.");
    try { console.warn("Dispatcharr admin status refresh failed", error); } catch (_) {}
  } finally {
    state.adminStatusRefreshing = false;
    renderAdminPage();
  }
}

async function refreshAdminProfiles() {
  if (state.adminProfileRefreshing) return;
  state.adminProfileRefreshing = true;
  renderAdminPage();
  try {
    await hydrateApp(await postJSON("/dispatcharr/api/refresh-channels", {}), { reuseSettings: true });
    for (let attempt = 0; attempt < 150; attempt++) {
      const profileAccess = state.app && state.app.source ? state.app.source.profileAccess || {} : {};
      const refresh = state.app && state.app.status ? state.app.status.refresh || {} : {};
      const refreshState = String(refresh.state || "").toLowerCase();
      if (profileAccess.status === "available") {
        showAppToast("Channel profiles refreshed.");
        return;
      }
      if (refreshState === "failed" || refreshState === "canceled") throw new Error(refresh.error || "profile refresh did not complete");
      await new Promise(function(resolve) { setTimeout(resolve, 2000); });
      await refreshStatusData();
    }
    throw new Error("profile refresh timed out");
  } catch (error) {
    showAppToast("Dispatcharr profile refresh failed.");
  } finally {
    state.adminProfileRefreshing = false;
    renderAdminPage();
  }
}
function renderExternalChannelManager() {
  const managerURL = adminECMURL();
  return "<div class=\"external-manager-surface\"><iframe class=\"external-manager-frame\" src=\"" + escapeHTML(managerURL) + "\" title=\"Channel Manager\"></iframe></div>";
}
function renderAdminECMSettings() {
  const settings = adminSettings();
  const root = byId("admin-ecm-settings");
  if (!root) return;
  root.innerHTML = adminSaveStatusHTML() + "<div class=\"settings-row ecm-url-row compact-row\"><span><strong>ECM URL</strong><small>Leave blank to hide Channel Manager.</small></span><input type=\"url\" data-admin-ecm-field=\"url\" value=\"" + escapeHTML(settings.ecmURL || "") + "\"></div>";
}
function renderAdminCategorySettings() {
  const settings = adminSettings();
  const root = byId("admin-category-settings");
  const profileAccess = state.app && state.app.source && state.app.source.profileAccess ? state.app.source.profileAccess : {};
  const sourceHelp = "Choose whether virtual groups come from Dispatcharr groups, profiles, channel names, or a combination. Every profile and group pipe segment becomes a nested folder.";
  const nested = settings.mode !== "normal" ? "<div class=\"settings-list-nested\">"
    + "<div class=\"settings-row settings-form-row\"><span class=\"settings-field-copy\"><strong>Delimiter</strong><small>Split source names into nested virtual groups.</small></span><select data-admin-category-field=\"delimiter\"><option value=\"pipe\"" + (settings.delimiter === "pipe" ? " selected" : "") + ">Pipe: Sports | NHL Teams</option><option value=\"dash\"" + (settings.delimiter === "dash" ? " selected" : "") + ">Dash: Sports - NHL Teams</option></select></div>"
    + "<div class=\"settings-row settings-form-row virtual-label-row\"><span class=\"settings-field-copy\"><strong>Virtual groups label</strong><small>Only the suffix after Virtual is editable.</small></span><div class=\"virtual-label-control\"><span>Virtual</span><input data-admin-category-field=\"virtualGroupLabel\" value=\"" + escapeHTML(virtualGroupLabelSuffix(settings.virtualGroupLabel)) + "\" placeholder=\"Groups\"></div></div>"
    + "</div>" : "";
  root.innerHTML = adminSaveStatusHTML()
    + "<div class=\"settings-row settings-form-row\"><span class=\"settings-field-copy\"><strong>Mode</strong><small>Choose how Silo builds the browse hierarchy.</small></span><select data-admin-category-field=\"mode\"><option value=\"normal\"" + (settings.mode === "normal" ? " selected" : "") + ">Normal</option><option value=\"delimiter\"" + (settings.mode === "delimiter" ? " selected" : "") + ">By delimiter</option></select></div>"
    + nested
    + "<div class=\"settings-row settings-form-row settings-source-row\"><span class=\"settings-field-copy\"><strong>Virtual group source</strong><small>" + escapeHTML(sourceHelp) + "</small></span><select data-admin-category-field=\"virtualGroupSource\"><option value=\"group\"" + (virtualGroupSourceMode() === "group" ? " selected" : "") + ">Group pipe</option><option value=\"group_channel\"" + (virtualGroupSourceMode() === "group_channel" ? " selected" : "") + ">Group pipe + channel pipe</option><option value=\"profile_group\"" + (virtualGroupSourceMode() === "profile_group" ? " selected" : "") + ">Profile pipe + group pipe</option><option value=\"channel\"" + (virtualGroupSourceMode() === "channel" ? " selected" : "") + ">Channel pipe</option></select></div>"
    + "<label class=\"settings-row settings-form-row\"><span class=\"settings-field-copy\"><strong>Collapse duplicate virtual groups</strong><small>Skip repeated names when group, profile, or channel path labels overlap.</small></span><input type=\"checkbox\" data-admin-category-field=\"collapseDuplicateVirtualGroups\"" + (settings.collapseDuplicateVirtualGroups !== false ? " checked" : "") + "></label>"
    + renderOrganizationPreview(settings)
    + (virtualGroupSourceMode() === "profile_group" && profileAccess.status !== "available" ? "<div class=\"settings-note settings-warning\">" + escapeHTML(profileAccess.message || "No Channel Profiles are available to the configured Dispatcharr account. Assign profiles in Dispatcharr, then refresh Live TV.") + "</div>" : "")
    + (settings.mode === "normal" ? "<div class=\"settings-note\">Channel groups are shown as provided, without remapping or resorting.</div>" : "");
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
function adminSourceGroups() {
  const groups = {};
  effectiveChannels(false).forEach(function(channel) {
    const sourcePath = sourceCategoryOriginalLabel(channel);
    if (!sourcePath) return;
    groups[sourcePath] = groups[sourcePath] || { sourcePath: sourcePath, count: 0 };
    groups[sourcePath].count++;
  });
  return Object.keys(groups).sort().map(function(sourcePath) { return groups[sourcePath]; });
}
function adminSourceGroupCount(sourcePath) {
  const path = configuredCategoryPath(sourcePath);
  const group = adminSourceGroups().find(function(item) {
    return item.sourcePath === sourcePath || configuredCategoryPath(item.sourcePath) === path;
  });
  return group ? group.count : 0;
}
function addAdminCategoryAlias() {
  const source = byId("admin-alias-source");
  const alias = byId("admin-alias-path");
  const sourcePath = source ? String(source.value || "").trim() : "";
  const aliasPath = alias ? String(alias.value || "").trim() : "";
  if (!sourcePath || !aliasPath) return;
  const settings = state.adminCategorySettings || defaultAdminCategorySettings();
  settings.categoryAliases = normalizeCategoryAliases(items(settings.categoryAliases).concat([{ sourcePath: sourcePath, aliasPath: aliasPath }]));
  state.adminCategorySettings = settings;
  markAdminSettingsDraft();
  renderAdminPage();
}
function removeAdminCategoryAlias(index) {
  const settings = state.adminCategorySettings || defaultAdminCategorySettings();
  settings.categoryAliases = items(settings.categoryAliases).filter(function(_, rowIndex) { return rowIndex !== index; });
  state.adminCategorySettings = settings;
  normalizeAdminCategorySettings();
  markAdminSettingsDraft();
  renderAdminPage();
}
function updateAdminCategoryAlias(index, field, value) {
  const settings = state.adminCategorySettings || defaultAdminCategorySettings();
  const aliases = items(settings.categoryAliases).slice();
  aliases[index] = Object.assign({}, aliases[index] || {});
  aliases[index][field] = value;
  settings.categoryAliases = aliases;
  state.adminCategorySettings = settings;
  markAdminSettingsDraft();
}
function renderAdminCategoryAliasSettings() {
  const root = byId("admin-category-alias-settings");
  if (!root) return;
  const settings = adminSettings();
  const sourceGroups = adminSourceGroups();
  const aliases = categoryAliases();
  const sourceOptions = sourceGroups.map(function(group) {
    return "<option value=\"" + escapeHTML(group.sourcePath) + "\">" + escapeHTML(group.sourcePath) + " (" + escapeHTML(String(group.count)) + ")</option>";
  }).join("");
  const addRow = "<div class=\"alias-builder\"><label><span>Source group</span><select id=\"admin-alias-source\"" + (!sourceGroups.length ? " disabled" : "") + ">" + sourceOptions + "</select></label><label><span>Also show as</span><input id=\"admin-alias-path\" placeholder=\"Sports | Arabic\"" + (!sourceGroups.length ? " disabled" : "") + "></label><button data-admin-alias-action=\"add\"" + (!sourceGroups.length ? " disabled" : "") + ">Add</button></div>";
  const rows = aliases.map(function(alias, index) {
    const count = adminSourceGroupCount(alias.sourcePath);
    return "<div class=\"alias-table-row" + (!count ? " stale" : "") + "\"><div class=\"alias-table-source\"><strong title=\"" + escapeHTML(alias.sourcePath) + "\">" + escapeHTML(alias.sourcePath) + "</strong>" + (!count ? "<small>Source not found</small>" : "") + "</div><span class=\"alias-table-arrow\">&rarr;</span><label><input data-admin-alias-index=\"" + index + "\" data-admin-alias-field=\"aliasPath\" value=\"" + escapeHTML(alias.aliasPath) + "\" title=\"" + escapeHTML(alias.aliasPath) + "\" aria-label=\"Alternative group name\"></label><span class=\"alias-table-count\">" + escapeHTML(String(count)) + "</span><span class=\"alias-table-actions\"><button data-admin-alias-action=\"remove\" data-admin-alias-index=\"" + index + "\">Remove</button></span></div>";
  }).join("");
  root.innerHTML = (settings.mode !== "delimiter" ? "<div class=\"settings-note settings-warning\">Alternative group names apply when category mode is By delimiter.</div>" : "")
    + addRow
    + "<div class=\"alias-table\"><div class=\"alias-table-head\"><span>Source group</span><span></span><span>Also show as</span><span>Channels</span><span>Actions</span></div>" + (rows || "<div class=\"empty\">No alternative group names yet.</div>") + "</div>";
}
function renderAdminEventKeywordSettings() {
  const root = byId("admin-event-keyword-settings");
  if (!root) return;
  const rows = normalizeEventKeywordRows(adminSettings().eventKeywords);
  root.innerHTML = rows.map(function(row, index) {
    const label = row.categoryName || eventCategoryName(row.categoryId);
    const series = row.eventSeries ? "<div class=\"event-keyword-options\"><label><span>Exclude</span><textarea data-admin-event-keyword-index=\"" + index + "\" data-admin-event-keyword-field=\"excludeKeywords\" aria-label=\"" + escapeHTML(label + " exclusion keywords") + "\">" + escapeHTML(row.excludeKeywords.join("\n")) + "</textarea></label><label class=\"event-window-field\"><span>Coverage window</span><span><input type=\"number\" min=\"15\" max=\"360\" step=\"15\" data-admin-event-keyword-index=\"" + index + "\" data-admin-event-keyword-field=\"groupWindowMinutes\" value=\"" + escapeHTML(String(row.groupWindowMinutes || 60)) + "\"><small>minutes</small></span></label></div>" : "";
    return "<div class=\"settings-row event-keyword-row" + (row.eventSeries ? " event-series-rule" : "") + "\"><span class=\"event-keyword-label\">" + escapeHTML(label) + (row.eventSeries ? "<small>Event series</small>" : "") + "</span><div class=\"event-keyword-fields\"><label><span>Match</span><textarea data-admin-event-keyword-index=\"" + index + "\" data-admin-event-keyword-field=\"keywords\" aria-label=\"" + escapeHTML(label + " event keywords") + "\">" + escapeHTML(row.keywords.join("\n")) + "</textarea></label>" + series + "</div></div>";
  }).join("");
}
function updateAdminEventKeywords(index, field, value) {
  const settings = state.adminCategorySettings || defaultAdminCategorySettings();
  const rows = normalizeEventKeywordRows(settings.eventKeywords);
  if (!rows[index]) return;
  const update = {};
  if (field === "groupWindowMinutes") update.groupWindowMinutes = Math.max(15, Math.min(360, Number(value) || 60));
  else update[field === "excludeKeywords" ? "excludeKeywords" : "keywords"] = normalizeKeywordList(value);
  rows[index] = Object.assign({}, rows[index], update);
  settings.eventKeywords = rows;
  state.adminCategorySettings = settings;
  state.events = null;
  markAdminSettingsDraft();
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
      : "<div class=\"custom-group-empty\"><strong>Create a group</strong><span>Build a personal channel lineup by adding channels from the current Dispatcharr catalog.</span></div>")
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
function audioTrackList() {
  const video = byId("player");
  if (!video || !video.audioTracks || typeof video.audioTracks.length !== "number") return [];
  const tracks = [];
  for (let index = 0; index < video.audioTracks.length; index++) tracks.push(video.audioTracks[index]);
  return tracks;
}
function audioTrackName(track, index) {
  return track && (track.label || track.language || track.kind || track.id) ? (track.label || track.language || track.kind || track.id) : "Audio " + (index + 1);
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
function updateSubtitlesButton() {
  const button = byId("player-subtitles-button");
  if (!button) return;
  const tracks = textTrackList();
  const activeIndex = tracks.findIndex(function(track) { return track.mode === "showing"; });
  if (activeIndex >= 0) state.selectedTextTrack = activeIndex;
  button.classList.toggle("active", activeIndex >= 0);
  button.setAttribute("aria-pressed", activeIndex >= 0 ? "true" : "false");
  button.setAttribute("aria-label", activeIndex >= 0 ? "Subtitles: " + textTrackName(tracks[activeIndex], activeIndex) : "Subtitles");
}
function toggleSubtitles() {
  const tracks = textTrackList();
  closePlayerPopovers();
  if (!tracks.length) {
    showPlayerToast("No subtitles are available for this stream.");
    updateSubtitlesButton();
    return;
  }
  const activeIndex = tracks.findIndex(function(track) { return track.mode === "showing"; });
  const nextIndex = activeIndex >= 0 && activeIndex < tracks.length - 1 ? activeIndex + 1 : (activeIndex >= 0 ? -1 : Math.max(0, state.selectedTextTrack));
  tracks.forEach(function(track, index) {
    track.mode = index === nextIndex ? "showing" : "disabled";
  });
  state.selectedTextTrack = nextIndex;
  updateSubtitlesButton();
  showPlayerToast(nextIndex >= 0 ? "Subtitles: " + textTrackName(tracks[nextIndex], nextIndex) : "Subtitles off.");
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
function selectAudioTrack(index) {
  const tracks = audioTrackList();
  if (!tracks.length) {
    state.selectedAudioTrack = 0;
    state.audioMenuOpen = false;
    updateAudioMenu();
    return;
  }
  tracks.forEach(function(track, trackIndex) { track.enabled = trackIndex === index; });
  state.selectedAudioTrack = index;
  state.audioMenuOpen = false;
  updateAudioMenu();
}
function volumeLabel() {
  if (state.muted || state.volume <= 0) return "0%";
  return Math.round(state.volume * 100) + "%";
}
function applyVolumeToVideo() {
  const video = byId("player");
  state.volume = Math.max(0, Math.min(1, Number(state.volume) || 0));
  state.muted = state.volume <= 0;
  if (video) {
    video.volume = state.volume;
    video.muted = state.muted;
  }
  updateVolumeMenu();
}
function updateVolumeMenu() {
  const button = byId("player-volume-button");
  const popover = byId("player-volume-popover");
  const slider = byId("player-volume-slider");
  const value = byId("player-volume-value");
  if (!button || !popover) return;
  button.innerHTML = icon(state.muted || state.volume <= 0 ? "speaker-off" : "speaker");
  button.setAttribute("aria-expanded", state.volumeMenuOpen ? "true" : "false");
  popover.classList.toggle("open", state.volumeMenuOpen);
  if (slider) slider.value = String(Math.round(state.volume * 100));
  if (value) value.textContent = volumeLabel();
  updatePlayerChrome();
}
function closePlayerPopovers(except) {
  if (except !== "audio") state.audioMenuOpen = false;
  if (except !== "volume") state.volumeMenuOpen = false;
  if (except !== "more") state.moreMenuOpen = false;
  updateAudioMenu();
  updateVolumeMenu();
  renderPlayerMoreMenu();
}
function showPlayerToast(message) {
  const toast = byId("player-toast");
  if (!toast) return;
  toast.textContent = message;
  toast.classList.add("show");
  clearTimeout(state.toastTimer);
  state.toastTimer = setTimeout(function() { toast.classList.remove("show"); }, 2400);
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
  if (!video || !button) return;
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
function fullscreenElement() {
  return document.fullscreenElement || document.webkitFullscreenElement || null;
}
function updateFullscreenButton() {
  const button = byId("player-fullscreen-button");
  if (!button) return;
  const active = !!fullscreenElement();
  button.innerHTML = icon(active ? "fullscreen-exit" : "fullscreen");
  button.classList.toggle("active", active);
  button.setAttribute("aria-pressed", active ? "true" : "false");
  button.setAttribute("aria-label", active ? "Exit fullscreen" : "Fullscreen");
  renderPlayerMoreMenu();
}
async function toggleFullscreen() {
  const shell = document.querySelector(".playback-shell");
  closePlayerPopovers();
  try {
    if (fullscreenElement()) {
      if (document.exitFullscreen) await document.exitFullscreen();
      else if (document.webkitExitFullscreen) document.webkitExitFullscreen();
    } else if (shell) {
      if (shell.requestFullscreen) await shell.requestFullscreen();
      else if (shell.webkitRequestFullscreen) shell.webkitRequestFullscreen();
      else showPlayerToast("Fullscreen is not available in this browser.");
    }
  } catch (error) {
    showPlayerToast("Fullscreen could not be changed.");
  }
  updateFullscreenButton();
}
function setVideoSource(url, options) {
  const video = byId("player");
  if (!video) return;
  const rewindable = !!(options && options.rewindable);
  video.controls = rewindable;
  applyVolumeToVideo();
  state.selectedAudioTrack = 0;
  state.selectedTextTrack = -1;
  state.audioMenuOpen = false;
  state.volumeMenuOpen = false;
  state.moreMenuOpen = false;
  updateAudioMenu();
  updateSubtitlesButton();
  updateVolumeMenu();
  renderPlayerMoreMenu();
  if (video.audioTracks && video.audioTracks.addEventListener) {
    video.audioTracks.addEventListener("addtrack", updateAudioMenu);
    video.audioTracks.addEventListener("removetrack", updateAudioMenu);
    video.audioTracks.addEventListener("change", updateAudioMenu);
  }
  video.addEventListener("loadedmetadata", updateAudioMenu, { once: true });
  video.addEventListener("loadedmetadata", updateSubtitlesButton, { once: true });
  video.addEventListener("waiting", function() { state.playerWaiting = true; updateCenterPlayButton(); });
  video.addEventListener("stalled", function() { state.playerWaiting = true; updateCenterPlayButton(); });
  video.addEventListener("canplay", function() { state.playerWaiting = false; updateCenterPlayButton(); });
  video.addEventListener("playing", function() { state.playerWaiting = false; updateCenterPlayButton(); });
  video.addEventListener("pause", updateCenterPlayButton);
  video.addEventListener("play", updateCenterPlayButton);
  video.addEventListener("error", function() { state.playerWaiting = false; updateCenterPlayButton(); });
  if (video.textTracks && video.textTracks.addEventListener) {
    video.textTracks.addEventListener("addtrack", updateSubtitlesButton);
    video.textTracks.addEventListener("removetrack", updateSubtitlesButton);
    video.textTracks.addEventListener("change", updateSubtitlesButton);
  }
  if (state.hls) { state.hls.destroy(); state.hls = null; }
  if (state.tsPlayer) { state.tsPlayer.destroy(); state.tsPlayer = null; }
  const attachment = attachVideoSource(video, url, { rewindable: rewindable, format: options && options.format, onFatal: options && options.onFatal });
  state.hls = attachment.hls;
  state.tsPlayer = attachment.tsPlayer;
  setTimeout(updateAudioMenu, 500);
  setTimeout(updateAudioMenu, 1800);
  setTimeout(updateSubtitlesButton, 500);
  setTimeout(updateSubtitlesButton, 1800);
  updateCenterPlayButton();
  applyAspectMode();
  video.play().then(updateCenterPlayButton).catch(function() { updateCenterPlayButton(); });
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
  startWatch(channel);
  if (liveRewindEnabled() && channel.streamFormat !== "hls") {
    showPlayerToast("Preparing Live Rewind...");
    try {
      const manifestURL = await prepareTimeShift(channel);
      setVideoSource(manifestURL, { rewindable: true, format: "hls", onFatal: function() { fallbackFromTimeShift(channel, "Live Rewind stopped. Continuing live."); } });
      state.timeShiftTimelineTimer = setInterval(updateTimeShiftUI, 1000);
      const video = byId("player");
      if (video) {
        video.addEventListener("timeupdate", updateTimeShiftUI);
        video.addEventListener("progress", updateTimeShiftUI);
      }
      updateTimeShiftUI();
      showPlayerToast("Live Rewind ready.");
    } catch (error) {
      if (timeShiftAttempt === state.timeShiftAttempt && !(error && error.superseded)) fallbackFromTimeShift(channel, "Live Rewind unavailable. Playing live.");
    }
  } else {
    setVideoSource(browserStreamURL(channel), { rewindable: isRewindableChannel(channel), format: channel.streamFormat });
  }
  if (timeShiftAttempt !== state.timeShiftAttempt || !state.currentChannel || state.currentChannel.id !== channel.id) return;
  const guide = await getJSON("/dispatcharr/api/guide?channel_id=" + encodeURIComponent(channel.id)).catch(function() { return { programs: [] }; });
  if (!state.currentChannel || state.currentChannel.id !== channel.id) return;
  const nowGuide = byId("now-guide");
  if (nowGuide) nowGuide.innerHTML = items(guide.programs).slice(0, 6).map(function(program) { return "<div class=\"program\"><time>" + escapeHTML(timeLabel(program.startUnix)) + "</time><strong>" + escapeHTML(program.title || "Untitled") + "</strong></div>"; }).join("") || "<div class=\"empty\">No guide entries.</div>";
}
function startWatch(channel) {
  if (state.currentSession) postJSON("/dispatcharr/api/watch/stop", { sessionId: state.currentSession.id, reason: "switch_channel" }).catch(function() {});
  recordWatchPreference(channel);
  postJSON("/dispatcharr/api/watch/start", { itemKind: "channel", itemId: channel.id, itemName: channel.name }).then(function(payload) {
    state.currentSession = payload.session;
    if (state.heartbeat) clearInterval(state.heartbeat);
    state.heartbeat = setInterval(function() {
      if (state.currentSession) postJSON("/dispatcharr/api/watch/heartbeat", { sessionId: state.currentSession.id }).catch(function() {});
    }, 30000);
    renderRail();
  }).catch(function() {});
}
function handlePlayerAction(action, button) {
  const video = byId("player");
  wakePlayerChrome();
  if (action === "back") {
    returnFromPlayer();
    return;
  }
  if (action === "guide") {
    state.playerGuideOpen = !state.playerGuideOpen;
    state.playerSportsOpen = false;
    stopPlayerSportsRefresh();
    closePlayerPopovers();
    renderPlayerGuidePanel();
    renderPlayerSportsDrawer();
    return;
  }
  if (action === "guide-close") {
    state.playerGuideOpen = false;
    renderPlayerGuidePanel();
    return;
  }
  if (action === "sports") {
    togglePlayerSports();
    return;
  }
  if (action === "sports-close") {
    togglePlayerSports(false);
    return;
  }
  if (action === "cast") {
    closePlayerPopovers();
    openCastPicker();
    return;
  }
  if (action === "pip") {
    togglePictureInPicture();
    return;
  }
  if (action === "play-toggle") {
    togglePlayPause();
    return;
  }
  if (action === "rewind-30") {
    timeShiftSeek(-30);
    return;
  }
  if (action === "forward-30") {
    timeShiftSeek(30);
    return;
  }
  if (action === "go-live") {
    timeShiftGoLive();
    return;
  }
  if (action === "fullscreen") {
    toggleFullscreen();
    return;
  }
  if (action === "subtitles") {
    toggleSubtitles();
    return;
  }
  if (action === "volume-menu") {
    state.volumeMenuOpen = !state.volumeMenuOpen;
    closePlayerPopovers("volume");
    updateVolumeMenu();
    return;
  }
  if (action === "audio-menu") {
    state.audioMenuOpen = !state.audioMenuOpen;
    closePlayerPopovers("audio");
    updateAudioMenu();
    return;
  }
  if (action === "language-menu") {
    toggleLanguageMenu();
    return;
  }
  if (action === "audio-track") {
    selectAudioTrack(Number(button && button.getAttribute("data-audio-index")) || 0);
    return;
  }
  if (action === "more") {
    state.moreMenuOpen = !state.moreMenuOpen;
    closePlayerPopovers("more");
    renderPlayerMoreMenu();
    return;
  }
  if (action === "aspect") {
    state.aspectMode = state.aspectMode === "fit" ? "fill" : "fit";
    applyAspectMode();
    renderPlayerMoreMenu();
    return;
  }
  if (action === "search-channel") {
    state.moreMenuOpen = false;
    renderPlayerMoreMenu();
    setView("search");
    return;
  }
  if (action === "add-multiview" && state.currentChannel) {
    state.moreMenuOpen = false;
    addChannelToMultiview(state.currentChannel);
    return;
  }
  if (action === "copy-stream") {
    const url = currentStreamURL();
    if (url && navigator.clipboard) navigator.clipboard.writeText(new URL(url, window.location.href).href).then(function() { showPlayerToast("Stream URL copied."); }).catch(function() { showPlayerToast("Could not copy stream URL."); });
    else showPlayerToast("No stream URL available.");
    state.moreMenuOpen = false;
    renderPlayerMoreMenu();
    return;
  }
  if (action === "open-stream") {
    const url = currentStreamURL();
    if (url) window.open(url, "_blank", "noopener");
    state.moreMenuOpen = false;
    renderPlayerMoreMenu();
    return;
  }
  if (action === "favorite" && state.currentChannel) {
    const id = state.currentChannel.id;
    const isFavorite = setChannelFavorite(id, !favoriteMap()[id]);
    if (button) {
      button.innerHTML = icon(isFavorite ? "heart-solid" : "heart");
      button.classList.toggle("active", isFavorite);
      button.setAttribute("aria-pressed", isFavorite ? "true" : "false");
      button.setAttribute("aria-label", isFavorite ? "Remove channel from favorites" : "Favorite channel");
    }
    renderRail();
  }
}
function returnFromPlayer() {
  const context = state.playerReturnContext;
  if (!context) {
    setView("live");
    return;
  }
  state.playerReturnContext = null;
  state.category = context.category || "";
  state.query = context.query || "";
  state.folderQuery = context.folderQuery || "";
  setView(context.view || "live", { preserveBrowseState: true });
  requestAnimationFrame(function() {
    window.scrollTo(0, context.scrollY || 0);
    const main = document.querySelector(".main");
    if (main) main.scrollTop = context.mainScrollTop || 0;
    const guideScroll = byId("guide-scroll");
    if (guideScroll) {
      guideScroll.scrollLeft = context.guideScrollLeft || 0;
      guideScroll.scrollTop = context.guideScrollTop || 0;
      renderGuideWindow(true);
    }
  });
}
document.addEventListener("click", function(event) {
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
  const adminProfileRefresh = event.target.closest("[data-admin-profile-refresh]");
  if (adminProfileRefresh) {
    event.preventDefault();
    refreshAdminProfiles();
    return;
  }
  const adminStatusRefresh = event.target.closest("[data-admin-status-refresh]");
  if (adminStatusRefresh) {
    event.preventDefault();
    refreshAdminStatus();
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
  const favoriteMove = event.target.closest("[data-favorite-move]");
  if (favoriteMove) {
    event.preventDefault();
    moveFavorite(favoriteMove.getAttribute("data-channel-id"), favoriteMove.getAttribute("data-favorite-move"));
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
  const id = event.target.getAttribute("data-hide");
  if (!id) return;
  if (event.target.checked) state.app.preferences.hiddenCategories[id] = true;
  else delete state.app.preferences.hiddenCategories[id];
  savePrefs();
  render();
});
document.addEventListener("input", function(event) {
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
window.addEventListener("beforeunload", function() {
  if (state.currentSession) navigator.sendBeacon(route("/dispatcharr/api/watch/stop"), JSON.stringify({ sessionId: state.currentSession.id, reason: "page_unload" }));
  items(state.multiviewTiles).forEach(function(tile) {
    if (tile.session) navigator.sendBeacon(route("/dispatcharr/api/watch/stop"), JSON.stringify({ sessionId: tile.session.id, reason: "page_unload" }));
  });
});
startGuideAutoRefresh();
loadApp().catch(function() {
  byId("view").innerHTML = emptyStateHTML("Unable to load Live TV.", "Check your Dispatcharr connection in Live TV Admin, then refresh this page.");
});
