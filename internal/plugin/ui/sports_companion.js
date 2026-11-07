// Shared with Xtream. A snapshot's receipt time, never the upstream clock,
// determines when it may be shown. Keep enough history for the maximum delay.
class SportsCompanionBuffer {
  constructor() {
    this.game = "";
    this.snapshots = [];
  }
  push(game, data, now) {
    if (game !== this.game) {
      this.game = game;
      this.snapshots = [];
    }
    this.snapshots.push({ data: data, at: now });
    this.snapshots = this.snapshots
      .filter((x) => now - x.at <= 360000)
      .slice(-32);
  }
  read(game, seconds, now) {
    if (game !== this.game) return null;
    return (
      this.snapshots.filter((x) => x.at <= now - seconds * 1000).slice(-1)[0] ||
      null
    );
  }
}
const sportsCompanion = {
  buffer: new SportsCompanionBuffer(),
  timer: null,
  abort: null,
  generation: 0,
  event: null,
  lastFetch: -Infinity,
  pending: false,
  tab: "box",
  team: "all",
  stale: false,
};
function sportsCompanionDelay() {
  return Math.max(
    0,
    Math.min(300, Number(prefs().sportsStatsDelaySeconds) || 0),
  );
}
function sportsCompanionCurrent() {
  const events = playerSportsEvents();

  return (
    events.find(
      (event) =>
        state.currentChannel &&
        playerSportsChannelMatches(event, state.currentChannel.id),
    ) || null
  );
}
function startSportsCompanion() {
  stopSportsCompanion();
  sportsCompanion.event = null;
  sportsCompanion.buffer = new SportsCompanionBuffer();
  if (!sportsFirstPlayerActive()) return;
  const generation = sportsCompanion.generation;
  function tick() {
    if (state.view !== "player" || !byId("player-sports-drawer")) {
      stopSportsCompanion();
      return;
    }
    if (document.hidden) return;
    const event = sportsCompanionCurrent();
    const id = sportsEventStateID(event);
    if (id !== sportsEventStateID(sportsCompanion.event)) {
      if (sportsCompanion.abort) sportsCompanion.abort.abort();
      sportsCompanion.event = event;
      sportsCompanion.buffer = new SportsCompanionBuffer();
      sportsCompanion.lastFetch = -Infinity;
      sportsCompanion.pending = false;
      sportsCompanion.stale = false;
    }
    renderSportsCompanion();
    if (
      !id ||
      sportsCompanion.pending ||
      performance.now() - sportsCompanion.lastFetch < 30000
    )
      return;
    sportsCompanion.pending = true;
    sportsCompanion.lastFetch = performance.now();
    const abort = new AbortController();
    sportsCompanion.abort = abort;
    const timeout = setTimeout(() => abort.abort(), 12000);
    coreFetch(route("/xtream/api/sports?game_stats=" + encodeURIComponent(id)), {
      signal: abort.signal,
      credentials: "same-origin",
    })
      .then((r) => {
        if (!r.ok) throw new Error("Stats unavailable");
        return r.json();
      })
      .then((data) => {
        if (
          generation !== sportsCompanion.generation ||
          id !== sportsEventStateID(sportsCompanion.event) ||
          abort.signal.aborted
        )
          return;
        if (
          data.available ||
          (data.homeScore !== undefined &&
            data.homeScore !== "" &&
            data.awayScore !== undefined &&
            data.awayScore !== "")
        ) {
          sportsCompanion.buffer.push(id, data, performance.now());
          sportsCompanion.stale = false;
        } else sportsCompanion.stale = true;
      })
      .catch(() => {
        if (
          generation === sportsCompanion.generation &&
          sportsCompanion.abort === abort &&
          id === sportsEventStateID(sportsCompanion.event)
        )
          sportsCompanion.stale = true;
      })
      .finally(() => {
        clearTimeout(timeout);
        if (sportsCompanion.abort === abort) {
          sportsCompanion.pending = false;
          sportsCompanion.abort = null;
          renderSportsCompanion();
        }
      });
  }
  tick();
  sportsCompanion.timer = setInterval(tick, 1000);
}
function stopSportsCompanion() {
  sportsCompanion.generation++;
  if (sportsCompanion.timer) clearInterval(sportsCompanion.timer);
  if (sportsCompanion.abort) sportsCompanion.abort.abort();
  sportsCompanion.timer = null;
  sportsCompanion.abort = null;
  sportsCompanion.pending = false;
  sportsCompanion.stale = false;
  sportsCompanion.event = null;
  sportsCompanion.buffer = new SportsCompanionBuffer();
  const overlay = byId("sports-score-overlay");
  if (overlay) overlay.remove();
  const panel = byId("sports-companion-panel");
  if (panel) panel.remove();
}
function sportsCompanionMarkup(event, sample) {
  const esc = escapeHTML;
  const delay = sportsCompanionDelay();
  const hidden = !!(
    prefs().sportsSpoilersHidden || prefs().sportsPlayerSpoilersHidden
  );
  let html =
    '<section class="sports-companion" aria-label="Game companion"><div class="companion-toolbar"><strong>Game companion</strong><label>Delay <select data-companion-delay aria-label="Delay game statistics">' +
    [0, 30, 60, 90, 120, 180, 300]
      .map(
        (n) =>
          '<option value="' +
          n +
          '"' +
          (delay === n ? " selected" : "") +
          ">" +
          (n ? n + "s" : "Live") +
          "</option>",
      )
      .join("") +
    "</select></label></div>";
  if (!event)
    return (
      html + "<p>No game has a confident match to this channel.</p></section>"
    );
  html +=
    '<p class="companion-match">' +
    esc(sportsTeamName(event.away)) +
    " at " +
    esc(sportsTeamName(event.home)) +
    "</p>";
  if (hidden)
    return (
      html +
      "<p>Scores and statistics are hidden. Use the scores control to show them.</p></section>"
    );
  if (!sample)
    return (
      html +
      '<p role="status">' +
      (sportsCompanion.stale
        ? "Game statistics are unavailable. Retrying shortly."
        : delay
          ? "Building a " + delay + "s delay…"
          : "Waiting for game statistics…") +
      "</p></section>"
    );
  const data = sample.data;
  html +=
    '<p class="companion-freshness">' +
    esc(data.statusText || "") +
    (sportsCompanion.stale ||
    performance.now() - sample.at > (delay + 75) * 1000
      ? " · Updates delayed"
      : "") +
    "</p>";
  html +=
    '<div class="companion-tabs" aria-label="Game detail view">' +
    [
      ["box", "Box score"],
      ["stats", "Statistics"],
      ["plays", "Play-by-play"],
    ]
      .map(
        (pair) =>
          '<button type="button" data-companion-tab="' +
          pair[0] +
          '" aria-pressed="' +
          (sportsCompanion.tab === pair[0]) +
          '">' +
          pair[1] +
          "</button>",
      )
      .join("") +
    "</div>";
  if (sportsCompanion.tab !== "plays")
    html +=
      '<div class="companion-teams" aria-label="Filter teams">' +
      [
        ["all", "Both teams"],
        ["away", sportsTeamName(event.away)],
        ["home", sportsTeamName(event.home)],
      ]
        .map(
          (pair) =>
            '<button type="button" data-companion-team="' +
            pair[0] +
            '" aria-pressed="' +
            (sportsCompanion.team === pair[0]) +
            '">' +
            esc(pair[1]) +
            "</button>",
        )
        .join("") +
      "</div>";
  if (sportsCompanion.tab === "box") {
    const groups = items(data.boxScores).filter(
      (g) => sportsCompanion.team === "all" || g.side === sportsCompanion.team,
    );
    html += groups
      .map(
        (g) =>
          "<h3>" +
          esc(sportsTeamName(event[g.side])) +
          " · " +
          esc(g.category || "Players") +
          '</h3><div class="companion-table-wrap" tabindex="0" role="region" aria-label="' +
          esc(sportsTeamName(event[g.side]) + " player statistics") +
          '"><table><thead><tr><th scope="col">Player</th>' +
          items(g.columns)
            .map((c) => '<th scope="col">' + esc(c) + "</th>")
            .join("") +
          "</tr></thead><tbody>" +
          items(g.players)
            .map(
              (p) =>
                '<tr><th scope="row">' +
                esc(p.name) +
                "<small>" +
                esc(
                  [p.jersey, p.position, p.starter ? "Starter" : ""]
                    .filter(Boolean)
                    .join(" · "),
                ) +
                "</small></th>" +
                (p.didNotPlay
                  ? '<td colspan="' + g.columns.length + '">Did not play</td>'
                  : items(p.stats)
                      .map((v) => "<td>" + esc(v) + "</td>")
                      .join("")) +
                "</tr>",
            )
            .join("") +
          "</tbody></table></div>",
      )
      .join("");
    if (!groups.length) html += "<p>No player box score is available yet.</p>";
  } else if (sportsCompanion.tab === "stats") {
    html +=
      '<div class="companion-table-wrap" tabindex="0" role="region" aria-label="Team statistics"><table><thead><tr><th scope="col">Statistic</th>' +
      ["away", "home"]
        .filter(
          (side) =>
            sportsCompanion.team === "all" || sportsCompanion.team === side,
        )
        .map(
          (side) =>
            '<th scope="col">' + esc(sportsTeamName(event[side])) + "</th>",
        )
        .join("") +
      "</tr></thead><tbody>" +
      items(data.rows)
        .map(
          (row) =>
            '<tr><th scope="row">' +
            esc(row.label) +
            "</th>" +
            ["away", "home"]
              .filter(
                (side) =>
                  sportsCompanion.team === "all" ||
                  sportsCompanion.team === side,
              )
              .map((side) => "<td>" + esc(row[side]) + "</td>")
              .join("") +
            "</tr>",
        )
        .join("") +
      "</tbody></table></div>";
    html +=
      '<div class="companion-leaders">' +
      items(data.leaders)
        .filter(
          (p) =>
            sportsCompanion.team === "all" || p.side === sportsCompanion.team,
        )
        .map(
          (p) =>
            "<article><small>" +
            esc(p.label) +
            "</small><strong>" +
            esc(p.name) +
            "</strong><span>" +
            esc(p.value) +
            "</span></article>",
        )
        .join("") +
      "</div>";
    if (!items(data.rows).length && !items(data.leaders).length)
      html += "<p>No team statistics are available yet.</p>";
  } else {
    html +=
      '<ol class="companion-plays">' +
      items(data.plays)
        .map(
          (p) =>
            "<li><small>" +
            esc([p.period, p.clock].filter(Boolean).join(" · ")) +
            "</small><p>" +
            esc(p.text) +
            "</p></li>",
        )
        .join("") +
      "</ol>";
    if (!items(data.plays).length)
      html += "<p>No recent plays are available yet.</p>";
  }
  return html + "</section>";
}
function renderSportsCompanion() {
  if (state.view !== "player") return;
  const drawer = byId("player-sports-drawer");
  if (!drawer) return;
  const event = sportsCompanion.event;
  const sample = sportsCompanion.buffer.read(
    sportsEventStateID(event),
    sportsCompanionDelay(),
    performance.now(),
  );
  let overlay = byId("sports-score-overlay");
  if (!overlay) {
    overlay = document.createElement("div");
    overlay.id = "sports-score-overlay";
    overlay.className = "sports-score-overlay";
    document.querySelector(".playback-stage").appendChild(overlay);
  }
  const hidden =
    prefs().sportsSpoilersHidden || prefs().sportsPlayerSpoilersHidden;
  overlay.hidden = !event;
  const accessibleScore = event
    ? ["away", "home"]
        .map(
          (side) =>
            sportsTeamName(event[side]) +
            " " +
            (hidden || !sample
              ? "score withheld"
              : sample.data[side + "Score"] || "score unavailable"),
        )
        .join(", ") +
      ". " +
      (hidden
        ? "Scores hidden"
        : sample
          ? sample.data.statusText || ""
          : sportsCompanionDelay()
            ? "Delay warming up"
            : "Waiting for stats") +
      ". Open game companion"
    : "";
  const score = event
    ? '<button type="button" data-player-action="sports" aria-label="' +
      escapeHTML(accessibleScore) +
      '">' +
      ["away", "home"]
        .map(
          (side) =>
            "<span>" +
            renderSportsTeamLogo(event[side], "companion-team-logo") +
            escCompanionTeam(event[side]) +
            "<b>" +
            escapeHTML(
              hidden || !sample ? "—" : sample.data[side + "Score"] || "—",
            ) +
            "</b></span>",
        )
        .join("") +
      "<small>" +
      escapeHTML(
        hidden
          ? "Scores hidden"
          : sample
            ? sample.data.statusText || ""
            : sportsCompanionDelay()
              ? "Delay warming up"
              : "Waiting for stats",
      ) +
      "</small></button>"
    : "";
  if (overlay.innerHTML !== score) overlay.innerHTML = score;
  if (!state.playerSportsOpen) return;
  let root = byId("sports-companion-panel");
  if (!root) {
    root = document.createElement("div");
    root.id = "sports-companion-panel";
    const body = drawer.querySelector(".player-sports-body") || drawer;
    body.prepend(root);
  }
  const markup = sportsCompanionMarkup(event, sample);
  if (root.companionMarkup !== markup) {
    const active = root.contains(document.activeElement)
      ? document.activeElement
      : null;
    const selector =
      active && active.matches("[data-companion-delay]")
        ? "[data-companion-delay]"
        : active && active.dataset.companionTab
          ? '[data-companion-tab="' + active.dataset.companionTab + '"]'
          : active && active.dataset.companionTeam
            ? '[data-companion-team="' + active.dataset.companionTeam + '"]'
            : "";
    if (active && active.tagName === "SELECT") return;
    root.innerHTML = markup;
    root.companionMarkup = markup;
    if (selector && root.querySelector(selector))
      root.querySelector(selector).focus({ preventScroll: true });
  }
}
function escCompanionTeam(team) {
  return (
    "<span>" +
    escapeHTML((team && (team.abbreviation || team.name)) || "Team") +
    "</span>"
  );
}
document.addEventListener("click", function (event) {
  const button =
    event.target.closest &&
    event.target.closest("[data-companion-tab], [data-companion-team]");
  if (!button) return;
  if (button.hasAttribute("data-companion-tab"))
    sportsCompanion.tab = button.dataset.companionTab;
  if (button.hasAttribute("data-companion-team"))
    sportsCompanion.team = button.dataset.companionTeam;
  const root = byId("sports-companion-panel");
  if (root) {
    root.innerHTML = sportsCompanionMarkup(
      sportsCompanion.event,
      sportsCompanion.buffer.read(
        sportsEventStateID(sportsCompanion.event),
        sportsCompanionDelay(),
        performance.now(),
      ),
    );
    root
      .querySelector(
        "[data-companion-" +
          (button.hasAttribute("data-companion-tab")
            ? 'tab="' + sportsCompanion.tab
            : 'team="' + sportsCompanion.team) +
          '"]',
      )
      .focus();
  }
});
document.addEventListener("change", function (event) {
  if (!event.target.matches("[data-companion-delay]")) return;
  prefs().sportsStatsDelaySeconds = Number(event.target.value);
  savePrefs();
  renderPlayerSportsDrawer();
  renderSportsCompanion();
});
