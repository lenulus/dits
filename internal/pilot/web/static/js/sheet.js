/* sheet.js — keyboard + selection island for the Sheet primitive.
 *
 * Port of the interaction layer in app/sheet.jsx (§9.3). The server renders
 * a correct, static .sht table (frozen columns, pinned width); this script
 * adds the ergonomics on top:
 *   Arrows      move the focused cell
 *   Tab / ⇧Tab  move horizontally
 *   Enter       edit the focused cell (TODO phase 5: inline editors)
 *   ⌘/Ctrl+Enter open the record panel for the focused row
 *   Space       toggle row selection
 *   Esc         cancel edit / clear selection
 * Click on a row's nav cell opens the panel; the gutter checkboxes select.
 *
 * Foundation: navigation, selection highlighting, panel-open via location,
 * and column-group band toggle (?group= query param) are wired. Inline cell
 * editing and live mutation land with the MCP data step.
 */
(function () {
  "use strict";

  function initSheet(wrap) {
    var navCol = parseInt(wrap.getAttribute("data-nav-col") || "1", 10);
    var focus = { r: 0, c: navCol };

    function rows() {
      return Array.prototype.slice.call(wrap.querySelectorAll("tbody tr[data-row-id]"));
    }
    function cellAt(r, c) {
      var tr = rows()[r];
      if (!tr) return null;
      return tr.querySelectorAll(".sht-cell")[c] || null;
    }
    function maxCol() {
      var tr = rows()[0];
      return tr ? tr.querySelectorAll(".sht-cell").length - 1 : 0;
    }
    function paint() {
      wrap.querySelectorAll(".sht-cell.is-focus").forEach(function (el) {
        el.classList.remove("is-focus");
      });
      var cell = cellAt(focus.r, focus.c);
      if (cell) cell.classList.add("is-focus");
    }

    function openRow(tr) {
      var id = tr && tr.getAttribute("data-row-id");
      if (!id) return;
      // Foundation: navigate with ?open=<id> so the server can render the
      // panel open. (Phase 5: HTMX swap the panel in place instead.)
      var u = new URL(window.location.href);
      u.searchParams.set("open", id);
      window.location.assign(u.toString());
    }

    wrap.addEventListener("keydown", function (e) {
      var rs = rows();
      var max = rs.length - 1;
      var mc = maxCol();
      if (e.key === "ArrowDown") { e.preventDefault(); focus.r = Math.min(max, focus.r + 1); paint(); }
      else if (e.key === "ArrowUp") { e.preventDefault(); focus.r = Math.max(0, focus.r - 1); paint(); }
      else if (e.key === "ArrowRight") { e.preventDefault(); focus.c = Math.min(mc, focus.c + 1); paint(); }
      else if (e.key === "ArrowLeft") { e.preventDefault(); focus.c = Math.max(0, focus.c - 1); paint(); }
      else if (e.key === "Tab") {
        e.preventDefault();
        focus.c = e.shiftKey ? Math.max(0, focus.c - 1) : Math.min(mc, focus.c + 1);
        paint();
      }
      else if (e.key === "Enter") {
        e.preventDefault();
        if (e.metaKey || e.ctrlKey) { openRow(rs[focus.r]); return; }
        // TODO(phase 5): begin inline edit of the focused cell.
      }
      else if (e.key === " ") {
        e.preventDefault();
        var box = rs[focus.r] && rs[focus.r].querySelector("[data-select-row]");
        if (box) { box.checked = !box.checked; syncRowSel(box); }
      }
      else if (e.key === "Escape") {
        clearSelection();
      }
    });

    // Click a nav cell → open the panel.
    wrap.querySelectorAll("[data-nav-cell]").forEach(function (cell) {
      cell.addEventListener("click", function () {
        var tr = cell.closest("tr[data-row-id]");
        openRow(tr);
      });
    });

    // Row selection via gutter checkboxes.
    function syncRowSel(box) {
      var tr = box.closest("tr");
      if (tr) tr.classList.toggle("is-selected", box.checked);
      syncSelectAll();
    }
    wrap.querySelectorAll("[data-select-row]").forEach(function (box) {
      box.addEventListener("change", function () { syncRowSel(box); });
    });
    var selectAll = wrap.querySelector("[data-select-all]");
    function syncSelectAll() {
      if (!selectAll) return;
      var boxes = wrap.querySelectorAll("[data-select-row]");
      var checked = Array.prototype.filter.call(boxes, function (b) { return b.checked; }).length;
      selectAll.checked = boxes.length > 0 && checked === boxes.length;
      selectAll.indeterminate = checked > 0 && checked < boxes.length;
    }
    if (selectAll) {
      selectAll.addEventListener("change", function () {
        wrap.querySelectorAll("[data-select-row]").forEach(function (b) {
          b.checked = selectAll.checked; syncRowSel(b);
        });
      });
    }
    function clearSelection() {
      wrap.querySelectorAll("[data-select-row]").forEach(function (b) {
        b.checked = false; b.closest("tr").classList.remove("is-selected");
      });
      syncSelectAll();
    }

    // Column-group band toggle: navigate with ?group=<key> so the server
    // recomputes the collapsed set. (Phase 5: do this client-side / HTMX.)
    wrap.querySelectorAll("[data-band-toggle]").forEach(function (band) {
      band.addEventListener("click", function () {
        var key = band.getAttribute("data-group");
        if (!key) return;
        var u = new URL(window.location.href);
        u.searchParams.set("group", key);
        window.location.assign(u.toString());
      });
    });

    paint();
  }

  function init() {
    document.querySelectorAll("[data-sheet]").forEach(initSheet);
  }
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
