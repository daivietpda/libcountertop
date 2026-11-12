(() => {
  const ask = (kind, raw) => window.countertopAuth(JSON.stringify({ kind, callback: JSON.parse(raw).callback }));
  const config = {
    hasSettingsScreen: false,
    canWriteTag: false,
    hasExoPlayer: false,
    canCommissionMatter: false,
    canImportThreadCredentials: false,
    hasAssist: false,
    hasBarCodeScanner: 0,
    canSetupImprov: false,
  };
  try {
    localStorage.setItem("dockedSidebar", JSON.stringify("always_hidden"));
  } catch (e) {}
  const css = ".header{display:none!important}";
  const root = () =>
    document
      .querySelector("home-assistant")
      ?.shadowRoot?.querySelector("home-assistant-main")
      ?.shadowRoot?.querySelector("partial-panel-resolver > *")
      ?.shadowRoot?.querySelector("hui-root")?.shadowRoot;
  setInterval(() => {
    document.documentElement.style.setProperty("--header-height", "0px");
    const r = root();
    if (!r || r.getElementById("countertop-full")) return;
    const s = document.createElement("style");
    s.id = "countertop-full";
    s.textContent = css;
    r.appendChild(s);
  }, 250);
  const typed = new Set(["", "text", "search", "email", "url", "tel", "password", "number"]);
  const deep = () => {
    let el = document.activeElement;
    while (el?.shadowRoot?.activeElement) el = el.shadowRoot.activeElement;
    return el;
  };
  const editable = (el) => {
    if (!el || el.disabled || el.readOnly) return false;
    if (el.isContentEditable || el.tagName === "TEXTAREA") return true;
    return el.tagName === "INPUT" && typed.has((el.getAttribute("type") || "").toLowerCase());
  };
  let touched = -Infinity;
  let open = false;
  const report = () => {
    const now = editable(deep()) && (open || performance.now() - touched < 1000);
    if (now === open) return;
    open = now;
    window.countertopKeyboard(now ? "1" : "0");
  };
  document.addEventListener("pointerdown", () => (touched = performance.now()), true);
  document.addEventListener("focusin", report, true);
  document.addEventListener("focusout", () => setTimeout(report, 50), true);
  window.addEventListener("resize", () => {
    if (open) deep()?.scrollIntoView({ block: "nearest" });
  });
  window.countertopHide = () => deep()?.blur();
  window.externalApp = {
    getExternalAuth: (raw) => ask("get", raw),
    revokeExternalAuth: (raw) => ask("revoke", raw),
    externalBus: (raw) => {
      const m = JSON.parse(raw);
      if (m.type === "config/get") {
        setTimeout(() => window.externalBus({ id: m.id, type: "result", success: true, result: config }));
      }
    },
  };
})();
