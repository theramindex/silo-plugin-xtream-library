# AerioTV Feature Parity Review

Reviewed: 2026-07-11

Reference: [AerioTV](https://github.com/jonzey231/AerioTV) at commit
`dd531aadf0fcaad02d6d6f938ebce85f9dda19b7`.

This review compares AerioTV with the Silo Dispatcharr plugin. It treats the
Silo plugin SDK and browser runtime as product boundaries. Native Apple
platform features are listed separately from gaps that are practical to close.

## Executive Summary

The plugin has strong parity for source connectivity, live channel browsing,
the EPG, basic playback, favorites, basic multiview, and Dispatcharr-managed
recording. VOD and series parity is explicitly out of scope. The plugin is
ahead of AerioTV in profile-aware browse organization,
presentation overrides, Silo administration, sports discovery, event browsing,
and the On Later workflow.

AerioTV is substantially ahead in recording options, advanced multiview,
playback diagnostics and controls, guide reminders, live rewind, catch-up, and
native device integration. The highest-value plugin work is not
to reproduce every native feature. It is to deepen the browser workflows that
fit the SDK: DVR options, stream diagnostics, a sleep timer, upstream switching,
guide organization, and a more capable four-tile multiview.

## Parity Matrix

Status meanings:

- **Parity**: materially equivalent user outcome.
- **Partial**: the core exists, but AerioTV has meaningful additional depth.
- **Missing**: practical in the plugin, but not currently present.
- **Platform-specific**: depends on native Apple APIs or an always-running
  device process and should not be treated as a plugin parity requirement.
- **Plugin ahead**: the Silo plugin has a stronger or unique implementation.

| Area | AerioTV | Dispatcharr plugin | Status |
| --- | --- | --- | --- |
| Dispatcharr Direct | API key and username/password | API key and username/password | Parity |
| Xtream Codes | Live TV, EPG, VOD, series | Live TV, EPG, VOD, series | Parity |
| M3U/XMLTV | Live TV and guide | Live TV and guide | Parity |
| Multiple saved sources | Multiple playlists/servers | One configured source per plugin installation | Missing |
| Live channel browse | Lists, groups, favorites | Browse tree, groups, favorites, custom groups | Parity |
| Channel profiles | Basic source-side profile use | All profiles, profile selection, nested profile/group paths, deduplication | Plugin ahead |
| Presentation organization | Group ordering and collections | Profile/group hierarchy, delimiter parsing, presentation overrides | Plugin ahead |
| EPG grid and list | Grid/list, details, search | Grid/list, details modal, search, lazy rendering | Parity |
| Guide reminders | Local reminders with iCloud sync | None | Missing |
| Guide ordering | Number, name, favorites, manual group order | Source order with favorite/custom-group organization | Partial |
| Guide window | Configurable compact/full range | Fixed application window with incremental loading | Partial |
| EPG category colors | User-configurable tinting | Live/no-data states, no category tint controls | Missing |
| Guide refresh | Manual and stale auto-refresh | Async manual refresh, scheduled tasks, stale cache/status polling | Parity |
| Live playback | MPV with broad codec support | Browser HLS and MPEG-TS playback | Partial |
| Live rewind | Native disk-backed buffers across supported Apple players | Shared Direct/H.264 disk-backed HLS ring buffers with global admin limits | Partial |
| Audio and subtitles | Track selection | Track selection | Parity |
| PiP, AirPlay, fullscreen, aspect | Native controls | Browser controls where supported | Partial |
| Playback options | Speed, sleep timer, audio-only, stream info | Aspect, tracks, volume, external player, guide | Partial |
| Stream switching | Select another upstream/provider | No upstream selector | Missing |
| Playback diagnostics | Codec, resolution, bitrate, FPS, dropped frames | No detailed diagnostics panel | Missing |
| Multiview | Up to 9, layouts, reorder, spotlight, per-tile options | Up to 4, focus controls and single active audio | Partial |
| Server-side DVR | Schedule, discover, inspect, play, download, manage | Schedule, list, and play for Dispatcharr Direct | Partial |
| Local recording | All source types | None | Platform-specific |
| DVR padding and Comskip | Scheduling and post-processing controls | None | Missing |
| DVR lifecycle controls | Stop/delete/download and in-progress playback | Playback only; no stop/delete/download | Missing |
| Sports and events | Standard guide discovery | Scoreboards, league filters, team favorites, matched channels, events | Plugin ahead |
| On Later | Standard guide search | Dedicated time/type filters and keyword passes | Plugin ahead |
| User personalization | Favorites and iCloud-synced preferences | Silo user preferences, profile selection, custom groups | Parity |
| Admin operations | Per-source settings and diagnostics | Health/status, refresh jobs, profile access, ECM integration | Plugin ahead |
| Apple TV integration | Top Shelf, Siri Remote, native mini-player | Browser/Silo shell | Platform-specific |
| Native background behavior | Local notifications, downloads, local DVR | Limited by browser and plugin process model | Platform-specific |

## Recommended Work

### P0: Complete Current Reliability Work

1. Finish and ship the pending profile selection, nested hierarchy,
   presentation override, and status refresh changes.
2. Keep refresh operations asynchronous and status-driven. Large Dispatcharr
   syncs must not hold an SDK request open until Silo's RPC deadline.
3. Preserve channel deduplication by stable channel ID while allowing one
   channel to appear in multiple selected profile/group browse paths.

### P1: Highest User Value

1. **DVR padding and Comskip**
   Add pre-roll and post-roll defaults plus per-recording overrides. Send
   adjusted start/end times without a `program` object to avoid double-applying
   Dispatcharr's global offsets. Offer Comskip only when the active Dispatcharr
   server supports it, using `custom_properties.comskip` on the recording.
2. **Guide reminders**
   Add per-user reminders persisted through Silo preferences. In-browser
   reminders can surface while the app is open; external notification delivery
   requires a Silo host capability and should not be promised by the plugin.
3. **Upstream switching**
   For Direct admin connections, load member streams from
   `/api/channels/channels/{id}/streams/`, switch through
   `/proxy/ts/change_stream/{uuid}`, and confirm against
   `/proxy/ts/status/{uuid}`. Hide the control when there is one stream or the
   caller lacks permission.
4. **Guide organization**
   Add channel sort modes, manual custom-group ordering, and a configurable
   guide horizon. Keep source order as the default.

### P2: Playback and Multiview

1. Add a stream diagnostics panel using browser media APIs and player-library
   statistics: selected URL type, dimensions, buffered duration, current
   bitrate where available, dropped frames, and recent playback errors.
2. Add sleep timer and audio-only mode. Playback speed, if added later, should
   apply to recordings rather than live channels by default.
3. Keep the four-stream browser limit as the safe default. Improve the current
   multiview with layout selection, tile reordering, spotlight mode, and
   per-tile track controls before considering a higher maximum.

### P3: Optional Polish

1. Add EPG category color rules and a compact/full guide density preference.
2. Add recording download actions only when the host can stream files
   without buffering the complete response through the plugin SDK.
3. Consider server-provided catch-up only after capability discovery and past
   guide retention are defined. Do not implement a disk-backed rolling rewind
   buffer inside the Silo plugin process.

## AerioTV Changes After v1.7.10

The repository has a large unreleased change set after `v1.7.10`. The dominant
addition is Live Rewind and catch-up playback:

- Disk-backed rolling live buffers with 15-180 minute depth settings.
- Pause, scrub, skip back/forward, and return-to-live controls.
- Playback of provider catch-up archives from past guide programs.
- Buffer cleanup, storage estimates, reconnect/fallback behavior, and tvOS
  remote interaction refinements.
- AVPlayer watchdog and fallback work around stalled or unrenderable streams.

The plugin now ports the core rolling-buffer idea without FFmpeg: a shared Go
ingest copies H.264 MPEG-TS packets into keyframe-aligned HLS segments, stores
them under a bounded global disk budget, and exposes independent browser
playheads through short-lived leases. It deliberately excludes multiview,
non-H.264 codecs, and non-Direct sources. A playback-stall watchdog and
server-provided catch-up remain separate future work.

## SDK and Runtime Boundaries

The following AerioTV features should not be direct parity targets:

- MPV-specific codec, HDR, deinterlacing, and frame-level controls.
- Apple TV Top Shelf and Siri Remote integration.
- Native iCloud synchronization and local notifications.
- Reliable background local recording and device downloads.
- Native LAN detection and automatic local/remote URL switching.
- Nine simultaneous streams as a default. Browser decoder and device resource
  limits make four a more responsible baseline.

The plugin should use Silo user preferences for personalization, scheduled task
capabilities for refresh work, bounded cached payloads for large catalogs, and
short request handlers that enqueue or poll long-running work. Any feature that
requires new user-profile fields, background notifications, raw file streaming,
or durable download jobs needs an explicit Silo SDK capability first.

## Plugin Advantages to Preserve

- Profile is the top-level lineup, nested groups are folders, and channels are
  deduplicated items that may appear in multiple browse paths.
- Per-user profile visibility is presentation-only and does not mutate the
  shared imported catalog.
- Presentation Overrides remain composable under profile paths.
- Sports, Events, On Later, keyword passes, and team favorites remain first-
  class discovery surfaces.
- Dispatcharr Direct is the only mode that advertises server DVR controls.
- Admin refresh/status workflows remain observable and asynchronous.
