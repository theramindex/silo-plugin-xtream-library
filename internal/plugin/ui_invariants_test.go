package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// browserFunctionGlobals are platform APIs the UI may feature-detect with a
// typeof guard. Everything else guarded that way must be one of our own
// functions, and therefore defined somewhere in the composed bundle.
var browserFunctionGlobals = map[string]bool{
	"AbortController":      true,
	"BroadcastChannel":     true,
	"IntersectionObserver": true,
	"ResizeObserver":       true,
	"requestIdleCallback":  true,
	"structuredClone":      true,
	"Hls":                  true,
	"mpegts":               true,
	"SiloLiveMedia":        true,
}

// TestUIFunctionGuardsReferenceDefinedFunctions catches the failure mode where
// a ported call such as `typeof showPlayerError === "function"` silently
// falls back forever because the function was never ported.
func TestUIFunctionGuardsReferenceDefinedFunctions(t *testing.T) {
	t.Parallel()
	script := playerAppJavaScript()
	guard := regexp.MustCompile(`typeof\s+([A-Za-z_$][\w$]*)\s*[!=]==?\s*["']function["']`)
	seen := map[string]bool{}
	for _, match := range guard.FindAllStringSubmatch(script, -1) {
		seen[match[1]] = true
	}
	var missing []string
	for name := range seen {
		if browserFunctionGlobals[name] {
			continue
		}
		declared := regexp.MustCompile(`(?m)(^|[^\w$.])(async\s+)?function\s+` + regexp.QuoteMeta(name) + `\s*\(|(^|[^\w$.])(const|let|var)\s+` + regexp.QuoteMeta(name) + `\s*=|window\.` + regexp.QuoteMeta(name) + `\s*=`)
		if !declared.MatchString(script) {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("UI guards these functions with typeof but never defines them: %s", strings.Join(missing, ", "))
	}
}

// TestUIFunctionGuardDetectorFindsUndefinedNames keeps the detector honest.
func TestUIFunctionGuardDetectorFindsUndefinedNames(t *testing.T) {
	t.Parallel()
	guard := regexp.MustCompile(`typeof\s+([A-Za-z_$][\w$]*)\s*[!=]==?\s*["']function["']`)
	sample := `if(typeof showPlayerError==="function")showPlayerError(m); if (typeof state.retry === "function") state.retry();`
	matches := guard.FindAllStringSubmatch(sample, -1)
	if len(matches) != 1 || matches[0][1] != "showPlayerError" {
		t.Fatalf("expected only the bare identifier guard to match, got %v", matches)
	}
}

// TestUIPlayerErrorPanelIsPorted pins the Dispatcharr player error contract:
// a persistent panel with Try again and Back, wired as the default fatal
// handler for the main player.
func TestUIPlayerErrorPanelIsPorted(t *testing.T) {
	t.Parallel()
	script := playerAppJavaScript()
	for _, marker := range []string{
		"function showPlayerError(message)",
		"function clearPlayerError()",
		"function handlePlaybackFatalError(error)",
		`id=\"player-error\" class=\"player-error hidden\" role=\"alert\"`,
		`data-player-action=\"retry\"`,
		"options.onFatal : handlePlaybackFatalError",
	} {
		if !strings.Contains(script, marker) {
			t.Fatalf("player error panel is missing %q", marker)
		}
	}
	css := playerStylesCSS()
	if !strings.Contains(css, ".player-error { position: absolute;") {
		t.Fatal("player error panel needs its styles")
	}
}

// TestUIStylesStayOnDesignScale enforces ../dispatcharr/DESIGN.md: weights
// 500-700 and corners at most 0.5rem (pills and circles excepted), and no
// light hardcoded colors in the Program Details modal.
func TestUIStylesStayOnDesignScale(t *testing.T) {
	t.Parallel()
	css := playerStylesCSS()
	for _, match := range regexp.MustCompile(`font-weight:\s*(\d+)`).FindAllStringSubmatch(css, -1) {
		weight, _ := strconv.Atoi(match[1])
		if weight < 500 || weight > 700 {
			t.Fatalf("font-weight %d is outside 500-700", weight)
		}
	}
	for _, match := range regexp.MustCompile(`border-radius:\s*([0-9.]+)rem`).FindAllStringSubmatch(css, -1) {
		radius, err := strconv.ParseFloat(match[1], 64)
		if err != nil || radius > 0.5 {
			t.Fatalf("border-radius %srem exceeds 0.5rem", match[1])
		}
	}
	for _, line := range strings.Split(css, "\n") {
		if strings.HasPrefix(line, ".program-modal") && regexp.MustCompile(`#[0-9a-fA-F]{3,6}\b|\bwhite\b`).MatchString(line) {
			t.Fatalf("Program Details modal must use theme variables, found: %s", line)
		}
	}
}

// uiInvariantResult is what runUIInvariantScript reads back from the browser
// sandbox (globalThis.__result).
type uiInvariantResult struct {
	StableResults bool `json:"stableResults"`
	Separate      bool `json:"separate"`
}

// runUIInvariantScript loads the UI scripts into a minimal browser sandbox, as
// the Dispatcharr plugin's tests do, and runs statements against them.
func runUIInvariantScript(t *testing.T, statements []string) uiInvariantResult {
	t.Helper()

	dir := t.TempDir()
	runnerPath := filepath.Join(dir, "runner.js")
	// The page also loads the shared media runtime (xc-runtime-*.js), which is
	// served from the livemedia package rather than ui/; the UI scripts are the
	// playerScriptOrder assets.
	var scripts []string
	for _, name := range playerScriptOrder {
		source, err := playerUIAssets.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		scripts = append(scripts, string(source))
	}
	scriptsJSON, err := json.Marshal(scripts)
	if err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, statement := range statements {
		body += statement + "\n"
	}
	nodeScript := fmt.Sprintf(`
const vm = require("vm");
const scripts = %s;
function makeElement() {
  const attributes = {};
  const element = {
    innerHTML: "", textContent: "", value: "", scrollTop: 0, scrollLeft: 0, hidden: false, style: {}, dataset: {},
    remove: () => {},
    classList: { add: () => {}, remove: () => {}, toggle: () => {}, contains: () => false },
    setAttribute: (name, value) => { attributes[name] = String(value); },
    getAttribute: (name) => attributes[name] || null,
    removeAttribute: (name) => { delete attributes[name]; },
    querySelector: () => makeElement(),
    querySelectorAll: () => [],
    addEventListener: () => {},
    closest: () => null,
    focus: () => {}
  };
  return element;
}
const elements = {};
elements["primary-browse-nav"] = makeElement();
elements["primary-browse-nav"].dataset.view = "channels";
const sandbox = {
  window: { location: { pathname: "/api/v1/plugins/14/dispatcharr", search: "" }, addEventListener: () => {}, innerHeight: 800, scrollY: 0 },
  document: {
    documentElement: { dataset: {} },
    body: makeElement(),
    activeElement: null,
    hidden: false,
    querySelectorAll: (selector) => selector === "[data-view]" ? [elements["primary-browse-nav"]] : [],
    querySelector: () => makeElement(),
    getElementById: (id) => elements[id] = elements[id] || makeElement(),
    addEventListener: () => {},
    contains: () => true
  },
  localStorage: { getItem: () => null, setItem: () => {} },
  sessionStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
  navigator: { sendBeacon: () => true },
  console, URLSearchParams,
  requestAnimationFrame: (callback) => { callback(); return 1; },
  cancelAnimationFrame: () => {},
  getComputedStyle: () => ({ getPropertyValue: () => "", fontSize: "16px" }),
  setTimeout, clearTimeout, setInterval, clearInterval,
  fetch: async () => ({ ok: true, status: 200, text: async () => "{}", json: async () => ({}) })
};
vm.createContext(sandbox);
for (const source of scripts) {
  vm.runInContext(source.replace(/startGuideAutoRefresh\(\);[\s\S]*$/, ""), sandbox);
}
vm.runInContext(%s, sandbox);
process.stdout.write(JSON.stringify(sandbox.__result || {}));
`, scriptsJSON, strconvQuote(body))
	if err := os.WriteFile(runnerPath, []byte(nodeScript), 0o600); err != nil {
		t.Fatalf("write ui invariant runner: %v", err)
	}
	output, err := exec.Command("node", runnerPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run ui invariant script: %v\n%s", err, output)
	}
	var result uiInvariantResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode ui invariant result: %v\n%s", err, output)
	}
	return result
}

func strconvQuote(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// cssRulePattern matches innermost rules ("selector { declarations }"), which
// also reaches rules nested inside @media blocks.
var cssRulePattern = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)

func uiStylesheetWithoutComments(t *testing.T) string {
	t.Helper()
	styles, err := playerUIAssets.ReadFile("ui/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(styles), "")
}

// TestStylesheetCustomPropertiesAreDeclared catches var(--x) references that
// silently fall back to the initial value because --x is never declared (a
// border that turns text-colored, a background that disappears). A reference
// with an inline fallback, var(--x, ...), is safe; anything else must be
// declared in styles.css or set inline by the UI scripts.
func TestStylesheetCustomPropertiesAreDeclared(t *testing.T) {
	t.Parallel()
	styles := uiStylesheetWithoutComments(t)
	declared := map[string]bool{}
	for _, match := range regexp.MustCompile(`(--[A-Za-z0-9_-]+)\s*:`).FindAllStringSubmatch(styles, -1) {
		declared[match[1]] = true
	}
	scripts, err := os.ReadDir("ui")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range scripts {
		if !strings.HasSuffix(entry.Name(), ".js") {
			continue
		}
		source, err := os.ReadFile(filepath.Join("ui", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range regexp.MustCompile(`(--[A-Za-z0-9_-]+)\s*:|setProperty\(\s*["'](--[A-Za-z0-9_-]+)`).FindAllStringSubmatch(string(source), -1) {
			declared[match[1]+match[2]] = true
		}
	}
	var missing []string
	seen := map[string]bool{}
	for _, match := range regexp.MustCompile(`var\(\s*(--[A-Za-z0-9_-]+)\s*([,)])`).FindAllStringSubmatch(styles, -1) {
		name, hasFallback := match[1], match[2] == ","
		if hasFallback || declared[name] || seen[name] {
			continue
		}
		seen[name] = true
		missing = append(missing, name)
	}
	if len(missing) > 0 {
		t.Fatalf("styles.css uses undeclared custom properties without a fallback: %s", strings.Join(missing, ", "))
	}
}

// TestStylesheetFocusRemovalHasAlternative catches rules that remove the focus
// outline from a control with nothing to replace it, which leaves keyboard and
// TV-remote users unable to see where they are. It checks :focus-visible
// selectors and selectors that target an input, button, select, or textarea
// (those override the global input:focus-visible ring). A rule passes when it
// draws its own box-shadow, outline, or border-color, or when its leading class
// has a :focus-within rule that does.
func TestStylesheetFocusRemovalHasAlternative(t *testing.T) {
	t.Parallel()
	styles := uiStylesheetWithoutComments(t)
	rules := cssRulePattern.FindAllStringSubmatch(styles, -1)
	removesOutline := regexp.MustCompile(`(?:^|;)\s*outline\s*:\s*(?:0|none)\s*(?:;|$)`)
	// A value must start with a non-space character that is not "0" or "none",
	// so "border: 0" or "box-shadow: none" never counts as an indicator.
	drawsIndicator := regexp.MustCompile(`(?:^|;)\s*(?:box-shadow|outline|outline-width|border|border-color)\s*:\s*(?:[^;\s0n]|n[^o;])[^;]*(?:;|$)`)
	controlSelector := regexp.MustCompile(`(?:^|[\s>+~])(?:input|button|select|textarea)(?:\[[^\]]*\]|:[\w-]+(?:\([^)]*\))?)*$`)
	leadingClass := regexp.MustCompile(`^\s*(\.[A-Za-z0-9_-]+)`)
	focusWithin := map[string]bool{}
	for _, rule := range rules {
		if !drawsIndicator.MatchString(rule[2]) {
			continue
		}
		for _, selector := range strings.Split(rule[1], ",") {
			if match := regexp.MustCompile(`(\.[A-Za-z0-9_-]+):focus-within`).FindStringSubmatch(selector); match != nil {
				focusWithin[match[1]] = true
			}
		}
	}
	var bad []string
	for _, rule := range rules {
		if !removesOutline.MatchString(rule[2]) || drawsIndicator.MatchString(rule[2]) {
			continue
		}
		for _, selector := range strings.Split(rule[1], ",") {
			selector = strings.TrimSpace(selector)
			if !strings.Contains(selector, ":focus-visible") && !controlSelector.MatchString(selector) {
				continue
			}
			if match := leadingClass.FindStringSubmatch(selector); match != nil && focusWithin[match[1]] {
				continue
			}
			bad = append(bad, selector)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("these selectors remove the focus outline without an alternative indicator:\n  %s", strings.Join(bad, "\n  "))
	}
}

// TestReadableErrorHidesRawDetailAndLetsFallbacksRun pins the user-facing
// error contract: known statuses become plain sentences, raw server text never
// reaches viewers, and unknown failures return "" so each caller's own
// fallback copy is what users see. Admin views keep the raw detail.
func TestReadableErrorHidesRawDetailAndLetsFallbacksRun(t *testing.T) {
	t.Parallel()
	result := runUIInvariantScript(t, []string{
		`function failure(status, message) { const error = new Error(message); error.status = status; return error; }`,
		`const raw = "request failed (502): {\"error\":\"upstream dial tcp 10.0.0.5:9191\"}";`,
		`const friendly = readableError(failure(502, raw));`,
		`const unknown = readableError(new Error("something odd"));`,
		`const admin = adminErrorDetail(failure(502, raw));`,
		`const timeout = new Error("Sports data took too long to respond. Try again."); timeout.timeout = true;`,
		`globalThis.__result = {`,
		`  stableResults: friendly.length > 0 && friendly.indexOf("request failed") < 0 && friendly.indexOf("10.0.0.5") < 0 && unknown === "" && (unknown || "fallback") === "fallback" && readableError(timeout) === timeout.message,`,
		`  separate: admin.indexOf(friendly) === 0 && admin.indexOf("10.0.0.5") > 0`,
		`};`,
	})
	if !result.StableResults {
		t.Fatal("readableError must map statuses to plain sentences, hide raw server text, and return \"\" for unknown failures")
	}
	if !result.Separate {
		t.Fatal("adminErrorDetail must keep the friendly sentence and append the raw detail")
	}
}
