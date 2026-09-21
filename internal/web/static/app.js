const state = { data: null, selected: null, drag: null };
const geom = {
  width: 1180, height: 760, margin: { top: 36, bottom: 34 },
  centerX: 590, laneW: 190, panelW: 380, connectorW: 420
};

function api(path, options = {}) {
  return fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...options
  }).then(async response => {
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || response.statusText);
    return body;
  });
}

function fmt(value, digits = 2) {
  return Number(value).toFixed(digits);
}

function y(depth, domain) {
  const usable = geom.height - geom.margin.top - geom.margin.bottom;
  return geom.margin.top + ((depth - domain[0]) / (domain[1] - domain[0])) * usable;
}

function depthFromY(pixel, domain) {
  const usable = geom.height - geom.margin.top - geom.margin.bottom;
  return domain[0] + ((pixel - geom.margin.top) / usable) * (domain[1] - domain[0]);
}

function el(tag, attrs = {}, children = []) {
  const node = document.createElementNS('http://www.w3.org/2000/svg', tag);
  for (const [key, value] of Object.entries(attrs)) node.setAttribute(key, value);
  children.forEach(child => node.appendChild(typeof child === 'string' ? document.createTextNode(child) : child));
  return node;
}

function scaleFor(curve, side) {
  const samples = side === 'A' ? curve.samplesA : curve.samplesB;
  let min = Infinity, max = -Infinity;
  samples.forEach(s => { min = Math.min(min, s.value); max = Math.max(max, s.value); });
  return { min, max };
}

function xForCurve(index, side) {
  const base = side === 'A' ? geom.centerX - geom.connectorW / 2 : geom.centerX + geom.connectorW / 2;
  const direction = side === 'A' ? -1 : 1;
  return base + direction * (28 + index * geom.laneW);
}

function render() {
  const s = state.data;
  if (!s) return;
  document.getElementById('run-title').textContent = `${s.run.name} · ${s.run.depthUnit} · ${s.run.description}`;
  document.getElementById('min-stretch').value = s.settings.minStretch;
  document.getElementById('max-stretch').value = s.settings.maxStretch;
  document.getElementById('soft-weight').value = s.settings.softWeight;
  document.getElementById('candidate-count').value = s.settings.maxCandidates;

  const result = s.latest?.result;
  if (result?.candidates?.length) {
    const preferred = state.selected || s.latest.selected;
    const rank = result.candidates.some(c => c.rank === preferred)
      ? preferred : result.candidates[0].rank;
    state.selected = rank;
  }
  renderConflicts(result?.conflicts || []);
  renderChart();
  renderCandidates(result);
  renderControls();
  renderGaps(result?.noDataSegments || s.run.gapsA.map((gap, i) => ({
    from: { left: gap.from, right: s.run.gapsB[i].from },
    to: { left: gap.to, right: s.run.gapsB[i].to },
    kind: 'no_data'
  })));
  renderRuns();
}

function renderConflicts(conflicts) {
  const box = document.getElementById('conflicts');
  if (!conflicts.length) {
    box.innerHTML = '<div class="conflict"><div><strong>未发现冲突</strong><span class="muted">硬锦标单调、软锦标允许按权重偏离。</span></div></div>';
    return;
  }
  box.innerHTML = conflicts.map(c => `
    <div class="conflict ${c.severity}">
      <div>
        <strong>${c.severity === 'blocking' ? '阻断冲突' : '提示冲突'} · ${c.code}</strong>
        <div>${c.message}</div>
        ${c.chain?.length ? `<div class="muted">最短冲突链：${c.chain.map(id => `<code>${id}</code>`).join(' → ')}</div>` : ''}
      </div>
      <div class="muted">${(c.controlIds || []).join(' / ')}</div>
    </div>`).join('');
  const revocable = conflicts.find(c => c.chain?.length > 1);
  if (revocable) {
    const first = revocable.chain[0];
    box.insertAdjacentHTML('beforeend', `<div class="conflict advisory"><div><strong>验收操作</strong><span>撤销冲突链中的 <code>${first}</code> 后可恢复求解。</span></div><button onclick="revoke('${first}')">撤销 ${first}</button></div>`);
  }
}

function renderChart() {
  const s = state.data;
  const domainA = [s.run.domainA.from, s.run.domainA.to];
  const domainB = [s.run.domainB.from, s.run.domainB.to];
  const svg = el('svg', { viewBox: `0 0 ${geom.width} ${geom.height}`, width: geom.width, height: geom.height });
  const defs = el('defs');
  defs.innerHTML = '<marker id="arrow" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto"><path d="M0,0 L0,6 L7,3 z" fill="#64748b"/></marker>';
  svg.appendChild(defs);

  for (let tick = 1000; tick <= 1100; tick += 10) {
    const yy = y(tick, domainA);
    svg.appendChild(el('line', { x1: 34, x2: geom.width - 34, y1: yy, y2: yy, class: 'grid-line' }));
    svg.appendChild(el('text', { x: 18, y: yy + 4, class: 'axis-label' }, [String(tick)]));
  }
  svg.appendChild(el('text', { x: 28, y: 20, class: 'axis-label' }, ['趟次 A（左井深 m）']));
  svg.appendChild(el('text', { x: geom.width - 190, y: 20, class: 'axis-label' }, ['趟次 B（右井深 m）']));

  drawPanel(svg, domainA, s.run.gapsA, 'A');
  drawPanel(svg, domainB, s.run.gapsB, 'B');
  s.run.curves.forEach((curve, index) => {
    drawCurve(svg, curve, index, 'A', domainA);
    drawCurve(svg, curve, index, 'B', domainB);
  });
  drawMapping(svg, domainA, domainB);
  s.controls.concat(s.proposals).forEach(control => drawControl(svg, control, domainA, domainB));
  attachDrag(svg);
  const chart = document.getElementById('chart');
  chart.replaceChildren(svg);
}

function drawPanel(svg, domain, gaps, side) {
  const startX = side === 'A' ? geom.centerX - geom.connectorW / 2 : geom.centerX + geom.connectorW / 2;
  const endX = startX + (side === 'A' ? -geom.panelW : geom.panelW);
  const left = Math.min(startX, endX);
  svg.appendChild(el('rect', { x: left, y: geom.margin.top, width: geom.panelW, height: geom.height - geom.margin.top - geom.margin.bottom, fill: '#fbfdff', stroke: '#cbd5e1' }));
  gaps.forEach(gap => svg.appendChild(el('rect', {
    x: left, width: geom.panelW,
    y: y(gap.from, domain), height: y(gap.to, domain) - y(gap.from, domain),
    class: 'gap-band'
  })));
}

function drawCurve(svg, curve, index, side, domain) {
  const x = xForCurve(index, side);
  const scale = scaleFor(curve, side);
  const samples = side === 'A' ? curve.samplesA : curve.samplesB;
  const points = samples.map(sample => {
    const normalized = (sample.value - scale.min) / (scale.max - scale.min || 1);
    const xx = x + (normalized - 0.5) * 150;
    return `${fmt(xx, 1)},${fmt(y(sample.depth, domain), 1)}`;
  }).join(' ');
  svg.appendChild(el('polyline', { points, class: 'curve-line', stroke: curve.color }));
  const labelX = Math.max(12, x - 70);
  svg.appendChild(el('text', { x: labelX, y: 22, fill: curve.color, class: 'axis-label' }, [`${curve.name} ${curve.unit}`]));
}

function chosenCandidate() {
  const result = state.data.latest?.result;
  return result?.candidates?.find(c => c.rank === state.selected) || null;
}

function drawMapping(svg, domainA, domainB) {
  const result = state.data.latest?.result;
  if (!result?.candidates) return;
  result.candidates.forEach(candidate => {
    const selected = candidate.rank === state.selected;
    const points = candidate.points.map(p => {
      const xa = geom.centerX - geom.connectorW / 2;
      const xb = geom.centerX + geom.connectorW / 2;
      return `${fmt(xa)},${fmt(y(p.left, domainA), 1)} ${fmt(xb)},${fmt(y(p.right, domainB), 1)}`;
    }).join(' ');
    const path = points.split(' ').reduce((acc, pair, index) => acc + (index ? ' L ' : 'M') + pair, '');
    svg.appendChild(el('path', { d: path, class: `map-line ${selected ? '' : 'alt'}` }));
  });
  const chosen = chosenCandidate();
  chosen?.segments.forEach((segment, index) => {
    if (segment.kind !== 'data' || (index % 3 !== 1 && !segment.to.required)) return;
    const xa = geom.centerX - 12;
    const xb = geom.centerX + 12;
    const midA = (segment.from.left + segment.to.left) / 2;
    const midB = (segment.from.right + segment.to.right) / 2;
    svg.appendChild(el('text', { x: geom.centerX - 18, y: (y(midA, domainA) + y(midB, domainB)) / 2, 'text-anchor': 'middle', class: 'axis-label', fill: segment.warning ? '#b91c1c' : '#166534' }, [`×${fmt(segment.stretch, 2)}`]));
  });
  chosen?.segments.filter(segment => segment.kind === 'gap_bridge').forEach(segment => {
    const midA = (segment.from.left + segment.to.left) / 2;
    const midB = (segment.from.right + segment.to.right) / 2;
    svg.appendChild(el('text', { x: geom.centerX + 18, y: (y(midA, domainA) + y(midB, domainB)) / 2, 'text-anchor': 'middle', class: 'axis-label', fill: '#64748b' }, [`空缺 ×${fmt(segment.stretch, 2)}`]));
  });
}

function drawControl(svg, control, domainA, domainB) {
  const active = control.status === 'active';
  const proposed = control.status === 'proposed';
  const cls = proposed ? 'proposed' : control.kind;
  const aX = geom.centerX - geom.connectorW / 2;
  const bX = geom.centerX + geom.connectorW / 2;
  const aY = y(control.leftDepth, domainA);
  const bY = y(control.rightDepth, domainB);
  const opacity = active || proposed ? 1 : 0.25;
  const line = el('line', { x1: aX, x2: bX, y1: aY, y2: bY, class: `marker-line ${cls}`, opacity, 'data-id': control.id });
  svg.appendChild(line);
  [['A', aX, aY, control.leftDepth, domainA], ['B', bX, bY, control.rightDepth, domainB]].forEach(([side, x, yy, depth, domain]) => {
    const group = el('g', { class: 'marker-handle', 'data-id': control.id, 'data-side': side, opacity });
    const diamond = el('polygon', {
      points: `${x},${yy - 7} ${x + 7},${yy} ${x},${yy + 7} ${x - 7},${yy}`,
      fill: cls === 'hard' ? '#dc2626' : cls === 'soft' ? '#2563eb' : '#d97706',
      stroke: '#fff', 'stroke-width': 1.5
    });
    group.appendChild(diamond);
    group.appendChild(el('text', {
      x: side === 'A' ? x - 12 : x + 12,
      y: yy + 4, 'text-anchor': side === 'A' ? 'end' : 'start',
      class: 'marker-label', fill: cls === 'hard' ? '#b91c1c' : '#1e40af'
    }, [`${control.id} ${fmt(depth, 1)}`]));
    svg.appendChild(group);
  });
}

function attachDrag(svg) {
  svg.querySelectorAll('.marker-handle').forEach(group => {
    group.addEventListener('pointerdown', event => {
      const id = group.dataset.id;
      const side = group.dataset.side;
      const control = state.data.controls.concat(state.data.proposals).find(c => c.id === id);
      if (!control || control.status === 'revoked' || control.source === 'formation') return;
      state.drag = { id, side, element: group, moved: false };
      group.setPointerCapture(event.pointerId);
      event.preventDefault();
    });
    group.addEventListener('pointermove', event => {
      if (!state.drag || state.drag.element !== group) return;
      const side = state.drag.side;
      const domain = side === 'A' ? [state.data.run.domainA.from, state.data.run.domainA.to] : [state.data.run.domainB.from, state.data.run.domainB.to];
      const depth = Math.round(depthFromY(event.clientY - svg.getBoundingClientRect().top, domain) * 2) / 2;
      state.drag.pending = depth;
      state.drag.moved = true;
    });
    group.addEventListener('pointerup', async () => {
      if (!state.drag?.moved) { state.drag = null; return; }
      const { id, side, pending } = state.drag;
      state.drag = null;
      const patch = side === 'A' ? { leftDepth: pending } : { rightDepth: pending };
      try {
        state.data = await api(`/api/controls/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(patch) });
        state.selected = null;
        render();
      } catch (error) { alert(error.message); }
    });
  });
}
function renderCandidates(result) {
  const box = document.getElementById('candidates');
  if (!result?.ok || !result.candidates?.length) {
    box.innerHTML = `<p class="muted">当前没有候选路径。先处理阻断冲突。</p><p>${result?.message || ''}</p>`;
    return;
  }
  box.innerHTML = result.candidates.map(candidate => `
    <div class="candidate-card ${candidate.rank === state.selected ? 'selected' : ''}" onclick="selectCandidate(${candidate.rank})">
      <strong>候选 ${candidate.rank}</strong>
      <span class="muted"> · 与最佳 MAD ${fmt(candidate.diffFromBestMad, 2)} m / Max ${fmt(candidate.diffFromBestMax, 2)} m</span>
      <div class="costs">
        <span>总成本 <b>${fmt(candidate.costs.total)}</b></span>
        <span>相似 <b>${fmt(candidate.costs.similarity)}</b></span>
        <span>软惩罚 <b>${fmt(candidate.costs.softPenalty)}</b></span>
        <span>正则 <b>${fmt(candidate.costs.regularity)}</b></span>
        <span>无数据桥 <b>${fmt(candidate.costs.gapBridgesM, 1)} m</b></span>
      </div>
      <div class="costs">
        ${candidate.residuals.map(r => `<span>${r.curveKey}: SSE <b>${fmt(r.sse, 2)}</b>, Max <b>${fmt(r.maxAbs, 3)}</b></span>`).join('')}
      </div>
      ${candidate.softDeviations?.length ? `<div class="costs">${candidate.softDeviations.map(d => `<span>${d.controlId}: ${fmt(d.actual)} m（偏离 ${fmt(d.deviation)} m，成本 ${fmt(d.cost)}）</span>`).join('')}</div>` : ''}
      ${candidate.segments.some(segment => segment.warning) ? `<div class="costs"><span class="danger">${candidate.segments.filter(segment => segment.warning).map(segment => `${segment.kind === 'gap_bridge' ? '无数据桥' : '段'} ${fmt(segment.from.left)}-${fmt(segment.to.left)}: ${segment.warning}`).join('；')}</span></div>` : ''}
    </div>`).join('');
}

function renderControls() {
  const all = state.data.controls.concat(state.data.proposals);
  document.getElementById('controls').innerHTML = `<table>
    <thead><tr><th>ID</th><th>类型</th><th>来源</th><th>状态</th><th>A 深</th><th>B 深</th><th>操作</th></tr></thead>
    <tbody>${all.map(c => `<tr>
      <td>${c.id}</td><td><span class="pill ${c.kind}">${c.kind}</span></td><td>${c.source}</td>
      <td><span class="pill ${c.status}">${c.status}</span></td>
      <td>${fmt(c.leftDepth, 1)}</td><td>${fmt(c.rightDepth, 1)}</td>
      <td>${c.status === 'active' ? `<button onclick="revoke('${c.id}')">撤销</button>` : c.status === 'revoked' ? `<button onclick="restore('${c.id}')">恢复</button>` : `<button onclick="acceptProposal('${c.id}')">确认</button>`}</td>
    </tr>`).join('')}</tbody></table>
    <p class="muted">注：三个 formation 地层硬锦标为已确认地层层位，页面禁止拖动；manual/auto 约束可拖动以验证局部伸缩率。</p>`;
}

function renderGaps(segments) {
  const box = document.getElementById('gaps');
  if (!segments.length) { box.innerHTML = '<p class="muted">无无数据段。</p>'; return; }
  box.innerHTML = `<table><thead><tr><th>段</th><th>A 范围</th><th>B 范围</th><th>跨段伸缩率</th><th>说明</th></tr></thead><tbody>
    ${segments.map((segment, i) => {
      const from = segment.from;
      const to = segment.to;
      const stretch = segment.stretch || ((to.right - from.right) / (to.left - from.left));
      return `<tr><td>${i + 1}</td><td>${fmt(from.left, 1)}–${fmt(to.left, 1)}</td><td>${fmt(from.right, 1)}–${fmt(to.right, 1)}</td><td>×${fmt(stretch, 3)}</td><td>不传播曲线相似度</td></tr>`;
    }).join('')}
  </tbody></table>`;
}

async function renderRuns() {
  const runs = await api('/api/runs?limit=20');
  document.getElementById('runs').innerHTML = runs.map(run => `
    <div class="run-row">
      <strong>#${run.id}</strong>
      <span class="${run.ok ? 'ok' : 'danger'}">${run.ok ? '可行' : '阻断'}</span>
      <span class="muted">${run.createdAt}</span>
      <div>${run.message}</div>
      ${run.result?.boundaryViolations?.length ? `<div class="danger">${run.result.boundaryViolations.map(v => v.message).join('；')}</div>` : ''}
    </div>`).join('');
}

async function revoke(id) {
  try { state.data = await api(`/api/controls/${encodeURIComponent(id)}/revoke`, { method: 'POST' }); state.selected = null; render(); }
  catch (error) { alert(error.message); }
}
async function restore(id) {
  try { state.data = await api(`/api/controls/${encodeURIComponent(id)}/restore`, { method: 'POST' }); state.selected = null; render(); }
  catch (error) { alert(error.message); }
}
async function acceptProposal(id) {
  try { state.data = await api(`/api/proposals/${encodeURIComponent(id)}/accept`, { method: 'POST' }); state.selected = null; render(); }
  catch (error) { alert(error.message); }
}
async function selectCandidate(rank) {
  state.selected = rank;
  render();
  const runId = state.data.latest?.id;
  if (runId) {
    try { await api('/api/runs/select', { method: 'POST', body: JSON.stringify({ runId, rank }) }); }
    catch (error) { console.warn(error); }
  }
}

async function saveSettings() {
  const settings = {
    ...state.data.settings,
    minStretch: Number(document.getElementById('min-stretch').value),
    maxStretch: Number(document.getElementById('max-stretch').value),
    softWeight: Number(document.getElementById('soft-weight').value),
    maxCandidates: Number(document.getElementById('candidate-count').value)
  };
  try { state.data = await api('/api/settings', { method: 'PUT', body: JSON.stringify(settings) }); state.selected = null; render(); }
  catch (error) { alert(error.message); }
}

document.getElementById('solve-btn').addEventListener('click', async () => {
  state.data = await api('/api/solve', { method: 'POST' }); state.selected = null; render();
});
document.getElementById('settings-btn').addEventListener('click', saveSettings);
document.getElementById('reset-btn').addEventListener('click', async () => {
  state.data = await api('/api/admin/reset', { method: 'POST' }); state.selected = null; render();
});
document.getElementById('export-btn').addEventListener('click', () => { window.location = '/api/export'; });
document.getElementById('import-file').addEventListener('change', async event => {
  const file = event.target.files[0];
  if (!file) return;
  try {
    const snapshot = JSON.parse(await file.text());
    state.data = await api('/api/admin/import', { method: 'POST', body: JSON.stringify(snapshot) });
    state.selected = null;
    render();
  } catch (error) { alert(error.message); }
  event.target.value = '';
});

api('/api/state').then(data => { state.data = data; render(); }).catch(error => alert(error.message));
