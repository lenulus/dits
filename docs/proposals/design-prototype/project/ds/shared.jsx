/* Shared React helpers for RX UI kits. Exports to window. */
const { useState, useEffect, useMemo, Fragment } = React;

// ------- Icons (Lucide-ish, 1.5 stroke) -------
const I = {};
function mkIcon(name, paths) {
  I[name] = ({ size = 14, style = {}, className = '' }) => (
    <svg viewBox="0 0 24 24" width={size} height={size} className={className}
      style={{ stroke: 'currentColor', strokeWidth: 1.5, fill: 'none',
        strokeLinecap: 'round', strokeLinejoin: 'round', ...style }}
      dangerouslySetInnerHTML={{ __html: paths }} />
  );
}
mkIcon('trend',     '<path d="M3 17l6-6 4 4 8-8"/><path d="M15 7h6v6"/>');
mkIcon('clock',     '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>');
mkIcon('calendar',  '<rect x="3" y="4" width="18" height="16" rx="1"/><path d="M3 9h18M8 2v4M16 2v4"/>');
mkIcon('home',      '<path d="M3 11l9-7 9 7v10H3z"/>');
mkIcon('search',    '<circle cx="11" cy="11" r="7"/><path d="M21 21l-5-5"/>');
mkIcon('plus',      '<path d="M12 5v14M5 12h14"/>');
mkIcon('check',     '<path d="M5 13l4 4L19 7"/>');
mkIcon('x',         '<path d="M6 6l12 12M18 6L6 18"/>');
mkIcon('alert',     '<circle cx="12" cy="12" r="9"/><path d="M12 8v4M12 16h.01"/>');
mkIcon('user',      '<circle cx="12" cy="8" r="4"/><path d="M4 20c1-4 4-6 8-6s7 2 8 6"/>');
mkIcon('users',     '<circle cx="9" cy="8" r="3.5"/><path d="M2 20c.8-3 3.2-5 7-5s6.2 2 7 5"/><circle cx="17" cy="9" r="3"/><path d="M17 13c3 0 5 2 5 5"/>');
mkIcon('menu',      '<path d="M4 7h16M4 12h16M4 17h16"/>');
mkIcon('caret',     '<path d="M6 9l6 6 6-6"/>');
mkIcon('arrow',     '<path d="M5 12h14M13 6l6 6-6 6"/>');
mkIcon('book',      '<path d="M4 5a2 2 0 012-2h13v16H6a2 2 0 00-2 2V5z"/><path d="M6 17h13"/>');
mkIcon('panel',     '<rect x="3" y="4" width="18" height="16" rx="1"/><path d="M3 9h18"/>');
mkIcon('edit',      '<path d="M4 20h4L20 8l-4-4L4 16v4z"/>');
mkIcon('gauge',     '<path d="M4 16a8 8 0 1116 0"/><path d="M12 16l4-5"/><circle cx="12" cy="16" r="1.2"/>');
mkIcon('file',      '<path d="M5 3h9l5 5v13H5z"/><path d="M14 3v5h5"/>');
mkIcon('tag',       '<path d="M3 12l9-9 8 1 1 8-9 9z"/><circle cx="14" cy="8" r="1.5"/>');
mkIcon('flag',      '<path d="M5 21V4h12l-2 4 2 4H5"/>');
mkIcon('map',       '<path d="M3 6l6-2 6 2 6-2v14l-6 2-6-2-6 2z"/><path d="M9 4v16M15 6v16"/>');
mkIcon('anchor',    '<circle cx="12" cy="5" r="2"/><path d="M12 7v14M5 14c0 4 3 7 7 7s7-3 7-7M3 14h4M17 14h4"/>');
mkIcon('compass',   '<circle cx="12" cy="12" r="9"/><path d="M15 9l-2 6-4 2 2-6z"/>');
mkIcon('scale',     '<path d="M12 3v18M6 7h12M4 13l2-6 2 6M16 13l2-6 2 6"/><path d="M3 19h18"/>');
mkIcon('lock',      '<rect x="5" y="11" width="14" height="10" rx="1"/><path d="M8 11V8a4 4 0 018 0v3"/>');
mkIcon('eye',       '<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>');
mkIcon('dot3',      '<circle cx="5" cy="12" r="1.2"/><circle cx="12" cy="12" r="1.2"/><circle cx="19" cy="12" r="1.2"/>');

// ------- TopBar -------
function TopBar({ workspace, workspaceTag, user = 'ELJ', userColor }) {
  return (
    <div className="rx-topbar">
      <div className="rx-topbar__brand">
        <div className="dots">
          <span className="dot" style={{ background: 'var(--ryg-green)' }}/>
          <span className="dot" style={{ background: 'var(--ryg-yellow)' }}/>
          <span className="dot" style={{ background: 'var(--ryg-red)' }}/>
        </div>
        DITS
      </div>
      <div className="rx-topbar__workspace">
        <span className="wtag">{workspaceTag}</span>
        <span>{workspace}</span>
        <I.caret size={11}/>
      </div>
      <div className="rx-topbar__search">
        <I.search size={13}/>
        <span>Search milestones, goals, RFCs, log entries</span>
        <kbd>⌘K</kbd>
      </div>
      <div className="rx-topbar__actions">
        <span className="rx-topbar__sync"><span className="rx-dot rx-dot--g" style={{margin:0}}/>synced · 14s</span>
        <I.alert size={15}/>
        <div className="avatar" style={userColor ? {background: userColor} : undefined}>{user}</div>
      </div>
    </div>
  );
}

// ------- Sidebar -------
function Sidebar({ sections }) {
  return (
    <div className="rx-sidebar">
      {sections.map((s, i) => (
        <div className="rx-sidebar__section" key={i}>
          {s.label && <div className="rx-sidebar__label">{s.label}</div>}
          {s.items.map((it, j) => {
            const Icon = I[it.icon] || I.panel;
            return (
              <div key={j} className={'rx-nav-item' + (it.active ? ' is-active' : '')}
                onClick={() => it.onClick && it.onClick()}>
                <Icon size={14}/>
                <span>{it.label}</span>
                {it.count != null && <span className="count">{it.count}</span>}
              </div>
            );
          })}
        </div>
      ))}
    </div>
  );
}

// ------- Primitives -------
function Eyebrow({ children }) { return <div className="rx-eyebrow">{children}</div>; }
function Panel({ title, meta, flush, children, right }) {
  return (
    <div className="rx-panel">
      {(title || meta || right) && (
        <div className="rx-panel__head">
          {title && <div className="rx-panel__title">{title}</div>}
          {meta && <div className="rx-panel__meta">{meta}</div>}
          {right && <div style={{marginLeft:'auto'}}>{right}</div>}
        </div>
      )}
      <div className={'rx-panel__body' + (flush ? ' rx-panel__body--flush' : '')}>{children}</div>
    </div>
  );
}
function Chip({ kind = 'neutral', children }) {
  return <span className={'rx-chip rx-chip--' + kind}>{children}</span>;
}
function RYG({ color, label }) {
  const text = label || { g: 'Green', y: 'Yellow', r: 'Red', stale: 'Stale' }[color];
  const kind = { g: 'green', y: 'yellow', r: 'red', stale: 'stale' }[color];
  return <Chip kind={kind}><span className="rx-dot" style={{margin:0}} /* visual dot via chip rule */ />{text}</Chip>;
}
function Dot({ color = 'g' }) { return <span className={'rx-dot rx-dot--' + color}/>; }
function Btn({ kind = 'secondary', size, icon, children, onClick }) {
  const Icon = icon && I[icon];
  return <button className={'rx-btn rx-btn--' + kind + (size === 'sm' ? ' rx-btn--sm' : '')} onClick={onClick}>
    {Icon && <Icon size={12}/>}{children}</button>;
}

// ------- KPI panel -------
function KPI({ label, value, unit, delta, deltaKind = 'up', spark, color = 'var(--ink-0)' }) {
  return (
    <div className="rx-kpi">
      <div className="rx-kpi__lab">{label}</div>
      <div className="rx-kpi__val">{value}{unit && <span className="rx-kpi__unit">{unit}</span>}</div>
      {delta && <div className={'rx-kpi__delta' + (deltaKind === 'down' ? ' rx-kpi__delta--down' : deltaKind === 'warn' ? ' rx-kpi__delta--warn' : '')}>{delta}</div>}
      {spark && (
        <svg className="rx-kpi__spark" viewBox="0 0 120 22" preserveAspectRatio="none">
          <polyline fill="none" stroke={color} strokeWidth="1.5" points={spark}/>
        </svg>
      )}
    </div>
  );
}

// ------- Log entry -------
function LogEntry({ num, kind, by, time, children }) {
  const labels = { decision: 'Decision', risk: 'Risk', stress_test: 'Stress test', observation: 'Observation', integrity_call: 'Integrity' };
  return (
    <div className="rx-log-entry">
      <div className="rx-log-entry__num">§{num}</div>
      <div className={'rx-log-entry__kind rx-log-entry__kind--' + kind}>{labels[kind]}</div>
      <div>
        <div className="rx-log-entry__body">{children}</div>
        <div className="rx-log-entry__by">{by} · {time}</div>
      </div>
    </div>
  );
}

// ------- Owner pill (avatar + name) -------
function Owner({ initials, name, color }) {
  return (
    <span className="rx-owner">
      <span className="av" style={{background: color || 'var(--ink-2)'}}>{initials}</span>
      <span>{name}</span>
    </span>
  );
}

// ------- Inline-editable cell -------
// Click to edit. Enter/Esc commits/cancels. Tab moves forward.
function InlineCell({ value, onChange, kind = 'text', options, placeholder, readonly, render, align }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value);
  const inputRef = React.useRef();
  useEffect(() => { setDraft(value); }, [value]);
  useEffect(() => {
    if (editing && inputRef.current) {
      inputRef.current.focus();
      if (inputRef.current.select) inputRef.current.select();
    }
  }, [editing]);
  function commit() { if (onChange) onChange(draft); setEditing(false); }
  function cancel() { setDraft(value); setEditing(false); }
  function onKey(e) {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); commit(); }
    else if (e.key === 'Escape') { cancel(); }
  }

  if (readonly) {
    return <div className="rx-cell rx-cell--readonly">{render ? render(value) : value}</div>;
  }
  const cellCls = 'rx-cell' + (kind === 'mono' ? ' rx-cell--mono' : '') + (align === 'right' ? ' rx-cell--num' : '') + (editing ? ' is-editing' : '');

  if (!editing) {
    return (
      <div className={cellCls} onClick={() => setEditing(true)}>
        {value
          ? (render ? render(value) : value)
          : <span className="placeholder">{placeholder || '—'}</span>}
      </div>
    );
  }
  if (kind === 'select') {
    return (
      <div className={cellCls}>
        <select ref={inputRef} value={draft || ''} onChange={e => setDraft(e.target.value)}
          onBlur={commit} onKeyDown={onKey}>
          {!draft && <option value=""></option>}
          {options.map(o => <option key={o.value ?? o} value={o.value ?? o}>{o.label ?? o}</option>)}
        </select>
      </div>
    );
  }
  return (
    <div className={cellCls}>
      <input ref={inputRef} value={draft || ''} onChange={e => setDraft(e.target.value)}
        onBlur={commit} onKeyDown={onKey} placeholder={placeholder}/>
    </div>
  );
}

// ------- Slide-over (replaces modal for record detail) -------
function SlideOver({ open, onClose, id, title, tabs, activeTab, onTab, children, footer }) {
  if (!open) return null;
  return (
    <div className="rx-slideover">
      <div className="rx-slideover__head">
        <div>
          {id && <div className="id">{id}</div>}
          <div className="title">{title}</div>
        </div>
        <button className="rx-slideover__close" onClick={onClose} aria-label="Close">
          <I.x size={14}/>
        </button>
      </div>
      {tabs && (
        <div className="rx-slideover__tabs">
          {tabs.map(t => (
            <div key={t} className={'rx-slideover__tab' + (t === activeTab ? ' is-active' : '')}
              onClick={() => onTab && onTab(t)}>{t}</div>
          ))}
        </div>
      )}
      <div className="rx-slideover__body">{children}</div>
      {footer && <div style={{borderTop:'1px solid var(--hairline)', padding:'12px 20px', background:'var(--surface)'}}>{footer}</div>}
    </div>
  );
}

// ------- Pivot / segmented control -------
function Pivot({ options, value, onChange }) {
  return (
    <div className="rx-pivot">
      {options.map(o => (
        <button key={o.value ?? o} className={(value === (o.value ?? o)) ? 'is-active' : ''}
          onClick={() => onChange(o.value ?? o)}>{o.label ?? o}</button>
      ))}
    </div>
  );
}

// ------- Filter chip -------
function FilterChip({ label, value, onRemove }) {
  return (
    <span className="rx-filterchip">
      <span className="lab">{label}</span>
      <span>{value}</span>
      {onRemove && <span className="x" onClick={onRemove}><I.x size={10}/></span>}
    </span>
  );
}

// ------- Bulk action bar (above grid when rows selected) -------
function BulkBar({ count, onClear, children }) {
  if (!count) return null;
  return (
    <div className="rx-bulkbar">
      <span className="count">{count}</span>
      <span>selected</span>
      <div className="actions">{children}<button className="rx-btn rx-btn--ghost rx-btn--sm" onClick={onClear}>Clear</button></div>
    </div>
  );
}

Object.assign(window, { I, TopBar, Sidebar, Eyebrow, Panel, Chip, RYG, Dot, Btn, KPI, LogEntry, InlineCell, Owner, SlideOver, Pivot, FilterChip, BulkBar });
