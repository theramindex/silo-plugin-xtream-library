# Dispatcharr Silo Plugin

Dispatcharr-specific Silo plugin that runs as a Silo-hosted Live TV app.

> **Important:** This app does **not** manage IPTV streams. Dispatcharr Direct
> with an **Admin API Key** is the recommended full-feature source. Xtream Codes
> and M3U/XMLTV are supported as reduced-capability source modes.

The plugin owns the Silo presentation layer for Live TV. It does not ingest,
host, remux, transcode, or manage IPTV streams. The configured upstream remains
responsible for streams, channels, guide data, and output URLs; Dispatcharr also
owns recording behavior in Direct mode.

## Requirements and ownership boundaries

- **Dispatcharr Direct is preferred.** It is required for DVR scheduling and
  recordings. Xtream and M3U/XMLTV expose playback and guide capabilities only.
- **Dispatcharr manages streams.** Stream health, stream ordering, channel
  mapping, logos, guide IDs, output profiles, proxy/output URLs, and recording
  behavior are Dispatcharr responsibilities.
- **Silo presents and plays.** This plugin reads Dispatcharr-backed catalog data,
  renders the Silo Live TV UI, stores Silo/user presentation preferences, and
  redirects playback to Dispatcharr/provider-owned stream URLs.
- **No server-side transcoding.** The plugin does not invoke ffmpeg or perform
  GPU/CPU video conversion. Browser playback uses the bundled client-side
  HLS/mpegts players when the stream format is compatible.
- **ECM is recommended.** Use Enhanced Channel Manager (ECM) with Dispatcharr for
  channel/group curation, lineup cleanup, and operational channel management.

Native Jellyfin `/LiveTv/*` export can be added later if Silo exposes a
first-class Live TV provider capability.

## Supported source modes

- **Dispatcharr Direct: API Key** (required/recommended)
  - Dispatcharr URL
  - Admin API key from `System > Users > Edit User > API & XC`
  - Uses Dispatcharr REST APIs for catalog data and Dispatcharr proxy/output routes for playback
- **Dispatcharr Direct Connect** (legacy/manual login)
  - Dispatcharr URL
  - Username
  - Password
  - Uses Dispatcharr REST APIs for catalog data and Dispatcharr proxy/output routes for playback
- **Xtream Codes**
  - Base URL
  - Username
  - Password
  - Live TV, EPG, VOD, and series metadata when the provider exposes those APIs
- **M3U/XMLTV** fallback
  - M3U URL
  - EPG XML URL
  - Live TV and guide data only

## Current behavior

- Validates admin configuration for Dispatcharr, Xtream, and M3U/XMLTV modes
- Syncs Live TV channels, groups, guide data, VOD, and series through Dispatcharr REST
- Keeps Xtream VOD and series support available when Xtream Codes mode is selected
- Resolves playback targets fresh at play time
- Stores favorites, hidden categories, recent channels, custom groups, and
  playback preferences in Silo's per-user plugin configuration
- Keeps backend app/status responses user-neutral because the current HTTP
  route SDK does not provide a user identity to plugin handlers
- Coordinates manual, scheduled, channel-only, and guide-only refreshes through
  one serialized job state and preserves the last known good guide on failure
- Loads guide, VOD, and series separately from the lightweight app bootstrap;
  refresh polling uses only the status endpoint
- Keeps stale metadata visible when sync fails
- Exposes a plugin status route at `/dispatcharr/status`
- Exposes a Silo-hosted IPTV app:
  - `/dispatcharr` (navigable user app shown in Silo's Apps sidebar section)
  - `/dispatcharr/player`
  - `/dispatcharr/api/app`
  - `/dispatcharr/api/channels`
  - `/dispatcharr/api/guide`
  - `/dispatcharr/api/categories`
  - `/dispatcharr/api/vod`
  - `/dispatcharr/api/series`
  - `/dispatcharr/api/recordings` (`GET` lists Dispatcharr DVR rows, `POST` schedules an EPG program on Dispatcharr)
  - `/dispatcharr/api/timeshift/*` (shared Live Rewind leases, status, and administration)
  - `/dispatcharr/api/sports`
  - `/dispatcharr/api/events`
  - `/dispatcharr/api/status`
  - `/dispatcharr/stream?channel_id=...`
  - `/dispatcharr/vod/stream?item_id=...`
- Exposes host-scheduled task keys `dispatcharr-sync`,
  `dispatcharr-refresh-channels`, and `dispatcharr-refresh-epg`
- Shows Dispatcharr-managed DVR recordings in the plugin app when using Dispatcharr Direct Login or API Key mode. Like AerioTV's Dispatcharr server-side DVR flow, the plugin schedules recordings through Dispatcharr and opens Dispatcharr-owned playback URLs for completed or in-progress server recordings. It does not expose cancel/delete/stop controls.
  - Silo owns task cadence; the plugin cannot declare or change intervals
  - A `startup` trigger is useful after install/restart so channels and EPG populate immediately

## Live Rewind

- Live Rewind is opt-in and limited to Dispatcharr Direct MPEG-TS channels with
  H.264 video. Unsupported streams retain the normal direct playback path.
- The plugin copies MPEG-TS packets into keyframe-aligned HLS segments without
  transcoding. Segments live under
  `/var/lib/continuum/plugins/silo.ramindex.dispatcharr/timeshift`.
- One channel has one shared ring buffer. Browser leases keep independent
  playheads while viewers of the same channel reuse the ingest and disk cache.
- Administrators control the global cache budget, maximum rewind window,
  minimum free-space reserve, and maximum distinct buffered channels. Defaults
  are 5 GB, 30 minutes, 2 GB free, and 20 channels.
- Old segments are evicted first. If the hard disk budget cannot be maintained,
  the least recently used channel buffer is stopped and affected players fall
  back to direct live playback.
- Buffers expire after their viewer leases stop heartbeating and are removed
  after a short idle period. Multiview never starts rewind buffers.

## Profile and group organization

- Leave **Channel Profile** blank to ingest all Dispatcharr profiles and attach
  every profile membership to each channel. Enter one profile ID or exact name
  to scope both the lineup and browse tree to that profile.
- Profiles answer “which lineup?” and channel groups answer “what kind of
  channel?” In `Profile pipe + group pipe` mode the browser combines them as
  `Profile / Group`, for example `US TV / NY / Locals / Buffalo`.
- Each Silo user can choose which imported Dispatcharr profiles appear under
  **Live TV Settings**. Existing users default to all profiles; selecting a
  subset filters browse folders, guide programs, search, On Later, sports,
  events, favorites, custom-group pickers, and multiview suggestions. This is
  presentation filtering, not a server-side channel entitlement boundary.
- **Collapse duplicate virtual groups** removes only repeated adjacent path
  labels such as `International TV / International TV`; it never drops a
  channel. Sports/event channel matches also collapse repeated channel IDs.

## Sports-first player

- Administrators can opt into a score and channel drawer in the video player.
- The drawer reuses the existing sports feed, prioritizes the current channel,
  live games, and favorite teams, and refreshes only while it is open.
- Event and channel actions require a medium-or-better guide match.
  [Teamarr](https://github.com/Pharaoh-Labs/teamarr) and the
  [Dispatcharr Sports Event Autocreator](https://github.com/titooo7/dispatcharr-sports-event-autocreator)
  remain upstream organizers; the plugin does not create, rename, or delete
  Dispatcharr channels.

## Event-series guide detection

- The Events view recognizes selected guide-driven series such as golf,
  Formula 1, combat sports, and tennis without adding another schedule service.
- Event rules support inclusion keywords, exclusion keywords for replays and
  studio programming, and a configurable 15-360 minute coverage window.
- Same-day airings inside a coverage window share one event window and
  deduplicate channels. Later coverage remains a separate ordered window on the
  same event card while the legacy flat event fields remain available.
- This adapts the EPG matching approach used by
  [Alertle V2](https://github.com/Deekerman/Alertle-V2); alerts, notification
  endpoints, and its scheduler remain outside the plugin.

## v1 limitations

- Exactly one Dispatcharr-backed source
- EPG is required for setup in Xtream and M3U/XMLTV-compatible modes
- HTTP route requests do not include a Silo user identifier. Per-user state is
  therefore read and written by the browser through Silo's user config API,
  never through process-global plugin state.
- Source-mode changes reset cached channel/guide state before rebuilding
- Dispatcharr Direct does not silently fall back to Xtream Codes; Direct failures are surfaced in plugin health/status
- Silo host integration still needs real environment validation
- Native Jellyfin `/LiveTv/*` export is not available until Silo exposes a Live TV provider SDK/host capability
- Continuous backend proxying and transcoding are not enabled because the HTTP
  route SDK buffers each response. Live Rewind serves bounded, immutable HLS
  segments through that interface instead.
- HTTP routes and navigation are static manifest declarations. Settings cannot
  dynamically add/remove navigation entries.
- Scheduled-task cadence and retry policy are host-owned. The plugin only
  implements task execution.

## Silo host notes

Silo shows plugin Apps entries in the normal user sidebar when an HTTP route is declared with:

```json
{
  "access": "authenticated",
  "navigable": true,
    "navigation_label": "Live TV",
  "navigation_kind": "user"
}
```

This plugin declares `/dispatcharr` that way. Admin-only plugin pages can use `navigation_kind: "admin"` with `access: "admin"`.

Scheduled task cadence is not read from the plugin manifest by current Silo host builds. The plugin exposes `scheduled_task.v1` capabilities, but Silo stores active schedules in its task trigger table. Configure `plugin:<installation_id>:dispatcharr-sync` or `plugin:<installation_id>:dispatcharr-refresh-channels` with a slower catalog interval such as 24 hours, and configure `plugin:<installation_id>:dispatcharr-refresh-epg` with a shorter guide interval such as 6 hours. A startup trigger is useful for immediate cache hydration after restarts.

## Build

```bash
go build ./...
```

## Build Upload ZIP (Silo Admin Upload)

Build a Linux binary and package a Silo-compatible upload ZIP containing
`plugin` + `manifest.json`:

```bash
VERSION="0.0.0-local-$(git rev-parse --short HEAD)"
BIN="dispatcharr-${VERSION}-linux-amd64"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w -X main.buildVersion=${VERSION}" -o "dist/${BIN}" .
go run ./cmd/package-upload \
  -binary "dist/${BIN}" \
  -version "${VERSION}" \
  -goos linux \
  -goarch amd64 \
  -plugin-id silo.ramindex.dispatcharr
```

Upload the generated `dist/<binary>.silo-plugin.zip` file in Silo.

## GitLab CI builds

The repository includes `.gitlab-ci.yml` to run tests and produce versioned plugin binaries.

- Tagged builds (`vX.Y.Z`) use `X.Y.Z` as the plugin manifest version.
- Branch builds use a snapshot version `0.0.0-<shortsha>`.
- Artifacts include:
  - Linux binaries (`amd64`, `arm64`)
  - generated manifest JSON from each binary (`<binary>.manifest.json`)
  - SHA256 files (`<binary>.sha256`)

## GitHub Actions builds and releases

The repository also includes `.github/workflows/ci.yml` for GitHub-hosted runners.

- Runs vet, unit tests, the race detector, JavaScript syntax checks, and manifest
  validation on every pull request and push.
- Builds Linux binaries for `amd64` and `arm64`.
- Publishes a GitHub Release only for a `v*` tag after confirming the tag and
  manifest versions match. Branch builds remain downloadable workflow artifacts.
- When `CATALOG_PUSH_TOKEN` is configured, the tagged workflow updates and
  validates `theramindex/silo-plugins` from the published binary checksums, then
  commits the catalog change as `ramindex-ci <github@ramindex.org>`.

## Test

```bash
./scripts/verify-release.sh
```

For a tagged release candidate, also verify the intended version:

```bash
./scripts/verify-release.sh 0.3.7 --version-only
```

## Inspect manifest

```bash
go run . manifest
```

## License

`silo-plugin-dispatcharr` is licensed under `AGPL-3.0-or-later`. See `LICENSE`.
