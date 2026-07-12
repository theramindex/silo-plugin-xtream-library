# Silo Host Change Proposal For Dispatcharr Live TV

## What This Proposal Does

This document defines the minimum host-side change likely needed in Silo to support a true Dispatcharr-backed **Live TV** source that can appear through Silo's Jellyfin-compatible API layer.

It does **not** propose changes to this plugin repository alone. The plugin repo is already aligned with the current Silo SDK and config/runtime contract. The gap is on the Silo host side.

## The Core Change Categories

### 1. Plugin capability for source creation

Silo likely needs a plugin capability that can create or register a host-visible media source rather than only enrich metadata on existing items.

Minimum requirement:
- a plugin must be able to declare that it provides a source/catalog surface, not just metadata

### 2. Live TV domain model ingestion

Silo likely needs a host-side path for plugin-owned:
- channels
- guide/program rows
- playback targets
- source identity and refresh state

Minimum requirement:
- a plugin must be able to populate or virtualize the internal models that the Jellyfin compatibility layer reads from

### 3. Jellyfin-facing exposure

Silo likely needs a host-side adapter that maps the plugin-backed Live TV source into the Jellyfin-compatible API in the same way other first-class host media surfaces are mapped.

Minimum requirement:
- the Jellyfin compatibility layer must understand plugin-backed Live TV entities as a supported content surface

## Package Structure

This proposal assumes the existing separation already present in Silo:

- plugin host / runtime process management
- plugin capability clients
- metadata provider chain
- Jellyfin compatibility HTTP layer

The proposed host change should add a new integration path alongside the existing metadata-provider flow rather than overloading `metadata_provider.v1`.

## How the Data Connects

### Current path

Today, Silo appears to support:
- plugin config delivery through `Runtime.Configure`
- plugin metadata enrichment through `metadata_provider.v1`
- plugin-owned HTTP endpoints through `http_routes.v1`

That path is suitable for search/enrichment, but not for creating a new Jellyfin-visible Live TV source.

### Proposed path

Silo would add a host-recognized plugin capability for plugin-backed media sources or Live TV sources.

The data flow would be:

1. plugin installation is configured and started
2. plugin registers or exposes a Live TV source capability
3. Silo host ingests or virtualizes plugin-provided channels and guide data
4. Silo stores or resolves playback through host-managed source abstractions
5. Jellyfin compatibility reads from that host-recognized Live TV surface

## Expected Deliverables

If Silo work resumes later, the minimum useful deliverables would be:

1. **Host capability definition**
   - new plugin capability for source/catalog or Live TV exposure

2. **Host-side adapter**
   - code that turns plugin data into Silo-recognized source/channel/guide structures

3. **Jellyfin compatibility bridge**
   - mapping from that host-recognized plugin source into the Jellyfin-facing API

4. **Playback integration**
   - host-managed playback path for plugin-backed live streams

## Known Constraints

- The current Silo plugin architecture appears to auto-register `metadata_provider.v1` into metadata enrichment paths, not source creation paths.
- The current Dispatcharr plugin repo should not pretend to solve the full Live TV product goal without the host-side capability.
- We are intentionally **not** editing Silo right now.

## Status

- Dispatcharr plugin repo: aligned with current SDK/config/runtime behavior
- Silo host: likely missing the capability needed for true plugin-backed Live TV exposure
- This proposal: documentation only, no Silo code changes made

## Recommended Direction

The least-wrong future direction is:

- add a new Silo host capability for plugin-backed source/catalog creation or plugin-backed Live TV
- keep `metadata_provider.v1` focused on metadata enrichment
- let the Dispatcharr plugin migrate to that new host capability once it exists

## Related Pages

- `docs/continuum-host-gap.md`
- `docs/sdk-fit-notes.md`
