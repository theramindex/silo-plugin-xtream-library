// Sports: scores, leagues, team follows, matchup art, and replays.
function isWorldCupReplayGroup(value) {
  return normalizedGroupName(value) === "world cup replays";
}
function programLooksSports(program) {
  if (programIsGuidePlaceholder(program)) return false;
  return /\b(vs\.?|@|game|match|cup|league|racing|football|baseball|basketball|soccer|hockey|f1|formula|mlb|nba|nfl|nhl|mls|wnba|premier)\b/i.test(programSearchText(program));
}
function loadSports(force, preparedOnly) {
  if (!sportsEnabled()) return Promise.resolve({ events: [], leagues: [] });
  if (state.sportsLoading) return Promise.resolve(state.sports || { events: [], leagues: [] });
  if (state.sports && !force) return Promise.resolve(state.sports);
  if (force && !preparedOnly) stopSportsPoll(true);
  state.sportsLoading = true;
  // Forced upstream refresh is admin-only; regular users just re-read the prepared payload.
  return getJSONWithin("/dispatcharr/api/sports" + (force && !preparedOnly && siloUserIsAdmin() ? "?refresh=1" : ""), 12000, "Sports data took too long to respond. Try again.").then(function(payload) {
    // A partially built payload (incomplete: true) is still usable; render what
    // arrived and keep polling while the backend reports it is refreshing.
    state.sports = payload || { events: [], leagues: [] };
    state.sports.events = items(state.sports.events);
    state.sports.leagues = items(state.sports.leagues);
    dropPhantomSportsFollows();
    applySportsFavoritesToPayload();
    if (state.sportsLeague) loadSportsLeagueTeams(sportsLeagueByID(state.sports, state.sportsLeague));
    loadSportsReplays(force && !preparedOnly);
    if (state.sports.refreshing) scheduleSportsPoll();
    else stopSportsPoll(true);
    return state.sports;
  }).catch(function(error) {
    if (!state.sports) state.sports = { events: [], leagues: [], error: readableError(error) };
    else state.sports.error = readableError(error);
    showAppToast("Could not refresh sports.");
    return state.sports;
  }).finally(function() {
    state.sportsLoading = false;
    renderRail();
    if (state.view === "sports") renderSportsPage();
    if (state.view === "mytv") {
      updateMyTVSearchSurface();
      ensureMyTVTeamCatalog(state.myTVQuery);
    }
    if (state.view === "search") updateSearchPageResults();
    if (state.view === "player" && state.playerSportsOpen) renderPlayerSportsDrawer();
  });
}
function applySportsFavoritesToPayload() {
  items(state.sports && state.sports.events).forEach(function(event) {
    if (event.home) event.home.favorite = sportsFavoriteTeamMatches(event.home);
    if (event.away) event.away.favorite = sportsFavoriteTeamMatches(event.away);
  });
}
function sportsTabLabel(tab) {
  return ({ live: "Live", upcoming: "Upcoming", replays: "Replays", favorites: "Favorites", all: "All" })[tab] || "Live";
}
function sportsTabButtonsHTML() {
  const tabs = ["live", "upcoming"];
  tabs.push("replays");
  tabs.push("favorites", "all");
  return tabs.map(function(tab) {
    return "<button type=\"button\" data-sports-tab=\"" + tab + "\" class=\"" + (state.sportsTab === tab ? "active" : "") + "\" aria-pressed=\"" + (state.sportsTab === tab ? "true" : "false") + "\">" + escapeHTML(sportsTabLabel(tab)) + "</button>";
  }).join("");
}
function renderSportsTopbarTabs() {
  const root = byId("sports-topbar-tabs");
  if (!root) return;
  root.innerHTML = "";
}
function renderSportsTabFilters(payload) {
  const refreshing = state.sportsLoading || !!(payload && payload.refreshing) || state.sportsReplaysLoading;
  const refreshClass = "sports-refresh" + (refreshing ? " is-loading" : "");
  const refreshDisabled = refreshing ? " disabled aria-busy=\"true\"" : "";
  const replayStatus = sportsReplayStatusLabel();
  return "<div class=\"sports-filter-row\"><div class=\"view-toggle\" aria-label=\"Sports filter\">" + sportsTabButtonsHTML() + "</div>"
    + "<span class=\"sports-data-source\">Data by " + escapeHTML(sportsDataSourceLabel(payload)) + (replayStatus ? " <span class=\"sports-replay-status\">· " + escapeHTML(replayStatus) + "</span>" : "") + "</span>"
    + "<button type=\"button\" class=\"" + refreshClass + "\" data-sports-refresh=\"true\"" + refreshDisabled + ">" + icon("loader") + "<span>Refresh scores</span></button></div>";
}
function renderSportsPage() {
  const root = byId("view");
  if (state.sportsTab === "replays") {
    root.innerHTML = '<div class="sports-page sports-experience"><div class="sports-pinned">' + renderSportsTabFilters(state.sports) + '</div><div class="sports-score-scroll">' + renderSportsReplays() + '</div></div>';
    return;
  }
  const scrollTop = root ? root.scrollTop : 0;
  const sportsScroll = root && root.querySelector(".sports-score-scroll");
  const sportsScrollTop = sportsScroll ? sportsScroll.scrollTop : 0;
  if (!state.sports && !state.sportsLoading) loadSports(false);
  renderSportsTopbarTabs();
  const payload = state.sports || { events: [], leagues: [] };
  const events = filteredSportsEvents(payload);
  let selectedEvent = items(payload.events).find(function(event) { return sportsEventStateID(event) === state.sportsSelectedEventID; });
  const stats = sportsGameStatsState.id === state.sportsSelectedEventID ? sportsGameStatsState.data : null;
  if (selectedEvent && stats && stats.available && sportsEventHasScores(stats)) {
    selectedEvent = Object.assign({}, selectedEvent, { homeScore: stats.homeScore, awayScore: stats.awayScore, statusText: stats.statusText, live: stats.live, completed: stats.completed, status: stats.completed ? "completed" : (stats.live ? "live" : selectedEvent.status) });
  }
  if (state.sportsSelectedEventID && !selectedEvent) state.sportsSelectedEventID = "";
  const league = sportsLeagueByID(payload, state.sportsLeague);
  const status = recoveryPanelHTML(payload.error, "sports");
  let content = "";
  if (selectedEvent) content = renderSportsEventDetail(payload, selectedEvent);
  else if (league) content = renderSportsLeagueDetail(payload, league, events);
  else content = renderSportsBrowse(payload, events);
  root.innerHTML = "<div class=\"sports-page sports-experience\">" + status + content + "</div>";
  if (root) root.scrollTop = scrollTop;
  const updatedSportsScroll = root && root.querySelector(".sports-score-scroll");
  if (updatedSportsScroll) updatedSportsScroll.scrollTop = sportsScrollTop;
  syncSportsGameStats(selectedEvent);
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
  const sourceEvents = items(payload && payload.events).concat(state.sportsTab === "replays" ? items(state.sportsReplayStandaloneEvents) : []);
  return sourceEvents.filter(function(event) {
    const channelCount = uniqueEventChannels(event.channels).length;
    const replayCount = sportsReplayMatchesForEvent(event).length;
    if (!sportsEventHasPlayableAccess(event)) return false;
    if (state.sportsLeague && event.leagueId !== state.sportsLeague) return false;
    const onNow = sportsEventIsOnNow(event);
    if (state.sportsTab === "live" && (!onNow || !channelCount)) return false;
    const startUnix = Number(event.startUnix || 0);
    if (state.sportsTab === "upcoming" && (!channelCount || event.completed || onNow || (startUnix > 0 && startUnix < now - 3600))) return false;
    if (state.sportsTab === "replays" && !replayCount) return false;
    if (state.sportsTab === "favorites" && !sportsEventIsFollowed(event)) return false;
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
  const phase = sportsEventIsLive(event) ? " live" : (event.completed ? " final" : " upcoming");
  return "<article class=\"sports-card" + phase + "\">"
    + renderSportsMatchup(event, status)
    + renderSportsChannels(event)
    + renderSportsReplays(event)
    + "</article>";
}
function sportsStatusLabel(event) {
  if (sportsEventIsLive(event)) {
    const liveParts = [];
    if (event.statusText && lower(event.statusText) !== "live") liveParts.push(event.statusText);
    if (event.period) liveParts.push(event.period);
    if (event.clock) liveParts.push(event.clock);
    return liveParts.join(" · ") || "Live";
  }
  const status = lower(event && event.status);
  if (["airing", "replay", "highlights", "ended"].indexOf(status) !== -1) return event.statusText || ({ airing: "On now", replay: "Replay", highlights: "Highlights", ended: "Ended" })[status];
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
  const label = sportsTeamAbbreviation(team).slice(0, 3);
  const logo = sportsPreferredLogo(team && team.logoUrl, team && team.logoFallbackUrl);
  if (logo) return "<img class=\"" + className + "\" src=\"" + escapeHTML(logo) + "\" alt=\"\" loading=\"lazy\" data-img-error=\"sports-media\"><span class=\"" + className + " logo-fallback\" hidden>" + escapeHTML(label) + "</span>";
  return "<span class=\"" + className + " logo-fallback\">" + escapeHTML(label) + "</span>";
}
function renderSportsMatchup(event, status) {
  if (sportsEventIsRace(event)) return renderSportsRaceSummary(event, "sports-race-summary");
  const center = event.leagueName || event.leagueId || "Sports";
  const detail = status;
  const live = sportsEventIsLive(event);
  const showScore = sportsEventHasScores(event);
  const stateClass = live ? " live" : (event.completed ? " final" : " upcoming");
  return "<div class=\"sports-matchup\">" + renderSportsMatchTeam(event.away || {}, event.awayScore, showScore) + "<div class=\"sports-versus\"><strong>" + escapeHTML(center) + "</strong>" + (detail ? "<span class=\"sports-match-status" + stateClass + "\">" + escapeHTML(detail) + "</span>" : "") + "</div>" + renderSportsMatchTeam(event.home || {}, event.homeScore, showScore) + "</div>";
}
function renderSportsMatchTeam(team, score, showScore) {
  const name = team.name || team.abbreviation || "Team";
  const favorite = sportsFavoriteTeamMatches(team);
  const logo = renderSportsTeamLogo(team, "sports-match-team-logo");
  const favoriteControl = team.id ? "<button class=\"sports-team-favorite" + (favorite ? " active" : "") + "\" type=\"button\" data-sports-favorite-team=\"" + escapeHTML(team.id || "") + "\" data-sports-favorite-enabled=\"" + (favorite ? "false" : "true") + "\" aria-label=\"" + escapeHTML(favorite ? "Unfollow " + name : "Follow " + name) + "\" aria-pressed=\"" + (favorite ? "true" : "false") + "\">" + icon(favorite ? "heart-solid" : "heart") + "<span>" + (favorite ? "Following" : "Follow") + "</span></button>" : "<span class=\"sports-team-favorite placeholder\" aria-hidden=\"true\"></span>";
  const scoreHTML = showScore ? "<span class=\"sports-match-team-score\">" + escapeHTML(sportsScoresHidden(false) ? "–" : (score || "0")) + "</span>" : "";
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
function renderSportsChannelChip(channel, context) {
  const meta = channel.categoryName || channel.reason || "Live TV";
  const name = channel.name || "Channel";
  const isEventFooter = context === "event-footer";
  const className = "sports-channel" + (isEventFooter ? " event-channel-link" : "");
  const label = isEventFooter ? "<span class=\"event-channel-prefix\">Watch on </span>" + escapeHTML(name) : escapeHTML(name);
  return "<div class=\"sports-channel-wrap\"><button class=\"" + className + "\" type=\"button\" data-channel=\"" + escapeHTML(channel.id) + "\" title=\"" + escapeHTML(channel.reason || meta) + "\"><span class=\"sports-channel-logo\">" + logoHTML(channel) + "</span><span class=\"sports-channel-copy\"><strong data-overflow-tooltip=\"" + escapeHTML(name) + "\">" + label + "</strong><small>" + escapeHTML(meta) + "</small></span>" + (isEventFooter ? "<span class=\"event-channel-chevron\" aria-hidden=\"true\">" + icon("chevron-right") + "</span>" : "") + "</button></div>";
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
  const expanded = !state.sportsExpandedEvents[eventID];
  if (expanded) state.sportsExpandedEvents[eventID] = true;
  else delete state.sportsExpandedEvents[eventID];
  const controls = Array.from(document.querySelectorAll("[data-sports-expand-event]"));
  const control = controls.find(function(node) { return node.getAttribute("data-sports-expand-event") === eventID; });
  const tile = control && control.closest(".sports-event-tile");
  const availability = tile && tile.querySelector(".sports-event-availability");
  const tray = tile && tile.querySelector(".sports-event-channel-reveal");
  if (!control || !availability || !tray) {
    renderSportsPage();
    return;
  }
  control.setAttribute("aria-expanded", expanded ? "true" : "false");
  control.setAttribute("aria-label", (expanded ? "Hide" : "Show") + control.getAttribute("aria-label").replace(/^(Show|Hide)/, ""));
  availability.classList.toggle("expanded", expanded);
  tile.classList.toggle("channels-expanded", expanded);
  tray.classList.toggle("expanded", expanded);
  tray.setAttribute("aria-hidden", expanded ? "false" : "true");
  if (expanded) tray.removeAttribute("inert");
  else tray.setAttribute("inert", "");
}
function toggleSportsTeamFavorite(teamID, enabled) {
  teamID = String(teamID || "");
  if (!teamID) return;
  if (enabled) state.app.preferences.sportsFavoriteTeams[teamID] = true;
  else {
    const people = myTVBuiltInSportsPeople().concat(myTVSportsPeople());
    const team = people.find(function(person) { return sportsTeamIdentityIDs(person).indexOf(teamID) !== -1; });
    delete state.app.preferences.sportsFavoriteTeams[teamID];
    if (team) {
      const slug = sportsGamePassSlug(team.name);
      people.filter(function(person) { return sportsGamePassSlug(person.name) === slug; }).forEach(function(person) {
        sportsTeamIdentityIDs(person).forEach(function(id) { delete state.app.preferences.sportsFavoriteTeams[id]; });
      });
    }
  }
  applySportsFavoritesToPayload();
  savePrefs();
  if (state.view === "mytv") updateMyTVSearchSurface();
  else if (state.view === "search") updateSearchPageResults();
  else renderSportsPage();
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
  const poster = artwork ? "<div class=\"event-poster\"><img src=\"" + escapeHTML(artwork) + "\" alt=\"\" data-img-error=\"event-poster\"></div>" : "";
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
function renderBroadcastEventChannels(event) {
  const channels = uniqueEventChannels(event.channels);
  if (!channels.length) return "<div class=\"sports-channel-empty muted\">No matching channels.</div>";
  const expanded = !!state.expandedEvents[event.id];
  const visible = expanded ? channels : channels.slice(0, 3);
  const hiddenCount = channels.length - visible.length;
  const more = hiddenCount > 0 ? "<button class=\"sports-channel-more\" type=\"button\" data-event-expand=\"" + escapeHTML(event.id || "") + "\">+" + hiddenCount + " more</button>" : (expanded && channels.length > 3 ? "<button class=\"sports-channel-more\" type=\"button\" data-event-expand=\"" + escapeHTML(event.id || "") + "\">Show less</button>" : "");
  return "<div class=\"sports-channels\">" + visible.map(renderSportsChannelChip).join("") + more + "</div>";
}
function toggleBroadcastEventChannels(eventID) {
  eventID = String(eventID || "");
  if (!eventID) return;
  if (state.expandedEvents[eventID]) delete state.expandedEvents[eventID];
  else state.expandedEvents[eventID] = true;
  renderEventsPage();
}

function sportsEnabled() { return adminSettings().sportsEnabled === true; }

function sportsFirstPlayerActive() {
  return sportsFirstPlayerEnabled() && state.playerSportsMode === true;
}

function sportsFavoriteLeagueMap() { return prefs().sportsFavoriteLeagues || {}; }

function sportsPreferredChannelMap() { return prefs().sportsPreferredChannels || {}; }

function sportsPreferredNetworkMap() { return prefs().sportsPreferredNetworks || {}; }

function sportsScoresHidden(playerOnly) { return !!(prefs().sportsSpoilersHidden || (playerOnly && prefs().sportsPlayerSpoilersHidden)); }

function sportsNavAvailable() {
  return sportsEnabled();
}
function sportsNavHasMatches() {
  if (!sportsEnabled()) return false;
  if (!state.sports) return true;
  if (state.sportsLoading) return true;
  const sourceEvents = items(state.sports.events).concat(items(state.sportsReplayStandaloneEvents));
  return sourceEvents.some(sportsEventHasPlayableAccess);
}

function sportsGamePassSlug(value) {
  return lower(value).normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

function myTVBuiltInSportsPeople() {
  const catalogs = [
    { id: "mlb", name: "MLB", teams: [
      ["Arizona Diamondbacks", "ARI"], ["Athletics", "ATH"], ["Atlanta Braves", "ATL"], ["Baltimore Orioles", "BAL"], ["Boston Red Sox", "BOS"],
      ["Chicago Cubs", "CHC"], ["Chicago White Sox", "CWS"], ["Cincinnati Reds", "CIN"], ["Cleveland Guardians", "CLE"], ["Colorado Rockies", "COL"],
      ["Detroit Tigers", "DET"], ["Houston Astros", "HOU"], ["Kansas City Royals", "KC"], ["Los Angeles Angels", "LAA"], ["Los Angeles Dodgers", "LAD"],
      ["Miami Marlins", "MIA"], ["Milwaukee Brewers", "MIL"], ["Minnesota Twins", "MIN"], ["New York Mets", "NYM"], ["New York Yankees", "NYY"],
      ["Philadelphia Phillies", "PHI"], ["Pittsburgh Pirates", "PIT"], ["San Diego Padres", "SD"], ["San Francisco Giants", "SF"], ["Seattle Mariners", "SEA"],
      ["St. Louis Cardinals", "STL"], ["Tampa Bay Rays", "TB"], ["Texas Rangers", "TEX"], ["Toronto Blue Jays", "TOR"], ["Washington Nationals", "WSH"]
    ] },
    { id: "nfl", name: "NFL", teams: [
      ["Arizona Cardinals", "ARI"], ["Atlanta Falcons", "ATL"], ["Baltimore Ravens", "BAL"], ["Buffalo Bills", "BUF"], ["Carolina Panthers", "CAR"],
      ["Chicago Bears", "CHI"], ["Cincinnati Bengals", "CIN"], ["Cleveland Browns", "CLE"], ["Dallas Cowboys", "DAL"], ["Denver Broncos", "DEN"],
      ["Detroit Lions", "DET"], ["Green Bay Packers", "GB"], ["Houston Texans", "HOU"], ["Indianapolis Colts", "IND"], ["Jacksonville Jaguars", "JAX"],
      ["Kansas City Chiefs", "KC"], ["Las Vegas Raiders", "LV"], ["Los Angeles Chargers", "LAC"], ["Los Angeles Rams", "LAR"], ["Miami Dolphins", "MIA"],
      ["Minnesota Vikings", "MIN"], ["New England Patriots", "NE"], ["New Orleans Saints", "NO"], ["New York Giants", "NYG"], ["New York Jets", "NYJ"],
      ["Philadelphia Eagles", "PHI"], ["Pittsburgh Steelers", "PIT"], ["San Francisco 49ers", "SF"], ["Seattle Seahawks", "SEA"], ["Tampa Bay Buccaneers", "TB"],
      ["Tennessee Titans", "TEN"], ["Washington Commanders", "WAS"]
    ] },
    { id: "nba", name: "NBA", teams: [
      ["Atlanta Hawks", "ATL"], ["Boston Celtics", "BOS"], ["Brooklyn Nets", "BKN"], ["Charlotte Hornets", "CHA"], ["Chicago Bulls", "CHI"],
      ["Cleveland Cavaliers", "CLE"], ["Dallas Mavericks", "DAL"], ["Denver Nuggets", "DEN"], ["Detroit Pistons", "DET"], ["Golden State Warriors", "GSW"],
      ["Houston Rockets", "HOU"], ["Indiana Pacers", "IND"], ["LA Clippers", "LAC"], ["Los Angeles Lakers", "LAL"], ["Memphis Grizzlies", "MEM"],
      ["Miami Heat", "MIA"], ["Milwaukee Bucks", "MIL"], ["Minnesota Timberwolves", "MIN"], ["New Orleans Pelicans", "NOP"], ["New York Knicks", "NYK"],
      ["Oklahoma City Thunder", "OKC"], ["Orlando Magic", "ORL"], ["Philadelphia 76ers", "PHI"], ["Phoenix Suns", "PHX"], ["Portland Trail Blazers", "POR"],
      ["Sacramento Kings", "SAC"], ["San Antonio Spurs", "SAS"], ["Toronto Raptors", "TOR"], ["Utah Jazz", "UTA"], ["Washington Wizards", "WAS"]
    ] },
    { id: "nhl", name: "NHL", teams: [
      ["Anaheim Ducks", "ANA"], ["Boston Bruins", "BOS"], ["Buffalo Sabres", "BUF"], ["Calgary Flames", "CGY"], ["Carolina Hurricanes", "CAR"],
      ["Chicago Blackhawks", "CHI"], ["Colorado Avalanche", "COL"], ["Columbus Blue Jackets", "CBJ"], ["Dallas Stars", "DAL"], ["Detroit Red Wings", "DET"],
      ["Edmonton Oilers", "EDM"], ["Florida Panthers", "FLA"], ["Los Angeles Kings", "LAK"], ["Minnesota Wild", "MIN"], ["Montreal Canadiens", "MTL"],
      ["Nashville Predators", "NSH"], ["New Jersey Devils", "NJD"], ["New York Islanders", "NYI"], ["New York Rangers", "NYR"], ["Ottawa Senators", "OTT"],
      ["Philadelphia Flyers", "PHI"], ["Pittsburgh Penguins", "PIT"], ["San Jose Sharks", "SJS"], ["Seattle Kraken", "SEA"], ["St. Louis Blues", "STL"],
      ["Tampa Bay Lightning", "TBL"], ["Toronto Maple Leafs", "TOR"], ["Utah Mammoth", "UTA"], ["Vancouver Canucks", "VAN"], ["Vegas Golden Knights", "VGK"],
      ["Washington Capitals", "WSH"], ["Winnipeg Jets", "WPG"]
    ] },
    { id: "wnba", name: "WNBA", teams: [
      ["Atlanta Dream", "ATL"], ["Chicago Sky", "CHI"], ["Connecticut Sun", "CON"], ["Dallas Wings", "DAL"], ["Golden State Valkyries", "GSV"],
      ["Indiana Fever", "IND"], ["Las Vegas Aces", "LVA"], ["Los Angeles Sparks", "LAS"], ["Minnesota Lynx", "MIN"], ["New York Liberty", "NYL"],
      ["Phoenix Mercury", "PHX"], ["Portland Fire", "POR"], ["Seattle Storm", "SEA"], ["Toronto Tempo", "TOR"], ["Washington Mystics", "WAS"]
    ] },
    { id: "mls", name: "MLS", teams: [
      ["Atlanta United", "ATL"], ["Austin FC", "ATX"], ["CF Montréal", "MTL"], ["Charlotte FC", "CLT"], ["Chicago Fire FC", "CHI"],
      ["Colorado Rapids", "COL"], ["Columbus Crew", "CLB"], ["D.C. United", "DC"], ["FC Cincinnati", "CIN"], ["FC Dallas", "DAL"],
      ["Houston Dynamo FC", "HOU"], ["Inter Miami CF", "MIA"], ["LA Galaxy", "LAG"], ["Los Angeles FC", "LAFC"], ["Minnesota United FC", "MIN"],
      ["Nashville SC", "NSH"], ["New England Revolution", "NE"], ["New York City FC", "NYC"], ["New York Red Bulls", "RBNY"], ["Orlando City SC", "ORL"],
      ["Philadelphia Union", "PHI"], ["Portland Timbers", "POR"], ["Real Salt Lake", "RSL"], ["San Diego FC", "SD"], ["San Jose Earthquakes", "SJ"],
      ["Seattle Sounders FC", "SEA"], ["Sporting Kansas City", "SKC"], ["St. Louis CITY SC", "STL"], ["Toronto FC", "TOR"], ["Vancouver Whitecaps FC", "VAN"]
    ] }
  ];
  return catalogs.reduce(function(all, catalog) {
    return all.concat(catalog.teams.map(function(team) {
      const slug = sportsGamePassSlug(team[0]);
      return { id: "gamepass:" + catalog.id + ":" + slug, name: team[0], abbreviation: team[1], kind: "Team", leagueName: catalog.name, logoUrl: "https://game-thumbs.swvn.io/" + catalog.id + "/" + slug + "/teamlogo.png" };
    }));
  }, []);
}

// Providers occasionally emit team entities with an ID but no name; those are
// phantoms and must never become (or resolve) a follow.
function sportsTeamHasName(team) {
  return !!(team && String(team.name || team.displayName || "").trim());
}
// Every ID a stored follow may use for this team: current id, follow ids, and
// the legacy ids / aliases the backend reports after identity changes.
function sportsTeamIdentityIDs(team) {
  if (!team) return [];
  return uniqueIDs([team.id].concat(items(team.followIds), items(team.legacyIds), items(team.aliases)).filter(function(value) {
    return value !== null && value !== undefined && typeof value !== "object" && String(value).trim();
  }).map(String));
}
function sportsKnownTeamEntities() {
  const teams = [];
  items(state.sports && state.sports.events).forEach(function(event) {
    if (event && event.away) teams.push(event.away);
    if (event && event.home) teams.push(event.home);
  });
  Object.keys(state.sportsLeagueTeams || {}).forEach(function(leagueID) {
    items(state.sportsLeagueTeams[leagueID]).forEach(function(team) { if (team) teams.push(team); });
  });
  return teams;
}
// Drop follows whose ID only ever matches nameless phantom entities (and has
// no saved label). Runs once sports data and real prefs are both loaded.
function dropPhantomSportsFollows() {
  if (!state.app || !state.app.preferences || !prefsSync.loaded || !state.sports) return false;
  const favorites = state.app.preferences.sportsFavoriteTeams || {};
  const labels = state.app.preferences.sportsFavoriteTeamLabels || {};
  const named = {};
  const phantom = {};
  sportsKnownTeamEntities().forEach(function(team) {
    const target = sportsTeamHasName(team) ? named : phantom;
    sportsTeamIdentityIDs(team).forEach(function(id) { target[id] = true; });
  });
  const dropped = Object.keys(favorites).filter(function(id) {
    return !!favorites[id] && phantom[id] && !named[id] && !(labels[id] && String(labels[id].name || "").trim()) && String(id).indexOf("gamepass:") !== 0;
  });
  if (!dropped.length) return false;
  dropped.forEach(function(id) {
    delete favorites[id];
    delete labels[id];
  });
  savePrefs({ quiet: true });
  return true;
}
function sportsFavoriteTeamMatches(team) {
  const favorites = sportsFavoriteTeamMap();
  // Nameless teams are phantoms. Teams with an empty id contribute no ID
  // matches (sportsTeamIdentityIDs drops blanks), but EPG-derived teams
  // without ids still match saved game-pass follows by name below.
  if (!sportsTeamHasName(team)) return false;
  if (sportsTeamIdentityIDs(team).some(function(id) { return !!favorites[id]; })) return true;
  const slug = sportsGamePassSlug(team && (team.name || team.abbreviation));
  if (!slug) return false;
  return Object.keys(favorites).some(function(id) { return !!favorites[id] && String(id).indexOf("gamepass:") === 0 && String(id).endsWith(":" + slug); });
}

function myTVLeagueCatalogPriority(league, query) {
  const identity = lower([league && league.id, league && league.name, league && league.sportName].join(" "));
  const major = ["mlb", "nfl", "nba", "nhl", "wnba", "mls"];
  const direct = searchMatchScore(league && league.name, identity, query) > 0 ? 100 : 0;
  const majorIndex = major.findIndex(function(slug) { return identity.indexOf(slug) !== -1; });
  return direct + (majorIndex === -1 ? 0 : 50 - majorIndex);
}

function ensureMyTVTeamCatalog(query) {
  query = String(query || "").trim();
  if (query.length < 2 || !sportsEnabled()) return Promise.resolve([]);
  const leagues = items(state.sports && state.sports.leagues);
  if (!leagues.length) {
    if (!state.sportsLoading) loadSports(false);
    return Promise.resolve([]);
  }
  if (state.myTVTeamCatalogLoading) return state.myTVTeamCatalogLoading;
  const pending = leagues.filter(function(league) {
    return league && league.id && !Object.prototype.hasOwnProperty.call(state.sportsLeagueTeams, String(league.id));
  }).sort(function(left, right) {
    return myTVLeagueCatalogPriority(right, query) - myTVLeagueCatalogPriority(left, query);
  });
  if (!pending.length) return Promise.resolve([]);
  let next = 0;
  const worker = function() {
    if (next >= pending.length) return Promise.resolve();
    const league = pending[next++];
    return loadSportsLeagueTeams(league).then(function() {
      if (state.view === "mytv" && String(state.myTVQuery || "").trim().length >= 2) updateMyTVSearchSurface();
    }).then(worker);
  };
  const workerCount = Math.min(3, pending.length);
  state.myTVTeamCatalogLoading = Promise.all(Array.from({ length: workerCount }, worker)).finally(function() {
    state.myTVTeamCatalogLoading = null;
    if (state.view === "mytv") updateMyTVSearchSurface();
  });
  return state.myTVTeamCatalogLoading;
}

function myTVSportsPeople() {
  if (!sportsEnabled()) return [];
  const found = {};
  items(state.sports && state.sports.events).forEach(function(event) {
    const combat = /boxing|mma|combat|ufc|fight/.test(lower([event.sportName, event.leagueName, event.name].join(" ")));
    [event.away, event.home].forEach(function(team) {
      if (!team || !team.id || found[team.id] || !sportsTeamHasName(team)) return;
      found[team.id] = Object.assign({}, team, { kind: combat ? "Fighter" : "Team", leagueName: event.leagueName || "" });
    });
  });
  Object.keys(state.sportsLeagueTeams || {}).forEach(function(leagueID) {
    items(state.sportsLeagueTeams[leagueID]).forEach(function(team) {
      if (team && team.id && !found[team.id] && sportsTeamHasName(team)) found[team.id] = Object.assign({}, team, { kind: "Team", leagueName: (sportsLeagueByID(state.sports, leagueID) || {}).name || "" });
    });
  });
  const knownNames = {};
  Object.keys(found).forEach(function(id) { knownNames[sportsGamePassSlug(found[id] && found[id].name)] = true; });
  myTVBuiltInSportsPeople().forEach(function(team) {
    if (!knownNames[sportsGamePassSlug(team.name)]) found[team.id] = team;
  });
  return Object.keys(found).map(function(id) { return found[id]; });
}

function myTVSportsPassLabel(team) {
  if (lower(team && team.kind) === "fighter") return "Fight pass";
  const league = String(team && team.leagueName || "").trim();
  return (league ? league + " " : "") + "game pass";
}

function myTVFollowedLeagues() {
  const leagues = {};
  items(state.sports && state.sports.leagues).forEach(function(league) { if (league && league.id) leagues[league.id] = league; });
  return Object.keys(sportsFavoriteLeagueMap()).filter(function(id) { return !!sportsFavoriteLeagueMap()[id]; }).map(function(id) {
    return leagues[id] || { id: id, name: "Saved league" };
  });
}

function myTVFollowedSportsEvents() {
  const now = Math.floor(Date.now() / 1000);
  return items(state.sports && state.sports.events).filter(function(event) {
    return sportsEventIsFollowed(event) && (!event.completed || Number(event.endUnix || 0) >= now - 3 * 3600);
  }).sort(compareSportsEventsForTab).slice(0, 18);
}

function stopSportsPoll(resetAttempts) {
  if (state.sportsPollTimer) clearTimeout(state.sportsPollTimer);
  state.sportsPollTimer = null;
  if (resetAttempts) state.sportsPollAttempts = 0;
}

function scheduleSportsPoll() {
  if (state.sportsPollTimer || state.sportsPollAttempts >= 24) return;
  state.sportsPollAttempts += 1;
  state.sportsPollTimer = setTimeout(function() {
    state.sportsPollTimer = null;
    loadSports(true, true);
  }, 1500);
}

function loadSportsLeagueTeams(league) {
  if (!league || !league.id) return Promise.resolve([]);
  const leagueID = String(league.id);
  if (state.sportsLeagueTeams[leagueID]) {
    league.teams = state.sportsLeagueTeams[leagueID];
    return Promise.resolve(league.teams);
  }
  if (state.sportsLeagueTeamsLoading[leagueID]) return state.sportsLeagueTeamsLoading[leagueID];
  const query = "?league_id=" + encodeURIComponent(leagueID);
  const request = getJSONWithin("/dispatcharr/api/sports/league-teams" + query, 12000, "League teams took too long to respond.").then(function(payload) {
    const teams = items(payload && payload.teams);
    state.sportsLeagueTeams[leagueID] = teams;
    league.teams = teams;
    dropPhantomSportsFollows();
    return teams;
  }).catch(function() {
    return [];
  }).finally(function() {
    delete state.sportsLeagueTeamsLoading[leagueID];
    if (state.view === "sports" && state.sportsLeague === leagueID) renderSportsPage();
  });
  state.sportsLeagueTeamsLoading[leagueID] = request;
  return request;
}

function sportsDataSourceLabel(payload) {
  return lower(payload && payload.source) === "sportarr" ? "Sportarr" : (payload && payload.source || "sports provider");
}

function sportsHasGameStats(event) {
  return event && (event.leagueId === "college-football" || event.leagueId === "mlb" || /^(college football|ncaa football|mlb|major league baseball)$/i.test(event.leagueName || ""));
}

function syncSportsGameStats(event) {
  const id = sportsHasGameStats(event) ? sportsEventStateID(event) : "";
  if (id !== sportsGameStatsState.id) {
    clearTimeout(sportsGameStatsState.timer);
    Object.assign(sportsGameStatsState, { id: id, data: null, loading: false, fetchedAt: 0, timer: null });
  }
  if (!id || sportsGameStatsState.loading || Date.now() - sportsGameStatsState.fetchedAt < 30000) return;
  sportsGameStatsState.loading = true;
  getJSONWithin("/dispatcharr/api/sports?game_stats=" + encodeURIComponent(id), 12000, "Stats took too long to respond.").then(function(data) {
    if (sportsGameStatsState.id !== id) return;
    if (!data.available && sportsGameStatsState.data && sportsGameStatsState.data.available) sportsGameStatsState.data.message = data.message;
    else sportsGameStatsState.data = data;
  }).catch(function() {
    if (sportsGameStatsState.id !== id) return;
    sportsGameStatsState.data = Object.assign({}, sportsGameStatsState.data || { available: false }, { message: "Live stats are temporarily unavailable. Retrying shortly." });
  }).finally(function() {
    if (sportsGameStatsState.id !== id) return;
    sportsGameStatsState.loading = false;
    sportsGameStatsState.fetchedAt = Date.now();
    if (state.view === "sports" && state.sportsSelectedEventID === id) renderSportsPage();
    clearTimeout(sportsGameStatsState.timer);
    if (sportsGameStatsState.data && sportsGameStatsState.data.completed) return;
    sportsGameStatsState.timer = setTimeout(function refreshStats() {
      if (state.view !== "sports" || state.sportsSelectedEventID !== id) return;
      if (document.hidden) {
        sportsGameStatsState.timer = setTimeout(refreshStats, 30000);
        return;
      }
      renderSportsPage();
    }, 30000);
  });
}

function renderSportsGameStats(event) {
  if (!sportsHasGameStats(event) || sportsScoresHidden(false)) return "";
  const data = sportsGameStatsState.id === sportsEventStateID(event) ? sportsGameStatsState.data : null;
  if (!data || !data.available) return sportsSectionHTML("Game stats", "", "<p class=\"sports-stats-note\">" + escapeHTML(data ? data.message : "Loading live stats…") + "</p>", "sports-stats-section");
  const source = "<a class=\"sports-section-count\" title=\"" + (data.completed ? "Final game statistics" : "Refreshes every 30 seconds while this page is visible") + "\"" + externalLinkAttrs(data.sourceUrl) + ">ESPN · Updated " + escapeHTML(new Date(data.updatedAtUnix * 1000).toLocaleTimeString([], { hour: "numeric", minute: "2-digit", second: "2-digit" })) + "</a>";
  const body = (data.message ? "<p class=\"sports-stats-note\">" + escapeHTML(data.message) + " Showing the last successful update.</p>" : "")
    + renderSportsInnings(event, data)
    + (data.lastPlay ? "<p class=\"sports-stats-play\"><strong>Latest play</strong><span>" + escapeHTML(data.lastPlay) + "</span></p>" : "")
    + "<table class=\"sports-stats-table\"><thead><tr><th scope=\"col\">" + escapeHTML(sportsTeamName(event.away)) + "</th><th scope=\"col\">Team stats</th><th scope=\"col\">" + escapeHTML(sportsTeamName(event.home)) + "</th></tr></thead><tbody>"
    + items(data.rows).map(function(row) { return "<tr><td>" + escapeHTML(row.away) + "</td><th scope=\"row\">" + escapeHTML(row.label) + "</th><td>" + escapeHTML(row.home) + "</td></tr>"; }).join("") + "</tbody></table>";
  return sportsSectionHTML(data.completed ? "Final stats" : (data.live ? "Live stats" : "Game stats"), source, body, "sports-stats-section");
}

function renderSportsInnings(event, data) {
  const innings = items(data.innings);
  if (!innings.length) return "";
  const heads = innings.map(function(inning) { return '<th scope="col">' + escapeHTML(inning.number) + '</th>'; }).join('');
  const rows = ['away', 'home'].map(function(side) {
    const cells = innings.map(function(inning) { return '<td>' + escapeHTML(inning[side] === '' || inning[side] == null ? '–' : inning[side]) + '</td>'; }).join('');
    return '<tr><th scope="row">' + escapeHTML(sportsTeamName(event[side])) + '</th>' + cells + '<td class="sports-inning-total">' + escapeHTML(data[side + 'Score'] || '0') + '</td></tr>';
  }).join('');
  return '<div class="sports-linescore-scroll" role="region" aria-label="Inning scores" tabindex="0"><table class="sports-linescore"><thead><tr><th scope="col">Team</th>' + heads + '<th scope="col">R</th></tr></thead><tbody>' + rows + '</tbody></table></div>';
}

function sportsLeagueByID(payload, leagueID) {
  if (!leagueID) return null;
  return items(payload && payload.leagues).find(function(league) { return String(league.id || "") === String(leagueID); }) || null;
}

function sportsEventTitle(event) {
  if (event && event.leagueId === "lanka-premier-league" && event.away && event.away.name && event.home && event.home.name) {
    return sportsTeamName(event.away) + " vs " + sportsTeamName(event.home) + (event.round ? " · " + event.round : "");
  }
  if (event && (event.name || event.shortName)) {
    const title = String(event.name || event.shortName);
    if (/^Next Game:\s*/i.test(title)) {
      return title.replace(/^Next Game:\s*/i, "").replace(/\s+on\s+\d{4}-\d{2}-\d{2}\s+at\s+\d{1,2}:\d{2}\s*(?:AM|PM)\s*[A-Z]{2,5}\s*$/i, "");
    }
    return title;
  }
  return sportsTeamName(event && event.away) + " at " + sportsTeamName(event && event.home);
}

function sportsEventStateID(event) {
  return String(event && (event.stableId || event.id) || "");
}

function sportsSectionHTML(title, action, body, className) {
  if (!body) return "";
  return "<section class=\"sports-section " + escapeHTML(className || "") + "\"><div class=\"sports-section-head\"><h2>" + escapeHTML(title) + "</h2>" + (action || "") + "</div>" + body + "</section>";
}

function renderSportsBrowse(payload, events) {
  const featured = sportsFeaturedEvent(events);
  const topMatchups = sportsTopMatchups(events, featured);
  const topIDs = {};
  topMatchups.forEach(function(event) { topIDs[sportsEventStateID(event)] = true; });
  const remaining = events.filter(function(event) { return (!featured || sportsEventStateID(event) !== sportsEventStateID(featured)) && !topIDs[sportsEventStateID(event)]; });
  const loading = state.sportsLoading || !!(payload && payload.refreshing) || (state.sportsTab === "replays" && state.sportsReplaysLoading);
  const eventBody = remaining.length ? "<div class=\"sports-event-grid\">" + remaining.map(renderSportsEventTile).join("") + "</div>" : (!featured ? "<div class=\"empty\">" + (loading ? "Loading sports..." : "No sports matches.") + "</div>" : "");
  return "<div class=\"sports-pinned\">" + renderSportsTabFilters(payload) + "</div>"
    + "<div class=\"sports-score-scroll sports-browse\">"
    + (featured ? renderSportsFeature(featured) : "")
    + renderSportsTopMatchups(topMatchups)
    + renderSportsLeagueShelf(payload, events)
    + sportsSectionHTML(sportsTabLabel(state.sportsTab), "", eventBody, "sports-events-section")
    + "</div>";
}

function sportsEffectiveRanking(event) {
  const ranking = Object.assign({ score: 0, raw: 0, knee: 8, signals: [] }, event && event.ranking || {});
  const signals = items(ranking.signals).map(function(signal) { return Object.assign({}, signal); });
  let raw = Number(ranking.raw || 0);
  let score = Number(ranking.score || 0);
  if (sportsEventIsFollowed(event) && !signals.some(function(signal) { return signal.key === "favorite"; })) {
    signals.push({ key: "favorite", label: "You follow this", detail: "A followed team or league", points: 2 });
    raw += 2;
    score = Math.round((10 * Math.tanh(raw / Math.max(1, Number(ranking.knee || 8)))) * 10) / 10;
  }
  signals.sort(function(left, right) { return Number(right.points || 0) - Number(left.points || 0) || String(left.label || "").localeCompare(String(right.label || "")); });
  return { raw: raw, score: score, signals: signals };
}

function sportsTopMatchups(events, featured) {
  return items(events).filter(function(event) {
    return event && !event.replayOnly && !event.completed && (!featured || sportsEventStateID(event) !== sportsEventStateID(featured)) && sportsEffectiveRanking(event).score >= 4;
  }).sort(function(left, right) {
    return sportsEffectiveRanking(right).score - sportsEffectiveRanking(left).score || compareSportsEventsForTab(left, right);
  }).slice(0, 6);
}

function renderSportsTopMatchups(events) {
  if (!events.length) return "";
  const body = "<div class=\"sports-top-matchups\">" + events.map(function(event) {
    const ranking = sportsEffectiveRanking(event);
    const reasons = ranking.signals.slice(0, 2).map(function(signal) { return signal.label; }).join(" · ");
    return "<button type=\"button\" class=\"sports-top-matchup\" data-sports-open-event=\"" + escapeHTML(sportsEventStateID(event)) + "\">"
      + "<span><small>" + escapeHTML(event.leagueName || event.leagueId || "Sports") + "</small><strong>" + escapeHTML(sportsEventTitle(event)) + "</strong><em>" + escapeHTML(reasons || sportsStatusLabel(event)) + "</em></span>"
      + icon("chevron-right") + "</button>";
  }).join("") + "</div>";
  const explanation = "<span class=\"sports-ranking-help\"><button type=\"button\" class=\"sports-ranking-trigger\" aria-describedby=\"sports-ranking-tooltip\">Why these games?</button><span role=\"tooltip\" id=\"sports-ranking-tooltip\">Recommendations use start time, championship stage, rivalries, rankings, close scores, your followed teams and leagues, and available channels.</span></span>";
  return sportsSectionHTML("Top matchups", explanation, body, "sports-top-section");
}

function sportsFeaturedEvent(events) {
  const values = items(events);
  const live = values.find(sportsEventIsLive);
  if (live) return live;
  const now = Math.floor(Date.now() / 1000);
  const upcoming = values.filter(function(event) { return !event.completed && Number(event.startUnix || 0) >= now; }).sort(function(left, right) { return Number(left.startUnix || 0) - Number(right.startUnix || 0); });
  if (upcoming.length) return upcoming[0];
  return values.slice().sort(function(left, right) { return Number(right.startUnix || 0) - Number(left.startUnix || 0); })[0] || null;
}

function sportsEventHasScores(event) {
  return event && event.homeScore != null && String(event.homeScore) !== "" && event.awayScore != null && String(event.awayScore) !== "";
}

function sportsEventIsLive(event) {
  if (!event || !event.live || event.completed) return false;
  const status = lower(event.status);
  if (["airing", "replay", "highlights", "ended"].indexOf(status) !== -1) return false;
  const startUnix = Number(event.startUnix || 0);
  if (!startUnix) return true;
  return startUnix <= Math.floor(Date.now() / 1000) + 3 * 3600;
}

function sportsEventIsOnNow(event) {
  if (!event || !event.live || event.completed) return false;
  const startUnix = Number(event.startUnix || 0);
  if (!startUnix) return true;
  return startUnix <= Math.floor(Date.now() / 1000) + 3 * 3600;
}

function sportsEventIsRace(event) {
  return lower(event && event.eventType) === "race";
}

function sportsEventIsProgram(event) {
  return lower(event && event.eventType) === "event";
}

function renderSportsFeature(event) {
  const channels = uniqueEventChannels(event.channels);
  const art = sportsEventArtwork(event, "backdrop");
  const artDimensions = sportsArtworkDimensions(event, "backdrop", art);
  const live = sportsEventIsLive(event);
  const onNow = sportsEventIsOnNow(event);
  const watch = onNow && channels[0] ? "<button type=\"button\" class=\"sports-primary-action\" data-channel=\"" + escapeHTML(channels[0].id) + "\">" + icon("play") + "<span>" + (live ? "Watch live" : "Watch now") + "</span></button>" : "";
  return "<section class=\"sports-feature" + (art ? " has-art" : " no-art") + "\">"
    + (art ? "<img class=\"sports-feature-art\" src=\"" + escapeHTML(art) + "\" alt=\"\"" + artDimensions + ">" : "")
    + (art ? "" : "<div class=\"sports-feature-fallback\">" + renderSportsMatchupThumbnail(event) + "</div>")
    + "<div class=\"sports-feature-copy\"><span class=\"sports-eyebrow\">" + escapeHTML(live ? "Featured live event" : (onNow ? sportsStatusLabel(event) : "Next up")) + "</span>"
    + "<h1>" + escapeHTML(sportsEventTitle(event)) + "</h1>"
    + (!onNow && event.startUnix ? "<p class=\"sports-feature-schedule\">" + escapeHTML(sportsDateLabel(event.startUnix)) + "</p>" : "")
    + (art ? renderSportsFeatureScore(event) : "")
    + "<div class=\"sports-feature-actions\">" + watch + "<button type=\"button\" class=\"sports-secondary-action\" data-sports-open-event=\"" + escapeHTML(sportsEventStateID(event)) + "\">Event details" + icon("chevron-right") + "</button></div>"
    + "</div></section>";
}

function renderSportsFeatureScore(event) {
  if (sportsEventIsRace(event)) return renderSportsRaceSummary(event, "sports-feature-race");
  if (sportsEventIsProgram(event)) return "";
  const live = sportsEventIsLive(event);
  const showScore = sportsEventHasScores(event);
  return "<div class=\"sports-feature-score\">"
    + renderSportsFeatureTeam(event.away || {}, event.awayScore, showScore)
    + "<em>" + escapeHTML(event.leagueName || event.leagueId || "Sports") + "<small class=\"" + (live ? "live" : "") + "\">" + escapeHTML(sportsStatusLabel(event)) + "</small></em>"
    + renderSportsFeatureTeam(event.home || {}, event.homeScore, showScore)
    + "</div>";
}

function renderSportsFeatureTeam(team, score, showScore) {
  return "<span class=\"sports-feature-team\">" + renderSportsTeamLogo(team, "sports-feature-team-logo") + "<span class=\"sports-feature-team-copy\"><span class=\"sports-team-name-action\"><strong>" + escapeHTML(sportsTeamName(team)) + "</strong>" + sportsTeamFavoriteButton(team) + "</span>" + (showScore ? "<b>" + escapeHTML(sportsScoresHidden(false) ? "–" : (score || "0")) + "</b>" : "") + "</span></span>";
}

function renderSportsLeagueShelf(payload, events) {
  const visibleLeagueIDs = {};
  items(events).forEach(function(event) { if (event.leagueId) visibleLeagueIDs[String(event.leagueId)] = true; });
  const favoriteLeagues = sportsFavoriteLeagueMap();
  const leagues = items(payload && payload.leagues).filter(function(league) { return visibleLeagueIDs[String(league.id || "")]; }).sort(function(left, right) {
    const favoriteOrder = Number(!!favoriteLeagues[right.id]) - Number(!!favoriteLeagues[left.id]);
    return favoriteOrder || String(left.name || left.id || "").localeCompare(String(right.name || right.id || ""));
  });
  if (!leagues.length) return "";
  const body = "<div class=\"sports-league-grid\">" + leagues.map(function(league) {
    const visibleEvents = items(events).filter(function(event) { return String(event.leagueId || "") === String(league.id || ""); });
    const liveCount = visibleEvents.filter(sportsEventIsLive).length;
    const detail = [league.id === "sports" ? "League not identified" : league.sportName, liveCount ? liveCount + " live" : visibleEvents.length + " events"].filter(Boolean).join(" · ");
    return "<button type=\"button\" class=\"sports-league-card\" data-sports-open-league=\"" + escapeHTML(league.id || "") + "\">" + renderSportsLeagueMark(league) + "<span><strong>" + escapeHTML(league.name || league.id || "League") + "</strong><small>" + escapeHTML(detail) + "</small></span>" + icon("chevron-right") + "</button>";
  }).join("") + "</div>";
  return sportsSectionHTML("Browse leagues", "", body, "sports-league-section");
}

function renderSportsLeagueMark(league) {
  const name = league && (league.name || league.id) || "League";
  if (league && league.id === "sports") return "<span class=\"sports-league-mark\" aria-label=\"Other sports\">" + icon("tv") + "</span>";
  const logo = sportsPreferredLogo(league && league.logoUrl, league && league.logoFallbackUrl);
  const fallback = sportsLeagueFallbackMark(league);
  if (logo) return "<span class=\"sports-league-mark has-logo\"><img src=\"" + escapeHTML(logo) + "\" alt=\"\" loading=\"lazy\" data-img-error=\"sports-media\"><span hidden>" + escapeHTML(fallback) + "</span></span>";
  return "<span class=\"sports-league-mark\" aria-label=\"" + escapeHTML(name) + "\"><span>" + escapeHTML(fallback) + "</span></span>";
}

function sportsLeagueFallbackMark(league) {
  const id = lower(league && league.id);
  const name = String(league && (league.name || league.id) || "Sports").trim();
  const value = lower(name + " " + id);
  if (value.indexOf("college football") !== -1 || id === "college-football") return "CFB";
  if (value.indexOf("formula e") !== -1 || id === "formula-e") return "FE";
  const initials = name.split(/[^A-Za-z0-9]+/).filter(Boolean).map(function(word) { return word.charAt(0); }).join("").slice(0, 3);
  return (initials || name.slice(0, 3) || "SP").toUpperCase();
}

function renderSportsEventTile(event) {
  const live = sportsEventIsLive(event);
  const art = sportsEventArtwork(event, "backdrop");
  const channelsExpanded = sportsEventChannelsExpanded(event);
  const thumbnail = art ? renderSportsArtworkThumbnail(event, art) : renderSportsMatchupThumbnail(event);
  const fallbackCopy = art ? "" : "<span class=\"sports-event-meta\"><b>" + escapeHTML(event.leagueName || event.leagueId || "Sports") + "</b><small class=\"" + (live ? "live" : "") + "\">" + escapeHTML(sportsStatusLabel(event)) + "</small></span><span class=\"sports-event-title\">" + escapeHTML(sportsEventTitle(event)) + "</span>";
  return "<article class=\"sports-event-tile" + (live ? " live" : "") + (art ? " has-art" : " no-art") + (channelsExpanded ? " channels-expanded" : "") + "\"><button type=\"button\" class=\"sports-event-main\" data-sports-open-event=\"" + escapeHTML(sportsEventStateID(event)) + "\">"
    + thumbnail + fallbackCopy + "</button>"
    + (!sportsEventIsRace(event) && !sportsEventIsProgram(event) ? '<div class="sports-tile-team-favorites">' + sportsTeamFavoriteButton(event.away || {}) + sportsTeamFavoriteButton(event.home || {}) + '</div>' : '')
    + renderSportsTileAvailability(event) + "</article>";
}

function sportsTeamFavoriteButton(team) {
  if (!team || !team.id || !team.name) return "";
  const favorite = sportsFavoriteTeamMatches(team);
  const slug = sportsGamePassSlug(team.name);
  const known = myTVBuiltInSportsPeople().find(function(person) { return sportsGamePassSlug(person.name) === slug; });
  const favoriteID = sportsSavedTeamID(team, slug) || (known && known.id) || team.id;
  const label = (favorite ? "Unfollow " : "Follow ") + team.name;
  return '<button class="sports-team-favorite sports-team-heart' + (favorite ? ' active' : '') + '" type="button" data-sports-favorite-team="' + escapeHTML(favoriteID) + '" data-sports-favorite-enabled="' + !favorite + '" aria-label="' + escapeHTML(label) + '" title="' + escapeHTML(label) + '" aria-pressed="' + favorite + '">' + icon(favorite ? "heart-solid" : "heart") + '</button>';
}

function sportsSavedTeamID(team, slug) {
  const favorites = sportsFavoriteTeamMap();
  if (favorites[team.id]) return team.id;
  return Object.keys(favorites).find(function(id) { return favorites[id] && id.indexOf("gamepass:") === 0 && id.endsWith(":" + slug); });
}

function renderSportsMatchupThumbnail(event) {
  if (sportsEventIsRace(event)) return renderSportsRaceThumbnail(event);
  if (sportsEventIsProgram(event)) return renderSportsProgramThumbnail(event);
  const away = event.away || {};
  const home = event.home || {};
  const awayColor = safeSportsTeamColor(away.primaryColor || away.secondaryColor, "#262a32");
  const homeColor = safeSportsTeamColor(home.primaryColor || home.secondaryColor, "#30343c");
  const leagueLogo = sportsPreferredLogo(event.leagueLogoUrl, event.leagueLogoFallbackUrl);
  const showScore = sportsEventHasScores(event);
  const fieldArtwork = sportsFieldBackgroundURL(event);
  const center = leagueLogo ? "<img src=\"" + escapeHTML(leagueLogo) + "\" alt=\"\" data-img-error=\"sports-media\"><b hidden>VS</b>" : "<b>VS</b>";
  return "<span class=\"sports-matchup-thumb" + (fieldArtwork ? " sports-field-thumb" : "") + "\" aria-hidden=\"true\" style=\"--match-away:" + awayColor + ";--match-home:" + homeColor + "\">"
    + renderSportsBackground(event)
    + "<span class=\"sports-matchup-thumb-team away\">" + renderSportsTeamLogo(away, "sports-matchup-thumb-logo") + "<strong>" + escapeHTML(sportsTeamName(away)) + "</strong>" + (showScore ? "<em>" + escapeHTML(sportsScoresHidden(false) ? "–" : (event.awayScore || "0")) + "</em>" : "") + "</span>"
    + "<span class=\"sports-matchup-thumb-center\">" + center + "<small>vs</small></span>"
    + "<span class=\"sports-matchup-thumb-team home\">" + renderSportsTeamLogo(home, "sports-matchup-thumb-logo") + "<strong>" + escapeHTML(sportsTeamName(home)) + "</strong>" + (showScore ? "<em>" + escapeHTML(sportsScoresHidden(false) ? "–" : (event.homeScore || "0")) + "</em>" : "") + "</span>"
    + "</span>";
}

function renderSportsProgramThumbnail(event) {
  const logo = sportsPreferredLogo(event.leagueLogoUrl, event.leagueLogoFallbackUrl);
  const mark = logo ? "<img src=\"" + escapeHTML(logo) + "\" alt=\"\" data-img-error=\"sports-media\"><b hidden>" + escapeHTML(sportsLeagueFallbackMark({ id: event.leagueId, name: event.leagueName })) + "</b>" : icon("trophy");
  const bouts = event.leagueId === "boxing" ? sportsEventTitle(event).split(";").map(function(bout) { return bout.trim().replace(/^Boxeo de Primera\s*:\s*/i, ""); }) : [];
  if (bouts.length > 1) return "<span class=\"sports-matchup-thumb sports-program-thumb\" aria-hidden=\"true\">" + renderSportsBackground(event) + "<span class=\"sports-program-mark\">" + mark + "</span><span class=\"sports-program-copy\"><small>Boxing · " + bouts.length + " bouts</small>" + bouts.map(function(bout) { return "<strong>" + escapeHTML(bout) + "</strong>"; }).join("") + "</span></span>";
  return "<span class=\"sports-matchup-thumb sports-program-thumb\" aria-hidden=\"true\">"
    + renderSportsBackground(event)
    + "<span class=\"sports-program-mark\">" + mark + "</span>"
    + "<span class=\"sports-program-copy\"><small>" + escapeHTML(event.leagueName || event.sportName || "Sports") + "</small><strong>" + escapeHTML(sportsEventTitle(event)) + "</strong></span>"
    + "</span>";
}

function renderSportsArtworkThumbnail(event, art) {
  const live = sportsEventIsLive(event);
  const status = sportsStatusLabel(event);
  const identity = sportsEventIsRace(event) ? renderSportsArtworkRace(event) : (sportsEventIsProgram(event) ? renderSportsArtworkProgram(event) : renderSportsArtworkMatchup(event));
  const dimensions = sportsArtworkDimensions(event, "backdrop", art);
  return "<span class=\"sports-artwork-thumb\">"
    + "<img class=\"sports-artwork-thumb-bg\" src=\"" + escapeHTML(art) + "\" alt=\"\"" + dimensions + ">"
    + "<span class=\"sports-artwork-status" + (live ? " live" : "") + "\">" + escapeHTML(status) + "</span>"
    + identity
    + "<span class=\"sports-artwork-copy\"><small>" + escapeHTML(event.leagueName || event.leagueId || "Sports") + "</small><strong>" + escapeHTML(sportsEventTitle(event)) + "</strong></span>"
    + "</span>";
}

function renderSportsArtworkProgram(event) {
  const logo = sportsPreferredLogo(event.leagueLogoUrl, event.leagueLogoFallbackUrl);
  return "<span class=\"sports-artwork-program\" aria-hidden=\"true\">"
    + (logo ? "<img src=\"" + escapeHTML(logo) + "\" alt=\"\" data-img-error=\"sports-media\">" : icon("trophy"))
    + "<strong>" + escapeHTML(event.leagueName || event.sportName || "Sports") + "</strong></span>";
}

function renderSportsArtworkMatchup(event) {
  const away = event.away || {};
  const home = event.home || {};
  const leagueLogo = sportsPreferredLogo(event.leagueLogoUrl, event.leagueLogoFallbackUrl);
  const showScore = sportsEventHasScores(event);
  const center = leagueLogo ? "<img src=\"" + escapeHTML(leagueLogo) + "\" alt=\"\" data-img-error=\"sports-media\"><b hidden>VS</b>" : "<b>VS</b>";
  return "<span class=\"sports-artwork-matchup\" aria-hidden=\"true\">"
    + "<span class=\"sports-artwork-team\">" + renderSportsTeamLogo(away, "sports-artwork-team-logo") + "<strong>" + escapeHTML(sportsTeamName(away)) + "</strong>" + (showScore ? "<em>" + escapeHTML(sportsScoresHidden(false) ? "–" : (event.awayScore || "0")) + "</em>" : "") + "</span>"
    + "<span class=\"sports-artwork-center\">" + center + "</span>"
    + "<span class=\"sports-artwork-team\">" + renderSportsTeamLogo(home, "sports-artwork-team-logo") + "<strong>" + escapeHTML(sportsTeamName(home)) + "</strong>" + (showScore ? "<em>" + escapeHTML(sportsScoresHidden(false) ? "–" : (event.homeScore || "0")) + "</em>" : "") + "</span>"
    + "</span>";
}

function renderSportsArtworkRace(event) {
  const logo = sportsPreferredLogo(event.leagueLogoUrl, event.leagueLogoFallbackUrl);
  const series = sportsTeamName(event.away || {}) || event.leagueName || "Motorsport";
  const location = sportsTeamName(event.home || {}) || "Race";
  return "<span class=\"sports-artwork-race\" aria-hidden=\"true\">"
    + (logo ? "<img src=\"" + escapeHTML(logo) + "\" alt=\"\" data-img-error=\"sports-media\">" : "")
    + "<span><strong>" + escapeHTML(series) + "</strong><small>" + escapeHTML(location) + "</small></span></span>";
}

function renderSportsRaceThumbnail(event) {
  const series = sportsTeamName(event.away || {}) || event.leagueName || "Motorsport";
  const location = sportsTeamName(event.home || {}) || "Race";
  const raceLabel = event.sportName || "Motorsport";
  const logo = sportsPreferredLogo(event.leagueLogoUrl, event.leagueLogoFallbackUrl);
  const mark = logo ? "<img src=\"" + escapeHTML(logo) + "\" alt=\"\" data-img-error=\"sports-media\"><b hidden>" + escapeHTML(sportsLeagueFallbackMark({ id: event.leagueId, name: event.leagueName })) + "</b>" : "<b>" + escapeHTML(sportsLeagueFallbackMark({ id: event.leagueId, name: event.leagueName })) + "</b>";
  return "<span class=\"sports-matchup-thumb sports-race-thumb\" aria-hidden=\"true\">"
    + renderSportsBackground(event)
    + "<span class=\"sports-race-mark\">" + mark + "</span>"
    + "<span class=\"sports-race-copy\"><small>" + escapeHTML(raceLabel) + "</small><strong>" + escapeHTML(series) + "</strong><span class=\"sports-race-location\"><small>Race</small><b>" + escapeHTML(location) + "</b></span></span>"
    + "</span>";
}

function renderSportsRaceSummary(event, className) {
  const series = sportsTeamName(event.away || {}) || event.leagueName || "Motorsport";
  const location = sportsTeamName(event.home || {}) || "Race";
  return "<div class=\"" + escapeHTML(className || "sports-race-summary") + "\"><span><small>Series</small><strong>" + escapeHTML(series) + "</strong></span><em>" + escapeHTML(sportsStatusLabel(event)) + "</em><span><small>Location</small><strong>" + escapeHTML(location) + "</strong></span></div>";
}

function renderSportsTileTeam(team, score, showScore) {
  return "<span class=\"sports-event-team\">" + renderSportsTeamLogo(team || {}, "sports-event-team-logo") + "<strong>" + escapeHTML(sportsTeamName(team)) + "</strong>" + (showScore ? "<b>" + escapeHTML(sportsScoresHidden(false) ? "–" : (score || "0")) + "</b>" : "") + "</span>";
}

function sportsEventChannelsExpanded(event, channels) {
  const availableChannels = channels || uniqueEventChannels(event.channels);
  return availableChannels.length > 1 && !!state.sportsExpandedEvents[event.id];
}

function renderSportsTileAvailability(event) {
  const channels = uniqueEventChannels(event.channels);
  const replays = sportsReplayMatchesForEvent(event);
  const title = sportsEventTitle(event);
  if (channels.length === 1) {
    const channel = channels[0];
    const channelName = channel.name || "Live channel";
    return "<button type=\"button\" class=\"sports-event-availability sports-event-direct\" data-channel=\"" + escapeHTML(channel.id || "") + "\" aria-label=\"" + escapeHTML("Watch " + title + " on " + channelName) + "\"><span>Watch on " + escapeHTML(channelName) + "</span>" + icon("chevron-right") + "</button>";
  }
  const parts = [];
  if (channels.length) parts.push(channels.length + (sportsEventIsLive(event) ? " live " + (channels.length === 1 ? "broadcast" : "broadcasts") : " matched " + (channels.length === 1 ? "channel" : "channels")));
  if (replays.length) parts.push(replays.length + " " + (replays.length === 1 ? "replay" : "replays"));
  const expanded = sportsEventChannelsExpanded(event, channels);
  const trayID = sportsEventChannelTrayID(event);
  const control = channels.length > 1 ? "<button type=\"button\" data-sports-expand-event=\"" + escapeHTML(event.id || "") + "\" aria-expanded=\"" + (expanded ? "true" : "false") + "\" aria-controls=\"" + escapeHTML(trayID) + "\" aria-label=\"" + escapeHTML((expanded ? "Hide" : "Show") + " channels for " + title) + "\">" + icon("chevron-right") + "</button>" : "<button type=\"button\" data-sports-open-event=\"" + escapeHTML(sportsEventStateID(event)) + "\" aria-label=\"Open " + escapeHTML(title) + "\">" + icon("chevron-right") + "</button>";
  const tray = channels.length > 1 ? "<div class=\"sports-event-channel-reveal" + (expanded ? " expanded" : "") + "\" id=\"" + escapeHTML(trayID) + "\" aria-hidden=\"" + (expanded ? "false" : "true") + "\"" + (expanded ? "" : " inert") + "><div><div class=\"sports-event-channel-list\">" + channels.map(renderSportsChannelChip).join("") + "</div></div></div>" : "";
  return "<div class=\"sports-event-availability" + (expanded ? " expanded" : "") + "\"><span>" + escapeHTML(parts.join(" · ") || (sportsEventIsRankedPlaceholder(event) ? "Broadcast not matched yet" : "Coverage unavailable")) + "</span>" + control + "</div>" + tray;
}

function sportsEventChannelTrayID(event) {
  const eventID = String(event && event.id || "event").replace(/[^a-z0-9_-]+/gi, "-");
  return "sports-event-channels-" + eventID;
}

function sportsLeagueEvents(payload, leagueID) {
  return items(payload && payload.events).filter(function(event) { return String(event.leagueId || "") === String(leagueID || ""); });
}

function sportsLeagueTeams(events, roster) {
  const seen = {};
  const teams = [];
  const add = function(team) {
    if (!team) return;
    const key = String(team.name || team.id || team.abbreviation || "").trim().toLowerCase();
    if (!key) return;
    if (seen[key] !== undefined) {
      teams[seen[key]] = Object.assign({}, teams[seen[key]], team);
      return;
    }
    seen[key] = teams.length;
    teams.push(team);
  };
  items(roster).forEach(add);
  items(events).forEach(function(event) {
    [event.away, event.home].forEach(add);
  });
  return teams.sort(function(left, right) { return sportsTeamName(left).localeCompare(sportsTeamName(right)); });
}

function renderSportsLeagueDetail(payload, league, events) {
  const leagueEvents = sportsLeagueEvents(payload, league.id);
  const teams = sportsLeagueTeams(leagueEvents, league.teams);
  const replayCount = leagueEvents.reduce(function(total, event) { return total + sportsReplayMatchesForEvent(event).length; }, 0);
  const teamBody = teams.length ? "<div class=\"sports-team-rail\">" + teams.map(renderSportsTeamShelfCard).join("") + "</div>" : "";
  const eventBody = events.length ? "<div class=\"sports-event-grid\">" + events.map(renderSportsEventTile).join("") + "</div>" : "<div class=\"empty\">No " + escapeHTML(sportsTabLabel(state.sportsTab).toLowerCase()) + " events in this league.</div>";
  const leagueSummary = [league.sportName, teams.length + " teams", leagueEvents.length + " events", replayCount ? replayCount + " replays" : ""].filter(Boolean).join(" · ");
  return "<div class=\"sports-pinned sports-detail-toolbar\"><button type=\"button\" class=\"sports-back\" data-sports-back=\"browse\">" + icon("arrow-left") + "<span>All sports</span></button>" + renderSportsTabFilters(payload) + "</div>"
    + "<div class=\"sports-score-scroll sports-league-detail\"><header class=\"sports-league-hero\">" + renderSportsLeagueMark(league) + "<div><span class=\"sports-eyebrow\">League</span><h1>" + escapeHTML(league.name || league.id || "League") + "</h1><p>" + escapeHTML(leagueSummary) + "</p>" + (league.description ? "<small>" + escapeHTML(league.description) + "</small>" : "") + "</div></header>"
    + sportsSectionHTML("Teams", "", teamBody, "sports-team-section")
    + sportsSectionHTML(sportsTabLabel(state.sportsTab) + " events", "", eventBody, "sports-events-section") + "</div>";
}

function renderSportsTeamShelfCard(team) {
  const name = sportsTeamName(team);
  const favorite = sportsFavoriteTeamMatches(team);
  return "<article class=\"sports-team-shelf-card\"><span class=\"sports-team-shelf-mark\">" + renderSportsTeamLogo(team, "sports-team-shelf-logo") + "</span><span class=\"sports-team-shelf-copy\"><strong>" + escapeHTML(name) + "</strong><small>" + (favorite ? "Following" : "Team") + "</small></span>" + (team.id ? "<button type=\"button\" class=\"sports-team-favorite" + (favorite ? " active" : "") + "\" data-sports-favorite-team=\"" + escapeHTML(team.id) + "\" data-sports-favorite-enabled=\"" + (favorite ? "false" : "true") + "\" aria-label=\"" + escapeHTML(favorite ? "Unfollow " + name : "Follow " + name) + "\" aria-pressed=\"" + (favorite ? "true" : "false") + "\">" + icon(favorite ? "heart-solid" : "heart") + "</button>" : "") + "</article>";
}



function renderSportsDetailScore(event) {
  if (sportsEventIsRace(event)) return renderSportsRaceSummary(event, "sports-detail-race");
  const live = sportsEventIsLive(event);
  const showScore = sportsEventHasScores(event);
  const phase = live ? "Live" : (event.completed ? "Final" : (event.startUnix ? sportsDateLabel(event.startUnix) : "Time TBD"));
  if (sportsEventIsProgram(event)) return "<div class=\"sports-detail-program\"><strong>" + escapeHTML(sportsStatusLabel(event)) + "</strong><span>" + escapeHTML(phase) + "</span></div>";
  const stats = sportsGameStatsState.id === sportsEventStateID(event) ? sportsGameStatsState.data : null;
  const possession = stats && stats.live && !stats.completed && !stats.message && !sportsScoresHidden(false) ? stats.possession : "";
  const position = possession && stats.fieldPosition ? "<p class=\"sports-field-position\">" + escapeHTML(sportsTeamName(event[possession])) + " ball · " + escapeHTML(stats.fieldPosition) + "</p>" : "";
  return "<div class=\"sports-detail-score\">" + renderSportsDetailTeam(event.away || {}, event.awayScore, showScore, possession === "away") + "<div class=\"sports-detail-status\"><strong>" + escapeHTML(sportsStatusLabel(event)) + "</strong><span>" + escapeHTML(phase) + "</span></div>" + renderSportsDetailTeam(event.home || {}, event.homeScore, showScore, possession === "home") + "</div>" + position;
}

function renderSportsDetailTeam(team, score, showScore, possession) {
  const accent = safeSportsTeamColor(team && team.primaryColor);
  return "<div class=\"sports-detail-team\"" + (accent ? " style=\"--sports-team-accent:" + escapeHTML(accent) + "\"" : "") + ">" + renderSportsTeamLogo(team, "sports-detail-team-logo") + "<span><span class=\"sports-team-name-action\"><strong>" + escapeHTML(sportsTeamName(team)) + "</strong>" + sportsTeamFavoriteButton(team) + "</span>" + (showScore ? "<b>" + escapeHTML(sportsScoresHidden(false) ? "–" : (score || "0")) + "</b>" : "") + "<small class=\"sports-possession\">" + (possession ? "Possession" : "") + "</small></span></div>";
}

function sportsDetailLeagueLabel(event) {
  const label = String(event.leagueName || event.leagueId || "").trim();
  return /^(sports|other sports|unknown)$/i.test(label) ? "" : label;
}

function renderSportsEventNavigation(payload, event) {
  const tab = state.sportsTab || "live";
  const route = function(league) { return appRouteHash({view: "sports", sportsTab: tab, sportsLeague: league || ""}); };
  const crumbs = ['<li><a href="#/sports/all">Sports</a></li>', '<li><a href="' + escapeHTML(route("")) + '">' + escapeHTML(sportsTabLabel(tab)) + '</a></li>'];
  const leagueLabel = sportsDetailLeagueLabel(event);
  if (leagueLabel && event.leagueId) crumbs.push('<li><a href="' + escapeHTML(route(event.leagueId)) + '">' + escapeHTML(leagueLabel) + '</a></li>');
  const current = event.away && event.away.name && event.home && event.home.name ? sportsTeamName(event.away) + " vs " + sportsTeamName(event.home) : sportsEventTitle(event);
  crumbs.push('<li class="sports-breadcrumb-current"><span aria-current="page" title="' + escapeHTML(current) + '">' + escapeHTML(current) + '</span></li>');
  return '<nav class="sports-breadcrumbs" aria-label="Breadcrumb"><ol>' + crumbs.join("") + '</ol></nav>';
}

function sportsBroadcastTraits(channel, event) {
  const text = lower([channel.name, channel.categoryName, channel.reason].join(" "));
  const away = lower(sportsTeamName(event.away));
  const home = lower(sportsTeamName(event.home));
  const traits = [];
  if (away && text.indexOf(away) !== -1) traits.push("Likely away");
  if (home && text.indexOf(home) !== -1) traits.push("Likely home");
  if (/\b(local|regional)\b/.test(text)) traits.push("Likely local");
  if (/\b(espn|fox|cbs|nbc|abc|tnt|national|league pass)\b/.test(text)) traits.push("Major network");
  if (/\b(alt|alternate|extra|plus|multiview)\b/.test(text)) traits.push("Alternate");
  if (/\b(spanish|español|latino|deportes)\b/.test(text)) traits.push("Spanish");
  else if (/\b(french|français)\b/.test(text)) traits.push("French");
  return traits.length ? traits : ["Live"];
}

function sportsBroadcastNetworkKey(channel) {
  const name = lower(channel && channel.name).split(/[|·]/)[0]
    .replace(/\([^)]*\)|\[[^\]]*\]/g, " ")
    .replace(/\b(?:uhd|fhd|hd|4k|local|home|away|national|alternate|alt|spanish|espanol|french)\b/g, " ")
    .replace(/\s+\d+\s*$/g, " ")
    .replace(/[^a-z0-9]+/g, " ")
    .trim();
  return name;
}

function sportsBroadcastGroup(channel) {
  if (channel.sportsPreferred || channel.sportsPreferredNetwork) return "Preferred";
  const traits = channel.sportsTraits || [];
  for (const label of ["Likely local", "Likely home", "Likely away", "Major network", "Alternate", "Spanish", "French"]) {
    if (traits.indexOf(label) >= 0) return label;
  }
  return "Other feeds";
}

function renderSportsBroadcastGroups(channels, event, compact) {
  if (compact) return '<div class="sports-broadcast-grid">' + channels.map(function(channel) { return renderSportsBroadcastCard(channel, event, true); }).join('') + '</div>';
  const groups = {};
  channels.forEach(function(channel) {
    const label = sportsBroadcastGroup(channel);
    if (!groups[label]) groups[label] = [];
    groups[label].push(channel);
  });
  const groupOrder = ["Preferred", "Likely local", "Likely home", "Likely away", "Major network", "Alternate", "Spanish", "French", "Other feeds"];
  const visibleGroups = groupOrder.filter(function(label) { return groups[label] && groups[label].length; });
  return "<div class=\"sports-broadcast-groups\">" + visibleGroups.map(function(label) {
    return '<section class="sports-broadcast-group">' + (visibleGroups.length > 1 ? '<h3>' + escapeHTML(label) + '</h3>' : '') + '<div class="sports-broadcast-grid">' + groups[label].map(function(channel) { return renderSportsBroadcastCard(channel, event); }).join("") + "</div></section>";
  }).join("") + "</div>";
}

function renderSportsBroadcastCard(channel, event, compact) {
  const appChannel = channelByID(channel.id) || channel;
  const current = currentProgram(appChannel) || {};
  const next = nextProgram(appChannel) || {};
  const traits = compact ? (channel.sportsPreferred ? ["Preferred"] : []) : (channel.sportsPreferred ? ["Preferred"] : (channel.sportsPreferredNetwork ? ["Preferred network"] : [])).concat(channel.sportsTraits || []);
  const guide = compact ? [] : [current.title ? "Now: " + current.title : "", next.title ? "Next: " + next.title : ""].filter(Boolean);
  return "<article class=\"sports-broadcast-card" + (channel.sportsPreferred ? " preferred" : "") + (compact ? " compact" : "") + "\"><button type=\"button\" class=\"sports-broadcast-play\" data-channel=\"" + escapeHTML(channel.id || "") + "\"><span class=\"sports-channel-logo\">" + logoHTML(channel) + "</span><span class=\"sports-broadcast-copy\"><span class=\"sports-broadcast-badges\">" + traits.map(function(trait) { return "<small>" + escapeHTML(trait) + "</small>"; }).join("") + "</span><strong>" + escapeHTML(channel.name || "Channel") + "</strong>" + guide.map(function(line) { return "<em>" + escapeHTML(line) + "</em>"; }).join("") + "</span>" + icon("play") + "</button><button type=\"button\" class=\"sports-broadcast-preference\" data-sports-preferred-channel=\"" + escapeHTML(channel.id || "") + "\" data-sports-preferred-league=\"" + escapeHTML(event.leagueId || "") + "\" aria-label=\"" + escapeHTML((channel.sportsPreferred ? "Remove preferred" : "Prefer") + " " + (channel.name || "channel")) + "\" aria-pressed=\"" + (channel.sportsPreferred ? "true" : "false") + "\">" + icon(channel.sportsPreferred ? "heart-solid" : "heart") + "</button></article>";
}

function sportsTypedArtwork(event, kind) {
  const typed = event && event.artwork || {};
  const ordered = kind === "poster"
    ? [typed.poster, typed.thumbnail, typed.backdrop, typed.banner]
    : [typed.backdrop, typed.thumbnail, typed.banner, typed.poster];
  for (let index = 0; index < ordered.length; index += 1) {
    const value = ordered[index];
    const url = safeSportsMediaURL(value && typeof value === "object" ? value.url : value);
    if (url) return { url: url, width: Number(value && value.width) || 0, height: Number(value && value.height) || 0 };
  }
  return null;
}

function sportsArtworkDimensions(event, kind, url) {
  const typed = sportsTypedArtwork(event, kind);
  return typed && typed.url === url && typed.width > 0 && typed.height > 0
    ? " width=\"" + escapeHTML(typed.width) + "\" height=\"" + escapeHTML(typed.height) + "\""
    : "";
}





function sportsEventHasPlayableAccess(event) {
  return uniqueEventChannels(event && event.channels).length > 0 || sportsReplayMatchesForEvent(event).length > 0;
}

function sportsEventIsRankedPlaceholder(event, now) {
  if (!event || event.completed || event.replayOnly || sportsEventHasPlayableAccess(event)) return false;
  const startUnix = Number(event.startUnix || 0);
  const clock = Number(now || Math.floor(Date.now() / 1000));
  if (!startUnix || startUnix < clock - 3600 || startUnix > clock + 48 * 3600) return false;
  return sportsEffectiveRanking(event).score >= 4;
}

function sportsEventIsFollowed(event) {
  return !!(sportsFavoriteTeamMatches(event.home || {}) || sportsFavoriteTeamMatches(event.away || {}) || sportsFavoriteLeagueMap()[event.leagueId]);
}

function sportsMediaFailed(value) {
  const logo = safeSportsMediaURL(value);
  return !!(logo && state.sportsFailedMedia[logo]);
}

function sportsPreferredLogo(value, fallbackValue) {
  const logo = safeSportsMediaURL(value);
  const fallback = safeSportsMediaURL(fallbackValue);
  if (logo && fallback && logo !== fallback) {
    if (!state.sportsLogoFallbacks) state.sportsLogoFallbacks = {};
    state.sportsLogoFallbacks[logo] = fallback;
  }
  if (logo && !sportsMediaFailed(logo)) return logo;
  return fallback && !sportsMediaFailed(fallback) ? fallback : "";
}

function sportsFieldBackgroundKind(event) {
  const normalize = function(value) { return String(value || "").toLowerCase().replace(/[^a-z0-9]+/g, " ").trim(); };
  const sport = normalize(event && event.sportName);
  const league = normalize([event && event.leagueId, event && event.leagueName].join(" "));
  // Check the supplied sport first, then league aliases. Bare "football" is
  // ambiguous until the league is known; specific codes take precedence.
  const rules = [
    ["table-tennis", /\b(table tennis|ping pong|wtt)\b/],
    ["field-hockey", /\b(field hockey|fih)\b/],
    ["baseball", /\b(baseball|softball|mlb|milb|kbo|npb|wbsc)\b/],
    ["basketball", /\b(basketball|nba|wnba|ncaab|fiba|euroleague|eurocup)\b/],
    ["cricket", /\b(cricket|icc|ipl|indian premier league|big bash|caribbean premier league|pakistan super league|australian rules|australian football|afl)\b/],
    ["rugby", /\b(rugby|nrl|six nations|united rugby championship)\b/],
    ["football", /\b(american football|canadian football|college football|nfl|ncaaf|cfl|xfl|ufl|fbs|fcs)\b/],
    ["hockey", /\b(ice hockey|hockey|nhl|ahl|echl|ohl|whl|qmjhl|khl|shl|liiga|ice skating|figure skating|speed skating)\b/],
    ["tennis", /\b(tennis|atp|wta|itf|davis cup|billie jean king cup)\b/],
    ["golf", /\b(golf|pga|lpga|dp world tour|liv golf|ryder cup)\b/],
    ["volleyball", /\b(volleyball|fivb|vnl)\b/],
    ["badminton", /\b(badminton|bwf)\b/],
    ["motorsport", /\b(motorsport|motor sport|motor racing|auto racing|automobilism|formula 1|formula one|formula e|f1|nascar|indycar|moto ?gp|motorcycle racing|superbike|wsbk|imsa|wec|wrc|dtm)\b/],
    ["mma", /\b(mma|mixed martial arts|ufc|bellator|pfl|one championship)\b/],
    ["boxing", /\b(boxing|kickboxing|muay thai|wwe|aew|professional wrestling|pro wrestling)\b/],
    ["swimming", /\b(swimming|aquatics|water polo|diving)\b/],
    ["cycling", /\b(cycling|bmx|uci|tour de france|giro d italia|vuelta)\b/],
    ["skiing", /\b(skiing|ski|snowboard|snowboarding|biathlon|bobsleigh|bobsled|luge|skeleton|winter sports)\b/],
    ["athletics", /\b(athletics|track and field|track field|cross country|marathon|running|diamond league)\b/],
    ["equestrian", /\b(equestrian|horse racing|horse riding|show jumping|dressage|kentucky derby|belmont stakes)\b/],
    ["darts", /\b(darts|pdc)\b/],
    ["snooker", /\b(snooker|billiards|pool|wst)\b/],
    ["soccer", /\b(soccer|association football|fifa|uefa|concacaf|conmebol|epl|mls|nwsl|laliga|la liga|bundesliga|serie a|serie b|ligue 1|ligue 2|premier league|english league|english league championship|efl|eredivisie|liga mx|usl|champions league|europa league)\b/]
  ];
  for (const value of [sport, league]) {
    const rule = rules.find(function(entry) { return entry[1].test(value); });
    if (rule) return rule[0];
  }
  // Some EPG entries only classify the sport in their title.
  if (/\b(soccer|nwsl)\b/.test(normalize(event && event.name))) return "soccer";
  return sport === "football" ? "soccer" : "";
}

function sportsFieldBackgroundURL(event) {
  // Decorative sport atmosphere, not the current venue or a live camera.
  // Fixed CDN photos; source and license details are in ui/ARTWORK.md.
  const photos = {
    "baseball": "photo-1431817986760-7cc7fbb937b2",
    "football": "photo-1575317988650-5faf389cabef",
    "basketball": "photo-1574907060871-4555aa8aca75",
    "soccer": "photo-1767729790212-661953ecaa90",
    "hockey": "photo-1711413236898-d2fef57ad3a3",
    "tennis": "photo-1499510318569-1a3d67dc3976",
    "cricket": "photo-1512719994953-eabf50895df7",
    "rugby": "photo-1529663297269-6d349ec39b57",
    "golf": "photo-1538648759472-7251f7cb2c2f",
    "motorsport": "photo-1467277378664-19a209d31b02",
    "swimming": "photo-1576610616656-d3aa5d1f4534",
    "volleyball": "photo-1479859546309-cd77fa21c8f6",
    "table-tennis": "photo-1511067007398-7e4b90cfa4bc",
    "badminton": "photo-1547934045-2942d193cb49",
    "athletics": "photo-1489976908522-aabacf277f49",
    "skiing": "photo-1546180043-e1475c173021",
    "cycling": "photo-1600403477955-2b8c2cfab221",
    "field-hockey": "photo-1723272156032-1eac173d47b5",
    "boxing": "photo-1575747515871-2e323827539e",
    "mma": "photo-1738982510191-ab5c4965f8b3",
    "darts": "photo-1579019163248-e7761241d85a",
    "snooker": "photo-1760903192559-17dc111d31e3",
    "equestrian": "flagged/photo-1569319388901-605a6d2d1299"
};
  const photo = photos[sportsFieldBackgroundKind(event)];
  if (!photo) return "";
  const url = "https://images.unsplash.com/" + photo + "?auto=format&fit=crop&w=1200&q=80";
  return sportsMediaFailed(url) ? "" : url;
}

function renderSportsBackground(event) {
  const photo = sportsFieldBackgroundURL(event);
  if (!photo) return sportsGeneratedBackground(event);
  const fallback = safeSportsMediaURL(event && event.gameThumbsBackgroundUrl);
  return '<img class="sports-field-bg" src="' + escapeHTML(photo) + '" data-sports-background-fallback="' + escapeHTML(fallback) + '" alt="" loading="lazy" data-img-load="generated-art" data-img-error="sports-bg">';
}

function sportsGeneratedBackground(event) {
  const background = safeSportsMediaURL(event && event.gameThumbsBackgroundUrl);
  if (!background || sportsMediaFailed(background)) return "";
  return "<img class=\"sports-generated-bg\" src=\"" + escapeHTML(background) + "\" alt=\"\" loading=\"lazy\" data-img-load=\"generated-art\" data-img-error=\"sports-bg\">";
}

function toggleSportsLeagueFavorite(leagueID, enabled) {
  leagueID = String(leagueID || "");
  if (!leagueID) return;
  if (enabled) state.app.preferences.sportsFavoriteLeagues[leagueID] = true;
  else delete state.app.preferences.sportsFavoriteLeagues[leagueID];
  savePrefs();
  if (state.view === "mytv") updateMyTVSearchSurface();
  else if (state.view === "search") updateSearchPageResults();
  else renderSportsPage();
}

function toggleSportsSpoilers(scope) {
  if (scope === "player" && state.app.preferences.sportsSpoilersHidden) {
    state.app.preferences.sportsSpoilersHidden = false;
    state.app.preferences.sportsPlayerSpoilersHidden = false;
  } else {
    const key = scope === "player" ? "sportsPlayerSpoilersHidden" : "sportsSpoilersHidden";
    state.app.preferences[key] = !state.app.preferences[key];
  }
  savePrefs({ quiet: true });
  if (state.view === "player") renderPlayerSportsDrawer();
  else renderSportsPage();
}





function markSportsMediaFailed(image) {
  const logo = safeSportsMediaURL(image && image.getAttribute("src"));
  if (logo) state.sportsFailedMedia[logo] = true;
  if (!image) return;
  const fallback = state.sportsLogoFallbacks && state.sportsLogoFallbacks[logo];
  if (fallback && !sportsMediaFailed(fallback)) {
    image.setAttribute("src", fallback);
    return;
  }
  image.hidden = true;
  if (image.nextElementSibling) image.nextElementSibling.hidden = false;
  if (image.parentElement && image.parentElement.classList.contains("sports-league-mark")) image.parentElement.classList.remove("has-logo");
}



function safeSportsTeamColor(value, fallback) {
  const color = String(value || "").trim();
  return /^#[0-9a-f]{6}$/i.test(color) ? color : fallback;
}



function rankedSportsBroadcasts(event) {
  const preferredID = String(sportsPreferredChannelMap()[event.leagueId] || "");
  const preferredNetwork = String(sportsPreferredNetworkMap()[event.leagueId] || "");
  return uniqueEventChannels(event.channels).map(function(channel, index) {
    const traits = sportsBroadcastTraits(channel, event);
    const preferred = String(channel.id || "") === preferredID;
    const preferredNetworkMatch = !!preferredNetwork && sportsBroadcastNetworkKey(channel) === preferredNetwork;
    const rank = (preferred ? 1000 : (preferredNetworkMatch ? 900 : 0)) + (traits.indexOf("Likely home") >= 0 ? 90 : 0) + (traits.indexOf("Likely away") >= 0 ? 80 : 0) + (traits.indexOf("Likely local") >= 0 ? 70 : 0) + (traits.indexOf("Major network") >= 0 ? 60 : 0) - (traits.indexOf("Alternate") >= 0 ? 10 : 0);
    return Object.assign({}, channel, { sportsPreferred: preferred, sportsPreferredNetwork: preferredNetworkMatch, sportsTraits: traits, sportsRank: rank, sportsOriginalIndex: index });
  }).sort(function(left, right) { return right.sportsRank - left.sportsRank || left.sportsOriginalIndex - right.sportsOriginalIndex; });
}

function markSportsDetailBackgroundFailed(image) {
  if (!image) return;
  const failed = safeSportsMediaURL(image.getAttribute("src"));
  if (failed) state.sportsFailedMedia[failed] = true;
  const fallback = safeSportsMediaURL(image.getAttribute("data-sports-detail-fallback"));
  image.removeAttribute("data-sports-detail-fallback");
  if (fallback && fallback !== failed && !sportsMediaFailed(fallback)) {
    image.setAttribute("src", fallback);
    return;
  }
  image.hidden = true;
  if (image.parentElement) {
    image.parentElement.classList.remove("has-art");
    image.parentElement.classList.add("no-art");
  }
}

function safeSportsMediaURL(value) {
  value = String(value || "").trim();
  if (value.indexOf("/dispatcharr/api/sports/image") === 0 || value.indexOf("/xtream/api/sports/image") === 0) return route(value);
  return /^(https?:\/\/|\/)/i.test(value) ? value : "";
}







function markSportsBackgroundFailed(image) {
  const url = safeSportsMediaURL(image && image.getAttribute("src"));
  if (url) state.sportsFailedMedia[url] = true;
  if (!image) return;
  const fallback = safeSportsMediaURL(image.getAttribute("data-sports-background-fallback"));
  if (fallback && fallback !== url && !sportsMediaFailed(fallback)) {
    image.removeAttribute("data-sports-background-fallback");
    image.classList.remove("sports-field-bg");
    image.classList.add("sports-generated-bg");
    image.setAttribute("src", fallback);
    return;
  }
  image.hidden = true;
  if (image.parentElement) image.parentElement.classList.remove("has-generated-art");
}

function openSportsEvent(eventID) {
  const enteringSports = state.view !== "sports";
  if (enteringSports) {
    state.view = "sports";
    state.sportsTab = "favorites";
  }
  state.sportsSelectedEventID = String(eventID || "");
  state.sportsExpandedEvents = {};
  if (enteringSports) render();
  else renderSportsPage();

}

function openSportsLeague(leagueID) {
  state.sportsLeague = String(leagueID || "");
  state.sportsSelectedEventID = "";
  state.sportsExpandedEvents = {};
  renderSportsPage();
  loadSportsLeagueTeams(sportsLeagueByID(state.sports, state.sportsLeague));

}

function backFromSportsDetail() {
  if (state.sportsSelectedEventID) {
    state.sportsSelectedEventID = "";
  } else {
    state.sportsLeague = "";
    state.sportsTab = "live";
  }
  renderSportsPage();
}

function setSportsPreferredChannel(leagueID, channelID) {
  leagueID = String(leagueID || "");
  channelID = String(channelID || "");
  if (!leagueID || !channelID) return;
  if (String(state.app.preferences.sportsPreferredChannels[leagueID] || "") === channelID) {
    delete state.app.preferences.sportsPreferredChannels[leagueID];
    delete state.app.preferences.sportsPreferredNetworks[leagueID];
  } else {
    state.app.preferences.sportsPreferredChannels[leagueID] = channelID;
    const networkKey = sportsBroadcastNetworkKey(channelByID(channelID));
    if (networkKey) state.app.preferences.sportsPreferredNetworks[leagueID] = networkKey;
    else delete state.app.preferences.sportsPreferredNetworks[leagueID];
  }
  savePrefs({ quiet: true });
  renderSportsPage();
}

function sportsReplayMatchesForEvent() { return []; }
function renderSportsEventDetail(payload, event) {
  const channels = rankedSportsBroadcasts(event);
  const providerArt = sportsEventArtwork(event, "backdrop");
  const sportArt = sportsFieldBackgroundURL(event);
  const art = providerArt || sportArt;
  const artDimensions = sportsArtworkDimensions(event, "backdrop", art);
  const related = sportsLeagueEvents(payload, event.leagueId).filter(function(item) {
    return item.id !== event.id && sportsEventHasPlayableAccess(item);
  }).slice(0, 6);
  const broadcasts = channels.length ? renderSportsBroadcastGroups(channels, event, true) : "";
  const relatedBody = related.length ? "<div class=\"sports-event-grid\">" + related.map(renderSportsEventTile).join("") + "</div>" : "";
  const metadata = [event.venue].filter(Boolean);
  const metadataHTML = metadata.length ? "<p class=\"sports-event-metadata\">" + metadata.map(escapeHTML).join(" · ") + "</p>" : "";
  const navigation = renderSportsEventNavigation(payload, event);
  const leagueFavorite = !!sportsFavoriteLeagueMap()[event.leagueId];
  const leagueLabel = sportsDetailLeagueLabel(event);
  const leagueAction = leagueLabel ? '<button type="button" class="sports-detail-tool' + (leagueFavorite ? ' active' : '') + '" data-sports-favorite-league="' + escapeHTML(event.leagueId || '') + '" data-sports-favorite-enabled="' + (leagueFavorite ? 'false' : 'true') + '" aria-pressed="' + (leagueFavorite ? 'true' : 'false') + '">' + icon(leagueFavorite ? 'heart-solid' : 'heart') + '<span>' + (leagueFavorite ? 'Following league' : 'Follow league') + '</span></button>' : '';
  const artHTML = art ? '<img class="sports-event-hero-art" src="' + escapeHTML(art) + '" alt=""' + artDimensions + ' data-sports-detail-fallback="' + escapeHTML(providerArt && sportArt !== providerArt ? sportArt : '') + '" data-img-error="sports-detail-bg">' : '';
  return '<div class="sports-pinned sports-detail-toolbar sports-event-toolbar">' + navigation + '<div class="sports-detail-actions"><button type="button" class="sports-detail-tool' + (sportsScoresHidden(false) ? ' active' : '') + '" data-sports-spoilers="global" aria-pressed="' + (sportsScoresHidden(false) ? 'true' : 'false') + '">' + icon(sportsScoresHidden(false) ? 'eye-off' : 'eye') + '<span>' + (sportsScoresHidden(false) ? 'Show scores' : 'Hide scores') + '</span></button>' + leagueAction + '<button type="button" class="sports-detail-tool sports-refresh" data-sports-refresh="true">' + icon("loader") + '<span>Refresh scores</span></button></div></div>'
    + '<div class="sports-score-scroll sports-event-detail"><header class="sports-event-hero' + (art ? ' has-art' : ' no-art') + '">' + artHTML
    + '<div class="sports-event-hero-copy">' + (leagueLabel ? '<span class="sports-eyebrow">' + escapeHTML(leagueLabel) + '</span>' : '') + '<h1>' + escapeHTML(sportsEventTitle(event)) + '</h1>' + metadataHTML + renderSportsDetailScore(event) + (broadcasts ? '<div class="sports-hero-feeds sports-broadcast-section" role="group" aria-label="Watch">' + broadcasts + '</div>' : '') + '</div></header>'
    + renderSportsGameStats(event)
    + sportsSectionHTML(leagueLabel ? "More from " + leagueLabel : "More events", "", relatedBody, "sports-related-section") + "</div>";
}

function sportsEventArtwork(event, kind) {
  const typed = sportsTypedArtwork(event, kind);
  if (typed) return typed.url;
  const direct = safeSportsMediaURL(event && event.imageUrl);
  if (direct) return direct;
  return "";
}

function replayChannels() {
  return searchableChannels().filter(function(channel) {
    return /\breplay(?:s)?\b/i.test([channel.name, channel.categoryName, channel.categoryId].join(" "));
  });
}
function loadSportsReplays() { return Promise.resolve(replayChannels()); }
function sportsReplayStatusLabel() { return "Provider REPLAY channels"; }
function renderSportsReplays() {
  const channels = replayChannels();
  return '<section class="my-tv-section"><header><h2>Replays</h2><span>Provider REPLAY channels</span></header>' + (channels.length ? '<div class="search-result-list">' + channels.map(myTVChannelRow).join('') + '</div>' : '<div class="empty">No REPLAY channels are available from your selected sources.</div>') + '</section>';
}
