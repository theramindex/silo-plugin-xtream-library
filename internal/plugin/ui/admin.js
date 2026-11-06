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
    if (sportsEnabled()) loadSports(true, true);
    if (state.view === "admin") renderAdminPage();
  }).catch(function(error) {
    state.adminSaveStatus = "error";
    state.adminSaveMessage = "Could not save plugin settings: " + adminErrorDetail(error);
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
  if (!adminSettings().userSourcesEnabled) {
    getJSON("/dispatcharr/api/channels").then(function(payload) { state.app.channels = items(payload && payload.channels); if (state.view === "admin") renderAdminPage(); }).catch(function() {});
  }
  if (sportsEnabled()) loadSports(false).then(function() { if (state.view === "admin") renderAdminPage(); });
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
// Re-rendering the source manager replaces the whole view, so keep focus on
// the same control (or move it into a newly opened dialog).
function sourceManagerFocusSelector(element) {
  if (!element || element === document.body || !element.getAttribute) return "";
  if (element.id) return "#" + cssEscape(element.id);
  const names = ["data-source-step", "data-source-format", "data-source-action", "data-account-action", "data-account-field", "data-source-id", "data-account-index"];
  const parts = names.filter(function(name) { return element.hasAttribute(name); }).map(function(name) {
    return "[" + name + "=\"" + cssEscape(element.getAttribute(name)) + "\"]";
  });
  return parts.join("");
}
function focusSourceManagerElement(selector) {
  let target = null;
  if (selector) {
    try { target = document.querySelector(selector); } catch (_) { target = null; }
  }
  if (target && typeof target.focus === "function") {
    try { target.focus({ preventScroll: true }); } catch (_) { target.focus(); }
    return true;
  }
  return false;
}
function rerenderSourceManager() {
  const active = document.activeElement;
  const dialogWasOpen = !!document.querySelector(".source-dialog");
  const focusSelector = sourceManagerFocusSelector(active);
  // An open delete confirmation is rebuilt on the re-rendered Delete button.
  const pendingConfirm = activeInlineConfirm;
  const pendingConfirmSelector = pendingConfirm ? sourceManagerFocusSelector(pendingConfirm.trigger) : "";
  if (isAdminRoute) renderAdminPage();
  else if (state.view === "settings") renderSettings();
  else return;
  const dialog = document.querySelector(".source-dialog");
  if (pendingConfirmSelector && activeInlineConfirm === pendingConfirm) {
    const freshTrigger = document.querySelector(pendingConfirmSelector);
    if (freshTrigger && freshTrigger !== pendingConfirm.trigger) {
      openInlineConfirm(freshTrigger, pendingConfirm.options);
      return;
    }
  }
  if (focusSourceManagerElement(state.sourceManagerFocusNext)) {
    state.sourceManagerFocusNext = "";
    return;
  }
  state.sourceManagerFocusNext = "";
  if (focusSelector && (!dialog || !dialogWasOpen || dialog.querySelector(focusSelector)) && focusSourceManagerElement(focusSelector)) {
    if (!dialog || dialog.contains(document.activeElement)) return;
  }
  if (dialog && !dialog.contains(document.activeElement)) {
    const first = dialog.querySelector(".source-dialog-content input:not([readonly]), .source-dialog-content select, .source-dialog-content button") || dialog.querySelector("button");
    if (first) first.focus();
  }
}
function sourceEditorDirty() {
  if (!state.adminSourceEditor) return false;
  return JSON.stringify(sourceEditorPayload("save")) !== state.adminSourceEditorBaseline;
}
// Escape is easy to press by accident, so it only closes an untouched editor.
// With edits, it explains how to discard and moves focus to Cancel; the Cancel
// button still discards straight away (as in Dispatcharr's connection dialog).
function dismissSourceEditor() {
  if (!sourceEditorDirty()) {
    handleAdminSourceAction("cancel", "");
    return;
  }
  setAdminSourceMessage("You have unsaved changes. Choose Cancel to discard them, or Save to keep them.", "error");
  rerenderSourceManager();
  const cancel = document.querySelector(".source-dialog footer [data-source-action=\"cancel\"]");
  if (cancel) cancel.focus();
}
function sourceDialogKeydown(event) {
  const dialog = document.querySelector(".source-dialog");
  if (!dialog || !state.adminSourceEditor) return false;
  if (event.key === "Escape") {
    event.preventDefault();
    dismissSourceEditor();
    return true;
  }
  if (event.key !== "Tab") return false;
  const focusable = Array.prototype.slice.call(dialog.querySelectorAll("button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])"));
  if (!focusable.length) return false;
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  // Focus outside the dialog, or on a non-tabbable element inside it (the
  // dialog itself), is not a control, so wrap instead of letting it escape.
  if (focusable.indexOf(document.activeElement) === -1) {
    event.preventDefault();
    (event.shiftKey ? last : first).focus();
    return true;
  }
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
// Users with their own providers get a fresh catalog once a save, delete or
// refresh lands. The refresh runs in the background, so watch the lightweight
// status until it finishes, then reload the app once.
async function reloadUserCatalog() {
  if (!userSourcesActive()) return;
  const attempt = (state.userCatalogReloadAttempt || 0) + 1;
  state.userCatalogReloadAttempt = attempt;
  const before = (state.app && state.app.status) || {};
  const initialSuccess = Number(before.lastSuccessUnix || 0);
  const initialFailure = Number(before.lastFailureUnix || 0);
  const initialChannels = Number(before.channelCount || 0);
  for (let round = 0; round < 10 && items(state.adminSources).length; round += 1) {
    await new Promise(function(resolve) { setTimeout(resolve, round === 0 ? 1000 : 3000); });
    if (attempt !== state.userCatalogReloadAttempt) return;
    const status = await getJSON("/dispatcharr/api/status").catch(function() { return null; });
    if (attempt !== state.userCatalogReloadAttempt) return;
    if (!status) break;
    const finished = Number(status.lastSuccessUnix || 0) > initialSuccess || Number(status.lastFailureUnix || 0) > initialFailure || (!initialChannels && Number(status.channelCount || 0) > 0);
    if (finished) break;
  }
  if (attempt !== state.userCatalogReloadAttempt) return;
  try {
    await hydrateApp(await fetchAppBootstrap(), { reuseSettings: true });
  } catch (_) {
    return;
  }
  if (attempt !== state.userCatalogReloadAttempt) return;
  if (state.view === "settings") rerenderSourceManager();
  else if (state.view !== "player" && state.view !== "multiview") render();
}
function renderMyProvidersCard() {
  if (!userSourcesActive()) return "";
  // core.js starts adminSources as [], so track the first load separately.
  if (!state.myProvidersRequested) {
    state.myProvidersRequested = true;
    state.adminSources = null;
    getJSON(sourceManagerEndpoint()).then(function(payload) {
      state.adminSources = items(payload && payload.sources);
      rerenderSourceManager();
    }).catch(function(error) {
      state.adminSources = [];
      setAdminSourceMessage(errorSentence("Could not load your providers.", error), "error");
      rerenderSourceManager();
    });
  }
  const actions = "<div class=\"my-providers-actions\"><button class=\"admin-outline-action\" type=\"button\" data-source-action=\"refresh\">" + icon("loader") + "<span>Refresh</span></button><button class=\"admin-primary-action\" type=\"button\" data-source-action=\"add\">" + icon("plus") + "<span>Add provider</span></button></div>";
  return "<div class=\"settings-card my-providers-card\"><div class=\"settings-card-head\"><div><h2>My providers</h2><p>Your own Xtream Codes accounts. Only you can use them; passwords are never shown again after saving, including to Silo admins.</p></div>" + actions + "</div></div>"
    + (state.adminSources === null ? "<div class=\"settings-card\"><div class=\"empty\" role=\"status\">Loading your providers…</div></div>" : renderAdminSourcesTab());
}
function setAdminSourceMessage(message, kind) {
  state.adminSourceMessage = message || "";
  state.adminSourceMessageKind = message ? (kind || "success") : "";
}
function adminSourceMessageHTML(extraClass) {
  if (!state.adminSourceMessage) return "";
  const error = state.adminSourceMessageKind === "error";
  return "<div class=\"settings-note" + (error ? " settings-warning" : "") + (extraClass ? " " + extraClass : "") + "\" role=\"" + (error ? "alert" : "status") + "\">" + escapeHTML(state.adminSourceMessage) + "</div>";
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
    if (sportsEnabled() && !state.sports && !state.sportsLoading) loadSports(false);
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
    root.innerHTML = "<button class=\"admin-outline-action\" type=\"button\" data-source-action=\"refresh\">" + icon("loader") + "<span>Refresh catalog</span></button><button class=\"admin-primary-action\" type=\"button\" data-source-action=\"add\">" + icon("plus") + "<span>Add source</span></button>";
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
    return "<div class=\"source-table-row\"><div class=\"source-primary\"><strong>" + escapeHTML(source.name || source.id) + "</strong><small>" + escapeHTML(source.baseUrl || "Server not configured") + "</small><small class=\"source-epg-summary\">" + escapeHTML(epg) + "</small></div><div class=\"source-user\"><span>" + escapeHTML(accountSummary) + "</span><small>" + escapeHTML(String(active) + " active" + (capacity ? " · " + capacity + " pooled" : "")) + "</small></div><div class=\"source-count\"><strong>" + escapeHTML(String(source.channelCount || 0)) + "</strong><small>Channels</small></div><div class=\"source-format\"><span>" + escapeHTML(String(source.liveFormat || "m3u8").toUpperCase()) + "</span><small>Live format</small></div><div class=\"source-state\"><span class=\"source-status" + (source.enabled ? " enabled" : "") + "\">" + status + "</span></div><div class=\"source-actions\"><button type=\"button\" data-source-action=\"refresh-source\" data-source-id=\"" + escapeHTML(source.id) + "\" aria-label=\"Refresh " + escapeHTML(source.name || source.id) + "\">" + icon("loader") + "<span>Refresh</span></button><button type=\"button\" data-source-action=\"test\" data-source-id=\"" + escapeHTML(source.id) + "\">Test</button><button type=\"button\" data-source-action=\"edit\" data-source-id=\"" + escapeHTML(source.id) + "\">Edit</button><button type=\"button\" data-source-action=\"toggle\" data-source-id=\"" + escapeHTML(source.id) + "\">" + (source.enabled ? "Disable" : "Enable") + "</button>" + sourceDeleteControlHTML(source) + "</div></div>";
  }).join("");
  const message = state.adminSourceEditor ? "" : adminSourceMessageHTML();
  const header = rows ? "<div class=\"source-table-head\"><span>" + (userSourcesActive() ? "Provider" : "Source") + "</span><span>Pool</span><span>Channels</span><span>Format</span><span>Status</span><span>Actions</span></div>" : "";
  const perUserNote = isAdminRoute && adminSettings().userSourcesEnabled === true ? "<div class=\"settings-note settings-warning\">Users add their own providers (Organization tab). These shared sources are not used for anyone&#39;s channels or playback while that is on.</div>" : "";
  return "<div class=\"settings-card source-manager-card\">" + perUserNote + message + "<div class=\"source-table\">" + header + (rows || (userSourcesActive() ? "<div class=\"source-empty\"><strong>No provider yet</strong><span>Add your Xtream Codes account to build your channels and guide.</span></div>" : "<div class=\"source-empty\"><strong>No sources configured</strong><span>Add an Xtream Codes account to begin building the Live TV catalog.</span></div>")) + "</div></div>" + renderAdminSourceEditor();
}
function sourceDisplayName(source) {
  return String((source && (source.name || source.id)) || (userSourcesActive() ? "this provider" : "this source"));
}
// Delete asks first through the shared inline confirmation (see
// handleAdminSourceAction and openInlineConfirm in core.js).
function sourceDeleteControlHTML(source) {
  return "<button type=\"button\" class=\"danger\" data-source-action=\"delete\" data-source-id=\"" + escapeHTML(source.id) + "\" aria-label=\"Delete " + escapeHTML(sourceDisplayName(source)) + "\">Delete</button>";
}
// Quick checks before a test or save, so an empty or malformed field points at
// itself instead of coming back as a provider error.
function sourceEditorValidationError(source) {
  const url = String(source.baseUrl || "").trim();
  if (!url) return "Enter the provider's server URL.";
  let parsed = null;
  try { parsed = new URL(url); } catch (_) { parsed = null; }
  if (!parsed || (parsed.protocol !== "http:" && parsed.protocol !== "https:")) return "Enter the server URL with http:// or https://, for example https://provider.example.com.";
  if (/player_api\.php|get\.php/i.test(parsed.pathname)) return "Use the provider's base URL, without player_api.php or get.php.";
  if (!String(source.username || "").trim()) return "Enter the account username.";
  if (!source.id && !source.password && !source.passwordConfigured) return "Enter the account password.";
  return "";
}
function sourceBusyLabel(action, idle, busy) {
  return state.adminSourceBusy === action ? busy : idle;
}
function sourceBusyAttr(action) {
  return state.adminSourceBusy ? " disabled" + (state.adminSourceBusy === action ? " aria-busy=\"true\"" : "") : "";
}
function renderAdminSourceEditor() {
  const source = state.adminSourceEditor;
  if (!source) return "";
  const editing = !!source.id;
  const step = state.adminSourceEditorStep || "general";
  const epgResult = state.adminSourceEPGResult;
  const epgError = state.adminSourceEPGError;
  const personal = userSourcesActive();
  const steps = [{ id: "general", label: "General", icon: "settings" }, { id: "connection", label: "Connection", icon: "integrations" }, { id: "accounts", label: "Sub-accounts", icon: "integrations" }, { id: "guide", label: "Alternate EPG", icon: "guide" }, { id: "playback", label: "Playback", icon: "play" }].filter(function(item) {
    // A user's own provider is one account; pooling is an admin concern.
    return !(personal && item.id === "accounts");
  });
  const nav = steps.map(function(item) {
    return "<button type=\"button\" data-source-step=\"" + item.id + "\" class=\"" + (step === item.id ? "active" : "") + "\"" + (step === item.id ? " aria-current=\"step\"" : "") + ">" + icon(item.icon) + "<span>" + item.label + "</span></button>";
  }).join("");
  let content = "";
  const sourceIDField = personal ? "" : "<label class=\"source-field-wide\"><span>Source ID</span><input value=\"" + escapeHTML(derivedSourceID(source.baseUrl, source.username) || source.id || "") + "\" placeholder=\"Generated after entering server and username\" readonly><small>Generated from the provider domain and username. Saving migrates legacy IDs such as primary.</small></label>";
  if (step === "general") content = "<div class=\"source-step-copy\"><h3>General</h3><p>" + (personal ? "Name this provider and choose whether it is used for your channels and guide." : "Name this source and choose whether it participates in catalog refreshes.") + "</p></div><div class=\"source-form\"><label class=\"source-field-wide\"><span>Display name <small>· optional</small></span><input id=\"source-name\" value=\"" + escapeHTML(source.name || "") + "\" placeholder=\"Defaults to provider domain\"></label>" + sourceIDField + "<label class=\"source-enabled\"><span class=\"source-enabled-copy\"><strong>Enabled</strong><small>" + (personal ? "Use this provider for your channels, guide, and content." : "Include this source in channel, guide, and content refreshes.") + "</small></span><span class=\"source-switch-control\"><input id=\"source-enabled\" type=\"checkbox\" aria-label=\"Enabled\"" + (source.enabled !== false ? " checked" : "") + "><span class=\"source-switch\" aria-hidden=\"true\"></span></span></label></div>";
  if (step === "connection") content = "<div class=\"source-step-copy\"><h3>Connection</h3><p>Enter the Xtream Codes server and account credentials.</p></div><div class=\"source-form\"><label class=\"source-field-wide\"><span>Server URL</span><input id=\"source-url\" type=\"url\" value=\"" + escapeHTML(source.baseUrl || "") + "\" placeholder=\"https://provider.example.com\"><small>Use the provider base URL, not player_api.php.</small></label><label><span>Username</span><input id=\"source-username\" value=\"" + escapeHTML(source.username || "") + "\" autocomplete=\"off\"></label><label><span>Password" + (source.passwordConfigured ? " · leave blank to keep current" : "") + "</span><input id=\"source-password\" type=\"password\" value=\"" + escapeHTML(source.password || "") + "\" autocomplete=\"new-password\"></label><div class=\"source-step-action\"><button type=\"button\" data-source-action=\"test-editor\"" + sourceBusyAttr("test") + ">" + sourceBusyLabel("test", "Test connection", "Testing…") + "</button></div></div>";
  if (step === "accounts") content = renderSourceAccountsStep(source);
  if (step === "guide") content = "<div class=\"source-step-copy\"><h3>Alternate EPG</h3><p>Overlay an optional XMLTV feed onto this source. Channel and playback IDs are never changed.</p></div><div class=\"source-form\"><label class=\"source-enabled\"><span class=\"source-enabled-copy\"><strong>Use alternate EPG</strong><small>Provider guide remains available if this feed fails.</small></span><span class=\"source-switch-control\"><input id=\"source-alternate-epg-enabled\" type=\"checkbox\" aria-label=\"Use alternate EPG\"" + (source.alternateEpgEnabled ? " checked" : "") + "><span class=\"source-switch\" aria-hidden=\"true\"></span></span></label><label class=\"source-field-wide\"><span>XMLTV URL</span><input id=\"source-alternate-epg-url\" type=\"url\" value=\"" + escapeHTML(source.alternateEpgUrl || "") + "\" placeholder=\"https://epg.example/guide.xml\"><small>Use a feed you are authorized to access. XC for Silo does not bundle a third-party guide.</small></label><label class=\"source-field-wide\"><span>Merge policy</span><select id=\"source-alternate-epg-policy\"><option value=\"fill_missing\"" + (source.alternateEpgPolicy !== "prefer_alternate" ? " selected" : "") + ">Fill missing guide data</option><option value=\"prefer_alternate\"" + (source.alternateEpgPolicy === "prefer_alternate" ? " selected" : "") + ">Prefer alternate guide</option></select><small>Fill missing preserves overlapping Xtream programs and fills actual schedule gaps. Prefer alternate replaces programs only on matched channels.</small></label>" + (epgResult ? "<div class=\"source-epg-result\" role=\"status\"><strong>Coverage test passed</strong><span>" + escapeHTML(String(epgResult.matchedChannels)) + " matched · " + escapeHTML(String(epgResult.unmatchedChannels)) + " unmatched · " + escapeHTML(String(epgResult.programCount)) + " programs</span></div>" : "") + (epgError ? "<div class=\"source-epg-result error\" role=\"alert\"><strong>Coverage test failed</strong><span>" + escapeHTML(epgError) + "</span></div>" : "") + "<div class=\"source-step-action\"><button type=\"button\" data-source-action=\"test-epg\"" + sourceBusyAttr("test_epg") + ">" + sourceBusyLabel("test_epg", "Test EPG and coverage", "Testing…") + "</button></div></div>";
  if (step === "playback") content = "<div class=\"source-step-copy\"><h3>Playback</h3><p>Choose the live stream container requested from this provider.</p></div><div class=\"source-format-options\"><button type=\"button\" data-source-format=\"m3u8\" class=\"" + (source.liveFormat !== "ts" ? "active" : "") + "\"><strong>HLS</strong><span>.m3u8 · recommended for browsers</span></button><button type=\"button\" data-source-format=\"ts\" class=\"" + (source.liveFormat === "ts" ? "active" : "") + "\"><strong>MPEG-TS</strong><span>.ts · provider compatibility</span></button></div>" + (source.liveFormat === "ts" ? "" : "<div class=\"source-form\"><label class=\"source-field-wide\"><span>Live resilience buffer</span><input id=\"source-hls-buffer-seconds\" type=\"number\" min=\"5\" max=\"60\" step=\"1\" value=\"" + escapeHTML(String(boundedHLSBufferSeconds(source.hlsBufferSeconds))) + "\"><small>Seconds behind the live edge. Playback still starts immediately; higher values trade live delay for fewer stalls.</small></label></div>");
  return "<div class=\"source-dialog-backdrop\"><section class=\"source-dialog\" role=\"dialog\" aria-modal=\"true\" aria-labelledby=\"source-dialog-title\"><header><div class=\"source-dialog-icon\">" + icon("guide") + "</div><div><h2 id=\"source-dialog-title\">" + (personal ? (editing ? "Edit provider" : "Add provider") : (editing ? "Edit source" : "Add source")) + "</h2><p>" + (personal ? "Your own Xtream Codes account. Only you can use it." : "Configure an Xtream Codes provider account.") + "</p></div><button type=\"button\" class=\"source-close\" data-source-action=\"cancel\" aria-label=\"" + (personal ? "Close provider editor" : "Close source editor") + "\">" + icon("x") + "</button></header><div class=\"source-dialog-body\"><nav class=\"source-dialog-nav\" aria-label=\"Source setup steps\">" + nav + "</nav><div class=\"source-dialog-content\">" + content + "</div></div><footer><div class=\"source-dialog-status\" aria-live=\"polite\">" + adminSourceMessageHTML("source-dialog-note") + "</div><button type=\"button\" data-source-action=\"cancel\">Cancel</button><button type=\"button\" class=\"admin-save\" data-source-action=\"save\"" + sourceBusyAttr("save") + ">" + sourceBusyLabel("save", editing ? "Save changes" : (personal ? "Add provider" : "Add source"), "Saving…") + "</button></footer></section></div>";
}
function renderSourceAccountsStep(source) {
  ensureSourceCatalogAccount(source);
  const accounts = items(source.accounts);
  const catalogID = source.catalogAccountId || (accounts.find(function(account) { return account.catalog; }) || {}).id || "";
  const rows = accounts.map(function(account, index) {
    const catalog = account.id === catalogID || account.catalog;
    const status = catalog || account.compatible ? "Ready" : "Test required";
    return "<div class=\"source-account-row\" data-account-row=\"" + index + "\"><div class=\"source-account-head\"><div><strong>" + escapeHTML(account.name || account.username || (catalog ? "Catalog account" : "Playback account")) + "</strong><small>" + (catalog ? "Catalog + playback" : status) + (account.activeConnections ? " · " + account.activeConnections + " active" : "") + "</small></div>" + (catalog ? "<span class=\"source-account-badge\">Primary</span>" : "<button type=\"button\" class=\"danger\" data-account-action=\"remove\" data-account-index=\"" + index + "\" aria-label=\"Remove account " + escapeHTML(account.name || account.username || String(index + 1)) + "\">" + icon("x") + "</button>") + "</div><div class=\"source-account-fields\"><label><span>Name</span><input data-account-field=\"name\" data-account-index=\"" + index + "\" value=\"" + escapeHTML(account.name || "") + "\" placeholder=\"Backup account\"></label><label><span>Username</span><input data-account-field=\"username\" data-account-index=\"" + index + "\" value=\"" + escapeHTML(account.username || "") + "\"" + (catalog ? " readonly" : "") + "></label><label><span>Password" + (account.passwordConfigured ? " · leave blank to keep" : "") + "</span><input type=\"password\" data-account-field=\"password\" data-account-index=\"" + index + "\" value=\"\" autocomplete=\"new-password\"" + (catalog ? " readonly" : "") + "></label><label><span>Stream limit <small>· 0 = unlimited</small></span><input type=\"number\" min=\"0\" data-account-field=\"connectionLimit\" data-account-index=\"" + index + "\" value=\"" + escapeHTML(String(account.connectionLimit || 0)) + "\"></label></div><div class=\"source-account-controls\"><label><input type=\"checkbox\" data-account-field=\"enabled\" data-account-index=\"" + index + "\"" + (account.enabled !== false ? " checked" : "") + (catalog ? " disabled" : "") + "> Enabled for playback</label>" + (!catalog ? "<button type=\"button\" data-account-action=\"test\" data-account-index=\"" + index + "\"" + sourceBusyAttr("test_account") + ">" + sourceBusyLabel("test_account", "Test account", "Testing…") + "</button>" : "") + "</div></div>";
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
const SOURCE_TEST_ACTIONS = { test: true, test_epg: true, test_account: true };
function sourceErrorLead(action) {
  const personal = userSourcesActive();
  if (action === "test") return "Connection test failed.";
  if (action === "test_account") return "Playback account test failed.";
  if (action === "test_epg") return "Alternate EPG test failed.";
  if (action === "delete") return personal ? "Could not delete your provider." : "Could not delete the source.";
  return personal ? "Could not save your provider." : "Could not save the source.";
}
async function submitAdminSource(payload, successMessage) {
  const requestID = ++state.adminSourceRequestID;
  const testing = !!SOURCE_TEST_ACTIONS[payload.action];
  setAdminSourceMessage("", "");
  if (payload.action === "test_epg") {
    state.adminSourceEPGResult = null;
    state.adminSourceEPGError = "";
  }
  if (state.adminSourceEditor && (payload.action === "test" || payload.action === "save")) {
    const invalid = sourceEditorValidationError(state.adminSourceEditor);
    if (invalid) {
      setAdminSourceMessage(invalid, "error");
      if (state.adminSourceEditorStep !== "connection") state.adminSourceEditorStep = "connection";
      rerenderSourceManager();
      return;
    }
  }
  state.adminSourceBusy = payload.action || "save";
  if (state.adminSourceEditor && testing) setAdminSourceMessage(payload.action === "test_epg" ? "Testing the XMLTV feed…" : "Testing the connection…", "info");
  rerenderSourceManager();
  let result = null;
  try {
    result = await postJSON(sourceManagerEndpoint(), payload);
    if (requestID !== state.adminSourceRequestID) return;
    if (result && result.sources) state.adminSources = items(result.sources);
    if (payload.action === "test_epg" && result && result.coverage) {
      state.adminSourceEPGResult = result.coverage;
      state.adminSourceEPGError = "";
      setAdminSourceMessage("Alternate EPG matched " + result.coverage.matchedChannels + " channels, left " + result.coverage.unmatchedChannels + " unmatched, and supplied " + result.coverage.programCount + " programs.", "success");
    } else setAdminSourceMessage(successMessage, "success");
    if (payload.action === "test_account" && result && result.compatible) {
      const account = items(state.adminSourceEditor && state.adminSourceEditor.accounts).find(function(candidate) { return candidate.id === result.accountId; });
      if (account) account.compatible = true;
      setAdminSourceMessage("Playback account verified.", "success");
    }
    if (!testing) {
      if (state.adminSourceEditor) state.sourceManagerFocusNext = state.adminSourceEditorOpener || "";
      state.adminSourceEditor = null;
      state.adminSourceEditorOpener = "";
    }
    if (userSourcesActive() && !testing) state.app.sourcesConfigured = items(state.adminSources).length > 0;
  } catch (error) {
    if (requestID !== state.adminSourceRequestID) return;
    setAdminSourceMessage(errorSentence(sourceErrorLead(payload.action), error), "error");
    if (payload.action === "test_epg") {
      state.adminSourceEPGResult = null;
      state.adminSourceEPGError = friendlyError(error, "The XMLTV feed could not be loaded or did not match any channels.");
    }
    result = null;
  } finally {
    if (requestID === state.adminSourceRequestID) state.adminSourceBusy = "";
  }
  rerenderSourceManager();
  if (result && !testing) reloadUserCatalog();
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
    state.sourceManagerFocusNext = "[data-account-action=\"add\"]";
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
function handleAdminSourceAction(action, sourceID, trigger) {
  const source = adminSourceByID(sourceID);
  const personal = userSourcesActive();
  if (action !== "delete") closeInlineConfirm(false);
  if (action === "refresh") {
    setAdminSourceMessage(personal ? "Refreshing your channels and guide…" : "Catalog refresh queued.", "info");
    postJSON(sourceRefreshEndpoint(), {}).then(function() {
      if (personal) setAdminSourceMessage("Refresh started. Your channels and guide update when it finishes.", "success");
      reloadUserCatalog();
      return refreshAdminSources();
    }).catch(function(error) {
      setAdminSourceMessage(errorSentence(personal ? "Could not refresh your provider." : "Could not refresh the catalog.", error), "error");
      rerenderSourceManager();
    });
    rerenderSourceManager();
    return;
  }
  if (action === "refresh-source" && source) {
    setAdminSourceMessage("Refreshing " + sourceDisplayName(source) + "…", "info");
    postJSON(sourceRefreshEndpoint(), {}).then(function() {
      setAdminSourceMessage(sourceDisplayName(source) + " refresh started.", "success");
      reloadUserCatalog();
      return refreshAdminSources();
    }).catch(function(error) {
      setAdminSourceMessage(errorSentence("Could not refresh " + sourceDisplayName(source) + ".", error), "error");
      rerenderSourceManager();
    });
    rerenderSourceManager();
    return;
  }
  if (action === "add" || (action === "edit" && source)) {
    state.adminSourceEditorOpener = action === "add" ? "[data-source-action=\"add\"]" : "[data-source-action=\"edit\"][data-source-id=\"" + cssEscape(source.id) + "\"]";
    setAdminSourceMessage("", "");
  }
  if (action === "add") { state.adminSourceEditor = { enabled: true, liveFormat: "m3u8", hlsBufferSeconds: 12, alternateEpgEnabled: false, alternateEpgPolicy: "fill_missing", accounts: [] }; state.adminSourceEditorStep = "general"; state.adminSourceEPGResult = null; state.adminSourceEPGError = ""; }
  if (action === "edit" && source) { state.adminSourceEditor = Object.assign({}, source, { password: "", accounts: items(source.accounts).map(function(account) { return Object.assign({}, account, { password: "" }); }) }); state.adminSourceEditorStep = "general"; state.adminSourceEPGResult = null; state.adminSourceEPGError = ""; }
  // Snapshot the draft so Escape can tell an untouched editor from one with edits.
  if (action === "add" || (action === "edit" && source)) state.adminSourceEditorBaseline = JSON.stringify(sourceEditorPayload("save"));
  if (action === "cancel") {
    state.adminSourceRequestID += 1;
    state.adminSourceBusy = "";
    state.adminSourceEditor = null;
    setAdminSourceMessage("", "");
    state.sourceManagerFocusNext = state.adminSourceEditorOpener || "";
    state.adminSourceEditorOpener = "";
  }
  if (action === "save") return submitAdminSource(sourceEditorPayload("save"), personal ? "Provider saved. Your channels and guide are refreshing." : "Source saved. Catalog refresh queued.");
  if (action === "test-editor") return submitAdminSource(sourceEditorPayload("test"), "Connection successful.");
  if (action === "test-epg") return submitAdminSource(sourceEditorPayload("test_epg"), "Alternate EPG is available.");
  if (action === "test" && source) return submitAdminSource(Object.assign({}, source, { action: "test", password: "" }), "Connection to " + sourceDisplayName(source) + " successful.");
  if (action === "toggle" && source) return submitAdminSource(Object.assign({}, source, { action: "save", enabled: !source.enabled, password: "" }), source.enabled ? sourceDisplayName(source) + " disabled." : sourceDisplayName(source) + " enabled. Refresh queued.");
  if (action === "delete" && source) {
    // Destructive: confirm inline first (openInlineConfirm in core.js).
    openInlineConfirm(trigger || document.querySelector("[data-source-action=\"delete\"][data-source-id=\"" + cssEscape(source.id) + "\"]"), {
      title: sourceDisplayName(source),
      message: personal ? "Your channels and guide from this provider will be removed." : "",
      onConfirm: function() {
        submitAdminSource({ action: "delete", id: source.id }, personal ? sourceDisplayName(source) + " deleted." : sourceDisplayName(source) + " deleted. Catalog refresh queued.");
      }
    });
    return;
  }
  rerenderSourceManager();
}
function renderAdminSettingsTab() {
  return ""
    + '<div class="settings-card"><div class="settings-card-head"><div><h2>Provider accounts</h2><p>Choose whether everyone shares the sources on the Sources tab, or each user brings their own.</p></div></div><label class="settings-row"><span><strong>Users add their own providers</strong><small>Each user enters their own Xtream Codes server and credentials in Settings and gets their own channels and guide. Users without a provider see nothing; shared sources are not used. Admins cannot view or use anyone&#39;s credentials.</small></span><input type="checkbox" data-admin-category-field="userSourcesEnabled"' + (adminSettings().userSourcesEnabled === true ? ' checked' : '') + '></label></div>'
    + '<div class="settings-card"><div class="settings-card-head"><div><h2>Playback privacy</h2><p>Choose whether streams that cannot be relayed may expose provider credentials.</p></div></div><label class="settings-row"><span><strong>Allow direct provider URLs</strong><small>TS live, VOD, series and catch-up can&#39;t be relayed through Silo, so playback sends the provider URL, including your account credentials, to the viewer&#39;s browser. Turn off to block those streams unless the provider serves HLS.</small></span><input type="checkbox" data-admin-category-field="allowDirectProviderURLs"' + (adminSettings().allowDirectProviderURLs !== false ? ' checked' : '') + '></label></div>'
    + '<div class="settings-card"><div class="settings-card-head"><div><h2>Sports</h2><p>Live scores and matched coverage. Replays come only from provider REPLAY channels.</p></div></div><label class="settings-row"><span><strong>Enable Sports</strong><small>Show Sports browsing and team follows.</small></span><input type="checkbox" data-admin-category-field="sportsEnabled"' + (sportsEnabled() ? ' checked' : '') + '></label></div>'
    + '<div class="settings-card">' + renderAdminSportsGuide(adminSettings()) + '</div>'
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
  const stations=document.createElement("div");stations.className="station-mappings";root.append(stations);
  loadPlayerLibrary("media.js","SiloLiveMedia").then(function(){if(stations.isConnected===false)return;SiloLiveMedia.stationEditor(stations,settings,items(state.app&&state.app.channels),function(rows){state.adminCategorySettings.stationMappings=rows;markAdminSettingsDraft();renderAdminTopbarActions();});}).catch(function(){stations.textContent="Station mapping controls could not be loaded. Reload to try again.";});

}
function renderAdminTimeShiftSettings() {
  const root = byId("admin-timeshift-settings");
  if (!root) return;
  const settings = adminSettings();
  const status = state.timeShiftAdminStatus || {};
  const direct = true;
  const usage = state.timeShiftAdminLoading ? "Loading cache usage..." : (status.unavailable ? "Cache status is unavailable." : (state.timeShiftAdminStatus ? byteSizeLabel(status.bytes) + " of " + byteSizeLabel(status.maxBytes) + " · " + String(status.activeBuffers || 0) + " buffered channels · " + String(status.activeLeases || 0) + " viewers" : "Usage has not been checked yet."));
  const windows = [15, 30, 60, 90, 120].map(function(minutes) { return "<option value=\"" + minutes + "\"" + (Number(settings.liveRewindWindowMinutes) === minutes ? " selected" : "") + ">" + minutes + " minutes</option>"; }).join("");
  root.innerHTML = adminSaveStatusHTML()
    + "<label class=\"settings-row compact-row\"><span><strong>Enable Live Rewind</strong><small>Buffer live channels for pause and rewind. Unsupported codecs use GPU conversion when available.</small></span><input type=\"checkbox\" data-admin-timeshift-field=\"enabled\"" + (settings.liveRewindEnabled ? " checked" : "") + (direct ? "" : " disabled") + "></label>"
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
    state.timeShiftAdminStatus = await getJSON("/dispatcharr/api/media/admin-status");
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
    await postJSON("/dispatcharr/api/media/clear", {});
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

function adminSportsGuidePolicy(settings) {
  const policy = settings.sportsGuide || {};
  return { enabled: policy.enabled !== false, groups: items(policy.groups).slice(), overrides: items(policy.overrides).slice() };
}
function renderAdminSportsGuide(settings) {
  const policy = adminSportsGuidePolicy(settings);
  const channels = items(state.app && state.app.channels);
  const groups = {};
  channels.forEach(function(ch) { if (ch.categoryId) groups[ch.categoryId] = ch.categoryName || ch.categoryId; });
  const leagueMap = {};
  items(state.sports && state.sports.leagues).forEach(function(league) { leagueMap[league.id] = league; });
  const events = items(state.sports && (state.sports.guideOptions || state.sports.events)).filter(function(event) { return String(event.id || "").indexOf("epg:") !== 0; });
  events.forEach(function(event) { if (event.leagueId && !leagueMap[event.leagueId]) leagueMap[event.leagueId] = { id: event.leagueId, name: event.leagueName || event.leagueId }; });
  const leagues = Object.keys(leagueMap).map(function(id) { return leagueMap[id]; });
  const option = function(value, label) { return "<option value=\"" + escapeHTML(value) + "\">" + escapeHTML(label) + "</option>"; };
  const channelOptions = option("", "Choose a channel") + channels.map(function(ch) { return option(ch.id, ch.name || ch.id); }).join("");
  const rules = policy.groups.map(function(rule, i) {
    return "<div class=\"profile-selection-row\"><span><strong>" + escapeHTML(groups[rule.groupId] || rule.groupId) + "</strong><small>" + escapeHTML((rule.leagueId || "Automatic league") + " · " + (rule.mode || "auto")) + "</small></span><button type=\"button\" data-admin-sports-action=\"guide-remove-group\" data-guide-index=\"" + i + "\" aria-label=\"Remove group hint\">" + icon("x") + "</button></div>";
  }).join("");
  const links = policy.overrides.map(function(link, i) {
    const channel = channels.find(function(ch) { return ch.id === link.channelId; });
    const event = events.find(function(event) { return event.id === link.eventId; });
    return "<div class=\"profile-selection-row\"><span><strong>" + escapeHTML((channel && channel.name) || link.channelId) + "</strong><small>" + escapeHTML(link.disabled ? "Automatic fill disabled" : ((event && event.name) || link.eventId)) + "</small></span><button type=\"button\" data-admin-sports-action=\"guide-remove-link\" data-guide-index=\"" + i + "\" aria-label=\"Remove event link\">" + icon("x") + "</button></div>";
  }).join("");
  return "<div class=\"settings-card-head sports-library-head\"><div><h3>Sports guide</h3><p>Fill gaps in the guide from confidently matched sports schedules. Provider listings stay in place. Generated windows are estimates.</p></div></div>"
    + "<label class=\"settings-row compact-row\"><span><strong>Fill missing sports listings</strong><small>Pregame, game, and postgame entries are cached for all guide views.</small></span><input type=\"checkbox\" data-admin-category-field=\"sportsGuideEnabled\"" + (policy.enabled ? " checked" : "") + "></label>"
    + "<h4>Group hints</h4><p class=\"settings-note\">Narrow a group to one league, use tournament/session matching, or turn filling off for that group.</p>"
    + (rules ? "<div class=\"profile-selection-list\">" + rules + "</div>" : "")
    + "<div class=\"sports-team-channel-form\"><label><span>Group</span><select id=\"admin-guide-group\">" + option("", "Choose a group") + Object.keys(groups).sort().map(function(id) { return option(id, groups[id]); }).join("") + "</select></label>"
    + "<label><span>League</span><select id=\"admin-guide-league\">" + option("", "Automatic league") + leagues.map(function(league) { return option(league.id, league.name || league.id); }).join("") + "</select></label>"
    + "<label><span>Matching</span><select id=\"admin-guide-mode\">" + option("auto", "Automatic") + option("matchup", "Two competitors") + option("session", "Tournament or session") + option("off", "Do not fill") + "</select></label><button type=\"button\" data-admin-sports-action=\"guide-add-group\">Set hint</button></div>"
    + "<h4>Manual event links</h4><p class=\"settings-note\">Link an ambiguous channel to a scheduled event. The link expires with that event; it never rolls forward to another game.</p>"
    + (links ? "<div class=\"profile-selection-list\">" + links + "</div>" : "")
    + "<div class=\"sports-team-channel-form\"><label><span>Channel</span><select id=\"admin-guide-channel\">" + channelOptions + "</select></label>"
    + "<label><span>Event</span><select id=\"admin-guide-event\">" + option("", "Choose an event") + option("__off", "Do not fill this channel") + events.map(function(event) { return option(event.id, (event.name || event.id) + " · " + new Date(Number(event.startUnix) * 1000).toLocaleString()); }).join("") + "</select></label><button type=\"button\" data-admin-sports-action=\"guide-add-link\">Set link</button></div>";
}
function updateAdminSportsGuide(action, target) {
  const settings = state.adminCategorySettings || defaultAdminCategorySettings();
  const policy = adminSportsGuidePolicy(settings);
  const value = function(id) { const input = byId(id); return input ? String(input.value || "") : ""; };
  if (action === "guide-add-group") {
    const groupId = value("admin-guide-group");
    if (!groupId) return;
    policy.groups = policy.groups.filter(function(rule) { return rule.groupId !== groupId; });
    policy.groups.push({ groupId: groupId, leagueId: value("admin-guide-league"), mode: value("admin-guide-mode") });
  } else if (action === "guide-add-link") {
    const channelId = value("admin-guide-channel"), eventId = value("admin-guide-event");
    if (!channelId || !eventId) return;
    policy.overrides = policy.overrides.filter(function(link) { return link.channelId !== channelId; });
    policy.overrides.push({ channelId: channelId, eventId: eventId === "__off" ? "" : eventId, disabled: eventId === "__off" });
  } else {
    const index = Number(target.getAttribute("data-guide-index"));
    const rows = action === "guide-remove-group" ? policy.groups : policy.overrides;
    if (!Number.isInteger(index) || index < 0 || index >= rows.length) return;
    rows.splice(index, 1);
  }
  settings.sportsGuide = policy;
  state.adminCategorySettings = settings;
  markAdminSettingsDraft();
  renderAdminPage();
}
