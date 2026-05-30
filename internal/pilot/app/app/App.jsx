/* App shell — state, routing, toolbars, hintbar. */
const D2 = window.DATA;

function App() {
  const [view, setView] = useState('attention');
  const [activeId, setActiveId] = useState(null);
  const [activeKind, setActiveKind] = useState('milestone');
  const [activeTab, setActiveTab] = useState(null);
  const [selectedIds, setSelectedIds] = useState(new Set());
  const [cmdOpen, setCmdOpen] = useState(false);
  const [pivot, setPivot] = useState('all');
  const [groupBy, setGroupBy] = useState('none');
  const [filters, setFilters] = useState([]);
  const [collapsedGroups, setCollapsedGroups] = useState(new Set(['health','classification','acks','roles','delivery','signals']));

  /* ----------- Mutable state (copies of seed so we can update) ----------- */
  const [milestones, setMilestones] = useState(D2.MILESTONES);
  const [rfcs,       setRfcs]       = useState(D2.RFCS);
  const [log,        setLog]        = useState(D2.PILOT_LOG);
  const [decisions,  setDecisions]  = useState(D2.DECISIONS);
  const [outcomes,   setOutcomes]   = useState(D2.OUTCOMES);
  const [bindings,   setBindings]   = useState(D2.ROLE_BINDINGS);
  const [events,     setEvents]     = useState(D2.EVENT_LOG);

  /* ----------- Open ⌘K on cmd-k anywhere ----------- */
  useEffect(() => {
    function onKey(e) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault(); setCmdOpen(true);
      }
      if (e.key === 'Escape' && !cmdOpen) {
        setActiveId(null);
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [cmdOpen]);

  /* ----------- Actions (mutators + side-effects) ----------- */
  function pushEvent(type, target, payload, actor='ejackson') {
    const max = events[0]?.num || 251;
    const e = { num: max+1, type, target, actor, time: new Date().toISOString().replace('T',' ').slice(0,16), payload, sig: rand8() };
    setEvents([e, ...events]);
  }

  /* persist fires the matching signed-event mutation against /api/mutate after
     an optimistic local update. Fire-and-forget: the UI already reflects the
     change; a full reload reconciles via /api/data. We never block on this. */
  function persist(action, payload) {
    fetch('/api/mutate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action, ...payload }),
    }).catch(err => console.warn('pilot: persist failed', action, err));
  }

  const actions = {
    setView: (v) => { setView(v); setActiveId(null); setSelectedIds(new Set()); },
    setSelectedIds,
    openRecord: (id, kind) => {
      setActiveId(id);
      if (kind) setActiveKind(kind);
      else {
        if (id?.startsWith('PROJ-')) {
          const m = milestones.find(x => x.id === id);
          if (m) setActiveKind(m.kind); // 'milestone' or 'rfc'
          else {
            const r = rfcs.find(x => x.id === id);
            if (r) setActiveKind('rfc');
            else setActiveKind('milestone');
          }
        } else if (id?.startsWith('DB-')) setActiveKind('decision');
        else if (id?.startsWith('OA-')) setActiveKind('outcome');
      }
    },
    closeRecord: () => setActiveId(null),
    updateMilestone: (id, patch) => {
      setMilestones(ms => ms.map(m => m.id === id ? { ...m, ...patch } : m));
      const key = Object.keys(patch)[0];
      if (key && !key.startsWith('_')) pushEvent('work.field_updated', id, `field=${key}`);
      persist('updateMilestone', { id, patch });
    },
    updateRfc: (id, patch) => {
      setRfcs(rs => rs.map(r => r.id === id ? { ...r, ...patch } : r));
      pushEvent('work.field_updated', id, `field=${Object.keys(patch)[0]}`);
      persist('updateRfc', { id, patch });
    },
    updateDecision: (id, patch) => {
      setDecisions(ds => ds.map(d => d.id === id ? { ...d, ...patch } : d));
      if (patch.status === 'resolved') pushEvent('work.closed', id, 'resolution=resolved');
      if (patch.status === 'escalated') pushEvent('work.review_requested', id, 'reviewer=leadership');
      persist('updateDecision', { id, patch });
    },
    updateOutcome: (id, patch) => {
      setOutcomes(os => os.map(o => o.id === id ? { ...o, ...patch } : o));
      if (patch.result) pushEvent('work.eval_completed', id, `result=${patch.result}`);
      persist('updateOutcome', { id, patch });
    },
    updateBinding: (id, patch) => {
      setBindings(bs => bs.map(b => {
        if (b.id !== id) return b;
        const next = { ...b, ...patch };
        // Recompute diagnostic
        next.diagnostic = computeBindingDiagnostic(next, bindings, milestones);
        return next;
      }));
      const b = bindings.find(x => x.id === id);
      pushEvent('work.role_bound', b?.milestone || '—', `role=${patch.role || '?'} · actor=${patch.actor || '?'}`);
      persist('updateBinding', { id: b?.milestone, role: patch.role || b?.role, actor: patch.actor !== undefined ? patch.actor : b?.actor });
    },
    addLogEntry: ({ kind, body, milestone }) => {
      const num = (log[0]?.num || 247) + 1;
      const e = { num, milestone, kind, by:'krivas', time: new Date().toISOString().replace('T',' ').slice(0,16), body };
      setLog([e, ...log]);
      pushEvent('work.observation_recorded', milestone, `pilot_log_entry_type=${kind}`, 'krivas');
      persist('addLogEntry', { id: milestone, kind, body });
    },
    newMilestone: () => {
      const id = 'PROJ-' + Math.floor(220 + Math.random()*40);
      const m = { id, kind:'milestone', title:'New milestone — name me', status:'draft', ryg:'g',
        specifierAck:'pending', builderAck:'pending', ackHistory:[],
        specifier:'ejackson', builder:null, pilot:null, target:'2026 Q3',
        orgNode:null, productNode:null, scopeChanges:0, depClosed:[0,0], fresh:'fresh', age:'0d', sig: rand8(),
        diagnostics:['pilot unassigned','no product classification'], customerVisible:false };
      setMilestones([m, ...milestones]);
      pushEvent('work.created', id, 'kind=milestone');
      setActiveId(id); setActiveKind('milestone');
      persist('newMilestone', { title: m.title });
    },
    newRfc: () => {
      const id = 'PROJ-' + Math.floor(160 + Math.random()*30);
      const r = { id, kind:'rfc', title:'New RFC — name me', status:'draft', specifier:'ejackson', target:'—', summary:'', age:'0d', sig: rand8() };
      setRfcs([r, ...rfcs]);
      pushEvent('work.created', id, 'kind=rfc');
      setActiveId(id); setActiveKind('rfc');
      persist('newRfc', { title: r.title });
    },
    newDecision: () => {
      const id = 'DB-' + Math.floor(95 + Math.random()*30);
      const d = { id, milestone:'PROJ-198', status:'open', opened: today(), idleDays:0, owner:'krivas', summary:'What is the impasse?' };
      setDecisions([d, ...decisions]);
      pushEvent('work.created', id, 'kind=decision_block');
      setActiveId(id); setActiveKind('decision');
      persist('newDecision', { title: d.summary });
    },
    newBinding: () => {
      const id = 'rb-' + (bindings.length + 1);
      const b = { id, milestone:'PROJ-204', role:'reviewer', actor: null };
      setBindings([...bindings, b]);
    },
    openAttention: (id, kind, tab) => {
      if (kind === 'milestone') { setView('portfolio'); }
      else if (kind === 'decision') { setView('decisions'); }
      else if (kind === 'outcome') { setView('outcomes'); }
      setActiveId(id);
      if (kind) setActiveKind(kind);
      setActiveTab(tab || null);
    },
    toggleGroup: (key) => {
      const next = new Set(collapsedGroups);
      next.has(key) ? next.delete(key) : next.add(key);
      setCollapsedGroups(next);
    },
    applyPreset: (presetVal) => {
      const p = window.PORTFOLIO_PRESETS.find(x => x.value === presetVal);
      if (p) setCollapsedGroups(new Set(p.collapsed));
    },
    spawnMilestone: (rfcId) => {
      const rfc = rfcs.find(r => r.id === rfcId);
      const id = 'PROJ-' + Math.floor(220 + Math.random()*40);
      const m = {
        id, kind:'milestone',
        title: rfc ? `${rfc.title} — v1` : 'New milestone — name me',
        status:'draft', ryg:'g',
        specifierAck:'pending', builderAck:'pending', ackHistory:[],
        specifier: rfc?.specifier || 'ejackson', builder:null, pilot:null,
        target:'2026 Q3', targetPrecision:'Q',
        orgNode:null, productNode:null,
        scopeChanges:0, depClosed:[0,0], fresh:'fresh', age:'0d', sig: rand8(),
        diagnostics:['pilot unassigned'],
        customerVisible:false,
        fromRfc: rfcId,
        stages:[], risks:[], nextSteps:[],
      };
      setMilestones([m, ...milestones]);
      pushEvent('work.created', id, `kind=milestone · from_rfc=${rfcId}`);
      setView('portfolio'); setActiveId(id); setActiveKind('milestone'); setActiveTab(null);
      persist('spawnMilestone', { title: m.title, rfcId });
    },
    addDep: (id, depId) => {
      setMilestones(ms => ms.map(m => m.id === id ? { ...m, deps: [...(m.deps || []), depId] } : m));
      pushEvent('work.linked', id, `depends_on=${depId}`);
      persist('addDep', { id, dep: depId });
    },
    removeDep: (id, depId) => {
      setMilestones(ms => ms.map(m => m.id === id ? { ...m, deps: (m.deps || []).filter(x => x !== depId) } : m));
      pushEvent('work.unlinked', id, `depends_on=${depId}`);
      persist('removeDep', { id, dep: depId });
    },
    addStatusUpdate: (id, narrative, by = 'ejackson') => {
      setMilestones(ms => ms.map(m => m.id === id ? {
        ...m,
        statusNarrative: narrative,
        statusUpdatedAt: today(),
        statusUpdatedBy: by,
      } : m));
      pushEvent('work.status_updated', id, 'narrative_length=' + narrative.length);
      persist('addStatusUpdate', { id, body: narrative });
    },
    addRisk: (id, body, severity = 'medium') => {
      setMilestones(ms => ms.map(m => m.id === id ? {
        ...m,
        risks: [{ body, severity, by:'ejackson', when: today() }, ...(m.risks || [])],
      } : m));
      pushEvent('work.risk_recorded', id, 'severity=' + severity);
      persist('addRisk', { id, body, severity, by: 'ejackson' });
    },
    addNextStep: (id, body, owner) => {
      setMilestones(ms => ms.map(m => m.id === id ? {
        ...m,
        nextSteps: [{ body, owner, when: today() }, ...(m.nextSteps || [])],
      } : m));
      pushEvent('work.next_recorded', id, 'owner=' + owner);
      persist('addNextStep', { id, body, owner });
    },
    commitAck: (id) => actions.acceptBoth(id),
    setAck: (id, who, action, note) => {
      const a = who === 'specifier' ? 'achen' : 'dnasser';
      setMilestones(ms => ms.map(m => {
        if (m.id !== id) return m;
        const key = who === 'specifier' ? 'specifierAck' : 'builderAck';
        const actor = who === 'specifier' ? m.specifier : m.builder;
        const history = [{ who, actor, action, when: today(), note: note || (action === 'accepted' ? 'Re-accepted.' : action === 'rejected' ? 'Rejected.' : 'Cleared.') }, ...(m.ackHistory || [])];
        return { ...m, [key]: action, ackHistory: history };
      }));
      pushEvent(action === 'accepted' ? 'work.ack_accepted' : action === 'rejected' ? 'work.ack_rejected' : 'work.ack_cleared',
        id, `who=${who}`, who === 'specifier' ? 'achen' : 'dnasser');
      persist('setAck', { id, who, ackAct: action, note });
    },
    updateAck: (id, patch) => {
      // Generic update for sheet edits on ACK columns; records history for each ACK field changed.
      setMilestones(ms => ms.map(m => {
        if (m.id !== id) return m;
        const next = { ...m };
        let history = m.ackHistory || [];
        if (patch.specifierAck && patch.specifierAck !== m.specifierAck) {
          history = [{ who:'specifier', actor: m.specifier, action: patch.specifierAck, when: today(), note: 'Updated from sheet.' }, ...history];
        }
        if (patch.builderAck && patch.builderAck !== m.builderAck) {
          history = [{ who:'builder', actor: m.builder, action: patch.builderAck, when: today(), note: 'Updated from sheet.' }, ...history];
        }
        Object.assign(next, patch, { ackHistory: history });
        return next;
      }));
      if (patch.specifierAck) { pushEvent('work.ack_' + patch.specifierAck, id, 'who=specifier'); persist('setAck', { id, who:'specifier', ackAct: patch.specifierAck }); }
      if (patch.builderAck)   { pushEvent('work.ack_' + patch.builderAck, id, 'who=builder'); persist('setAck', { id, who:'builder', ackAct: patch.builderAck }); }
      if (patch.specifier || patch.builder || patch.target) {
        const k = Object.keys(patch)[0];
        pushEvent('work.field_updated', id, `field=${k}`);
        persist('updateMilestone', { id, patch });
      }
    },
    acceptBoth: (id) => {
      setMilestones(ms => ms.map(m => {
        if (m.id !== id) return m;
        const hist = [
          { who:'builder', actor: m.builder,   action:'accepted', when: today(), note:'Quick-accept.' },
          { who:'specifier', actor: m.specifier, action:'accepted', when: today(), note:'Quick-accept.' },
          ...(m.ackHistory || [])
        ];
        return { ...m, specifierAck:'accepted', builderAck:'accepted', ackHistory: hist };
      }));
      pushEvent('work.ack_accepted', id, 'who=both');
      persist('acceptBoth', { id });
    },
    amendAck: (id) => {
      setMilestones(ms => ms.map(m => {
        if (m.id !== id) return m;
        const hist = [
          { who:'specifier', actor: m.specifier, action:'cleared', when: today(), note:'Auto-cleared by material amendment.' },
          { who:'builder',   actor: m.builder,   action:'cleared', when: today(), note:'Auto-cleared by material amendment.' },
          ...(m.ackHistory || [])
        ];
        return { ...m, specifierAck:'pending', builderAck:'pending', ackHistory: hist, scopeChanges: (m.scopeChanges || 0) + 1 };
      }));
      pushEvent('work.ack_amended', id, 'type=scope_change · auto-cleared both ACKs');
      persist('amendAck', { id, kind: 'scope_change' });
    },
  };

  const state = { milestones, rfcs, log, decisions, outcomes, bindings, events,
    activeId, selectedIds, pivot, groupBy, filters, collapsedGroups };

  /* ----------- View routing ----------- */
  const currentUser = 'ejackson';
  const VIEWS = {
    attention: {
      label:'For you', tag:'ATTENTION', kind:'home',
      h1: 'For you', sub: 'Everything the substrate thinks you should touch today. ACKs pending, targets passing, risks flagged, decisions idle. Inline actions — no detours.',
      body: <AttentionView state={state} actions={actions} currentUser={currentUser}/>,
    },
    leadership: {
      label:'Leadership', tag:'LEADERSHIP', kind:'home',
      h1: 'Leadership portfolio', sub: 'Four leading indicators on the substrate. Goal rollup, signal health, pattern blocks. No project list.',
      body: <Leadership state={state} actions={actions}/>,
    },
    portfolio: {
      label:'Portfolio', tag:'DATA', kind:'panel',
      h1: 'Portfolio', sub: 'Every cell is a signed event waiting to happen. Click to edit. Cmd+Enter opens the record panel. No modals.',
      body: <PortfolioView state={state} actions={actions}/>,
      toolbar: () => <PortfolioToolbar state={state} actions={actions} pivot={pivot} setPivot={setPivot} groupBy={groupBy} setGroupBy={setGroupBy} filters={filters} setFilters={setFilters} collapsedGroups={collapsedGroups}/>,
    },
    ack: {
      label:'ACK', tag:'ACK', kind:'check',
      h1: 'ACK', sub: 'Specifier ACK and Builder ACK — the only people who can stand behind a commitment. Click any cell. A = Accept · P = Pending · R = Reject.',
      body: <AckView state={state} actions={actions} currentUser={currentUser}/>,
    },
    rfcs: {
      label:'RFCs', tag:'GOVERNANCE', kind:'flag',
      h1: 'RFC review queue', sub: 'Specifiers propose. Leadership approves. Backlog and resourcing live here. Drag the status cell forward.',
      body: <RfcsView state={state} actions={actions}/>,
    },
    log: {
      label:"Pilot's Log", tag:'PILOT', kind:'compass',
      h1: "Pilot's Log", sub: 'Pilot’s journal. The top row is a quick-entry: pick a kind, type the note, Cmd+Enter files a signed event.',
      body: <PilotLogView state={state} actions={actions}/>,
    },
    decisions: {
      label:'Decisions', tag:'BLOCKED', kind:'scale',
      h1: 'DecisionBlocks', sub: 'What is the team stuck on? Idle ≥5d turns the chip yellow; ≥9d the scheduler escalates to Leadership.',
      body: <DecisionsView state={state} actions={actions}/>,
    },
    outcomes: {
      label:'Outcomes', tag:'EVAL', kind:'check',
      h1: 'Outcome assessments', sub: 'Did the milestone matter? Verdict + value. “Achieved · low” is the pattern that wakes Leadership.',
      body: <OutcomesView state={state} actions={actions}/>,
    },
    roles: {
      label:'Role bindings', tag:'SUBSTRATE', kind:'users',
      h1: 'Role bindings', sub: 'Specifier, Builder, Pilot, Reviewer, Leadership. Constraints flag — they don’t gate. Every change is an event.',
      body: <RolesView state={state} actions={actions}/>,
    },
    taxonomies: {
      label:'Taxonomies', tag:'SUBSTRATE', kind:'tag',
      h1: 'Taxonomies', sub: 'Three independent classification axes — org, product, and goals. Goals: Goal → node → result. Nodes live in meta, versioned with the project.',
      body: <TaxonomyView state={state} actions={actions}/>,
    },
    events: {
      label:'Event log', tag:'SUBSTRATE', kind:'book',
      h1: 'Event log', sub: 'The DAG’s ground truth. Every row is a signed event. Read-only here — mutate elsewhere, the log catches it.',
      body: <EventLogView state={state}/>,
    },
    roadmap: {
      label:'Customer roadmap', tag:'EXTERNAL', kind:'map',
      h1: 'Customer roadmap', sub: 'The only public surface. No RYG, no indicators, no internal commentary. Just committed scope and timing.',
      body: <RoadmapView state={state}/>,
    },
  };

  const v = VIEWS[view];
  const activeRecord =
    activeId == null ? null :
    activeKind === 'milestone' ? milestones.find(m => m.id === activeId) :
    activeKind === 'rfc' ? rfcs.find(r => r.id === activeId) :
    activeKind === 'decision' ? decisions.find(d => d.id === activeId) :
    activeKind === 'outcome' ? outcomes.find(o => o.id === activeId) :
    null;

  return (
    <div className="rx-app">
      <TopBar workspace={v.label} workspaceTag={v.tag} user="EJ" userColor="#1E3F60"/>

      <Sidebar sections={[
        { label:'Workspace', items: [
          navItem(VIEWS.attention,   view, setView, undefined),
          navItem(VIEWS.leadership,  view, setView, 4),
          navItem(VIEWS.portfolio,   view, setView, milestones.length),
          navItem(VIEWS.ack,         view, setView, milestones.filter(m => ackRollup(m.specifierAck, m.builderAck) !== 'aligned').length),
          navItem(VIEWS.rfcs,        view, setView, rfcs.filter(r => r.status === 'leadership_review').length || rfcs.length),
          navItem(VIEWS.log,         view, setView, log.length),
        ]},
        { label:'Methodology', items: [
          navItem(VIEWS.decisions,   view, setView, decisions.filter(d => d.status !== 'resolved').length),
          navItem(VIEWS.outcomes,    view, setView, outcomes.filter(o => o.status !== 'completed').length),
        ]},
        { label:'Substrate', items: [
          navItem(VIEWS.roles,       view, setView, bindings.filter(b => b.diagnostic).length),
          navItem(VIEWS.taxonomies,  view, setView),
          navItem(VIEWS.events,      view, setView, events.length),
        ]},
        { label:'External', items: [
          navItem(VIEWS.roadmap,     view, setView),
        ]},
      ]}/>

      <div className="rx-main">
        <div className="rx-main__header">
          <div className="rx-eyebrow">
            <span>DITS · SUBSTRATE</span><span className="sep">/</span>
            <span>RE · {v.tag}</span><span className="sep">/</span>
            <span className="em">{countersFor(view, state)}</span>
          </div>
          <h1 className="rx-main__h1">{v.h1}</h1>
          <div className="rx-main__sub">{v.sub}</div>
        </div>

        <div className="rx-main__body">
          {view === 'leadership' && <LeadershipKpis data={D2.INDICATORS}/>}

          {v.toolbar ? v.toolbar() : null}

          <BulkBar count={selectedIds.size} onClear={() => setSelectedIds(new Set())}>
            <Btn kind="ghost" size="sm" icon="user">Reassign</Btn>
            <Btn kind="ghost" size="sm" icon="calendar">Set quarter</Btn>
            <Btn kind="ghost" size="sm" icon="tag">Classify</Btn>
            <Btn kind="ghost" size="sm" icon="check">Accept ACK</Btn>
          </BulkBar>

          {v.body}
        </div>

        {activeRecord && (
          <RecordPanel record={activeRecord} kind={activeKind}
            initialTab={activeTab}
            onClose={actions.closeRecord} actions={actions} state={state}/>
        )}
      </div>

      <CmdPalette open={cmdOpen} onClose={() => setCmdOpen(false)} actions={actions} state={state}/>

      <div className="hintbar">
        <span><span className="kbd">⌘K</span>palette</span>
        <span><span className="kbd">Enter</span>edit cell</span>
        <span><span className="kbd">⌘↵</span>open detail</span>
        <span><span className="kbd">Tab</span>next</span>
        <span><span className="kbd">␣</span>select row</span>
        <span><span className="kbd">Esc</span>cancel / close</span>
      </div>
    </div>
  );
}

function navItem(v, current, setView, count) {
  return { icon: v.kind, label: v.label, count, active: current === v.label.toLowerCase().split(' ')[0] || matchView(current, v),
    onClick: () => setView(viewKeyOf(v)) };
}
function matchView(current, v) {
  return viewKeyOf(v) === current;
}
function viewKeyOf(v) {
  const m = { 'For you':'attention','Leadership':'leadership','Portfolio':'portfolio','ACK':'ack','RFCs':'rfcs',"Pilot's Log":'log',
    'Decisions':'decisions','Outcomes':'outcomes','Role bindings':'roles','Taxonomies':'taxonomies',
    'Event log':'events','Customer roadmap':'roadmap' };
  return m[v.label];
}

function PortfolioToolbar({ state, actions, pivot, setPivot, groupBy, setGroupBy, filters, setFilters, collapsedGroups }) {
  const currentPreset = window.presetFor ? window.presetFor(collapsedGroups) : 'custom';
  return (
    <div className="sht-toolbar">
      <span className="label-tag">View</span>
      <Pivot value={currentPreset} onChange={(v) => v !== 'custom' && actions.applyPreset(v)} options={[
        ...(window.PORTFOLIO_PRESETS || []),
        ...(currentPreset === 'custom' ? [{ value:'custom', label:'Custom' }] : []),
      ]}/>
      <span className="sep">·</span>
      <Pivot value={pivot} onChange={setPivot} options={[
        { value:'all',       label:'All' },
        { value:'mine',      label:'Mine' },
      ]}/>
      <span className="sep">·</span>
      <span className="label-tag">Group by</span>
      <Pivot value={groupBy} onChange={setGroupBy} options={[
        { value:'none',   label:'None' },
        { value:'status', label:'Status' },
        { value:'orgNode', label:'Team' },
        { value:'_productTop', label:'Product' },
        { value:'target', label:'Quarter' },
      ]}/>
      <span className="sep">·</span>
      <FilterChips filters={filters} setFilters={setFilters}/>
      <span className="sht-count">
        <em>{state.milestones.length}</em> total
      </span>
      <div className="actions">
        <Btn kind="accent" icon="plus" onClick={() => actions.newMilestone()}>New milestone</Btn>
      </div>
    </div>
  );
}

/* ================ Filter chips =================
filters: [{ field, op, value }]
Field definitions live in PORTFOLIO_FILTER_FIELDS — each provides options or an editor.
================================================ */
const PORTFOLIO_FILTER_FIELDS = [
  { field:'productTop', label:'Product', kind:'enum',
    options: () => {
      const roots = (D2.TAXONOMIES.product.nodes || []).filter(n => !n.parent);
      return [...roots.map(n => ({ value:n.slug, label:n.name })), { value:'_none', label:'Unclassified' }];
    } },
  { field:'orgNode', label:'Team', kind:'enum',
    options: () => {
      return (D2.TAXONOMIES.org.nodes || []).map(n => ({ value:n.slug, label:n.name }));
    } },
  { field:'specifier', label:'Specifier', kind:'actor' },
  { field:'builder',   label:'Builder',   kind:'actor' },
  { field:'pilot',     label:'Pilot',     kind:'actor' },
  { field:'status',    label:'Status',    kind:'enum',
    options: () => [
      {value:'draft',label:'Draft'},{value:'ack_filed',label:'ACK filed'},
      {value:'ack_committed',label:'ACK committed'},{value:'in_flight',label:'In flight'},
      {value:'shipped',label:'Shipped'},{value:'aborted',label:'Aborted'},
    ]},
  { field:'ryg',       label:'RYG',       kind:'enum',
    options: () => [{value:'g',label:'Green'},{value:'y',label:'Yellow'},{value:'r',label:'Red'}] },
  { field:'alignment', label:'Alignment', kind:'enum',
    options: () => [
      {value:'aligned',label:'Aligned'},{value:'both_pending',label:'Both pending'},
      {value:'specifier_pending',label:'Specifier pending'},{value:'builder_pending',label:'Builder pending'},
      {value:'rejected',label:'Rejected'},
    ]},
  { field:'target',    label:'Target',    kind:'enum',
    options: () => (window.TARGETS || ['2026 Q2','2026 Q3','2026 Q4','2027 Q1']).map(t => ({value:t,label:t})) },
  { field:'customerVisible', label:'Visibility', kind:'enum',
    options: () => [{value:true,label:'Public'},{value:false,label:'Internal'}] },
  { field:'_hasDiagnostics', label:'Diagnostics', kind:'enum',
    options: () => [{value:'any',label:'Any flagged'},{value:'violation',label:'Violations only'}] },
  { field:'_hasRisksHigh', label:'High-severity risk', kind:'bool' },
  { field:'_isStale',  label:'Stale (>14d)', kind:'bool' },
  { field:'_fromRfc',  label:'Origin RFC',   kind:'enum',
    options: () => {
      const ids = Array.from(new Set((window.DATA.MILESTONES || []).map(m => m.fromRfc).filter(Boolean)));
      return [{value:'_any',label:'Any RFC'}, {value:'_none',label:'No RFC origin'}, ...ids.map(id => ({value: id, label: id}))];
    }},
];

function fieldDef(field) { return PORTFOLIO_FILTER_FIELDS.find(f => f.field === field); }

function FilterChips({ filters, setFilters }) {
  const [addOpen, setAddOpen] = useState(false);
  const [pickField, setPickField] = useState(null);
  return (
    <span className="flt-row">
      {filters.map((f, i) => (
        <FilterChip key={i} f={f}
          onRemove={() => setFilters(filters.filter((_, j) => j !== i))}
          onChange={(nf) => setFilters(filters.map((x, j) => j === i ? nf : x))}/>
      ))}
      <span className="flt-add-wrap">
        <button className="flt-add" onClick={() => { setAddOpen(true); setPickField(null); }}>
          <I.plus size={11}/> Add filter
        </button>
        {addOpen && (
          <FilterAddPopover
            onClose={() => { setAddOpen(false); setPickField(null); }}
            pickField={pickField} setPickField={setPickField}
            onPick={(filter) => {
              setFilters([...filters, filter]);
              setAddOpen(false); setPickField(null);
            }}/>
        )}
      </span>
      {filters.length > 0 && (
        <span className="flt-clear" onClick={() => setFilters([])}>clear all</span>
      )}
    </span>
  );
}

function FilterChip({ f, onRemove, onChange }) {
  const def = fieldDef(f.field);
  const [editing, setEditing] = useState(false);
  if (!def) return null;
  const displayVal = renderFilterValue(def, f);
  return (
    <span className="flt-chip">
      <span className="flt-chip__lab">{def.label}</span>
      <span className="flt-chip__sep">{f.op || 'is'}</span>
      <span className="flt-chip__val" onClick={() => setEditing(true)}>{displayVal}</span>
      {editing && (
        <FilterValuePopover def={def} value={f.value}
          onCommit={v => { onChange({ ...f, value: v }); setEditing(false); }}
          onCancel={() => setEditing(false)}/>
      )}
      <span className="flt-chip__x" onClick={onRemove}>×</span>
    </span>
  );
}

function renderFilterValue(def, f) {
  if (def.kind === 'bool') return 'true';
  if (def.kind === 'actor') {
    const a = window.DATA.ACTORS[f.value];
    return a ? a.name : (f.value || '—');
  }
  if (def.kind === 'enum') {
    const opts = def.options();
    const opt = opts.find(o => String(o.value) === String(f.value));
    return opt ? opt.label : String(f.value);
  }
  return String(f.value);
}

function FilterAddPopover({ onClose, pickField, setPickField, onPick }) {
  // Step 1: choose field. Step 2: choose value.
  useEffect(() => {
    function onKey(e) { if (e.key === 'Escape') onClose(); }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);
  if (!pickField) {
    return (
      <span className="flt-pop" onMouseLeave={onClose}>
        <span className="flt-pop__head">Filter by…</span>
        {PORTFOLIO_FILTER_FIELDS.map(def => (
          <span key={def.field} className="flt-pop__opt"
            onClick={() => def.kind === 'bool' ? onPick({field: def.field, op:'is', value:true}) : setPickField(def.field)}>
            {def.label}
          </span>
        ))}
      </span>
    );
  }
  const def = fieldDef(pickField);
  return (
    <span className="flt-pop">
      <span className="flt-pop__head">
        <span className="flt-pop__back" onClick={() => setPickField(null)}>←</span>
        {def.label}
      </span>
      <FilterValueList def={def}
        onPick={(v) => onPick({ field: def.field, op:'is', value: v })}/>
    </span>
  );
}

function FilterValuePopover({ def, value, onCommit, onCancel }) {
  useEffect(() => {
    function onKey(e) { if (e.key === 'Escape') onCancel(); }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onCancel]);
  return (
    <span className="flt-pop flt-pop--val">
      <FilterValueList def={def} onPick={onCommit}/>
    </span>
  );
}

function FilterValueList({ def, onPick }) {
  if (def.kind === 'actor') {
    const actors = Object.values(window.DATA.ACTORS);
    return (
      <>
        {actors.map(a => (
          <span key={a.id} className="flt-pop__opt flt-pop__opt--actor" onClick={() => onPick(a.id)}>
            <span className="dx-av" style={{background:a.color, width:18, height:18, fontSize:8.5}}>{a.initials}</span>
            {a.name}
          </span>
        ))}
      </>
    );
  }
  if (def.kind === 'enum') {
    return (
      <>
        {def.options().map(o => (
          <span key={String(o.value)} className="flt-pop__opt" onClick={() => onPick(o.value)}>
            {o.label}
          </span>
        ))}
      </>
    );
  }
  return null;
}

function countersFor(view, state) {
  if (view === 'attention') return `Today · ${state.milestones.filter(m => m.specifier === 'ejackson' || m.builder === 'ejackson').length} on you`;
  if (view === 'portfolio') return `${state.milestones.length} work items · ${state.selectedIds.size || 'none'} selected`;
  if (view === 'rfcs')      return `${state.rfcs.length} RFCs · ${state.rfcs.filter(r => r.status === 'leadership_review').length} in review`;
  if (view === 'log')       return `${state.log.length} entries · last filed 3d ago`;
  if (view === 'decisions') return `${state.decisions.filter(d => d.status === 'open').length} open · ${state.decisions.filter(d => d.status === 'escalated').length} escalated`;
  if (view === 'outcomes')  return `${state.outcomes.length} assessments · ${state.outcomes.filter(o => o.status === 'overdue').length} overdue`;
  if (view === 'roles')     return `${state.bindings.length} bindings · ${state.bindings.filter(b => b.diagnostic).length} diagnostics`;
  if (view === 'taxonomies')return `3 axes · ${D2.TAXONOMIES.org.nodes.length + D2.TAXONOMIES.product.nodes.length + D2.TAXONOMIES.goals.nodes.length} nodes`;
  if (view === 'events')    return `${state.events.length} recent events · synced 14s ago`;
  if (view === 'roadmap')   return 'public projection · no auth';
  if (view === 'leadership')return `${state.milestones.filter(m => m.status === 'in_flight').length} in flight · ${state.milestones.filter(m => m.ryg === 'r').length} red`;
  return '';
}

/* ============== Leadership view body (small portfolio) ============== */
function Leadership({ state, actions }) {
  const adjudication = state.decisions.filter(d => d.status === 'escalated' || d.idleDays >= 9);
  return (
    <>
      <div className="rx-grid-main-side" style={{gap:20, marginTop:0}}>
        <div>
          <Panel title="In flight" meta={`${state.milestones.filter(m => m.status === 'in_flight').length} milestones`} flush>
            <table className="rx-table">
              <thead><tr><th>ID</th><th>Title</th><th>RYG</th><th>Alignment</th><th>Pilot</th><th>Target</th><th style={{textAlign:'right'}}>Indicators</th></tr></thead>
              <tbody>
                {state.milestones.filter(m => m.status === 'in_flight').map(m => (
                  <tr key={m.id} onClick={() => actions.openRecord(m.id, 'milestone')} style={{cursor:'pointer'}}>
                    <td className="id">{m.id}</td>
                    <td style={{fontWeight:500, color:'var(--ink-0)'}}>{m.title}</td>
                    <td>{renderRyg(m.ryg)}</td>
                    <td>{renderRollup(null, {row: m})}</td>
                    <td>{renderActor(m.pilot, {compact:true})}</td>
                    <td className="rx-mono" style={{fontSize:11}}>{m.target}</td>
                    <td style={{textAlign:'right'}}>{m.indicators?.depRate != null ? (m.indicators.depRate*100).toFixed(0)+'% deps' : '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Panel>

          <div style={{marginTop:16}}>
            <Panel title="Pattern blocks" meta="3 active" flush>
              <div style={{padding:'12px 14px', borderBottom:'1px solid var(--hairline)'}}>
                <div style={{display:'flex', alignItems:'center', gap:8, marginBottom:5}}>
                  <span className="dx-pill dx-pill--y">scope-velocity</span>
                  <span style={{fontFamily:'var(--font-mono)', fontSize:10.5, color:'var(--ink-3)'}}>3 milestones · last 14d</span>
                </div>
                <div style={{fontFamily:'var(--font-serif)', fontStyle:'italic', fontSize:13.5, color:'var(--ink-2)', lineHeight:1.55}}>
                  PROJ-198, PROJ-141, PROJ-176 are amending scope at &gt;0.5/wk against pre-commit baseline. Worth a Specifier sync.
                </div>
              </div>
              <div style={{padding:'12px 14px', borderBottom:'1px solid var(--hairline)'}}>
                <div style={{display:'flex', alignItems:'center', gap:8, marginBottom:5}}>
                  <span className="dx-pill dx-pill--plum">landed-low-value</span>
                  <span style={{fontFamily:'var(--font-mono)', fontSize:10.5, color:'var(--ink-3)'}}>OA-028 · M. Hong</span>
                </div>
                <div style={{fontFamily:'var(--font-serif)', fontStyle:'italic', fontSize:13.5, color:'var(--ink-2)', lineHeight:1.55}}>
                  Shipped on time. Adoption flat. The team should retro this — not a failure of execution, a failure of bet.
                </div>
              </div>
              <div style={{padding:'12px 14px'}}>
                <div style={{display:'flex', alignItems:'center', gap:8, marginBottom:5}}>
                  <span className="dx-pill dx-pill--r">pilot-collapse</span>
                  <span style={{fontFamily:'var(--font-mono)', fontSize:10.5, color:'var(--ink-3)'}}>PROJ-176</span>
                </div>
                <div style={{fontFamily:'var(--font-serif)', fontStyle:'italic', fontSize:13.5, color:'var(--ink-2)', lineHeight:1.55}}>
                  Pilot reports to Builder within 1 level on the org taxonomy. Diagnostic is firing; nothing is blocked. Re-bind.
                </div>
              </div>
            </Panel>
          </div>
        </div>

        <div className="rx-stack" style={{gap:16}}>
          <Panel title="Adjudication queue" meta={`${adjudication.length} pending`} flush>
            {adjudication.length === 0 && (
              <div style={{padding:'14px', fontFamily:'var(--font-serif)', fontStyle:'italic', color:'var(--ink-3)', fontSize:13}}>
                Nothing on you today. Watch for silence — that’s usually data.
              </div>
            )}
            {adjudication.map(d => (
              <div key={d.id} style={{padding:'10px 14px', borderBottom:'1px solid var(--hairline)', cursor:'pointer'}}
                onClick={() => { actions.setView('decisions'); actions.openRecord(d.id, 'decision'); }}>
                <div style={{display:'flex', alignItems:'center', gap:8, marginBottom:3}}>
                  <span className="dx-id" style={{color:'var(--accent)'}}>{d.id}</span>
                  <span style={{fontFamily:'var(--font-mono)', fontSize:10.5, color:'var(--ink-3)'}}>{d.milestone}</span>
                  <span style={{marginLeft:'auto'}}><span className={'dx-pill dx-pill--' + (d.idleDays >= 9 ? 'r' : 'y')}>{d.idleDays}d idle</span></span>
                </div>
                <div style={{fontSize:12.5, color:'var(--ink-1)', lineHeight:1.4}}>{d.summary}</div>
              </div>
            ))}
          </Panel>

          <Panel title="Goal health" meta="rolled up" flush>
            <div style={{padding:'12px 14px'}}>
              {['Customer trust','Margin','Substrate maturity'].map((g, i) => (
                <div key={g} style={{display:'flex', alignItems:'center', padding:'7px 0', borderBottom: i < 2 ? '1px solid var(--hairline)' : 'none'}}>
                  <span style={{fontFamily:'var(--font-display)', fontSize:14, fontWeight:500, color:'var(--ink-0)'}}>{g}</span>
                  <span style={{marginLeft:'auto'}}>{renderRyg(['g','y','g'][i])}</span>
                </div>
              ))}
            </div>
          </Panel>
        </div>
      </div>
    </>
  );
}

/* ============== helpers ============== */
function rand8() { return Math.random().toString(16).slice(2, 10); }
function today() { return new Date().toISOString().slice(0,10); }

function computeBindingDiagnostic(b, all, milestones) {
  if (!b.actor) return 'unassigned';
  // Pilot-on-self check
  const sib = all.filter(x => x.milestone === b.milestone && x.id !== b.id);
  if (b.role === 'pilot' && sib.some(s => (s.role === 'builder' || s.role === 'specifier') && s.actor === b.actor)) {
    return 'role collapse · pilot shares actor with ' + (sib.find(s => s.actor === b.actor)?.role);
  }
  return null;
}

ReactDOM.createRoot(document.getElementById('root')).render(<App/>);
