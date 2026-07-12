# Source contract

## Xtream primary source

The in-memory Catalog uses the provider's `player_api.php` actions:

- `get_live_categories` and `get_live_streams` for channels and Collections.
- `get_short_epg` for channel program windows.
- `get_vod_categories` and `get_vod_streams` for VOD.
- `get_series_categories`, `get_series`, and `get_series_info` for series,
  seasons, and episodes.

Stable catalog identities use the provider's numeric stream, series, and
episode identifiers. A series-info response is normalized into a single ordered
episode list with a numeric season number.

Live streams advertise provider Catch-up only when `tv_archive` is enabled and
`tv_archive_duration` is positive. Catch-up playback targets use the provider's
timeshift URL convention; unsupported or malformed inputs produce no target.

## M3U/XMLTV secondary source

M3U contributes channels and source URLs; XMLTV contributes matching program
data. This mode intentionally has no VOD, series, episode, or provider
Catch-up capability.

## Boundary

The upstream client may construct a playback target only inside a
provider-bound Playback Gateway. The target must not be copied into normal
Catalog or UI payloads.
