const { chromium } = require(process.env.PLAYWRIGHT_PATH || "playwright");
const fs = require("fs");
const assert = require("assert/strict");
const root = require("node:path").resolve(__dirname, "..");
(async () => {
  const browser = await chromium.launch();
  try {
    for (const repo of [require("node:path").basename(root)]) {
      const page = await browser.newPage({
        viewport: { width: 1440, height: 1000 },
      });
      await page.clock.install();
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      const source = fs.readFileSync(
        root + "/internal/plugin/ui/sports_companion.js",
        "utf8",
      );
      const css = fs.readFileSync(
        root + "/internal/plugin/ui/styles.css",
        "utf8",
      );
      const snapshot = {
        available: true,
        homeScore: "31",
        awayScore: "28",
        statusText: "Q2 · 4:20",
        rows: [{ label: "Rebounds", home: "12", away: "9" }],
        boxScores: [
          {
            side: "home",
            category: "Players",
            columns: ["MIN", "PTS", "REB", "AST", "STL"],
            players: [
              {
                name: "Jalen Brunson",
                jersey: "11",
                position: "G",
                starter: true,
                stats: ["18", "12", "2", "4", "0"],
              },
            ],
          },
          {
            side: "away",
            category: "Players",
            columns: ["MIN", "PTS", "REB", "AST", "STL"],
            players: [
              {
                name: "Jayson Tatum",
                jersey: "0",
                position: "F",
                stats: ["19", "10", "5", "1", "0"],
              },
            ],
          },
        ],
        leaders: [
          { side: "home", label: "Points", name: "Jalen Brunson", value: "12" },
        ],
        plays: [
          {
            period: "Q2",
            clock: "4:20",
            text: "Jalen Brunson makes a jump shot.",
          },
        ],
      };
      await page.route("http://sports.test/**", (route) =>
        route.request().url().endsWith("/api/sports")
          ? route.fulfill({json: {updatedAtUnix: Math.floor(Date.now()/1000), events: [{
              id: "game", live: true, home: {name: "New York Knicks"}, away: {name: "Boston Celtics"}, channels: [{id: "knicks", score: 90}]
            }]}})
          : route.request().url().includes("/api/sports?")
          ? route.fulfill({ json: snapshot })
          : route.fulfill({
              contentType: "text/html",
              body:
                "<html><head><style>" +
                css +
                '</style></head><body><section class="playback-shell sports-enabled sports-open" style="height:100dvh;width:100vw"><div class="playback-stage" style="height:100%;background:#090b10"><div class="player-sports-drawer open" id="player-sports-drawer"><div class="player-sports-head">Live sports<button aria-label="Close">Close</button></div><div class="player-sports-body"></div></div></div></section></body></html>',
            }),
      );
      await page.goto("http://sports.test/");
      await page.evaluate(() => {
        window.state = {
          view: "player",
          currentChannel: { id: "knicks" },
          playerSportsOpen: true,
        };
        window.fixturePrefs = { sportsStatsDelaySeconds: 0 };
        window.items = (v) => (Array.isArray(v) ? v : []);
        window.prefs = () => fixturePrefs;
        window.byId = (id) => document.getElementById(id);
        window.escapeHTML = (v) =>
          String(v ?? "").replace(
            /[&<>"']/g,
            (c) =>
              ({
                "&": "&amp;",
                "<": "&lt;",
                ">": "&gt;",
                '"': "&quot;",
                "'": "&#39;",
              })[c],
          );
        window.eventFixture = {
          id: "game",
          home: { name: "New York Knicks", abbreviation: "NYK" },
          away: { name: "Boston Celtics", abbreviation: "BOS" },
        };
        window.playerSportsEvents = () => [eventFixture];
        window.playerSportsCurrentEvent = (e) => e[0];
        window.playerSportsChannelMatches = () => true;
        window.sportsEventStateID = (e) => e?.id || "";
        window.sportsFirstPlayerActive = () => true;
        window.sportsTeamName = (t) => t?.name || "Team";
        window.renderSportsTeamLogo = () => "";
        window.route = (p) => "http://sports.test" + p;
        window.coreFetch = (url, options) => fetch(url, options);
        window.savePrefs = () => {};
        window.renderPlayerSportsDrawer = () =>
          document.getElementById("sports-companion-panel")?.remove();
      });
      await page.evaluate(() => {
        const video = document.createElement("video"); video.id = "player";
        Object.defineProperties(video, {paused: {get: () => false}, readyState: {get: () => 4}});
        document.querySelector(".playback-stage").prepend(video);
        window.sportsFavoriteTeamMatches = team => team?.name === "New York Knicks";
        window.playerSportsEventChannel = event => event.channels?.[0];
        window.playedChannels = [];
        window.playChannel = channel => { playedChannels.push(channel.id); state.currentChannel = channel; };
      });
      await page.addScriptTag({content: fs.readFileSync(root + "/internal/plugin/ui/sports_assist.js", "utf8")});
      await page.addScriptTag({ content: source });
      await page.evaluate(() => startSportsCompanion());
      await page.getByRole("rowheader", { name: /Jalen Brunson/ }).waitFor();
      assert.match(
        await page
          .locator("#sports-score-overlay button")
          .getAttribute("aria-label"),
        /Boston Celtics 28/,
      );
      await page.screenshot({
        path: "/tmp/" + repo + "-companion-desktop.png",
      });
      await page
        .getByRole("button", { name: "Statistics", exact: true })
        .click();
      await page.getByRole("cell", { name: "12", exact: true }).waitFor();
      await page
        .getByRole("button", { name: "Play-by-play", exact: true })
        .click();
      await page.getByText("Jalen Brunson makes a jump shot.").waitFor();
      await page
        .getByRole("button", { name: "Box score", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Boston Celtics", exact: true })
        .click();
      assert.equal(
        await page.getByRole("rowheader", { name: /Jalen Brunson/ }).count(),
        0,
      );
      await page
        .getByRole("button", { name: "Both teams", exact: true })
        .click();
      await page.setViewportSize({ width: 390, height: 844 });
      await page.screenshot({ path: "/tmp/" + repo + "-companion-mobile.png" });
      await page.locator("[data-companion-delay]").selectOption("60");
      await page.getByText("Building a 60s delay…").waitFor();
      const scoreName = await page
        .locator("#sports-score-overlay button")
        .getAttribute("aria-label");
      assert.match(scoreName, /score withheld/);
      assert.doesNotMatch(scoreName, /28|31/);
      assert.equal(
        await page.locator("#sports-score-overlay b").first().textContent(),
        "—",
      );
      assert.equal(
        await page.getByRole("rowheader", { name: /Jalen Brunson/ }).count(),
        0,
      );
      assert.equal(await page.evaluate(() => fixturePrefs.sportsStatsDelaySeconds), 0);
      assert.equal(await page.evaluate(() => fixturePrefs.sportsChannelDelays.knicks), 60);
      await page.evaluate(() => { state.currentChannel = {id: "other"}; startSportsCompanion(); });
      await page.getByRole("rowheader", {name: /Jalen Brunson/}).waitFor();
      assert.equal(await page.locator("[data-companion-delay]").inputValue(), "default");
      await page.locator("[data-companion-delay]").selectOption("120");
      await page.evaluate(() => { state.currentChannel = {id: "knicks"}; startSportsCompanion(); });
      await page.getByText("Building a 60s delay…").waitFor();
      await page.locator("[data-companion-delay]").selectOption("default");
      await page.getByRole("rowheader", {name: /Jalen Brunson/}).waitFor();
      assert.deepEqual(await page.evaluate(() => fixturePrefs.sportsChannelDelays), {other: 120});
      await page.evaluate(() => {
        state.currentChannel = {id: "news"};
        state.app = {channels: [{id: "news"}, {id: "knicks"}]};
      });
      await page.getByRole("combobox", {name: "Game switching"}).selectOption("auto");
      await page.locator("#sports-assist-banner").getByText(/Switching to/).waitFor();
      await page.locator("#sports-assist-banner").getByRole("button", {name: "Cancel", exact: true}).click();
      await page.clock.runFor(6000);
      assert.deepEqual(await page.evaluate(() => playedChannels), []);
      await page.getByRole("combobox", {name: "Game switching"}).selectOption("off");
      await page.getByRole("combobox", {name: "Game switching"}).selectOption("auto");
      await page.locator("#sports-assist-banner").getByText(/Switching to/).waitFor();
      await page.locator("#sports-assist-banner").getByRole("button", {name: "Stay here", exact: true}).click();
      await page.clock.runFor(6000);
      assert.deepEqual(await page.evaluate(() => playedChannels), []);
      await page.getByRole("button", {name: "Unlock channel", exact: true}).click();
      await page.getByRole("combobox", {name: "Game switching"}).selectOption("off");
      await page.getByRole("combobox", {name: "Game switching"}).selectOption("auto");
      await page.locator("#sports-assist-banner").getByText(/Switching to/).waitFor();
      await page.clock.runFor(6000);
      assert.deepEqual(await page.evaluate(() => playedChannels), ["knicks"]);
      await page.evaluate(() => stopSportsCompanion());
      assert.equal(await page.evaluate(() => sportsCompanion.timer), null);
      assert.equal(await page.locator("#sports-score-overlay").count(), 0);
      assert.deepEqual(errors, []);
      await page.close();
      console.log(
        repo +
          ": PASS tabs, team filter, delayed warmup, hidden fresh scores, mobile layout and timer cleanup",
      );
    }
  } finally {
    await browser.close();
  }
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
