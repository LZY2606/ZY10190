const svgNS='http://www.w3.org/2000/svg';
const state={dataset:null,latest:null,selected:null,config:{min_stretch:.75,max_stretch:1.2,soft_weight:2}};
const geom={top:55,bottom:710,leftA:42,leftMap:320,leftB:700,widthA:220,widthMap:250,widthB:300};
const colors=['#16a34a','#7c3aed','#2563eb','#ea580c'];
const $=id=>document.getElementById(id);
function el(name,attrs={},parent){const n=document.createElementNS(svgNS,name);Object.entries(attrs).forEach(([k,v])=>n.setAttribute(k,v));if(parent)parent.appendChild(n);return n}
function fmt(v,d=2){return Number(v).toFixed(d)}
function depthMin(pass){return Math.min(...Object.values(state.dataset.curves).filter(c=>c.pass===pass).map(c=>c.min_depth))}
function depthMax(pass){return Math.max(...Object.values(state.dataset.curves).filter(c=>c.pass===pass).map(c=>c.max_depth))}
function yScale(depth,pass){const lo=depthMin(pass),hi=depthMax(pass);return geom.top+(depth-lo)/(hi-lo)*(geom.bottom-geom.top)}
function result(){return state.latest?.result||null}
function currentCandidate(){const r=result();if(!r||!r.ok)return null;const id=state.selected||r.selected_id;return r.candidates.find(c=>c.id===id)||r.candidates[0]||null}
function markerConflicts(id){const r=result();if(!r)return[];return r.conflicts.filter(c=>c.chain.some(m=>m.id===id)).concat((r.warnings||[]).filter(c=>c.chain.some(m=>m.id===id)))}
function isConflictMarker(id){return markerConflicts(id).some(c=>c.kind==='hard_crossing')}
function render(){renderConflicts();renderMarkers();renderChart();renderCandidates();renderMeta();renderRuns()}
function renderConflicts(){
 const r=result(),p=$('conflictPanel');p.innerHTML='';
 const list=[...(r?.conflicts||[]).map(c=>({...c,warning:false})),...((r?.warnings||[]).map(c=>({...c,warning:true})))];
 if(!list.length){p.innerHTML='<div class="conflict-card">当前无硬冲突；自动建议与人工约束冲突会以警告列示。</div>';return}
 list.forEach(c=>{const card=document.createElement('div');card.className='conflict'+(c.warning?' warning':'');card.innerHTML=`<strong>${c.warning?'自动建议冲突':'求解阻断'}：</strong>${c.message}<div class="conflict-chain"></div>`;const chain=card.querySelector('.conflict-chain');c.chain.forEach((m,i)=>{if(i)chain.appendChild(document.createTextNode('→'));const b=document.createElement('span');b.className='chain-marker';b.textContent=`${m.label} (${fmt(m.a_from,1)}→${fmt(m.b_to,1)})`;chain.appendChild(b);if(c.segment){const s=document.createElement('small');s.textContent=`；伸缩率 ${fmt(c.segment.stretch,3)}`;chain.appendChild(s)}});p.appendChild(card)})
}
function renderMarkers(){
 const box=$('markerList');box.innerHTML='';state.dataset.markers.forEach(m=>{const issues=markerConflicts(m.id);const d=document.createElement('div');d.className='marker';d.innerHTML=`<div class="marker-title"><span>${m.label}</span><span>${m.active?'启用':'停用'}</span></div><small>${m.kind==='hard'?'硬锦标':'软锦标'} · ${m.origin} · A ${fmt(m.a_from,1)} m ↔ B ${fmt(m.b_to,1)} m${issues.length?' · '+issues.length+' 个冲突':''}</small><div class="marker-actions"></div>`;const acts=d.querySelector('.marker-actions');const toggle=document.createElement('button');toggle.textContent=m.active?'停用（撤销）':'启用';toggle.onclick=()=>toggleMarker(m.id,!m.active);acts.appendChild(toggle);if(m.origin!=='fixture'&&m.kind==='hard'){const soft=document.createElement('button');soft.textContent='转软锦标';soft.onclick=()=>upsert(m,{kind:'soft'});acts.appendChild(soft)}box.appendChild(d)})
}
function renderCandidates(){
 const box=$('candidateList');box.innerHTML='';const c=currentCandidate();const r=result();if(!r?.ok){box.innerHTML='<div class="panel-card">存在冲突时不生成路径。撤销冲突锦标后重算。</div>';return}
 r.candidates.forEach(cand=>{const d=document.createElement('div');d.className='candidate'+(cand.id===c.id?' active':'');d.innerHTML=`<div class="candidate-title"><span>${cand.name}</span><span>${fmt(cand.costs.total,3)}</span></div><small>数据 ${fmt(cand.costs.data,3)} · 伸缩 ${fmt(cand.costs.stretch,3)} · 软锦标 ${fmt(cand.costs.soft,3)}</small><br><small>伸缩率 ${fmt(cand.stretch_min,3)}–${fmt(cand.stretch_max,3)} · 差异 ${(cand.diffs||[]).map(x=>x.name+':'+fmt(x.rms,3)).join(' ')}</small>`;d.onclick=()=>{state.selected=cand.id;render()};box.appendChild(d)})
}
function renderMeta(){
 const c=currentCandidate(),cost=$('costPanel'),nod=$('nodataPanel'),diff=$('diffPanel');
 if(!c){cost.textContent='未求解或求解被冲突阻断。'}else{cost.innerHTML=`<div class="metric"><span>数据相似</span><b>${fmt(c.costs.data,4)}</b><span>伸缩平滑</span><b>${fmt(c.costs.stretch,4)}</b><span>软锦标偏离</span><b>${fmt(c.costs.soft,4)}</b><span>该目标函数合计</span><b>${fmt(c.costs.total,4)}</b></div>`;
 diff.innerHTML=(c.diffs||[]).map(d=>`<div class="diff-row"><span>${d.name}</span><b>RMS ${fmt(d.rms,4)}</b></div>`).join('')||'无跨空缺差异样本。'}
 const gaps=result()?.no_data_segments||[];nod.innerHTML=gaps.map(g=>`A 趟 ${fmt(g.from,1)}–${fmt(g.to,1)} m`).join('<br>')||'无';$('candidateName').textContent=c?c.name:'候选路径';$('stretchSummary').textContent=c?`局部伸缩率 ${fmt(c.stretch_min,3)} – ${fmt(c.stretch_max,3)}`:''
}
async function renderRuns(){const runs=await api('/api/runs');$('runList').innerHTML=runs.map(r=>`<div class="panel-card">#${r.id} ${r.created_at.replace('T',' ').slice(0,19)}<br>${r.result.ok?'成功：'+r.result.selected_id:'冲突阻断：'+r.result.conflicts.length+' 条链'}</div>`).join('')||'<div class="panel-card">暂无运行。</div>'}
function renderChart(){
 const svg=$('chart');svg.innerHTML='';
 ['A 趟测井曲线','分段单调映射','B 趟测井曲线'].forEach((t,i)=>{el('text',{x:[165,445,855][i],y:28,'text-anchor':'middle',class:'lane-title'},svg).textContent=t});
 const names=[...new Set(Object.values(state.dataset.curves).map(c=>c.name))];
 drawAxis(svg,'A');drawAxis(svg,'B');
 names.forEach((name,ci)=>{drawLaneCurves(svg,name,'A',geom.leftA+75+ci*105);drawLaneCurves(svg,name,'B',geom.leftB+75+ci*125)});
 drawMapping(svg);drawMarkers(svg);
}
function drawAxis(svg,pass){
 const x=pass==='A'?geom.leftA:geom.leftB,w=pass==='A'?geom.widthA:geom.widthB;
 el('line',{x1:x,y1:geom.top,x2:x,y2:geom.bottom,stroke:'#98a2b3'},svg);
 for(let d=Math.ceil(depthMin(pass)/20)*20;d<=depthMax(pass);d+=40){const y=yScale(d,pass);el('line',{x1:x-5,y1:y,x2:x,y2:y,stroke:'#98a2b3'},svg);el('text',{x:x-8,y:y+4,'text-anchor':'end',class:'axis-label'},svg).textContent=d}
 el('rect',{x:x+8,y:geom.top,width:w-12,height:geom.bottom-geom.top,fill:'transparent'},svg)
}
function curveBounds(name,pass){let lo=Infinity,hi=-Infinity;Object.values(state.dataset.curves).filter(c=>c.name===name).forEach(c=>c.samples.forEach(s=>{lo=Math.min(lo,s.value);hi=Math.max(hi,s.value)}));return[lo,hi]}
function drawLaneCurves(svg,name,pass,cx){
 const curve=state.dataset.curves[`${name}:${pass}`];const[lo,hi]=curveBounds(name,pass);
 el('text',{x:cx,y:44,'text-anchor':'middle',class:'axis-label'},svg).textContent=name==='gamma'?'伽马':'电阻率';
 curve.nodata.forEach(g=>el('rect',{class:'nodata',x:cx-58,y:yScale(g.from,pass),width:116,height:Math.max(1,yScale(g.to,pass)-yScale(g.from,pass))},svg));
 const pts=curve.samples.map(s=>{const y=yScale(s.depth,pass),x=cx+(s.value-lo)/(hi-lo||1)*100-50;return`${fmt(x,1)},${fmt(y,1)}`}).join(' ');
 el('polyline',{points:pts,class:'curve-'+name},svg);
 el('line',{x1:cx-55,y1:geom.bottom+12,x2:cx+55,y2:geom.bottom+12,stroke:'#d0d5dd'},svg);el('text',{x:cx,y:geom.bottom+28,'text-anchor':'middle',class:'axis-label'},svg).textContent=fmt(lo,0)+'–'+fmt(hi,0)
}
function drawMapping(svg){
 const c=currentCandidate();if(!c)return;
 const gapKeys=new Set((c.nodata_edges||[]).map(g=>g.from.toFixed(2)+'-'+g.to.toFixed(2)));
 const cfg=result().config;
 for(let i=0;i<c.points.length-1;i++){
  const p=c.points[i],q=c.points[i+1],s=(q.to-p.to)/(q.from-p.from);
  const key=p.from.toFixed(2)+'-'+q.from.toFixed(2),inGap=gapKeys.has(key);
  const stroke=inGap?'#9ca3af':(s<cfg.min_stretch||s>cfg.max_stretch)?'#dc2626':'#2563eb';
  el('line',{class:'mapping',x1:570,y1:yScale(p.from,'A'),x2:690,y2:yScale(q.to,'B'),stroke,'stroke-opacity':inGap?1:.72},svg)
 }
 c.violations.forEach(v=>el('circle',{cx:630,cy:yScale((v.from+v.to)/2,'A'),r:4,fill:'#dc2626'},svg));
}
function drawMarkers(svg){
 state.dataset.markers.forEach(m=>{const yA=yScale(m.a_from,'A'),yB=yScale(m.b_to,'B'),cls=m.kind==='soft'?'soft':'',bad=isConflictMarker(m.id)?'bad':m.kind==='soft'?'warn':'ok';if(m.active)el('line',{x1:306,y1:yA,x2:694,y2:yB,stroke:m.kind==='soft'?'#d97706':'#475467','stroke-dasharray':m.kind==='soft'?'4 4':'',opacity:.62},svg);markerTriangle(svg,306,yA,m,bad,cls,'A');markerTriangle(svg,694,yB,m,bad,cls,'B');el('text',{x:m.a_from?292:292,y:yA-7,'text-anchor':'end',class:'marker-label',fill:bad==='bad'?'#dc2626':'#344054'},svg).textContent=m.label;el('text',{x:708,y:yB-7,class:'marker-label',fill:bad==='bad'?'#dc2626':'#344054'},svg).textContent=m.label})
}
function markerTriangle(svg,x,y,m,bad,cls,pass){const p=el('polygon',{class:'marker-handle '+cls,points:pass==='A'?`${x},${y-7} ${x+12},${y} ${x},${y+7}`:`${x},${y-7} ${x-12},${y} ${x},${y+7}`,fill:m.active?(bad==='bad'?'#dc2626':m.kind==='soft'?'#f59e0b':'#2563eb'):'#cbd5e1',stroke:bad==='bad'?'#7f1d1d':'#334155','stroke-width':1.5},svg);p.dataset.id=m.id;p.dataset.pass=pass;p.addEventListener('pointerdown',startDrag)}
function svgPoint(evt){const pt=$('chart').createSVGPoint();pt.x=evt.clientX;pt.y=evt.clientY;return pt.matrixTransform($('chart').getScreenCTM().inverse())}
function startDrag(evt){
 const shape=evt.currentTarget,id=shape.dataset.id,pass=shape.dataset.pass;evt.preventDefault();
 const marker=state.dataset.markers.find(m=>m.id===id);
 function move(ev){const p=svgPoint(ev),min=depthMin(pass),max=depthMax(pass),depth=min+(p.y-geom.top)/(geom.bottom-geom.top)*(max-min);const clamped=Math.max(min,Math.min(max,depth));const rounded=Math.round(clamped*10)/10;if(pass==='A')marker.a_from=rounded;else marker.b_to=rounded;render()}
 async function up(){window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',up);await saveMarker(marker)}
 window.addEventListener('pointermove',move);window.addEventListener('pointerup',up,{once:true})
}
async function api(path,method='GET',body){const opt={method,headers:{'Content-Type':'application/json'}};if(body)opt.body=JSON.stringify(body);const res=await fetch(path,opt);const data=await res.json();if(!res.ok)throw new Error(data.error||res.statusText);return data}
async function refresh(autoSolve=false){const st=await api('/api/state');state.dataset=st.dataset;state.latest=st.latest;state.selected=null;if(autoSolve&&!state.latest){state.latest=await api('/api/solve','POST',currentConfig())}render();await renderRuns()}
async function saveMarker(marker){try{await api('/api/markers','POST',marker);const st=await api('/api/state');state.dataset=st.dataset;state.latest=await api('/api/solve','POST',currentConfig());state.selected=null;render();await renderRuns()}catch(e){alert(e.message);await refresh()}}
async function upsert(marker,patch){const next={...marker,...patch};await saveMarker(next)}
async function toggleMarker(id,active){await api('/api/markers/toggle','POST',{id,active});const st=await api('/api/state');state.dataset=st.dataset;state.latest=await api('/api/solve','POST',currentConfig());state.selected=null;render();await renderRuns()}
function currentConfig(){return{min_stretch:parseFloat($('minStretch').value),max_stretch:parseFloat($('maxStretch').value),soft_weight:2}}
async function solve(){try{state.latest=await api('/api/solve','POST',currentConfig());state.dataset=state.dataset||(await api('/api/state')).dataset;state.selected=null;render()}catch(e){alert(e.message)}}
async function resetRefresh(){await api('/api/reset','POST');await refresh(true)}
$('solveBtn').onclick=solve;$('resetBtn').onclick=async()=>{if(confirm('清空数据库并重新导入固定 fixture？')){await resetRefresh()}};
$('exportBtn').onclick=()=>window.location='/api/export';
$('importBtn').onclick=()=>$('importFile').click();
$('importFile').onchange=async e=>{const text=await e.target.files[0].text();await api('/api/import','POST',JSON.parse(text));await refresh(true)};
$('replayBtn').onclick=async()=>{const r=await api('/api/replay','POST');alert('已重放 '+r.operations+' 条操作');await refresh()};
async function init(){const st=await api('/api/state');state.dataset=st.dataset;state.latest=st.latest;if(!state.latest){state.latest=await api('/api/solve','POST',currentConfig())}state.selected=null;render();await renderRuns()}
init();
