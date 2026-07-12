# Dispatcharr Admin Virtual Folder Aliases Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make admin-managed virtual folders behave and read as additive aliases, so Dispatcharr source folders stay visible while Silo adds alternate admin paths like `Sports | Argentina`.

**Architecture:** The frontend already stores admin virtual paths in `adminGroups` and channel assignments in `adminGroupMemberships`. Keep that payload for compatibility, clarify UI copy as aliases, and add tests proving `admin_delimiter` includes both source delimiter paths and admin alias paths.

**Tech Stack:** Go plugin with embedded JavaScript HTML in `internal/plugin/routes.go`, Go tests in `internal/plugin/routes_test.go`, manifest/version release flow.

---

### Task 1: Lock Alias Semantics In Tests

**Files:**
- Modify: `internal/plugin/routes_test.go`

- [ ] **Step 1: Add a JavaScript behavior test for additive aliases**

Add a test near the existing route HTML marker tests that extracts the embedded script and evaluates the virtual path functions with a small fake state:

```go
func TestAdminVirtualFolderAliasesKeepSourceAndAliasPaths(t *testing.T) {
	t.Parallel()

	script := extractPlayerScript(t)
	context := map[string]any{
		"state": map[string]any{
			"app": map[string]any{
				"channels": []map[string]any{
					{"id": "channel:argentina-sports", "name": "Argentina Sports", "categoryId": "cat:argentina-sports", "categoryName": "International | Argentina | Sports"},
				},
				"categories": []map[string]any{
					{"id": "cat:argentina-sports", "name": "International | Argentina | Sports"},
				},
			},
			"adminCategorySettings": map[string]any{
				"mode":      "admin_delimiter",
				"delimiter": "pipe",
				"adminGroups": []map[string]any{
					{"id": "admin:sports-argentina", "name": "Sports | Argentina", "order": 1},
				},
				"adminGroupMemberships": map[string]any{
					"admin:sports-argentina": []string{"channel:argentina-sports"},
				},
				"presentationOverrides": map[string]any{},
			},
		},
	}

	result := runVirtualAliasScript(t, script, context)
	if !result["sourcePath"] {
		t.Fatalf("expected source path to remain visible: %+v", result)
	}
	if !result["aliasPath"] {
		t.Fatalf("expected admin alias path to be added: %+v", result)
	}
	if result["sourceCount"] != 1 || result["aliasCount"] != 1 {
		t.Fatalf("expected source and alias counts to be one channel each: %+v", result)
	}
}
```

- [ ] **Step 2: Add helper functions for script extraction/evaluation**

Add helpers in `routes_test.go` if equivalent helpers do not already exist:

```go
func extractPlayerScript(t *testing.T) string {
	t.Helper()
	response, err := NewHTTPRoutesServer(cache.NewStore()).Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/admin"})
	if err != nil {
		t.Fatalf("admin route: %v", err)
	}
	body := string(response.GetBody())
	start := strings.Index(body, "<script>")
	end := strings.LastIndex(body, "</script>")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("expected embedded script in admin page")
	}
	return body[start+len("<script>") : end]
}

func runVirtualAliasScript(t *testing.T, script string, context map[string]any) map[string]bool {
	t.Helper()
	payload, err := json.Marshal(context)
	if err != nil {
		t.Fatalf("marshal context: %v", err)
	}
	nodeScript := fmt.Sprintf(`
const vm = require("vm");
const input = %s;
const sandbox = {
  window: { location: { pathname: "/api/v1/plugins/14/dispatcharr/admin", search: "" }, innerHeight: 800, scrollY: 0 },
  document: { documentElement: { dataset: {} }, querySelectorAll: () => [], querySelector: () => ({ classList: { toggle: () => {} } }), getElementById: () => null },
  localStorage: { getItem: () => null, setItem: () => {} },
  navigator: { sendBeacon: () => true },
  console,
  setTimeout,
  clearTimeout,
};
vm.createContext(sandbox);
vm.runInContext(script, sandbox);
Object.assign(sandbox.state, input.state);
const all = sandbox.virtualCategoriesFromPaths("", () => true, true);
const source = all.find((item) => item.name === "International / Argentina / Sports");
const alias = all.find((item) => item.name === "Sports / Argentina");
const channelsInSource = sandbox.channelsForCategory("virtual:International / Argentina / Sports");
const channelsInAlias = sandbox.channelsForCategory("virtual:Sports / Argentina");
console.log(JSON.stringify({
  sourcePath: !!source,
  aliasPath: !!alias,
  sourceCount: channelsInSource.length,
  aliasCount: channelsInAlias.length
}));
`, string(payload))
	cmd := exec.Command("node", "-e", nodeScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run node script: %v\n%s", err, output)
	}
	var result map[string]bool
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode node result: %v\n%s", err, output)
	}
	return result
}
```

- [ ] **Step 3: Run the targeted test and confirm it fails or exposes missing helpers**

Run:

```bash
go test ./internal/plugin -run TestAdminVirtualFolderAliasesKeepSourceAndAliasPaths -count=1
```

Expected before implementation cleanup: the test may fail because the helper needs imports or because the embedded script is not safely evaluable. Fix only the test harness until the failure is about behavior.

- [ ] **Step 4: Commit the failing/behavior test once it compiles**

Run:

```bash
git add internal/plugin/routes_test.go
git commit -m "Test Dispatcharr admin virtual folder aliases"
```

### Task 2: Rename Admin Group UI To Alias UI

**Files:**
- Modify: `internal/plugin/routes.go`
- Modify: `internal/plugin/routes_test.go`

- [ ] **Step 1: Update visible admin copy**

In `renderAdminPage`, `renderAdminCategorySettings`, and `renderAdminGroupSettings`, use alias language:

```js
"<div class=\"settings-card\"><h2>Admin virtual folder aliases</h2><div id=\"admin-group-settings\" class=\"settings-list\"></div></div>"
```

Use labels:

```js
"New alias folder"
"Edit alias folder"
"Create an admin virtual folder alias to place channels in another Silo folder."
"No channels in this admin virtual folder alias yet."
```

- [ ] **Step 2: Keep internal payload names unchanged**

Do not rename `adminGroups` or `adminGroupMemberships`. They are persisted in existing Silo runtime config and must remain backwards compatible.

- [ ] **Step 3: Update route HTML marker tests**

In `TestHTTPRoutesServerAdminPageIncludesCategoryMapping`, assert the new copy:

```go
`Admin virtual folder aliases`,
`New alias folder`,
`Edit alias folder`,
```

- [ ] **Step 4: Run UI marker tests**

Run:

```bash
go test ./internal/plugin -run 'TestHTTPRoutesServerAdminPageIncludesCategoryMapping|TestHTTPRoutesServerAppRouteIncludesAppLayerPayload' -count=1
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add internal/plugin/routes.go internal/plugin/routes_test.go
git commit -m "Label admin virtual folders as aliases"
```

### Task 3: Ensure Alias Rendering Is Additive

**Files:**
- Modify: `internal/plugin/routes.go`
- Modify: `internal/plugin/routes_test.go`

- [ ] **Step 1: Verify source and alias paths are both returned**

Confirm `virtualPathsForChannel(channel)` includes:

```js
const sourcePath = sourceVirtualPathForChannel(channel);
if (sourcePath) paths.push(sourcePath);
adminGroupsForChannel(channel).forEach(function(group) {
  const groupPath = virtualPathFromText(group.name);
  if (groupPath) paths.push(groupPath);
});
return uniqueIDs(paths);
```

If the current implementation already does this, leave it unchanged.

- [ ] **Step 2: Ensure virtual folder counts de-dupe by path**

Confirm `virtualCategoriesFromPaths` tracks `channelIDs` per `childPath`, not globally:

```js
groups[childPath].channelIDs[channel.id] = true;
groups[childPath].count = Object.keys(groups[childPath].channelIDs).length;
```

If this already exists, leave it unchanged.

- [ ] **Step 3: Run alias behavior test**

Run:

```bash
go test ./internal/plugin -run TestAdminVirtualFolderAliasesKeepSourceAndAliasPaths -count=1
```

Expected: pass.

- [ ] **Step 4: Run full plugin tests**

Run:

```bash
go test ./...
```

Expected: pass.

- [ ] **Step 5: Commit if code changed**

If Task 3 required code changes:

```bash
git add internal/plugin/routes.go internal/plugin/routes_test.go
git commit -m "Keep source paths with admin aliases"
```

### Task 4: Release And Deploy

**Files:**
- Modify: `manifest.json`
- Modify catalog repo files:
  - `/Users/jonathanfinley/Developer/LeZen/silo-server-plugins/ramindex-continuum-plugins/manifest.json`
  - `/Users/jonathanfinley/Developer/LeZen/silo-server-plugins/ramindex-continuum-plugins/checksums.txt`

- [ ] **Step 1: Bump plugin version**

Update `manifest.json` from the current version to the next patch version.

- [ ] **Step 2: Verify locally**

Run:

```bash
go test ./...
go run . manifest >/tmp/dispatcharr-manifest.json
```

Expected: tests pass and manifest reports the bumped version.

- [ ] **Step 3: Commit version bump**

```bash
git add manifest.json
git commit -m "Release Dispatcharr admin alias folders"
```

- [ ] **Step 4: Push and tag**

```bash
git push origin clean-main:main
git tag vX.Y.Z
git push origin vX.Y.Z
```

- [ ] **Step 5: Wait for GitHub release**

Run:

```bash
gh run list --repo theramindex/silo-plugin-dispatcharr --limit 5
gh run watch <tag-run-id> --repo theramindex/silo-plugin-dispatcharr --exit-status
```

Expected: release succeeds.

- [ ] **Step 6: Update catalog**

Download checksums:

```bash
gh release download vX.Y.Z --repo theramindex/silo-plugin-dispatcharr --pattern checksums.txt --dir /tmp/dispatcharr-X.Y.Z-release
```

Patch catalog `manifest.json` and `checksums.txt` with version, URLs, and checksums.

- [ ] **Step 7: Validate and push catalog**

```bash
cd /Users/jonathanfinley/Developer/LeZen/silo-server-plugins/ramindex-continuum-plugins
node scripts/validate-catalog.mjs
git add manifest.json checksums.txt
git commit -m "Update Dispatcharr plugin to X.Y.Z"
git push origin main
```

- [ ] **Step 8: Deploy to rs112**

Download linux/amd64 binary and manifest, verify SHA256, copy to rs112, update plugin installation `14`, and restart `continuum`.

- [ ] **Step 9: Verify deployment**

Check:

```bash
ssh seed@208.99.62.112 'sudo docker exec postgres psql -U continuum -d continuum -x -c "select id, plugin_id, version, install_path, enabled, updated_at from plugin_installations where id = 14;"'
```

Expected: install `14` reports the new version and enabled `t`.

---

## Self-Review

- Spec coverage: additive source plus alias paths are covered by Task 1 and Task 3. UI alias language is covered by Task 2. Persistence remains unchanged and is covered by the existing admin settings save flow.
- Placeholder scan: no TBD/TODO placeholders remain.
- Type consistency: persisted names remain `adminGroups` and `adminGroupMemberships`; user-facing language changes to aliases only.
