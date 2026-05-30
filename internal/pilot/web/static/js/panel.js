/* panel.js — slide-over record panel island.
 *
 * Port of the interaction layer in app/panels.jsx + ds/shared.jsx SlideOver
 * (§9.4). The server renders the panel open (when ?open=<id> is present);
 * this script wires close, tab switching, and Esc-to-close.
 *
 * Foundation: close removes ?open= and reloads; tab clicks set ?tab=<key>
 * (deep-link to a tab). Phase 5 swaps tab bodies in place via HTMX rather
 * than reloading.
 */
(function () {
  "use strict";

  function init() {
    var panel = document.querySelector("[data-panel]");

    // Esc closes the panel (when one is open). Mirrors App.jsx's global Esc.
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && panel && !document.querySelector(".cmd-overlay")) {
        closePanel();
      }
    });

    if (!panel) return;

    var closeBtn = panel.querySelector("[data-panel-close]");
    if (closeBtn) closeBtn.addEventListener("click", closePanel);

    panel.querySelectorAll("[data-panel-tab]").forEach(function (tab) {
      tab.addEventListener("click", function () {
        var key = tab.getAttribute("data-panel-tab");
        var u = new URL(window.location.href);
        u.searchParams.set("tab", key);
        window.location.assign(u.toString());
      });
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
