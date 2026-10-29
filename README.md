# XC for Silo

Standalone Silo IPTV plugin with Xtream Codes as the primary source and
M3U/XMLTV as a reduced-feature secondary source.

## Features

- Live TV channel browsing, guide, search, favorites, and in-plugin playback
- Xtream VOD, series details, and episode playback
- Provider-supported Catch-up replay for eligible guide programs
- Four-tile Multiview with one active audio tile
- Manual, startup, and host-scheduled catalog refreshes
- Optional per-source alternate XMLTV EPG with fill-missing and prefer-alternate policies
- Per-provider playback credential pools with sticky session leasing and configurable account limits
- M3U/XMLTV Live TV and guide support

## Source modes

### Xtream Codes

Configure provider accounts in **XC Admin → Sources**. This mode supports the
full catalog: Live TV, guide, VOD, series, episodes, and provider Catch-up.

Each source can optionally use an alternate XMLTV EPG URL. **Fill missing**
keeps overlapping Xtream guide rows and fills actual schedule gaps;
**Prefer alternate** replaces guide rows only for channels matched by the
alternate feed. Matching uses exact guide IDs first and unique normalized
channel names second. Ambiguous names remain unmatched, `.xml.gz` feeds are
supported, and a failed alternate feed leaves the provider guide intact.

### M3U/XMLTV

Configure an M3U playlist URL and XMLTV guide URL. This mode supports Live TV,
guide browsing, and playback only. VOD, series, episodes, and Catch-up are not
advertised.

## Playback and privacy

The app serves catalog data without provider URLs or credentials. Embedded live playback uses the Ramindex Silo fork's continuous loopback relay for MPEG-TS and HLS. Browser media routes are authenticated and bound to the Silo user/profile; nested HLS playlists, keys, and segments stay on Silo's origin.

- **Guarded source fetches.** Only HTTP/HTTPS is allowed, with dial-time address checks, bounded redirects, and no environment proxies. Explicit administrator-configured source hosts may be LAN addresses. Per-user sources retain the public-address-only policy.
- **Compatible playback.** One codec failure can trigger GPU conversion to H.264/AAC HLS; a manual player option is also available. It requires host FFmpeg and `/dev/dri/renderD128`, caps output at 1080p/30 fps, and shares two GPU encoder slots with Dispatcharr and HDHomeRun. Decoding and scaling use the CPU. There is no software encoder fallback.
- **Live Rewind.** The admin enable switch, disk budget, free-space reserve, window, and buffer-count settings govern a temporary viewer-owned cache. Supported H.264 MPEG-TS is copied into segments; incompatible rewind inputs can use a GPU history. VOD and provider Catch-up remain separate.
- **Station alternatives.** Map equivalent channel IDs to the same station key in admin settings. Priority selects the preferred feed; startup can try another available account or explicitly mapped source before bytes are sent. Failed accounts cool down briefly. There is no seamless midstream switching.
- **Legacy and file playback.** `allowDirectProviderURLs` still governs non-HLS VOD, episodes, Catch-up, and legacy direct stream routes. When enabled, those routes can expose account credentials to the browser through redirects; when disabled, credentialed non-HLS files return `stream_requires_hls` (422). Embedded live channels use the private relay with either setting.

Shared playback uses SDK v0.30.0. Leaving playback releases the source and encoder; abandoned shared leases expire after 75 seconds. A browser watch reservation is adopted once, so starting its managed relay does not consume a second provider connection.

Each provider keeps one primary account for catalog and guide refreshes. XC
Admin can add compatible sub-accounts for playback and set the provider-issued
stream limit for each account. A watch session stays on one account, the
least-used available account is selected for new playback, and the lease is
released when playback stops or its heartbeat expires. Each Multiview tile
uses one lease. These counters cover XC for Silo sessions only; they cannot see
connections created outside this plugin.

## Guide refreshes

- Sports preparation reuses an indexed guide cache, returns within the route deadline, and refreshes slow guide fallbacks in the background.

- XMLTV guides (M3U/XMLTV guide URL, Xtream `xmltv.php`, and alternate EPG
  feeds, plain or gzip) are decoded incrementally from the response body,
  capped at 256 MiB, instead of being buffered in full.
- The last known good guide is kept when a refresh fails, and also when a
  refresh returns less than half of the previous still-current program count
  or channel coverage; that is treated as an upstream glitch and
  `status.epgWarning` explains why. Source-mode or source-configuration
  changes always accept the new guide.

## User identity and admin-only actions

The Silo host's plugin HTTP proxy forwards only an allowlist of client headers
(`accept`, `accept-language`, `content-type`, `if-none-match`,
`if-modified-since`, `range`, `user-agent`, `origin`, `referer`) and stamps its
own identity headers on authenticated requests: `X-Silo-User-Id`,
`X-Silo-User-Role` (`admin` or `user`), `X-Silo-User-Name`, and
`X-Silo-Profile-Name`. A browser cannot inject or override them, so the plugin
trusts `X-Silo-User-Role` for authorization.

- `POST /dispatcharr/api/refresh` (force refresh) and
  `POST /dispatcharr/api/refresh-channels` are declared `"access": "admin"` in
  the manifest and are also enforced in the handlers, which return
  `403 {"ok":false,"error":"admin_required",...}` for non-admins.
- XC Admin settings and source management already require the admin role.
- `GET /dispatcharr/api/app` includes `"isAdmin": true|false` so the UI can
  hide admin-only controls.
- Outside per-user providers (below), the plugin does not key server-side
  state by `X-Silo-User-Id`; per-user preferences remain browser-managed
  through Silo's user config API.

## Per-user providers

XC Admin → Organization → **Users add their own providers** switches XC from
one shared source registry to a private registry per Silo user. It is off by
default.

- Each user adds their own Xtream Codes server and credentials under
  Settings → My providers (`/dispatcharr/api/my-sources`, `my-refresh`) and
  gets their own catalog, guide, snapshot, and refresh jobs. A user without a
  provider gets no channels; the admin's sources are never a fallback.
- Requests are routed by the host-stamped `X-Silo-User-Id`. XC Admin and its
  APIs stay on the shared server, which never opens a user's registry, so an
  admin cannot list or use anyone's provider. An admin's own Live TV is a
  separate user like any other.
- Passwords are write-only: responses carry `passwordConfigured` only. Each
  registry is stored AES-256-GCM sealed at `users/<id>/sources.sealed` under
  the plugin state directory, with the key in `users/.sources.key` (0600).
  This keeps credentials out of anything XC serves or logs; it cannot hide
  them from someone with root on the host, because XC must decrypt them to
  open a stream.
- Every request made for a user's provider (catalog, XMLTV, connection tests,
  HLS relay, images) uses a restricted HTTP client that refuses loopback,
  private, link-local, and multicast addresses after DNS resolution, so a user
  cannot reach the server's LAN through XC.
- `livetv/v1/info` reports `"perUserLineup": true`, telling Silo's Jellyfin
  bridge not to share cached lineups or guides between users.
- Scheduled refreshes run once per user with saved providers; one user's
  failing provider does not stop the others.

## Guide API

`GET /dispatcharr/api/guide` returns `{"programs":[...]}` ordered by start time, then program ID.
Schedules are persisted in an indexed local SQLite database. Channel/time
filters read only the requested window. Optional query parameters:

| Parameter | Meaning |
| --- | --- |
| `channel_id` | Single channel ID (legacy). |
| `channel_ids` | Comma-separated channel IDs; merged with `channel_id`. Max 2000. |
| `start` | Unix seconds. With `end`, returns programs overlapping `[start, end)`. |
| `end` | Unix seconds. Either bound may be omitted to leave that side open. |

With no parameters the full guide is returned (backward compatible). A program
without an end time matches when its start lies inside the window. Invalid
numbers, more than 2000 channels, or `start >= end` return `400`.

### Local guide storage

The private catalog directory contains `catalog-snapshot.sqlite`. The database
uses WAL mode and must be on a local filesystem. Each refresh commits the
lineup metadata and programme replacement together. Readers continue using
the last committed guide while the import runs. Health-only updates do not
rewrite programme rows.

Existing `catalog-snapshot.json` files are served while a background import
migrates them. The JSON file is retained as a rollback copy; after migration,
SQLite takes precedence and the JSON copy is no longer updated. Startup reads
catalog metadata without deserializing the complete schedule. Normal guide
requests fetch channel/time windows; the unfiltered full-guide export remains
available. A cache read failure returns `503`, rather than an empty successful
guide. Sports preparation keeps its derived matching index in memory.

Each account keeps its own database beside its existing sealed source registry;
lineups and guide rows are not shared between accounts.

## Out of scope

- DVR and recording management
- Local rolling-buffer timeshift
- Dispatcharr integration and Dispatcharr-specific sports/event features
- Plugin-owned databases, migrations, or persisted catalog tables

## Build and test

```bash
go test ./...
go vet ./...
./scripts/verify-release.sh
```

Tagged `v*` releases build Linux amd64/arm64 and Darwin arm64 artifacts in
GitHub Actions. Catalog publication to `theramindex/silo-plugins` is a manual,
checksum-verified step after a real release.
