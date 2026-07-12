---
title: Map: Xtreme Codes for Silo
labels: [wayfinder:map]
status: open
---

## Destination

Produce an implementation-ready, Silo-SDK-compatible specification for a
deployable **Xtreme Codes for Silo** plugin: Xtream-first with M3U/XMLTV
support, full Live TV/EPG/VOD/series/episode playback, provider catch-up, and
four-tile multiview.

## Notes

- Work only within the confirmed Silo plugin SDK surface.
- Use no plugin-owned database or migrations. Catalog state is in-memory.
- The Xtream account is admin-configured and shared; v1 has no per-user or
  plugin-enforced concurrent-stream limits.
- Consult `research`, `domain-modeling`, and the SDK source before resolving
  SDK or playback decisions.

## Decisions so far

<!-- Closed child issues are indexed here as they resolve. -->

## Not yet specified

- Silo SDK's verified capabilities for authenticated identity, response
  streaming, redirects, user configuration, and scheduled task semantics.
- The exact opaque playback-gateway contract permitted by that SDK.
- The UI information architecture and player interaction details after the
  gateway boundary is clear.
- Refresh error behavior, source switching behavior, packaging, catalog, and
  release process.

## Out of scope

- DVR, local rolling-buffer timeshift, Dispatcharr integration, and
  Dispatcharr-specific sports/event features.
