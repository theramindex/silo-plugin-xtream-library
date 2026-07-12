# Dispatcharr Admin Virtual Folder Aliases Design

## Goal

Add admin-managed virtual folder aliases for Dispatcharr categories.

The source Dispatcharr category path must remain visible. Admin aliases add additional Silo-only virtual paths over the same channels.

Example:

- Source folder from Dispatcharr: `International | Argentina | Sports`
- Admin alias folder in Silo: `Sports | Argentina`

Users should see both:

- `International > Argentina > Sports`
- `Sports > Argentina`

No Dispatcharr data is changed. No M3U is rewritten. No channel is duplicated in storage.

## Product Model

Admin aliases are special virtual folders.

They do not replace source categories. They add extra paths into the virtual category tree.

The existing category modes remain:

- `normal`: show source categories as provided.
- `delimiter`: split source category names by delimiter into virtual folders.
- `admin_delimiter`: split source category names and include admin virtual folder aliases.

The default behavior for aliases is additive. A future option may hide or replace the source path, but that is out of scope for this change.

## Data Model

Extend the existing admin category settings payload with alias entries.

The current internal names can remain compatible:

- `adminGroups`: admin-defined virtual folders.
- `adminGroupMemberships`: channel IDs assigned to each admin virtual folder.

For the UI and product language, these are presented as:

- Admin virtual folder aliases.
- Alias folder.
- Channels in alias.

Each alias stores:

- Stable alias ID.
- Alias folder path, using the selected delimiter, such as `Sports | Argentina`.
- Channel memberships by channel ID.

Aliases point directly at channel IDs. If a channel disappears from Dispatcharr, stale IDs are ignored during rendering and counts.

## Rendering Behavior

When `admin_delimiter` mode is active:

- Build source virtual folders from Dispatcharr category names.
- Build admin alias virtual folders from alias folder paths.
- Merge both sets into the virtual category tree.
- Do not de-duplicate a channel across different virtual folder paths.
- Do de-duplicate channels within the same virtual folder path.

Counts should reflect unique channels within that folder path.

Opening either the source path or the alias path shows the same underlying channel objects, with presentation overrides applied.

## Admin UX

Rename admin copy from generic groups to virtual folder aliases:

- Section title: `Admin virtual folder aliases`.
- Create label: `New alias folder`.
- Edit label: `Edit alias folder`.
- Empty state: `Create an admin virtual folder alias to place channels in another Silo folder.`

Suggested future workflow text:

- Source folder: `International | Argentina | Sports`
- Alias folder: `Sports | Argentina`

For this change, alias membership can continue to be managed by selecting channels manually. Folder-to-folder bulk aliasing may be added later if needed.

## Persistence

Use the existing durable admin settings path:

- `POST /dispatcharr/api/admin-settings`
- Silo runtime config key: `category_settings`

The admin page remains admin-only. The save API remains protected by the admin-page token.

## Error Handling

If a saved alias references stale channel IDs, ignore those IDs.

If two aliases use the same folder path, merge their channels in that path and de-duplicate by channel ID.

If an alias has an empty name/path, do not render it.

## Testing

Add or update tests for:

- Admin UI copy uses virtual folder alias language.
- `admin_delimiter` keeps source virtual paths visible.
- Alias virtual paths are added alongside source paths.
- Same channel may appear through both source and alias paths.
- Counts de-duplicate within a single path.
- Missing channel IDs do not render or count.
