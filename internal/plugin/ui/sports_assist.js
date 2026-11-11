// Decision state is separate from playback: only the player adapter can tune a real channel.
class SportsSwitchEngine {
  constructor() {
    this.pending = null;
    this.current = null;
    this.mode = "off";
    this.locked = false;
    this.blockedUntil = 0;
    this.lastTick = null;
  }
  cancel(now) { this.pending = null; this.blockedUntil = now + 120000; }
  lock(channelID) { this.current = channelID; this.locked = true; this.pending = null; }
  unlock(now) { this.locked = false; this.pending = null; this.blockedUntil = now + 30000; }
  stop() { this.pending = null; this.locked = false; }
  tick(input) {
    const {now, current, mode, candidates, active, fresh} = input;
    // A throttled timer must not return from the background and tune immediately.
    if (this.lastTick !== null && (now - this.lastTick > 2500 || now < this.lastTick)) this.pending = null;
    this.lastTick = now;
    if (this.current !== current) {
      if (this.current !== null) this.blockedUntil = now + 90000;
      this.current = current; this.pending = null; this.locked = false;
    }
    if (this.mode !== mode) {
      this.pending = null;
      if (this.mode === "off" && mode !== "off") this.blockedUntil = now;
      this.mode = mode;
    }
    const rows = candidates || [];
    const playing = rows.find(row => row.channelID === current);
    const best = rows.filter(row => row.channelID && row.channelID !== current && row.live && row.followed)
      .sort((a, b) => (Number(b.priority) || 0) - (Number(a.priority) || 0) || a.channelID.localeCompare(b.channelID))[0];
    if ((mode !== "auto" && mode !== "suggest") || this.locked || now < this.blockedUntil || !active || !fresh || !best || (playing && playing.live && playing.followed)) {
      this.pending = null;
      return {};
    }
    if (!this.pending || this.pending.channelID !== best.channelID) {
      this.pending = Object.assign({}, best, {deadline: mode === "auto" ? now + 5000 : 0});
    }
    if (mode === "auto" && now >= this.pending.deadline) {
      this.pending = null; this.blockedUntil = now + 90000;
      return {switchTo: best.channelID};
    }
    return {pending: this.pending};
  }
}

const sportsAssist = {
  engine: new SportsSwitchEngine(), viewer: null, generation: 0,
  payload: null, receivedAt: -Infinity, lastFetch: -Infinity, request: null,
  stale: true, message: "", channelSource: null, channels: new Map(), practice: null,
};
function sportsAssistMode() {
  const value = prefs().sportsSwitchMode;
  return value === "suggest" || value === "auto" ? value : "off";
}
function stopSportsAssist() {
  sportsAssist.generation++;
  if (sportsAssist.request) sportsAssist.request.abort();
  sportsAssist.request = null;
  sportsAssist.payload = null;
  sportsAssist.stale = true;
  sportsAssist.lastFetch = -Infinity;
  sportsAssist.engine.stop();
  const banner = byId("sports-assist-banner");
  if (banner) banner.remove();
}
function sportsAssistPoll(now) {
  if (sportsAssist.request || now - sportsAssist.lastFetch < 30000) return;
  sportsAssist.lastFetch = now;
  const abort = new AbortController(), generation = sportsAssist.generation;
  const viewer = sportsCompanionViewer();
  sportsAssist.request = abort;
  const timeout = setTimeout(() => abort.abort(), 12000);
  // Read the existing prepared payload. Never force a provider/guide rebuild.
  coreFetch(route("/dispatcharr/api/sports"), {credentials: "same-origin", signal: abort.signal})
    .then(response => { if (!response.ok) throw new Error("Sports unavailable"); return response.json(); })
    .then(payload => {
      if (abort.signal.aborted || generation !== sportsAssist.generation || viewer !== sportsCompanionViewer() || document.hidden || state.view !== "player" || sportsAssistMode() === "off") return;
      sportsAssist.payload = payload;
      sportsAssist.receivedAt = performance.now();
      sportsAssist.stale = false;
    })
    .catch(() => { if (generation === sportsAssist.generation) sportsAssist.stale = true; })
    .finally(() => { clearTimeout(timeout); if (sportsAssist.request === abort) sportsAssist.request = null; });
}
function sportsAssistCandidates(payload) {
  const channels = state.app && state.app.channels;
  if (sportsAssist.channelSource !== channels) {
    sportsAssist.channelSource = channels;
    sportsAssist.channels = new Map(items(channels).map(channel => [String(channel.id), channel]));
  }
  const current = String((state.currentChannel && state.currentChannel.id) || "");
  const rows = [];
  for (const event of items(payload && payload.events)) {
    const teamFollowed = sportsFavoriteTeamMatches(event.home || {}) || sportsFavoriteTeamMatches(event.away || {});
    const followed = teamFollowed || !!(prefs().sportsFavoriteLeagues || {})[event.leagueId];
    const isCurrent = items(event.channels).some(channel => String(channel.id) === current);
    if ((!event.live || event.completed) && !isCurrent) continue;
    if (!followed && !isCurrent) continue;
    const match = playerSportsEventChannel(event);
    if (!match || !(Number(match.score) >= 60) || !sportsAssist.channels.has(String(match.id))) continue;
    let live = event.live === true && !event.completed;
    // Never use an undelayed final flag to cut off the end of the current game.
    if (isCurrent && sportsCompanionDelay() > 0 && typeof sportsCompanion !== "undefined") {
      const sample = sportsCompanion.buffer.read(sportsEventStateID(event), sportsCompanionDelay(), performance.now());
      if (sample) live = !sample.data.completed;
    }
    rows.push({channelID: String(match.id), eventID: String(event.id || event.stableId || ""),
      label: sportsTeamName(event.away) + " at " + sportsTeamName(event.home),
      followed, live, priority: teamFollowed ? 2 : 1});
    if (rows.length >= 32) break;
  }
  return rows;
}
function sportsAssistFresh(now) {
  const payload = sportsAssist.payload;
  const age = payload ? Date.now() / 1000 - Number(payload.updatedAtUnix) : Infinity;
  return !!payload && !sportsAssist.stale && !payload.error && !payload.refreshing && !payload.incomplete &&
    Number.isFinite(age) && age >= -30 && age <= 90 && now - sportsAssist.receivedAt <= 75000;
}
function sportsAssistPlaybackActive() {
  const video = byId("player");
  if (document.hidden || state.view !== "player" || state.playerErrorMessage || !video || video.paused || video.ended || video.seeking || video.readyState < 3) return false;
  if (state.timeShiftSession && state.timeShiftSession.rewound) return false;
  if (video.seekable && video.seekable.length && Number.isFinite(video.currentTime)) {
    const latency = (state.hls && Number(state.hls.targetLatency)) || 0;
    if (video.seekable.end(video.seekable.length - 1) - video.currentTime > Math.max(20, latency + 8)) return false;
  }
  if (sportsCompanionDelay() > 0) {
    if (typeof sportsCompanion === "undefined" || !sportsCompanion.event || sportsCompanion.viewer !== sportsCompanionViewer()) return false;
    if (!sportsCompanion.buffer.read(sportsEventStateID(sportsCompanion.event), sportsCompanionDelay(), performance.now())) return false;
  }
  return true;
}
function sportsAssistSwitch(channelID) {
  const now = performance.now();
  if (!sportsAssistFresh(now) || !sportsAssistPlaybackActive() || sportsAssist.viewer !== sportsCompanionViewer() || sportsAssistMode() === "off") return;
  const candidate = sportsAssistCandidates(sportsAssist.payload).find(row => row.channelID === channelID && row.live && row.followed);
  const channel = candidate && sportsAssist.channels.get(channelID);
  if (!channel) { sportsAssist.engine.cancel(now); return; }
  sportsAssist.engine.pending = null;
  sportsAssist.engine.blockedUntil = now + 90000;
  try {
    Promise.resolve(playChannel(channel)).catch(() => { sportsAssist.message = "This game could not be opened. Choose another channel."; });
  } catch (_) { sportsAssist.message = "This game could not be opened. Choose another channel."; }
}
function renderSportsAssistBanner() {
  const pending = sportsAssist.engine.pending;
  let root = byId("sports-assist-banner");
  if (!pending) { if (root) root.remove(); return; }
  if (!root) {
    const stage = document.querySelector(".playback-stage");
    if (!stage) return;
    root = document.createElement("aside");
    root.id = "sports-assist-banner"; root.className = "sports-assist-banner";
    root.innerHTML = '<p data-assist-message role="status" aria-live="polite"></p><div><button type="button" data-sports-assist-watch>Watch now</button><button type="button" data-sports-assist-cancel>Cancel</button><button type="button" data-sports-assist-lock>Stay here</button></div>';
    stage.appendChild(root);
  }
  const seconds = Math.max(0, Math.ceil((pending.deadline - performance.now()) / 1000));
  root.querySelector("[data-assist-message]").textContent = pending.deadline
    ? "Switching to " + pending.label + " in " + seconds + "s"
    : "Suggested game: " + pending.label;
}
function renderSportsAssistControls() {
  const mode = sportsAssistMode(), locked = sportsAssist.engine.locked;
  return '<section class="sports-assist-controls" aria-label="Game switching"><label>Game switching <select data-sports-switch-mode aria-label="Game switching">' +
    [["off", "Off"], ["suggest", "Suggest only"], ["auto", "Switch after countdown"]].map(row => '<option value="' + row[0] + '"' + (row[0] === mode ? " selected" : "") + '>' + row[1] + '</option>').join("") +
    '</select></label><button type="button" data-sports-assist-lock aria-pressed="' + locked + '">' + (locked ? "Unlock channel" : "Stay here") + '</button>' +
    '<p>' + escapeHTML(sportsAssist.message || (locked ? "Locked to this channel until you unlock it or choose another." : "Suggestions prioritize followed live games. Your manual channel choices get a 90-second break.")) + '</p></section>';
}
function sportsAssistTick() {
  if (state.view !== "player") { stopSportsAssist(); return; }
  const viewer = sportsCompanionViewer();
  if (viewer !== sportsAssist.viewer) {
    stopSportsAssist(); sportsAssist.viewer = viewer; sportsAssist.engine = new SportsSwitchEngine();
  }
  const now = performance.now(), mode = sportsAssistMode();
  if (document.hidden || mode === "off" || sportsAssist.practice) {
    if (sportsAssist.request) sportsAssist.request.abort();
    sportsAssist.payload = null; sportsAssist.lastFetch = -Infinity;
    sportsAssist.engine.tick({now, current: String((state.currentChannel && state.currentChannel.id) || ""), mode, candidates: [], active: false, fresh: false});
    renderSportsAssistBanner(); return;
  }
  sportsAssistPoll(now);
  const fresh = sportsAssistFresh(now);
  const result = sportsAssist.engine.tick({now, current: String((state.currentChannel && state.currentChannel.id) || ""), mode,
    candidates: fresh ? sportsAssistCandidates(sportsAssist.payload) : [], active: sportsAssistPlaybackActive(), fresh});
  if (result.switchTo) sportsAssistSwitch(result.switchTo);
  renderSportsAssistBanner();
}
document.addEventListener("change", event => {
  if (!event.target.matches("[data-sports-switch-mode]")) return;
  prefs().sportsSwitchMode = ["off", "suggest", "auto"].includes(event.target.value) ? event.target.value : "off";
  sportsAssist.engine.pending = null; sportsAssist.message = "";
  savePrefs(); sportsAssistTick(); renderSportsCompanion();
});
document.addEventListener("click", event => {
  const button = event.target.closest && event.target.closest("[data-sports-assist-cancel], [data-sports-assist-lock], [data-sports-assist-watch]");
  if (!button) return;
  const now = performance.now();
  if (button.hasAttribute("data-sports-assist-cancel")) sportsAssist.engine.cancel(now);
  else if (button.hasAttribute("data-sports-assist-lock")) {
    if (sportsAssist.engine.locked) sportsAssist.engine.unlock(now);
    else sportsAssist.engine.lock(String((state.currentChannel && state.currentChannel.id) || ""));
  } else if (sportsAssist.engine.pending) sportsAssistSwitch(sportsAssist.engine.pending.channelID);
  renderSportsAssistBanner(); renderSportsCompanion();
});
