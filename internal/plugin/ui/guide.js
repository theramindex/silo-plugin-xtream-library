// Guide: the program grid, guide windows, EPG gaps, and program details.
function guideChannelMatchesQuery(channel) {
  if (!state.query || channelMatchesQuery(channel)) return true;
  return programsFor(channel.id).some(programMatchesQuery);
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
function navigateGuideCategory(id) {
  id = id || "";
  if (id !== state.category) state.folderQuery = "";
  state.category = id;
  state.view = id && nestedFolderChildren(id).length ? "live" : "guide";
  render();
  ensureGuideCoverageForView(state.view);
}
function guideProgramKey(program) {
  if (program && program.id !== undefined && program.id !== null && String(program.id)) return "id:" + String(program.id);
  return String(program && program.channelId || "") + "@" + Number(program && program.startUnix || 0) + "@" + String(program && program.title || "");
}
function mergeGuideWindowPrograms(existing, fresh, start, end) {
  const floor = Math.floor(Date.now() / 1000) - guideLookbehindSeconds - 3600;
  const merged = {};
  const order = [];
  const add = function(program) {
    const key = guideProgramKey(program);
    if (!merged[key]) order.push(key);
    merged[key] = program;
  };
  items(existing).forEach(function(program) {
    const programEnd = Number(program && program.endUnix || 0);
    if (programEnd && programEnd < floor) return;
    // Programs inside the refreshed window are replaced by the fresh copy.
    if (programOverlapsWindow(program, start, end)) return;
    add(program);
  });
  items(fresh).forEach(add);
  return order.map(function(key) { return merged[key]; });
}
function guideWindowIgnoredByBackend(programs, start, end) {
  return items(programs).some(function(program) {
    const programStart = Number(program && program.startUnix || 0);
    const programEnd = Number(program && program.endUnix || 0);
    return programStart >= end || (programEnd && programEnd <= start);
  });
}
async function fetchGuideWindow(start, end) {
  if (!state.app) return false;
  start = Math.floor(start);
  end = Math.floor(end);
  if (end <= start) return false;
  const payload = guideCoverage.full ? await getJSON("/dispatcharr/api/guide") : await getJSON("/dispatcharr/api/guide?start=" + start + "&end=" + end);
  if (!state.app || !payload) return false;
  const programs = items(payload.programs);
  if (guideCoverage.full || guideWindowIgnoredByBackend(programs, start, end)) {
    guideCoverage.full = true;
    state.app.programs = programs;
  } else {
    state.app.programs = mergeGuideWindowPrograms(state.app.programs, programs, start, end);
    guideCoverage.start = guideCoverage.start ? Math.min(guideCoverage.start, start) : start;
    guideCoverage.end = Math.max(guideCoverage.end, end);
  }
  rebuildProgramIndex();
  return true;
}
// Make sure programs up to `untilUnix` are loaded; resolves true when new data arrived.
function ensureGuideCoverage(untilUnix) {
  if (!state.app || guideCoverage.full || !guideCoverage.end) return Promise.resolve(false);
  untilUnix = Math.floor(Number(untilUnix || 0));
  if (untilUnix <= guideCoverage.end) return Promise.resolve(false);
  if (guideCoverage.pending) return guideCoverage.pending;
  guideCoverage.pending = fetchGuideWindow(guideCoverage.end, untilUnix).catch(function(error) {
    try { console.warn("Xtream guide window load failed", error); } catch (_) {}
    return false;
  }).finally(function() {
    guideCoverage.pending = null;
  });
  return guideCoverage.pending;
}
function ensureGuideCoverageForView(view) {
  let until = 0;
  if (view === "guide") until = guideWindow().end;
  else if (view === "search" || view === "mytv") until = Math.floor(Date.now() / 1000) + guideSearchLookaheadSeconds;
  if (!until) return;
  ensureGuideCoverage(until).then(function(loaded) {
    if (!loaded || state.view !== view) return;
    if (view === "guide") refreshVisibleGuideBlock();
    else if (view === "search") updateSearchPageResults();
    else if (view === "mytv") updateMyTVSearchSurface();
  });
}
function guideHasPrograms() {
  return items(state.app && state.app.programs).length > 0;
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
  if (!siloUserIsAdmin()) {
    showAppToast("Only Silo admins can force a guide refresh.");
    return;
  }
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
    if (!adminOnlyDenied(error, "force a guide refresh")) showAppToast("Dispatcharr refresh failed.");
  } finally {
    state.refreshing = false;
    setGuideRefreshButtonsLoading(false);
    refreshVisibleGuideBlock();
  }
}
function renderGuideChannelButton(channel) {
  const channelName = channel.name || "Untitled";
  return "<button class=\"epg-channel\" data-channel=\"" + escapeHTML(channel.id) + "\" data-channel-name=\"" + escapeHTML(channelName) + "\" aria-label=\"" + escapeHTML(channelName) + "\" title=\"" + escapeHTML(channelName) + "\">" + logoHTML(channel) + "<span class=\"epg-channel-title\">" + escapeHTML(channelName) + "</span></button>";
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
function guideUnavailableLabel() {
  return "No Guide Data Available";
}
function programIsGuidePlaceholder(program) {
  const title = normalizeProgramTitle(program && program.title);
  return /^(no games? today|data not available|no (guide )?(data|information|programming)( available)?|programming unavailable|information not available)$/.test(title);
}
function refreshGuideRowsForQuery() {
  if (state.view !== "guide" || !byId("epg")) return false;
  resetGuideRows();
  renderEPG();
  return true;
}
function guideFilterCategories() {
  const hidden = hiddenMap();
  const includeChannel = function(channel) { return !(channel.categoryId && hidden[channel.categoryId]); };
  const custom = customGroupCategories();
  if (!categoryParsing().enabled) return custom.concat(sourceCategoriesWithChannels(includeChannel));
  return custom
    .concat(featuredCategoriesFromPaths("", includeChannel, true))
    .concat(virtualCategoriesFromPaths("", includeChannel, true))
    .sort(compareCategoryDisplayName);
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
  const now = Date.now();
  const scopeKey = String(key || "guide");
  const missingChannels = items(channels).filter(function(channel) {
    if (channelHasNearGuide(channel)) return false;
    const attemptKey = scopeKey + ":" + String(channel && channel.id || "");
    return !state.guideWarmPings[attemptKey] || now - state.guideWarmPings[attemptKey] >= 5 * 60 * 1000;
  });
  if (!missingChannels.length) return;
  const channelIds = missingChannels.slice(0, guidePingChannelLimit).map(function(channel) { return channel && channel.id; }).filter(Boolean);
  if (!channelIds.length) return;
  channelIds.forEach(function(channelID) { state.guideWarmPings[scopeKey + ":" + channelID] = now; });
  postJSON("/dispatcharr/api/guide/ping", { channelIds: channelIds }).then(function() {
    return refreshStatusData().then(function() { return refreshSupplementalData(false); });
  }).then(function() {
    if (state.view === "guide") {
      resetGuideRows();
      renderEPG();
    } else {
      render();
    }
    setTimeout(function() { maybeWarmGuideForChannels(channels, key); }, 250);
  }).catch(function(error) {
    try { console.warn("Dispatcharr guide warm ping failed", error); } catch (_) {}
  });
}
function homeGuideChannels(watched, favorites) {
  const seen = {};
  const pool = [];
  items(watched).concat(items(favorites)).forEach(function(channel) {
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
	const canCatchup = !!(channel.catchup && Number(channel.catchupMinutes || 0) > 0 && end > start && end <= Math.floor(Date.now() / 1000));
  return "<div class=\"program-modal-backdrop\" data-program-modal-close=\"true\"></div><section class=\"program-modal\" role=\"dialog\" aria-modal=\"true\" aria-labelledby=\"program-modal-title\" aria-describedby=\"program-modal-description\">"
    + "<button class=\"program-modal-close\" type=\"button\" data-program-modal-close=\"true\" aria-label=\"Close\">" + icon("x") + "</button>"
    + "<div class=\"program-modal-head\"><div class=\"program-modal-art\">" + logoHTML(channel) + "</div><div class=\"program-modal-title\"><h2 id=\"program-modal-title\">" + escapeHTML(title) + "</h2><p>" + escapeHTML(channelName) + "</p><div class=\"program-modal-time\">" + icon("clock") + "<span>" + escapeHTML(timeText || (programIsLive(program) ? "Live now" : "Guide airing")) + "</span></div><div class=\"program-modal-tags\">" + tags.map(function(tag) { return "<span" + (tag === "Live now" ? " class=\"is-live\"" : "") + ">" + escapeHTML(tag) + "</span>"; }).join("") + "</div></div></div>"
    + "<div id=\"program-modal-description\" class=\"program-modal-body\">" + (description ? escapeHTML(description) : "No additional details are available for this airing.") + "</div>"
    + "<div class=\"program-modal-actions\"><button type=\"button\" data-search-airing=\"" + escapeHTML(title) + "\">" + icon("search") + "<span>More Airings</span></button><button type=\"button\" data-program-detail-watch=\"" + escapeHTML(channel.id) + "\">" + icon("play") + "<span>Watch Now</span></button>" + (canCatchup ? "<button type=\"button\" data-catchup-channel=\"" + escapeHTML(channel.id) + "\" data-catchup-start=\"" + escapeHTML(String(start)) + "\" data-catchup-end=\"" + escapeHTML(String(end)) + "\">" + icon("rewind") + "<span>Replay</span></button>" : "") + (canSchedule ? "<button type=\"button\" data-program-detail-schedule=\"" + escapeHTML(channel.id) + "\" data-program-detail-program=\"" + escapeHTML(program.id || "") + "\">" + icon("record") + "<span>Record</span></button>" : "") + "</div>"
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
function renderGuidePage() {
  clearGuideSearchTimer();
  const categories = guideFilterCategories();
  const slots = guideSlots();
  state.guideLastSlotStart = guideSlotStart();
  const searchHTML = '<div class="guide-search-wrap"><label class="guide-search-field"><span>' + icon("search") + '</span><input id="guide-search" class="search" placeholder="Search programs or channels" value="' + escapeHTML(state.query) + '" aria-label="Search programs or channels" aria-controls="guide-search-results" autocomplete="off"></label><section id="guide-search-results" class="guide-search-results" aria-label="Matching programs" hidden></section></div>';
  const actionsHTML = '<div class="guide-commandbar-actions"><button type="button" class="section-action" data-guide-now aria-keyshortcuts="N" title="Return to now (N). Use arrow keys to move through the guide; Enter opens a program.">Now</button>' + (siloUserIsAdmin() ? '<button type="button" class="section-action" data-guide-refresh="true">Refresh</button>' : '') + '</div>';
  byId("view").innerHTML = '<div class="guide-page"><div class="guide-commandbar"><div class="guide-commandbar-title"><strong>TV Guide</strong>' + guideFreshnessHTML() + '</div>' + renderGuideCategoryPicker(categories) + searchHTML + actionsHTML + '</div><div id="guide-scroll" class="guide-scroll"><div class="guide-timeline" style="' + guideTimelineStyle(slots) + '"><div class="time-head"><span>Today</span>' + slots.map(function(slot) { return "<span>" + escapeHTML(timeLabel(slot)) + "</span>"; }).join("") + '</div><div id="epg" class="guide-window-spacer" style="height:0px"><div class="guide-window" style="transform:translateY(0px)"></div></div></div></div></div>';
  const search = byId("guide-search");
  search.oninput = function(event) { if (!event.isComposing) scheduleGuideSearch(event.target); };
  search.oncompositionend = function(event) { scheduleGuideSearch(event.target); };
  const guideScroll = byId("guide-scroll");
  if (guideScroll) guideScroll.onscroll = scheduleGuideWindowRender;
  resetGuideRows();
  maybeWarmGuideForChannels(state.guideChannels.slice(0, guideWindowOverscan() * 2), "guide:" + (state.category || "all"));
  renderEPG();
  renderGuideProgramSearch();
}
function guideCategoryOptionHTML(category) {
  const selected = String((category && category.id) || "") === String(state.category || "");
  const fullName = String((category && (category.name || category.id)) || allGroupLabel());
  const parts = fullName.split(" / ").filter(Boolean);
  const label = parts.length ? parts[parts.length - 1] : fullName;
  const parent = parts.length > 1 ? parts.slice(0, -1).join(" / ") : "";
  return "<button type=\"button\" class=\"guide-category-option" + (selected ? " selected" : "") + "\" data-guide-category=\"" + escapeHTML((category && category.id) || "") + "\" role=\"option\" aria-selected=\"" + (selected ? "true" : "false") + "\"><span class=\"guide-category-check\">" + (selected ? icon("check") : "") + "</span><span><strong>" + escapeHTML(label) + "</strong>" + (parent ? "<small>" + escapeHTML(parent) + "</small>" : "") + "</span></button>";
}
function guideCategoryOptionsHTML(categories) {
  const query = lower(state.guideCategoryQuery).trim();
  const filtered = items(categories).filter(function(category) { return !query || lower(category.name || category.id).indexOf(query) !== -1; });
  const all = guideCategoryOptionHTML({ id: "", name: allGroupLabel() });
  return all + (filtered.length ? filtered.map(guideCategoryOptionHTML).join("") : "<div class=\"guide-category-empty\">No matching categories</div>");
}
function renderGuideCategoryPicker(categories) {
  const open = !!state.guideCategoryPickerOpen;
  return "<div class=\"guide-category-picker" + (open ? " open" : "") + "\"><button type=\"button\" class=\"guide-category-trigger\" data-guide-category-toggle=\"true\" aria-haspopup=\"listbox\" aria-expanded=\"" + (open ? "true" : "false") + "\"><span class=\"guide-category-trigger-copy\"><small>Category</small><strong>" + escapeHTML(guideCategoryInputValue(categories)) + "</strong></span><span class=\"guide-category-chevron\">" + icon("chevron-down") + "</span></button><div class=\"guide-category-popover\"><label class=\"guide-category-search\"><span>" + icon("search") + "</span><input id=\"guide-category-search\" value=\"" + escapeHTML(state.guideCategoryQuery) + "\" placeholder=\"Find a category\" autocomplete=\"off\"></label><div class=\"guide-category-options\" role=\"listbox\">" + guideCategoryOptionsHTML(categories) + "</div></div></div>";
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
  navigateGuideCategory(next);
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
    cells.push("<div class=\"epg-cell program" + (isLive ? " live" : "") + "\" style=\"" + epgCellStyle(start, end, windowInfo) + "\"><button class=\"epg-play\" data-guide-focus=\"program\" data-guide-start=\"" + start + "\" data-guide-end=\"" + end + "\" data-program-detail-channel=\"" + escapeHTML(channel.id) + "\" data-program-detail=\"" + escapeHTML(program.id || "") + "\" aria-label=\"" + escapeHTML(programTime + " " + accessibleTitle) + "\"><time>" + escapeHTML(programTime) + "</time><strong>" + escapeHTML(titleParts.title) + (titleParts.live ? "<span class=\"epg-live-marker\" aria-hidden=\"true\">" + escapeHTML(titleParts.marker) + "</span>" : "") + "</strong></button>" + (canSchedule ? "<button class=\"epg-schedule\" data-schedule-channel=\"" + escapeHTML(channel.id) + "\" data-schedule-program=\"" + escapeHTML(program.id || "") + "\" aria-label=\"Schedule recording\">" + icon("record") + "</button>" : "") + "</div>");
    cursor = end;
  });
  if (cursor < windowEnd) cells.push(renderEPGGapCell(channel, cursor, windowEnd, windowInfo));
  return cells.join("");
}
function renderEPGGapCell(channel, startUnix, endUnix, windowInfo) {
  if (endUnix <= startUnix) return "";
  const emptyTitle = guideUnavailableLabel();
  const emptyTime = timeLabel(startUnix);
  return "<button class=\"epg-cell program epg-gap\" data-guide-focus=\"gap\" data-guide-start=\"" + startUnix + "\" data-guide-end=\"" + endUnix + "\" data-channel=\"" + escapeHTML(channel.id) + "\" aria-label=\"" + escapeHTML(emptyTime + " " + emptyTitle) + "\" style=\"" + epgCellStyle(startUnix, endUnix, windowInfo) + "\"><time>" + escapeHTML(emptyTime) + "</time><strong>" + escapeHTML(emptyTitle) + "</strong></button>";
}
function renderEPGRow(channel, channelIndex) {
  return "<div class=\"epg-row\" data-guide-row=\"" + escapeHTML(channel.id) + "\" data-guide-row-index=\"" + channelIndex + "\">" + renderGuideChannelButton(channel).replace('class="epg-channel"', 'class="epg-channel" data-guide-focus="channel"') + "<div class=\"epg-programs\">" + renderEPGCells(channel, channelIndex) + "</div></div>";
}
function renderEPG() {
  renderGuideWindow(true);
}
function scheduleGuideWindowRender() {
  if (state.guideRenderFrame) return;
  state.guideRenderFrame = requestAnimationFrame(function() {
    state.guideRenderFrame = 0;
    renderGuideWindow(false);
    // Scrolling forward past loaded data pulls the next guide window.
    const scroll = byId("guide-scroll");
    if (scroll && typeof ensureGuideCoverageForView === "function" && scroll.scrollLeft + scroll.clientWidth * 2 >= scroll.scrollWidth) ensureGuideCoverageForView("guide");
  });
}
function guideVisibleRange(totalRows, scrollTop, viewportHeight, rowHeight, headerHeight) {
  if (totalRows <= 0) return { start: 0, end: 0 };
  const visibleRows = Math.max(1, Math.ceil(Math.max(0, viewportHeight) / rowHeight));
  const overscan = guideWindowOverscan();
  const rowsScrollTop = Math.max(0, scrollTop - headerHeight);
  const start = Math.min(Math.max(0, totalRows - visibleRows), Math.max(0, Math.floor(rowsScrollTop / rowHeight) - overscan));
  const end = Math.min(totalRows, start + Math.min(40, visibleRows + overscan * 2));
  return { start: start, end: end };
}
function renderGuideWindow(force) {
  if (state.view !== "guide" || state.guideLoading) return;
  const root = byId("epg");
  if (!root) return;
  const activeFocus = guideFocusInfo(document.activeElement);
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
  updateGuideTabStops();
  if (activeFocus) {
    const cell = Array.from(root.querySelectorAll("[data-guide-focus]")).find(function(element) { return guideFocusMatches(element, activeFocus); });
    if (cell) cell.focus({ preventScroll: true });
  }
  state.guideLoading = false;
  maybeWarmGuideForChannels(state.guideChannels.slice(start, end), "guide:" + (state.category || "all") + ":" + start + ":" + end);
}

function myTVGuidePrograms() {
  const passes = keywordPasses();
  const now = Math.floor(Date.now() / 1000);
  if (!passes.length) return [];
  return distinctOnLaterPrograms(programsFor("").filter(function(program) {
    if (programIsGuidePlaceholder(program) || Number(program.endUnix || 0) < now) return false;
    const text = lower(programSearchText(program));
    return passes.some(function(pass) { return text.indexOf(lower(pass.keyword)) !== -1; });
  })).sort(function(left, right) { return Number(left.startUnix || 0) - Number(right.startUnix || 0); });
}

function guideSearchPrograms(channels, programsByChannel, query, windowInfo, limit) {
  const needle = String(query || "").trim().toLocaleLowerCase();
  const result = { entries: [], total: 0 };
  if (needle.length < 2) return result;
  const maximum = Math.max(1, Math.min(20, Number(limit) || 20));
  const seen = new Set();
  items(channels).forEach(function(channel, channelIndex) {
    items(programsByChannel && programsByChannel[channel.id]).forEach(function(program) {
      const start = Number(program.startUnix) || windowInfo.start;
      const end = Number(program.endUnix) || start + 1800;
      if (end <= windowInfo.start || start >= windowInfo.end || programIsGuidePlaceholder(program)) return;
      if ([program.title, program.description].join(" ").toLocaleLowerCase().indexOf(needle) === -1) return;
      const key = JSON.stringify([channel.id, program.id || "", start, end]);
      if (seen.has(key)) return;
      seen.add(key);
      result.total++;
      result.entries.push({ channel: channel, program: program, start: start, channelIndex: channelIndex });
      result.entries.sort(function(a, b) {
        return Math.max(a.start, windowInfo.start) - Math.max(b.start, windowInfo.start) || a.channelIndex - b.channelIndex;
      });
      if (result.entries.length > maximum) result.entries.pop();
    });
  });
  return result;
}

function clearGuideSearchTimer() {
  if (state.guideSearchTimer) clearTimeout(state.guideSearchTimer);
  state.guideSearchTimer = 0;
  state.guideSearchGeneration = (state.guideSearchGeneration || 0) + 1;
}

function scheduleGuideSearch(input) {
  clearGuideSearchTimer();
  state.query = input.value;
  state.guideSearchDismissed = false;
  const generation = state.guideSearchGeneration;
  state.guideSearchTimer = setTimeout(function() {
    if (state.view !== "guide" || !input.isConnected || generation !== state.guideSearchGeneration) return;
    state.guideSearchTimer = 0;
    applyGuideSearch();
  }, 300);
}

function applyGuideSearch() {
  clearGuideSearchTimer();
  const scroll = byId("guide-scroll");
  if (scroll) { scroll.scrollTop = 0; scroll.scrollLeft = 0; }
  state.guideFocus = null;
  resetGuideRows();
  renderEPG();
  renderGuideProgramSearch();
}

function renderGuideProgramSearch() {
  const root = byId("guide-search-results");
  if (!root) return;
  const query = String(state.query || "").trim();
  root.hidden = state.guideSearchDismissed || query.length < 2;
  if (root.hidden) { root.innerHTML = ""; return; }
  if (root.contains(document.activeElement)) return;
  const matches = guideSearchPrograms(visibleChannels(true), state.programsByChannel, query, guideWindow(), 20);
  root.innerHTML = "<div class=\"guide-search-results-head\"><strong>Programs</strong><span role=\"status\">" + (matches.total > 20 ? "First 20 of " + matches.total : matches.total + " matches") + "</span></div>"
    + (matches.entries.length ? "<div class=\"guide-search-result-list\">" + matches.entries.map(function(entry) {
      const program = entry.program;
      return "<button type=\"button\" class=\"guide-search-result\" data-guide-search-channel=\"" + escapeHTML(entry.channel.id) + "\" data-guide-search-program=\"" + escapeHTML(program.id || "") + "\" data-guide-search-start=\"" + entry.start + "\" aria-label=\"" + escapeHTML("Show " + program.title + " on " + entry.channel.name + " at " + dateTimeLabel(entry.start) + " in guide") + "\"><strong>" + escapeHTML(program.title || guideUnavailableLabel()) + "</strong><span>" + escapeHTML(entry.channel.name || "Channel") + " · " + escapeHTML(dateTimeLabel(entry.start)) + "</span></button>";
    }).join("") + "</div>" : "<p class=\"guide-search-empty\">No matching programs in the next 25 hours. Channel matches appear in the guide below.</p>");
}

function dismissGuideProgramSearch() {
  state.guideSearchDismissed = true;
  const root = byId("guide-search-results");
  if (root) root.hidden = true;
}

function guideFocusInfo(element) {
  if (!element || !element.matches || !element.matches("[data-guide-focus]")) return null;
  const row = element.closest("[data-guide-row]");
  if (!row || !element.closest("#epg")) return null;
  return { channelID: row.getAttribute("data-guide-row"), kind: element.getAttribute("data-guide-focus"), programID: element.getAttribute("data-program-detail") || "", startUnix: Number(element.getAttribute("data-guide-start")) || guideWindow().start };
}

function guideFocusMatches(element, focus) {
  const info = guideFocusInfo(element);
  if (!info || !focus || info.channelID !== focus.channelID || info.kind !== focus.kind) return false;
  return info.kind === "channel" || (info.programID && info.programID === focus.programID) || info.startUnix === focus.startUnix;
}

function updateGuideTabStops(preferred) {
  const root = byId("epg");
  if (!root) return;
  const cells = Array.from(root.querySelectorAll("[data-guide-focus]"));
  const selected = preferred || cells.find(function(cell) { return guideFocusMatches(cell, state.guideFocus); }) || cells[0];
  cells.forEach(function(cell) { cell.tabIndex = cell === selected ? 0 : -1; });
}

function rememberGuideFocus(element) {
  const info = guideFocusInfo(element);
  if (!info) return;
  const old = state.guideFocus;
  info.anchorUnix = old && guideFocusMatches(element, old) ? old.anchorUnix : info.startUnix;
  state.guideFocus = info;
  updateGuideTabStops(element);
}

function guideCellIndexAtTime(cells, time) {
  let best = -1, distance = Infinity;
  cells.forEach(function(cell, index) {
    if (cell.kind === "channel") return;
    const start = Number(cell.start) || 0;
    const end = Number(cell.end) || start + 1800;
    const gap = time < start ? start - time : (time >= end ? time - end + 1 : 0);
    if (gap < distance) { best = index; distance = gap; }
  });
  return best < 0 ? 0 : best;
}

function guideRowFocusCells(row) {
  return row ? Array.from(row.querySelectorAll("[data-guide-focus]")) : [];
}

function guideCellForTime(row, time) {
  const cells = guideRowFocusCells(row);
  return cells[guideCellIndexAtTime(cells.map(function(cell) {
    return { kind: cell.getAttribute("data-guide-focus"), start: cell.getAttribute("data-guide-start"), end: cell.getAttribute("data-guide-end") };
  }), time)];
}

function focusGuideCell(element, anchorUnix) {
  const info = guideFocusInfo(element);
  if (!info) return false;
  info.anchorUnix = Number(anchorUnix) || info.startUnix;
  state.guideFocus = info;
  updateGuideTabStops(element);
  element.focus({ preventScroll: true });
  const scroll = byId("guide-scroll");
  if (scroll && info.kind !== "channel") {
    const bounds = scroll.getBoundingClientRect();
    const channel = element.closest("[data-guide-row]").querySelector(".epg-channel");
    const left = bounds.left + (channel ? channel.getBoundingClientRect().width : 0);
    const right = bounds.left + scroll.clientWidth;
    const rect = element.getBoundingClientRect();
    if (rect.left < left) scroll.scrollLeft += rect.left - left;
    else if (rect.right > right) scroll.scrollLeft += Math.min(rect.right - right, rect.left - left);
  }
  return true;
}

function focusGuideRow(index, anchorUnix, kind, edge, programID) {
  const channel = state.guideChannels[index];
  const scroll = byId("guide-scroll");
  if (!channel || !scroll) return false;
  const header = scroll.querySelector(".time-head");
  const headerHeight = header ? header.offsetHeight : 0;
  const rowHeight = guideRowHeight();
  const top = index * rowHeight;
  if (top < scroll.scrollTop) scroll.scrollTop = top;
  else if (top + rowHeight + headerHeight > scroll.scrollTop + scroll.clientHeight) scroll.scrollTop = Math.max(0, top + rowHeight + headerHeight - scroll.clientHeight);
  renderGuideWindow(true);
  const root = byId("epg");
  const row = Array.from(root.querySelectorAll("[data-guide-row]")).find(function(item) { return item.getAttribute("data-guide-row") === String(channel.id); });
  const cells = guideRowFocusCells(row);
  const match = programID ? cells.find(function(cell) { return cell.getAttribute("data-program-detail") === programID; }) : null;
  const cell = match || (edge === "last" ? cells[cells.length - 1] : (kind === "channel" || edge === "first" ? cells[0] : guideCellForTime(row, anchorUnix)));
  return focusGuideCell(cell, anchorUnix);
}

function jumpGuideToProgram(channelID, programID, startUnix) {
  clearGuideSearchTimer();
  state.query = "";
  const input = byId("guide-search");
  if (input) input.value = "";
  dismissGuideProgramSearch();
  resetGuideRows();
  const index = state.guideChannels.findIndex(function(channel) { return String(channel.id) === String(channelID); });
  if (index >= 0) focusGuideRow(index, startUnix, "program", "", programID);
}

function refreshGuideTimeline() {
  const scroll = byId("guide-scroll");
  if (!scroll) return;
  const slots = guideSlots();
  const timeline = scroll.querySelector(".guide-timeline");
  const header = scroll.querySelector(".time-head");
  if (timeline) timeline.setAttribute("style", guideTimelineStyle(slots));
  if (header) header.innerHTML = "<span>Today</span>" + slots.map(function(slot) { return "<span>" + escapeHTML(timeLabel(slot)) + "</span>"; }).join("");
  state.guideLastSlotStart = guideSlotStart();
}

function jumpGuideToNow() {
  dismissGuideProgramSearch();
  refreshGuideTimeline();
  const scroll = byId("guide-scroll");
  if (!scroll) return;
  scroll.scrollLeft = 0;
  const focused = state.guideFocus;
  const index = focused ? state.guideChannels.findIndex(function(channel) { return channel.id === focused.channelID; }) : -1;
  const visibleIndex = Math.min(state.guideChannels.length - 1, Math.floor(scroll.scrollTop / guideRowHeight()));
  focusGuideRow(index >= 0 ? index : Math.max(0, visibleIndex), Math.floor(Date.now() / 1000), "program");
}

function handleGuideKeyboard(event) {
  if (state.view !== "guide" || event.defaultPrevented || event.isComposing || state.programDetails) return false;
  const target = event.target;
  const input = byId("guide-search");
  if (target === input) {
    if (event.key === "Escape") { event.preventDefault(); applyGuideSearch(); dismissGuideProgramSearch(); return true; }
    if (event.key === "ArrowDown" || event.key === "Enter") {
      event.preventDefault();
      state.guideSearchDismissed = false;
      applyGuideSearch();
      const result = document.querySelector(".guide-search-result");
      if (result) result.focus();
      else if (state.guideChannels.length) focusGuideRow(0, guideWindow().start, "channel");
      return true;
    }
    return false;
  }
  if (target && target.matches && target.matches(".guide-search-result")) {
    if (event.key === "Escape") { event.preventDefault(); dismissGuideProgramSearch(); if (input) input.focus(); return true; }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const results = Array.from(document.querySelectorAll(".guide-search-result"));
      const index = results.indexOf(target) + (event.key === "ArrowDown" ? 1 : -1);
      if (index < 0 && input) input.focus();
      else if (results[index]) results[index].focus();
      return true;
    }
    return false;
  }
  const info = guideFocusInfo(target);
  if (!info || event.altKey || event.metaKey) return false;
  const row = target.closest("[data-guide-row]");
  const cells = guideRowFocusCells(row);
  const index = cells.indexOf(target);
  const rowIndex = Number(row.getAttribute("data-guide-row-index"));
  const anchor = state.guideFocus && state.guideFocus.anchorUnix || info.startUnix;
  const pageRows = Math.max(1, Math.floor(byId("guide-scroll").clientHeight / guideRowHeight()) - 1);
  if (["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "PageUp", "PageDown", "Home", "End", "n", "N"].indexOf(event.key) === -1) return false;
  event.preventDefault();
  if (event.key === "n" || event.key === "N") { jumpGuideToNow(); return true; }
  if ((event.key === "Home" || event.key === "End") && event.ctrlKey) {
    focusGuideRow(event.key === "Home" ? 0 : state.guideChannels.length - 1, anchor, info.kind, event.key === "Home" ? "first" : "last");
  } else if (event.key === "Home" || event.key === "End") {
    focusGuideCell(event.key === "Home" ? cells[0] : cells[cells.length - 1]);
  } else if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
    focusGuideCell(cells[index + (event.key === "ArrowRight" ? 1 : -1)]);
  } else {
    const direction = event.key === "ArrowUp" || event.key === "PageUp" ? -1 : 1;
    const step = event.key === "PageUp" || event.key === "PageDown" ? pageRows : 1;
    focusGuideRow(Math.max(0, Math.min(state.guideChannels.length - 1, rowIndex + direction * step)), anchor, info.kind);
  }
  return true;
}
