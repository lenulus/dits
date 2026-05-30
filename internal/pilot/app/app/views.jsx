/* Views — one renderer per "sheet" in the app.
   Each view is a function that takes ({state, actions}) and returns a React node.
   The shell wires them up. Renderers / formatters are at the top.
*/

const D = window.DATA;
const { useState, useEffect, useMemo, Fragment } = React;

/* ---------------- Formatters & inline renderers ---------------- */
function actorOf(id) { return id ? D.ACTORS[id] : null; }

function renderActor(actorId, opts = {}) {
  const a = actorOf(actorId);
  if (!a) return <span className="placeholder">unassigned</span>;
  return (
    <span className="dx-owner">
      <span className={'dx-av' + (a.agent ? ' dx-av--agent' : '')} style={{background: a.agent ? undefined : a.color}}>{a.initials}</span>
      {!opts.compact && <span className="dx-owner__name">{a.name}</span>}
    </span>
  );
}

const STATUS_LABEL = {
  draft: 'Draft', leadership_review: 'Leadership review',
  approved: 'Approved', backlog: 'Backlog', resourced: 'Resourced', rejected: 'Rejected',
  ack_filed: 'ACK filed', ack_committed: 'ACK committed',
  in_flight: 'In flight', shipped: 'Shipped', aborted: 'Aborted',
  open: 'Open', resolved: 'Resolved', escalated: 'Escalated',
  pending: 'Pending', completed: 'Completed', overdue: 'Overdue',
};

const STATUS_KIND = {
  draft: 'neutral', leadership_review: 'plum', approved: 'green', backlog: 'neutral', resourced: 'blue', rejected: 'r',
  ack_filed: 'blue', ack_committed: 'g',
  in_flight: 'g', shipped: 'neutral', aborted: 'r',
  open: 'y', resolved: 'g', escalated: 'r',
  pending: 'blue', completed: 'g', overdue: 'r',
};

function renderStatus(v) {
  if (!v) return <span className="placeholder">—</span>;
  const kind = STATUS_KIND[v] || 'neutral';
  return <span className={'dx-pill dx-pill--' + kind}>{STATUS_LABEL[v] || v}</span>;
}

/* ACK split: Specifier ACK + Builder ACK, each pending|accepted|rejected.
   Rollup is computed. */
const ACK_STATES = ['pending','accepted','rejected'];
const ACK_KIND = { accepted:'g', pending:'neutral', rejected:'r' };
const ACK_LABEL = { accepted:'Accepted', pending:'Pending', rejected:'Rejected' };
function renderAckCell(v, ctx) {
  const r = ctx?.row;
  const last = r && Array.isArray(r.ackHistory) && r.ackHistory.find(h => h.who === (ctx?.who || ''));
  return (
    <span className="ack-cellbox">
      <span className={'dx-pill dx-pill--' + (ACK_KIND[v] || 'neutral')}>{ACK_LABEL[v] || v || 'pending'}</span>
      {last && <span className="ack-cellbox__when">{relativeDay(last.when)}</span>}
    </span>
  );
}
function ackRollup(s, b) {
  if (s === 'rejected' || b === 'rejected') return 'rejected';
  if (s === 'accepted' && b === 'accepted') return 'aligned';
  if ((s||'pending') === 'pending' && (b||'pending') === 'pending') return 'both_pending';
  if ((s||'pending') === 'pending') return 'specifier_pending';
  if ((b||'pending') === 'pending') return 'builder_pending';
  return 'both_pending';
}
const ROLLUP_LABEL = {
  aligned:'Aligned', builder_pending:'Builder pending', specifier_pending:'Specifier pending',
  both_pending:'Both pending', rejected:'Rejected',
};
const ROLLUP_KIND = { aligned:'g', builder_pending:'y', specifier_pending:'y', both_pending:'neutral', rejected:'r' };
function renderRollup(_v, ctx) {
  const r = ctx?.row;
  if (!r) return null;
  const k = ackRollup(r.specifierAck, r.builderAck);
  return <span className={'dx-pill dx-pill--' + ROLLUP_KIND[k]}>{ROLLUP_LABEL[k]}</span>;
}

function relativeDay(when) {
  if (!when) return '';
  const d = new Date(when + 'T00:00:00');
  const days = Math.floor((Date.now() - d.getTime()) / 86400000);
  if (days <= 0) return 'today';
  if (days === 1) return '1d ago';
  if (days < 14) return days + 'd ago';
  if (days < 60) return Math.floor(days/7) + 'w ago';
  return Math.floor(days/30) + 'mo ago';
}

/* ACK editor — three-button popover for fast accept/pending/reject */
function AckPicker({ value, commit, cancel }) {
  React.useEffect(() => {
    function onKey(e) {
      if (e.key === 'a' || e.key === 'A') { e.preventDefault(); commit('accepted'); }
      else if (e.key === 'p' || e.key === 'P') { e.preventDefault(); commit('pending'); }
      else if (e.key === 'r' || e.key === 'R') { e.preventDefault(); commit('rejected'); }
      else if (e.key === 'Escape') { cancel(); }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);
  return (
    <div className="sht-cell is-editing">
      <div className="ackpop">
        {ACK_STATES.map(s => (
          <button key={s} className={'ackpop__btn ackpop__btn--' + ACK_KIND[s] + (s === value ? ' is-active' : '')}
            onMouseDown={() => commit(s)}>
            <span className="ackpop__lab">{ACK_LABEL[s]}</span>
            <span className="ackpop__kbd">{s[0].toUpperCase()}</span>
          </button>
        ))}
      </div>
    </div>
  );
}

function renderRyg(v) {
  if (!v) return <span className="placeholder">—</span>;
  const lbl = { g: 'Green', y: 'Yellow', r: 'Red' }[v] || v;
  return <span className={'dx-pill dx-pill--' + v}>{lbl}</span>;
}

function renderFresh(_v, ctx) {
  const r = ctx?.row;
  if (!r) return null;
  const kind = { fresh: 'g', aging: 'y', stale: 'r' }[r.fresh] || 'neutral';
  return <span className={'dx-pill dx-pill--' + kind}>{r.age}</span>;
}

function renderDepClosed(v) {
  if (!Array.isArray(v)) return <span className="placeholder">—</span>;
  const [d, t] = v;
  const pct = t === 0 ? 0 : d/t;
  const kind = pct >= 0.8 ? 'g' : pct >= 0.5 ? 'y' : pct === 0 ? 'neutral' : 'r';
  return <span className={'dx-pill dx-pill--' + kind}>{d}/{t}</span>;
}

function renderProductPath(v) {
  if (!v) return <span className="placeholder">unclassified</span>;
  const parts = String(v).split('/');
  const last = parts[parts.length - 1];
  return (
    <span style={{display:'inline-flex', flexDirection:'column', minWidth:0}}>
      <span style={{fontSize:12, color:'var(--ink-1)', fontWeight:500}}>{last}</span>
      <span style={{fontFamily:'var(--font-mono)', fontSize:9.5, color:'var(--ink-4)', letterSpacing:'0.02em', textOverflow:'ellipsis', overflow:'hidden', whiteSpace:'nowrap'}}>{v}</span>
    </span>
  );
}

function renderOrgPath(v) {
  if (!v) return <span className="placeholder">—</span>;
  const parts = String(v).split('/');
  return <span style={{fontFamily:'var(--font-mono)', fontSize:11, color:'var(--ink-2)'}}>{parts[parts.length-1]}</span>;
}

function renderDiagnostics(_v, ctx) {
  const r = ctx?.row;
  if (!r || !r.diagnostics || r.diagnostics.length === 0) {
    return <span style={{fontFamily:'var(--font-mono)', color:'var(--ink-5)', fontSize:11}}>—</span>;
  }
  const violation = r.diagnostics.some(d => /reports-to|collapse|violation/i.test(d));
  return (
    <span className={'dx-diag-count' + (violation ? ' dx-diag-count--violation' : '')} title={r.diagnostics.join(' · ')}>
      {r.diagnostics.length}
    </span>
  );
}

function renderTitle(v, ctx) {
  return (
    <span style={{display:'inline-flex', alignItems:'center', gap:8, minWidth:0, fontWeight:500, color:'var(--ink-0)', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap'}}>
      {v}
    </span>
  );
}

function renderKind(v) {
  const kindLabel = { milestone:'MILE', rfc:'RFC', decision_block:'DB', outcome_assessment:'OA', goal:'GOAL' }[v] || v.toUpperCase();
  return <span className="dx-pill dx-pill--neutral" style={{fontFamily:'var(--font-mono)', fontSize:9.5, letterSpacing:'0.06em'}}>{kindLabel}</span>;
}

function renderSpark(_v, ctx) {
  const r = ctx?.row;
  if (!r?.indicators) return null;
  // Simple per-row tiny sparkline
  return (
    <svg className="spark" viewBox="0 0 40 14" preserveAspectRatio="none">
      <polyline stroke="var(--accent)" points="0,11 6,10 12,9 18,7 24,8 30,5 36,4 40,3"/>
    </svg>
  );
}

function renderCustomerVisible(v) {
  return v
    ? <span className="dx-pill dx-pill--blue">public</span>
    : <span className="dx-pill dx-pill--neutral">internal</span>;
}

const TARGETS = ['2026 Q2','2026 Q3','2026 Q4','2027 Q1'];
const M_STATUSES = ['draft','ack_filed','ack_committed','in_flight','shipped','aborted'];
const R_STATUSES = ['draft','leadership_review','approved','backlog','resourced','rejected'];

/* --------- Tree sort: parent first, then children in order. _depth set on each row. --------- */
function treeSorted(items, parentKey = 'parent', idKey = 'id') {
  const byParent = new Map();
  items.forEach(m => {
    const p = m[parentKey] || '';
    if (!byParent.has(p)) byParent.set(p, []);
    byParent.get(p).push(m);
  });
  const out = [];
  function walk(parent, depth) {
    (byParent.get(parent) || []).forEach(m => {
      const kids = byParent.get(m[idKey]) || [];
      out.push({ ...m, _depth: depth, _hasChildren: kids.length > 0 });
      walk(m[idKey], depth + 1);
    });
  }
  walk('', 0);
  // Catch orphans whose parent isn't visible
  items.forEach(m => {
    if (!out.find(o => o[idKey] === m[idKey])) {
      out.push({ ...m, _depth: 0, _hasChildren: !!byParent.get(m[idKey])?.length });
    }
  });
  return out;
}

/* --------- Title renderer with tree indent + caret --------- */
function renderTitleTree(v, ctx) {
  const r = ctx?.row || {};
  const depth = r._depth || 0;
  return (
    <span style={{display:'inline-flex', alignItems:'center', gap:6, minWidth:0, fontWeight:500, color:'var(--ink-0)', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap'}}>
      {depth > 0 && (
        <span style={{display:'inline-flex', alignItems:'center', color:'var(--ink-5)', fontFamily:'var(--font-mono)', fontSize:11, marginLeft: (depth - 1) * 16}}>
          └─
        </span>
      )}
      {r._hasChildren && (
        <span style={{color:'var(--accent)', fontSize:11, fontFamily:'var(--font-mono)'}}>▾</span>
      )}
      <span style={{overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap'}}>{v}</span>
    </span>
  );
}

/* --------- Stages mini-timeline --------- */
function renderStages(_v, ctx) {
  const r = ctx?.row;
  if (!r || !Array.isArray(r.stages) || r.stages.length === 0) {
    return <span className="placeholder">no stages</span>;
  }
  return (
    <span className="stages">
      {r.stages.map((s, i) => (
        <span key={s.key || i} className={'stage stage--' + (s.state || 'open')}>
          <span className="stage__dot"/>
          <span className="stage__lab">{s.label}</span>
          <span className="stage__when">{formatTarget(s.date, s.precision)}</span>
        </span>
      ))}
    </span>
  );
}

/* --------- Target with precision --------- */
function formatTarget(value, precision) {
  if (!value) return '—';
  if (precision === 'Q' || /\d{4} Q\d/.test(value)) return value;
  if (precision === 'M') {
    // value is yyyy-mm or "Sept 2026"
    if (/^\d{4}-\d{2}/.test(value)) {
      const [y,m] = value.split('-');
      const names = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sept','Oct','Nov','Dec'];
      return `${names[+m - 1]} ${y}`;
    }
    return value;
  }
  if (precision === 'D') {
    if (/^\d{4}-\d{2}-\d{2}/.test(value)) {
      const [y,m,d] = value.split('-');
      const names = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sept','Oct','Nov','Dec'];
      return `${names[+m - 1]} ${+d}, ${y}`;
    }
    return value;
  }
  return value;
}
function renderTarget(v, ctx) {
  const r = ctx?.row;
  const p = r?.targetPrecision || 'Q';
  return (
    <span style={{display:'inline-flex', alignItems:'center', gap:6, fontFamily:'var(--font-mono)', fontSize:11.5, color:'var(--ink-1)'}}>
      <span>{formatTarget(r?.targetISO || v, p)}</span>
      <span style={{fontFamily:'var(--font-mono)', fontSize:9, padding:'1px 4px', background:'var(--paper-1)', color:'var(--ink-4)', borderRadius:3, letterSpacing:'0.05em'}}>{p}</span>
    </span>
  );
}

/* TargetEditor popover — precision toggle + appropriate input. */
function TargetEditor({ value, row, commit, cancel }) {
  const [prec, setPrec] = useState(row?.targetPrecision || 'Q');
  const [val, setVal]   = useState(row?.targetISO || value || '');
  const ref = React.useRef();
  useEffect(() => { if (ref.current) ref.current.focus(); }, [prec]);
  function done(nextVal, nextPrec) {
    const finalPrec = nextPrec || prec;
    const finalVal = nextVal !== undefined ? nextVal : val;
    commit({ targetPrecision: finalPrec, targetISO: finalVal, target: formatTarget(finalVal, finalPrec) });
  }
  return (
    <div className="sht-cell is-editing">
      <div className="ap" style={{minWidth:280, padding:8}}>
        <div className="targed-prec">
          {['Q','M','D'].map(p => (
            <button key={p}
              className={'targed-prec__btn' + (p === prec ? ' is-active' : '')}
              onMouseDown={(e) => { e.preventDefault(); setPrec(p); }}>
              {p === 'Q' ? 'Quarter' : p === 'M' ? 'Month' : 'Exact date'}
            </button>
          ))}
        </div>
        {prec === 'Q' && (
          <select ref={ref} className="ap-input" value={val.startsWith('20') && /Q\d/.test(val) ? val : '2026 Q3'}
            onChange={e => setVal(e.target.value)}
            onBlur={() => done()}
            onKeyDown={e => { if (e.key === 'Enter') done(); if (e.key === 'Escape') cancel(); }}>
            {TARGETS.map(t => <option key={t} value={t}>{t}</option>)}
          </select>
        )}
        {prec === 'M' && (
          <input ref={ref} type="month" className="ap-input"
            defaultValue={val && /^\d{4}-\d{2}/.test(val) ? val.slice(0,7) : '2026-09'}
            onChange={e => setVal(e.target.value + '-01')}
            onBlur={() => done()}
            onKeyDown={e => { if (e.key === 'Enter') done(); if (e.key === 'Escape') cancel(); }}/>
        )}
        {prec === 'D' && (
          <input ref={ref} type="date" className="ap-input"
            defaultValue={val && /^\d{4}-\d{2}-\d{2}/.test(val) ? val.slice(0,10) : '2026-09-30'}
            onChange={e => setVal(e.target.value)}
            onBlur={() => done()}
            onKeyDown={e => { if (e.key === 'Enter') done(); if (e.key === 'Escape') cancel(); }}/>
        )}
        <div style={{marginTop:6, fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-4)'}}>
          Click outside to commit · Esc cancels
        </div>
      </div>
    </div>
  );
}

/* Status / Risks / Next cell renderers */
function renderStatusCell(_v, ctx) {
  const r = ctx?.row;
  if (!r || !r.statusNarrative) {
    return <span className="placeholder">no status yet</span>;
  }
  return (
    <span style={{display:'inline-flex', flexDirection:'column', minWidth:0, lineHeight:1.2}}>
      <span style={{fontSize:12, color:'var(--ink-1)', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap'}}>{r.statusNarrative}</span>
      <span style={{fontFamily:'var(--font-mono)', fontSize:9.5, color:'var(--ink-4)'}}>{r.statusUpdatedBy ? D.ACTORS[r.statusUpdatedBy]?.initials || r.statusUpdatedBy : '—'} · {r.statusUpdatedAt ? relativeDay(r.statusUpdatedAt) : 'never'}</span>
    </span>
  );
}
const SEV_KIND = { high:'r', medium:'y', low:'neutral' };
function renderRisksCell(_v, ctx) {
  const r = ctx?.row;
  const xs = (r?.risks || []);
  if (xs.length === 0) return <span className="placeholder">—</span>;
  const high = xs.filter(x => x.severity === 'high').length;
  return (
    <span style={{display:'inline-flex', alignItems:'center', gap:6, minWidth:0}}>
      <span className={'dx-diag-count' + (high ? ' dx-diag-count--violation' : '')}>{xs.length}</span>
      <span style={{fontFamily:'var(--font-serif)', fontStyle:'italic', fontSize:12, color:'var(--ink-2)', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap'}}>{xs[0].body}</span>
    </span>
  );
}
function renderNextCell(_v, ctx) {
  const r = ctx?.row;
  const xs = (r?.nextSteps || []);
  if (xs.length === 0) return <span className="placeholder">—</span>;
  return (
    <span style={{display:'inline-flex', alignItems:'center', gap:6, minWidth:0}}>
      <span className="dx-diag-count" style={{background:'var(--accent)'}}>{xs.length}</span>
      <span style={{fontFamily:'var(--font-serif)', fontStyle:'italic', fontSize:12, color:'var(--ink-2)', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap'}}>{xs[0].body}</span>
    </span>
  );
}

/* Evaluate one filter chip against a milestone */
function evalFilter(m, f) {
  const { field, value } = f;
  if (field === 'productTop') {
    if (value === '_none') return !m.productNode;
    return m.productNode && m.productNode.startsWith(value);
  }
  if (field === 'orgNode') return m.orgNode === value || (m.orgNode || '').startsWith(value + '/');
  if (field === 'specifier' || field === 'builder' || field === 'pilot' || field === 'status' || field === 'ryg' || field === 'target') {
    return m[field] === value;
  }
  if (field === 'alignment') return ackRollup(m.specifierAck, m.builderAck) === value;
  if (field === 'customerVisible') return !!m.customerVisible === !!value;
  if (field === '_hasDiagnostics') {
    if (value === 'any') return (m.diagnostics || []).length > 0;
    if (value === 'violation') return (m.diagnostics || []).some(d => /reports-to|collapse/i.test(d));
  }
  if (field === '_hasRisksHigh') return (m.risks || []).some(r => r.severity === 'high');
  if (field === '_isStale') {
    if (!m.statusUpdatedAt) return true;
    const days = Math.floor((Date.now() - new Date(m.statusUpdatedAt).getTime()) / 86400000);
    return days > 14;
  }
  if (field === '_fromRfc') {
    if (value === '_any')  return !!m.fromRfc;
    if (value === '_none') return !m.fromRfc;
    return m.fromRfc === value;
  }
  return true;
}

/* ===================== Portfolio (the milestone grid) ===================== */
const PORTFOLIO_GROUPS = [
  { key:'identity', label:'Identity', alwaysOpen: true },
  { key:'classification', label:'Classification',
    summary: (row) => row.productNode
      ? <span style={{fontFamily:'var(--font-mono)', fontSize:10.5, color:'var(--ink-2)'}}>{row.productNode.split('/').slice(-1)[0]}</span>
      : <span className="placeholder">unclassified</span> },
  { key:'health',   label:'Health',
    summary: (row) => (
      <span style={{display:'inline-flex', alignItems:'center', gap:6}}>
        {renderRyg(row.ryg)}
        <span style={{fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-3)'}}>{STATUS_LABEL[row.status] || row.status}</span>
      </span>
    )},
  { key:'acks',     label:'ACKs',
    summary: (row) => renderRollup(null, {row}) },
  { key:'roles',    label:'Roles',
    summary: (row) => (
      <span className="role-stack">
        {['specifier','builder','pilot'].map(r => {
          const a = actorOf(row[r]);
          if (!a) return <span key={r} className="role-stack__slot role-stack__slot--empty" title={r + ': unassigned'}>?</span>;
          return <span key={r} className="role-stack__slot" title={r + ': ' + a.name} style={{background: a.color}}>{a.initials}</span>;
        })}
      </span>
    )},
  { key:'delivery', label:'Delivery',
    summary: (row) => renderTarget(null, {row}) },
  { key:'narrative',label:'Updates',
    summary: (row) => {
      const r = (row.risks || []).length;
      const n = (row.nextSteps || []).length;
      const hasStatus = !!row.statusNarrative;
      return (
        <span style={{display:'inline-flex', alignItems:'center', gap:8, fontFamily:'var(--font-mono)', fontSize:10.5, color:'var(--ink-3)'}}>
          <span title="status update" style={{color: hasStatus ? 'var(--accent)' : 'var(--ink-5)'}}>● status</span>
          <span title="risks" style={{color: r > 0 ? 'var(--ryg-yellow-ink)' : 'var(--ink-5)'}}>{r} risk{r === 1 ? '' : 's'}</span>
          <span title="next steps" style={{color: n > 0 ? 'var(--ink-1)' : 'var(--ink-5)'}}>{n} next</span>
        </span>
      );
    }},
  { key:'signals',  label:'Signals',
    summary: (row) => (
      <span style={{display:'inline-flex', alignItems:'center', gap:6}}>
        {renderDepClosed(row.depClosed)}
        {renderDiagnostics(null, {row})}
      </span>
    )},
];

/* JTBD presets: which groups are collapsed for each task. Identity is alwaysOpen. */
const PORTFOLIO_PRESETS = [
  { value:'overview',  label:'Overview',
    collapsed:['health','classification','acks','roles','delivery','signals'] },
  { value:'ownership', label:'Ownership',
    collapsed:['health','classification','acks','delivery','narrative','signals'] },
  { value:'status',    label:'Status & risk',
    collapsed:['health','classification','acks','roles','delivery','signals'] },
  { value:'everything',label:'Everything',
    collapsed:[] },
];
function presetFor(collapsedGroups) {
  const arr = [...(collapsedGroups || [])].sort().join(',');
  return PORTFOLIO_PRESETS.find(p => [...p.collapsed].sort().join(',') === arr)?.value || 'custom';
}

function PortfolioView({ state, actions }) {
  const { pivot, groupBy, filters, selectedIds, activeId, milestones, collapsedGroups } = state;

  // Filter: primary pivot + chip filters AND-together
  const filtered = useMemo(() => milestones.filter(m => {
    // primary pivot
    if (pivot === 'mine' && m.specifier !== 'ejackson' && m.builder !== 'ejackson' && m.pilot !== 'ejackson') return false;
    if (pivot === 'in_flight' && m.status !== 'in_flight') return false;
    if (pivot === 'drifting' && ackRollup(m.specifierAck, m.builderAck) === 'aligned') return false;
    if (pivot === 'rfcs' && m.kind !== 'rfc') return false;
    // chip filters
    for (const f of (filters || [])) {
      if (!evalFilter(m, f)) return false;
    }
    return true;
  }), [milestones, pivot, filters]);

  // Tree-sort + derive _productTop for group-by
  const sortedRows = useMemo(() => {
    const sorted = treeSorted(filtered);
    return sorted.map(r => ({
      ...r,
      _productTop: r.productNode ? (r.productNode.split('/')[0] || 'unclassified') : 'unclassified',
    }));
  }, [filtered]);

  return (
    <Sheet
      columnGroups={PORTFOLIO_GROUPS}

      collapsedGroups={collapsedGroups}

      onToggleGroup={(key) => actions.toggleGroup(key)}

      columns={[
        { key:'id',       label:'ID',       width:96, group:'identity',  frozen:true, readonly:true,
          render: v => <span className="dx-id">{v}</span> },
        { key:'title',    label:'Title',    width:260, group:'identity', frozen:true,
          render: renderTitleTree },
        { key:'status',   label:'Status',   width:130, group:'health', kind:'select',
          options: M_STATUSES.map(s => ({ value:s, label: STATUS_LABEL[s] })),
          render: renderStatus, sortable:true },
        { key:'ryg',      label:'RYG',      width:82, group:'health', kind:'select',
          options: [{value:'g',label:'Green'},{value:'y',label:'Yellow'},{value:'r',label:'Red'}],
          render: renderRyg },
        { key:'specifierAck', label:'S-ACK', width:104, group:'acks',
          edit: ({value, commit, cancel}) => <AckPicker value={value} commit={commit} cancel={cancel}/>,
          render: (v, ctx) => renderAckCell(v, { ...ctx, who:'specifier' }) },
        { key:'builderAck',   label:'B-ACK', width:104, group:'acks',
          edit: ({value, commit, cancel}) => <AckPicker value={value} commit={commit} cancel={cancel}/>,
          render: (v, ctx) => renderAckCell(v, { ...ctx, who:'builder' }) },
        { key:'specifier',label:'Specifier',width:148, group:'roles',
          edit: ({value, commit, cancel}) => <ActorPicker value={value} commit={commit} cancel={cancel}/>,
          render: (v) => renderActor(v) },
        { key:'builder',  label:'Builder',  width:148, group:'roles',
          edit: ({value, commit, cancel}) => <ActorPicker value={value} commit={commit} cancel={cancel} agentOk/>,
          render: (v) => renderActor(v) },
        { key:'pilot',    label:'Pilot',    width:148, group:'roles',
          edit: ({value, commit, cancel}) => <ActorPicker value={value} commit={commit} cancel={cancel}/>,
          render: (v) => renderActor(v) },
        { key:'target',   label:'Target',   width:140, group:'delivery',
          edit: ({value, row, commit, cancel}) => <TargetEditor value={value} row={row}
            commit={(patch) => commit(patch.target)} cancel={cancel}/>,
          render: renderTarget, sortable:true },
        { key:'stages',   label:'Stages',   width:280, group:'delivery', readonly:true, sortable:false,
          render: renderStages },
        { key:'statusNarrative', label:'Status →', width:320, group:'narrative',
          render: renderStatusCell,
          edit: ({value, commit, cancel}) => (
            <CellEditor kind={undefined} value={value || ''}
              onCommit={commit} onCancel={cancel}/>
          ) },
        { key:'risks',    label:'Risks',    width:240, group:'narrative', readonly:true, sortable:false,
          render: renderRisksCell },
        { key:'nextSteps',label:'Next',     width:240, group:'narrative', readonly:true, sortable:false,
          render: renderNextCell },
        { key:'orgNode',  label:'Org', width:170, group:'classification',
          edit: ({value, commit, cancel}) => <TaxonomyPicker value={value} commit={commit} cancel={cancel} taxonomy="org"/>,
          render: renderOrgPath },
        { key:'productNode', label:'Product', width:200, group:'classification',
          edit: ({value, commit, cancel}) => <TaxonomyPicker value={value} commit={commit} cancel={cancel} taxonomy="product"/>,
          render: renderProductPath },
        { key:'scopeChanges', label:'Scope Δ', width:80, group:'signals', num:true, align:'right', sortable:true,
          render: v => v === '—' || v == null ? '—' : <span style={{fontFamily:'var(--font-mono)', color: v >= 3 ? 'var(--ryg-red)' : v >= 1 ? 'var(--ryg-yellow)' : 'var(--ink-2)'}}>{v}</span> },
        { key:'depClosed', label:'Deps',    width:80, group:'signals', readonly:true,
          render: renderDepClosed },
        { key:'fresh',    label:'Freshness', width:96, group:'signals', readonly:true,
          render: renderFresh },
        { key:'diagnostics', label:'Diag', width:60, group:'signals', readonly:true,
          render: renderDiagnostics },
        { key:'customerVisible', label:'Visibility', width:96, group:'signals', kind:'select',
          options: [{value:true,label:'Public'},{value:false,label:'Internal'}],
          render: renderCustomerVisible },
      ]}
      rows={sortedRows}
      groupBy={groupBy === 'none' ? null : groupBy}
      selectedIds={selectedIds}
      setSelectedIds={actions.setSelectedIds}
      activeId={activeId}
      navColumn={1}
      onSelectRow={actions.openRecord}
      onUpdate={(id, patch) => actions.updateMilestone(id, patch)}
      onAddRow={() => actions.newMilestone()}
      addLabel="New milestone…"
    />
  );
}

/* ===================== RFCs ===================== */
function RfcsView({ state, actions }) {
  return (
    <Sheet
      columns={[
        { key:'id', label:'ID', width:96, frozen:true, readonly:true,
          render: v => <span className="dx-id">{v}</span> },
        { key:'title', label:'Title', width:280, frozen:true,
          render: renderTitle },
        { key:'status', label:'Status', width:160, kind:'select',
          options: R_STATUSES.map(s => ({value:s,label:STATUS_LABEL[s]})),
          render: renderStatus },
        { key:'specifier', label:'Specifier', width:160,
          edit: ({value, commit, cancel}) => <ActorPicker value={value} commit={commit} cancel={cancel}/>,
          render: v => renderActor(v) },
        { key:'target', label:'Target', width:110, kind:'select',
          options: [{value:'—',label:'—'}, ...TARGETS.map(t=>({value:t,label:t}))] },
        { key:'summary', label:'Summary', width:520,
          render: v => <span style={{color:'var(--ink-2)', fontFamily:'var(--font-serif)', fontStyle:'italic'}}>{v}</span> },
        { key:'age', label:'Age', width:80, readonly:true,
          render: v => <span style={{fontFamily:'var(--font-mono)', fontSize:11, color:'var(--ink-3)'}}>{v}</span> },
        { key:'sig', label:'Sig', width:96, readonly:true,
          render: v => <span className="dx-sig">{v}…</span> },
      ]}
      rows={state.rfcs}
      selectedIds={state.selectedIds}
      setSelectedIds={actions.setSelectedIds}
      activeId={state.activeId}
      onSelectRow={actions.openRecord}
      onUpdate={(id, patch) => actions.updateRfc(id, patch)}
      onAddRow={() => actions.newRfc()}
      addLabel="New RFC…"
    />
  );
}

/* ===================== Pilot's Log (with quick-entry row) ===================== */
function PilotLogView({ state, actions }) {
  const [draftKind, setDraftKind] = useState('observation');
  const [draftBody, setDraftBody] = useState('');
  const [draftMile, setDraftMile] = useState('PROJ-198');
  const taRef = React.useRef();

  function commit() {
    if (!draftBody.trim()) return;
    actions.addLogEntry({ kind: draftKind, body: draftBody.trim(), milestone: draftMile });
    setDraftBody(''); setDraftKind('observation');
    setTimeout(() => taRef.current && taRef.current.focus(), 30);
  }
  function onKey(e) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); commit(); }
  }

  const kindLabels = {
    observation: 'Observation', risk: 'Risk', stress_test: 'Stress test',
    decision: 'Decision', integrity_call: 'Integrity call',
  };

  return (
    <div>
      <div className="qe">
        <div className="qe-num">§{248 + (state.log.length - D.PILOT_LOG.length)}</div>
        <div className="qe-kind">
          <select value={draftKind} onChange={e => setDraftKind(e.target.value)}>
            {Object.entries(kindLabels).map(([k, lbl]) => <option key={k} value={k}>{lbl}</option>)}
          </select>
        </div>
        <div className="qe-body">
          <textarea ref={taRef} placeholder="Pilot's note. Cmd+Enter to file. Specific is kind."
            value={draftBody} onChange={e => setDraftBody(e.target.value)} onKeyDown={onKey} rows={2}/>
        </div>
        <div className="qe-action" onClick={commit}>FILE<br/>§</div>
        <div className="qe-meta">
          <span><strong>Milestone</strong> <select className="so-select" style={{display:'inline-block', width:120, padding:'1px 5px', fontFamily:'var(--font-mono)', fontSize:10.5}} value={draftMile} onChange={e => setDraftMile(e.target.value)}>
            {state.milestones.map(m => <option key={m.id} value={m.id}>{m.id}</option>)}
          </select></span>
          <span><strong>By</strong> CR · C. Rivas</span>
          <span style={{marginLeft:'auto'}}>⌘+↵ files · Esc clears</span>
        </div>
      </div>

      <div style={{background:'var(--surface)', border:'1px solid var(--hairline)', borderRadius:5, boxShadow:'var(--shadow-1)'}}>
        {state.log.map(e => (
          <LogEntry key={e.num} num={e.num} kind={e.kind} by={e.by} time={e.time}>
            <span style={{
              display:'inline-block', fontFamily:'var(--font-mono)', fontSize:10,
              padding:'1px 6px', marginRight:8, color:'var(--accent)',
              border:'1px solid var(--accent-rule)', borderRadius:3, verticalAlign:'2px',
            }}>{e.milestone}</span>
            {e.body}
            {e.integrity && <span style={{marginLeft:8}}>
              <span className={'dx-pill dx-pill--' + ({intact:'g',drifting:'y',invalid:'r'}[e.integrity])}>integrity: {e.integrity}</span>
            </span>}
          </LogEntry>
        ))}
      </div>
    </div>
  );
}

/* ===================== Decision Blocks ===================== */
function DecisionsView({ state, actions }) {
  return (
    <Sheet
      columns={[
        { key:'id', label:'ID', width:88, frozen:true, readonly:true,
          render: v => <span className="dx-id">{v}</span> },
        { key:'milestone', label:'Milestone', width:120, frozen:true, readonly:true,
          render: v => <span style={{fontFamily:'var(--font-mono)', fontSize:11.5, color:'var(--accent)', fontWeight:500}}>{v}</span> },
        { key:'summary', label:'What needs deciding', width:420,
          render: v => <span style={{color:'var(--ink-1)'}}>{v}</span> },
        { key:'status', label:'Status', width:130, kind:'select',
          options: [{value:'open',label:'Open'},{value:'resolved',label:'Resolved'},{value:'escalated',label:'Escalated'}],
          render: renderStatus },
        { key:'owner', label:'Owner', width:150,
          edit: ({value, commit, cancel}) => <ActorPicker value={value} commit={commit} cancel={cancel}/>,
          render: v => renderActor(v) },
        { key:'opened', label:'Opened', width:110, readonly:true,
          render: v => <span style={{fontFamily:'var(--font-mono)', fontSize:11, color:'var(--ink-3)'}}>{v}</span> },
        { key:'idleDays', label:'Idle', width:80, readonly:true, num:true, align:'right',
          render: v => {
            const kind = v >= 9 ? 'r' : v >= 5 ? 'y' : 'g';
            return v === 0 ? <span style={{color:'var(--ink-4)', fontFamily:'var(--font-mono)'}}>—</span>
              : <span className={'dx-pill dx-pill--' + kind} style={{fontFamily:'var(--font-mono)'}}>{v}d</span>;
          }},
      ]}
      rows={state.decisions}
      selectedIds={state.selectedIds}
      setSelectedIds={actions.setSelectedIds}
      onUpdate={(id, patch) => actions.updateDecision(id, patch)}
      onAddRow={() => actions.newDecision()}
      addLabel="Open DecisionBlock…"
    />
  );
}

/* ===================== Outcome Assessments ===================== */
function OutcomesView({ state, actions }) {
  return (
    <Sheet
      columns={[
        { key:'id', label:'ID', width:88, frozen:true, readonly:true,
          render: v => <span className="dx-id">{v}</span> },
        { key:'milestone', label:'Milestone', width:120, frozen:true, readonly:true,
          render: v => <span style={{fontFamily:'var(--font-mono)', fontSize:11.5, color:'var(--accent)', fontWeight:500}}>{v}</span> },
        { key:'status', label:'Status', width:120, kind:'select',
          options:[{value:'pending',label:'Pending'},{value:'completed',label:'Completed'},{value:'overdue',label:'Overdue'}],
          render: renderStatus },
        { key:'due', label:'SLA due', width:110, readonly:true,
          render: v => <span style={{fontFamily:'var(--font-mono)', fontSize:11, color:'var(--ink-3)'}}>{v}</span> },
        { key:'result', label:'Verdict', width:120, kind:'select',
          options:[{value:'—',label:'—'},{value:'achieved',label:'Achieved'},{value:'partial',label:'Partial'},{value:'missed',label:'Missed'},{value:'aborted',label:'Aborted'}],
          render: v => v === '—' ? <span className="placeholder">—</span> : <span className={'dx-pill dx-pill--' + ({achieved:'g',partial:'y',missed:'r',aborted:'neutral'}[v])}>{v}</span> },
        { key:'value', label:'Value', width:96, kind:'select',
          options:[{value:'—',label:'—'},{value:'high',label:'high'},{value:'medium',label:'medium'},{value:'low',label:'low'}],
          render: v => v === '—' || !v ? <span className="placeholder">—</span> : <span className={'dx-pill dx-pill--' + ({high:'g',medium:'y',low:'r'}[v])}>{v}</span> },
        { key:'by', label:'Recorded by', width:160,
          edit: ({value, commit, cancel}) => <ActorPicker value={value} commit={commit} cancel={cancel}/>,
          render: v => renderActor(v) },
        { key:'note', label:'Note', width:440,
          render: v => v ? <span style={{fontFamily:'var(--font-serif)', fontStyle:'italic', color:'var(--ink-2)'}}>{v}</span> : <span className="placeholder">no note yet</span> },
      ]}
      rows={state.outcomes}
      selectedIds={state.selectedIds}
      setSelectedIds={actions.setSelectedIds}
      onUpdate={(id, patch) => actions.updateOutcome(id, patch)}
    />
  );
}

/* ===================== Role bindings ===================== */
function RolesView({ state, actions }) {
  return (
    <>
      <div className="cdiag" style={{marginBottom:14}}>
        <I.alert size={14}/>
        <div>
          <span className="lab">Diagnostic engine</span>
          Constraints are <strong>flagged, not enforced</strong>. Edit any binding inline — diagnostics recompute on commit. Nothing is locked.
        </div>
      </div>
      <Sheet
        columns={[
          { key:'milestone', label:'Milestone', width:120, frozen:true, readonly:true,
            render: v => <span style={{fontFamily:'var(--font-mono)', fontSize:11.5, color:'var(--accent)', fontWeight:500}}>{v}</span> },
          { key:'role', label:'Role', width:130, kind:'select',
            options: D.ROLES.map(r => ({value:r.slug, label: r.name})),
            render: v => <span className={'dx-pill dx-pill--' + (v === 'pilot' ? 'blue' : v === 'leadership' ? 'plum' : 'neutral')}>{v}</span> },
          { key:'actor', label:'Actor', width:200,
            edit: ({value, commit, cancel}) => <ActorPicker value={value} commit={commit} cancel={cancel}/>,
            render: v => renderActor(v) },
          { key:'_actorRole', label:'Actor role', width:140, readonly:true,
            render: (_v, ctx) => {
              const a = actorOf(ctx.row.actor);
              return a ? <span style={{fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-3)', letterSpacing:'0.06em', textTransform:'uppercase'}}>{a.role}</span> : <span className="placeholder">—</span>;
            }},
          { key:'diagnostic', label:'Constraint diagnostic', width:440, readonly:true,
            render: v => v
              ? <span className={'dx-diag' + (v.includes('violation') || v.includes('reports-to') ? ' dx-diag--violation' : '')}>
                  <I.alert size={11}/> {v}
                </span>
              : <span style={{fontFamily:'var(--font-mono)', fontSize:11, color:'var(--ryg-green)'}}>✓ ok</span> },
        ]}
        rows={state.bindings}
        selectedIds={state.selectedIds}
        setSelectedIds={actions.setSelectedIds}
        onUpdate={(id, patch) => actions.updateBinding(id, patch)}
        onAddRow={() => actions.newBinding()}
        addLabel="Bind role to milestone…"
      />
    </>
  );
}

/* ===================== ACK Workspace (fast bulk acking) ===================== */
const ACK_PIVOTS = [
  { value:'all',           label:'All' },
  { value:'needs_me_spec', label:'Needs my S-ACK' },
  { value:'needs_me_bld',  label:'Needs my B-ACK' },
  { value:'misaligned',    label:'Out of alignment' },
  { value:'rejected',      label:'Rejected' },
  { value:'aligned',       label:'Aligned' },
];

function lastAckOf(r, who) {
  if (!Array.isArray(r.ackHistory)) return null;
  return r.ackHistory.find(h => h.who === who);
}

function AckView({ state, actions, currentUser }) {
  const [pivot, setPivot] = useState('all');
  const milestones = state.milestones.filter(m => m.kind === 'milestone');
  const filtered = milestones.filter(m => {
    if (pivot === 'all') return true;
    if (pivot === 'needs_me_spec') return m.specifier === currentUser && m.specifierAck !== 'accepted';
    if (pivot === 'needs_me_bld')  return m.builder === currentUser && m.builderAck !== 'accepted';
    if (pivot === 'misaligned')    return ackRollup(m.specifierAck, m.builderAck) !== 'aligned';
    if (pivot === 'rejected')      return m.specifierAck === 'rejected' || m.builderAck === 'rejected';
    if (pivot === 'aligned')       return ackRollup(m.specifierAck, m.builderAck) === 'aligned';
    return true;
  });

  const counts = {
    me_spec: milestones.filter(m => m.specifier === currentUser && m.specifierAck !== 'accepted').length,
    me_bld:  milestones.filter(m => m.builder === currentUser && m.builderAck !== 'accepted').length,
    misaligned: milestones.filter(m => ackRollup(m.specifierAck, m.builderAck) !== 'aligned').length,
    rejected: milestones.filter(m => m.specifierAck === 'rejected' || m.builderAck === 'rejected').length,
  };

  return (
    <>
      <div className="sht-toolbar">
        <Pivot value={pivot} onChange={setPivot} options={ACK_PIVOTS}/>
        <span className="sht-count">
          <em>{counts.me_spec}</em> need your S-ACK · <em>{counts.me_bld}</em> need your B-ACK · <em>{counts.misaligned}</em> misaligned · <em>{counts.rejected}</em> rejected
        </span>
        <div className="actions">
          <span style={{fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-4)'}}>
            <span className="kbd">A</span> accept · <span className="kbd">P</span> pending · <span className="kbd">R</span> reject (cell focused)
          </span>
        </div>
      </div>

      <Sheet
        rowKey={r => r.id}
        rows={filtered}
        selectedIds={state.selectedIds}
        setSelectedIds={actions.setSelectedIds}
        activeId={state.activeId}
        onSelectRow={actions.openRecord}
        onUpdate={(id, patch) => actions.updateAck(id, patch)}
        navColumn={1}
        columns={[
          { key:'id', label:'ID', width:96, frozen:true, readonly:true,
            render: v => <span className="dx-id">{v}</span> },
          { key:'title', label:'Milestone', width:280, frozen:true,
            render: renderTitle },
          { key:'specifier', label:'Specifier', width:148,
            edit: ({value, commit, cancel}) => <ActorPicker value={value} commit={commit} cancel={cancel}/>,
            render: (v, ctx) => {
              const a = actorOf(v); const me = v === currentUser;
              return <span style={{display:'inline-flex',alignItems:'center',gap:6}}>
                {renderActor(v)}
                {me && <span className="ack-me">me</span>}
              </span>;
            }},
          { key:'specifierAck', label:'Specifier ACK', width:160,
            edit: ({value, commit, cancel}) => <AckPicker value={value} commit={commit} cancel={cancel}/>,
            render: (v, ctx) => {
              const last = lastAckOf(ctx.row, 'specifier');
              return (
                <span className="ack-cellbox ack-cellbox--rich">
                  <span className={'dx-pill dx-pill--' + (ACK_KIND[v||'pending'])}>{ACK_LABEL[v||'pending']}</span>
                  {last
                    ? <span className="ack-cellbox__when">{relativeDay(last.when)}</span>
                    : <span className="ack-cellbox__when ack-cellbox__when--ghost">never</span>}
                </span>
              );
            }},
          { key:'builder', label:'Builder', width:148,
            edit: ({value, commit, cancel}) => <ActorPicker value={value} commit={commit} cancel={cancel} agentOk/>,
            render: (v) => {
              const me = v === currentUser;
              return <span style={{display:'inline-flex',alignItems:'center',gap:6}}>
                {renderActor(v)}
                {me && <span className="ack-me">me</span>}
              </span>;
            }},
          { key:'builderAck', label:'Builder ACK', width:160,
            edit: ({value, commit, cancel}) => <AckPicker value={value} commit={commit} cancel={cancel}/>,
            render: (v, ctx) => {
              const last = lastAckOf(ctx.row, 'builder');
              return (
                <span className="ack-cellbox ack-cellbox--rich">
                  <span className={'dx-pill dx-pill--' + (ACK_KIND[v||'pending'])}>{ACK_LABEL[v||'pending']}</span>
                  {last
                    ? <span className="ack-cellbox__when">{relativeDay(last.when)}</span>
                    : <span className="ack-cellbox__when ack-cellbox__when--ghost">never</span>}
                </span>
              );
            }},
          { key:'_rollup', label:'Alignment', width:160, readonly:true, sortable:false,
            render: renderRollup },
          { key:'target', label:'Target', width:96, kind:'select',
            options: TARGETS.map(t => ({value:t,label:t})), sortable:true },
          { key:'_lastNote', label:'Last note', width:340, readonly:true,
            render: (_v, ctx) => {
              const all = ctx.row.ackHistory || [];
              const last = all[0];
              if (!last) return <span className="placeholder">no acks yet</span>;
              return (
                <span style={{display:'inline-flex', flexDirection:'column', minWidth:0, lineHeight:1.25}}>
                  <span style={{fontFamily:'var(--font-serif)', fontStyle:'italic', fontSize:12, color:'var(--ink-2)', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap'}}>"{last.note}"</span>
                  <span style={{fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-4)'}}>{last.who === 'specifier' ? 'S' : 'B'} · {actorOf(last.actor)?.initials || last.actor} · {last.when}</span>
                </span>
              );
            }},
        ]}
      />
    </>
  );
}
/* ===================== Attention (aggregated "for you" view) ===================== */
function AttentionView({ state, actions, currentUser }) {
  const ms = state.milestones;
  const now = new Date('2026-05-26');

  function daysSince(when) {
    if (!when) return 9999;
    return Math.floor((now - new Date(when)) / 86400000);
  }
  function daysUntil(iso, precision) {
    if (!iso) return 9999;
    let target;
    if (precision === 'Q' || /Q\d/.test(iso)) {
      const m = iso.match(/(\d{4}).*Q(\d)/);
      if (m) {
        const q = +m[2];
        const month = q * 3; // end of quarter
        target = new Date(`${m[1]}-${String(month).padStart(2,'0')}-30`);
      } else { target = new Date(iso); }
    } else {
      target = new Date(iso);
    }
    return Math.floor((target - now) / 86400000);
  }

  /* ----- buckets ----- */
  const acksNeededS = ms.filter(m => m.specifier === currentUser && m.specifierAck === 'pending');
  const acksNeededB = ms.filter(m => m.builder === currentUser && m.builderAck === 'pending');
  const acksRejected = ms.filter(m =>
    (m.specifier === currentUser && m.specifierAck === 'rejected') ||
    (m.builder === currentUser && m.builderAck === 'rejected') ||
    m.specifierAck === 'rejected' || m.builderAck === 'rejected');
  const targetPassed = ms.filter(m => {
    if (m.status === 'shipped' || m.status === 'aborted') return false;
    return daysUntil(m.targetISO || m.target, m.targetPrecision) < 0;
  });
  const targetSoon = ms.filter(m => {
    if (m.status === 'shipped' || m.status === 'aborted') return false;
    const d = daysUntil(m.targetISO || m.target, m.targetPrecision);
    return d >= 0 && d <= 21;
  });
  const staleStatus = ms.filter(m =>
    (m.status === 'in_flight' || m.status === 'ack_committed') &&
    daysSince(m.statusUpdatedAt) > 14);
  const myDiagnostics = ms.filter(m =>
    (m.specifier === currentUser || m.builder === currentUser) &&
    (m.diagnostics || []).length > 0);
  const myDecisions = state.decisions.filter(d => d.status === 'open' && d.idleDays >= 5);
  const myOutcomes = state.outcomes.filter(o => o.status === 'overdue');
  const myRisksHigh = ms.flatMap(m =>
    (m.specifier === currentUser || m.builder === currentUser)
      ? (m.risks || []).filter(r => r.severity === 'high').map(r => ({ ...r, milestone: m.id, milestoneTitle: m.title }))
      : []);
  const rfcsDeadOnArrival = (state.rfcs || []).filter(r => {
    if (r.status !== 'approved' && r.status !== 'resourced') return false;
    if (r.specifier !== currentUser) return false;
    const produced = ms.filter(m => m.fromRfc === r.id);
    if (produced.length > 0) return false;
    return daysSince(r.approvedAt) >= 30;
  });

  const total =
    acksNeededS.length + acksNeededB.length + acksRejected.length +
    targetPassed.length + targetSoon.length + staleStatus.length +
    myDiagnostics.length + myDecisions.length + myOutcomes.length +
    myRisksHigh.length + rfcsDeadOnArrival.length;

  return (
    <div className="att">
      <div className="att-summary">
        <div className="att-summary__line">
          <span className="att-summary__count">{total}</span>
          <span className="att-summary__lab">things on you, {actorOf(currentUser)?.name}.</span>
        </div>
        <div className="att-summary__quip">
          Specific is kind. Work through the list — the substrate catches every change.
        </div>
      </div>

      <AttSection title="ACKs needed from you" count={acksNeededS.length + acksNeededB.length} severity="warn">
        {acksNeededS.map(m => (
          <AttRow key={'sa-'+m.id} severity="warn"
            chip={<span className="dx-pill dx-pill--y" style={{fontSize:9.5}}>S-ACK pending</span>}
            id={m.id} title={m.title}
            context={`Builder ${actorOf(m.builder)?.name || '—'} · target ${formatTarget(m.targetISO || m.target, m.targetPrecision)}`}
            actions={[
              { label:'Accept', kind:'accent', onClick: () => actions.setAck(m.id, 'specifier', 'accepted') },
              { label:'Reject', kind:'ghost',  onClick: () => actions.setAck(m.id, 'specifier', 'rejected') },
            ]}
            onOpen={() => actions.openAttention(m.id, 'milestone', 'ACK')}/>
        ))}
        {acksNeededB.map(m => (
          <AttRow key={'ba-'+m.id} severity="warn"
            chip={<span className="dx-pill dx-pill--y" style={{fontSize:9.5}}>B-ACK pending</span>}
            id={m.id} title={m.title}
            context={`Specifier ${actorOf(m.specifier)?.name || '—'} · target ${formatTarget(m.targetISO || m.target, m.targetPrecision)}`}
            actions={[
              { label:'Accept', kind:'accent', onClick: () => actions.setAck(m.id, 'builder', 'accepted') },
              { label:'Reject', kind:'ghost',  onClick: () => actions.setAck(m.id, 'builder', 'rejected') },
            ]}
            onOpen={() => actions.openAttention(m.id, 'milestone', 'ACK')}/>
        ))}
      </AttSection>

      <AttSection title="Targets passed or imminent" count={targetPassed.length + targetSoon.length} severity={targetPassed.length ? 'crit' : 'warn'}>
        {targetPassed.map(m => {
          const d = daysUntil(m.targetISO || m.target, m.targetPrecision);
          return (
            <AttRow key={'tp-'+m.id} severity="crit"
              chip={<span className="dx-pill dx-pill--r" style={{fontSize:9.5}}>{Math.abs(d)}d past</span>}
              id={m.id} title={m.title}
              context={`Target ${formatTarget(m.targetISO || m.target, m.targetPrecision)} · status ${STATUS_LABEL[m.status]}`}
              actions={[
                { label:'Update', kind:'accent', onClick: () => actions.openAttention(m.id, 'milestone', 'Status') },
                { label:'Re-target', kind:'ghost', onClick: () => actions.openAttention(m.id, 'milestone', 'Stages') },
              ]}
              onOpen={() => actions.openAttention(m.id, 'milestone', 'Status')}/>
          );
        })}
        {targetSoon.map(m => {
          const d = daysUntil(m.targetISO || m.target, m.targetPrecision);
          return (
            <AttRow key={'ts-'+m.id} severity="warn"
              chip={<span className="dx-pill dx-pill--y" style={{fontSize:9.5}}>{d}d to target</span>}
              id={m.id} title={m.title}
              context={`Target ${formatTarget(m.targetISO || m.target, m.targetPrecision)} · stages ${(m.stages||[]).filter(s => s.state==='done').length}/${(m.stages||[]).length} done`}
              actions={[
                { label:'Open', kind:'ghost', onClick: () => actions.openAttention(m.id, 'milestone', 'Stages') },
              ]}
              onOpen={() => actions.openAttention(m.id, 'milestone', 'Stages')}/>
          );
        })}
      </AttSection>

      <AttSection title="Stale status updates" count={staleStatus.length} severity="warn">
        {staleStatus.map(m => (
          <AttRow key={'st-'+m.id} severity="warn"
            chip={<span className="dx-pill dx-pill--y" style={{fontSize:9.5}}>{daysSince(m.statusUpdatedAt)}d silent</span>}
            id={m.id} title={m.title}
            context={`Last update by ${m.statusUpdatedBy ? actorOf(m.statusUpdatedBy)?.name : '—'} · RYG ${m.ryg.toUpperCase()}`}
            actions={[
              { label:'Update status', kind:'accent', onClick: () => actions.openAttention(m.id, 'milestone', 'Status') },
            ]}
            onOpen={() => actions.openAttention(m.id, 'milestone', 'Status')}/>
        ))}
      </AttSection>

      <AttSection title="High-severity risks on your work" count={myRisksHigh.length} severity="crit">
        {myRisksHigh.map((r, i) => (
          <AttRow key={'rh-'+i} severity="crit"
            chip={<span className="dx-pill dx-pill--r" style={{fontSize:9.5}}>{r.severity}</span>}
            id={r.milestone} title={r.milestoneTitle}
            context={r.body}
            actions={[{ label:'Open', kind:'ghost', onClick: () => actions.openAttention(r.milestone, 'milestone', 'Status') }]}
            onOpen={() => actions.openAttention(r.milestone, 'milestone', 'Status')}/>
        ))}
      </AttSection>

      <AttSection title="Diagnostics on your work" count={myDiagnostics.length} severity="warn">
        {myDiagnostics.map(m => (
          <AttRow key={'di-'+m.id} severity="warn"
            chip={<span className="dx-pill dx-pill--y" style={{fontSize:9.5}}>{(m.diagnostics||[]).length} flag{m.diagnostics.length===1?'':'s'}</span>}
            id={m.id} title={m.title}
            context={(m.diagnostics||[]).join(' · ')}
            actions={[{ label:'Open', kind:'ghost', onClick: () => actions.openAttention(m.id, 'milestone', 'ACK') }]}
            onOpen={() => actions.openAttention(m.id, 'milestone', 'ACK')}/>
        ))}
      </AttSection>

      <AttSection title="Decisions idle on your work" count={myDecisions.length} severity={myDecisions.some(d => d.idleDays>=9) ? 'crit' : 'warn'}>
        {myDecisions.map(d => (
          <AttRow key={'db-'+d.id} severity={d.idleDays>=9 ? 'crit' : 'warn'}
            chip={<span className={'dx-pill dx-pill--' + (d.idleDays>=9 ? 'r' : 'y')} style={{fontSize:9.5}}>{d.idleDays}d idle</span>}
            id={d.id} title={d.summary}
            context={`On ${d.milestone} · owner ${actorOf(d.owner)?.name || d.owner}`}
            actions={[
              { label:'Resolve', kind:'accent', onClick: () => actions.updateDecision(d.id, {status:'resolved', idleDays:0}) },
              { label:'Escalate', kind:'ghost', onClick: () => actions.updateDecision(d.id, {status:'escalated'}) },
            ]}
            onOpen={() => { actions.setView('decisions'); actions.openRecord(d.id, 'decision'); }}/>
        ))}
      </AttSection>

      <AttSection title="Outcome assessments overdue" count={myOutcomes.length} severity="warn">
        {myOutcomes.map(o => (
          <AttRow key={'oa-'+o.id} severity="warn"
            chip={<span className="dx-pill dx-pill--r" style={{fontSize:9.5}}>overdue</span>}
            id={o.id} title={`Assess ${o.milestone}`}
            context={`SLA due ${o.due} · ${o.note}`}
            actions={[{ label:'Open', kind:'ghost', onClick: () => { actions.setView('outcomes'); actions.openRecord(o.id, 'outcome'); } }]}
            onOpen={() => { actions.setView('outcomes'); actions.openRecord(o.id, 'outcome'); }}/>
        ))}
      </AttSection>

      <AttSection title="Rejected ACKs in your view" count={acksRejected.length} severity="crit">
        {acksRejected.map(m => (
          <AttRow key={'rj-'+m.id} severity="crit"
            chip={<span className="dx-pill dx-pill--r" style={{fontSize:9.5}}>rejected</span>}
            id={m.id} title={m.title}
            context={(m.ackHistory || []).find(h => h.action === 'rejected')?.note || ''}
            actions={[{ label:'Open', kind:'ghost', onClick: () => actions.openAttention(m.id, 'milestone', 'ACK') }]}
            onOpen={() => actions.openAttention(m.id, 'milestone', 'ACK')}/>
        ))}
      </AttSection>

      <AttSection title="Approved RFCs with nothing spawned" count={rfcsDeadOnArrival.length} severity="warn">
        {rfcsDeadOnArrival.map(r => (
          <AttRow key={'rfc-'+r.id} severity="warn"
            chip={<span className="dx-pill dx-pill--plum" style={{fontSize:9.5}}>RFC · 30d+ idle</span>}
            id={r.id} title={r.title}
            context={`Approved ${r.approvedAt} · ${r.summary}`}
            actions={[
              { label:'Spawn milestone', kind:'accent', onClick: () => actions.spawnMilestone(r.id) },
              { label:'Open RFC', kind:'ghost', onClick: () => { actions.setView('rfcs'); actions.openRecord(r.id, 'rfc'); } },
            ]}
            onOpen={() => { actions.setView('rfcs'); actions.openRecord(r.id, 'rfc'); }}/>
        ))}
      </AttSection>
    </div>
  );
}

function AttSection({ title, count, severity, children }) {
  if (count === 0) return null;
  return (
    <div className="att-section">
      <div className="att-section__head">
        <span className={'att-sev att-sev--' + (severity || 'info')}/>
        <h3 className="att-section__title">{title}</h3>
        <span className="att-section__count">{count}</span>
      </div>
      <div className="att-rows">{children}</div>
    </div>
  );
}

function AttRow({ severity, chip, id, title, context, actions, onOpen }) {
  return (
    <div className="att-row" onClick={onOpen}>
      <span className={'att-sev att-sev--' + (severity || 'info')}/>
      <span className="att-row__chip">{chip}</span>
      <span className="att-row__id dx-id">{id}</span>
      <span className="att-row__body">
        <span className="att-row__title">{title}</span>
        <span className="att-row__ctx">{context}</span>
      </span>
      <span className="att-row__actions" onClick={e => e.stopPropagation()}>
        {(actions || []).map((a, i) => (
          <Btn key={i} kind={a.kind || 'ghost'} size="sm" onClick={a.onClick}>{a.label}</Btn>
        ))}
      </span>
    </div>
  );
}
const RESULT_LABEL = { achieved:'Achieved', partial:'Partial', missed:'Missed', in_progress:'In progress', aborted:'Aborted' };
const RESULT_KIND  = { achieved:'g', partial:'y', missed:'r', in_progress:'blue', aborted:'neutral' };
function TaxonomyView({ state, actions }) {
  const [tax, setTax] = useState('org');
  const def = D.TAXONOMIES[tax] || {nodes:[], levels:[]};
  const nodes = def.nodes;
  const isGoals = tax === 'goals';

  function depthOf(slug) { return slug.split('/').length - 1; }

  return (
    <>
      <div className="sht-toolbar">
        <Pivot value={tax} onChange={setTax} options={[
          { value:'org', label:'Organization' },
          { value:'product', label:'Product' },
          { value:'goals', label:'Goals' },
        ]}/>
        <span className="sep">·</span>
        <span className="label-tag">Levels</span>
        <span style={{fontFamily:'var(--font-mono)', fontSize:11, color:'var(--ink-3)'}}>
          {def.levels.join(' → ')}
        </span>
        <div className="actions">
          <Btn kind="ghost" icon="plus" size="sm">Add node</Btn>
          <Btn kind="ghost" icon="edit" size="sm">Move</Btn>
        </div>
      </div>
      <div style={{background:'var(--surface)', border:'1px solid var(--hairline)', borderRadius:5, boxShadow:'var(--shadow-1)'}}>
        <div style={{display:'grid', gridTemplateColumns: isGoals ? '1fr 200px 160px 140px' : '1fr 200px 160px 80px', padding:'0 14px', background:'var(--paper-2)', borderBottom:'1px solid var(--hairline-strong)', height:32, alignItems:'center', fontFamily:'var(--font-mono)', fontSize:9.5, color:'var(--ink-3)', textTransform:'uppercase', letterSpacing:'0.1em'}}>
          <span>Name / slug</span><span>Parent</span><span>{isGoals ? 'Result note' : 'Used by'}</span><span style={{textAlign:'right'}}>{isGoals ? 'Result' : 'State'}</span>
        </div>
        {nodes.map(n => {
          const dpt = depthOf(n.slug);
          const usedBy = !isGoals
            ? state.milestones.filter(m => (tax==='org' ? m.orgNode : m.productNode) === n.slug).length
            : 0;
          return (
            <div key={n.slug} className="tx-node">
              <span className="indent" style={{width: 8 + dpt * 18}}/>
              <span style={{display:'flex', flexDirection:'column', minWidth:0, flex:1}}>
                <span className="name">{n.name}</span>
                <span className="slug">{n.slug}</span>
              </span>
              <span style={{width:200, fontFamily:'var(--font-mono)', fontSize:10.5, color:'var(--ink-3)'}}>{n.parent || '—'}</span>
              <span style={{width:160, fontFamily:'var(--font-mono)', fontSize:11, color: (!isGoals && usedBy) ? 'var(--accent)' : 'var(--ink-4)'}}>
                {isGoals ? (n.resultNote || (dpt < 2 ? '—' : 'unmeasured')) : (usedBy > 0 ? `${usedBy} milestone${usedBy>1?'s':''}` : 'unused')}
              </span>
              <span style={{width: isGoals ? 140 : 80, textAlign:'right'}}>
                {isGoals
                  ? (n.result
                      ? <span className={'dx-pill dx-pill--' + RESULT_KIND[n.result]}>{RESULT_LABEL[n.result]}</span>
                      : <span style={{fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-4)'}}>{dpt === 0 ? 'goal' : 'node'}</span>)
                  : (n.retired
                      ? <span className="dx-pill dx-pill--neutral">retired</span>
                      : <span style={{fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ryg-green)'}}>active</span>)}
                <span className="actions" style={{marginLeft:8}}><I.dot3 size={11}/></span>
              </span>
            </div>
          );
        })}
      </div>
    </>
  );
}

/* ===================== Event log (substrate-receipts read-only) ===================== */
function EventLogView({ state }) {
  return (
    <div style={{background:'var(--surface)', border:'1px solid var(--hairline)', borderRadius:5, boxShadow:'var(--shadow-1)'}}>
      <div className="evt-row" style={{background:'var(--paper-2)', borderBottom:'1px solid var(--hairline-strong)', fontFamily:'var(--font-mono)', fontSize:9.5, color:'var(--ink-3)', textTransform:'uppercase', letterSpacing:'0.1em', fontWeight:500}}>
        <span>§</span><span>Type</span><span>Target</span><span>Payload</span><span>Actor</span><span style={{textAlign:'right'}}>Time</span>
      </div>
      {state.events.map(e => (
        <div key={e.num} className="evt-row">
          <span className="sec">§{e.num}</span>
          <span className="typ">{e.type}</span>
          <span className="tgt">{e.target}</span>
          <span className="pay">{e.payload}</span>
          <span style={{display:'inline-flex',alignItems:'center',gap:6}}>
            {renderActor(e.actor, {compact:true})}
            <span className="sig">{e.sig}</span>
          </span>
          <span className="tim">{e.time.slice(5)}</span>
        </div>
      ))}
    </div>
  );
}

/* ===================== Leadership KPIs (top of portfolio) ===================== */
function LeadershipKpis({ data }) {
  return (
    <div className="kpi-row">
      <KPI label="Dependency closure rate"
        value={Math.round(data.depRate.value * 100)} unit="%" delta={data.depRate.delta} deltaKind="up"
        spark={data.depRate.spark} color="var(--ryg-green)"/>
      <KPI label="Scope-change velocity"
        value={data.scopeVel.value} unit={data.scopeVel.unit} delta={data.scopeVel.delta} deltaKind="warn"
        spark={data.scopeVel.spark} color="var(--ryg-yellow)"/>
      <KPI label="Decision friction"
        value={data.decFric.value} unit={data.decFric.unit} delta={data.decFric.delta} deltaKind="down"
        spark={data.decFric.spark} color="var(--ryg-red)"/>
      <KPI label="ACK-to-start latency"
        value={data.ackLat.value} unit={data.ackLat.unit} delta={data.ackLat.delta} deltaKind="up"
        spark={data.ackLat.spark} color="var(--accent)"/>
    </div>
  );
}

/* ===================== Customer Roadmap (public projection) ===================== */
function RoadmapView({ state }) {
  const visible = state.milestones.filter(m => m.customerVisible && (m.status === 'ack_committed' || m.status === 'in_flight' || m.status === 'shipped'));
  const byQuarter = {};
  for (const m of visible) {
    if (!byQuarter[m.target]) byQuarter[m.target] = [];
    byQuarter[m.target].push(m);
  }
  return (
    <div>
      <div style={{padding:'8px 0 14px', fontFamily:'var(--font-serif)', fontStyle:'italic', fontSize:14, color:'var(--ink-2)', maxWidth:'72ch'}}>
        Read-only projection over committed ACKs. No auth. No RYG. No internal commentary. Just the scope and the timing the team has committed to.
      </div>
      {Object.entries(byQuarter).map(([q, items]) => (
        <div className="roadmap-quarter" key={q}>
          <h3>{q}</h3>
          {items.map(m => (
            <div key={m.id} className="roadmap-card">
              <span style={{flex:1}}>
                <div className="ttl">{m.title}</div>
                <div className="sub">{m.productNode ? m.productNode.split('/').slice(-2).join(' / ') : ''}</div>
              </span>
              <span className="pill">{m.status === 'shipped' ? <span className="dx-pill dx-pill--neutral">shipped</span> : <span className="dx-pill dx-pill--blue">committed</span>}</span>
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}

Object.assign(window, {
  PortfolioView, RfcsView, PilotLogView, DecisionsView, OutcomesView,
  RolesView, TaxonomyView, EventLogView, LeadershipKpis, RoadmapView, AckView, AttentionView,
  PORTFOLIO_PRESETS, presetFor,
  renderActor, renderStatus, renderAckCell, renderRollup, ackRollup, ACK_STATES, ACK_LABEL, ACK_KIND,
  AckPicker, renderRyg, renderFresh, renderDepClosed,
  renderProductPath, renderOrgPath, renderTitle, renderTitleTree, renderKind, renderStages, renderTarget,
  renderStatusCell, renderRisksCell, renderNextCell, TargetEditor,
  formatTarget, treeSorted, SEV_KIND,
  STATUS_LABEL, ACK_KIND, TARGETS, M_STATUSES, R_STATUSES, actorOf, relativeDay, ROLLUP_LABEL, ROLLUP_KIND,
});
