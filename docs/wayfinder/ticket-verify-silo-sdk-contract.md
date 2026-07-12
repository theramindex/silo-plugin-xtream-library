---
title: Verify the Silo SDK contract for Xtreme Codes routes and playback
labels: [wayfinder:research]
parent: map-xtreme-codes-for-silo
status: complete
assignee: codex
blocked_by: []
---

## Question

What does the current Silo plugin SDK actually guarantee for authenticated HTTP
routes, request identity, redirects, response streaming, global and user
configuration secrets, static assets, and scheduled tasks—and which of those
capabilities can the plugin rely on without host changes?

## Resolution

Verified against SDK v0.8.1. The SDK route request has no typed trusted viewer
identity and the response body is a finite byte slice, so v1 cannot claim a
server-side media proxy or per-user upstream enforcement. See
`docs/sdk-capability-contract.md`.
