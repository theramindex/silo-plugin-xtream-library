// Player: playback, controls, tracks, Live Rewind, and watch sessions.
function playerRequestHeaders() {
  const headers = {};
  if (coreAccessToken) headers.Authorization = "Bearer " + coreAccessToken;
  const profileID = coreStoredValue("profile_id");
  const profileToken = coreStoredValue("profile_token");
  if (profileID) headers["X-Profile-Id"] = profileID;
  if (profileToken) headers["X-Profile-Token"] = profileToken;
  return headers;
}
function stopCurrentWatch(reason) {
  // Invalidate any /watch/start still in flight so its session is stopped on arrival.
  state.watchAttempt = (state.watchAttempt || 0) + 1;
  if (!state.currentSession) return;
  postJSON("/dispatcharr/api/watch/stop", { sessionId: state.currentSession.id, reason: reason || "stop" }).catch(function() {});
  state.currentSession = null;
  if (state.heartbeat) {
    clearInterval(state.heartbeat);
    state.heartbeat = null;
  }
}
function channelProgramSubtitle(channel) {
  const program = currentProgram(channel) || {};
  const title = String(program.title || "").trim();
  const channelName = String(channel && channel.name || "").trim();
  return title && lower(title) !== lower(channelName) ? title : "Live channel";
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
  const playbackShellClass = (replayMode ? "playback-shell is-replay" : "playback-shell") + (sportsFirstPlayerActive() ? " sports-enabled" : "");
  const videoAttributes = " autoplay playsinline" + (replayMode ? " controls" : "");
  const modeTag = replayMode ? "Replay" : "AV";
  const liveProgramWindow = !replayMode ? "<div class=\"player-live-window\"><span>Started " + escapeHTML(start) + "</span><strong><span class=\"live-dot\"></span>Live</strong><span>Ends " + escapeHTML(end) + "</span></div>" : "";
  const timeShiftControls = "<div id=\"player-timeshift-controls\" class=\"player-timeshift-controls hidden\"><button class=\"player-icon\" data-player-action=\"rewind-30\" aria-label=\"Rewind 30 seconds\">" + icon("rewind") + "</button><button id=\"player-timeshift-play\" class=\"player-icon\" data-player-action=\"play-toggle\" aria-label=\"Play\">" + icon("play") + "</button><button class=\"player-icon\" data-player-action=\"forward-30\" aria-label=\"Forward 30 seconds\">" + icon("forward") + "</button><input id=\"player-timeshift-range\" type=\"range\" min=\"0\" max=\"1\" step=\"0.25\" value=\"1\" aria-label=\"Live Rewind position\"><button class=\"player-live-button\" data-player-action=\"go-live\"><span class=\"live-dot\"></span><span id=\"player-timeshift-label\">LIVE</span></button></div>";
  const topActions = "<div class=\"player-top-actions\">"
    + "<div class=\"player-audio\"><button id=\"player-audio-button\" class=\"player-chip\" data-player-action=\"audio-menu\" aria-haspopup=\"true\" aria-expanded=\"false\"><span>Audio</span>" + icon("chevron-down") + "</button><div id=\"player-audio-menu\" class=\"player-menu\" role=\"menu\"></div></div>"
    + "<div class=\"player-volume\"><button id=\"player-volume-button\" class=\"player-icon\" data-player-action=\"volume-menu\" aria-label=\"Volume\" aria-haspopup=\"true\" aria-expanded=\"false\">" + icon("speaker") + "</button><div id=\"player-volume-popover\" class=\"volume-popover\"><span>VOL</span><input id=\"player-volume-slider\" type=\"range\" min=\"0\" max=\"100\" step=\"1\" value=\"" + Math.round(state.volume * 100) + "\" aria-label=\"Volume\"><span id=\"player-volume-value\" class=\"volume-value\"></span></div></div>"
    + "<button class=\"player-icon\" data-player-action=\"cast\" aria-label=\"AirPlay or Cast\">" + icon("airplay") + "</button>"
    + "<button id=\"player-guide-button\" class=\"player-icon player-guide-button\" data-player-action=\"guide\" aria-label=\"Guide\" aria-haspopup=\"true\" aria-expanded=\"false\">" + icon("guide") + "</button>"
    + "<div class=\"player-more\"><button id=\"player-more-button\" class=\"player-icon\" data-player-action=\"more\" aria-label=\"More\" aria-haspopup=\"true\" aria-expanded=\"false\">" + icon("ellipsis") + "</button><div id=\"player-more-menu\" class=\"player-more-menu\"></div></div></div>";
  const bottomActions = "<div class=\"player-bottom-actions\">" + playerFavoriteButtonHTML(channel)
    + "<button class=\"player-icon\" data-player-action=\"add-multiview\" aria-label=\"Add current channel to multiview\">" + icon("multiview") + "</button>"
    + "<button id=\"player-subtitles-button\" class=\"player-icon\" data-player-action=\"subtitles\" aria-label=\"Subtitles\" aria-pressed=\"false\">" + icon("captions") + "</button>"
    + "<button id=\"player-language-button\" class=\"player-icon\" data-player-action=\"language-menu\" aria-label=\"Audio language\" aria-haspopup=\"true\" aria-expanded=\"false\">" + icon("language") + "</button></div>";
  byId("view").innerHTML = "<section class=\"" + playbackShellClass + "\"><div class=\"playback-stage\">"
    + "<video id=\"player\" class=\"playback-video\"" + videoAttributes + "></video><div class=\"playback-scrim\"></div>"
    + "<button id=\"player-center-button\" class=\"player-center-button hidden\" data-player-action=\"play-toggle\" aria-label=\"Play\">" + icon("play") + "</button>"
    + "<div class=\"player-top\"><button class=\"player-exit player-icon\" data-player-action=\"back\" aria-label=\"Back to Live TV browse\">" + icon("arrow-left") + "</button>" + topActions + "</div>"
    + "<div id=\"player-toast\" class=\"player-toast\" role=\"status\"></div><div id=\"player-guide-panel\" class=\"player-guide-panel\"></div>"
    + "<div id=\"player-sports-status\" class=\"sr-only\" role=\"status\"></div><div id=\"player-sports-drawer\" class=\"player-sports-drawer\"></div>"
    + "<div class=\"player-bottom\"><div class=\"player-bottom-row\"><div class=\"player-meta\">" + playerLogoHTML(channel)
    + "<div class=\"player-kicker\">" + escapeHTML(channelName) + "</div><h2 class=\"player-title\">" + escapeHTML(title) + "</h2>"
    + "<p class=\"player-description\" data-overflow-description=\"true\">" + escapeHTML(description) + "</p><div class=\"player-tags\"><span class=\"player-tag\">" + escapeHTML(categoryNameText) + "</span><span id=\"player-mode-tag\" class=\"player-tag\">" + escapeHTML(modeTag) + "</span></div></div>"
    + bottomActions + "</div>" + timeShiftControls + liveProgramWindow + "</div></div></section>";
  updateAudioMenu();
  updateVolumeMenu();
  renderPlayerGuidePanel();
  renderPlayerSportsDrawer();
  if (sportsFirstPlayerActive()) {
    loadSports(false);
    startPlayerSportsRefresh();
  }
  renderPlayerMoreMenu();
  updateFullscreenButton();
  wakePlayerChrome(1800);
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
function currentStreamURL() {
  return state.currentChannel ? browserStreamURL(state.currentChannel, state.currentSession && state.currentSession.id) : "";
}
function browserStreamURL(channel, sessionID) {
  return route("/dispatcharr/stream?channel_id=" + encodeURIComponent(channel.id) + "&output_profile=2" + (sessionID ? "&session_id=" + encodeURIComponent(sessionID) : ""));
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
  setVideoSource(browserStreamURL(channel, state.currentSession && state.currentSession.id), { rewindable: isRewindableChannel(channel), format: channel.streamFormat, hlsBufferSeconds: channel.hlsBufferSeconds });
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
  const managedTimeShift = !!(options && options.managedTimeShift);
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
    const hlsOptions = managedTimeShift ? { liveSyncDurationCount: 1, liveMaxLatencyDurationCount: 5, maxBufferLength: 60 } : liveHLSOptions(options && options.hlsBufferSeconds);
    hlsOptions.xhrSetup = function(xhr) {
      const headers = playerRequestHeaders();
      Object.keys(headers).forEach(function(name) { xhr.setRequestHeader(name, headers[name]); });
      xhr.withCredentials = true;
    };
    attachment.hls = new Hls(hlsOptions);
    let recoveryAttempts = 0;
    let fatalHandled = false;
    attachment.hls.on(Hls.Events.FRAG_LOADED, function() { recoveryAttempts = 0; });
    attachment.hls.on(Hls.Events.ERROR, function(_, data) {
      if (!data || !data.fatal) return;
      if (!managedTimeShift && recoveryAttempts < 2) {
        recoveryAttempts += 1;
        if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
          attachment.hls.startLoad();
          return;
        }
        if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
          attachment.hls.recoverMediaError();
          return;
        }
      }
      if (!fatalHandled && options && typeof options.onFatal === "function") {
        fatalHandled = true;
        options.onFatal(data);
      }
    });
    attachment.hls.loadSource(url);
    attachment.hls.attachMedia(video);
  } else if (window.mpegts && mpegts.isSupported() && !isHLS) {
    attachment.tsPlayer = mpegts.createPlayer({ type: "mpegts", isLive: !rewindable, url: url, headers: playerRequestHeaders() });
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
  const sportsControl = sportsFirstPlayerEnabled()
    ? "<button data-player-action=\"sports\">" + menuIcon("trophy") + "<span>Sports center</span></button>"
    : "";
  menu.innerHTML = "<div class=\"player-more-kicker\">Playback</div>"
    + "<button data-player-action=\"aspect\">" + menuIcon("aspect") + "<span>Aspect ratio<small>" + (state.aspectMode === "fit" ? "Fit to screen" : "Fill screen") + "</small></span></button>"
    + "<button data-player-action=\"fullscreen\">" + menuIcon(document.fullscreenElement ? "fullscreen-exit" : "fullscreen") + "<span>Fullscreen<small>" + (document.fullscreenElement ? "Exit player fullscreen" : "Fill the display") + "</small></span></button>"
    + "<button data-player-action=\"pip\">" + menuIcon("pip") + "<span>Picture in Picture</span></button>"
    + "<button data-player-action=\"guide\">" + menuIcon("guide") + "<span>Channel guide</span></button>"
    + sportsControl
    + "<button data-player-action=\"add-multiview\">" + menuIcon("multiview") + "<span>Add to multiview</span></button>"
    + "<button data-player-action=\"search-channel\">" + menuIcon("search") + "<span>Search channel</span></button>"
    + (recent.length ? "<div class=\"player-more-separator\"></div><div class=\"player-more-kicker\">Recent channels</div>" + recent.map(function(channel) { return "<button data-channel=\"" + escapeHTML(channel.id) + "\">" + logoHTML(channel) + "<span>" + escapeHTML(channel.name || "Untitled") + "<small>" + escapeHTML(channel.categoryName || "Live TV") + "</small></span></button>"; }).join("") : "")
    + "<div class=\"player-more-separator\"></div><div class=\"player-more-kicker\">Open elsewhere</div>"
    + "<button data-player-action=\"copy-stream\">" + menuIcon("copy") + "<span>Copy stream URL</span></button>"
    + "<button data-player-action=\"open-stream\">" + menuIcon("external") + "<span>Open stream in new tab</span></button>";
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
function showPlayerToast(message, durationMs) {
  const toast = byId("player-toast");
  if (!toast) return;
  toast.textContent = message;
  toast.classList.add("show");
  clearTimeout(state.toastTimer);
  state.toastTimer = setTimeout(function() { toast.classList.remove("show"); }, durationMs || 2400);
}
// With "Allow direct provider URLs" off, the gateway answers credentialed
// non-HLS streams (TS live, VOD, series, catch-up) with 422
// {"code":"stream_requires_hls"}. Media elements cannot read that body, so
// probe the gateway first; redirect: "manual" never follows to the provider.
async function streamBlockedMessage(url, format) {
  if (format === "hls" || adminSettings().allowDirectProviderURLs !== false) return "";
  try {
    const response = await coreFetch(url, { redirect: "manual", headers: { accept: "application/json" } });
    if (response.status !== 422) {
      if (response.body && response.body.cancel) response.body.cancel().catch(function() {});
      return "";
    }
    const payload = await response.json().catch(function() { return {}; });
    if (!payload || payload.code !== "stream_requires_hls") return "";
    return String(payload.error || "This stream can only play when the provider serves HLS.");
  } catch (_) {
    return "";
  }
}
async function showStreamBlockedIfNeeded(url, format) {
  const message = await streamBlockedMessage(url, format);
  if (!message) return false;
  showPlayerToast(message, 12000);
  return true;
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
// One AbortController per video source: switching sources removes every
// listener the previous source registered instead of stacking duplicates.
function videoSourceListenerOptions() {
  if (state.videoSourceListeners) state.videoSourceListeners.abort();
  state.videoSourceListeners = typeof AbortController === "function" ? new AbortController() : null;
  const signal = state.videoSourceListeners ? state.videoSourceListeners.signal : null;
  return function(extra) {
    const options = Object.assign({}, extra || {});
    if (signal) options.signal = signal;
    return options;
  };
}
function videoSourceListenerSignal() {
  return state.videoSourceListeners ? state.videoSourceListeners.signal : undefined;
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
  const listen = videoSourceListenerOptions();
  if (video.audioTracks && video.audioTracks.addEventListener) {
    video.audioTracks.addEventListener("addtrack", updateAudioMenu, listen());
    video.audioTracks.addEventListener("removetrack", updateAudioMenu, listen());
    video.audioTracks.addEventListener("change", updateAudioMenu, listen());
  }
  video.addEventListener("loadedmetadata", updateAudioMenu, listen({ once: true }));
  video.addEventListener("loadedmetadata", updateSubtitlesButton, listen({ once: true }));
  video.addEventListener("waiting", function() { state.playerWaiting = true; updateCenterPlayButton(); }, listen());
  video.addEventListener("stalled", function() { state.playerWaiting = true; updateCenterPlayButton(); }, listen());
  video.addEventListener("canplay", function() { state.playerWaiting = false; updateCenterPlayButton(); }, listen());
  video.addEventListener("playing", function() { state.playerWaiting = false; updateCenterPlayButton(); }, listen());
  video.addEventListener("pause", updateCenterPlayButton, listen());
  video.addEventListener("play", updateCenterPlayButton, listen());
  video.addEventListener("error", function() { state.playerWaiting = false; updateCenterPlayButton(); }, listen());
  if (video.textTracks && video.textTracks.addEventListener) {
    video.textTracks.addEventListener("addtrack", updateSubtitlesButton, listen());
    video.textTracks.addEventListener("removetrack", updateSubtitlesButton, listen());
    video.textTracks.addEventListener("change", updateSubtitlesButton, listen());
  }
  if (state.hls) { state.hls.destroy(); state.hls = null; }
  if (state.tsPlayer) { state.tsPlayer.destroy(); state.tsPlayer = null; }
  const attachment = attachVideoSource(video, url, { rewindable: rewindable, managedTimeShift: options && options.managedTimeShift, format: options && options.format, hlsBufferSeconds: options && options.hlsBufferSeconds, onFatal: options && options.onFatal });
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
function startWatch(channel) {
  const attempt = (state.watchAttempt || 0) + 1;
  state.watchAttempt = attempt;
  if (state.currentSession) postJSON("/dispatcharr/api/watch/stop", { sessionId: state.currentSession.id, reason: "switch_channel" }).catch(function() {});
  state.currentSession = null;
  recordWatchPreference(channel);
  return postJSON("/dispatcharr/api/watch/start", { itemKind: "channel", itemId: channel.id, itemName: channel.name }).then(function(payload) {
    const session = payload && payload.session;
    if (attempt !== state.watchAttempt) {
      // A newer switch (or a stop) happened while this start was in flight:
      // release the late provider connection instead of orphaning it.
      if (session && session.id) postJSON("/dispatcharr/api/watch/stop", { sessionId: session.id, reason: "superseded" }).catch(function() {});
      const stale = new Error("watch session superseded");
      stale.superseded = true;
      throw stale;
    }
    state.currentSession = session;
    if (state.heartbeat) clearInterval(state.heartbeat);
    state.heartbeat = setInterval(function() {
      if (state.currentSession) postJSON("/dispatcharr/api/watch/heartbeat", { sessionId: state.currentSession.id }).catch(function() {});
    }, 30000);
    renderRail();
    return state.currentSession;
  });
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
function stopWatchOnUnload(sessionID) {
  if (!sessionID) return;
  // keepalive fetch (not sendBeacon) so the Silo auth/profile headers from
  // coreRequestOptions travel with the request.
  try {
    fetch(route("/dispatcharr/api/watch/stop"), coreRequestOptions({
      method: "POST",
      keepalive: true,
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ sessionId: sessionID, reason: "page_unload" })
    })).catch(function() {});
  } catch (_) {}
}
