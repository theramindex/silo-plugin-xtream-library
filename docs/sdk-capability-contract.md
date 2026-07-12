# Silo SDK capability contract

Verified against `github.com/Silo-Server/silo-plugin-sdk` v0.8.1.

## HTTP routes

- `HandleHTTPRequest` provides `method`, `path`, `headers`, `body`, and
  structured `query` values. It does not define a typed, trusted Silo-user
  identity.
- `HandleHTTPResponse` provides a status code, string headers, and a finite
  `[]byte` body. The SDK surface does not expose a streaming response reader or
  writer.
- Routes can serve embedded static assets and return normal HTTP headers. A
  redirect response can be represented by status and headers, but it does not
  conceal the resulting upstream URL from the browser.

## Configuration

- Global configuration is delivered as object-shaped `ConfigEntry` values.
- Password fields use JSON Schema `writeOnly` and the admin-form `Secret` flag.
- Configure updates must preserve a current secret when the host omits that
  field from an update.
- This plugin must not assume the SDK provides encrypted per-user secret
  storage. User preferences are non-secret presentation data only.

## Scheduled tasks

- The SDK provides a scheduled-task capability and an invocation interface.
- The host owns task cadence and retry behavior. The plugin implements task
  execution only.

## Consequences for Xtreme Codes for Silo

- No server-side media proxy, transcoder, or local stream buffer is part of
  v1. The Browser Player attaches provider media using a provider-bound
  Playback Gateway contract verified by a later feature.
- No per-user upstream credential selection or plugin-enforced stream quota is
  claimed in v1.
- Provider URLs, usernames, and passwords stay out of normal Catalog, status,
  and UI payloads. If the verified gateway must redirect the browser upstream,
  that exposure is documented rather than presented as secret isolation.
