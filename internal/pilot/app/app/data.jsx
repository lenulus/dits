/* Seed data — exposed as window.DATA. Mutable in place (cheap optimistic updates). */

const ACTORS = {
  ejackson:  { id: 'ejackson',  initials: 'EJ', name: 'E. Jackson', color: '#1E3F60', role: 'Specifier' },
  achen:     { id: 'achen',     initials: 'AC', name: 'A. Chen',    color: '#5C1E50', role: 'Specifier' },
  mhong:     { id: 'mhong',     initials: 'MH', name: 'M. Hong',    color: '#C56A1A', role: 'Specifier' },
  krivas:    { id: 'krivas',    initials: 'CR', name: 'C. Rivas',   color: '#2F7A3D', role: 'Pilot' },
  kokafor:   { id: 'kokafor',   initials: 'KO', name: 'K. Okafor',  color: '#1E3F60', role: 'Pilot' },
  bliang:    { id: 'bliang',    initials: 'BL', name: 'B. Liang',   color: '#5C1E50', role: 'Pilot' },
  dnasser:   { id: 'dnasser',   initials: 'DN', name: 'D. Nasser',  color: '#3F424D', role: 'Builder' },
  tpark:     { id: 'tpark',     initials: 'TP', name: 'T. Park',    color: '#3F424D', role: 'Builder' },
  jwhite:    { id: 'jwhite',    initials: 'JW', name: 'J. White',   color: '#1F4FB8', role: 'Leadership' },
  buildbot:  { id: 'buildbot',  initials: 'BB', name: 'buildbot.agent', color: '#888B94', role: 'agent', agent: true },
};

const TAXONOMIES = {
  org: {
    slug: 'org', name: 'Organization', levels: ['org','node','team'],
    nodes: [
      { slug: 'acme', name: 'Acme', parent: null },
      { slug: 'acme/platform',        name: 'Platform',   parent: 'acme' },
      { slug: 'acme/platform/payments-team',  name: 'Payments Team',  parent: 'acme/platform' },
      { slug: 'acme/platform/identity-team',  name: 'Identity Team',  parent: 'acme/platform' },
      { slug: 'acme/platform/infra-team',     name: 'Infra Team',     parent: 'acme/platform' },
      { slug: 'acme/product',         name: 'Product',    parent: 'acme' },
      { slug: 'acme/product/mobile-team',     name: 'Mobile Team',    parent: 'acme/product' },
      { slug: 'acme/product/web-team',        name: 'Web Team',       parent: 'acme/product' },
      { slug: 'acme/compliance',      name: 'Compliance', parent: 'acme' },
      { slug: 'acme/compliance/audit-team',   name: 'Audit Team',     parent: 'acme/compliance' },
    ],
  },
  goals: {
    slug: 'goals', name: 'Goals', levels: ['goal','node','result'],
    nodes: [
      { slug: 'customer-trust', name: 'Customer trust', parent: null },
      { slug: 'customer-trust/p0-incidents',           name: 'P0 incidents \u2264 2/qtr', parent: 'customer-trust' },
      { slug: 'customer-trust/p0-incidents/q2-2026',   name: 'Q2 2026',                  parent: 'customer-trust/p0-incidents', result:'achieved', resultNote:'1 incident' },
      { slug: 'customer-trust/p0-incidents/q3-2026',   name: 'Q3 2026',                  parent: 'customer-trust/p0-incidents', result:'in_progress', resultNote:'0 so far' },
      { slug: 'customer-trust/audit-coverage',         name: '100% audit-log coverage',  parent: 'customer-trust' },
      { slug: 'customer-trust/audit-coverage/q2-2026', name: 'Q2 2026',                  parent: 'customer-trust/audit-coverage', result:'missed',   resultNote:'87%' },
      { slug: 'customer-trust/audit-coverage/q3-2026', name: 'Q3 2026',                  parent: 'customer-trust/audit-coverage', result:'in_progress', resultNote:'94%' },
      { slug: 'margin', name: 'Margin', parent: null },
      { slug: 'margin/cost-per-tx',                    name: 'Cost / transaction \u221220%', parent: 'margin' },
      { slug: 'margin/cost-per-tx/q2-2026',            name: 'Q2 2026',                  parent: 'margin/cost-per-tx', result:'partial', resultNote:'\u221212%' },
      { slug: 'substrate-maturity', name: 'Substrate maturity', parent: null },
      { slug: 'substrate-maturity/event-emission',     name: 'All state via signed events', parent: 'substrate-maturity' },
      { slug: 'substrate-maturity/event-emission/q2-2026', name: 'Q2 2026',              parent: 'substrate-maturity/event-emission', result:'achieved', resultNote:'all paths' },
      { slug: 'substrate-maturity/constraint-coverage',name: 'Role-constraint diagnostics', parent: 'substrate-maturity' },
      { slug: 'substrate-maturity/constraint-coverage/q3-2026', name: 'Q3 2026',         parent: 'substrate-maturity/constraint-coverage', result:'in_progress', resultNote:'two predicates live' },
    ],
  },
  product: {
    slug: 'product', name: 'Product', levels: ['product_group','product','node','features'],
    nodes: [
      { slug: 'commerce', name: 'Commerce', parent: null },
      { slug: 'commerce/checkout',  name: 'Checkout',  parent: 'commerce' },
      { slug: 'commerce/checkout/payments',  name: 'Payments',  parent: 'commerce/checkout' },
      { slug: 'commerce/checkout/payments/recurring-billing', name: 'Recurring billing', parent: 'commerce/checkout/payments' },
      { slug: 'commerce/checkout/payments/webhooks',          name: 'Webhooks',          parent: 'commerce/checkout/payments' },
      { slug: 'commerce/checkout/payments/idempotency',       name: 'Idempotency',       parent: 'commerce/checkout/payments' },
      { slug: 'platform', name: 'Platform', parent: null },
      { slug: 'platform/identity',  name: 'Identity', parent: 'platform' },
      { slug: 'platform/identity/key-rotation',  name: 'Key rotation', parent: 'platform/identity' },
      { slug: 'platform/observability',          name: 'Observability', parent: 'platform' },
      { slug: 'platform/observability/audit-log',name: 'Audit log',     parent: 'platform/observability' },
    ],
  },
};

const ROLES = [
  { slug: 'specifier',   name: 'Specifier',   cardinality: 'exactly_one' },
  { slug: 'builder',     name: 'Builder',     cardinality: 'exactly_one' },
  { slug: 'pilot',       name: 'Pilot',       cardinality: 'exactly_one' },
  { slug: 'reviewer',    name: 'Reviewer',    cardinality: 'many' },
  { slug: 'leadership',  name: 'Leadership',  cardinality: 'many' },
];

const ROLE_CONSTRAINTS = [
  { slug: 'no_role_collapse', name: 'No role collapse',
    predicate: 'distinct_actors',
    args: { roles: ['specifier','builder','pilot'] },
    severity: 'violation',
    description: 'Specifier, Builder, and Pilot must be three distinct actors on a single milestone.' },
  { slug: 'pilot_independence', name: 'Pilot independence',
    predicate: 'not_reports_to_within',
    args: { role: 'pilot', other_roles: ['specifier','builder'], taxonomy: 'org', max_levels: 2 },
    severity: 'warning',
    description: 'The Pilot must not report to the Specifier or Builder within 2 levels of the org taxonomy.' },
  { slug: 'requires_product_classification', name: 'Product classification required',
    predicate: 'requires_classification',
    args: { taxonomy: 'product' },
    severity: 'warning',
    description: 'Every milestone must be classified within the product taxonomy.' },
];

/* --------- Milestones + RFCs (same kind universe) --------- */
const MILESTONES = [
  { id:'PROJ-204', kind:'milestone', title:'Recurring billing v1',
    status:'in_flight', ryg:'g',
    specifierAck:'accepted', builderAck:'accepted',
    ackHistory:[
      {who:'specifier', actor:'achen',  action:'accepted', when:'2026-05-21', note:'Re-accepted after clarification amendment.'},
      {who:'builder',   actor:'dnasser',action:'accepted', when:'2026-05-08', note:'Scope is implementable inside the quarter.'},
      {who:'specifier', actor:'achen',  action:'accepted', when:'2026-05-05', note:'Initial ACK.'},
    ],
    target:'2026 Q3', customerVisible:true,
    specifier:'achen', builder:'dnasser', pilot:'krivas',
    orgNode:'acme/platform/payments-team',
    productNode:'commerce/checkout/payments/recurring-billing',
    scopeChanges:0, depClosed:[7,9], latency:'4d',
    fresh:'fresh', age:'3d', sig:'a1b2c3d4',
    indicators:{depRate:0.88, scopeVel:0.0, decFric:'2.1d', ackLat:'4d'},
    diagnostics:[] },
  { id:'PROJ-198', kind:'milestone', title:'Webhook retry policy',
    status:'in_flight', ryg:'y',
    specifierAck:'accepted', builderAck:'pending',
    ackHistory:[
      {who:'builder',   actor:'tpark',   action:'cleared', when:'2026-05-16', note:'Auto-cleared after scope amendment (retry-storm hardening).'},
      {who:'specifier', actor:'ejackson',action:'accepted', when:'2026-05-18', note:'Re-accepted after retry-storm scope add.'},
      {who:'builder',   actor:'tpark',   action:'accepted', when:'2026-05-02', note:'OK on Q3 timing.'},
      {who:'specifier', actor:'ejackson',action:'accepted', when:'2026-05-02', note:'Initial ACK.'},
    ],
    target:'2026 Q3', customerVisible:true,
    specifier:'ejackson', builder:'tpark', pilot:'kokafor',
    orgNode:'acme/platform/payments-team',
    productNode:'commerce/checkout/payments/webhooks',
    scopeChanges:2, depClosed:[4,9], latency:'6d',
    fresh:'aging', age:'11d', sig:'d8f1a3c2',
    indicators:{depRate:0.44, scopeVel:0.5, decFric:'3.8d', ackLat:'6d'},
    diagnostics:['scope-velocity > baseline'] },
  { id:'PROJ-187', kind:'milestone', title:'Customer audit log export',
    status:'ack_committed', ryg:'g',
    specifierAck:'accepted', builderAck:'accepted',
    ackHistory:[
      {who:'builder',   actor:'dnasser', action:'accepted', when:'2026-05-22', note:'Confirms Q3 with compliance buffer.'},
      {who:'specifier', actor:'mhong',   action:'accepted', when:'2026-05-20', note:'Initial ACK.'},
    ],
    target:'2026 Q3', customerVisible:true,
    specifier:'mhong', builder:'dnasser', pilot:'krivas',
    orgNode:'acme/compliance/audit-team',
    productNode:'platform/observability/audit-log',
    scopeChanges:0, depClosed:[0,3], latency:null,
    fresh:'fresh', age:'1d', sig:'7e0f44b1',
    indicators:{depRate:0.00, scopeVel:0.0, decFric:'1.2d', ackLat:'—'},
    diagnostics:[] },
  { id:'PROJ-176', kind:'milestone', title:'Pilot independence enforcement',
    status:'in_flight', ryg:'r',
    specifierAck:'rejected', builderAck:'pending',
    ackHistory:[
      {who:'specifier', actor:'ejackson',action:'rejected', when:'2026-05-16', note:'Target shifted twice; need re-spec before I re-ACK.'},
      {who:'builder',   actor:'tpark',   action:'cleared',  when:'2026-05-16', note:'Auto-cleared by material amendment.'},
      {who:'builder',   actor:'tpark',   action:'accepted', when:'2026-04-23', note:'Scope is implementable.'},
      {who:'specifier', actor:'ejackson',action:'accepted', when:'2026-04-22', note:'Initial ACK.'},
    ],
    target:'2026 Q4', customerVisible:false,
    specifier:'ejackson', builder:'tpark', pilot:'kokafor',
    orgNode:'acme/platform/infra-team',
    productNode:null,
    scopeChanges:4, depClosed:[2,11], latency:'14d',
    fresh:'stale', age:'41d', sig:'3c9e0a17',
    indicators:{depRate:0.18, scopeVel:1.0, decFric:'9.4d', ackLat:'14d'},
    diagnostics:['pilot reports to builder','no product classification','scope-velocity high'] },
  { id:'PROJ-163', kind:'milestone', title:'Mobile API parity',
    status:'shipped', ryg:'g',
    specifierAck:'accepted', builderAck:'accepted',
    ackHistory:[
      {who:'builder',   actor:'tpark', action:'accepted', when:'2026-04-04', note:'Locked in for Q2.'},
      {who:'specifier', actor:'achen', action:'accepted', when:'2026-04-02', note:'Initial ACK.'},
    ],
    target:'2026 Q2', customerVisible:true,
    specifier:'achen', builder:'tpark', pilot:'bliang',
    orgNode:'acme/product/mobile-team',
    productNode:'commerce/checkout/payments',
    scopeChanges:1, depClosed:[12,12], latency:'3d',
    fresh:'fresh', age:'2d', sig:'f12a4b9e',
    indicators:{depRate:1.0, scopeVel:0.25, decFric:'1.9d', ackLat:'3d'},
    diagnostics:[] },
  { id:'PROJ-141', kind:'milestone', title:'Idempotency keys v2',
    status:'in_flight', ryg:'y',
    specifierAck:'pending', builderAck:'accepted',
    ackHistory:[
      {who:'builder',   actor:'dnasser',action:'accepted', when:'2026-05-15', note:'Migration path is implementable.'},
      {who:'specifier', actor:'achen',  action:'cleared',  when:'2026-05-14', note:'Auto-cleared after scope amendment.'},
      {who:'specifier', actor:'achen',  action:'accepted', when:'2026-05-02', note:'Initial ACK.'},
    ],
    target:'2026 Q3', customerVisible:false,
    specifier:'achen', builder:'dnasser', pilot:'krivas',
    orgNode:'acme/platform/payments-team',
    productNode:'commerce/checkout/payments/idempotency',
    scopeChanges:3, depClosed:[5,8], latency:'7d',
    fresh:'aging', age:'18d', sig:'2d6a1f93',
    indicators:{depRate:0.62, scopeVel:0.75, decFric:'4.1d', ackLat:'7d'},
    diagnostics:['scope-velocity > baseline'] },
  { id:'PROJ-128', kind:'milestone', title:'Outcome assessment scheduler',
    status:'ack_filed', ryg:'g',
    specifierAck:'accepted', builderAck:'pending',
    ackHistory:[
      {who:'specifier', actor:'kokafor', action:'accepted', when:'2026-05-22', note:'Initial ACK.'},
    ],
    target:'2026 Q3', customerVisible:false,
    specifier:'kokafor', builder:'tpark', pilot:'bliang',
    orgNode:'acme/platform/infra-team',
    productNode:'platform/observability',
    scopeChanges:0, depClosed:[0,4], latency:null,
    fresh:'fresh', age:'2d', sig:'b4e7c102',
    indicators:{depRate:0.0, scopeVel:0.0, decFric:'—', ackLat:'—'},
    diagnostics:[] },
  { id:'PROJ-119', kind:'milestone', title:'Taxonomy admin UI',
    status:'draft', ryg:'g',
    specifierAck:'pending', builderAck:'pending',
    ackHistory:[],
    target:'2026 Q4', customerVisible:false,
    specifier:'ejackson', builder:'dnasser', pilot:null,
    orgNode:'acme/platform/infra-team',
    productNode:null,
    scopeChanges:'—', depClosed:'—', latency:null,
    fresh:'fresh', age:'5d', sig:'9a02fe55',
    indicators:{}, diagnostics:['pilot unassigned','no product classification'] },
  { id:'PROJ-112', kind:'milestone', title:'Role-constraint diagnostic engine',
    status:'in_flight', ryg:'g',
    specifierAck:'accepted', builderAck:'accepted',
    ackHistory:[
      {who:'builder',   actor:'dnasser', action:'accepted', when:'2026-05-04', note:'Constraint API is a clean lift.'},
      {who:'specifier', actor:'ejackson',action:'accepted', when:'2026-05-03', note:'Initial ACK.'},
    ],
    target:'2026 Q3', customerVisible:false,
    specifier:'ejackson', builder:'dnasser', pilot:'bliang',
    orgNode:'acme/platform/infra-team',
    productNode:'platform/identity',
    scopeChanges:0, depClosed:[3,5], latency:'2d',
    fresh:'fresh', age:'4d', sig:'5be91e44',
    indicators:{depRate:0.6, scopeVel:0.0, decFric:'2.0d', ackLat:'2d'},
    diagnostics:[] },
  { id:'PROJ-103', kind:'milestone', title:'Custodial key signing flow',
    status:'in_flight', ryg:'y',
    specifierAck:'accepted', builderAck:'accepted',
    ackHistory:[
      {who:'builder',   actor:'tpark', action:'accepted', when:'2026-05-01', note:'OK on KMS abstraction.'},
      {who:'specifier', actor:'mhong', action:'accepted', when:'2026-04-30', note:'Initial ACK.'},
    ],
    target:'2026 Q3', customerVisible:false,
    specifier:'mhong', builder:'tpark', pilot:'krivas',
    orgNode:'acme/platform/identity-team',
    productNode:'platform/identity/key-rotation',
    scopeChanges:1, depClosed:[2,4], latency:'8d',
    fresh:'aging', age:'14d', sig:'c3d8e201',
    indicators:{depRate:0.5, scopeVel:0.25, decFric:'5.2d', ackLat:'8d'},
    diagnostics:[] },
];

const RFCS = [
  { id:'PROJ-152', kind:'rfc', title:'Multi-tenant key rotation',
    status:'leadership_review', target:'2026 Q4', specifier:'mhong', age:'9d', sig:'8b27c5e0',
    approvedAt:null,
    summary:'Per-tenant Ed25519 key rotation cadence and revocation flow. Affects identity and audit-log.' },
  { id:'PROJ-149', kind:'rfc', title:'Public roadmap projection',
    status:'approved', target:'2026 Q3', specifier:'ejackson', age:'14d', sig:'4f2c80a1',
    approvedAt:'2026-04-22',
    summary:'Customer-facing read-only view over committed ACKs. No auth, no RYG.' },
  { id:'PROJ-145', kind:'rfc', title:'Constraint-predicate WASM modules',
    status:'approved', target:'2027 Q1', specifier:'ejackson', age:'30d', sig:'00b1f3aa',
    approvedAt:'2026-04-26',
    summary:'Allow methodology packs to ship their own constraint predicates as WASM modules.' },
  { id:'PROJ-138', kind:'rfc', title:'Outcome-assessment SLA',
    status:'resourced', target:'2026 Q3', specifier:'kokafor', age:'21d', sig:'7710bc44',
    approvedAt:'2026-04-30',
    summary:'Default 90d SLA on outcome assessments; per-team override via meta.' },
  { id:'PROJ-130', kind:'rfc', title:'Streaming via WebSocket',
    status:'rejected', target:'—', specifier:'tpark', age:'58d', sig:'aa00ee01',
    summary:'Defer real-time push; polling against v2 query API is sufficient for v1.' },
  { id:'PROJ-160', kind:'rfc', title:'Auto-escalation thresholds per team',
    status:'draft', target:'2026 Q4', specifier:'krivas', age:'2d', sig:'21ab09c0',
    summary:'Allow teams to override the default 5-day DecisionBlock escalation window.' },
];

/* --------- Pilot's Log entries --------- */
const PILOT_LOG = [
  { num:247, milestone:'PROJ-198', kind:'integrity_call', by:'krivas', time:'2026-05-19 14:22',
    body:'Integrity: drifting. Scope-change velocity is up 0.3/mo against pre-commit baseline. Recommend Specifier review before Q3 freeze.', integrity:'drifting' },
  { num:245, milestone:'PROJ-198', kind:'risk', by:'krivas', time:'2026-05-17 09:08',
    body:'Upstream dep on PROJ-176 hasn\u2019t closed; downstream consumers cannot integration-test. Two weeks runway.' },
  { num:241, milestone:'PROJ-198', kind:'stress_test', by:'krivas', time:'2026-05-15 11:30',
    body:'Asked the team how this fails. Three independent answers, all about retry storms. Convergent risk \u2014 worth a hardening cycle.' },
  { num:240, milestone:'PROJ-176', kind:'integrity_call', by:'kokafor', time:'2026-05-15 10:02',
    body:'Integrity: invalid. ACK target shifted twice with no recommit. Pilot stands down on RYG until commitment is reformed.', integrity:'invalid' },
  { num:238, milestone:'PROJ-198', kind:'observation', by:'krivas', time:'2026-05-12 16:45',
    body:'Engineering tempo looks healthy: 4 PRs/day median across the squad. No silence patterns.' },
  { num:236, milestone:'PROJ-141', kind:'risk', by:'krivas', time:'2026-05-11 13:11',
    body:'Two consecutive scope changes within one week. Worth calling drift even if RYG is still green.' },
  { num:233, milestone:'PROJ-204', kind:'observation', by:'krivas', time:'2026-05-09 08:30',
    body:'Builders are calling deps proactively. Dependency closure rate now at 78% \u2014 a healthy two weeks out from target.' },
  { num:231, milestone:'PROJ-176', kind:'decision', by:'kokafor', time:'2026-05-08 17:20',
    body:'Escalating to Leadership. The team has held the same DecisionBlock open for nine days; that\u2019s past the threshold.' },
  { num:229, milestone:'PROJ-187', kind:'observation', by:'krivas', time:'2026-05-07 11:00',
    body:'Compliance is the bottleneck here, not engineering. Watching for cross-team handoff latency.' },
  { num:225, milestone:'PROJ-112', kind:'stress_test', by:'bliang', time:'2026-05-04 15:40',
    body:'Posed: what happens when both Specifier and Builder are unavailable? Diagnostic engine still flags constraints; nothing is blocked. Good.' },
];

/* --------- DecisionBlocks --------- */
const DECISIONS = [
  { id:'DB-091', milestone:'PROJ-176', status:'escalated', opened:'2026-05-04', idleDays:11, owner:'kokafor', summary:'Whether to abort or re-spec given two consecutive target shifts.' },
  { id:'DB-088', milestone:'PROJ-198', status:'open',      opened:'2026-05-12', idleDays:6,  owner:'krivas',  summary:'Pick retry-storm hardening approach: token bucket vs adaptive backoff.' },
  { id:'DB-086', milestone:'PROJ-141', status:'open',      opened:'2026-05-13', idleDays:5,  owner:'krivas',  summary:'Drop idempotency-key Q4 backport, or hold for parity?' },
  { id:'DB-082', milestone:'PROJ-204', status:'resolved',  opened:'2026-05-02', idleDays:0,  owner:'krivas',  summary:'Whether to launch with trial-period support in v1. Resolved: yes.' },
  { id:'DB-077', milestone:'PROJ-103', status:'open',      opened:'2026-05-08', idleDays:10, owner:'krivas',  summary:'Which KMS to standardize on \u2014 single provider vs portable wrapper.' },
  { id:'DB-072', milestone:'PROJ-112', status:'resolved',  opened:'2026-04-22', idleDays:0,  owner:'bliang',  summary:'Predicate language for v1 \u2014 hard-coded Go vs Starlark. Resolved: hard-coded.' },
];

/* --------- Outcome assessments --------- */
const OUTCOMES = [
  { id:'OA-031', milestone:'PROJ-163', status:'completed', due:'2026-08-15', result:'achieved', value:'high',  by:'achen',   note:'Mobile-web parity drove a 14% lift in mobile checkouts. Customers noticed.' },
  { id:'OA-028', milestone:'PROJ-155', status:'completed', due:'2026-07-09', result:'achieved', value:'low',   by:'mhong',   note:'Shipped on time. Adoption was flat. Pattern: landed but didn\u2019t matter.' },
  { id:'OA-025', milestone:'PROJ-149', status:'pending',   due:'2026-09-01', result:'—',        value:'—',     by:'ejackson',note:'' },
  { id:'OA-022', milestone:'PROJ-142', status:'overdue',   due:'2026-04-30', result:'—',        value:'—',     by:'mhong',   note:'Specifier unavailable; needs reassignment.' },
];

/* --------- Role bindings (active) --------- */
const ROLE_BINDINGS = [
  { id:'rb-1',  milestone:'PROJ-176', role:'specifier', actor:'ejackson' },
  { id:'rb-2',  milestone:'PROJ-176', role:'builder',   actor:'tpark' },
  { id:'rb-3',  milestone:'PROJ-176', role:'pilot',     actor:'kokafor', diagnostic:'reports-to-builder (1 level)' },
  { id:'rb-4',  milestone:'PROJ-204', role:'specifier', actor:'achen' },
  { id:'rb-5',  milestone:'PROJ-204', role:'builder',   actor:'dnasser' },
  { id:'rb-6',  milestone:'PROJ-204', role:'pilot',     actor:'krivas' },
  { id:'rb-7',  milestone:'PROJ-119', role:'specifier', actor:'ejackson' },
  { id:'rb-8',  milestone:'PROJ-119', role:'builder',   actor:'dnasser' },
  { id:'rb-9',  milestone:'PROJ-119', role:'pilot',     actor:null,      diagnostic:'pilot unassigned' },
  { id:'rb-10', milestone:'PROJ-112', role:'specifier', actor:'ejackson' },
  { id:'rb-11', milestone:'PROJ-112', role:'builder',   actor:'dnasser' },
  { id:'rb-12', milestone:'PROJ-112', role:'pilot',     actor:'bliang' },
];

/* --------- Event log (read-only) --------- */
const EVENT_LOG = [
  { num:251, type:'work.role_bound',        target:'PROJ-119', actor:'ejackson', time:'2026-05-22 09:14', payload:'role=builder · actor=dnasser', sig:'9a0f12ce' },
  { num:250, type:'work.classified',        target:'PROJ-204', actor:'achen',    time:'2026-05-22 08:51', payload:'taxonomy=product · node=…/recurring-billing', sig:'4d180a22' },
  { num:249, type:'work.ack_recommitted',   target:'PROJ-204', actor:'achen',    time:'2026-05-21 16:30', payload:'after clarification amendment', sig:'71e0ffcd' },
  { num:248, type:'work.ack_amended',       target:'PROJ-204', actor:'achen',    time:'2026-05-21 16:28', payload:'type=clarification · fields=[acceptance_criteria]', sig:'a1b2c3d4' },
  { num:247, type:'work.observation_recorded', target:'PROJ-198', actor:'krivas', time:'2026-05-19 14:22', payload:'pilot_log_entry_type=integrity_call · ack_integrity=drifting', sig:'02bcf09e' },
  { num:246, type:'work.review_requested',  target:'DB-091',   actor:'kokafor',  time:'2026-05-19 11:00', payload:'reviewer=leadership · reason=auto_escalation', sig:'59cc1107' },
  { num:245, type:'work.finding_recorded',  target:'PROJ-198', actor:'krivas',   time:'2026-05-17 09:08', payload:'pilot_log_entry_type=risk', sig:'d2210b91' },
  { num:244, type:'work.ack_amended',       target:'PROJ-176', actor:'ejackson', time:'2026-05-16 13:55', payload:'type=target_change · auto-cleared commitment', sig:'2c33a01b' },
  { num:243, type:'work.ack_cleared',       target:'PROJ-176', actor:'system',   time:'2026-05-16 13:55', payload:'reason=material_amendment', sig:'5d8e0c19' },
  { num:242, type:'work.execution_started', target:'PROJ-141', actor:'buildbot', time:'2026-05-15 18:12', payload:'lease=att_03 · agent=buildbot.agent', sig:'b1c00177' },
  { num:241, type:'work.eval_completed',    target:'PROJ-198', actor:'krivas',   time:'2026-05-15 11:30', payload:'pilot_log_entry_type=stress_test', sig:'88aabbcc' },
  { num:240, type:'work.observation_recorded', target:'PROJ-176', actor:'kokafor', time:'2026-05-15 10:02', payload:'pilot_log_entry_type=integrity_call · ack_integrity=invalid', sig:'cd003314' },
  { num:239, type:'work.linked',            target:'PROJ-198', actor:'ejackson', time:'2026-05-13 14:00', payload:'depends_on=PROJ-176', sig:'fe09a811' },
  { num:238, type:'work.observation_recorded', target:'PROJ-198', actor:'krivas', time:'2026-05-12 16:45', payload:'pilot_log_entry_type=observation', sig:'7700bbaa' },
  { num:237, type:'work.role_bound',        target:'PROJ-128', actor:'kokafor',  time:'2026-05-12 09:00', payload:'role=specifier · actor=kokafor', sig:'1e22f0f0' },
  { num:236, type:'work.finding_recorded',  target:'PROJ-141', actor:'krivas',   time:'2026-05-11 13:11', payload:'pilot_log_entry_type=risk', sig:'9001a2b3' },
];

/* --------- ACK amendments per milestone --------- */
const ACK_AMENDMENTS = {
  'PROJ-198':[
    { id:'amd-1', type:'scope_change',   by:'ejackson', when:'2026-05-10', fields:['scope_summary'], reason:'Added retry-storm hardening pass.' },
    { id:'amd-2', type:'timeline_change',by:'ejackson', when:'2026-05-04', fields:['delivery_timing'], reason:'Q3 mid-quarter slip; one sprint.' },
  ],
  'PROJ-176':[
    { id:'amd-3', type:'target_change',  by:'ejackson', when:'2026-05-16', fields:['target_outcome'], reason:'Constraint-engine carve-out moved out.' },
    { id:'amd-4', type:'scope_change',   by:'ejackson', when:'2026-05-09', fields:['scope_summary'],  reason:'Removed legacy compat path.' },
    { id:'amd-5', type:'scope_change',   by:'ejackson', when:'2026-04-29', fields:['scope_summary'],  reason:'Carved out independence diagnostics.' },
    { id:'amd-6', type:'clarification',  by:'ejackson', when:'2026-04-23', fields:['acceptance_criteria'], reason:'Made constraint output schema explicit.' },
  ],
  'PROJ-141':[
    { id:'amd-7', type:'scope_change',   by:'achen', when:'2026-05-14', fields:['scope_summary'], reason:'Added migration path.' },
    { id:'amd-8', type:'scope_change',   by:'achen', when:'2026-05-09', fields:['scope_summary'], reason:'Cut storage-format change.' },
    { id:'amd-9', type:'clarification',  by:'achen', when:'2026-05-02', fields:['acceptance_criteria'], reason:'Pinned compat criteria.' },
  ],
};

/* --------- Leading indicators (portfolio aggregate) --------- */
const INDICATORS = {
  depRate:    { value: 0.71, delta: '+0.04 wk', kind: 'up',   spark: '0,18 10,15 20,16 30,14 40,12 50,11 60,10 70,8 80,9 90,7 100,6 110,5' },
  scopeVel:   { value: 0.42, delta: '+0.10 wk', kind: 'warn', unit: '/wk', spark: '0,12 10,11 20,14 30,13 40,10 50,12 60,8 70,9 80,6 90,7 100,5 110,4' },
  decFric:    { value: '4.2', delta: '+1.1 d',  kind: 'down', unit: 'd', spark: '0,18 10,17 20,15 30,16 40,12 50,13 60,10 70,9 80,8 90,7 100,6 110,4' },
  ackLat:     { value: '6.1', delta: '\u22120.3 d', kind: 'up', unit: 'd', spark: '0,16 10,14 20,13 30,11 40,10 50,9 60,11 70,8 80,7 90,8 100,6 110,5' },
};

/* --------- Per-milestone enrichments: parent, stages, target precision,
   status narrative, risks, next steps. Applied after seed so we don't
   bloat the row literals above. --------- */
const EXTRA = {
  'PROJ-204': {
    targetPrecision:'D', targetISO:'2026-09-30', target:'2026-09-30',
    deps:['PROJ-141'],
    stages:[
      {key:'dogfood', label:'Dogfood', date:'2026-06-15', precision:'D', state:'done'},
      {key:'beta',    label:'Beta',    date:'2026-08-01', precision:'D', state:'soon'},
      {key:'ga',      label:'GA',      date:'2026-09-30', precision:'D', state:'open'},
    ],
    statusNarrative:'Two of three engine subsystems are integrated. Trial-period flow is staged for review next week; team confidence in Q3 GA is high.',
    statusUpdatedAt:'2026-05-22', statusUpdatedBy:'dnasser',
    risks:[
      {body:'Stripe webhooks have flakey IPv6 routes — fallback path needs hardening.', severity:'medium', by:'krivas', when:'2026-05-18'},
    ],
    nextSteps:[
      {body:'Cut Beta tag, dogfood with internal billing team.', owner:'dnasser', when:'2026-05-30'},
      {body:'Draft customer migration guide.', owner:'achen', when:'2026-06-04'},
    ],
  },
  'PROJ-198': {
    targetPrecision:'M', targetISO:'2026-09-15', target:'Sept 2026',
    deps:['PROJ-176'],
    stages:[
      {key:'dogfood', label:'Dogfood', date:'2026-07-15', precision:'D', state:'open'},
      {key:'beta',    label:'Beta',    date:'2026 Q3',    precision:'Q', state:'open'},
      {key:'ga',      label:'GA',      date:'2026 Q4',    precision:'Q', state:'open'},
    ],
    statusNarrative:'Retry-storm hardening pass added late. Builder pending re-ACK while we choose between token bucket and adaptive backoff (see DB-088).',
    statusUpdatedAt:'2026-05-19', statusUpdatedBy:'krivas',
    risks:[
      {body:'Scope velocity +0.5/wk above pre-commit baseline. Drift risk.', severity:'high', by:'krivas', when:'2026-05-19'},
      {body:'Upstream PROJ-176 still pending; downstream cannot integration-test.', severity:'medium', by:'krivas', when:'2026-05-17'},
    ],
    nextSteps:[
      {body:'Resolve DB-088 (retry algorithm choice) by end of week.', owner:'krivas', when:'2026-05-27'},
      {body:'Builder re-ACK after spec amendment lands.', owner:'tpark', when:'2026-05-28'},
    ],
  },
  'PROJ-187': {
    targetPrecision:'Q', targetISO:'2026-09-30',
    stages:[
      {key:'ga', label:'GA', date:'2026 Q3', precision:'Q', state:'open'},
    ],
    statusNarrative:'Compliance review is the long pole. Engineering is ahead — waiting on audit-team handoff confirmation.',
    statusUpdatedAt:'2026-05-20', statusUpdatedBy:'mhong',
    risks:[
      {body:'Cross-team handoff to audit-team has no SLA.', severity:'low', by:'krivas', when:'2026-05-07'},
    ],
    nextSteps:[
      {body:'Confirm audit-team review window.', owner:'mhong', when:'2026-05-28'},
    ],
  },
  'PROJ-176': {
    targetPrecision:'Q', targetISO:'2026-12-31',
    stages:[
      {key:'spike', label:'Spike',    date:'2026-04-30', precision:'D', state:'done'},
      {key:'beta',  label:'Beta',     date:'2026 Q3',    precision:'Q', state:'open'},
      {key:'ga',    label:'GA',       date:'2026 Q4',    precision:'Q', state:'open'},
    ],
    statusNarrative:'Stalled. Target shifted twice, Specifier has rejected the latest re-spec. Pilot has stood down on RYG until commitment is reformed (Pilot\u2019s Log §240).',
    statusUpdatedAt:'2026-04-15', statusUpdatedBy:'ejackson',
    risks:[
      {body:'Pilot reports to Builder within 1 level — independence diagnostic firing.', severity:'high', by:'engine', when:'2026-05-09'},
      {body:'Decision DB-091 has been idle 11 days; auto-escalated to Leadership.', severity:'high', by:'engine', when:'2026-05-19'},
      {body:'No product classification — rollups exclude this work.', severity:'medium', by:'engine', when:'2026-05-01'},
    ],
    nextSteps:[
      {body:'Re-bind Pilot to a different actor outside the Builder\u2019s reporting line.', owner:'ejackson', when:'2026-05-26'},
      {body:'Reform commitment with concrete acceptance criteria.', owner:'ejackson', when:'2026-05-30'},
    ],
  },
  'PROJ-163': {
    targetPrecision:'D', targetISO:'2026-04-30', target:'Apr 30, 2026',
    stages:[
      {key:'ga', label:'GA', date:'2026-04-30', precision:'D', state:'done'},
    ],
    statusNarrative:'Shipped. Outcome assessment OA-031 verdict: achieved · high value (mobile checkout lift +14%).',
    statusUpdatedAt:'2026-05-01', statusUpdatedBy:'achen',
    risks:[],
    nextSteps:[
      {body:'Capture mobile-web parity playbook for future client work.', owner:'achen', when:'2026-06-15'},
    ],
  },
  'PROJ-141': {
    targetPrecision:'Q', targetISO:'2026-09-30',
    stages:[
      {key:'beta', label:'Beta', date:'2026 Q3', precision:'Q', state:'open'},
      {key:'ga',   label:'GA',   date:'2026 Q3', precision:'Q', state:'open'},
    ],
    statusNarrative:'Scope amended twice this month. Specifier re-ACK pending. Open DecisionBlock DB-086 on Q4 backport decision.',
    statusUpdatedAt:'2026-05-15', statusUpdatedBy:'achen',
    risks:[
      {body:'Two scope changes in one week despite green RYG.', severity:'medium', by:'krivas', when:'2026-05-11'},
    ],
    nextSteps:[
      {body:'Resolve DB-086 (Q4 backport) before Friday.', owner:'krivas', when:'2026-05-29'},
    ],
  },
  'PROJ-128': {
    targetPrecision:'Q', targetISO:'2026-09-30',
    deps:['PROJ-112'],
    fromRfc:'PROJ-138',
    stages:[
      {key:'beta', label:'Beta', date:'2026 Q3', precision:'Q', state:'open'},
      {key:'ga',   label:'GA',   date:'2026 Q3', precision:'Q', state:'open'},
    ],
    statusNarrative:'Filed yesterday. Builder ACK pending — looking for a Q3 commit signal from infra-team.',
    statusUpdatedAt:'2026-05-22', statusUpdatedBy:'kokafor',
    risks:[],
    nextSteps:[
      {body:'Ping T. Park for Builder ACK by Wed.', owner:'kokafor', when:'2026-05-28'},
    ],
  },
  'PROJ-119': {
    targetPrecision:'Q', targetISO:'2026-12-31',
    stages:[
      {key:'spike', label:'Spike', date:'2026 Q4', precision:'Q', state:'open'},
    ],
    statusNarrative:'',
    statusUpdatedAt:null, statusUpdatedBy:null,
    risks:[
      {body:'Pilot unassigned.', severity:'medium', by:'engine', when:'2026-05-17'},
      {body:'No product classification.', severity:'low', by:'engine', when:'2026-05-17'},
    ],
    nextSteps:[
      {body:'Assign Pilot, classify under platform / observability.', owner:'ejackson', when:'2026-05-28'},
    ],
  },
  'PROJ-112': {
    targetPrecision:'M', targetISO:'2026-07-31', target:'July 2026',
    fromRfc:'PROJ-145',
    stages:[
      {key:'beta', label:'Beta', date:'2026-07-01', precision:'D', state:'open'},
      {key:'ga',   label:'GA',   date:'2026-07-31', precision:'D', state:'open'},
    ],
    statusNarrative:'Two predicates live in dev (independence, role-collapse). Builder is wiring up the public diagnostic API.',
    statusUpdatedAt:'2026-05-21', statusUpdatedBy:'dnasser',
    risks:[],
    nextSteps:[
      {body:'Add product-classification predicate.', owner:'dnasser', when:'2026-06-05'},
      {body:'Pilot stress-test the API.', owner:'bliang', when:'2026-06-10'},
    ],
  },
  'PROJ-103': {
    targetPrecision:'Q', targetISO:'2026-09-30',
    deps:['PROJ-112'],
    fromRfc:'PROJ-152',
    stages:[
      {key:'spike', label:'Spike', date:'2026-05-10', precision:'D', state:'done'},
      {key:'beta',  label:'Beta',  date:'2026 Q3',    precision:'Q', state:'open'},
      {key:'ga',    label:'GA',    date:'2026 Q3',    precision:'Q', state:'open'},
    ],
    statusNarrative:'KMS choice still open (DB-077, 10 days idle). Stalling Beta.',
    statusUpdatedAt:'2026-05-18', statusUpdatedBy:'mhong',
    risks:[
      {body:'DB-077 is 10 days idle — escalation threshold tomorrow.', severity:'medium', by:'engine', when:'2026-05-18'},
    ],
    nextSteps:[
      {body:'Force KMS choice in DB-077.', owner:'krivas', when:'2026-05-27'},
    ],
  },
};

/* --------- Child milestones (deliverables can nest) --------- */
const CHILD_MILESTONES = [
  { id:'PROJ-204a', kind:'milestone', parent:'PROJ-204', title:'Subscription engine',
    status:'in_flight', ryg:'g', specifierAck:'accepted', builderAck:'accepted', ackHistory:[
      {who:'builder', actor:'dnasser', action:'accepted', when:'2026-05-08', note:'Schema is fine.'},
      {who:'specifier', actor:'achen', action:'accepted', when:'2026-05-05', note:'Initial ACK.'},
    ],
    target:'2026-08-01', targetPrecision:'D', targetISO:'2026-08-01',
    stages:[{key:'beta', label:'Beta', date:'2026-08-01', precision:'D', state:'soon'},{key:'ga', label:'GA', date:'2026-09-30', precision:'D', state:'open'}],
    customerVisible:false, specifier:'achen', builder:'dnasser', pilot:'krivas',
    orgNode:'acme/platform/payments-team', productNode:'commerce/checkout/payments/recurring-billing',
    scopeChanges:0, depClosed:[3,4], latency:'2d', fresh:'fresh', age:'2d', sig:'7c11ee04',
    statusNarrative:'Recurrence model + scheduler are integrated. Idempotency layer wires through PROJ-141.',
    statusUpdatedAt:'2026-05-24', statusUpdatedBy:'dnasser',
    risks:[],
    nextSteps:[{body:'Migration tests on production-like data.', owner:'dnasser', when:'2026-06-01'}],
    diagnostics:[], indicators:{depRate:0.75, scopeVel:0.0, decFric:'1.4d', ackLat:'2d'} },
  { id:'PROJ-204b', kind:'milestone', parent:'PROJ-204', title:'Trial period support',
    status:'ack_filed', ryg:'g', specifierAck:'accepted', builderAck:'pending', ackHistory:[
      {who:'specifier', actor:'achen', action:'accepted', when:'2026-05-21', note:'Initial ACK; awaiting Builder.'},
    ],
    target:'2026-07-31', targetPrecision:'D', targetISO:'2026-07-31',
    stages:[{key:'beta', label:'Beta', date:'2026-07-15', precision:'D', state:'open'},{key:'ga', label:'GA', date:'2026-07-31', precision:'D', state:'open'}],
    customerVisible:true, specifier:'achen', builder:'dnasser', pilot:'krivas',
    orgNode:'acme/platform/payments-team', productNode:'commerce/checkout/payments/recurring-billing',
    scopeChanges:0, depClosed:[0,2], latency:null, fresh:'fresh', age:'3d', sig:'9f02abdc',
    statusNarrative:'Filed last week — Builder ACK pending. Likely fast follow once engine lands.',
    statusUpdatedAt:'2026-05-22', statusUpdatedBy:'achen',
    risks:[], nextSteps:[{body:'Builder ACK by Wed.', owner:'dnasser', when:'2026-05-28'}],
    diagnostics:[], indicators:{} },
];

/* Merge enrichments into the seed array. */
const MILESTONES_FINAL = [
  ...MILESTONES.map(m => ({ ...m, ...(EXTRA[m.id] || {}) })),
  ...CHILD_MILESTONES,
];

/* The hand-authored seed, kept as a fallback for when /api/data is
   unreachable (local dev with no substrate). Wrapped in a function so the
   live load below can fall back to it without re-running the file. */
function seedData() {
  return {
    ACTORS, TAXONOMIES, ROLES, ROLE_CONSTRAINTS,
    MILESTONES: MILESTONES_FINAL, RFCS, PILOT_LOG, DECISIONS, OUTCOMES,
    ROLE_BINDINGS, EVENT_LOG, ACK_AMENDMENTS, INDICATORS,
  };
}

/* Live data load. The prototype reads window.DATA synchronously at module
   load (views.jsx/App.jsx capture it before first render), and data.jsx runs
   first in script order — so we fetch /api/data with a *synchronous* XHR here
   to preserve that contract with zero async restructuring. This is acceptable
   for an internal tool booting once. On any failure (no substrate, bad JSON,
   empty body) we fall back to the authored seed so the prototype still runs. */
(function loadLiveData() {
  try {
    const xhr = new XMLHttpRequest();
    xhr.open('GET', '/api/data', false); // false = synchronous
    xhr.send(null);
    if (xhr.status >= 200 && xhr.status < 300 && xhr.responseText) {
      const live = JSON.parse(xhr.responseText);
      if (live && typeof live === 'object') {
        window.DATA = live;
        return;
      }
    }
  } catch (e) {
    /* fall through to seed */
    console.warn('pilot: /api/data load failed, using seed fallback', e);
  }
  window.DATA = seedData();
})();
