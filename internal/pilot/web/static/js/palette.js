/* palette.js — ⌘K command palette island.
 *
 * Port of app/panels.jsx CmdPalette (§9.2). ⌘/Ctrl+K opens a fuzzy navigator.
 * It indexes three kinds of command:
 *   - NAVIGATE — the twelve views (read from the #pilot-nav JSON the layout
 *     emits). Always present, filtered client-side.
 *   - records  — MILESTONES / RFCS / DECISIONS, fetched (debounced) from
 *     /partials/palette/search?q=…; selecting one navigates to its panel.
 *   - CREATE   — New milestone / RFC / DecisionBlock; selecting one POSTs to
 *     /partials/palette/create and follows the HX-Redirect to the new record.
 *
 * Arrow keys move, Enter runs, Esc/overlay-click closes.
 */
(function () {
  "use strict";

  var NAV = [];
  try {
    var el = document.getElementById("pilot-nav");
    if (el) NAV = JSON.parse(el.textContent);
  } catch (e) { NAV = []; }

  // Static Create actions (kind is the word /partials/palette/create expects).
  var CREATE = [
    { label: "New milestone", kind: "milestone" },
    { label: "New RFC", kind: "rfc" },
    { label: "Open DecisionBlock", kind: "decision" },
  ];

  var mount = document.getElementById("cmd-mount");
  var open = false;
  var active = 0;
  var query = "";
  var records = []; // [{label, sub, icon, href}] from the search endpoint
  var results = []; // flat list of runnable rows, rebuilt by recompute()
  var searchTimer = null;
  var searchSeq = 0; // guards against out-of-order async responses

  function svgSearch() {
    return '<svg viewBox="0 0 24 24" width="16" height="16" style="stroke:currentColor;stroke-width:1.5;fill:none;stroke-linecap:round;stroke-linejoin:round"><circle cx="11" cy="11" r="7"/><path d="M21 21l-5-5"/></svg>';
  }

  // recompute rebuilds the flat results list (NAVIGATE + records + CREATE) and
  // the group boundaries, applying the query filter to the nav + create rows
  // (records are already server-filtered).
  function recompute() {
    var q = query.toLowerCase();
    var rows = [];

    NAV.filter(function (r) {
      return !q || r.label.toLowerCase().indexOf(q) >= 0 || r.path.toLowerCase().indexOf(q) >= 0;
    }).forEach(function (r) {
      rows.push({ group: "NAVIGATE", label: r.label, sub: r.path, kind: "nav", path: r.path });
    });

    records.forEach(function (r) {
      rows.push({ group: r.group || "RECORDS", label: r.label, sub: r.sub, kind: "open", path: r.href });
    });

    CREATE.filter(function (c) {
      return !q || c.label.toLowerCase().indexOf(q) >= 0;
    }).forEach(function (c) {
      rows.push({ group: "CREATE", label: c.label, sub: "+", kind: "create", createKind: c.kind });
    });

    results = rows;
    if (active >= results.length) active = Math.max(0, results.length - 1);
  }

  function render() {
    if (!open) { mount.innerHTML = ""; return; }

    // Group adjacent rows under their group label.
    var html = "";
    var lastGroup = null;
    results.forEach(function (r, i) {
      if (r.group !== lastGroup) {
        html += '<div class="cmd__group-lab">' + escapeHtml(r.group) + "</div>";
        lastGroup = r.group;
      }
      html += '<div class="cmd__opt' + (i === active ? " is-active" : "") + '" data-idx="' + i + '">' +
        '<span style="flex:1">' + escapeHtml(r.label) + "</span>" +
        '<span class="sub">' + escapeHtml(r.sub || "") + "</span></div>";
    });
    if (!html) {
      html = '<div style="padding:14px 18px;font-family:var(--font-serif);font-style:italic;color:var(--ink-3);font-size:13px">Nothing matches. Try shorter terms — IDs and titles both index.</div>';
    }

    mount.innerHTML =
      '<div class="cmd-overlay" data-cmd-overlay>' +
      '  <div class="cmd" data-cmd>' +
      '    <div class="cmd__input">' + svgSearch() +
      '      <input autofocus placeholder="Jump anywhere · create anything · type to filter…" data-cmd-input value="' + escapeHtml(query) + '">' +
      '      <span class="cmd__hint">esc closes</span>' +
      "    </div>" +
      '    <div class="cmd__list">' + html + "</div>" +
      "  </div>" +
      "</div>";
    wire();
    var input = mount.querySelector("[data-cmd-input]");
    if (input) {
      input.focus();
      // Keep the caret at the end after a re-render driven by typing.
      var v = input.value;
      input.value = "";
      input.value = v;
    }
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  // fetchRecords queries the server for record matches (debounced by the caller)
  // and re-renders when they arrive, ignoring stale responses.
  function fetchRecords() {
    var seq = ++searchSeq;
    var url = "/partials/palette/search?q=" + encodeURIComponent(query);
    fetch(url, { headers: { "HX-Request": "true" } })
      .then(function (res) { return res.ok ? res.json() : { groups: [] }; })
      .then(function (data) {
        if (seq !== searchSeq || !open) return; // superseded or closed
        records = [];
        (data.groups || []).forEach(function (g) {
          (g.items || []).forEach(function (it) {
            records.push({ group: g.label, label: it.label, sub: it.sub, icon: it.icon, href: it.href });
          });
        });
        recompute();
        render();
      })
      .catch(function () { /* leave records as-is on failure */ });
  }

  function scheduleSearch() {
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(fetchRecords, 140);
  }

  function setQuery(q) {
    query = q || "";
    recompute();
    render();
    scheduleSearch();
  }

  function run(i) {
    var r = results[i];
    if (!r) return;
    if (r.kind === "create") {
      createRecord(r.createKind);
      return;
    }
    if (r.path) window.location.assign(r.path);
  }

  // createRecord POSTs to the create endpoint and follows the HX-Redirect the
  // server returns (the new record's panel URL).
  function createRecord(kind) {
    var body = new URLSearchParams();
    body.set("kind", kind);
    fetch("/partials/palette/create", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "HX-Request": "true" },
      body: body.toString(),
    }).then(function (res) {
      var to = res.headers.get("HX-Redirect");
      if (to) window.location.assign(to);
    }).catch(function () { /* swallow — palette stays open on failure */ });
  }

  function openPalette() {
    open = true;
    active = 0;
    query = "";
    records = [];
    recompute();
    render();
    fetchRecords(); // seed the record groups with an empty query
  }
  function closePalette() {
    open = false;
    if (searchTimer) { clearTimeout(searchTimer); searchTimer = null; }
    render();
  }

  function wire() {
    var overlay = mount.querySelector("[data-cmd-overlay]");
    if (overlay) {
      overlay.addEventListener("click", function (e) {
        if (e.target === overlay) closePalette();
      });
    }
    var input = mount.querySelector("[data-cmd-input]");
    if (input) {
      input.addEventListener("input", function () { setQuery(input.value); });
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
