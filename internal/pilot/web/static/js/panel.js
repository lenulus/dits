/* panel.js — slide-over record panel island.
 *
 * Port of the interaction layer in app/panels.jsx + ds/shared.jsx SlideOver
 * (§9.4). The server renders the panel open (when ?open=<id> is present);
 * this script wires close, tab switching, and Esc-to-close.
 *
 * Tab clicks fetch the tab body fragment from the panel-partial endpoint via
 * htmx and swap it into [data-panel-body] in place — no full-page reload — and
 * keep the ?tab=<key> deep-link in the URL in sync. Mutations inside the body
 * are htmx posts that swap the same region (see panels_detail.go); on success
 * we refresh the active tab so derived state (rollups, RYG, feeds) re-renders.
 */
(function () {
  "use strict";

  function init() {
    var panel = document.querySelector("[data-panel]");

    // Esc closes the panel (when one is open). Mirrors App.jsx's global Esc.
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && panel && !document.querySelector(".cmd-overlay")) {
        var t = e.target;
        if (t && (t.tagName === "TEXTAREA" || t.tagName === "INPUT" || t.tagName === "SELECT")) return;
        closePanel();
      }
    });

    if (!panel) return;

    var closeBtn = panel.querySelector("[data-panel-close]");
    if (closeBtn) closeBtn.addEventListener("click", closePanel);

    panel.querySelectorAll("[data-panel-tab]").forEach(function (tab) {
      tab.addEventListener("click", function () {
        switchTab(panel, tab.getAttribute("data-panel-tab"), tab.getAttribute("data-panel-id"));
      });
    });

    // ⌘/Ctrl+Enter submits a composer textarea (status / quick-update). The
    // body is re-rendered after htmx swaps, so delegate from the panel root.
    panel.addEventListener("keydown", function (e) {
      if (e.key !== "Enter" || !(e.metaKey || e.ctrlKey)) return;
      var t = e.target;
      if (!t || t.tagName !== "TEXTAREA" || !t.hasAttribute("data-cmd-enter-submit")) return;
      e.preventDefault();
      var form = t.closest("form");
      if (form) (form.requestSubmit ? form.requestSubmit() : form.submit());
    });
  }

  // switchTab swaps just the panel body to the requested tab via htmx, marks
  // the tab active, and reflects ?tab=<key> in the URL without a reload.
  function switchTab(panel, key, id) {
    var body = panel.querySelector("[data-panel-body]");
    if (!body || !window.htmx || !id) {
      // No htmx (or no id): fall back to the reload deep-link.
      var u0 = new URL(window.location.href);
      u0.searchParams.set("tab", key);
      window.location.assign(u0.toString());
      return;
    }
    panel.querySelectorAll("[data-panel-tab]").forEach(function (t) {
      t.classList.toggle("is-active", t.getAttribute("data-panel-tab") === key);
    });
    var u = new URL(window.location.href);
    u.searchParams.set("tab", key);
    window.history.replaceState({}, "", u.toString());
    window.htmx.ajax("GET", "/partials/panel/" + encodeURIComponent(id) + "/tab/" + encodeURIComponent(key), {
      target: body,
      swap: "innerHTML",
    });
  }

  function closePanel() {
    var u = new URL(window.location.href);
    u.searchParams.delete("open");
    u.searchParams.delete("tab");
    window.location.assign(u.toString());
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
