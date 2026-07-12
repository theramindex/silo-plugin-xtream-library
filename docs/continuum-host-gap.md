# Silo Host Gap For Dispatcharr Live TV

## What is implemented in this plugin repo

- Silo SDK upgraded to match the host app version in use.
- `Runtime.Configure` handling aligned with Silo's actual grouped `ConfigEntry` contract.
- `global_config_schema` aligned with Silo's admin UI expectations, including `admin_form` descriptors.
- Xtream and M3U/XMLTV configuration, sync, cache, playback-resolution, and status-route behavior implemented and tested locally.

## What is not possible from the plugin alone right now

The current Silo host code appears to support plugins in two relevant ways:

- `metadata_provider.v1` for metadata search and enrichment
- `http_routes.v1` for proxied plugin-owned HTTP endpoints

What it does **not** appear to support is a plugin creating a brand-new Jellyfin-visible **Live TV** source/catalog/channel/guide model that Silo can expose as its own first-class media surface.

## Why this is a blocker

The original Dispatcharr goal is not just configuration or metadata lookup. It requires Silo to present IPTV channels and guide data as a real Jellyfin-facing source.

Based on the reviewed Silo code:

- plugin metadata providers are wired into the existing metadata provider chain
- that path is scoped to movie/series metadata enrichment
- Jellyfin compatibility reads from Silo's internal catalog/content models
- no observed plugin capability populates those models with a new Live TV source

## Decision for now

We are **not** editing Silo at this time.

So the practical decision is:

- keep this plugin repo aligned with the real Silo SDK/config contracts
- document the host limitation clearly
- stop short of claiming that the plugin can already produce a true Silo/Jellyfin Live TV source

## Future host-side change likely needed

Silo likely needs a new plugin capability or host integration path that can do at least one of these:

- create a plugin-backed media source/catalog
- create a plugin-backed Live TV channel and guide model
- register plugin-owned playback/catalog entities that the Jellyfin compatibility layer can surface directly

## Recommended resume point

If work resumes later, start by defining the minimal Silo host capability needed for plugin-backed Live TV sources before adding more plugin-side code.

See also:
- `docs/continuum-host-change-proposal.md`
