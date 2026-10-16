// Multiview: tiled live channels.
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
  tile.attaching = false;
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
  if (!tile || !tile.channel) return Promise.resolve(tile && tile.session ? tile.session : null);
  if (tile.session) return Promise.resolve(tile.session);
  if (tile.sessionPromise) return tile.sessionPromise;
  recordWatchPreference(tile.channel);
  const attempt = (tile.watchAttempt || 0) + 1;
  tile.watchAttempt = attempt;
  const pending = postJSON("/dispatcharr/api/watch/start", { itemKind: "channel", itemId: tile.channel.id, itemName: tile.channel.name }).then(function(payload) {
    const session = payload && payload.session;
    if (attempt !== tile.watchAttempt || items(state.multiviewTiles).indexOf(tile) === -1) {
      // The tile was removed (or multiview closed) while this start was in flight.
      if (session && session.id) postJSON("/dispatcharr/api/watch/stop", { sessionId: session.id, reason: "superseded" }).catch(function() {});
      const stale = new Error("multiview watch session superseded");
      stale.superseded = true;
      throw stale;
    }
    tile.session = session;
    startMultiviewHeartbeat();
    renderRail();
    return tile.session;
  }).finally(function() {
    if (tile.sessionPromise === pending) tile.sessionPromise = null;
  });
  tile.sessionPromise = pending;
  return pending;
}
function stopMultiviewWatch(tile, reason) {
  if (!tile) return;
  // Invalidate any /watch/start still in flight for this tile.
  tile.watchAttempt = (tile.watchAttempt || 0) + 1;
  tile.sessionPromise = null;
  if (!tile.session) return;
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
  const guidance = tiles.length ? "Select a tile to hear its audio." : "Choose up to four channels.";
  byId("view").innerHTML = "<section class=\"multiview-page\"><div class=\"multiview-toolbar\"><div><h2>Multiview</h2><p>" + escapeHTML(guidance) + "</p></div><div class=\"multiview-actions\"><span class=\"multiview-count\">" + escapeHTML(String(tiles.length)) + "/4</span>" + (tiles.length ? "<button class=\"chip\" type=\"button\" data-multiview-action=\"clear\">Clear</button>" : "") + "</div></div>"
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
    if (!video || tile.attached || tile.attaching || !tile.channel) return;
    tile.attaching = true;
    startMultiviewWatch(tile).then(function(session) {
      if (!document.body.contains(video) || tile.attached) return;
      const attachment = attachVideoSource(video, browserStreamURL(tile.channel, session && session.id), { rewindable: isRewindableChannel(tile.channel), format: tile.channel.streamFormat, hlsBufferSeconds: tile.channel.hlsBufferSeconds });
      tile.hls = attachment.hls;
      tile.tsPlayer = attachment.tsPlayer;
      tile.attached = true;
      video.addEventListener("click", function() { focusMultiviewTile(tile.id); });
      video.addEventListener("dblclick", function() { openMultiviewTileSingle(tile.id); });
      video.play().catch(function() {});
    }).catch(function(error) {
      if (error && error.superseded) return;
      showAppToast("Could not reserve a provider connection: " + readableError(error));
    }).finally(function() { tile.attaching = false; });
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
