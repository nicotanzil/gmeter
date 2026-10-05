'use strict';

const $ = id => document.getElementById(id);
const token = document.querySelector('meta[name="gmeter-token"]').content;
let examples = [], library = [], runs = [], current = null, selectedScenario = '', activeID = '', view = 'build';
let selectedRuns = new Set(), timer = null, polling = false;
const fmt = (value, digits = 1) => Number(value || 0).toLocaleString(undefined, {maximumFractionDigits: digits, minimumFractionDigits: digits});
const count = value => Number(value || 0).toLocaleString();
const active = run => run && ['running', 'stopping'].includes(run.status);
const node = (tag, text, className) => { const e = document.createElement(tag); if (text !== undefined) e.textContent = text; if (className) e.className = className; return e; };

async function api(path, method = 'GET', body) {
  const options = {method, headers: {'X-Gmeter-Token': token}};
  if (body !== undefined) { options.headers['Content-Type'] = 'application/json'; options.body = typeof body === 'string' ? body : JSON.stringify(body); }
  const response = await fetch(path, options);
  if (!response.ok) {
    let error; try { error = (await response.json()).error; } catch { error = `Request failed (${response.status}).`; }
    throw new Error(error);
  }
  return response;
}

function notice(message, bad = false) { $('notice').textContent = message; $('notice').className = bad ? 'bad' : ''; $('notice').hidden = false; }
function validateMessage(message, kind = '') { $('validation').textContent = message; $('validation').className = `validation ${kind}`; }
function showView(next) {
  view = next;
  for (const name of ['build', 'live', 'history']) $('view-' + name).hidden = name !== next;
  for (const e of document.querySelectorAll('.nav')) e.classList.toggle('active', e.dataset.view === next);
  window.scrollTo(0, 0);
  if (next === 'history') refresh().catch(e => notice(e.message, true));
  if (next === 'live' && current) requestAnimationFrame(drawCharts);
}
for (const e of document.querySelectorAll('[data-view]')) e.addEventListener('click', () => showView(e.dataset.view));

function readEditor() {
  const c = JSON.parse($('config-json').value);
  if (!c || Array.isArray(c) || typeof c !== 'object') throw new Error('The configuration must be a JSON object.');
  return c;
}
function setEditor(config, id = '') {
  selectedScenario = id;
  $('config-json').value = JSON.stringify(config, null, 2);
  syncForm(config);
  validateMessage('Configuration changed. Validate or start a run.');
  $('save-scenario').textContent = id ? 'Update scenario' : 'Save scenario';
  $('save-copy').hidden = !id;
}
function syncForm(c) {
  const journey = !!c.scenarios?.length, stages = !!c.stages?.length, rateMode = (c.mode || 'users') === 'rate';
  $('name').value = c.name || 'HTTP test'; $('url').value = c.url || '';
  $('mode').value = c.mode || 'users';
  const method = c.method || 'GET';
  if (![...$('method').options].some(o => o.value === method)) $('method').append(node('option', method));
  $('method').value = method;
  $('rate').value = c.rate || 20; $('vus').value = c.vus ?? 10;
  $('capacity').value = c.max_in_flight ?? 100; $('duration').value = c.duration || '10s';
  $('timeout').value = c.timeout || '5s'; $('distribution').value = c.arrival_distribution || 'constant';
  $('diagnostics').value = c.diagnostics === false ? 'false' : 'true';
  $('workers').value = (c.workers || []).join('\n');
  $('rate-label').textContent = journey ? 'Journeys / second' : 'Requests / second';
  $('url-hint').textContent = journey ? 'Base origin for every step in your journeys.' : 'Full HTTP or HTTPS endpoint.';
  $('method-field').hidden = journey; $('rate-field').hidden = !rateMode; $('vus-field').hidden = rateMode;
  $('capacity-field').hidden = !rateMode; $('distribution-field').hidden = !rateMode;
  $('duration').disabled = stages; $('rate').disabled = stages; $('stages-hint').hidden = !stages;
  $('phase-summary').hidden = !stages; $('single-phase').hidden = !stages;
  $('phase-summary').replaceChildren(...(c.stages || []).map(s => node('span', `${s.name || 'Phase'} · ${s.rate}/s · ${s.duration}`, 'phase-chip')));
}

$('workers').addEventListener('input',()=>{
  try {
    const c=readEditor(), workers=$('workers').value.split(/[\n,]/).map(x=>x.trim()).filter(Boolean);
    if (workers.length) c.workers=workers; else delete c.workers;
    $('config-json').value=JSON.stringify(c,null,2); validateMessage('Configuration changed. Validate or start a run.');
  } catch { validateMessage('Fix the scenario JSON before changing workers.','bad'); }
});

const fields = {name: 'name', url: 'url', method: 'method', rate: 'rate', vus: 'vus', capacity: 'max_in_flight', duration: 'duration', timeout: 'timeout', distribution: 'arrival_distribution', diagnostics:'diagnostics'};
for (const [id, key] of Object.entries(fields)) $(id).addEventListener('input', () => {
  try {
    const c = readEditor(); c[key] = id==='diagnostics' ? $(id).value==='true' : (['rate', 'vus', 'capacity'].includes(id) ? Number($(id).value) : $(id).value);
    $('config-json').value = JSON.stringify(c, null, 2); validateMessage('Configuration changed. Validate or start a run.');
  } catch { validateMessage('Fix the scenario JSON before changing the form fields.', 'bad'); }
});
$('mode').addEventListener('change', () => {
  try {
    const c = readEditor(); c.mode = $('mode').value;
    if (c.mode === 'users') { c.rate = 0; delete c.stages; c.arrival_distribution = 'constant'; }
    else { c.rate = c.rate || 20; delete c.think_time; }
    $('config-json').value = JSON.stringify(c, null, 2); syncForm(c); validateMessage('Configuration changed. Validate or start a run.');
  } catch (e) { validateMessage(e.message, 'bad'); }
});
$('config-json').addEventListener('input', () => {
  try { syncForm(readEditor()); validateMessage('Configuration changed. Validate or start a run.'); }
  catch (e) { validateMessage('Invalid JSON: ' + e.message, 'bad'); }
});
$('single-phase').addEventListener('click', () => { try { const c = readEditor(); delete c.stages; c.rate = 20; setEditor(c, selectedScenario); } catch (e) { notice(e.message, true); } });
$('starter').addEventListener('change', () => {
  const selected = examples.find(e => e.id === $('starter').value);
  if (selected) { setEditor(selected.config); $('starter').value = ''; notice('Example loaded. The examples target the local demo server on port 8787.'); }
});
$('validate').addEventListener('click', async () => {
  try {
    const data = await (await api('/api/validate', 'POST', readEditor())).json();
    validateMessage(`Valid · ${fmt(data.load_seconds)}s load · ${data.phase_count} phase(s) · ${data.workload} workload`, 'good');
  } catch (e) { validateMessage(e.message, 'bad'); }
});
$('config-form').addEventListener('submit', async e => {
  e.preventDefault(); $('start').disabled = true;
  try {
    const run = await (await api('/api/runs', 'POST', readEditor())).json();
    activeID = run.id; await loadRun(run.id); showView('live'); schedulePoll();
    notice('Run started. You can change views or reload this page while it continues.');
  } catch (error) { validateMessage(error.message, 'bad'); notice(error.message, true); }
  finally { $('start').disabled = !!activeID; }
});
$('stop').addEventListener('click', async () => {
  $('stop').disabled = true;
  try { await api(`/api/runs/${activeID}/stop`, 'POST'); notice('Stopping new work and canceling active requests. Partial results will be saved.'); await poll(); }
  catch (e) { notice(e.message, true); }
  finally { $('stop').disabled = current?.status === 'stopping'; }
});
$('view-active').addEventListener('click', async () => {
  try { if (activeID) { await loadRun(activeID); showView('live'); } } catch (e) { notice(e.message, true); }
});

async function saveScenario(copy = false) {
  const id = copy ? '' : selectedScenario;
  try {
    const saved = await (await api(id ? `/api/scenarios/${id}` : '/api/scenarios', id ? 'PUT' : 'POST', readEditor())).json();
    selectedScenario = saved.id; $('save-scenario').textContent = 'Update scenario'; $('save-copy').hidden = false;
    await refresh(); notice('Scenario saved to the local library.');
  } catch (e) { notice(e.message, true); }
}
$('save-scenario').addEventListener('click', () => saveScenario());
$('save-copy').addEventListener('click', () => saveScenario(true));
$('export-config').addEventListener('click', async () => {
  try { const exported = await (await api('/api/config-export', 'POST', readEditor())).json(); downloadURL(exported.path, 'gmeter-scenario.json'); }
  catch (e) { notice(e.message, true); }
});
$('import-config').addEventListener('click', () => $('file-input').click());
$('import-history').addEventListener('click', () => $('file-input').click());
$('file-input').addEventListener('change', async () => {
  const file = $('file-input').files[0]; if (!file) return;
  try {
    if (file.size > 16777216) throw new Error('Import exceeds 16 MiB.');
    const text = await file.text(), parsed = JSON.parse(text);
    if (parsed?.result && parsed?.config) {
      const imported = await (await api('/api/runs/import', 'POST', text)).json();
      await refresh(); await loadRun(imported.id); showView('live'); notice('Saved run imported. Its traffic measurements have not been independently verified.'); return;
    }
    if (file.size > 1048576) throw new Error('Configuration exceeds 1 MiB.');
    const data = await (await api('/api/validate', 'POST', text)).json();
    setEditor(data.config); validateMessage(`Imported and validated · ${data.workload} workload`, 'good'); notice('Scenario imported into the editor. Save it to keep it in your library.');
  } catch (e) { notice(e.message, true); }
  finally { $('file-input').value = ''; }
});

async function refresh() {
  const state = await (await api('/api/state')).json(); library = state.scenarios; runs = state.runs;
  renderLibrary(); renderHistory();
  const running = state.current && active(state.current);
  activeID = running ? state.current.id : '';
  $('start').disabled = !!activeID; $('live-dot').hidden = !activeID;
  if (activeID && !active(current)) { await loadRun(activeID); schedulePoll(); }
  if (current?.id === state.current?.id && active(current) && !running) await loadRun(current.id);
  if (current) renderRun();
}
function renderLibrary() {
  $('library-count').textContent = library.length;
  if (!library.length) { $('library').replaceChildren(node('div', 'Save a configuration to reuse it here.', 'empty-small')); return; }
  $('library').replaceChildren(...library.map(s => {
    const item = node('div', undefined, 'library-item'), button = node('button', s.name || 'HTTP test');
    button.type = 'button'; button.addEventListener('click', async () => {
      try { const saved = await (await api(`/api/scenarios/${s.id}`)).json(); setEditor(saved.config, s.id); showView('build'); notice('Saved scenario loaded into the editor.'); } catch (e) { notice(e.message, true); }
    });
    const remove = node('button', 'Delete', 'text-button'); remove.type = 'button';
    remove.addEventListener('click', async () => {
      if (!confirm('Delete this saved scenario? Run results will remain.')) return;
      try { await api(`/api/scenarios/${s.id}`, 'DELETE'); if (selectedScenario === s.id) setEditor(readEditor()); await refresh(); notice('Scenario deleted.'); } catch (e) { notice(e.message, true); }
    });
    item.append(button, node('small', `${s.workload === 'journeys' ? 'Journeys' : 'Requests'} · ${s.mode} mode`), remove); return item;
  }));
}

async function loadRun(id) { current = await (await api(`/api/runs/${id}`)).json(); renderRun(); }
function schedulePoll() { clearTimeout(timer); if (activeID) timer = setTimeout(poll, 1000); }
async function poll() {
  if (polling || !activeID) return;
  polling = true;
  try {
    const runningID = activeID;
    const run = await (await api(`/api/runs/${runningID}`)).json();
    // Keep a saved run selected while another run is active.
    if (!current || current.id === runningID) { current = run; renderRun(); }
    if (!active(run)) {
      activeID = ''; $('start').disabled = false; $('live-dot').hidden = true;
      await refresh(); notice(run.save_error || run.error || `Run ${run.status}. Results are saved locally.`, !!run.save_error || !!run.error);
    }
  } catch (e) { notice('Live connection interrupted: ' + e.message + ' The run continues while the dashboard server is running.', true); }
  finally { polling = false; schedulePoll(); }
}
function renderRun() {
  if (!current) return;
  const run = current, latest = run.samples.at(-1) || {}, r = run.result || latest.requests || {}, j = run.result?.journeys || latest.journeys || {};
  const journeys = run.workload === 'journeys', traffic = journeys ? j : r;
  $('live-empty').hidden = true; $('run-content').hidden = false;
  $('run-title').textContent = run.name || 'HTTP test'; $('run-target').textContent = run.config.url;
  $('live-eyebrow').textContent = `${run.workload.toUpperCase()} WORKLOAD · ${run.config.mode.toUpperCase()} MODE`;
  if (run.imported) $('live-eyebrow').textContent += ' · IMPORTED';
  if (run.config.workers?.length) $('live-eyebrow').textContent += ` · ${run.config.workers.length} WORKERS`;
  $('run-status').textContent = run.status.toUpperCase(); $('run-status').className = 'status ' + run.status;
  $('stop').hidden = !activeID || run.id !== activeID; $('stop').disabled = run.status === 'stopping';
  $('view-active').hidden = !activeID || run.id === activeID;
  $('phase-name').textContent = latest.phase ? `${latest.draining && active(run) ? 'Draining · ' : ''}${latest.phase}` : 'Preparing';
  $('elapsed').textContent = `${fmt(latest.load_seconds)} / ${fmt(run.planned_seconds)}s load${latest.elapsed_seconds > latest.load_seconds + .05 ? ` · ${fmt(latest.elapsed_seconds - latest.load_seconds)}s ${run.config.workers?.length ? 'drain + collection' : 'drain'}` : ''}`;
  $('progress').max = run.planned_seconds || 1; $('progress').value = latest.load_seconds || 0;
  $('metric-rps').textContent = fmt(r.started_rps); $('metric-requests').textContent = `${count(r.started)} HTTP attempts · ${count(r.in_flight)} in flight`;
  $('metric-latency').textContent = r.latency?.samples ? fmt(r.latency.p95_ms) + ' ms' : '—';
  $('metric-errors').textContent = fmt(r.error_percent, 2) + '%'; $('metric-errors').classList.toggle('bad', !!r.failed);
  $('metric-failed').textContent = `${count(r.failed)} / ${count(r.completed)} completed attempts`;
  $('metric-drops').textContent = fmt(traffic.drop_percent, 2) + '%'; $('metric-drops').classList.toggle('bad', !!(traffic.dropped_capacity || traffic.dropped_scheduler));
  $('metric-missed').textContent = `${count((traffic.dropped_capacity || 0) + (traffic.dropped_scheduler || 0))} / ${count(traffic.scheduled)} ${run.workload}`;
  $('journey-summary').hidden = !journeys; $('journey-key').hidden = !journeys;
  $('journey-summary').replaceChildren(...[`${fmt(j.started_per_second)} journeys/s`, `${count(j.completed)} completed journeys`, `Journey p95 ${j.latency?.samples ? fmt(j.latency.p95_ms) + ' ms' : '—'}`, `${fmt(j.error_percent, 2)}% journey errors`].map(t => node('span', t)));
  $('traffic-unit').textContent = journeys ? 'HTTP req/s + journeys/s' : 'req/s'; $('phase-unit').textContent = run.config.mode === 'users' ? 'concurrent users' : (journeys ? 'journeys/s' : 'requests/s');
  $('results-panel').hidden = !run.result;
  if (run.result) renderResults(run.result);
  renderDiagnostics(run, latest, r, traffic);
  renderWorkers(run,latest);
  if (run.error || run.save_error) notice(run.save_error || run.error, true);
  if (view === 'live') requestAnimationFrame(drawCharts);
}
function renderWorkers(run,latest) {
  const urls=run.config.workers||[], distribution=run.result?.distribution;
  $('workers-panel').hidden=!urls.length;
  if (!urls.length) return;
  const finished=!!run.result;
  $('workers-state').textContent=distribution ? (distribution.complete?'COMPLETE':'INCOMPLETE') : (latest.workers?.length?'COORDINATING':'PREPARING');
  $('workers-warning').hidden=!distribution || distribution.complete;
  $('workers-warning').textContent=distribution ? `${distribution.failure||'Worker measurements are incomplete.'} ${count(distribution.unreported_planned_arrivals)} planned arrivals remain unreported. Global counts include only workers with final results.` : '';
  const rows=urls.map((url,index)=>{
    const w=distribution?.workers.find(x=>x.index===index), point=latest.workers?.find(x=>x.index===index);
    const journeys=run.workload==='journeys', a=w ? (journeys?w.journeys:w.requests) : null;
    const known=!finished || ['finished','interrupted'].includes(w?.status);
    const cpu=w?.generator?.cpu_available ? fmt(w.generator.average_cpu_percent)+'%' : '—';
    const heap=w?.generator ? fmt(w.generator.peak_heap_bytes/1048576)+' MiB' : '—';
    const label=node('div',`${index+1}. ${w?.name||url}`); if (w) label.append(node('div',w.url,'helper'));
    return [label,w?.status||point?.status||'preparing',w ? count(w.planned_arrivals) : '—',known ? count(a?.scheduled??point?.scheduled) : 'Unknown',known ? count(w?.requests.completed??point?.http_completed) : 'Unknown',cpu,heap];
  });
  $('workers-table').replaceChildren(makeTable(['Worker','Status','Assigned arrivals','Reported arrivals','HTTP completed','CPU avg','Heap peak'],rows));
  const facts=distribution ? [`${count(distribution.planned_arrivals)} planned ${run.workload}`,`Concurrency cap: ${run.config.max_in_flight} across ${urls.length} workers`,...distribution.workers.map(w=>`${w.name||w.url}: cap ${w.capacity} · probe RTT ${fmt(w.round_trip_ms,2)} ms · clock offset ${fmt(w.clock_offset_ms,2)} ms`)] : [latest.workers?.length ? 'Latest received worker counters. Allocation details and resource summaries appear after final collection.' : 'Reserving all workers before committing a shared start.'];
  $('workers-facts').replaceChildren(...facts.map(x=>node('span',x)));
}
function renderDiagnostics(run, latest, request, traffic) {
  const summary=run.result?.generator, resources=summary?.final || latest.generator, http=request.http;
  $('generator-panel').hidden=!resources; $('http-panel').hidden=!http;
  $('diagnostics-empty').hidden=!!(resources||http) || active(run);
  if (resources) {
    const cpuAvailable=summary ? summary.cpu_available : resources.cpu_available;
    $('cpu-label').textContent=summary ? 'Process CPU · run average' : 'Process CPU · last interval';
    $('generator-cpu').textContent=cpuAvailable ? fmt(summary ? summary.average_cpu_percent : resources.cpu_percent)+'%' : 'Unavailable';
    $('generator-cpu-note').textContent=summary&&cpuAvailable ? `100% = one core · peak interval ${fmt(summary.peak_cpu_percent)}%` : '100% equals one CPU core';
    $('generator-heap').textContent=fmt(resources.heap_bytes/1048576)+' MiB';
    $('generator-heap-note').textContent=summary ? `Peak sampled ${fmt(summary.peak_heap_bytes/1048576)} MiB` : 'Includes objects awaiting GC';
    $('generator-rss').textContent=resources.rss_available ? fmt(resources.process_peak_rss_bytes/1048576)+' MiB' : 'Unavailable';
    $('generator-goroutines').textContent=count(resources.goroutines);
    $('generator-goroutines-note').textContent=summary ? `Peak sampled ${count(summary.peak_goroutines)}` : 'Process-wide, sampled';
    const facts=[`GC cycles: ${count(resources.gc_cycles)}`, `GC pause: ${fmt(resources.gc_pause_ms,2)} ms`, `Arrival start delay p95: ${fmt(traffic.start_delay?.p95_ms,2)} ms`, `Missed: ${count(traffic.dropped_capacity)} capacity / ${count(traffic.dropped_scheduler)} scheduler`, `GOMAXPROCS: ${resources.gomaxprocs}`];
    if (summary) facts.push(summary.platform,summary.go_version,`${summary.logical_cpus} logical CPUs`,`${summary.samples} resource samples`);
    $('generator-facts').replaceChildren(...facts.map(text=>node('span',text)));
  }
  if (http) {
    const labels=[['Connection acquisition','connection_wait'],['DNS lookup','dns'],['TCP connect','connect'],['TLS handshake','tls'],['Request write','write'],['First header byte from attempt start','first_byte'],['Response body read','body_read']];
    const rows=labels.map(([name,key])=>{const value=http[key]; return [name,count(value.samples),...['mean_ms','p50_ms','p95_ms','max_ms'].map(field=>value.samples ? fmt(value[field],2) : '—')];});
    $('http-timings').replaceChildren(makeTable(['Observed interval','Samples','Mean','p50','p95','Max'],rows));
    const facts=[`${count(http.connections_acquired)} acquired / ${count(http.reused_connections)} reused`,`${count(http.connection_requests)} acquisition requests`,`${count(http.idle_reuses)} idle reuses`,`${count(http.dial_attempts)} TCP dials / ${count(http.dial_failures)} failed`,`${count(http.dns_lookups)} DNS lookups / ${count(http.dns_failures)} failed`,`${count(http.tls_handshakes)} TLS handshakes / ${count(http.tls_failures)} failed`,`${count(http.write_completions)} write completions / ${count(http.write_failures)} failed`,`${count(http.body_read_failures)} body read failures`,`${count(http.unfinished_events)} unfinished pending events`,...Object.entries(http.protocols||{}).map(([protocol,n])=>`${protocol}: ${count(n)} response headers`)];
    $('connection-facts').replaceChildren(...facts.map(text=>node('span',text)));
  }
}
function renderResults(result) {
  $('checks').replaceChildren(...result.checks.map(c => node('span', `${c.passed ? '✓' : '×'} ${c.name.replaceAll('_', ' ')} · ${fmt(c.actual, 2)} / ${fmt(c.limit, 2)} ${c.unit}`, `check-item${c.passed ? '' : ' bad'}`)));
  $('scenario-results').replaceChildren(...(result.scenarios || []).map(s => {
    const section = node('div', undefined, 'step-section');
    section.append(node('h3', s.name), node('p', `${count(s.journeys.completed)} journeys · ${fmt(s.journeys.error_percent, 2)}% errors · ${s.journeys.latency.samples ? fmt(s.journeys.latency.p95_ms) + ' ms p95' : 'no latency samples'} · ${s.checks?.length ? (s.passed ? 'checks passed' : 'checks failed') : 'no scenario checks'}`));
    const rows = s.steps.map(step => [step.name, step.method, count(step.completed), count(step.skipped), fmt(step.error_percent, 2) + '%', step.latency.samples ? fmt(step.latency.p95_ms) + ' ms' : '—']);
    const wrap = node('div', undefined, 'table-wrap'); wrap.append(makeTable(['Step', 'Method', 'Completed', 'Skipped', 'Errors', 'p95'], rows)); section.append(wrap);
    if (s.checks?.length) section.append(...s.checks.map(c => node('span', `${c.passed ? '✓' : '×'} ${c.name.replaceAll('_', ' ')} · ${fmt(c.actual, 2)} / ${fmt(c.limit, 2)} ${c.unit}`, `check-item${c.passed ? '' : ' bad'}`)));
    return section;
  }));
}
for (const [id, suffix, extension] of [['download-json', 'report.json', 'json'], ['download-html', 'report.html', 'html'], ['download-run', 'export', 'json']]) $(id).addEventListener('click', async () => {
  downloadURL(`/download/${current.id}/${suffix}`, `gmeter-${suffix === 'export' ? 'run-' : ''}${current.id}.${extension}`);
});
function downloadURL(path, filename) {
  const a = node('a'); a.href = path; a.download = filename;
  document.body.append(a); a.click(); a.remove();
}

// Counter deltas are interval measurements. Histogram quantiles are cumulative.
function chartData(run) {
  const values = {traffic: [], journeys: [], target: [], p95: [], p50: [], errors: [], drops: []};
  run.samples.forEach((s, index) => {
    const x = s.elapsed_seconds, prev = run.samples[index - 1], r = s.requests || {}, traffic = run.workload === 'journeys' ? (s.journeys || {}) : r;
    const dt = prev ? x - prev.elapsed_seconds : 0, pr = prev?.requests || {}, pt = run.workload === 'journeys' ? (prev?.journeys || {}) : pr;
    if (dt > 0) {
      const completed = (r.completed || 0) - (pr.completed || 0), scheduled = (traffic.scheduled || 0) - (pt.scheduled || 0);
      values.traffic.push([x, Math.max(0, ((r.started || 0) - (pr.started || 0)) / dt)]);
      values.journeys.push([x, Math.max(0, ((s.journeys?.started || 0) - (prev.journeys?.started || 0)) / dt)]);
      values.errors.push([x, completed > 0 ? 100 * ((r.failed || 0) - (pr.failed || 0)) / completed : null]);
      const drops = (traffic.dropped_capacity || 0) + (traffic.dropped_scheduler || 0) - (pt.dropped_capacity || 0) - (pt.dropped_scheduler || 0);
      values.drops.push([x, scheduled > 0 ? 100 * drops / scheduled : null]);
    }
    values.target.push([x, run.config.mode === 'rate' ? (s.draining ? 0 : s.target_rate) : null]);
    values.p95.push([x, r.latency?.samples ? r.latency.p95_ms : null]); values.p50.push([x, r.latency?.samples ? r.latency.p50_ms : null]);
  });
  return values;
}
const colors = {teal: '#087f75', blue: '#527bca', gray: '#94a5af', red: '#bd3c45', amber: '#b27b14'};
function drawChart(id, series, xEnd, fixedMax, xStart = 0) {
  const canvas = $(id), width = canvas.clientWidth, height = canvas.clientHeight;
  if (!width) return;
  const ratio = window.devicePixelRatio || 1; canvas.width = Math.round(width * ratio); canvas.height = Math.round(height * ratio);
  const ctx = canvas.getContext('2d'); ctx.scale(ratio, ratio);
  const left = 42, right = width - 12, top = 12, bottom = height - 25;
  let max = fixedMax || Math.max(1, ...series.flatMap(s => s.points.map(p => Number(p[1]) || 0))) * 1.15;
  ctx.font = '10px -apple-system, sans-serif'; ctx.lineWidth = 1;
  for (let i = 0; i <= 4; i++) {
    const y = bottom - (bottom - top) * i / 4;
    ctx.strokeStyle = '#eaf0f3'; ctx.beginPath(); ctx.moveTo(left, y); ctx.lineTo(right, y); ctx.stroke();
    ctx.fillStyle = '#84969e'; ctx.textAlign = 'right'; ctx.fillText(fmt(max * i / 4, max < 5 ? 1 : 0), left - 8, y + 3);
  }
  xEnd = Math.max(xStart + 1, xEnd);
  const X = x => left + (x - xStart) / (xEnd - xStart) * (right - left), Y = y => bottom - Math.max(0, y) / max * (bottom - top);
  ctx.fillStyle = '#84969e'; ctx.textAlign = 'center';
  for (let i = 0; i <= 4; i++) { const x = xStart + (xEnd - xStart) * i / 4; ctx.fillText(fmt(x, xEnd > 10 ? 0 : 1) + 's', X(x), height - 6); }
  let visiblePoints = 0;
  for (const s of series) {
    ctx.strokeStyle = s.color; ctx.lineWidth = 2; ctx.setLineDash(s.dashed ? [4, 4] : []); ctx.beginPath(); let previous = null;
    for (const point of s.points) {
      if (point[1] === null || point[1] === undefined) { previous = null; continue; }
      visiblePoints++;
      if (previous) { if (s.step) ctx.lineTo(X(point[0]), Y(previous[1])); ctx.lineTo(X(point[0]), Y(point[1])); }
      else ctx.moveTo(X(point[0]), Y(point[1]));
      previous = point;
    }
    ctx.stroke(); ctx.setLineDash([]);
    const available = s.points.filter(p => p[1] !== null && p[1] !== undefined);
    if (available.length === 1) { ctx.beginPath(); ctx.arc(X(available[0][0]), Y(available[0][1]), 3, 0, Math.PI * 2); ctx.fillStyle = s.color; ctx.fill(); }
  }
  if (!visiblePoints) { ctx.fillStyle = '#8a9da4'; ctx.textAlign = 'center'; ctx.fillText('Waiting for measurements', (left + right) / 2, (top + bottom) / 2); }
}
function duration(value) {
  const match = String(value).matchAll(/([0-9]*\.?[0-9]+)(ns|us|µs|μs|ms|s|m|h)/g);
  let seconds = 0; for (const m of match) seconds += Number(m[1]) * ({ns: 1e-9, us: 1e-6, 'µs': 1e-6, 'μs': 1e-6, ms: .001, s: 1, m: 60, h: 3600}[m[2]]); return seconds;
}
function drawCharts() {
  if (!current || view !== 'live') return;
  const v = chartData(current), end = current.samples.at(-1)?.elapsed_seconds || 1, start = current.samples[0]?.elapsed_seconds || 0;
  const traffic = [{points: v.traffic, color: colors.teal}, {points: v.target, color: colors.gray, dashed: true, step: true}];
  if (current.workload === 'journeys') traffic.push({points: v.journeys, color: colors.blue});
  drawChart('traffic-chart', traffic, end, null, start);
  drawChart('latency-chart', [{points: v.p95, color: colors.teal}, {points: v.p50, color: colors.blue}], end, null, start);
  drawChart('errors-chart', [{points: v.errors, color: colors.red}, {points: v.drops, color: colors.amber}], end, 100, start);
  const phases = current.config.stages?.length ? current.config.stages : [{name: current.config.name, duration: current.config.duration, rate: current.config.rate}];
  let offset = 0, phasePoints = [];
  $('phase-list').replaceChildren(...phases.map(p => {
    const d = duration(p.duration), target = current.config.mode === 'rate' ? (p.rate || 0) : current.config.vus;
    phasePoints.push([offset, target], [offset + d, target]);
    offset += d; return node('span', `${p.name || 'Phase'} · ${current.config.mode === 'rate' ? (p.rate || 0) + '/s' : current.config.vus + ' users'} · ${p.duration}`);
  }));
  drawChart('phase-chart', [{points: phasePoints, color: colors.teal, step: true}], current.planned_seconds || offset || 1);
  if (!$('generator-panel').hidden) {
    const cpu=[],heap=[],memory=[];
    for (const sample of current.samples) {
      const r=sample.generator;
      cpu.push([sample.elapsed_seconds,r?.cpu_available ? r.cpu_percent : null]);
      heap.push([sample.elapsed_seconds,r ? r.heap_bytes/1048576 : null]);
      memory.push([sample.elapsed_seconds,r ? r.runtime_bytes/1048576 : null]);
    }
    drawChart('cpu-chart',[{points:cpu,color:colors.teal}],end,null,start);
    drawChart('memory-chart',[{points:heap,color:colors.teal},{points:memory,color:colors.blue}],end,null,start);
  }
}
window.addEventListener('resize', () => requestAnimationFrame(drawCharts));

function makeTable(headers, rows) {
  const table = node('table'), head = node('thead'), body = node('tbody'), tr = node('tr'); tr.append(...headers.map(t => node('th', t))); head.append(tr);
  for (const row of rows) { const r = node('tr'); for (const value of row) { const td = node('td'); td.append(value instanceof Node ? value : document.createTextNode(String(value))); r.append(td); } body.append(r); }
  table.append(head, body); return table;
}
function renderHistory() {
  selectedRuns = new Set([...selectedRuns].filter(id => runs.some(r => r.id === id)));
  if (!runs.length) { $('history').replaceChildren(node('div', 'Completed and interrupted runs will appear here.', 'empty-small')); updateCompare(); return; }
  const rows = runs.map(r => {
    const checkbox = node('input', undefined, 'history-select'); checkbox.type = 'checkbox'; checkbox.checked = selectedRuns.has(r.id); checkbox.setAttribute('aria-label', 'Select ' + r.name + ' for comparison'); checkbox.disabled = ['running', 'stopping', 'abandoned', 'error'].includes(r.status);
    checkbox.addEventListener('change', () => {
      if (checkbox.checked && selectedRuns.size >= 2) { checkbox.checked = false; notice('Select at most two runs to compare.', true); return; }
      if (checkbox.checked) selectedRuns.add(r.id); else selectedRuns.delete(r.id); updateCompare();
    });
    const label = node('div', r.name || 'HTTP test'); label.append(node('small', new Date(r.created_at).toLocaleString()));
    const actions = node('div');
    const open = node('button', 'View', 'text-button'); open.addEventListener('click', async () => { try { await loadRun(r.id); showView('live'); } catch (e) { notice(e.message, true); } });
    const reuse = node('button', 'Reuse', 'text-button'); reuse.addEventListener('click', async () => { try { const c = await (await api(`/api/runs/${r.id}/config`)).json(); setEditor(c); showView('build'); notice('Run configuration loaded.'); } catch (e) { notice(e.message, true); } });
    const remove = node('button', 'Delete', 'text-button'); remove.disabled = active(r); remove.addEventListener('click', async () => {
      if (!confirm('Delete this saved run and its results? Export it first if you need a copy.')) return;
      try { await api(`/api/runs/${r.id}`, 'DELETE'); if (current?.id === r.id) { current = null; $('live-empty').hidden = false; $('run-content').hidden = true; } await refresh(); notice('Run deleted.'); } catch (e) { notice(e.message, true); }
    });
    actions.append(open, reuse, remove); return [checkbox, label, r.workload, node('span', r.status.toUpperCase(), 'status ' + r.status), actions];
  });
  $('history').replaceChildren(makeTable(['', 'Test', 'Workload', 'Result', 'Actions'], rows)); updateCompare();
}
function updateCompare() { $('compare').disabled = selectedRuns.size !== 2; $('compare').textContent = `Compare selected (${selectedRuns.size}/2)`; }
$('refresh-history').addEventListener('click', () => refresh().catch(e => notice(e.message, true)));
$('compare').addEventListener('click', async () => {
  try {
    const [a, b] = await Promise.all([...selectedRuns].map(async id => (await api(`/api/runs/${id}`)).json()));
    if (!a.result || !b.result) throw new Error('Both runs need final results to compare.');
    const A = a.result, B = b.result, arrivalA = A.workload === 'journeys' ? A.journeys : A, arrivalB = B.workload === 'journeys' ? B.journeys : B;
    const rows = [
      ['Target URL', A.url, B.url, '—'], ['Workload', A.workload, B.workload, '—'], ['Model', A.mode, B.mode, '—'], ['Result', a.status, b.status, '—'],
    ];
    function metric(label, x, y, unit, digits = 1, deltaUnit = unit) {
      const delta = y - x, sign = delta > 0 ? '+' : '', pct = x !== 0 ? ` (${sign}${fmt(100 * delta / Math.abs(x), 1)}%)` : '';
      rows.push([label, fmt(x, digits) + unit, fmt(y, digits) + unit, node('span', sign + fmt(delta, digits) + deltaUnit + pct, 'delta')]);
    }
    metric('Load window', A.load_seconds, B.load_seconds, 's'); metric('Concurrency limit', A.concurrency_limit, B.concurrency_limit, '', 0);
    metric('Started HTTP attempts', A.started, B.started, '', 0); metric('Average started HTTP rate', A.started_rps, B.started_rps, ' req/s');
    if (A.latency.samples && B.latency.samples) metric('Request p95', A.latency.p95_ms, B.latency.p95_ms, ' ms'); else rows.push(['Request p95', A.latency.samples ? fmt(A.latency.p95_ms)+' ms' : 'No samples', B.latency.samples ? fmt(B.latency.p95_ms)+' ms' : 'No samples', '—']);
    metric('Request errors', A.error_percent, B.error_percent, '%', 2, ' percentage points');
    metric('Missed workload demand', arrivalA.drop_percent, arrivalB.drop_percent, '%', 2, ' percentage points');
    if (A.workload === 'journeys' && B.workload === 'journeys') { metric('Journey arrivals', A.journeys.started_per_second, B.journeys.started_per_second, ' journeys/s'); if (A.journeys.latency.samples && B.journeys.latency.samples) metric('Journey p95', A.journeys.latency.p95_ms, B.journeys.latency.p95_ms, ' ms'); metric('Journey errors', A.journeys.error_percent, B.journeys.error_percent, '%', 2, ' percentage points'); }
    if (A.http?.first_byte?.samples && B.http?.first_byte?.samples) metric('First header byte p95',A.http.first_byte.p95_ms,B.http.first_byte.p95_ms,' ms');
    if (A.http?.body_read?.samples && B.http?.body_read?.samples) metric('Body read p95',A.http.body_read.p95_ms,B.http.body_read.p95_ms,' ms');
    if (A.generator && B.generator) {
      if (A.generator.cpu_available && B.generator.cpu_available) metric('Generator average CPU',A.generator.average_cpu_percent,B.generator.average_cpu_percent,'%',1,' percentage points');
      metric('Peak sampled Go heap',A.generator.peak_heap_bytes/1048576,B.generator.peak_heap_bytes/1048576,' MiB');
    }
    // Configuration mismatches are visible; no causal or automatic verdict.
    const mismatches = [];
    for (const [key, label] of [['url', 'target'], ['mode', 'model'], ['rate', 'rate'], ['stages', 'phases'], ['duration', 'planned duration'], ['vus', 'user count'], ['max_in_flight', 'capacity'], ['scenarios', 'journey definitions'], ['method', 'method'], ['body', 'body'], ['headers', 'headers'], ['timeout', 'timeout'], ['think_time', 'think time'], ['arrival_distribution', 'arrival spacing'], ['seed', 'seed'],['workers','worker pool']]) if (JSON.stringify(a.config[key]) !== JSON.stringify(b.config[key])) mismatches.push(label);
    if (A.workload !== B.workload) mismatches.push('arrival unit'); if (A.interrupted || B.interrupted) mismatches.push('partial run');
    const hasDiagnostics=result=>!!(result.generator||result.http||result.distribution?.workers?.some(w=>w.generator||w.requests?.http));
    if (hasDiagnostics(A)!==hasDiagnostics(B)) mismatches.push('diagnostics instrumentation');
    const environment=result=>result.distribution ? result.distribution.workers.filter(w=>w.generator).map(w=>[w.index,w.generator.platform,w.generator.go_version,w.generator.logical_cpus,w.generator.final.gomaxprocs]) : (result.generator ? [[0,result.generator.platform,result.generator.go_version,result.generator.logical_cpus,result.generator.final.gomaxprocs]] : []);
    const envA=environment(A),envB=environment(B);
    if (envA.length && envB.length && JSON.stringify(envA)!==JSON.stringify(envB)) mismatches.push('generator environment');
    $('comparison-warning').hidden = !mismatches.length; $('comparison-warning').textContent = 'Conditions differ: ' + mismatches.join(', ') + '.';
    $('comparison-table').replaceChildren(makeTable(['Measurement', `${a.name} (A)`, `${b.name} (B)`, 'B − A'], rows));
    $('comparison').hidden = false; $('comparison').scrollIntoView({behavior: 'smooth', block: 'start'});
  } catch (e) { notice(e.message, true); }
});

async function initialize() {
  try {
    examples = await (await api('/api/examples')).json();
    $('starter').append(...examples.map(e => { const option = node('option', e.name); option.value = e.id; return option; }));
    setEditor(examples[0].config);
    await refresh();
    if (activeID) { showView('live'); schedulePoll(); }
  } catch (e) { notice('Cannot load the local workspace: ' + e.message, true); }
}
initialize();
