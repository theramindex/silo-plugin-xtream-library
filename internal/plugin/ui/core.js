const path = window.location.pathname;
const base = path.endsWith("/xtream/player") ? path.slice(0, -"/xtream/player".length) : (path.endsWith("/xtream/admin") ? path.slice(0, -"/xtream/admin".length) : (path.endsWith("/xtream") ? path.slice(0, -"/xtream".length) : ""));
const isAdminRoute = path.endsWith("/xtream/admin");
const adminSettingsKey = "adminCategorySettings";
const pluginInstallationID = (base.match(/\/api\/v1\/plugins\/(\d+)/) || [])[1] || "";
const localCacheSuffix = pluginInstallationID || "default";
const appCacheKey = "silo.ramindex.xtream.appSnapshot.v1." + localCacheSuffix;
const assetVersionMeta = document.querySelector('meta[name="xtream-asset-version"]');
const assetVersion = assetVersionMeta ? String(assetVersionMeta.content || "") : "";
const assetPrefix = path.endsWith("/xtream") ? "xtream/assets" : "assets";
const guidePingChannelLimit = 8;
const state = { app: null, appLoadedFromCache: false, programsByChannel: {}, sortedPrograms: [], view: isAdminRoute ? "admin" : "home", category: "", categoryBrowseView: "grid", query: "", folderQuery: "", searchQuery: "", searchType: "all", searchReturnView: "home", recentSearches: [], onLaterTime: "all", onLaterType: "all", hls: null, tsPlayer: null, currentChannel: null, currentSession: null, heartbeat: null, muted: false, volume: 1, volumeMenuOpen: false, audioMenuOpen: false, moreMenuOpen: false, playerGuideOpen: false, playerGuideQuery: "", playerSportsOpen: false, playerSportsTimer: null, playerReturnContext: null, selectedAudioTrack: 0, selectedTextTrack: -1, aspectMode: "fill", playerChromeIdle: false, playerChromeTimer: null, playerWaiting: false, multiviewTiles: [], multiviewActiveTileID: "", multiviewQuery: "", multiviewHeartbeat: null, recordings: null, recordingsLoading: false, recordingCapability: null, sports: null, sportsLoading: false, sportsLeague: "", sportsExpandedEvents: {}, events: null, eventsLoading: false, eventsTab: "upcoming", eventCategory: "", expandedEvents: {}, guideChannels: [], guideRendered: 0, guideLoading: false, guideWindowStart: -1, guideWindowEnd: -1, guideRenderFrame: 0, guideWarmPings: {}, guideAutoTimer: null, guideLastSlotStart: 0, guideLastAutoFetchAt: 0, guideAutoFetching: false, programDetails: null, refreshing: false, virtualCategoryView: "guide", selectedCustomGroup: "", customGroupQuery: "", customGroupChannelID: "", profileSettingsQuery: "", categorySettingsQuery: "", categorySettingsOpen: { live: true, north_america: false, international: false }, profileSelectionIDMap: null, profileChannelFilterMap: null, adminTab: "settings", adminCategorySettings: null, savedAdminCategorySettings: null, profileSaveStatus: "idle", profileSaveMessage: "", adminSaveStatus: "idle", adminSaveMessage: "", timeShiftSession: null, timeShiftHeartbeat: null, timeShiftTimelineTimer: null, timeShiftAttempt: 0, timeShiftAdminStatus: null, timeShiftAdminLoading: false };
state.folderGroupCategoryID = "";
state.folderGroupPickerOpen = false;
state.searchAiringChannel = "";
state.myTVQuery = "";
state.playerSportsMode = false;
state.playerSportsMoreOpen = false;
state.sportsPollTimer = null;
state.sportsPollAttempts = 0;
state.sportsFailedMedia = {};
state.sportsTab = "live";
state.sportsSelectedEventID = "";
state.sportsLeagueTeams = {};
state.sportsLeagueTeamsLoading = {};
state.sportsLibraries = null;
state.sportsLibrariesLoading = false;
state.sportsLibrariesPromise = null;
state.sportsLibrariesError = "";
state.sportsReplayItems = [];
state.sportsReplayMatches = {};
state.sportsReplaysLoading = false;
state.sportsReplaysError = "";
state.sportsReplayKey = "";
state.savedLineupEditor = null;
state.activeSavedLineupID = "";
state.savedLineupGroupCategoryID = "";
state.adminConnection = null;
state.savedAdminConnection = null;
state.adminConnectionEditorOpen = false;
state.adminConnectionEditorStep = "connection";
state.adminConnectionStatus = "idle";
state.adminConnectionMessage = "";
state.adminConnectionLoading = false;
state.adminConnectionLoadError = "";
state.adminStatusRefreshing = false;
state.adminProfileRefreshing = false;
state.adminSourceGroupsLoaded = false;
state.adminSourceGroupsLoading = false;
state.adminSourceGroupsError = "";
state.onLaterShelfLimits = {};
state.myTVTeamCatalogLoading = null;
state.sportsReplayStandaloneEvents = [];
state.aspectMode = "fit";

state.guideCategoryPickerOpen = false;
state.guideCategoryQuery = "";
state.categoryBrowseSort = "provider";
state.categoryLogoStyle = "color";
state.categoryCleanNames = false;
state.categoryMenuOpen = false;
state.adminTab = "sources";
state.adminSources = [];
state.adminSourcesLoading = false;
state.adminSourceEditor = null;
state.adminSourceEditorStep = "general";
state.adminSourceMessage = "";
state.adminSourceEPGResult = null;
state.adminSourceEPGError = "";
state.adminSourceRequestID = 0;

function applySiloTheme() {
  const params = new URLSearchParams(window.location.search);
  const theme = String(params.get("theme") || document.documentElement.dataset.siloTheme || "").trim().toLowerCase().replace(/[^a-z0-9_-]/g, "");
  if (theme) document.documentElement.dataset.siloTheme = theme;
}

applySiloTheme();

function route(url) { return base + String(url || "").replace(/^\/dispatcharr(?=\/|$)/, "/xtream"); }
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
  if (format === "hls") return loadPlayerLibrary("xc-runtime-a.js", "Hls");
  if (format === "mpegts") return loadPlayerLibrary("xc-runtime-b.js", "mpegts");
  return Promise.all([
    loadPlayerLibrary("xc-runtime-a.js", "Hls"),
    loadPlayerLibrary("xc-runtime-b.js", "mpegts")
  ]);
}
// CSP forbids inline handlers, so rendered images declare their fallback with
// data-img-error / data-img-load and one capture-phase listener applies it
// (load/error do not bubble, but they do pass through the capture phase).
const imageErrorActions = {
  "logo-fallback": function(image) {
    image.hidden = true;
    if (image.nextElementSibling) image.nextElementSibling.hidden = false;
  },
  "remove": function(image) { image.remove(); },
  "event-poster": function(image) {
    const poster = image.closest(".event-poster");
    if (poster) poster.remove();
    else image.remove();
  },
  "sports-media": function(image) { markSportsMediaFailed(image); },
  "sports-detail-bg": function(image) { markSportsDetailBackgroundFailed(image); },
  "sports-bg": function(image) { markSportsBackgroundFailed(image); }
};
const imageLoadActions = {
  "generated-art": function(image) {
    if (image.classList.contains("sports-generated-bg") && image.parentElement) image.parentElement.classList.add("has-generated-art");
  }
};
function handleDeclarativeImageEvent(event) {
  const image = event && event.target;
  if (!image || image.tagName !== "IMG" || !image.dataset) return;
  const actions = event.type === "error" ? imageErrorActions : imageLoadActions;
  const name = event.type === "error" ? image.dataset.imgError : image.dataset.imgLoad;
  const action = name && Object.prototype.hasOwnProperty.call(actions, name) ? actions[name] : null;
  if (!action) return;
  try { action(image); } catch (error) {
    try { console.warn("Xtream image " + event.type + " handler failed", error); } catch (_) {}
  }
}
document.addEventListener("error", handleDeclarativeImageEvent, true);
document.addEventListener("load", handleDeclarativeImageEvent, true);
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
  return (value == null ? "" : String(value)).replace(/[&<>"']/g, function(ch) {
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
    "chevron-right": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='m9 6 6 6-6 6'/></svg>",
    "chevron-down": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='m6 9 6 6 6-6'/></svg>",
    "check": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='m5 12.5 4.25 4.25L19 7'/></svg>",
    "plus": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' d='M12 5v14M5 12h14'/></svg>",
    "ellipsis": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M6.75 12a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0Zm6 0a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0Zm6 0a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0Z'/></svg>",
    "play": "<svg viewBox='0 0 24 24' fill='currentColor' aria-hidden='true'><path d='M8 5.6v12.8c0 .55.6.9 1.08.62l10.1-6.4a.73.73 0 0 0 0-1.24L9.08 4.98A.72.72 0 0 0 8 5.6Z'/></svg>",
    "record": "<svg viewBox='0 0 24 24' fill='currentColor' aria-hidden='true'><path d='M12 20.25a8.25 8.25 0 1 0 0-16.5 8.25 8.25 0 0 0 0 16.5Zm0-4a4.25 4.25 0 1 1 0-8.5 4.25 4.25 0 0 1 0 8.5Z'/></svg>",
    "pause": "<svg viewBox='0 0 24 24' fill='currentColor' aria-hidden='true'><path d='M7.25 5.25h3.25v13.5H7.25zM13.5 5.25h3.25v13.5H13.5z'/></svg>",
    "loader": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' d='M12 3a9 9 0 1 1-8.3 5.5'/></svg>",
    "speaker": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M19.1 8.9a7 7 0 0 1 0 6.2M16.2 10.9a3 3 0 0 1 0 2.2M4.5 14.25h3l4.25 3.25V6.5L7.5 9.75h-3v4.5Z'/></svg>",
    "speaker-off": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='m4.5 4.5 15 15M5 14.25h2.5l4.25 3.25v-5.75M11.75 8.7V6.5L8.8 8.75M16 10.8a3 3 0 0 1 .2 2.2'/></svg>",
    "airplay": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M6.75 17.25h-1.5A2.25 2.25 0 0 1 3 15V6.75A2.25 2.25 0 0 1 5.25 4.5h13.5A2.25 2.25 0 0 1 21 6.75V15a2.25 2.25 0 0 1-2.25 2.25h-1.5M8.25 21h7.5L12 16.5 8.25 21Z'/></svg>",
    "guide": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M4.5 6.75h15M4.5 12h15M4.5 17.25h15M8.25 4.5v15M15.75 4.5v15'/></svg>",
    "kids": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><circle cx='9' cy='8.25' r='3'/><circle cx='16.5' cy='9.5' r='2.25'/><path stroke-linecap='round' stroke-linejoin='round' d='M3.75 19c.45-3.5 2.2-5.25 5.25-5.25S13.8 15.5 14.25 19M14.25 14.25c3.5-.65 5.5.95 6 4.75'/></svg>",
    "news": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M5.25 5.25h11.5v13.5H6.5a2.25 2.25 0 0 1-2.25-2.25V6.25a1 1 0 0 1 1-1ZM16.75 8h3v8.5a2.25 2.25 0 0 1-2.25 2.25h-.75'/><path stroke-linecap='round' d='M7.75 9h6.5M7.75 12h6.5M7.75 15h4'/></svg>",
    "tv": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='m8.5 3.75 3.5 3 3.5-3M5.5 6.75h13A2.25 2.25 0 0 1 20.75 9v8.25a2.25 2.25 0 0 1-2.25 2.25h-13a2.25 2.25 0 0 1-2.25-2.25V9A2.25 2.25 0 0 1 5.5 6.75Z'/><path stroke-linecap='round' stroke-linejoin='round' d='M8.5 10.25v5.75l5-2.875-5-2.875ZM17.5 11.25h.01M17.5 15h.01'/></svg>",
    "collection": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M4.5 5.25h6.25v5.5H4.5zM13.25 5.25h6.25v5.5h-6.25zM4.5 13.25h6.25v5.5H4.5zM13.25 13.25h6.25v5.5h-6.25z'/></svg>",
    "clock": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z'/><path stroke-linecap='round' stroke-linejoin='round' d='M12 7.5V12l3 2.25'/></svg>",
    "multiview": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M4.5 5.75h6.25v5.75H4.5zM13.25 5.75h6.25v5.75h-6.25zM4.5 14h6.25v4.25H4.5zM13.25 14h6.25v4.25h-6.25z'/></svg>",
    "trophy": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M8 4.5h8v4.25a4 4 0 0 1-8 0V4.5ZM9.5 14.5h5M12 12.75V18M8.5 20h7'/><path stroke-linecap='round' stroke-linejoin='round' d='M8 6H5.25v1.5A3.5 3.5 0 0 0 8.5 11M16 6h2.75v1.5A3.5 3.5 0 0 1 15.5 11'/></svg>",
    "settings": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M12 8.25a3.75 3.75 0 1 1 0 7.5 3.75 3.75 0 0 1 0-7.5Z'/><path stroke-linecap='round' stroke-linejoin='round' d='M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.6 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 8.92 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9c.23.64.84 1 1.51 1H21a2 2 0 0 1 0 4h-.09A1.65 1.65 0 0 0 19.4 15Z'/></svg>",
    "fullscreen": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M8.25 4.5H4.5v3.75M15.75 4.5h3.75v3.75M19.5 15.75v3.75h-3.75M4.5 15.75v3.75h3.75M9 9 4.5 4.5M15 9l4.5-4.5M15 15l4.5 4.5M9 15l-4.5 4.5'/></svg>",
    "fullscreen-exit": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M4.5 9h4.25V4.75M15.25 4.75V9h4.25M19.5 15h-4.25v4.25M8.75 19.25V15H4.5M8.75 9 4.5 4.75M15.25 9l4.25-4.25M15.25 15l4.25 4.25M8.75 15 4.5 19.25'/></svg>",
    "heart": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M21 8.25c0 6.25-9 11.25-9 11.25s-9-5-9-11.25A4.75 4.75 0 0 1 11.25 5 4.75 4.75 0 0 1 21 8.25Z'/></svg>",
    "heart-solid": "<svg viewBox='0 0 24 24' fill='currentColor' aria-hidden='true'><path d='M12 21s-9-5.1-9-12.25A5.45 5.45 0 0 1 12 4.7a5.45 5.45 0 0 1 9 4.05C21 15.9 12 21 12 21Z'/></svg>",
    "eye": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='M2.75 12s3.25-6 9.25-6 9.25 6 9.25 6-3.25 6-9.25 6-9.25-6-9.25-6Z'/><circle cx='12' cy='12' r='2.75'/></svg>",
    "eye-off": "<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' aria-hidden='true'><path stroke-linecap='round' stroke-linejoin='round' d='m3 3 18 18M10.55 6.13A9.7 9.7 0 0 1 12 6c6 0 9.25 6 9.25 6a14.5 14.5 0 0 1-2.14 2.82M6.1 6.1C3.9 7.73 2.75 12 2.75 12s3.25 6 9.25 6a9.8 9.8 0 0 0 3.2-.53M9.8 9.8a3.1 3.1 0 0 0 4.4 4.4'/></svg>",
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
  return { favorites: {}, autoFavorites: {}, favoriteOrder: [], hiddenCategories: {}, sportsFavoriteTeams: {}, keywordPasses: [], groupCategoriesByPipe: false, recentSearches: [], recentChannels: [], continueWatching: {}, playback: { backendProxySupported: false, streamMode: "redirect", outputFormat: "ts" }, categoryBrowse: { sort: "provider", layout: "grid", logoStyle: "color", cleanNames: false }, customGroups: [], customGroupMemberships: {} };
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
  return { userSourcesEnabled: false, sportsEnabled: false, mode: "normal", delimiter: "pipe", virtualGroupLabel: "Categories", virtualGroupSource: "group", collapseDuplicateVirtualGroups: true, allowRecordingsByDefault: true, allowDirectProviderURLs: true, sportsFirstPlayerEnabled: false, liveRewindEnabled: false, liveRewindCacheGB: 5, liveRewindWindowMinutes: 30, liveRewindMinFreeGB: 2, liveRewindMaxChannels: 20, inferChannelNameGroups: false, ecmEnabled: false, ecmURL: "", categoryRenames: [], categoryAliases: [], eventKeywords: defaultEventKeywordRules() };
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
function sportsFirstPlayerEnabled() { return sportsEnabled(); }
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
  // Keys not normalized below (sports follows/preferences, profileSelection,
  // spoiler toggles, ...) pass through untouched; dropping them here would
  // silently erase them from the Silo profile on the next save.
  return Object.assign({}, remote, {
    favorites: Object.assign({}, remote.favorites),
    autoFavorites: Object.assign({}, remote.autoFavorites),
    favoriteOrder: uniqueIDs(items(remote.favoriteOrder)),
    hiddenCategories: Object.assign({}, remote.hiddenCategories),
    sportsFavoriteTeams: Object.assign({}, remote.sportsFavoriteTeams),
    keywordPasses: normalizeKeywordPasses(remote.keywordPasses),
    groupCategoriesByPipe: remote.groupCategoriesByPipe === true,
    recentSearches: uniqueIDs(items(remote.recentSearches).map(function(value) { return String(value || "").trim(); }).filter(Boolean)).slice(0, 12),
    recentChannels: uniqueIDs(items(remote.recentChannels)).slice(0, 24),
    continueWatching: Object.assign({}, remote.continueWatching),
    playback: Object.assign({}, remote.playback),
    categoryBrowse: Object.assign({}, defaultPrefs().categoryBrowse, remote.categoryBrowse || {}),
    customGroups: items(remote.customGroups),
    customGroupMemberships: Object.assign({}, remote.customGroupMemberships)
  });
}
function normalizePreferences() {
  if (!state.app || !state.app.preferences) return;
  state.app.preferences = Object.assign(defaultPrefs(), state.app.preferences || {});
  state.app.preferences.sportsFavoriteLeagues = state.app.preferences.sportsFavoriteLeagues || {};
  state.app.preferences.sportsPreferredChannels = state.app.preferences.sportsPreferredChannels || {};
  state.app.preferences.sportsPreferredNetworks = state.app.preferences.sportsPreferredNetworks || {};
  state.app.preferences.recentSearches = uniqueIDs(items(state.app.preferences.recentSearches).map(function(value) { return String(value || "").trim(); }).filter(Boolean)).slice(0, 12);
  state.app.preferences.customGroups = items(state.app.preferences.customGroups);
  state.app.preferences.customGroupMemberships = state.app.preferences.customGroupMemberships || {};
  state.app.preferences.categoryBrowse = Object.assign({}, defaultPrefs().categoryBrowse, state.app.preferences.categoryBrowse || {});
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
  state.adminCategorySettings.allowDirectProviderURLs = state.adminCategorySettings.allowDirectProviderURLs !== false;
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
