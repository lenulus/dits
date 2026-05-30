/* sheet.js — keyboard + selection + interactivity island for the Sheet.
 *
 * Port of the interaction layer in app/sheet.jsx (§9.3). The server renders a
 * correct, static .sht table (frozen columns, pinned width) plus the toolbar /
 * filter chips / bulk bar; this script adds the ergonomics and HTMX glue:
 *   Arrows      move the focused cell
 *   Tab / ⇧Tab  move horizontally
 *   Enter       open the inline editor of the focused cell (HTMX hx-get swap)
 *   ⌘/Ctrl+Enter open the record panel for the focused row
 *   Space       toggle row selection
 *   Esc         cancel an open editor / clear selection
 * Click on an editable cell opens its editor; click on a nav cell opens the
 * panel; gutter checkboxes select and raise the bulk-action bar. The toolbar
 * Pivots (preset / group-by) and filter chips navigate the view URL with query
 * params (?preset / ?groupby / ?flt_<field>=<value>); the quick-add row + the
 * toolbar +New button POST to /partials/sheet/add via a hidden form.
 */
(function () {
  "use strict";

  /* ---- editable-cell helpers (HTMX does the actual fetch/swap) ---- */
  function openEditor(cell) {
    if (!cell || !cell.hasAttribute("data-editable")) return;
    if (cell.querySelector("form.is-editing")) return; // already editing this one
    cancelOpenEditors(); // only one editor open at a time
    cell.dataset.display = cell.innerHTML; // remember the display for cancel
    // The cell carries hx-get + hx-trigger="edit"; fire it (swaps the editor in).
    if (window.htmx) {
      window.htmx.trigger(cell, "edit");
    }
  }

  // cancelOpenEditors restores every cell with an open editor to its saved
  // display markup (no mutation), so editors never pile up across cells.
  function cancelOpenEditors(wrap) {
    var root = wrap || document;
    root.querySelectorAll("[data-editable]").forEach(function (cell) {
      if (cell.querySelector("form.is-editing") && cell.dataset.display != null) {
        cell.innerHTML = cell.dataset.display;
        delete cell.dataset.display;
      }
    });
  }

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
      var u = new URL(window.location.href);
      u.searchParams.set("open", id);
      window.location.assign(u.toString());
    }

    wrap.addEventListener("keydown", function (e) {
      // While an inline editor is open, let it own the keystrokes (Esc cancels).
      var editing = wrap.querySelector(".sht-cell .is-editing, .sht-cell.is-editing");
      if (editing) {
        if (e.key === "Escape") {
          e.preventDefault();
          cancelOpenEditors(wrap);
        }
        return;
      }
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
        openEditor(cellAt(focus.r, focus.c));
      }
      else if (e.key === " ") {
        e.preventDefault();
        var box = rs[focus.r] && rs[focus.r].querySelector("[data-select-row]");
        if (box) { box.checked = !box.checked; syncRowSel(box); }
      }
      else if (e.key === "Escape") {
        // Esc cancels an open editor first, otherwise clears selection.
        if (wrap.querySelector("[data-editable] form.is-editing")) cancelOpenEditors(wrap);
        else clearSelection();
      }
    });

    // Click an editable cell → open its editor (closing any other). Click a nav
    // cell → open the record panel. Click anywhere else → cancel open editors.
    wrap.addEventListener("click", function (e) {
      var nav = e.target.closest("[data-nav-cell]");
      var editable = e.target.closest("[data-editable]");
      if (nav && !editable) {
        cancelOpenEditors(wrap);
        openRow(nav.closest("tr[data-row-id]"));
        return;
      }
      if (editable) {
        // A click inside the already-open editor (its form) must not re-open.
        if (editable.querySelector("form.is-editing")) return;
        var tr = editable.closest("tr[data-row-id]");
        if (tr) {
          focus.r = rows().indexOf(tr);
          var cells = tr.querySelectorAll(".sht-cell");
          focus.c = Array.prototype.indexOf.call(cells, editable);
          paint();
        }
        openEditor(editable);
        return;
      }
      // Clicked a non-editable, non-nav area inside the sheet → cancel editors.
      cancelOpenEditors(wrap);
    });

    // A click anywhere outside this sheet cancels any open editor too.
    document.addEventListener("click", function (e) {
      if (!wrap.contains(e.target)) cancelOpenEditors(wrap);
    });

    /* ---- row selection + bulk bar ---- */
    function selectedIds() {
      return Array.prototype.filter
        .call(wrap.querySelectorAll("[data-select-row]"), function (b) { return b.checked; })
        .map(function (b) { return b.getAttribute("data-select-row"); });
    }
    function syncBulkBar() {
      var bar = document.querySelector("[data-bulkbar]");
      if (!bar) return;
      var ids = selectedIds();
      var cnt = bar.querySelector("[data-bulk-count]");
      if (cnt) cnt.textContent = String(ids.length);
      bar.hidden = ids.length === 0;
    }
    function syncRowSel(box) {
      var tr = box.closest("tr");
      if (tr) tr.classList.toggle("is-selected", box.checked);
      syncSelectAll();
      syncBulkBar();
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
      syncBulkBar();
    }

    // Quick-add row → POST to /partials/sheet/add with the view kind.
    var addKind = wrap.getAttribute("data-add-kind");
    wrap.querySelectorAll("[data-add-row]").forEach(function (row) {
      row.addEventListener("click", function () { quickAdd(addKind); });
    });

    // Column-group band toggle: flip THIS group within the full collapsed set
    // (read from the rendered bands) and navigate with ?collapsed=a,b,c so each
    // group folds independently. ?collapsed= (empty) means everything expanded.
    wrap.querySelectorAll("[data-band-toggle]").forEach(function (band) {
      band.addEventListener("click", function () {
        var key = band.getAttribute("data-group");
        if (!key) return;
        var set = [];
        wrap.querySelectorAll("[data-band-toggle]").forEach(function (b) {
          var k = b.getAttribute("data-group");
          if (!k) return;
          var collapsed = b.classList.contains("sht-band--collapsed");
          if (k === key) collapsed = !collapsed; // flip the clicked group
          if (collapsed) set.push(k);
        });
        navWith({ collapsed: set.join(","), group: null });
      });
    });

    // No initial paint(): the focus ring (a heavy inset border, doubly so on
    // the inline-editable title cell) should only appear once the user moves
    // with the keyboard or clicks — matching the prototype's clean load.
  }

  /* ---- toolbar: preset / group-by Pivots + filter chips ---- */
  function navWith(params) {
    var u = new URL(window.location.href);
    Object.keys(params).forEach(function (k) {
      if (params[k] === null) u.searchParams.delete(k);
      else u.searchParams.set(k, params[k]);
    });
    window.location.assign(u.toString());
  }

  function initToolbar(tb) {
    tb.querySelectorAll("[data-preset]").forEach(function (b) {
      // A preset reasserts its default collapsed-set, so drop any custom set.
      b.addEventListener("click", function () { navWith({ preset: b.getAttribute("data-preset"), collapsed: null }); });
    });
    tb.querySelectorAll("[data-groupby]").forEach(function (b) {
      b.addEventListener("click", function () { navWith({ groupby: b.getAttribute("data-groupby") }); });
    });
    // Remove a filter chip → drop its query param.
    tb.querySelectorAll("[data-chip-remove]").forEach(function (x) {
      x.addEventListener("click", function () {
        var p = {}; p["flt_" + x.getAttribute("data-chip-remove")] = null;
        navWith(p);
      });
    });
    var clearAll = tb.querySelector("[data-filter-clear]");
    if (clearAll) {
      clearAll.addEventListener("click", function () {
        var u = new URL(window.location.href);
        Array.prototype.slice.call(u.searchParams.keys())
          .filter(function (k) { return k.indexOf("flt_") === 0; })
          .forEach(function (k) { u.searchParams.delete(k); });
        window.location.assign(u.toString());
      });
    }
    // +Add filter: two-step popover (field → value).
    var addWrap = tb.querySelector("[data-filter-add]");
    if (addWrap) {
      var pop = addWrap.querySelector("[data-filter-pop]");
      var valuesTpl = tb.querySelector("[data-filter-values]");
      addWrap.querySelector(".flt-add").addEventListener("click", function () {
        if (pop) pop.hidden = !pop.hidden;
      });
      if (pop) {
        pop.querySelectorAll(".flt-pop__opt[data-field]").forEach(function (opt) {
          opt.addEventListener("click", function () {
            var field = opt.getAttribute("data-field");
            var kind = opt.getAttribute("data-kind");
            if (kind === "bool") { var p = {}; p["flt_" + field] = "true"; navWith(p); return; }
            showValueStep(pop, valuesTpl, field);
          });
        });
      }
    }
  }

  function showValueStep(pop, valuesTpl, field) {
    pop.innerHTML = '<span class="flt-pop__head"><span class="flt-pop__back">←</span> ' + field + "</span>";
    pop.querySelector(".flt-pop__back").addEventListener("click", function () {
      window.location.reload(); // simplest reset back to the field list
    });
    var src = valuesTpl && valuesTpl.content
      ? valuesTpl.content.querySelector('[data-field-values="' + field + '"]') : null;
    if (src) {
      src.querySelectorAll(".flt-pop__opt[data-value]").forEach(function (vo) {
        var clone = vo.cloneNode(true);
        clone.addEventListener("click", function () {
          var p = {}; p["flt_" + field] = vo.getAttribute("data-value"); navWith(p);
        });
        pop.appendChild(clone);
      });
    }
  }

  /* ---- bulk-action bar ---- */
  function initBulkBar(bar) {
    function ids() {
      return Array.prototype.filter
        .call(document.querySelectorAll("[data-select-row]"), function (b) { return b.checked; })
        .map(function (b) { return b.getAttribute("data-select-row"); });
    }
    bar.querySelectorAll("[data-bulk-action]").forEach(function (b) {
      b.addEventListener("click", function () {
        var action = b.getAttribute("data-bulk-action");
        var value = "";
        if (action === "reassign") value = window.prompt("Reassign pilot to actor id:") || "";
        else if (action === "quarter") value = window.prompt("Set quarter (e.g. 2026 Q3):") || "";
        else if (action === "classify") value = window.prompt("Classify into product node slug:") || "";
        if ((action === "reassign" || action === "quarter" || action === "classify") && !value) return;
        postBulk(action, value, ids());
      });
    });
    var clear = bar.querySelector("[data-bulk-clear]");
    if (clear) {
      clear.addEventListener("click", function () {
        document.querySelectorAll("[data-select-row]").forEach(function (b) {
          b.checked = false;
          var tr = b.closest("tr"); if (tr) tr.classList.remove("is-selected");
        });
        bar.hidden = true;
        var cnt = bar.querySelector("[data-bulk-count]"); if (cnt) cnt.textContent = "0";
      });
    }
  }

  function postBulk(action, value, ids) {
    var bar = document.querySelector("[data-sheet][data-bulk-url]") || document.querySelector("[data-bulk-url]");
    var url = bar ? bar.getAttribute("data-bulk-url") : "/partials/sheet/bulk";
    var body = new URLSearchParams();
    body.set("action", action);
    body.set("value", value);
    ids.forEach(function (id) { body.append("ids", id); });
    fetch(url, { method: "POST", headers: { "HX-Request": "true" }, body: body })
      .then(function (res) {
        if (res.headers.get("HX-Refresh") === "true" || res.ok) window.location.reload();
      });
  }

  /* ---- quick-add (+New) ---- */
  function quickAdd(kind) {
    if (!kind) return;
    var body = new URLSearchParams();
    body.set("kind", kind);
    fetch("/partials/sheet/add", { method: "POST", headers: { "HX-Request": "true" }, body: body })
      .then(function (res) {
        var dest = res.headers.get("HX-Redirect");
        if (dest) window.location.assign(dest);
        else if (res.ok) window.location.reload();
      });
  }

  function init() {
    document.querySelectorAll("[data-sheet]").forEach(initSheet);
    document.querySelectorAll("[data-toolbar]").forEach(initToolbar);
    document.querySelectorAll("[data-bulkbar]").forEach(initBulkBar);
    // Toolbar +New button (mirrors the quick-add row).
    document.querySelectorAll(".sht-toolbar [data-add-row]").forEach(function (b) {
      var sheet = document.querySelector("[data-sheet][data-add-kind]");
      var kind = sheet ? sheet.getAttribute("data-add-kind") : null;
      b.addEventListener("click", function () { quickAdd(kind); });
    });
  }
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
