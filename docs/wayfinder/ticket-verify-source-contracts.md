---
title: Verify Xtream and M3U/XMLTV catalog and provider catch-up contracts
labels: [wayfinder:research]
parent: map-xtreme-codes-for-silo
status: complete
assignee: codex
blocked_by: []
---

## Question

Which upstream calls and item identifiers are needed to build an in-memory
catalog for Xtream Live TV, EPG, VOD, series, episodes, and provider catch-up,
and how should the secondary M3U/XMLTV mode represent its reduced feature set?

## Resolution

Xtream uses the player API catalog actions and stable provider IDs. Series info
normalizes into ordered episodes; Catch-up requires `tv_archive` plus a positive
`tv_archive_duration`. M3U/XMLTV remains a Live TV/guide-only source. See
`docs/source-contract.md`.
