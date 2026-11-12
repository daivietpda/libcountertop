package lovelaceview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

const page = `
var calls = [], bus = [], stored = {}, timers = [];
var view = {
	children: [],
	getElementById(id) { return this.children.find((c) => c.id === id) || null; },
	appendChild(c) { this.children.push(c); },
};
var root = { style: { props: {}, setProperty(k, v) { this.props[k] = v; } } };
var ha = null;
function dashboardOpens() {
	var panel = { shadowRoot: { querySelector: (s) => (s === "hui-root" ? { shadowRoot: view } : null) } };
	var main = { shadowRoot: { querySelector: (s) => (s === "partial-panel-resolver > *" ? panel : null) } };
	ha = { shadowRoot: { querySelector: (s) => (s === "home-assistant-main" ? main : null) } };
}
var heard = {}, keys = [], clock = 0;
var document = {
	documentElement: root,
	activeElement: null,
	querySelector: (s) => (s === "home-assistant" ? ha : null),
	createElement: (tag) => ({ tag }),
	addEventListener(kind, f) { heard[kind] = f; },
};
var performance = { now: () => clock };
var localStorage = { setItem(k, v) { stored[k] = v; } };
function setInterval(f) { timers.push(f); }
function setTimeout(f) { f(); }
function tick() { timers.forEach((f) => f()); }
function field(tag, type) {
	return { tag, tagName: tag, getAttribute: (a) => (a === "type" ? type : null), blur() { document.activeElement = null; heard.focusout(); } };
}
function inside(el) { return { tagName: "HA-TEXTFIELD", shadowRoot: { activeElement: el } }; }
function touch(at) { clock = at; heard.pointerdown(); }
function focus(el, at) { clock = at; document.activeElement = el; heard.focusin(); }
function blur() { document.activeElement = null; heard.focusout(); }
var window = {
	countertopAuth(s) { calls.push(JSON.parse(s)); },
	countertopKeyboard(s) { keys.push(s); },
	externalBus(m) { bus.push(m); },
	addEventListener() {},
};
`

func loadBridge(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	if _, err := vm.RunString(page); err != nil {
		t.Fatalf("fake page: %v", err)
	}
	if _, err := vm.RunString(bridge); err != nil {
		t.Fatalf("bridge.js: %v", err)
	}
	return vm
}

func run(t *testing.T, vm *goja.Runtime, js string) string {
	t.Helper()
	v, err := vm.RunString("JSON.stringify(" + js + ")")
	if err != nil {
		t.Fatalf("%s: %v", js, err)
	}
	return v.String()
}

func do(t *testing.T, vm *goja.Runtime, js string) {
	t.Helper()
	if _, err := vm.RunString(js); err != nil {
		t.Fatalf("%s: %v", js, err)
	}
}

func TestBridgeHidesTheSidebar(t *testing.T) {
	vm := loadBridge(t)
	if got := run(t, vm, "stored.dockedSidebar"); got != `"\"always_hidden\""` {
		t.Errorf("dockedSidebar stored as %s", got)
	}
}

func TestBridgeAsksTheDeviceForSignIn(t *testing.T) {
	vm := loadBridge(t)
	do(
		t,
		vm,
		`window.externalApp.getExternalAuth(JSON.stringify({callback: "externalAuthSetToken"}))`,
	)
	do(
		t,
		vm,
		`window.externalApp.revokeExternalAuth(JSON.stringify({callback: "externalAuthRevokeToken"}))`,
	)
	want := `[{"kind":"get","callback":"externalAuthSetToken"},{"kind":"revoke","callback":"externalAuthRevokeToken"}]`
	if got := run(t, vm, "calls"); got != want {
		t.Errorf("binding calls %s, want %s", got, want)
	}
}

func TestBridgeOffersNoExtraAppFeatures(t *testing.T) {
	vm := loadBridge(t)
	do(t, vm, `window.externalApp.externalBus(JSON.stringify({id: 7, type: "config/get"}))`)
	var answers []struct {
		ID      int            `json:"id"`
		Type    string         `json:"type"`
		Success bool           `json:"success"`
		Result  map[string]any `json:"result"`
	}
	if err := json.Unmarshal([]byte(run(t, vm, "bus")), &answers); err != nil {
		t.Fatal(err)
	}
	if len(answers) != 1 || answers[0].ID != 7 || answers[0].Type != "result" ||
		!answers[0].Success {
		t.Fatalf("answers %+v", answers)
	}
	for _, feature := range []string{"hasSettingsScreen", "hasAssist", "canCommissionMatter", "canWriteTag"} {
		if on, ok := answers[0].Result[feature].(bool); !ok || on {
			t.Errorf("%s = %v", feature, answers[0].Result[feature])
		}
	}
}

func TestBridgeHidesTheHeaderOnceTheDashboardOpens(t *testing.T) {
	vm := loadBridge(t)
	do(t, vm, "tick()")
	if got := run(t, vm, "view.children.length"); got != "0" {
		t.Fatalf("styled %s children before the dashboard opened", got)
	}
	do(t, vm, "dashboardOpens(); tick(); tick();")
	if got := run(t, vm, "view.children.length"); got != "1" {
		t.Fatalf("added %s styles, want one", got)
	}
	css := run(t, vm, "view.children[0].textContent")
	if !strings.Contains(css, ".header{display:none") {
		t.Errorf("style %s", css)
	}
	if got := run(t, vm, `root.style.props["--header-height"]`); got != `"0px"` {
		t.Errorf("--header-height %s", got)
	}
}

func TestBridgeAsksForTheKeyboardWhenATouchFocusesAField(t *testing.T) {
	vm := loadBridge(t)
	do(t, vm, `touch(100); focus(inside(field("INPUT", "text")), 300);`)
	do(t, vm, `blur(); touch(5000); focus(field("TEXTAREA", null), 5200); blur();`)
	if got := run(t, vm, "keys"); got != `["1","0","1","0"]` {
		t.Errorf("keyboard asks %s", got)
	}
}

func TestBridgeLeavesTheKeyboardDownForFieldsFocusedWithoutATouch(t *testing.T) {
	vm := loadBridge(t)
	do(t, vm, `touch(100); focus(field("INPUT", "search"), 3000);`)
	if got := run(t, vm, "keys"); got != `[]` {
		t.Errorf("keyboard asks %s", got)
	}
}

func TestBridgeKeepsTheKeyboardMovingBetweenFields(t *testing.T) {
	vm := loadBridge(t)
	do(t, vm, `touch(100); focus(field("INPUT", "text"), 200);`)
	do(
		t,
		vm,
		`document.activeElement = field("INPUT", "number"); heard.focusout(); clock = 9000; heard.focusin();`,
	)
	if got := run(t, vm, "keys"); got != `["1"]` {
		t.Errorf("keyboard asks %s", got)
	}
}

func TestBridgeIgnoresFieldsThatTakeNoText(t *testing.T) {
	vm := loadBridge(t)
	do(t, vm, `touch(100); focus(field("INPUT", "checkbox"), 200);`)
	do(t, vm, `var off = field("INPUT", "text"); off.readOnly = true; focus(off, 250);`)
	if got := run(t, vm, "keys"); got != `[]` {
		t.Errorf("keyboard asks %s", got)
	}
}

func TestBridgeHidesByBlurringTheField(t *testing.T) {
	vm := loadBridge(t)
	do(t, vm, `touch(100); focus(inside(field("INPUT", "text")), 200);`)
	do(
		t,
		vm,
		`var host = document.activeElement; host.shadowRoot.activeElement.blur = function () { host.shadowRoot.activeElement = null; document.activeElement = null; heard.focusout(); }; window.countertopHide();`,
	)
	if got := run(t, vm, "keys"); got != `["1","0"]` {
		t.Errorf("keyboard asks %s", got)
	}
}
