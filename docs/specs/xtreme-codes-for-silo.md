---
title: Spec: Xtreme Codes for Silo
labels: [ready-for-agent]
status: open
---

## Problem Statement

Silo needs a standalone IPTV plugin for administrators who use Xtream Codes
providers and, secondarily, M3U/XMLTV sources. The current Dispatcharr plugin
contains useful player and catalog ideas but is not the right product boundary:
it includes Dispatcharr-specific connection modes, DVR behavior, storage
assumptions, and features that do not belong in an Xtream-focused plugin.

Administrators need a Silo-native Live TV experience that works within the
actual Silo plugin SDK, keeps provider secrets out of normal app/catalog/status
payloads, does not create a plugin-owned database, and is clear about upstream
connection limits. Viewers need to browse and watch Live TV, guide programs,
VOD, and series episodes in one coherent app.

## Solution

Deliver **Xtreme Codes for Silo**, a standalone Silo plugin with an
administrator-configured Xtream Account as its primary Source Mode and
M3U/XMLTV as a secondary Source Mode. The plugin maintains a bounded,
in-memory Catalog; it never creates database tables, migrations, or persistent
catalog storage.

The plugin serves a Silo-native Browser Player with bundled local HLS and
MPEG-TS player libraries. A provider-bound Playback Gateway resolves a catalog
item only at playback time. The final gateway behavior must stay within the
verified Silo SDK contract; the product must not claim a server stream proxy,
trusted per-viewer identity, or plugin-enforced stream quotas unless the SDK
actually provides those capabilities.

Xtream supports Live TV, EPG, VOD, series, episode playback, provider Catch-up,
and Multiview with at most four simultaneous live tiles. M3U/XMLTV supports its
compatible Live TV and guide subset. DVR, local buffering, and Dispatcharr
features are excluded.

## User Stories

1. As an administrator, I want to install a clearly named Xtreme Codes for
   Silo plugin, so that I know it is separate from Dispatcharr.
2. As an administrator, I want to configure an Xtream base URL, username, and
   password as plugin secrets, so that the app can access my provider.
3. As an administrator, I want the configuration to validate the upstream
   account before normal use, so that invalid credentials are surfaced early.
4. As an administrator, I want to select M3U/XMLTV as a secondary Source Mode,
   so that a compatible provider remains usable when Xtream is unavailable.
5. As an administrator, I want the app to state that one Xtream Account is
   shared upstream, so that I can provision an appropriate provider connection
   allowance.
6. As an administrator, I want provider secrets absent from normal catalog,
   status, and UI responses, so that routine use does not reveal credentials.
7. As a viewer, I want a Live TV app entry in Silo, so that I can reach
   channels without leaving the application.
8. As a viewer, I want to browse channels by Collection and search them, so
   that I can find a channel quickly.
9. As a viewer, I want to view current and upcoming guide programs, so that I
   can choose what to watch.
10. As a viewer, I want to start a live channel in a full-screen-capable
    Browser Player, so that playback feels native to Silo.
11. As a viewer, I want the player to use the stream format my browser can
    handle, so that HLS and MPEG-TS streams work reliably where supported.
12. As a viewer, I want to browse VOD categories and titles, so that my
    provider's on-demand catalog is usable.
13. As a viewer, I want to open a series and select an episode, so that series
    playback is complete rather than merely a catalog listing.
14. As a viewer, I want a VOD item or episode to play in the same Browser
    Player, so that playback controls are consistent.
15. As a viewer, I want eligible guide programs to expose provider Catch-up,
    so that I can replay an archived program without creating a recording.
16. As a viewer, I want to add up to four live channels to Multiview, so that I
    can follow multiple broadcasts at once.
17. As a viewer, I want one Multiview tile to have active audio, so that the
    experience remains understandable.
18. As a viewer, I want the app to explain that each Multiview tile consumes a
    provider connection, so that I understand why provider limits matter.
19. As an administrator, I want a manual refresh and periodic host-scheduled
    refresh capabilities, so that channels and guide data can recover from
    upstream changes.
20. As a viewer, I want a transient refresh failure to preserve usable current
    data when possible, so that a short provider outage does not immediately
    empty the app.
21. As an administrator, I want the plugin to rebuild its Catalog after a
    restart rather than depend on a plugin database, so that operations stay
    simple and portable.
22. As a viewer in M3U/XMLTV mode, I want Live TV and guide behavior to work
    without Xtream-only menus, so that unavailable features are not misleading.

## Implementation Decisions

- The product name is **Xtreme Codes for Silo**. Its primary Source Mode is
  Xtream; M3U/XMLTV is a secondary Source Mode.
- An Xtream Account is administrator-configured global plugin configuration.
  It is a shared upstream identity, not a Silo user identity.
- The plugin must use only confirmed Silo SDK functionality. SDK verification
  is a blocking implementation task for route authentication, secret storage,
  response behavior, static assets, and scheduled tasks.
- The Catalog is a bounded in-memory representation of channels, guide data,
  VOD, series, and episodes. No plugin-owned database, migrations, catalog
  tables, or persistent session tables are permitted.
- The app hydrates its Catalog on startup or first use, supports an
  authenticated manual refresh, and exposes scheduled-task capabilities for
  host-configured periodic channel and guide refreshes.
- The Browser Player is a fresh Xtream-focused client module. It may reuse
  compatible local HLS and MPEG-TS player libraries and interaction patterns,
  but it does not inherit Dispatcharr product modes or UI features.
- Playback must flow through provider-bound Playback Gateway routes. Normal
  Catalog, status, and UI responses may not include provider stream URLs,
  usernames, or passwords.
- Whether the final gateway redirects, streams, or uses another SDK-supported
  response mechanism is deliberately deferred until the SDK capability
  contract is verified. No generic arbitrary-URL proxy is allowed.
- The plugin does not promise or implement per-user upstream credentials,
  trusted per-user tracking, or plugin-enforced concurrent stream limits.
  Upstream connection enforcement remains the provider's responsibility.
- Full Xtream content includes Live TV, EPG, VOD, series metadata, episode
  discovery, and episode playback.
- Provider Catch-up means the provider's archived/replay support only. It must
  not create a local rolling buffer, recording, or DVR implementation.
- Multiview supports a maximum of four simultaneous live tiles with one active
  audio source. Every tile is an upstream provider connection.
- M3U/XMLTV offers only its compatible Live TV, guide, and playback subset.
  It does not advertise Xtream VOD, series, episode, or provider Catch-up
  capabilities.
- The product may use Nodecast TV and prior Silo Live TV work as architecture
  references only. It must not adopt Nodecast's credential-bearing URLs,
  generic URL proxy, database design, or GPL-3.0-only code.

## Testing Decisions

- The primary test seam is the authenticated Silo plugin HTTP-route boundary,
  backed by controllable fake Xtream and M3U/XMLTV providers. These tests must
  prove user-observable behavior rather than internal call structure.
- Route-boundary tests cover configuration validation, app/catalog payloads,
  source-specific capabilities, manual refresh, scheduled task behavior,
  playback-gateway behavior, Catch-up eligibility, and Multiview limits.
- Focused upstream-adapter tests use controlled HTTP responses to verify
  upstream request shape, mapping, identifier stability, series episode
  discovery, and provider Catch-up mapping.
- SDK-level tests verify manifest/config schema validity, global secret
  handling, static assets, and only those HTTP response semantics that the SDK
  demonstrably supports.
- Redaction tests prove that provider usernames, passwords, and stream URLs do
  not occur in normal Catalog, status, or UI payloads and are not emitted in
  routine error output.
- Browser-player tests focus on the observable source selection, lifecycle,
  audio focus, and bounded Multiview behavior. They should not assert private
  state structure.
- Existing Dispatcharr route, configuration, upstream client, and player tests
  are useful behavioral prior art but must be adapted to the standalone domain
  rather than copied as Dispatcharr compatibility tests.

## Out of Scope

- Dispatcharr API integration, Direct Login/API-key modes, and
  Dispatcharr-specific sports or event features.
- DVR, recording scheduling, recording management, local stream buffering, or
  local timeshift.
- Per-user Xtream credentials, trusted per-user stream accounting, or
  plugin-enforced concurrency limits.
- Plugin-owned databases, migrations, persistent catalog/session tables, and
  external database infrastructure.
- A generic arbitrary-URL media proxy, server-side transcoding, and any
  unverified host extension.
- Release publication, catalog publication, and deployment until a destination
  repository is supplied.

## Further Notes

### Delivery status

- Complete: the SDK capability contract is verified against SDK v0.8.1 and
  encoded in a regression test. The contract rules out typed viewer identity
  and streaming response bodies as v1 assumptions.
- Complete: Xtream source identifiers, episode discovery, and provider
  Catch-up metadata are documented and covered by upstream-adapter tests.
- Complete: the manifest and browser client expose the standalone `/xtream`
  namespace; a handler adapter keeps existing implementation routes reachable
  while later features remove inherited behavior.
- Complete: the Go module and all internal imports identify the standalone
  `silo-plugin-xtream-library` repository rather than the Dispatcharr plugin.
- Complete: runtime configuration accepts only the standalone `connection`
  object; inherited Dispatcharr, category, and split legacy configuration
  entries cannot activate or change unsupported behavior.
- Complete: the inherited local-timeshift manager, media endpoint, and route
  handlers are removed; Xtream provider Catch-up remains the only replay path.
- Complete: Dispatcharr sports and event-discovery providers, matching logic,
  route handlers, and tests are removed from the Xtreme plugin.
- Complete: the runtime constructs no on-disk catalog storage; every plugin
  process begins with an in-memory catalog and refreshes its configured source.
- Complete: manifest-scheduled refresh capability IDs and the task executor
  use the same `xtream-*` identifiers for catalog, channel, and guide refresh.
- Complete: runtime configuration validates the standalone source before it is
  saved, surfacing incomplete Xtream/M3U settings during configuration rather
  than at the first background refresh.
- Complete: inherited Dispatcharr admin/category routes are absent from the
  packaged manifest and explicitly unavailable through the Xtreme namespace.
- Complete: XC Admin sources can opt into an alternate XMLTV EPG using
  fill-missing or prefer-alternate merge behavior. Exact IDs and unique
  normalized names are matched without changing channel or playback identity;
  failed alternate feeds preserve the Xtream guide.
- Complete: Live TV, guide responses, and provider-bound playback are available
  through the Xtreme namespace; normal Catalog payloads redact provider targets.
- Complete: VOD items and Xtream series episodes open through the in-plugin
  Browser Player using redacted detail and provider-bound gateway routes.
- Complete: eligible archived guide programs expose Xtream provider Catch-up;
  replay uses the Browser Player without local buffering or recording.
- Complete: Xtreme public requests for inherited recordings, sports, events,
  and local-timeshift endpoints are explicitly rejected, independently of the
  manifest route filter.

- The in-memory Catalog is intentionally disposable. After a process restart,
  the plugin refreshes from its configured Source Mode.
- Provider playback URLs may contain sensitive upstream credentials. Their
  unavoidable exposure characteristics under the verified SDK response model
  must be documented honestly; the UI must not imply stronger protection than
  the SDK can provide.
- The local Wayfinder map and ticket plan remain planning artifacts. This spec
  is the implementation source of truth for the tickets.
