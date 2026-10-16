// XC Admin: sources and organization settings, and users' own providers.
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
async function loadAdminApp() {
  state.app = { preferences: defaultPrefs(), channels: [], programs: [], source: { mode: "xtream" }, status: {} };
  const results = await Promise.all([
    getJSON("/dispatcharr/api/status").catch(function() { return {}; }),
    getJSON("/dispatcharr/api/admin-settings").catch(function() { return defaultAdminCategorySettings(); }),
    getJSON("/dispatcharr/api/admin-sources")
  ]);
  state.app.status = results[0] || {};
  state.adminCategorySettings = readAdminSettingsValue(results[1]);
  normalizeAdminCategorySettings();
  state.savedAdminCategorySettings = cloneAdminCategorySettings(state.adminCategorySettings);
  state.adminSources = items(results[2] && results[2].sources);
  render();
}
// The /api/app bootstrap carries isAdmin (stamped from X-Silo-User-Role by the
// host). Anything other than an explicit true is treated as a regular user.
function siloUserIsAdmin() {
  return isAdminRoute || !!(state.app && state.app.isAdmin === true);
}
function adminOnlyDenied(error, action) {
  if (!error || Number(error.status || 0) !== 403) return false;
  showAppToast("Only Silo admins can " + action + ".");
  return true;
}
function adminListingTitle() {
  if (categoryParsing().enabled) return configuredGroupLabel();
  return "Categories";
}
function adminListingCategories(parentPath, includeChannel) {
  const hidden = hiddenMap();
  includeChannel = includeChannel || function(channel) { return !(channel.categoryId && hidden[channel.categoryId]); };
  if (categoryParsing().enabled) return parentPath ? sourceVirtualChildCategories(parentPath, includeChannel) : sourceVirtualChildCategories("", includeChannel);
  return sourceCategoriesWithChannels(includeChannel);
}
// Per-user providers: the admin's source editor also serves each user's own
// providers (/api/my-sources). The server keeps them private to that user and
// never returns a saved password.
function userSourcesActive() {
  return !isAdminRoute && !!(state.app && state.app.userSources);
}
function userSourcesMissing() {
  return userSourcesActive() && state.app.sourcesConfigured === false;
}
function sourceManagerEndpoint() {
  return userSourcesActive() ? "/dispatcharr/api/my-sources" : "/dispatcharr/api/admin-sources";
}
function sourceRefreshEndpoint() {
  return userSourcesActive() ? "/dispatcharr/api/my-refresh" : "/dispatcharr/api/refresh-channels";
}
function rerenderSourceManager() {
  if (isAdminRoute) renderAdminPage();
  else if (state.view === "settings") renderSettings();
}
function renderMyProvidersCard() {
  if (!userSourcesActive()) return "";
  if (state.adminSources === undefined) {
    state.adminSources = null;
    getJSON(sourceManagerEndpoint()).then(function(payload) {
      state.adminSources = items(payload && payload.sources);
      rerenderSourceManager();
    }).catch(function(error) {
      state.adminSources = [];
      state.adminSourceMessage = "Could not load your providers: " + readableError(error);
      rerenderSourceManager();
    });
  }
  const actions = "<div class=\"my-providers-actions\"><button class=\"admin-outline-action\" type=\"button\" data-source-action=\"refresh\">" + icon("loader") + "<span>Refresh</span></button><button class=\"admin-primary-action\" type=\"button\" data-source-action=\"add\">" + icon("plus") + "<span>Add provider</span></button></div>";
  return "<div class=\"settings-card my-providers-card\"><div class=\"settings-card-head\"><div><h2>My providers</h2><p>Your own Xtream Codes accounts. Only you can use them; passwords are never shown again after saving, including to Silo admins.</p></div>" + actions + "</div></div>"
    + (state.adminSources === null ? "<div class=\"settings-card\"><div class=\"empty\">Loading your providers…</div></div>" : renderAdminSourcesTab());
}
function adminSaveStatusHTML() {
  if (!state.adminSaveMessage) return "";
  const warning = state.adminSaveStatus === "error" || state.adminSaveStatus === "dirty";
  return "<div class=\"settings-note" + (warning ? " settings-warning" : "") + "\">" + escapeHTML(state.adminSaveMessage) + "</div>";
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
  renderAdminTopbarTabs();
  renderAdminTopbarActions();
  const shell = document.querySelector(".shell");
  if (shell) shell.classList.remove("is-admin-manager");
  byId("view").innerHTML = "<div class=\"settings-stack\">" + (state.adminTab === "sources" ? renderAdminSourcesTab() : renderAdminSettingsTab()) + "</div>";
  if (state.adminTab === "sources") return;
  if (state.adminTab === "settings") {
    renderAdminCategorySettings();
    renderAdminCategoryAliasSettings();
  }
}
function renderAdminTopbarTabs() {
  const root = byId("admin-tabs");
  if (!root) return;
  root.innerHTML = "<button type=\"button\" data-admin-tab=\"sources\" class=\"" + (state.adminTab === "sources" ? "active" : "") + "\">" + icon("integrations") + "<span>Sources</span><small>" + items(state.adminSources).length + "</small></button>"
    + "<button type=\"button\" data-admin-tab=\"settings\" class=\"" + (state.adminTab === "settings" ? "active" : "") + "\">" + icon("settings") + "<span>Organization</span></button>";
}
function renderAdminTopbarActions() {
  const root = byId("admin-actions");
  if (!root) return;
  if (state.adminTab === "sources") {
    root.innerHTML = "<button class=\"admin-outline-action\" type=\"button\" data-source-action=\"refresh\">" + icon("loader") + "<span>Refresh Catalog</span></button><button class=\"admin-primary-action\" type=\"button\" data-source-action=\"add\">" + icon("plus") + "<span>Add Source</span></button>";
    return;
  }
  const dirty = adminSettingsDirty();
  const saving = state.adminSaveStatus === "saving";
  root.innerHTML = "<button class=\"admin-save\" data-admin-settings-action=\"save\"" + ((!dirty || saving) ? " disabled" : "") + ">Save</button><button class=\"admin-discard\" data-admin-settings-action=\"discard\"" + ((!dirty || saving) ? " disabled" : "") + ">Discard</button>";
}
function setAdminTab(tab) {
  state.adminTab = tab === "settings" ? "settings" : "sources";
  renderAdminPage();
}
function renderAdminSourcesTab() {
  const sources = items(state.adminSources);
  const rows = sources.map(function(source) {
    const status = source.enabled ? "Enabled" : "Disabled";
    const epg = source.alternateEpgEnabled ? "Alternate EPG · " + (source.alternateEpgPolicy === "prefer_alternate" ? "Preferred" : "Fill missing") : "Provider EPG";
    const accounts = items(source.accounts);
    const active = accounts.reduce(function(total, account) { return total + (Number(account.activeConnections) || 0); }, 0);
    const capacity = accounts.reduce(function(total, account) { return total + (Number(account.connectionLimit) || 0); }, 0);
    const accountSummary = accounts.length ? accounts.length + " account" + (accounts.length === 1 ? "" : "s") : "1 account";
    return "<div class=\"source-table-row\"><div class=\"source-primary\"><strong>" + escapeHTML(source.name || source.id) + "</strong><small>" + escapeHTML(source.baseUrl || "Server not configured") + "</small><small class=\"source-epg-summary\">" + escapeHTML(epg) + "</small></div><div class=\"source-user\"><span>" + escapeHTML(accountSummary) + "</span><small>" + escapeHTML(String(active) + " active" + (capacity ? " · " + capacity + " pooled" : "")) + "</small></div><div class=\"source-count\"><strong>" + escapeHTML(String(source.channelCount || 0)) + "</strong><small>Channels</small></div><div class=\"source-format\"><span>" + escapeHTML(String(source.liveFormat || "m3u8").toUpperCase()) + "</span><small>Live format</small></div><div class=\"source-state\"><span class=\"source-status" + (source.enabled ? " enabled" : "") + "\">" + status + "</span></div><div class=\"source-actions\"><button type=\"button\" data-source-action=\"refresh-source\" data-source-id=\"" + escapeHTML(source.id) + "\" aria-label=\"Refresh " + escapeHTML(source.name || source.id) + "\">" + icon("loader") + "<span>Refresh</span></button><button type=\"button\" data-source-action=\"test\" data-source-id=\"" + escapeHTML(source.id) + "\">Test</button><button type=\"button\" data-source-action=\"edit\" data-source-id=\"" + escapeHTML(source.id) + "\">Edit</button><button type=\"button\" data-source-action=\"toggle\" data-source-id=\"" + escapeHTML(source.id) + "\">" + (source.enabled ? "Disable" : "Enable") + "</button><button type=\"button\" class=\"danger\" data-source-action=\"delete\" data-source-id=\"" + escapeHTML(source.id) + "\">Delete</button></div></div>";
  }).join("");
  const message = state.adminSourceMessage ? "<div class=\"settings-note" + (state.adminSourceMessage.indexOf("Could not") === 0 ? " settings-warning" : "") + "\">" + escapeHTML(state.adminSourceMessage) + "</div>" : "";
  const header = rows ? "<div class=\"source-table-head\"><span>Source</span><span>Pool</span><span>Channels</span><span>Format</span><span>Status</span><span>Actions</span></div>" : "";
  const perUserNote = isAdminRoute && adminSettings().userSourcesEnabled === true ? "<div class=\"settings-note settings-warning\">Users add their own providers (Organization tab). These shared sources are not used for anyone&#39;s channels or playback while that is on.</div>" : "";
  return "<div class=\"settings-card source-manager-card\">" + perUserNote + message + "<div class=\"source-table\">" + header + (rows || "<div class=\"source-empty\"><strong>No sources configured</strong><span>Add an Xtreme Codes account to begin building the Live TV catalog.</span></div>") + "</div></div>" + renderAdminSourceEditor();
}
function renderAdminSourceEditor() {
  const source = state.adminSourceEditor;
  if (!source) return "";
  const editing = !!source.id;
  const step = state.adminSourceEditorStep || "general";
  const epgResult = state.adminSourceEPGResult;
  const epgError = state.adminSourceEPGError;
  const nav = [{ id: "general", label: "General", icon: "settings" }, { id: "connection", label: "Connection", icon: "integrations" }, { id: "accounts", label: "Sub-accounts", icon: "integrations" }, { id: "guide", label: "Alternate EPG", icon: "guide" }, { id: "playback", label: "Playback", icon: "play" }].map(function(item) {
    return "<button type=\"button\" data-source-step=\"" + item.id + "\" class=\"" + (step === item.id ? "active" : "") + "\">" + icon(item.icon) + "<span>" + item.label + "</span></button>";
  }).join("");
  let content = "";
  if (step === "general") content = "<div class=\"source-step-copy\"><h3>General</h3><p>Name this source and choose whether it participates in catalog refreshes.</p></div><div class=\"source-form\"><label class=\"source-field-wide\"><span>Display name <small>· optional</small></span><input id=\"source-name\" value=\"" + escapeHTML(source.name || "") + "\" placeholder=\"Defaults to provider domain\"></label><label class=\"source-field-wide\"><span>Source ID</span><input value=\"" + escapeHTML(derivedSourceID(source.baseUrl, source.username) || source.id || "") + "\" placeholder=\"Generated after entering server and username\" readonly><small>Generated from the provider domain and username. Saving migrates legacy IDs such as primary.</small></label><label class=\"source-enabled\"><span class=\"source-enabled-copy\"><strong>Enabled</strong><small>Include this source in channel, guide, and content refreshes.</small></span><span class=\"source-switch-control\"><input id=\"source-enabled\" type=\"checkbox\" aria-label=\"Enabled\"" + (source.enabled !== false ? " checked" : "") + "><span class=\"source-switch\" aria-hidden=\"true\"></span></span></label></div>";
  if (step === "connection") content = "<div class=\"source-step-copy\"><h3>Connection</h3><p>Enter the Xtreme Codes server and account credentials.</p></div><div class=\"source-form\"><label class=\"source-field-wide\"><span>Server URL</span><input id=\"source-url\" type=\"url\" value=\"" + escapeHTML(source.baseUrl || "") + "\" placeholder=\"https://provider.example.com\"><small>Use the provider base URL, not player_api.php.</small></label><label><span>Username</span><input id=\"source-username\" value=\"" + escapeHTML(source.username || "") + "\" autocomplete=\"off\"></label><label><span>Password" + (source.passwordConfigured ? " · leave blank to keep current" : "") + "</span><input id=\"source-password\" type=\"password\" value=\"" + escapeHTML(source.password || "") + "\" autocomplete=\"new-password\"></label><div class=\"source-step-action\"><button type=\"button\" data-source-action=\"test-editor\">Test connection</button></div></div>";
  if (step === "accounts") content = renderSourceAccountsStep(source);
  if (step === "guide") content = "<div class=\"source-step-copy\"><h3>Alternate EPG</h3><p>Overlay an optional XMLTV feed onto this source. Channel and playback IDs are never changed.</p></div><div class=\"source-form\"><label class=\"source-enabled\"><span class=\"source-enabled-copy\"><strong>Use alternate EPG</strong><small>Provider guide remains available if this feed fails.</small></span><span class=\"source-switch-control\"><input id=\"source-alternate-epg-enabled\" type=\"checkbox\" aria-label=\"Use alternate EPG\"" + (source.alternateEpgEnabled ? " checked" : "") + "><span class=\"source-switch\" aria-hidden=\"true\"></span></span></label><label class=\"source-field-wide\"><span>XMLTV URL</span><input id=\"source-alternate-epg-url\" type=\"url\" value=\"" + escapeHTML(source.alternateEpgUrl || "") + "\" placeholder=\"https://epg.example/guide.xml\"><small>Use a feed you are authorized to access. XC for Silo does not bundle a third-party guide.</small></label><label class=\"source-field-wide\"><span>Merge policy</span><select id=\"source-alternate-epg-policy\"><option value=\"fill_missing\"" + (source.alternateEpgPolicy !== "prefer_alternate" ? " selected" : "") + ">Fill missing guide data</option><option value=\"prefer_alternate\"" + (source.alternateEpgPolicy === "prefer_alternate" ? " selected" : "") + ">Prefer alternate guide</option></select><small>Fill missing preserves overlapping Xtream programs and fills actual schedule gaps. Prefer alternate replaces programs only on matched channels.</small></label>" + (epgResult ? "<div class=\"source-epg-result\"><strong>Coverage test passed</strong><span>" + escapeHTML(String(epgResult.matchedChannels)) + " matched · " + escapeHTML(String(epgResult.unmatchedChannels)) + " unmatched · " + escapeHTML(String(epgResult.programCount)) + " programs</span></div>" : "") + (epgError ? "<div class=\"source-epg-result error\"><strong>Coverage test failed</strong><span>" + escapeHTML(epgError) + "</span></div>" : "") + "<div class=\"source-step-action\"><button type=\"button\" data-source-action=\"test-epg\">Test EPG and coverage</button></div></div>";
  if (step === "playback") content = "<div class=\"source-step-copy\"><h3>Playback</h3><p>Choose the live stream container requested from this provider.</p></div><div class=\"source-format-options\"><button type=\"button\" data-source-format=\"m3u8\" class=\"" + (source.liveFormat !== "ts" ? "active" : "") + "\"><strong>HLS</strong><span>.m3u8 · recommended for browsers</span></button><button type=\"button\" data-source-format=\"ts\" class=\"" + (source.liveFormat === "ts" ? "active" : "") + "\"><strong>MPEG-TS</strong><span>.ts · provider compatibility</span></button></div>" + (source.liveFormat === "ts" ? "" : "<div class=\"source-form\"><label class=\"source-field-wide\"><span>Live resilience buffer</span><input id=\"source-hls-buffer-seconds\" type=\"number\" min=\"5\" max=\"60\" step=\"1\" value=\"" + escapeHTML(String(boundedHLSBufferSeconds(source.hlsBufferSeconds))) + "\"><small>Seconds behind the live edge. Playback still starts immediately; higher values trade live delay for fewer stalls.</small></label></div>");
  return "<div class=\"source-dialog-backdrop\"><section class=\"source-dialog\" role=\"dialog\" aria-modal=\"true\" aria-labelledby=\"source-dialog-title\"><header><div class=\"source-dialog-icon\">" + icon("guide") + "</div><div><h2 id=\"source-dialog-title\">" + (editing ? "Edit Source" : "Add Source") + "</h2><p>Configure an Xtreme Codes provider account.</p></div><button type=\"button\" class=\"source-close\" data-source-action=\"cancel\" aria-label=\"Close source editor\">" + icon("x") + "</button></header><div class=\"source-dialog-body\"><nav class=\"source-dialog-nav\" aria-label=\"Source setup steps\">" + nav + "</nav><div class=\"source-dialog-content\">" + content + "</div></div><footer><button type=\"button\" data-source-action=\"cancel\">Cancel</button><button type=\"button\" class=\"admin-save\" data-source-action=\"save\">" + (editing ? "Save Changes" : "Add Source") + "</button></footer></section></div>";
}
function renderSourceAccountsStep(source) {
  ensureSourceCatalogAccount(source);
  const accounts = items(source.accounts);
  const catalogID = source.catalogAccountId || (accounts.find(function(account) { return account.catalog; }) || {}).id || "";
  const rows = accounts.map(function(account, index) {
    const catalog = account.id === catalogID || account.catalog;
    const status = catalog || account.compatible ? "Ready" : "Test required";
    return "<div class=\"source-account-row\" data-account-row=\"" + index + "\"><div class=\"source-account-head\"><div><strong>" + escapeHTML(account.name || account.username || (catalog ? "Catalog account" : "Playback account")) + "</strong><small>" + (catalog ? "Catalog + playback" : status) + (account.activeConnections ? " · " + account.activeConnections + " active" : "") + "</small></div>" + (catalog ? "<span class=\"source-account-badge\">Primary</span>" : "<button type=\"button\" class=\"danger\" data-account-action=\"remove\" data-account-index=\"" + index + "\" aria-label=\"Remove account\">" + icon("x") + "</button>") + "</div><div class=\"source-account-fields\"><label><span>Name</span><input data-account-field=\"name\" data-account-index=\"" + index + "\" value=\"" + escapeHTML(account.name || "") + "\" placeholder=\"Backup account\"></label><label><span>Username</span><input data-account-field=\"username\" data-account-index=\"" + index + "\" value=\"" + escapeHTML(account.username || "") + "\"" + (catalog ? " readonly" : "") + "></label><label><span>Password" + (account.passwordConfigured ? " · leave blank to keep" : "") + "</span><input type=\"password\" data-account-field=\"password\" data-account-index=\"" + index + "\" value=\"\" autocomplete=\"new-password\"" + (catalog ? " readonly" : "") + "></label><label><span>Stream limit <small>· 0 = unlimited</small></span><input type=\"number\" min=\"0\" data-account-field=\"connectionLimit\" data-account-index=\"" + index + "\" value=\"" + escapeHTML(String(account.connectionLimit || 0)) + "\"></label></div><div class=\"source-account-controls\"><label><input type=\"checkbox\" data-account-field=\"enabled\" data-account-index=\"" + index + "\"" + (account.enabled !== false ? " checked" : "") + (catalog ? " disabled" : "") + "> Enabled for playback</label>" + (!catalog ? "<button type=\"button\" data-account-action=\"test\" data-account-index=\"" + index + "\">Test account</button>" : "") + "</div></div>";
  }).join("");
  return "<div class=\"source-step-copy\"><h3>Sub-accounts</h3><p>Pool credentials for this provider. The primary account builds the catalog; compatible enabled accounts share playback.</p></div><div class=\"source-account-list\">" + (rows || "<div class=\"source-account-empty\">The connection account becomes the primary catalog account when you save.</div>") + "</div><div class=\"source-step-action source-account-add-action\"><button type=\"button\" data-account-action=\"add\">" + icon("plus") + "<span>Add playback account</span></button></div>";
}
function sourceAccountID(username) {
  return String(username || "catalog").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") || "catalog";
}
function derivedSourceID(baseUrl, username) {
  let host = "";
  try { host = new URL(String(baseUrl || "").trim()).host; } catch (_) {}
  return (host + "-" + String(username || "").trim()).toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}
function adminSourceByID(id) {
  return items(state.adminSources).find(function(source) { return source.id === id; }) || null;
}
function sourceEditorPayload(action) {
  const source = state.adminSourceEditor || {};
  ensureSourceCatalogAccount(source);
  let accounts = items(source.accounts).map(function(account) { return Object.assign({}, account); });
  let catalog = accounts.find(function(account) { return account.id === source.catalogAccountId || account.catalog; });
  if (catalog) {
    catalog.username = source.username || catalog.username || "";
    if (source.password) catalog.password = source.password;
    catalog.enabled = true;
    catalog.catalog = true;
    catalog.compatible = true;
  }
  return { action: action || "save", id: source.id || "", name: source.name || "", baseUrl: source.baseUrl || "", username: source.username || "", password: source.password || "", liveFormat: source.liveFormat || "m3u8", hlsBufferSeconds: boundedHLSBufferSeconds(source.hlsBufferSeconds), enabled: source.enabled !== false, alternateEpgEnabled: !!source.alternateEpgEnabled, alternateEpgUrl: source.alternateEpgUrl || "", alternateEpgPolicy: source.alternateEpgPolicy || "fill_missing", catalogAccountId: source.catalogAccountId || (catalog && catalog.id) || "", accounts: accounts };
}
async function refreshAdminSources() {
  const payload = await getJSON(sourceManagerEndpoint());
  state.adminSources = items(payload && payload.sources);
  rerenderSourceManager();
}
async function submitAdminSource(payload, successMessage) {
  const requestID = ++state.adminSourceRequestID;
  state.adminSourceMessage = "";
  if (payload.action === "test_epg") {
    state.adminSourceEPGResult = null;
    state.adminSourceEPGError = "";
    rerenderSourceManager();
  }
  try {
    const result = await postJSON(sourceManagerEndpoint(), payload);
    if (requestID !== state.adminSourceRequestID) return;
    if (result && result.sources) state.adminSources = items(result.sources);
    if (payload.action === "test_epg" && result && result.coverage) {
      state.adminSourceEPGResult = result.coverage;
      state.adminSourceEPGError = "";
      state.adminSourceMessage = "Alternate EPG matched " + result.coverage.matchedChannels + " channels, left " + result.coverage.unmatchedChannels + " unmatched, and supplied " + result.coverage.programCount + " programs.";
    } else state.adminSourceMessage = successMessage;
    if (payload.action === "test_account" && result && result.compatible) {
      const account = items(state.adminSourceEditor && state.adminSourceEditor.accounts).find(function(candidate) { return candidate.id === result.accountId; });
      if (account) account.compatible = true;
      state.adminSourceMessage = "Playback account verified.";
    }
    if (payload.action !== "test" && payload.action !== "test_epg" && payload.action !== "test_account") state.adminSourceEditor = null;
    if (userSourcesActive() && payload.action !== "test" && payload.action !== "test_epg" && payload.action !== "test_account") state.app.sourcesConfigured = items(state.adminSources).length > 0;
  } catch (error) {
    if (requestID !== state.adminSourceRequestID) return;
    const errorPrefix = payload.action === "test" ? "Connection test failed: " : (payload.action === "test_account" ? "Playback account test failed: " : (payload.action === "test_epg" ? "Alternate EPG test failed: " : "Could not save source: "));
    state.adminSourceMessage = errorPrefix + readableError(error);
    if (payload.action === "test_epg") {
      state.adminSourceEPGResult = null;
      state.adminSourceEPGError = readableError(error);
    }
  }
  rerenderSourceManager();
}
function handleSourceAccountAction(action, index) {
  const source = state.adminSourceEditor;
  if (!source) return;
  source.accounts = items(source.accounts);
  if (action === "add") {
    source.accounts.push({ id: "", name: "", username: "", password: "", enabled: true, compatible: false, connectionLimit: 0 });
    rerenderSourceManager();
    return;
  }
  const account = source.accounts[index];
  if (!account) return;
  if (action === "remove") {
    source.accounts.splice(index, 1);
    rerenderSourceManager();
    return;
  }
  if (action === "test") {
    if (!account.id) account.id = String(account.username || "account").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
    const payload = sourceEditorPayload("test_account");
    payload.accountId = account.id;
    submitAdminSource(payload, "Playback account verified.");
  }
}
function handleAdminSourceAction(action, sourceID) {
  const source = adminSourceByID(sourceID);
  if (action === "refresh") {
    state.adminSourceMessage = "Catalog refresh queued.";
    postJSON(sourceRefreshEndpoint(), {}).then(function() { refreshAdminSources(); }).catch(function(error) {
      state.adminSourceMessage = "Could not refresh catalog: " + readableError(error);
      rerenderSourceManager();
    });
    rerenderSourceManager();
    return;
  }
  if (action === "refresh-source" && source) {
    state.adminSourceMessage = "Refreshing " + (source.name || source.id) + "…";
    postJSON(sourceRefreshEndpoint(), {}).then(function() {
      state.adminSourceMessage = (source.name || source.id) + " catalog refreshed.";
      return refreshAdminSources();
    }).catch(function(error) {
      state.adminSourceMessage = "Could not refresh " + (source.name || source.id) + ": " + readableError(error);
      rerenderSourceManager();
    });
    rerenderSourceManager();
    return;
  }
  if (action === "add") { state.adminSourceEditor = { enabled: true, liveFormat: "m3u8", hlsBufferSeconds: 12, alternateEpgEnabled: false, alternateEpgPolicy: "fill_missing", accounts: [] }; state.adminSourceEditorStep = "general"; state.adminSourceEPGResult = null; state.adminSourceEPGError = ""; }
  if (action === "edit" && source) { state.adminSourceEditor = Object.assign({}, source, { password: "", accounts: items(source.accounts).map(function(account) { return Object.assign({}, account, { password: "" }); }) }); state.adminSourceEditorStep = "general"; state.adminSourceEPGResult = null; state.adminSourceEPGError = ""; }
  if (action === "cancel") state.adminSourceEditor = null;
  if (action === "save") return submitAdminSource(sourceEditorPayload("save"), "Source saved. Catalog refresh queued.");
  if (action === "test-editor") return submitAdminSource(sourceEditorPayload("test"), "Connection successful.");
  if (action === "test-epg") return submitAdminSource(sourceEditorPayload("test_epg"), "Alternate EPG is available.");
  if (action === "test" && source) return submitAdminSource(Object.assign({}, source, { action: "test", password: "" }), "Connection successful.");
  if (action === "toggle" && source) return submitAdminSource(Object.assign({}, source, { action: "save", enabled: !source.enabled, password: "" }), source.enabled ? "Source disabled." : "Source enabled. Catalog refresh queued.");
  if (action === "delete" && source) return submitAdminSource({ action: "delete", id: source.id }, "Source deleted. Catalog refresh queued.");
  rerenderSourceManager();
}
function renderAdminSettingsTab() {
  return ""
    + '<div class="settings-card"><div class="settings-card-head"><div><h2>Provider accounts</h2><p>Choose whether everyone shares the sources on the Sources tab, or each user brings their own.</p></div></div><label class="settings-row"><span><strong>Users add their own providers</strong><small>Each user enters their own Xtream Codes server and credentials in Settings and gets their own channels and guide. Users without a provider see nothing; shared sources are not used. Admins cannot view or use anyone&#39;s credentials.</small></span><input type="checkbox" data-admin-category-field="userSourcesEnabled"' + (adminSettings().userSourcesEnabled === true ? ' checked' : '') + '></label></div>'
    + '<div class="settings-card"><div class="settings-card-head"><div><h2>Playback privacy</h2><p>Choose whether streams that cannot be relayed may expose provider credentials.</p></div></div><label class="settings-row"><span><strong>Allow direct provider URLs</strong><small>TS live, VOD, series and catch-up can&#39;t be relayed through Silo, so playback sends the provider URL, including your account credentials, to the viewer&#39;s browser. Turn off to block those streams unless the provider serves HLS.</small></span><input type="checkbox" data-admin-category-field="allowDirectProviderURLs"' + (adminSettings().allowDirectProviderURLs !== false ? ' checked' : '') + '></label></div>'
    + '<div class="settings-card"><div class="settings-card-head"><div><h2>Sports</h2><p>Live scores and matched coverage. Replays come only from provider REPLAY channels.</p></div></div><label class="settings-row"><span><strong>Enable Sports</strong><small>Show Sports browsing and team follows.</small></span><input type="checkbox" data-admin-category-field="sportsEnabled"' + (sportsEnabled() ? ' checked' : '') + '></label></div>'
    + "<div class=\"settings-card organization-card\"><div class=\"settings-card-head\"><div><h2>Category organization</h2><p>Choose how provider categories become folders in XC for Silo.</p></div></div><section class=\"organization-section\"><div id=\"admin-category-settings\" class=\"settings-list\"></div></section><section class=\"organization-section organization-alias-section\"><div class=\"organization-section-head\"><div><h3>Alternate paths</h3><p>Show a provider category in another location without changing the source.</p></div></div><div id=\"admin-category-alias-settings\" class=\"settings-list\"></div></section></div>"
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
function renderAdminECMSettings() {
  const settings = adminSettings();
  const root = byId("admin-ecm-settings");
  if (!root) return;
  root.innerHTML = adminSaveStatusHTML() + "<div class=\"settings-row ecm-url-row compact-row\"><span><strong>ECM URL</strong><small>Leave blank to hide Channel Manager.</small></span><input type=\"url\" data-admin-ecm-field=\"url\" value=\"" + escapeHTML(settings.ecmURL || "") + "\"></div>";
}
function renderAdminCategorySettings() {
  const settings = adminSettings();
  const root = byId("admin-category-settings");
  const sourceHelp = "Choose which Xtream names Silo uses to build nested folders.";
  const nested = settings.mode !== "normal" ? "<div class=\"settings-list-nested\">"
    + "<div class=\"settings-row settings-form-row\"><span class=\"settings-field-copy\"><strong>Build folders from</strong><small>" + escapeHTML(sourceHelp) + "</small></span><select data-admin-category-field=\"virtualGroupSource\"><option value=\"group\"" + (virtualGroupSourceMode() === "group" ? " selected" : "") + ">Provider categories</option><option value=\"group_channel\"" + (virtualGroupSourceMode() === "group_channel" ? " selected" : "") + ">Provider categories and channel names</option><option value=\"channel\"" + (virtualGroupSourceMode() === "channel" ? " selected" : "") + ">Channel names only</option></select></div>"
    + "<div class=\"settings-row settings-form-row\"><span class=\"settings-field-copy\"><strong>Separator</strong><small>Each matching separator starts another folder level.</small></span><select data-admin-category-field=\"delimiter\"><option value=\"pipe\"" + (settings.delimiter === "pipe" ? " selected" : "") + ">Pipe · Sports | NHL Teams</option><option value=\"dash\"" + (settings.delimiter === "dash" ? " selected" : "") + ">Dash · Sports - NHL Teams</option></select></div>"
    + "<div class=\"settings-row settings-form-row virtual-label-row\"><span class=\"settings-field-copy\"><strong>Folder collection label</strong><small>Name the top-level collection that contains generated folders.</small></span><div class=\"virtual-label-control\"><span>Virtual</span><input data-admin-category-field=\"virtualGroupLabel\" value=\"" + escapeHTML(virtualGroupLabelSuffix(settings.virtualGroupLabel)) + "\" placeholder=\"Categories\"></div></div>"
    + "<label class=\"settings-row settings-form-row\"><span class=\"settings-field-copy\"><strong>Collapse repeated folders</strong><small>Remove adjacent duplicate names when category and channel paths overlap.</small></span><input type=\"checkbox\" data-admin-category-field=\"collapseDuplicateVirtualGroups\"" + (settings.collapseDuplicateVirtualGroups !== false ? " checked" : "") + "></label>"
    + renderOrganizationPreview(settings)
    + "</div>" : "";
  root.innerHTML = adminSaveStatusHTML()
    + "<div class=\"settings-row settings-form-row\"><span class=\"settings-field-copy\"><strong>Folder structure</strong><small>Keep provider categories intact or split their names into nested folders.</small></span><select data-admin-category-field=\"mode\"><option value=\"normal\"" + (settings.mode === "normal" ? " selected" : "") + ">Keep provider categories</option><option value=\"delimiter\"" + (settings.mode === "delimiter" ? " selected" : "") + ">Split by separator</option></select></div>"
    + nested
    + (settings.mode === "normal" ? "<div class=\"settings-note organization-mode-note\">Provider categories appear exactly as supplied by each Xtream source.</div>" : "");
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
  if (settings.mode !== "delimiter") {
    root.innerHTML = "<div class=\"settings-note alias-disabled-state\"><strong>Available with split folders</strong><span>Choose <em>Split by separator</em> above to create alternate category paths.</span></div>";
    return;
  }
  if (!sourceGroups.length && !aliases.length) {
    root.innerHTML = "<div class=\"settings-note alias-disabled-state\"><strong>No provider categories loaded</strong><span>Refresh the catalog after enabling a source, then return here to add alternate paths.</span></div>";
    return;
  }
  const addRow = "<div class=\"alias-builder\"><label><span>Provider category</span><select id=\"admin-alias-source\"" + (!sourceGroups.length ? " disabled" : "") + ">" + sourceOptions + "</select></label><label><span>Alternate path</span><input id=\"admin-alias-path\" placeholder=\"Sports | Arabic\"" + (!sourceGroups.length ? " disabled" : "") + "></label><button data-admin-alias-action=\"add\"" + (!sourceGroups.length ? " disabled" : "") + ">Add path</button></div>";
  const rows = aliases.map(function(alias, index) {
    const count = adminSourceGroupCount(alias.sourcePath);
    return "<div class=\"alias-table-row" + (!count ? " stale" : "") + "\"><div class=\"alias-table-source\"><strong title=\"" + escapeHTML(alias.sourcePath) + "\">" + escapeHTML(alias.sourcePath) + "</strong>" + (!count ? "<small>Source not found</small>" : "") + "</div><span class=\"alias-table-arrow\">&rarr;</span><label><input data-admin-alias-index=\"" + index + "\" data-admin-alias-field=\"aliasPath\" value=\"" + escapeHTML(alias.aliasPath) + "\" title=\"" + escapeHTML(alias.aliasPath) + "\" aria-label=\"Alternate path\"></label><span class=\"alias-table-count\">" + escapeHTML(String(count)) + "</span><span class=\"alias-table-actions\"><button data-admin-alias-action=\"remove\" data-admin-alias-index=\"" + index + "\">Remove</button></span></div>";
  }).join("");
  root.innerHTML = addRow
    + "<div class=\"alias-table\"><div class=\"alias-table-head\"><span>Provider category</span><span></span><span>Alternate path</span><span>Channels</span><span>Actions</span></div>" + (rows || "<div class=\"empty alias-empty\"><strong>No alternate paths</strong><span>Categories will only appear in their provider location.</span></div>") + "</div>";
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

function adminFeaturedEventMap() {
  const featured = {};
  items(adminSettings().featuredEventIds).forEach(function(id) {
    id = String(id || "").trim();
    if (id) featured[id] = true;
  });
  return featured;
}
