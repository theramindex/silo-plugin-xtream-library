# Tickets: Xtreme Codes for Silo

Tracer-bullet delivery plan for the SDK-compatible standalone IPTV plugin.
Source of truth: `docs/specs/xtreme-codes-for-silo.md`.
Work the **frontier**: any ticket whose blockers are all complete.

## Establish the standalone Xtreme Codes for Silo plugin shell

**What to build:** A standalone, SDK-valid Xtreme Codes for Silo plugin with
Xtream-first administrator configuration, no Dispatcharr integration, and no
plugin-owned database.

**Blocked by:** None — can start immediately.

- [ ] The plugin identifies itself as Xtreme Codes for Silo in its manifest,
  configuration, status, and user-facing app entry.
- [ ] Only the intended source modes are offered: Xtream primary and M3U/XMLTV
  secondary.
- [ ] The project builds and its baseline tests pass as an independent plugin.

## Verify and codify the Silo SDK capability contract

**What to build:** Tested, documented decisions for routes, configuration
secrets, assets, redirects or streaming, and scheduled tasks that use only
verified Silo SDK behavior.

**Blocked by:** None — can start immediately.

- [ ] Every capability the plugin relies on is traced to the SDK source or an
  executable SDK-level test.
- [ ] Unsupported assumptions about viewer identity, stream proxying, or
  quotas are excluded from the product contract.
- [ ] The verified contract is available to all subsequent implementation work.

## Deliver Xtream Live TV and guide browsing

**What to build:** An authenticated Live TV app with an in-memory Xtream
channel and guide catalog, category browse, search, and safe operational
refreshes.

**Blocked by:** Establish the standalone Xtreme Codes for Silo plugin shell;
Verify and codify the Silo SDK capability contract.

- [ ] An administrator can configure and validate one Xtream Account without
  secret values appearing in user-facing catalog or status payloads.
- [ ] Users can browse and search current channels and EPG programs.
- [ ] The catalog hydrates on startup or first use, supports manual refresh,
  and exposes SDK scheduled tasks without database persistence.

## Deliver the Xtream browser playback engine

**What to build:** A fresh Silo-native HLS/mpegts browser player for live
channels, using provider-bound playback gateway routes.

**Blocked by:** Deliver Xtream Live TV and guide browsing.

- [ ] The player handles the supported browser stream formats using bundled
  local libraries.
- [ ] Catalog and UI responses do not contain provider stream URLs or
  credentials.
- [ ] Playback behavior matches the verified Silo SDK contract and does not
  claim unsupported proxying or stream-limit enforcement.

## Deliver Xtream VOD, series, and episode playback

**What to build:** Xtream VOD browsing/playback plus series detail, episode
discovery, and episode playback through the same browser engine.

**Blocked by:** Deliver the Xtream browser playback engine.

- [ ] Users can browse and search Xtream VOD categories and items.
- [ ] Users can open a series, see its episodes, and start an episode.
- [ ] VOD and episode playback preserve the same secret-redaction boundary as
  Live TV.

## Add provider catch-up/replay

**What to build:** Provider-supplied replay for eligible Xtream broadcasts,
without local buffering, recording, or DVR behavior.

**Blocked by:** Deliver Xtream Live TV and guide browsing; Deliver the Xtream
browser playback engine.

- [ ] Eligible guide programs expose provider catch-up actions only when the
  source supports them.
- [ ] Replay uses the normal browser player and Playback Gateway.
- [ ] The experience never creates local recordings or a rolling stream buffer.

## Add four-tile multiview

**What to build:** A live multiview experience with up to four concurrent
tiles and one active audio source.

**Blocked by:** Deliver the Xtream browser playback engine.

- [ ] Users can add, remove, and focus up to four live-channel tiles.
- [ ] Only the focused tile supplies audio.
- [ ] The app explains that each tile consumes an upstream provider connection.

## Add M3U/XMLTV secondary-source support

**What to build:** A secondary source mode that provides compatible Live TV,
guide browsing, and playback while clearly omitting Xtream-only features.

**Blocked by:** Deliver Xtream Live TV and guide browsing; Deliver the Xtream
browser playback engine.

- [ ] An administrator can configure an M3U playlist and XMLTV guide as the
  active source mode.
- [ ] Users can browse, search, and play available live channels and guide
  programs.
- [ ] The UI does not offer Xtream-only VOD, series, or provider catch-up.

## Harden, package, and verify the first release

**What to build:** A release-ready plugin with security/redaction checks,
resilient refresh failures, SDK-compatible artifacts, and verified behavior.

**Blocked by:** Deliver Xtream VOD, series, and episode playback; Add provider
catch-up/replay; Add four-tile multiview; Add M3U/XMLTV secondary-source
support.

- [ ] Automated checks prove secrets do not appear in catalog, status, logs, or
  normal UI payloads.
- [ ] Transient source failures preserve a useful last in-memory catalog where
  possible and communicate recovery state clearly.
- [ ] The project produces validated artifacts and a documented deployment
  handoff for the destination repository.
