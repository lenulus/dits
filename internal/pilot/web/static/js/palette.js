/* palette.js — ⌘K command palette island.
 *
 * Port of app/panels.jsx CmdPalette (§9.2). ⌘/Ctrl+K opens a fuzzy
 * navigator over the twelve views (read from the #pilot-nav JSON the layout
 * emits). Arrow keys move, Enter navigates, Esc/overlay-click closes.
 *
 * Foundation: navigation entries only. Phase 5 adds record jump-in and
 * create actions (sourced from the MCP client) to the same list.
 */
(function () {
  "use strict";

  var NAV = [];
  try {
    var el = document.getElementById("pilot-nav");
    if (el) NAV = JSON.parse(el.textContent);
  } catch (e) { NAV = []; }

  var mount = document.getElementById("cmd-mount");
  var open = false;
  var active = 0;
  var results = NAV.slice();

  function svgSearch() {
    return '<svg viewBox="0 0 24 24" width="16" height="16" style="stroke:currentColor;stroke-width:1.5;fill:none;stroke-linecap:round;stroke-linejoin:round"><circle cx="11" cy="11" r="7"/><path d="M21 21l-5-5"/></svg>';
  }

  function render() {
    if (!open) { mount.innerHTML = ""; return; }
    var opts = results.map(function (r, i) {
      return '<div class="cmd__opt' + (i === active ? " is-active" : "") + '" data-idx="' + i + '">' +
        '<span style="flex:1">' + escapeHtml(r.label) + '</span>' +
        '<span class="sub">' + escapeHtml(r.path) + '</span></div>';
    }).join("");
    if (!opts) {
      opts = '<div style="padding:14px 18px;font-family:var(--font-serif);font-style:italic;color:var(--ink-3);font-size:13px">Nothing matches. Try shorter terms.</div>';
    }
    mount.innerHTML =
      '<div class="cmd-overlay" data-cmd-overlay>' +
      '  <div class="cmd" data-cmd>' +
      '    <div class="cmd__input">' + svgSearch() +
      '      <input autofocus placeholder="Jump anywhere · type to filter…" data-cmd-input>' +
      '      <span class="cmd__hint">esc closes</span>' +
      '    </div>' +
      '    <div class="cmd__list"><div class="cmd__group-lab">NAVIGATE</div>' + opts + '</div>' +
      '  </div>' +
      '</div>';
    wire();
    var input = mount.querySelector("[data-cmd-input]");
    if (input) input.focus();
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  function filter(q) {
    q = (q || "").toLowerCase();
    results = q
      ? NAV.filter(function (r) {
          return r.label.toLowerCase().indexOf(q) >= 0 || r.path.toLowerCase().indexOf(q) >= 0;
        })
      : NAV.slice();
    active = 0;
  }

  function run(i) {
    var r = results[i];
    if (r) window.location.assign(r.path);
  }

  function openPalette() { open = true; filter(""); render(); }
  function closePalette() { open = false; render(); }

  function wire() {
    var overlay = mount.querySelector("[data-cmd-overlay]");
    if (overlay) {
      overlay.addEventListener("click", function (e) {
        if (e.target === overlay) closePalette();
      });
    }
    var input = mount.querySelector("[data-cmd-input]");
    if (input) {
      input.addEventListener("input", function () { filter(input.value); render(); });
    }
    mount.querySelectorAll(".cmd__opt").forEach(function (opt) {
      opt.addEventListener("mouseenter", function () { active = parseInt(opt.getAttribute("data-idx"), 10); });
      opt.addEventListener("click", function () { run(parseInt(opt.getAttribute("data-idx"), 10)); });
    });
  }

  document.addEventListener("keydown", function (e) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      open ? closePalette() : openPalette();
      return;
    }
    if (!open) return;
    if (e.key === "Escape") { e.preventDefault(); closePalette(); }
    else if (e.key === "ArrowDown") { e.preventDefault(); active = Math.min(results.length - 1, active + 1); render(); }
    else if (e.key === "ArrowUp") { e.preventDefault(); active = Math.max(0, active - 1); render(); }
    else if (e.key === "Enter") { e.preventDefault(); run(active); }
  });

  // Clicking the topbar search box also opens the palette.
  var search = document.getElementById("rx-search");
  if (search) search.addEventListener("click", openPalette);
})();
