# XC for Silo

Standalone Silo IPTV plugin with Xtream Codes as the primary source and
M3U/XMLTV as a reduced-feature secondary source.

## Features

- Live TV channel browsing, guide, search, favorites, and in-plugin playback
- Xtream VOD, series details, and episode playback
- Provider-supported Catch-up replay for eligible guide programs
- Four-tile Multiview with one active audio tile
- Manual, startup, and host-scheduled catalog refreshes
- M3U/XMLTV Live TV and guide support

## Source modes

### Xtream Codes

Configure the provider base URL, username, and password in Silo's global plugin
settings. This mode supports the full catalog: Live TV, guide, VOD, series,
episodes, and provider Catch-up.

### M3U/XMLTV

Configure an M3U playlist URL and XMLTV guide URL. This mode supports Live TV,
guide browsing, and playback only. VOD, series, episodes, and Catch-up are not
advertised.

## Playback and privacy

The app serves catalog data without provider URLs or credentials. Playback is
resolved only through authenticated Silo plugin gateway routes. The current
Silo SDK does not provide a typed viewer identity or streaming response body,
so this plugin does not claim per-user upstream credentials, plugin-enforced
stream limits, server transcoding, or a media proxy.

An administrator-configured Xtream account is shared upstream. Each Multiview
tile consumes a provider connection; provision the upstream account
accordingly.

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
