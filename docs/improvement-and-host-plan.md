# Dispatcharr Improvement and Host Integration Plan

> Historical planning record. Current implementation and SDK boundaries are
> documented in `README.md` and `docs/sdk-fit-notes.md`; status tables below
> describe the state when this plan was written and are not a release checklist.

Consolidated plan for three parallel tracks:

1. **Plugin maintainability** — extract the embedded UI from `routes.go`
2. **Plugin design** — UI fixes (completed and remaining)
3. **Silo host / SDK** — the real integration gap and minimum host capability

This document supersedes scattered notes for day-to-day execution. Related background:

- `docs/sdk-fit-notes.md`
- `docs/continuum-host-gap.md`
- `docs/continuum-host-change-proposal.md`
- `PRODUCT.md`

---

## Executive summary

| Track | Problem | Who fixes it | Outcome |
|-------|---------|--------------|---------|
| Route extraction | 3,500-line `routes.go` mixes handlers, HTML, CSS, JS | **This plugin repo** | Reviewable code, lintable frontend, smaller diffs |
| UI polish | Theme drift, accessibility gaps, misleading UX | **This plugin repo** | App feels native inside Silo across themes |
| SDK / host gap | No Jellyfin-visible Live TV source; buffered HTTP routes | **Silo host** | Native `/LiveTv/*` or equivalent first-class source |

**Extracting routes does not fix the SDK issue.** The plugin already works around host limits by serving a self-contained Live TV app at `/dispatcharr` via `http_routes.v1`. That workaround stays until Silo adds a Live TV provider capability.

---

## Part 1 — Silo host / SDK gap

### What works today

The Silo SDK v1 surface used by this plugin:

| Primitive | Status | How Dispatcharr uses it |
|-----------|--------|-------------------------|
| `manifest.Load` + `runtime.Serve` | ✅ Supported | Embedded `manifest.json`, capability servers |
| `Runtime.Configure` | ✅ Supported | Grouped `ConfigEntry` for connection/admin settings |
| `global_config_schema` / `user_config_schema` | ✅ Partial | Admin forms, per-user `preferences` via Silo plugin settings |
| `http_routes.v1` | ✅ Supported | `/dispatcharr` app, `/dispatcharr/admin`, API routes, player |
| `scheduled_task.v1` | ✅ Supported | `dispatcharr-sync`, channel refresh, EPG refresh (plugin-side keys) |
| Masked secret fields | ✅ Supported | Password fields in admin schema |

### What does not work (host blockers)

| Limitation | Impact on Dispatcharr | Workaround in plugin |
|------------|----------------------|----------------------|
| **No Live TV provider capability** | Cannot register channels/guide as a Jellyfin-visible source | Full SPA at `/dispatcharr` |
| **Buffered HTTP route responses** | Cannot proxy/remux/transcode through plugin process | Redirect playback to upstream URLs |
| **Task cadence not in manifest** | Schedule lives in Silo `task_triggers` table, not plugin manifest | Document manual trigger setup in README |
| **In-memory preference fallback** | Prefs lost on plugin restart unless Silo profile/localStorage saves | Silo plugin settings API + `localStorage` merge |

### Minimum host capability proposal

Silo needs a new capability alongside `metadata_provider.v1` — not a replacement for it.

**Proposed name:** `livetv_provider.v1` (or `media_source.v1` with Live TV specialization)

#### Capability contract (minimal)

```protobuf
// Conceptual — not implemented in Silo today
service LiveTVProvider {
  // Host calls on configure / scheduled refresh
  rpc SyncCatalog(SyncCatalogRequest) returns (SyncCatalogResponse);

  // Jellyfin compatibility layer calls at play time
  rpc ResolvePlayback(ResolvePlaybackRequest) returns (ResolvePlaybackResponse);

  // Optional: host polls or plugin pushes health
  rpc GetSourceHealth(GetSourceHealthRequest) returns (SourceHealthResponse);
}
```

#### Data the plugin would expose to the host

| Entity | Fields (minimum) | Notes |
|--------|------------------|-------|
| **Source** | `id`, `name`, `mode` (direct/xtream/m3u), `last_sync_unix`, `health_status` | One source per plugin install (v1) |
| **Channel** | `id`, `name`, `number`, `logo_url`, `category_id`, `stream_url_template` | Order preserved from upstream |
| **Category** | `id`, `name`, `parent_id` | Empty categories filtered |
| **Program** | `id`, `channel_id`, `title`, `start_unix`, `end_unix`, `description` | EPG grid |
| **Recording** | `id`, `channel_id`, `title`, `status`, `playback_url` | Direct mode only |

#### Host-side deliverables

1. **Capability registration** — Silo discovers `livetv_provider.v1` at plugin install, same as `http_routes.v1`
2. **Catalog ingestion** — Host stores or virtualizes plugin-provided channels/programs (does not require copying all EPG into Postgres on every sync; virtual read-through is acceptable for v1)
3. **Jellyfin bridge** — Map ingested catalog to `/LiveTv/Channels`, `/LiveTv/Programs`, `/LiveTv/Recordings` (or Silo-native equivalents already consumed by clients)
4. **Playback path** — Host resolves fresh stream URL at play time (plugin `ResolvePlayback`), returns redirect or host-managed proxy if buffering limitation is lifted
5. **Navigation** — Optional: host sidebar entry from provider metadata instead of only `http_routes` navigable flag

#### Data flow (target state)

```
Dispatcharr upstream
        │
        ▼
Plugin sync (existing internal/app)
        │
        ▼
Plugin livetv_provider.v1 adapter  ──►  Silo host catalog store
        │                                        │
        │                                        ▼
        │                              Jellyfin compatibility API
        │                                        │
        ▼                                        ▼
http_routes.v1 (rich UI — optional)     Native client Live TV surfaces
```

The existing `/dispatcharr` SPA remains valuable as the **rich guide/player/admin UI** even after host ingestion exists. Native export and embedded app are complementary, not either/or.

#### Phased host rollout

| Phase | Host work | Plugin work | User-visible result |
|-------|-----------|-------------|---------------------|
| **H0 (now)** | None | `http_routes.v1` SPA | Live TV works inside plugin app |
| **H1** | Define proto + stub capability | Implement adapter behind feature flag | No user change; contract validated |
| **H2** | Jellyfin bridge for channels + guide | Sync pushes catalog to host | Clients see channels in native Live TV |
| **H3** | Playback resolution through host | Remove redirect-only limitation if buffer fixed | Unified playback in all Silo clients |
| **H4** | Per-user prefs in host user config | Drop in-memory fallback | Favorites/order survive restarts without localStorage |

#### Explicit non-goals for host v1

- Multiple Dispatcharr sources per install
- In-plugin transcoding/remux (unless HTTP route streaming is unbuffered)
- Replacing `metadata_provider.v1` for TMDB enrichment

---

## Part 2 — Route extraction plan

### Problem

`internal/plugin/routes.go` (~3,500 lines) contains:

| Lines (approx) | Content |
|----------------|---------|
| 1–986 | Go HTTP handlers, payload types, stream resolution, admin settings |
| 987–1328 | Embedded CSS (~330 lines) |
| 1329–1355 | HTML shell |
| 1356–3493 | Embedded JS (~2,100 lines) |

This blocks ESLint, Prettier, CSS review, and causes painful merge conflicts.

### Target layout

```
internal/plugin/
├── routes.go              # HTTPRoutesServer, Handle(), route dispatch only
├── routes_handlers.go     # API handlers (app, guide, recordings, prefs, watch, admin)
├── routes_playback.go     # Stream URL resolution, redirect helpers
├── routes_page.go         # playerPageHTML(), template substitution, admin topbar
├── routes_types.go        # AppPayload, ChannelsPayload, etc.
├── routes_capabilities.go # appCapabilities, dvrEnabledForSource
├── admin_settings_store.go
├── tasks.go
├── health.go
└── ui/
    ├── embed.go           //go:embed directives
    ├── page.html          # Shell template with __PLACEHOLDER__ tokens
    ├── styles.css         # All app CSS
    ├── app.js             # State, render, navigation, guide, settings, admin
    ├── player.js          # Playback, HLS/mpegts, player chrome (optional split)
    └── assets/
        ├── hls.min.js
        └── mpegts.min.js
```

### Placeholder tokens (unchanged contract)

The HTML template keeps existing substitution markers:

| Token | Set by |
|-------|--------|
| `__SILO_THEME__` | `sanitizeThemeSlug()` |
| `__APP_TITLE__` | manifest / route |
| `__PLAYER_LIBRARIES__` | `playerLibrariesHTML()` |
| `__ROUTE_CLASS__` | player / admin / default |
| `__ADMIN_SETTINGS_TOKEN__` | per-process admin token |

### Migration steps (PR sequence)

#### PR 1 — Extract CSS (no behavior change)

1. Cut `<style>...</style>` body to `ui/styles.css`
2. Add `//go:embed ui/styles.css` in `ui/embed.go`
3. In `routes_page.go`, inject `<style>` block from embedded file
4. Run `go test ./internal/plugin/...` — substring tests for CSS classes must still pass

#### PR 2 — Extract HTML shell

1. Move `<!doctype html>...<script>` wrapper and static markup to `ui/page.html`
2. Leave `<script>` as external reference: `<script src="" id="app-script">` replaced at serve time, **or** keep inline script placeholder `__APP_SCRIPT__`
3. Verify `/dispatcharr`, `/dispatcharr/player`, `/dispatcharr/admin` render identically

#### PR 3 — Extract JavaScript

1. Move `<script>` body to `ui/app.js`
2. Keep Node `vm` smoke tests in `routes_test.go` — update extractor to read from `ui/app.js` instead of parsing `routes.go`
3. Optional: split `player.js` if playback section is cleanly separable (~lines 2400–3300)

#### PR 4 — Split Go handlers

1. Move handler methods to `routes_handlers.go`, playback to `routes_playback.go`, types to `routes_types.go`
2. `routes.go` retains only `Handle()` switch and constructor
3. Target: no file >800 lines

### Testing requirements per PR

- `go test ./...` green
- `routes_test.go` JS extraction tests updated to new paths
- Manual smoke: Home, Guide, Favorites, Recordings (Direct), Player, Admin settings
- No change to manifest routes or API response shapes

### Optional follow-up (not blocking)

- Add `esbuild` or `bun build` step in CI to minify `app.js` (output still `go:embed`'d)
- Add ESLint for `ui/app.js` in CI
- Source maps for debugging (dev builds only)

---

## Part 3 — UI design fixes

Design intent from `PRODUCT.md`: serious TV app embedded in Silo — focused, compact, media-native.

### Completed (current branch)

| Fix | Change |
|-----|--------|
| Theme tokenization | `--player-bg`, `--poster-fallback`, `--tooltip-bg`, `--focus-ring`; sidebar uses `var(--rail)` gradient |
| Light theme compatibility | Breadcrumb hover, EPG channel text, poster fallback use tokens not hardcoded `white` / `#b19398` |
| Honest home section | Renamed "Continue watching" → **"Recently watched"**; removed fake 62% progress bar |
| Accessibility | Global `focus-visible` rings; `prefers-reduced-motion`; search `aria-label` |
| Touch EPG | Schedule buttons visible under `@media (hover: none)` |
| Favorites HTML | `.favorite-card` wrapper — no nested interactive elements |
| `favoriteOrder` persistence | Added to Go `Preferences`, store clone, `SetFavorite`, config schema |
| ECM default | Aligned Go + JS to `ecmEnabled: false` |

### Remaining UI work (plugin repo)

#### Priority A — polish before next release

| Item | Detail | Files |
|------|--------|-------|
| Mobile nav tab bar | At `≤900px`, convert vertical `.nav` to horizontal scroll tabs | `ui/styles.css` |
| Empty states | Replace generic copy with connection-aware messages ("Syncing…", "Check admin connection") | `app.js` render functions |
| EPG keyboard path | Channel name tooltip via `aria-describedby` + visible label on focus, not only `::after` hover | CSS + JS |
| Player focus on idle | When chrome hides (`is-idle`), ensure Escape or first tap restores controls and focus trap | `app.js` |
| Recording cards | Match player polish — status badge hierarchy, primary action emphasis | CSS |

#### Priority B — after route extraction

| Item | Detail |
|------|--------|
| Typographic scale | CSS tokens: `--text-sm`, `--text-base`, `--text-lg`, `--text-display`; cap weights at 700–800 for body labels |
| Incremental EPG render | Reduce full `innerHTML` rewrites on guide search/filter |
| `aria-live` regions | Guide refresh and recording schedule toasts announce to screen readers |
| Admin alias table | Responsive card layout on mobile (partially done); inline validation feedback |

#### Priority C — nice to have

| Item | Detail |
|------|--------|
| Home guide preview loading | Skeleton rows while EPG lazy-loads |
| Category tile icons | Optional genre/category glyphs for scanability |
| Reduced chrome motion | Player idle hide uses opacity only (already mostly true); verify no layout shift |

### Design acceptance checklist (release gate)

- [ ] Sidebar and panels match Silo theme in `midnight-cinema` and `light` / `daylight`
- [ ] All interactive elements show visible focus ring when tabbing
- [ ] No decorative progress/placeholder data that implies real state
- [ ] Recordings nav hidden for Xtream and M3U/XMLTV (Direct only)
- [ ] Guide schedule buttons usable on iPad / touch without hover
- [ ] `prefers-reduced-motion: reduce` disables spin and slide transitions

---

## Part 4 — Plugin-side SDK alignment (no host changes)

These improve correctness within the current SDK contract.

| Item | Status | Action |
|------|--------|--------|
| Declare `dispatcharr-refresh-channels` in manifest | ❌ Missing | Add second and third `scheduled_task.v1` capability entries |
| Declare `dispatcharr-refresh-epg` in manifest | ❌ Missing | Same |
| README navigation label | ⚠️ Drift | README says "IPTV", manifest says "Live TV" — align to **Live TV** |
| `Configure` duplication in `main.go` | ⚠️ Tech debt | Extract per-key normalizer functions |
| Dispatcharr client test coverage | ⚠️ ~5% | Add httptest coverage for auth, recordings, pagination |
| API key source mode reporting | ⚠️ | Ensure sync writes actual `source_mode` not always `direct_login` |

---

## Part 5 — Implementation roadmap

### Phase 1 — Ship current UI fixes ✅

- Theme tokens, recently watched, focus rings, reduced motion, favoriteOrder, ECM default
- **Gate:** `go test ./...`

### Phase 2 — Route extraction ✅

- `internal/plugin/ui/page.html`, `styles.css`, `app.js`
- `internal/plugin/ui_assets.go` assembles template at init
- `routes.go` reduced from ~3,500 → ~986 lines (handlers only)
- `routes_test.go` reads `ui/app.js` for Node smoke tests

### Phase 3 — Remaining UI polish (partial ✅)

- ✅ Mobile nav horizontal tab bar at `≤900px`
- ✅ Connection-aware empty states (`emptyStateHTML`, `catalogEmptyDetail`)
- ⏳ EPG keyboard path, player idle focus, recording card polish

### Phase 4 — Plugin SDK alignment (partial ✅)

- ✅ README navigation label aligned to **Live TV**
- ⏳ Manifest refresh tasks stay off plugin card by design (`TestManifestKeepsInternalRefreshTasksOffPluginCard`); channel/EPG refresh keys remain configurable via Silo task triggers
- ⏳ Source mode fix, Dispatcharr client tests, `main.go` Configure dedup

### Phase 5 — Silo host capability (separate repo)

- Proto definition, host ingestion, Jellyfin bridge
- **Blocked on:** Silo team prioritization; document only until then

**Estimated effort:** multi-week host project, not plugin-only

---

## Appendix A — File reference map (current)

| Concern | Primary file |
|---------|--------------|
| HTTP routing | `internal/plugin/routes.go` |
| Embedded UI | `routes.go` `playerPageHTMLTemplate` |
| Sync / upstream | `internal/app/sync.go` |
| Preferences model | `internal/cache/preferences.go` |
| Admin ECM defaults | `internal/plugin/admin_settings_store.go` |
| Manifest / capabilities | `manifest.json` |
| SDK bootstrap | `main.go` |
| Route tests + JS smoke | `internal/plugin/routes_test.go` |

## Appendix B — Decision log

| Decision | Rationale |
|----------|-----------|
| Keep `/dispatcharr` SPA after host Live TV export | Rich guide/admin/player UX exceeds what Jellyfin Live TV API typically exposes |
| Redirect playback until buffer limitation fixed | Documented SDK constraint; proxy mode flag exists but `BackendProxySupported: false` |
| DVR only for Direct mode | Product rule; Dispatcharr owns recording lifecycle |
| `favoriteOrder` in server prefs | Favorites reorder must survive device changes without relying on localStorage alone |
| ECM default `false` | Opt-in admin feature; matches config schema default |

---

## Next action

Recommended immediate sequence:

1. **Merge Phase 1 UI fixes** (already implemented locally)
2. **Open PR 2.1** — extract CSS to `internal/plugin/ui/styles.css`
3. **Open host issue** — paste Part 1 capability proposal into Silo tracker for `livetv_provider.v1`
4. **Catalog + deploy** — bump version, validate, push to `ramindex-continuum-plugins`
