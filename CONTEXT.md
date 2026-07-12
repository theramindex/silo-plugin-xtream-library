# Xtreme Codes for Silo

This context describes the Silo-hosted IPTV experience backed primarily by
Xtream Codes, with M3U/XMLTV as a secondary source mode.

## Language

**Xtream Account**:
The single administrator-configured upstream Xtream Codes account used by the
plugin. It is an upstream connection identity, not a Silo user account.

**Source Mode**:
The configured upstream protocol. **Xtream** is the primary source mode;
**M3U/XMLTV** is the secondary compatible mode.

**Catalog**:
The current in-memory collection of channels, guide programs, VOD, series, and
episodes fetched from the configured source. It is not a database and is
rebuilt after restart.

**Playback Gateway**:
An authenticated Silo plugin route that resolves a catalog item into the
playback target accepted by the browser player. It never places upstream
credentials in catalog or status responses.

**Browser Player**:
The Silo-hosted, client-side playback engine. It uses bundled format-specific
libraries such as HLS.js and mpegts.js and owns player controls and media
attachment.

**Provider Catch-up**:
An Xtream provider's own replay capability for eligible archived broadcasts.
It is not local buffering, recording, or DVR.

**Multiview**:
A watch mode with up to four concurrently playing channels and one active audio
source. Each tile consumes an upstream provider connection.

## Relationships

- An **Xtream Account** belongs to the plugin administrator, not to an
  individual Silo user.
- A **Source Mode** produces a **Catalog**.
- The **Browser Player** requests playback only through the **Playback
  Gateway**.
- **Provider Catch-up** is available only when the active source exposes it.
- **Multiview** is limited to four tiles and relies on the provider's
  connection allowance.

## Scope boundaries

- No plugin-owned database, migrations, persisted catalog tables, or
  per-user/concurrent-stream enforcement.
- No DVR, local rolling buffer, Dispatcharr integration, or Dispatcharr
  sports/event features.
