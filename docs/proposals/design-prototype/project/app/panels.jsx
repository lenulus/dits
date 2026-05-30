/* Detail panel (slide-over) + ⌘K command palette + small helpers */

const DP_TABS_MILESTONE = ['ACK','Status','Stages','Deps','Updates','Receipts'];

function RecordPanel({ record, kind, onClose, actions, state, initialTab }) {
  const [tab, setTab] = useState(initialTab || (kind === 'milestone' ? 'ACK' : 'Detail'));
  useEffect(() => { setTab(initialTab || (kind === 'milestone' ? 'ACK' : 'Detail')); }, [record?.id, kind, initialTab]);
  if (!record) return null;

  let body = null;
  if (kind === 'milestone') body = <MilestoneDetail record={record} tab={tab} state={state} actions={actions}/>;
  else if (kind === 'rfc')      body = <RfcDetail record={record} actions={actions} state={state}/>;
  else if (kind === 'decision') body = <DecisionDetail record={record} actions={actions}/>;
  else if (kind === 'outcome')  body = <OutcomeDetail record={record} actions={actions}/>;

  const tabs = kind === 'milestone' ? DP_TABS_MILESTONE : ['Detail'];

  return (
    <SlideOver
      open
      onClose={onClose}
      id={record.id + (kind === 'milestone' ? ' · §' + Math.floor(247 + (record.id?.charCodeAt(5)||0) % 30) : '')}
      title={record.title || record.summary || ''}
      tabs={tabs}
      activeTab={tab}
      onTab={setTab}
      footer={
        <div style={{display:'flex', gap:8, alignItems:'center'}}>
          {kind === 'milestone' && (record.specifierAck !== 'accepted' || record.builderAck !== 'accepted') && (
            <Btn kind="accent" icon="check" onClick={() => actions.acceptBoth(record.id)}>Accept both</Btn>
          )}
          {kind === 'milestone' && (record.specifierAck === 'accepted' && record.builderAck === 'accepted') && (
            <Btn kind="secondary" icon="edit" onClick={() => actions.amendAck(record.id)}>Amend (clears ACKs)</Btn>
          )}
          {kind === 'decision' && record.status === 'open' && (
            <>
              <Btn kind="accent" icon="check" onClick={() => actions.updateDecision(record.id, {status:'resolved', idleDays:0})}>Resolve</Btn>
              <Btn kind="ghost" icon="alert" onClick={() => actions.updateDecision(record.id, {status:'escalated'})}>Escalate</Btn>
            </>
          )}
          <div style={{marginLeft:'auto', fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-4)'}}>
            signed · {record.sig || 'a1b2c3d4'}
          </div>
        </div>
      }>
      {body}
    </SlideOver>
  );
}

/* -------- Milestone detail body -------- */
function MilestoneDetail({ record, tab, state, actions }) {
  if (tab === 'ACK') return <MilestoneAck record={record} actions={actions}/>;
  if (tab === 'Status') return <MilestoneStatus record={record} actions={actions}/>;
  if (tab === 'Stages') return <MilestoneStages record={record}/>;
  if (tab === 'Deps') return <MilestoneDeps record={record} state={state} actions={actions}/>;
  if (tab === 'Updates') return <MilestoneUpdates record={record} state={state}/>;
  if (tab === 'Receipts') return <MilestoneActivity record={record} state={state}/>;
  return null;
}

function InlineEditableField({ value, onCommit, kind, options, mono, multiline, placeholder }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value ?? '');
  useEffect(() => setDraft(value ?? ''), [value]);
  const ref = React.useRef();
  useEffect(() => { if (editing && ref.current) { ref.current.focus(); if (ref.current.select) ref.current.select(); } }, [editing]);
  function commit() { onCommit && onCommit(draft); setEditing(false); }
  function cancel() { setDraft(value); setEditing(false); }
  function onKey(e) {
    if (e.key === 'Escape') { e.preventDefault(); cancel(); }
    if (e.key === 'Enter' && !e.shiftKey && !multiline) { e.preventDefault(); commit(); }
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && multiline) { e.preventDefault(); commit(); }
  }
  if (!editing) {
    return (
      <span className="so-edit" onClick={() => setEditing(true)}
        style={{fontFamily: mono ? 'var(--font-mono)' : 'inherit'}}>
        <span className="so-edit__val">
          {value || <span className="placeholder">{placeholder || '—'}</span>}
        </span>
        <I.edit size={11} className="so-edit__pen"/>
      </span>
    );
  }
  if (kind === 'select') {
    return (
      <select ref={ref} className="so-select" value={draft} onChange={e => setDraft(e.target.value)}
        onBlur={commit} onKeyDown={onKey}>
        {options.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
      </select>
    );
  }
  if (multiline) {
    return <textarea ref={ref} className="so-textarea" value={draft} onChange={e => setDraft(e.target.value)} onBlur={commit} onKeyDown={onKey}/>;
  }
  return <input ref={ref} className="so-input" value={draft} onChange={e => setDraft(e.target.value)} onBlur={commit} onKeyDown={onKey}/>;
}

function MilestoneAck({ record, actions }) {
  const sAck = record.specifierAck || 'pending';
  const bAck = record.builderAck || 'pending';
  const rollup = ackRollup(sAck, bAck);
  const history = Array.isArray(record.ackHistory) ? record.ackHistory : [];
  const lastSpec = history.find(h => h.who === 'specifier');
  const lastBld  = history.find(h => h.who === 'builder');
  const [editingTarget, setEditingTarget] = useState(false);

  return (
    <>
      <div className="so-meta">
        {renderStatus(record.status)}
        <span className={'dx-pill dx-pill--' + ROLLUP_KIND[rollup]}>{ROLLUP_LABEL[rollup]}</span>
        {renderRyg(record.ryg)}
        {renderFresh(null, {row: record})}
        <span style={{marginLeft:'auto'}}>kind=milestone</span>
      </div>

      {record.diagnostics && record.diagnostics.length > 0 && (
        <div className="diag-stack">
          {record.diagnostics.map((d, i) => (
            <div key={i} className={'diag-line' + (/reports-to|collapse/i.test(d) ? ' diag-line--violation' : '')}>
              <I.alert size={11}/>
              <span className="diag-line__lab">{(/reports-to|collapse/i.test(d) ? 'violation' : 'advisory')}</span>
              <span className="diag-line__body">{d}</span>
            </div>
          ))}
        </div>
      )}

      {/* WHO is on this milestone */}
      <div className="so-section" style={{marginTop:0}}>
        <span>Roles &amp; classification</span>
        <span className="so-section__hint">Specifier authors · Builder commits · Pilot observes</span>
      </div>
      <FieldRow label="Specifier">
        <PanelActorEdit value={record.specifier}
          onCommit={v => actions.updateMilestone(record.id, {specifier: v})}/>
      </FieldRow>
      <FieldRow label="Builder">
        <PanelActorEdit value={record.builder} agentOk
          onCommit={v => actions.updateMilestone(record.id, {builder: v})}/>
      </FieldRow>
      <FieldRow label="Pilot">
        <PanelActorEdit value={record.pilot}
          onCommit={v => actions.updateMilestone(record.id, {pilot: v})}/>
      </FieldRow>
      <FieldRow label="Org node">
        <PanelTaxEdit value={record.orgNode} taxonomy="org" placeholder="unclassified"
          onCommit={v => actions.updateMilestone(record.id, {orgNode: v})}/>
      </FieldRow>
      <FieldRow label="Product">
        <PanelTaxEdit value={record.productNode} taxonomy="product" placeholder="unclassified"
          onCommit={v => actions.updateMilestone(record.id, {productNode: v})}/>
      </FieldRow>

      {/* THE CONTRACT — what we are committing to */}
      <div className="so-section">
        <span>The commitment</span>
        <span className="so-section__hint">what the Specifier and Builder are aligning to: scope, schedule, outcome</span>
      </div>
      <FieldRow label="Delivery target">
        {editingTarget ? (
          <TargetEditor value={record.target} row={record}
            commit={(patch) => { actions.updateMilestone(record.id, patch); setEditingTarget(false); }}
            cancel={() => setEditingTarget(false)}/>
        ) : (
          <button className="so-target" onClick={() => setEditingTarget(true)}>
            <span className="so-target__val">{formatTarget(record.targetISO || record.target, record.targetPrecision || 'Q')}</span>
            <span className="so-target__prec">{record.targetPrecision || 'Q'}</span>
            <I.edit size={11} className="so-target__ed"/>
          </button>
        )}
        <span className="so-target__hint">
          {(record.stages || []).length > 0 ? `${(record.stages || []).length} stages defined` : 'No staging defined yet'}
          <span className="so-link" onClick={() => actions._setTab && actions._setTab('Stages')}>edit stages →</span>
        </span>
      </FieldRow>
      {record.fromRfc && (
        <FieldRow label="Origin RFC">
          <span className="origin-rfc" onClick={() => actions.openRecord(record.fromRfc, 'rfc')}>
            <span className="dx-pill dx-pill--plum" style={{fontSize:10}}>RFC</span>
            <span className="dx-id" style={{color:'var(--accent)'}}>{record.fromRfc}</span>
            <span className="origin-rfc__title">spawned from this RFC →</span>
          </span>
        </FieldRow>
      )}
      <FieldRow label="Scope">
        <InlineEditableField multiline
          placeholder="What this milestone commits to. Be specific."
          value={record._scope || 'Stand up the substrate primitives for ACK lifecycle and emit signed events for filing, accepting, rejecting, and amending. Builder owns commit; Specifier owns amendments.'}
          onCommit={v => actions.updateMilestone(record.id, {_scope: v})}/>
      </FieldRow>
      <FieldRow label="Target outcome">
        <InlineEditableField multiline
          value={record._outcome || 'Reducer + scheduler can express the full ACK lifecycle end-to-end with no out-of-band state.'}
          onCommit={v => actions.updateMilestone(record.id, {_outcome:v})}/>
      </FieldRow>
      <FieldRow label="Acceptance">
        <InlineEditableField multiline
          value={record._accept || 'All five ACK events round-trip through the v2 query API with full attribution. Diagnostic surfaces flag any drift. No state changes occur without an event in the DAG.'}
          onCommit={v => actions.updateMilestone(record.id, {_accept:v})}/>
      </FieldRow>

      {/* Stages preview */}
      {(record.stages || []).length > 0 && (
        <FieldRow label="Stages">
          <div className="stages" style={{gap:14}}>
            {(record.stages || []).map((s, i) => (
              <span key={s.key || i} className={'stage stage--' + (s.state || 'open')}>
                <span className="stage__dot"/>
                <span className="stage__lab">{s.label}</span>
                <span className="stage__when">{s.date}</span>
              </span>
            ))}
          </div>
        </FieldRow>
      )}

      {/* THE ACKs — stand-behind from each role */}
      <div className="so-section">
        <span>The ACKs</span>
        <span className="so-section__hint">each role independently stands behind (or rejects)</span>
      </div>
      <div className="ack-stack">
        <AckSide who="specifier" actorId={record.specifier} state={sAck} last={lastSpec}
          onSet={(v) => actions.setAck(record.id, 'specifier', v)}/>
        <AckSide who="builder" actorId={record.builder} state={bAck} last={lastBld}
          onSet={(v) => actions.setAck(record.id, 'builder', v)}/>
      </div>

      <div className="so-section">
        <span>ACK history</span>
        <span className="so-section__count">{history.length}</span>
      </div>
      {history.length === 0 ? (
        <div className="empty-line">No ACKs filed yet. Roles haven’t stood behind this commitment.</div>
      ) : (
        <div className="ack-hist">
          {history.map((h, i) => (
            <div key={i} className="ack-hist__row">
              <span className="ack-hist__when">{h.when}</span>
              <span className="ack-hist__who">{h.who === 'specifier' ? 'S-ACK' : 'B-ACK'}</span>
              <span className={'dx-pill dx-pill--' + (ACK_KIND[h.action] || 'neutral')} style={{fontSize:10}}>{h.action}</span>
              <span style={{display:'inline-flex', alignItems:'center', gap:6}}>{renderActor(h.actor, {compact:true})}</span>
              <span className="ack-hist__note">{h.note}</span>
            </div>
          ))}
        </div>
      )}

    </>
  );
}

function PanelActorEdit({ value, onCommit, agentOk }) {
  const [editing, setEditing] = useState(false);
  if (editing) {
    return (
      <div className="panel-pick">
        <ActorPicker value={value} agentOk={agentOk}
          commit={(v) => { onCommit(v); setEditing(false); }}
          cancel={() => setEditing(false)}/>
      </div>
    );
  }
  return (
    <span className="so-edit" onClick={() => setEditing(true)}>
      <span className="so-edit__val">
        {value ? renderActor(value) : <span className="placeholder">unassigned</span>}
      </span>
      <I.edit size={11} className="so-edit__pen"/>
    </span>
  );
}

function PanelTaxEdit({ value, onCommit, taxonomy, placeholder }) {
  const [editing, setEditing] = useState(false);
  if (editing) {
    return (
      <div className="panel-pick">
        <TaxonomyPicker value={value} taxonomy={taxonomy}
          commit={(v) => { onCommit(v); setEditing(false); }}
          cancel={() => setEditing(false)}/>
      </div>
    );
  }
  return (
    <span className="so-edit" onClick={() => setEditing(true)}>
      <span className="so-edit__val" style={{fontFamily:'var(--font-mono)', fontSize:11.5}}>
        {value || <span className="placeholder">{placeholder || 'unclassified'}</span>}
      </span>
      <I.edit size={11} className="so-edit__pen"/>
    </span>
  );
}

function FieldRow({ label, children }) {
  return (
    <div className="so-line">
      <span className="lab">{label}</span>
      <span className="val">{children}</span>
    </div>
  );
}

function AckSide({ who, actorId, state, last, onSet }) {
  const a = actorOf(actorId);
  return (
    <div className={'ack-side ack-side--' + state}>
      <div className="ack-side__head">
        <span className="ack-side__who">{who === 'specifier' ? 'Specifier ACK' : 'Builder ACK'}</span>
        {a && <span className="ack-side__actor">{renderActor(actorId, {compact:false})}</span>}
      </div>
      <div className="ack-side__state">
        <span className={'dx-pill dx-pill--' + ACK_KIND[state]} style={{fontSize:12, padding:'4px 12px', height:24}}>{ACK_LABEL[state]}</span>
        {last && <span className="ack-side__when">last · {last.when}</span>}
        <div className="ack-side__btns">
          {ACK_STATES.map(s => (
            <button key={s} className={'ackpop__btn ackpop__btn--' + ACK_KIND[s] + (s === state ? ' is-active' : '')}
              onClick={() => onSet(s)}>
              {ACK_LABEL[s]}
            </button>
          ))}
        </div>
      </div>
      {last && last.note && (
        <div className="ack-side__note">“{last.note}”</div>
      )}
    </div>
  );
}

function _UnusedMilestoneLog({ record, state }) {
  const entries = state.log.filter(e => e.milestone === record.id);
  return (
    <div style={{margin:'-16px -20px -24px', borderTop:'1px solid var(--hairline)'}}>
      {entries.length === 0 && (
        <div style={{padding:'18px 20px', fontFamily:'var(--font-serif)', fontStyle:'italic', color:'var(--ink-3)', fontSize:13.5}}>
          No Pilot's Log entries on this milestone yet. Add an observation when the team's tempo feels off — silence is usually data.
        </div>
      )}
      {entries.map(e => (
        <LogEntry key={e.num} num={e.num} kind={e.kind} by={e.by} time={e.time}>
          {e.body}
          {e.integrity && <span style={{marginLeft:8}}>
            <span className={'dx-pill dx-pill--' + ({intact:'g',drifting:'y',invalid:'r'}[e.integrity])}>integrity: {e.integrity}</span>
          </span>}
        </LogEntry>
      ))}
    </div>
  );
}

function MilestoneStatus({ record, actions }) {
  const [draft, setDraft] = useState('');
  const taRef = React.useRef();
  const sev = { high:'r', medium:'y', low:'neutral' };
  const needsRisk = (record.ryg === 'r' || record.ryg === 'y') && (record.risks || []).length === 0;

  function commitStatus() {
    if (!draft.trim()) return;
    actions.addStatusUpdate(record.id, draft.trim());
    setDraft('');
  }

  return (
    <>
      <div className="so-section" style={{marginTop:0}}>Health</div>
      <div className="ryg-row">
        {['g','y','r'].map(v => (
          <button key={v} className={'ryg-btn ryg-btn--' + v + (record.ryg === v ? ' is-active' : '')}
            onClick={() => actions.updateMilestone(record.id, { ryg: v })}>
            <span className="ryg-btn__dot"/>
            <span className="ryg-btn__lab">{ {g:'Green', y:'Yellow', r:'Red'}[v] }</span>
          </button>
        ))}
      </div>

      <div className="so-section">Current status</div>
      {record.statusNarrative ? (
        <div className="status-now">
          <div className="status-now__body">{record.statusNarrative}</div>
          <div className="status-now__meta">
            posted {record.statusUpdatedAt ? relativeDay(record.statusUpdatedAt) : 'never'}
            {record.statusUpdatedBy && <> · by {actorOf(record.statusUpdatedBy)?.name}</>}
          </div>
        </div>
      ) : (
        <div className="status-now status-now--empty">No status posted yet. The silence is itself data — file one when you can.</div>
      )}
      <div className="status-post">
        <div className="status-post__lab">Replace with new update</div>
        <textarea ref={taRef} className="status-post__ta" placeholder="State of the milestone in one paragraph. Cmd+Enter files it."
          value={draft} onChange={e => setDraft(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) commitStatus(); }} rows={3}/>
        <div className="status-post__actions">
          <span style={{fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-4)'}}>⌘+↵ to post · supersedes the current status above</span>
          <Btn kind="accent" size="sm" onClick={commitStatus} disabled={!draft.trim()}>Post update</Btn>
        </div>
      </div>

      <div className="so-section">
        <span>Next steps</span>
        <span className="so-section__count">{(record.nextSteps || []).length}</span>
      </div>
      <div className="upd-list">
        {(record.nextSteps || []).length === 0 ? (
          <div className="empty-line">No explicit next steps. Implicit ones don’t count.</div>
        ) : (record.nextSteps || []).map((s, i) => (
          <div key={i} className="risk-row" style={{gridTemplateColumns:'1fr 130px 100px'}}>
            <span style={{fontFamily:'var(--font-serif)', fontSize:13, color:'var(--ink-1)', lineHeight:1.45}}>{s.body}</span>
            <span>{renderActor(s.owner, {compact:false})}</span>
            <span style={{fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-4)', textAlign:'right'}}>by {s.when}</span>
          </div>
        ))}
      </div>
      <div className="so-mini-add" onClick={() => {
        const body = prompt('Next step?');
        if (body) actions.addNextStep(record.id, body, record.builder || record.specifier || 'ejackson');
      }}>+ next step</div>

      <div className="so-section" style={{marginTop:18}}>
        <span>Risks</span>
        <span className="so-section__count">{(record.risks || []).length}</span>
        {needsRisk && <span className="so-section__req">required for {record.ryg === 'r' ? 'Red' : 'Yellow'}</span>}
      </div>
      <div className="upd-list">
        {(record.risks || []).length === 0 ? (
          <div className="empty-line">
            {needsRisk ? <strong>RYG is {record.ryg === 'r' ? 'Red' : 'Yellow'} — the substrate expects at least one risk.</strong> : 'No risks logged. If the team has felt friction, name it.'}
          </div>
        ) : (record.risks || []).map((r, i) => (
          <div key={i} className="risk-row">
            <span><span className={'dx-pill dx-pill--' + sev[r.severity]} style={{fontSize:10}}>{r.severity}</span></span>
            <span style={{fontFamily:'var(--font-serif)', fontSize:13, color:'var(--ink-1)', lineHeight:1.45}}>{r.body}</span>
            <span style={{fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-4)', textAlign:'right'}}>{D.ACTORS[r.by]?.initials || r.by} · {r.when}</span>
          </div>
        ))}
      </div>
      <div className="so-mini-add" onClick={() => {
        const body = prompt('Risk description?');
        if (body) actions.addRisk(record.id, body, 'medium');
      }}>+ log risk</div>
    </>
  );
}

function MilestoneStages({ record }) {
  const stages = record.stages || [];
  return (
    <>
      <div className="so-meta">
        <span>{stages.length} stage{stages.length === 1 ? '' : 's'}</span>
        <span style={{marginLeft:'auto'}}>final target · {record.target}</span>
      </div>
      {stages.length === 0 && (
        <div style={{padding:'14px 0', fontFamily:'var(--font-serif)', fontStyle:'italic', color:'var(--ink-3)', fontSize:13.5}}>
          No staging defined. Add Dogfood / Beta / GA stages with their own dates.
        </div>
      )}
      <div style={{display:'flex', flexDirection:'column', gap:10, padding:'12px 0'}}>
        {stages.map((s, i) => (
          <div key={s.key || i} className={'stage-row stage-row--' + (s.state || 'open')}>
            <span className={'stage__dot'} style={{width:14, height:14}}/>
            <span>
              <div style={{fontFamily:'var(--font-display)', fontSize:15, fontWeight:500, color:'var(--ink-0)', letterSpacing:'-0.005em'}}>{s.label}</div>
              <div style={{fontFamily:'var(--font-mono)', fontSize:10.5, color:'var(--ink-3)'}}>{s.state === 'done' ? 'Completed' : s.state === 'soon' ? 'Next up' : 'Open'}</div>
            </span>
            <span style={{marginLeft:'auto', fontFamily:'var(--font-mono)', fontSize:13, color:'var(--ink-1)'}}>{s.date}</span>
            <span style={{fontFamily:'var(--font-mono)', fontSize:9, padding:'1px 5px', background:'var(--paper-2)', borderRadius:3, color:'var(--ink-4)', marginLeft:8}}>{s.precision}</span>
          </div>
        ))}
      </div>
      <div className="so-mini-add">+ add stage</div>
    </>
  );
}

function MilestoneDeps({ record, state, actions }) {
  const deps = record.deps || [];
  const dependents = state.milestones.filter(m => (m.deps || []).includes(record.id));
  const [adding, setAdding] = useState(false);
  const [q, setQ] = useState('');
  const candidates = state.milestones.filter(m =>
    m.id !== record.id && !deps.includes(m.id) && !dependents.find(d => d.id === m.id) &&
    (!q || m.id.toLowerCase().includes(q.toLowerCase()) || m.title.toLowerCase().includes(q.toLowerCase()))
  );

  function DepRow({ m, dir }) {
    return (
      <div className="dep-row" onClick={() => actions.openRecord(m.id, 'milestone')}>
        <span style={{fontFamily:'var(--font-mono)', fontSize:9, letterSpacing:'0.1em', textTransform:'uppercase', color:'var(--ink-4)', width:54}}>
          {dir === 'up' ? 'depends' : 'blocks'}
        </span>
        <span className="dx-id" style={{color:'var(--accent)', width:88}}>{m.id}</span>
        <span style={{flex:1, fontSize:13, color:'var(--ink-0)', fontWeight:500, overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap'}}>{m.title}</span>
        <span>{renderStatus(m.status)}</span>
        {dir === 'up' && (
          <span style={{marginLeft:6, fontFamily:'var(--font-mono)', fontSize:9, color:'var(--ink-4)', cursor:'pointer', padding:'2px 5px'}}
            onClick={(e) => { e.stopPropagation(); actions.removeDep(record.id, m.id); }}>remove</span>
        )}
      </div>
    );
  }

  return (
    <>
      <div className="so-section" style={{marginTop:0}}>
        <span>Upstream (this depends on)</span>
        <span className="so-section__count">{deps.length}</span>
      </div>
      <div className="dep-list">
        {deps.length === 0 && <div className="empty-line">No upstream dependencies.</div>}
        {deps.map(id => {
          const m = state.milestones.find(x => x.id === id);
          if (!m) return null;
          return <DepRow key={id} m={m} dir="up"/>;
        })}
      </div>
      {!adding ? (
        <div className="so-mini-add" onClick={() => setAdding(true)}>+ link dependency</div>
      ) : (
        <div className="dep-add">
          <input className="so-input" placeholder="Search milestones…" autoFocus value={q} onChange={e => setQ(e.target.value)}
            onKeyDown={e => { if (e.key === 'Escape') { setAdding(false); setQ(''); } }}/>
          <div className="dep-add__list">
            {candidates.slice(0, 6).map(m => (
              <div key={m.id} className="dep-add__opt"
                onClick={() => { actions.addDep(record.id, m.id); setAdding(false); setQ(''); }}>
                <span className="dx-id" style={{color:'var(--accent)'}}>{m.id}</span>
                <span style={{flex:1, fontSize:12, color:'var(--ink-0)', fontWeight:500, overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap'}}>{m.title}</span>
                {renderStatus(m.status)}
              </div>
            ))}
            {candidates.length === 0 && <div className="empty-line">No matches.</div>}
          </div>
        </div>
      )}

      <div className="so-section">
        <span>Downstream (blocks)</span>
        <span className="so-section__count">{dependents.length}</span>
      </div>
      <div className="dep-list">
        {dependents.length === 0 && <div className="empty-line">Nothing depends on this milestone.</div>}
        {dependents.map(m => <DepRow key={m.id} m={m} dir="down"/>)}
      </div>
    </>
  );
}

function MilestoneUpdates({ record, state }) {
  const entries = state.log.filter(e => e.milestone === record.id);
  const history = [
    { kind:'status_transition', from: record.status === 'in_flight' ? 'ack_committed' : 'ack_filed',
      to: record.status, by:'achen', when:'2026-05-08' },
    { kind:'status_transition', from:'ack_filed', to:'ack_committed', by:'achen', when:'2026-05-05' },
    { kind:'status_transition', from:'draft', to:'ack_filed', by:'ejackson', when:'2026-05-02' },
  ];
  const combined = [
    ...entries.map(e => ({ ...e, _t: e.time })),
    ...history.map(h => ({ ...h, _t: h.when })),
  ].sort((a, b) => (b._t || '').localeCompare(a._t || ''));

  return (
    <div style={{margin:'-16px -20px -24px'}}>
      <div className="upd-feed-bar">
        Combined feed · pilot-log entries and status transitions
      </div>
      {combined.length === 0 && (
        <div style={{padding:'18px 20px', fontFamily:'var(--font-serif)', fontStyle:'italic', color:'var(--ink-3)', fontSize:13.5}}>
          No updates yet on this milestone.
        </div>
      )}
      {combined.map((e, i) => {
        if (e.kind === 'status_transition') {
          return (
            <div key={'h-'+i} className="upd-feed-row upd-feed-row--transition">
              <span className="upd-feed-row__when">{e.when}</span>
              <span className="upd-feed-row__chip">
                <span className="dx-pill dx-pill--neutral" style={{fontSize:9.5}}>status</span>
              </span>
              <span className="upd-feed-row__body">
                <strong>{STATUS_LABEL[e.to]}</strong> <span style={{color:'var(--ink-4)'}}>← {STATUS_LABEL[e.from]}</span>
                <span style={{marginLeft:8, fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-4)'}}>by {actorOf(e.by)?.initials || e.by}</span>
              </span>
            </div>
          );
        }
        return (
          <div key={'e-'+e.num} className="upd-feed-row">
            <span className="upd-feed-row__when">§{e.num}</span>
            <span className="upd-feed-row__chip">
              <span className={'dx-pill dx-pill--' + (e.kind === 'risk' ? 'y' : e.kind === 'integrity_call' ? 'r' : e.kind === 'decision' ? 'plum' : 'blue')} style={{fontSize:9.5}}>{e.kind.replace('_',' ')}</span>
            </span>
            <span className="upd-feed-row__body">
              {e.body}
              {e.integrity && (
                <span style={{marginLeft:8}}>
                  <span className={'dx-pill dx-pill--' + ({intact:'g',drifting:'y',invalid:'r'}[e.integrity])} style={{fontSize:9.5}}>integrity: {e.integrity}</span>
                </span>
              )}
              <span style={{display:'block', marginTop:3, fontFamily:'var(--font-mono)', fontSize:10, color:'var(--ink-4)'}}>{actorOf(e.by)?.initials || e.by} · {e.time}</span>
            </span>
          </div>
        );
      })}
    </div>
  );
}

function MilestoneActivity({ record, state }) {
  const events = state.events.filter(e => e.target === record.id);
  return (
    <div style={{fontFamily:'var(--font-mono)', fontSize:11.5}}>
      {events.length === 0 && (
        <div style={{padding:'14px 0', fontFamily:'var(--font-serif)', fontStyle:'italic', color:'var(--ink-3)', fontSize:13.5}}>
          No events scoped to this milestone in the visible window.
        </div>
      )}
      {events.map(e => (
        <div key={e.num} className="rx-row" style={{padding:'6px 0', borderBottom:'1px solid var(--hairline)', color:'var(--ink-2)', gap:10}}>
          <span style={{color:'var(--accent)', width:60}}>§{e.num}</span>
          <span style={{flex:1}}>{e.type} · {e.payload}</span>
          <span style={{color:'var(--ink-4)'}}>{e.time.slice(5)}</span>
        </div>
      ))}
    </div>
  );
}

/* -------- RFC / Decision / Outcome detail bodies -------- */
function RfcDetail({ record, actions, state }) {
  const produced = (state?.milestones || []).filter(m => m.fromRfc === record.id && m.kind === 'milestone');
  return (
    <>
      <div className="so-meta">{renderStatus(record.status)} <span>target={record.target}</span>{record.approvedAt && <span style={{marginLeft:'auto'}}>approved {record.approvedAt}</span>}</div>
      <div className="so-line">
        <span className="lab">Specifier</span>
        <span className="val">{renderActor(record.specifier)}</span>
      </div>
      <div className="so-line">
        <span className="lab">Summary</span>
        <span className="val">
          <InlineEditableField multiline value={record.summary}
            onCommit={v => actions.updateRfc(record.id, {summary:v})}/>
        </span>
      </div>
      <div className="so-section">Acceptance criteria</div>
      <div className="so-line">
        <span className="val" style={{paddingLeft:0}}>
          <InlineEditableField multiline value={record._accept || ''}
            placeholder="What this RFC must satisfy to move to backlog."
            onCommit={v => actions.updateRfc(record.id, {_accept:v})}/>
        </span>
      </div>

      <div className="so-section">
        <span>Produced milestones</span>
        <span className="so-section__count">{produced.length}</span>
        {record.status === 'approved' && produced.length === 0 && (
          <span className="so-section__req">approved, nothing spawned</span>
        )}
      </div>
      {produced.length === 0 ? (
        <div className="empty-line">
          {record.status === 'approved' || record.status === 'resourced'
            ? 'No milestones produced from this RFC yet. The substrate is watching — approved RFCs that don\u2019t spawn work get flagged.'
            : 'Once this RFC is approved and a team picks it up, the milestones they spawn from it will land here.'}
        </div>
      ) : (
        <div className="dep-list">
          {produced.map(m => (
            <div key={m.id} className="dep-row" onClick={() => actions.openRecord(m.id, 'milestone')}>
              <span className="dx-id" style={{color:'var(--accent)', width:88}}>{m.id}</span>
              <span style={{flex:1, fontSize:13, color:'var(--ink-0)', fontWeight:500, overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap'}}>{m.title}</span>
              <span>{renderStatus(m.status)}</span>
              <span style={{marginLeft:6}}>{renderRollup(null, {row: m})}</span>
            </div>
          ))}
        </div>
      )}
      {(record.status === 'approved' || record.status === 'resourced') && (
        <div className="so-mini-add" onClick={() => actions.spawnMilestone(record.id)}>+ spawn milestone from this RFC</div>
      )}
    </>
  );
}

function DecisionDetail({ record, actions }) {
  return (
    <>
      <div className="so-meta">{renderStatus(record.status)} <span>opened {record.opened}</span> <span style={{marginLeft:'auto'}}>idle {record.idleDays}d</span></div>
      <div className="so-line">
        <span className="lab">Milestone</span>
        <span className="val rx-mono" style={{color:'var(--accent)', fontWeight:500}}>{record.milestone}</span>
      </div>
      <div className="so-line">
        <span className="lab">Owner</span>
        <span className="val">{renderActor(record.owner)}</span>
      </div>
      <div className="so-line">
        <span className="lab">Summary</span>
        <span className="val">
          <InlineEditableField multiline value={record.summary}
            onCommit={v => actions.updateDecision(record.id, {summary:v})}/>
        </span>
      </div>
      {record.idleDays >= 5 && record.status === 'open' && (
        <div className={'cdiag' + (record.idleDays >= 9 ? ' cdiag--violation' : '')} style={{marginTop:14}}>
          <I.alert size={14}/>
          <div>
            <span className="lab">Auto-escalation</span>
            {record.idleDays >= 9
              ? <>Threshold exceeded. Scheduler will emit <strong>review_requested → leadership</strong> on next poll.</>
              : <>{5 - (5 - record.idleDays)} day{5 - record.idleDays === 1 ? '' : 's'} until auto-escalation.</>}
          </div>
        </div>
      )}
    </>
  );
}

function OutcomeDetail({ record, actions }) {
  return (
    <>
      <div className="so-meta">{renderStatus(record.status)} <span>SLA {record.due}</span></div>
      <div className="so-line">
        <span className="lab">Verdict</span>
        <span className="val val--inline">
          <InlineEditableField kind="select" value={record.result}
            options={[{value:'—',label:'—'},{value:'achieved',label:'Achieved'},{value:'partial',label:'Partial'},{value:'missed',label:'Missed'},{value:'aborted',label:'Aborted'}]}
            onCommit={v => actions.updateOutcome(record.id, {result:v})}/>
          {' / '}
          <InlineEditableField kind="select" value={record.value}
            options={[{value:'—',label:'—'},{value:'high',label:'high'},{value:'medium',label:'medium'},{value:'low',label:'low'}]}
            onCommit={v => actions.updateOutcome(record.id, {value:v})}/>
        </span>
      </div>
      <div className="so-line">
        <span className="lab">Note</span>
        <span className="val">
          <InlineEditableField multiline value={record.note} placeholder="What landed, what didn't, what the team learned."
            onCommit={v => actions.updateOutcome(record.id, {note:v})}/>
        </span>
      </div>
      {record.result === 'achieved' && record.value === 'low' && (
        <div className="cdiag" style={{marginTop:14, background:'var(--ink-plum-paper)', borderColor:'#B08FA0', color:'var(--ink-plum)'}}>
          <I.alert size={14}/>
          <div>
            <span className="lab">Pattern</span>
            Landed but didn't matter. Worth a pattern block on Leadership.
          </div>
        </div>
      )}
    </>
  );
}

/* =========================================================== */
/* Command palette */
/* =========================================================== */
function CmdPalette({ open, onClose, actions, state }) {
  const [q, setQ] = useState('');
  const [active, setActive] = useState(0);

  const cmds = useMemo(() => {
    const out = [];

    // Navigation commands
    [
      ['Leadership',      'view',  'leadership', 'home'],
      ['Portfolio',       'view',  'portfolio',  'panel'],
      ['RFC review queue','view',  'rfcs',       'flag'],
      ["Pilot's Log",     'view',  'log',        'compass'],
      ['DecisionBlocks',  'view',  'decisions',  'scale'],
      ['Outcome assessments','view','outcomes',  'check'],
      ['Role bindings',   'view',  'roles',      'users'],
      ['Taxonomies',      'view',  'taxonomies', 'tag'],
      ['Event log',       'view',  'events',     'book'],
      ['Customer roadmap','view',  'roadmap',    'map'],
    ].forEach(([lbl,kind,id,icon]) => out.push({ label: lbl, group:'NAVIGATE', kind, id, icon, sub:'go' }));

    // Records — jump straight in
    state.milestones.forEach(m => out.push({ label: m.title, group:'MILESTONES', kind:'open', record:m, recordKind:'milestone', icon:'panel', sub: m.id }));
    state.rfcs.forEach(r => out.push({ label: r.title, group:'RFCS', kind:'open', record:r, recordKind:'rfc', icon:'flag', sub: r.id }));
    state.decisions.forEach(d => out.push({ label: d.summary.split('.')[0], group:'DECISIONS', kind:'open', record:d, recordKind:'decision', icon:'scale', sub: d.id }));

    // Actions
    out.push({ label:'New milestone',    group:'CREATE', kind:'create', what:'milestone', icon:'plus', sub:'+' });
    out.push({ label:'New RFC',          group:'CREATE', kind:'create', what:'rfc', icon:'flag', sub:'+' });
    out.push({ label:'Open DecisionBlock', group:'CREATE', kind:'create', what:'decision', icon:'scale', sub:'+' });
    out.push({ label:'File Pilot\'s log entry', group:'CREATE', kind:'view', id:'log', icon:'compass', sub:'go to log' });

    const filtered = q
      ? out.filter(c =>
          c.label.toLowerCase().includes(q.toLowerCase())
          || (c.sub || '').toLowerCase().includes(q.toLowerCase())
          || c.group.toLowerCase().includes(q.toLowerCase()))
      : out;
    return filtered;
  }, [q, state]);

  useEffect(() => setActive(0), [q]);
  useEffect(() => {
    if (!open) return;
    function onKey(e) {
      if (e.key === 'Escape') { onClose(); return; }
      if (e.key === 'ArrowDown') { e.preventDefault(); setActive(a => Math.min(cmds.length-1, a+1)); }
      if (e.key === 'ArrowUp')   { e.preventDefault(); setActive(a => Math.max(0, a-1)); }
      if (e.key === 'Enter')     { e.preventDefault(); const c = cmds[active]; if (c) run(c); }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, cmds, active, onClose]);

  function run(c) {
    if (c.kind === 'view') actions.setView(c.id);
    else if (c.kind === 'open') { actions.setView(c.recordKind === 'rfc' ? 'rfcs' : c.recordKind === 'decision' ? 'decisions' : 'portfolio'); actions.openRecord(c.record.id, c.recordKind); }
    else if (c.kind === 'create') {
      if (c.what === 'milestone') actions.newMilestone();
      else if (c.what === 'rfc') actions.newRfc();
      else if (c.what === 'decision') actions.newDecision();
    }
    onClose();
  }

  if (!open) return null;

  // Group adjacent
  const groups = [];
  let curr = null;
  cmds.forEach((c, i) => {
    if (!curr || curr.label !== c.group) { curr = { label: c.group, items: [] }; groups.push(curr); }
    curr.items.push({ ...c, idx: i });
  });

  return (
    <div className="cmd-overlay" onClick={onClose}>
      <div className="cmd" onClick={e => e.stopPropagation()}>
        <div className="cmd__input">
          <I.search size={16}/>
          <input autoFocus placeholder="Jump anywhere · create anything · type to filter…"
            value={q} onChange={e => setQ(e.target.value)}/>
          <span className="cmd__hint">esc closes</span>
        </div>
        <div className="cmd__list">
          {groups.slice(0, 6).flatMap(g => [
            <div key={'lab-'+g.label} className="cmd__group-lab">{g.label}</div>,
            ...g.items.slice(0, 6).map(c => {
              const Icon = I[c.icon] || I.panel;
              return (
                <div key={'opt-'+c.idx} className={'cmd__opt' + (c.idx === active ? ' is-active' : '')}
                  onMouseEnter={() => setActive(c.idx)}
                  onClick={() => run(c)}>
                  <Icon size={14} className="ic"/>
                  <span style={{flex:1}}>{c.label}</span>
                  <span className="sub">{c.sub}</span>
                </div>
              );
            })
          ])}
          {groups.length === 0 && (
            <div style={{padding:'14px 18px', fontFamily:'var(--font-serif)', fontStyle:'italic', color:'var(--ink-3)', fontSize:13}}>
              Nothing matches. Try shorter terms — IDs and titles both index.
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

Object.assign(window, { RecordPanel, CmdPalette, InlineEditableField, MilestoneStatus, MilestoneStages, MilestoneDeps, MilestoneUpdates });
