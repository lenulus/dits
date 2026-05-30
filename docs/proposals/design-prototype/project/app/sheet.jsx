/* Sheet — the ergonomics workhorse.
   Click cell → edits. ⌫/Esc cancels. Enter commits + moves down. Tab/⇧Tab moves horiz.
   Arrow keys navigate cells. Shift+arrows extends selection on first column.
   Frozen left columns. Per-column sort + filter. Inline quick-add row.

   columns: [{key, label, width, frozen, kind, options, render, edit, align, num, sortable, filterable, readonly}]
   rows: opaque objects with row.id
   onUpdate(id, patch), onAddRow(draft), onSelectRow(id) — open detail.
*/

function Sheet({
  columns, rows, onUpdate, onAddRow, onSelectRow, activeId,
  selectedIds, setSelectedIds,
  groupBy, groups,           // groups: [{key, rows}]
  rowKey = r => r.id,
  navColumn = 1,             // which column index opens the detail panel on click
  addLabel = 'New row…',
  cmdAction,                 // hook to register cell ops with cmd palette
  columnGroups,              // [{key,label,alwaysOpen?,summary?}]
  collapsedGroups,           // Set<string>
  onToggleGroup,             // (groupKey) => void
}) {
  const [editing, setEditing] = useState(null);   // {rowIdx, colIdx}
  const [focus, setFocus]     = useState({ r: 0, c: navColumn });
  const [sort, setSort]       = useState(null);   // {col, dir}
  const [filters, setFilters] = useState({});     // {colKey: value}
  const [filterOpen, setFilterOpen] = useState(null); // colKey or null
  const tableRef = React.useRef();

  // Compute display columns: collapse groups whose key is in collapsedGroups into a single summary column.
  const displayColumns = useMemo(() => {
    if (!columnGroups || columnGroups.length === 0) return columns;
    const byGroup = {};
    for (const c of columns) {
      const g = c.group || '_default';
      (byGroup[g] = byGroup[g] || []).push(c);
    }
    const out = [];
    for (const g of columnGroups) {
      const collapsed = collapsedGroups && collapsedGroups.has(g.key) && !g.alwaysOpen;
      if (collapsed) {
        const firstCol = (byGroup[g.key] || [])[0];
        out.push({
          key: '__group_' + g.key,
          label: g.label,
          width: g.summaryWidth || 132,
          readonly: true,
          sortable: false,
          filterable: false,
          frozen: firstCol?.frozen || false,
          _isGroupSummary: true,
          _groupKey: g.key,
          render: (_v, ctx) => g.summary ? g.summary(ctx.row, ctx) : <span className="placeholder">—</span>,
        });
      } else {
        (byGroup[g.key] || []).forEach(c => out.push({ ...c, _groupKey: g.key }));
      }
    }
    // Any ungrouped columns
    (byGroup['_default'] || []).forEach(c => out.push(c));
    return out;
  }, [columns, columnGroups, collapsedGroups]);

  // Toggle selection
  function toggleSel(id, idx, e) {
    if (e && e.shiftKey && selectedIds && selectedIds.lastIdx != null) {
      const [a,b] = [Math.min(selectedIds.lastIdx, idx), Math.max(selectedIds.lastIdx, idx)];
      const next = new Set(selectedIds);
      for (let i=a;i<=b;i++) next.add(rowKey(rows[i]));
      next.lastIdx = idx;
      setSelectedIds(next);
      return;
    }
    const next = new Set(selectedIds);
    next.has(id) ? next.delete(id) : next.add(id);
    next.lastIdx = idx;
    setSelectedIds(next);
  }
  function selectAll() {
    if (!selectedIds) return;
    const ids = rows.map(rowKey);
    const all = ids.every(i => selectedIds.has(i));
    setSelectedIds(all ? new Set() : new Set(ids));
  }

  // Sort
  const displayedRows = useMemo(() => {
    let rs = rows;
    // Filter
    for (const [k, v] of Object.entries(filters)) {
      if (!v) continue;
      rs = rs.filter(r => String(r[k] ?? '').toLowerCase().includes(String(v).toLowerCase())
                       || String(r[k] ?? '') === v);
    }
    // Sort
    if (sort && sort.col) {
      const dir = sort.dir === 'desc' ? -1 : 1;
      rs = [...rs].sort((a,b) => {
        const av = a[sort.col], bv = b[sort.col];
        if (av == null) return 1;
        if (bv == null) return -1;
        return (av > bv ? 1 : av < bv ? -1 : 0) * dir;
      });
    }
    return rs;
  }, [rows, sort, filters]);

  // Group
  const groupedDisplay = useMemo(() => {
    if (!groupBy) return [{ key: null, rows: displayedRows }];
    const m = new Map();
    for (const r of displayedRows) {
      const k = r[groupBy] || '—';
      if (!m.has(k)) m.set(k, []);
      m.get(k).push(r);
    }
    return Array.from(m, ([key, rs]) => ({ key, rows: rs }));
  }, [displayedRows, groupBy]);

  function flatIdx() {
    const out = [];
    groupedDisplay.forEach((g, gi) => g.rows.forEach((r, ri) => out.push({ row: r, gi, ri })));
    return out;
  }

  // Cell key handler
  function onKey(e) {
    if (editing) return; // editor handles
    const flat = flatIdx();
    const max = flat.length - 1;
    if (e.key === 'ArrowDown')  { e.preventDefault(); setFocus(f => ({ r: Math.min(max, f.r+1), c: f.c })); }
    else if (e.key === 'ArrowUp')   { e.preventDefault(); setFocus(f => ({ r: Math.max(0, f.r-1), c: f.c })); }
    else if (e.key === 'ArrowRight'){ e.preventDefault(); setFocus(f => ({ r: f.r, c: Math.min(displayColumns.length-1, f.c+1) })); }
    else if (e.key === 'ArrowLeft') { e.preventDefault(); setFocus(f => ({ r: f.r, c: Math.max(0, f.c-1) })); }
    else if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      const col = displayColumns[focus.c];
      const row = flat[focus.r]?.row;
      if (!col || !row) return;
      if (e.metaKey || e.ctrlKey) { onSelectRow && onSelectRow(rowKey(row)); return; }
      if (col.readonly) return;
      setEditing({ rowKey: rowKey(row), colKey: col.key });
    }
    else if (e.key === ' ' && (e.target.tagName !== 'INPUT')) {
      e.preventDefault();
      const row = flat[focus.r]?.row;
      if (row && selectedIds) toggleSel(rowKey(row), focus.r);
    }
  }

  function startEdit(row, col) {
    if (col.readonly || !onUpdate) return;
    setEditing({ rowKey: rowKey(row), colKey: col.key });
  }
  function commitEdit(row, col, val) {
    onUpdate && onUpdate(rowKey(row), { [col.key]: val });
    setEditing(null);
  }
  function cancelEdit() { setEditing(null); }

  function clickHeader(col) {
    if (!col.sortable) return;
    setSort(s => {
      if (!s || s.col !== col.key) return { col: col.key, dir: 'asc' };
      if (s.dir === 'asc') return { col: col.key, dir: 'desc' };
      return null;
    });
  }

  function HeaderFilter({ col }) {
    if (filterOpen !== col.key) return null;
    const optionSet = new Set();
    rows.forEach(r => { if (r[col.key] != null) optionSet.add(r[col.key]); });
    const opts = Array.from(optionSet);
    return (
      <div className="sht-filter-pop"
        onClick={e => e.stopPropagation()}>
        <input className="sht-filter-input" placeholder="Filter…"
          autoFocus value={filters[col.key] || ''}
          onChange={e => setFilters({ ...filters, [col.key]: e.target.value })}
          onKeyDown={e => { if (e.key === 'Escape') setFilterOpen(null); if (e.key === 'Enter') setFilterOpen(null); }}/>
        <div className="sht-filter-opts">
          {opts.slice(0, 8).map(o => (
            <div key={String(o)} className="sht-filter-opt"
              onClick={() => { setFilters({ ...filters, [col.key]: String(o) }); setFilterOpen(null); }}>
              {col.render ? col.render(o, {filter:true}) : String(o)}
            </div>
          ))}
          {(filters[col.key]) && <div className="sht-filter-clear"
            onClick={() => { const { [col.key]:_, ...rest } = filters; setFilters(rest); setFilterOpen(null); }}>Clear filter</div>}
        </div>
      </div>
    );
  }

  // Render a cell — editor when in editing, otherwise display
  function CellRenderer({ row, rowIdx, col, colIdx }) {
    const isEditing = editing && editing.rowKey === rowKey(row) && editing.colKey === col.key;
    const isFocus   = focus.r === rowIdx && focus.c === colIdx;
    const val = row[col.key];

    // Custom edit component (e.g. multi-actor pickers)
    if (isEditing && col.edit) {
      return col.edit({
        value: val, row,
        commit: v => commitEdit(row, col, v),
        cancel: cancelEdit,
      });
    }
    // Inline text/select editor
    if (isEditing) {
      return (
        <CellEditor kind={col.kind} options={col.options}
          value={val}
          onCommit={v => commitEdit(row, col, v)}
          onCancel={cancelEdit}
          align={col.align}/>
      );
    }

    const showVal = col.render
      ? col.render(val, { row })
      : (val == null || val === '' ? <span className="placeholder">{col.placeholder || '—'}</span> : val);

    const handleClick = (e) => {
      if (colIdx === navColumn && onSelectRow && !col.readonly) {
        // primary nav column: title click opens detail panel
        onSelectRow(rowKey(row));
        setFocus({ r: rowIdx, c: colIdx });
      } else if (col.readonly) {
        setFocus({ r: rowIdx, c: colIdx });
      } else {
        setFocus({ r: rowIdx, c: colIdx });
        startEdit(row, col);
      }
    };
    return (
      <div className={'sht-cell' + (col.align === 'right' ? ' is-right' : '') + (col.num ? ' is-num' : '') + (col.readonly ? ' is-readonly' : '') + (isFocus ? ' is-focus' : '')}
        onClick={handleClick}>
        {showVal}
      </div>
    );
  }

  // Header
  const colCount = displayColumns.length + (selectedIds ? 1 : 0);

  return (
    <div className="sht-wrap" tabIndex={0} onKeyDown={onKey} ref={tableRef}>
      <table className="sht" style={{width: totalWidth(displayColumns, !!selectedIds) + 'px'}}>
        <colgroup>
          {selectedIds && <col style={{width:'32px'}}/>}
          {displayColumns.map((c, i) => <col key={i} style={{ width: (c.width || 140) + 'px' }}/>)}
        </colgroup>
        <thead>
          {columnGroups && (
            <tr className="sht-bands">
              {selectedIds && <th className="sht-band sht-band--gutter sht-gutter"/>}
              {(() => {
                const bands = [];
                let pos = 0;
                for (const g of columnGroups) {
                  const groupCols = displayColumns.filter(c => c._groupKey === g.key);
                  const span = groupCols.length;
                  if (span === 0) continue;
                  const collapsed = collapsedGroups && collapsedGroups.has(g.key) && !g.alwaysOpen;
                  const isFrozen = groupCols.every(c => c.frozen);
                  const left = isFrozen ? frozenLeftOf(pos, displayColumns, !!selectedIds) : undefined;
                  bands.push(
                    <th key={g.key} colSpan={span}
                      className={'sht-band' + (collapsed ? ' sht-band--collapsed' : '') + (isFrozen ? ' sht-band--frozen' : '') + (g.alwaysOpen ? ' sht-band--locked' : '')}
                      style={isFrozen ? { left } : undefined}
                      onClick={() => !g.alwaysOpen && onToggleGroup && onToggleGroup(g.key)}>
                      <span className="sht-band__lab">{g.label}</span>
                      {!g.alwaysOpen && (
                        <span className="sht-band__tog">{collapsed ? '\u25b8' : '\u25be'}</span>
                      )}
                    </th>
                  );
                  pos += span;
                }
                return bands;
              })()}
            </tr>
          )}
          <tr>
            {selectedIds && (
              <th className="sht-gutter">
                <input type="checkbox"
                  checked={rows.length > 0 && rows.every(r => selectedIds.has(rowKey(r)))}
                  onChange={selectAll}/>
              </th>
            )}
            {displayColumns.map((c, i) => {
              const arrow = sort && sort.col === c.key ? (sort.dir === 'asc' ? '\u2191' : '\u2193') : null;
              const filtered = filters[c.key];
              return (
                <th key={c.key}
                  className={(c.sortable !== false ? 'is-sortable ' : '') + (c.frozen ? 'is-frozen ' : '') + (c._isGroupSummary ? 'is-summary ' : '') + (filtered ? 'is-filtered' : '')}
                  style={c.frozen ? { left: frozenLeftOf(i, displayColumns, !!selectedIds) } : undefined}>
                  <div className="sht-th-row">
                    <span className="sht-th-lab" onClick={() => clickHeader(c)}>{c.label}</span>
                    {arrow && <span className="sht-th-arrow" onClick={() => clickHeader(c)}>{arrow}</span>}
                    {c.filterable !== false && !c._isGroupSummary && (
                      <span className="sht-th-filt" onClick={() => setFilterOpen(filterOpen === c.key ? null : c.key)} title="Filter"><I.dot3 size={11}/></span>
                    )}
                  </div>
                  <HeaderFilter col={c}/>
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody>
          {(() => {
            let rowIdx = -1;
            const out = [];
            groupedDisplay.forEach((g, gi) => {
              if (g.key !== null) {
                out.push(
                  <tr key={'gh-'+gi} className="sht-group">
                    <td colSpan={colCount}>
                      <span className="sht-group__lab">{g.key}</span>
                      <span className="sht-group__count">{g.rows.length}</span>
                    </td>
                  </tr>
                );
              }
              g.rows.forEach((row) => {
                rowIdx += 1;
                const id = rowKey(row);
                const isSel = selectedIds && selectedIds.has(id);
                const isAct = id === activeId;
                const ri = rowIdx;
                out.push(
                  <tr key={id} className={(isSel ? 'is-selected ' : '') + (isAct ? 'is-active' : '')}>
                    {selectedIds && (
                      <td className="sht-gutter">
                        <input type="checkbox" checked={isSel} onChange={(e) => toggleSel(id, ri, e.nativeEvent)} onClick={e => e.stopPropagation()}/>
                      </td>
                    )}
                    {displayColumns.map((c, ci) => (
                      <td key={c.key}
                        className={(c.frozen ? 'is-frozen ' : '') + (c.num ? 'is-num' : '') + (c._isGroupSummary ? ' is-summary' : '')}
                        style={c.frozen ? { left: frozenLeftOf(ci, displayColumns, !!selectedIds) } : undefined}>
                        <CellRenderer row={row} rowIdx={ri} col={c} colIdx={ci}/>
                      </td>
                    ))}
                  </tr>
                );
              });
            });
            return out;
          })()}
          {onAddRow && (
            <tr className="sht-add" onClick={onAddRow}>
              {selectedIds && <td className="sht-gutter"></td>}
              <td colSpan={displayColumns.length}>
                <div className="sht-cell"><I.plus size={11}/>&nbsp;{addLabel} <span className="sht-add-key">N</span></div>
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

function totalWidth(columns, hasGutter) {
  let w = hasGutter ? 32 : 0;
  for (const c of columns) w += (c.width || 140);
  return w;
}

function frozenLeftOf(idx, columns, hasGutter) {
  let left = hasGutter ? 32 : 0;
  for (let i = 0; i < idx; i++) {
    if (columns[i].frozen) left += (columns[i].width || 140);
    else break;
  }
  return left;
}

function frozenLeftOfDisplayed(idx, displayColumns, hasGutter) {
  return frozenLeftOf(idx, displayColumns, hasGutter);
}

/* CellEditor — text or select, auto-commit on blur, Enter, Tab */
function CellEditor({ kind, options, value, onCommit, onCancel, align }) {
  const [draft, setDraft] = useState(value ?? '');
  const ref = React.useRef();
  useEffect(() => { if (ref.current) { ref.current.focus(); if (ref.current.select) ref.current.select(); } }, []);
  function onKey(e) {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); onCommit(draft); }
    else if (e.key === 'Escape') { e.preventDefault(); onCancel(); }
    else if (e.key === 'Tab') { onCommit(draft); }
  }
  if (kind === 'select') {
    return (
      <div className="sht-cell is-editing">
        <select ref={ref} value={draft || ''} onChange={e => { setDraft(e.target.value); }} onBlur={() => onCommit(draft)} onKeyDown={onKey}>
          {!draft && <option value=""></option>}
          {options.map(o => <option key={o.value ?? o} value={o.value ?? o}>{o.label ?? o}</option>)}
        </select>
      </div>
    );
  }
  return (
    <div className={'sht-cell is-editing' + (align === 'right' ? ' is-right' : '')}>
      <input ref={ref} value={draft} onChange={e => setDraft(e.target.value)} onBlur={() => onCommit(draft)} onKeyDown={onKey}/>
    </div>
  );
}

/* Inline actor picker — popover that searches actor list */
function ActorPicker({ value, commit, cancel, agentOk }) {
  const [q, setQ] = useState('');
  const ref = React.useRef();
  useEffect(() => { if (ref.current) ref.current.focus(); }, []);
  const list = Object.values(window.DATA.ACTORS).filter(a => agentOk || !a.agent)
    .filter(a => !q || (a.name.toLowerCase().includes(q.toLowerCase()) || a.role.toLowerCase().includes(q.toLowerCase())));
  return (
    <div className="sht-cell is-editing">
      <div className="ap">
        <input ref={ref} className="ap-input" placeholder="Search actor…" value={q}
          onChange={e => setQ(e.target.value)}
          onKeyDown={e => { if (e.key === 'Escape') cancel(); if (e.key === 'Enter' && list[0]) commit(list[0].id); }}/>
        <div className="ap-list">
          {list.slice(0, 8).map(a => (
            <div key={a.id} className={'ap-opt' + (a.id === value ? ' is-active' : '')}
              onMouseDown={() => commit(a.id)}>
              <span className="ap-av" style={{background:a.color}}>{a.initials}</span>
              <span className="ap-name">{a.name}</span>
              <span className="ap-role">{a.role}</span>
            </div>
          ))}
          {value && (
            <div className="ap-clear" onMouseDown={() => commit(null)}>Unassign</div>
          )}
        </div>
      </div>
    </div>
  );
}

/* Taxonomy picker — searchable tree of node slugs */
function TaxonomyPicker({ value, commit, cancel, taxonomy }) {
  const [q, setQ] = useState('');
  const ref = React.useRef();
  useEffect(() => { if (ref.current) ref.current.focus(); }, []);
  const nodes = (window.DATA.TAXONOMIES[taxonomy] || {nodes:[]}).nodes
    .filter(n => !q || n.slug.toLowerCase().includes(q.toLowerCase()) || n.name.toLowerCase().includes(q.toLowerCase()));
  return (
    <div className="sht-cell is-editing">
      <div className="ap">
        <input ref={ref} className="ap-input" placeholder="Search nodes…" value={q}
          onChange={e => setQ(e.target.value)}
          onKeyDown={e => { if (e.key === 'Escape') cancel(); if (e.key === 'Enter' && nodes[0]) commit(nodes[0].slug); }}/>
        <div className="ap-list">
          {nodes.slice(0, 10).map(n => (
            <div key={n.slug} className={'ap-opt' + (n.slug === value ? ' is-active' : '')}
              onMouseDown={() => commit(n.slug)}>
              <span className="ap-slug rx-mono">{n.slug}</span>
            </div>
          ))}
          {value && <div className="ap-clear" onMouseDown={() => commit(null)}>Declassify</div>}
        </div>
      </div>
    </div>
  );
}

Object.assign(window, { Sheet, CellEditor, ActorPicker, TaxonomyPicker });
