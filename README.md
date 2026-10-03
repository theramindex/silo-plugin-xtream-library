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

The app serves catalog data without provider URLs or credentials. Playback is
resolved only through authenticated Silo plugin gateway routes. The Silo host
stamps the caller's identity on every route request (see "User identity and
admin-only actions"), but the plugin route SDK has no streaming response body,
so this plugin does not claim per-user upstream credentials, server
transcoding, or a general media proxy.

- **HLS is relayed.** Xtream HLS live channels (the default `m3u8` live
  format), and any other HLS playback URL that embeds provider credentials
  (Xtream-style `/USER/PASS/` paths, URL user-info, password/token query
  parameters, or a configured account password), are fetched by the plugin.
  The browser only receives manifests whose segment, key, and rendition URIs
  are rewritten to short-lived AES-GCM relay tokens.
- **Relay fetches are SSRF-guarded.** Manifest references are untrusted, so
  relay requests refuse loopback, private, link-local, unspecified, and
  multicast destinations at dial time (after DNS resolution, so rebinding is
  caught), allow only `http`/`https`, follow at most 3 redirects under the same
  policy, and ignore environment proxies. The configured provider hosts (every
  source, which covers its playback sub-accounts) and the host of the stream
  the relay chain started from (sealed into each token) may be LAN addresses.
- **MPEG-TS, VOD, episodes, and Catch-up depend on an admin setting.**
  Continuous MPEG-TS (`ts` live format), movie/episode files, and Catch-up
  (`timeshift/...ts`) cannot pass through the buffered route SDK. XC Admin →
  Settings → **Allow direct provider URLs** (`allowDirectProviderURLs`,
  default **on**) decides what happens to them:
  - **On** (default, keeps existing installs playing): the route redirects to
    the provider URL, so the viewer's browser receives the playback account's
    credentials.
  - **Off**: credentialed non-HLS streams are refused with
    `422 {"ok":false,"code":"stream_requires_hls","error":"..."}` and the
    player shows that message. HLS is still relayed and credential-free URLs
    still redirect.

  The trade-off: leaving it on exposes provider credentials to every viewer
  who plays those streams; turning it off blocks TS live, VOD, series, and
  Catch-up unless the provider serves HLS. Use the `m3u8` live format to keep
  live credentials out of the browser either way.

Each provider keeps one primary account for catalog and guide refreshes. XC
Admin can add compatible sub-accounts for playback and set the provider-issued
stream limit for each account. A watch session stays on one account, the
least-used available account is selected for new playback, and the lease is
released when playback stops or its heartbeat expires. Each Multiview tile
uses one lease. These counters cover XC for Silo sessions only; they cannot see
connections created outside this plugin.

## Guide refreshes

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
- The plugin does not key server-side state by `X-Silo-User-Id`; per-user
  preferences remain browser-managed through Silo's user config API.

## Guide API

`GET /dispatcharr/api/guide` returns `{"programs":[...]}` ordered by start time, then program ID.
The guide is indexed once per catalog snapshot, so requests never copy or
re-sort the full guide. Optional query parameters:

| Parameter | Meaning |
| --- | --- |
| `channel_id` | Single channel ID (legacy). |
| `channel_ids` | Comma-separated channel IDs; merged with `channel_id`. Max 2000. |
| `start` | Unix seconds. With `end`, returns programs overlapping `[start, end)`. |
| `end` | Unix seconds. Either bound may be omitted to leave that side open. |

With no parameters the full guide is returned (backward compatible). A program
without an end time matches when its start lies inside the window. Invalid
numbers, more than 2000 channels, or `start >= end` return `400`.

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
