/* LiteAgent 结构图览 · 交互 */

(function () {
  'use strict';

  const en = { 'model-context': true, 'tools': true, 'file-tools': true, 'shell-tools': true };
  const NODE_W = 158, NODE_H = 90;

  const owner = {
    loop: 'agent', agent: 'agent', session: 'session', llm: 'llm-openai',
    'model-context': 'model-context', tools: 'tools', file: 'file-tools', shell: 'shell-tools', host: 'Host',
  };

  const $ = (s, r) => (r || document).querySelector(s);
  const $$ = (s, r) => Array.prototype.slice.call((r || document).querySelectorAll(s));
  const esc = (s) => String(s).replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  const bytes = (s) => new TextEncoder().encode(s).length;
  const hex = (n) => n.toString(16).toUpperCase().padStart(2, '0');
  const reduced = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  function hlJSON(v) {
    const src = typeof v === 'string' ? v : JSON.stringify(v, null, 2);
    return esc(src).replace(
      /("(?:\\.|[^"\\])*"\s*:?|\b-?\d+(?:\.\d+)?\b|\btrue\b|\bfalse\b|\bnull\b)/g,
      (m) => {
        let cls = 'json-num';
        if (m[0] === '"') cls = /"\s*:$/.test(m) ? 'json-key' : 'json-str';
        else if (m === 'true' || m === 'false') cls = 'json-bool';
        else if (m === 'null') cls = 'json-null';
        return '<span class="' + cls + '">' + m + '</span>';
      }
    );
  }

  function segbar(host, items, current, onPick, ariaLabel) {
    host.innerHTML = items.map((it) =>
      '<button type="button" class="seg" role="radio" data-v="' + esc(it.v) + '" aria-checked="' +
      (it.v === current) + '" aria-label="' + esc(ariaLabel || '') + '">' + esc(it.t) + '</button>'
    ).join('');
    host.addEventListener('click', (e) => {
      const b = e.target.closest('.seg');
      if (!b) return;
      onPick(b.dataset.v);
    });
  }

  function syncSeg(host, current) {
    $$('.seg', host).forEach((b) => b.setAttribute('aria-checked', String(b.dataset.v === current)));
  }

  /* ---------- hero ---------- */
  function initHero() {
    $('#heroFacts').innerHTML = LA.facts.map((f) =>
      '<li><b>' + esc(f.k) + '</b><span>' + esc(f.t) + '</span><span class="fd">' + esc(f.d) + '</span></li>'
    ).join('');

    const w = LA.heroWire;
    $('#heroWire').innerHTML = '<div class="wh">stdio · 4-byte length + JSON · 实测抓取形状</div>' +
      w.map((l) => '<div class="wl" data-f="' + l.type + '"><span class="wt">' + esc(l.type) + '</span><span>' + esc(l.text) + '</span></div>').join('');
    if (reduced) { $$('.wl').forEach((n) => n.classList.add('on')); return; }
    let i = 0;
    setInterval(() => { $$('.wl')[i % w.length].classList.toggle('on'); i++; }, 900);
  }

  /* ---------- 01 layers ---------- */
  const kindLab = { outside: '展示面', platform: '平台模块', ext: '扩展层' };
  let layerSel = 'host';

  function initLayers() {
    const stack = $('#layerStack');
    stack.innerHTML = LA.layers.map((l, i) => {
      const sep = (l.id === 'plugins' ? '<div class="stack-sep">—— Host 与 Plugin 的边界 ——</div>' : '');
      return sep + '<button type="button" class="stack-item" data-id="' + l.id + '" aria-pressed="false">' +
        '<span><span class="si-name">' + esc(l.name) + '</span><span class="si-path">' + esc(l.path) + '</span></span>' +
        '<span class="si-kind ' + l.kind + '">' + esc(kindLab[l.kind]) + '</span></button>';
    }).join('');
    stack.addEventListener('click', (e) => {
      const b = e.target.closest('.stack-item');
      if (!b) return;
      layerSel = b.dataset.id;
      renderLayers();
    });
    renderLayers();
  }

  function renderLayers() {
    $$('.stack-item').forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.id === layerSel)));
    const l = LA.layers.find((x) => x.id === layerSel);
    $('#layerDetail').innerHTML =
      '<h3>' + esc(l.name) + '</h3><p class="sub">' + esc(l.path) + '</p>' +
      '<dl class="dl">' +
      '<dt>职责</dt><dd>' + esc(l.role) + '</dd>' +
      '<dt class="neg">非职责</dt><dd>' + esc(l.not) + '</dd>' +
      '<dt>代码位置</dt><dd class="chiprow">' + l.files.map((f) => '<span class="chip">' + esc(f) + '</span>').join('') + '</dd>' +
      '</dl>';
  }

  /* ---------- 02 frame ---------- */
  const fieldReq = {
    version: '当前填 2',
    id: 'Request / Response 闭环标识',
    type: 'req、res、evt 或 cancel',
    from: 'Host 注入真实来源；插件出站 Frame 不填',
    context_id: '可选执行上下文标识，用于选择 Plugin Instance',
    capability: 'Request 主题',
    method: 'Request 主题',
    payload: '业务 JSON，对 Host 不透明',
    reason: 'cancel 的原因，只允许 cancelled 或 deadline_exceeded',
    priority: 'Host 管理的流水线位置',
    error_code: '成功时为空字符串；每条 Frame 都有这个字段',
    error_msg: '成功时为空字符串；每条 Frame 都有这个字段',
  };
  const order = ['version', 'id', 'type', 'from', 'context_id', 'capability', 'method', 'payload', 'reason', 'priority', 'error_code', 'error_msg'];

  const F = { type: 'req', cap: 'loop', method: 'turn', id: 'caller-1', ctx: false, pipe: true, err: '', payload: '{\n  "session_id": "s-7",\n  "input": "读一下 README"\n}' };

  function buildFrame(t) {
    const f = { version: LA.wire.version, id: F.id, type: t };
    const p = safeJSON(F.payload);
    if (t === 'req') {
      if (F.ctx) f.context_id = 'ctx-1';
      f.capability = F.cap; f.method = F.method;
      f.payload = p; f.priority = 2147483647;
    } else if (t === 'res') {
      f.from = owner[F.cap] || 'provider-plugin';
      f.capability = F.cap; f.method = F.method;
      if (!F.err) f.payload = p;
    } else if (t === 'evt') {
      f.from = owner[F.cap] || 'provider-plugin';
      f.capability = F.cap; f.method = F.method; f.payload = p;
    } else {
      f.from = 'caller-plugin';
      if (F.ctx) f.context_id = 'ctx-1';
      f.capability = F.cap; f.method = F.method; f.reason = 'cancelled';
    }
    const c = LA.errCodes.find((x) => x.code === F.err);
    f.error_code = t === 'res' ? F.err : '';
    f.error_msg = t === 'res' && c ? c.msg : '';
    const out = {};
    order.forEach((k) => { if (k in f) out[k] = f[k]; });
    return out;
  }

  function safeJSON(s) {
    try { return JSON.parse(s); } catch (e) { return { _parse_error: 'bad_payload' }; }
  }

  function wireHTML(frame) {
    const body = JSON.stringify(frame);
    const n = bytes(body);
    const b = [n >>> 24 & 255, n >>> 16 & 255, n >>> 8 & 255, n & 255];
    const head = b.map((x) => '<span class="byte len">' + hex(x) + '</span>').join('');
    const bodyBytes = new TextEncoder().encode(body);
    let dots = '';
    for (let i = 0; i < Math.min(8, bodyBytes.length); i++) dots += '<span class="byte">' + hex(bodyBytes[i]) + '</span>';
    const over = n > LA.wire.max;
    return '<div class="wire-strip"><div class="wire-lab">长度前缀（big-endian 4 字节） = ' + n + '</div>' +
      '<div class="bytes">' + head + '<span class="byte brace">|</span>' + dots + '<span class="byte brace">… 共 ' + n + ' 字节 JSON body</span></div>' +
      '<div class="' + (over ? 'warn' : 'ok') + '">' + (over ? '超过 16 MiB 上限，传输返回 frame_too_large' : '在 16 MiB 上限内，UTF-8 JSON body 原样追加') + '</div></div>';
  }

  function fieldsHTML(frame) {
    return '<table class="field-table"><thead><tr><th>字段</th><th>本帧取值要求</th></tr></thead><tbody>' +
      Object.keys(frame).map((k) =>
        '<tr><td>' + esc(k) + '</td><td>' + esc(fieldReq[k] || '—') + '</td></tr>'
      ).join('') + '</tbody></table>';
  }

  function initFrame() {
    segbar($('#frameType'), LA.frameTypes.map((t) => ({ v: t, t: t })), F.type, (v) => { F.type = v; syncSeg($('#frameType'), v); renderFrame(); }, 'Frame 类型');
    $('#frameCap').addEventListener('input', (e) => {
      const raw = e.target.value.trim();
      const dot = raw.indexOf('.');
      if (dot > 0) {
        F.cap = raw.slice(0, dot);
        F.method = raw.slice(dot + 1);
        $('#frameMethod').value = F.method;
      } else {
        F.cap = raw;
      }
      renderFrame();
    });
    $('#frameMethod').addEventListener('input', (e) => { F.method = e.target.value.trim(); renderFrame(); });
    $('#frameId').addEventListener('input', (e) => { F.id = e.target.value.trim(); renderFrame(); });
    $('#frameCtx').addEventListener('change', (e) => { F.ctx = e.target.checked; renderFrame(); });
    $('#framePipe').addEventListener('change', (e) => { F.pipe = e.target.checked; renderFrame(); });
    $('#framePayload').addEventListener('input', (e) => { F.payload = e.target.value; renderFrame(); });

    $('#capMethods').innerHTML = LA.capMethods.map((m) => '<option value="' + esc(m) + '"></option>').join('');
    const sel = $('#frameErr');
    sel.innerHTML = '<option value="">（空，表示成功）</option>' +
      LA.errCodes.map((c) => '<option value="' + c.code + '">' + c.code + ' · ' + c.src + '</option>').join('');
    sel.addEventListener('change', (e) => { F.err = e.target.value; renderFrame(); });
    renderFrame();
  }

  function renderErrTable() {
    $('#errTable').innerHTML = '<thead><tr><th>error_code</th><th>来源</th><th>含义</th></tr></thead><tbody>' +
      LA.errCodes.map((c) =>
        '<tr><td>' + esc(c.code) + '</td><td>' + esc(c.src) + '</td><td>' + esc(c.msg) +
        (c.note ? '<br><span class="mono-dim">' + esc(c.note) + '</span>' : '') + '</td></tr>').join('') +
      '</tbody>';
  }

  function renderFrame() {
    const frame = buildFrame(F.type);
    $('#errField').hidden = F.type !== 'res';
    const chosen = LA.errCodes.find((c) => c.code === F.err);
    $('#frameErrHint').textContent = chosen ? chosen.src + ' · ' + chosen.msg + (chosen.note ? ' · ' + chosen.note : '') : '';

    let out = '<div class="frame-note">' + esc(LA.frameNotes[F.type]) + '</div>';
    out += '<div class="wire-strip">' + wireHTML(frame) + '</div>';
    out += '<pre class="code">' + hlJSON(frame) + '</pre>';
    if (F.type === 'req' && F.pipe) {
      const fwd = { version: 2, id: 'fwd-1', type: 'req', from: 'caller-plugin', capability: frame.capability, method: frame.method, payload: frame.payload, priority: frame.priority };
      out += '<div class="frame-note">Host 为跨插件 Request 分配内部 <code>fwd-N</code> id 并注入真实 <code>from</code>；Response 返回调用方时恢复原始 id。</div>' +
        '<pre class="code small">' + hlJSON(fwd) + '</pre>';
    }
    if (F.type === 'evt' && !F.id) out += '<div class="frame-note">id 为空即广播 Event：Host 记录日志后丢弃，不投递。</div>';
    if (F.type === 'evt' && F.id) out += '<div class="frame-note">带 id 的归因 Event 属于一次在途 Call：Host 还原调用方 id 后投递给发起方。</div>';
    if (F.type === 'res' && F.err) out += '<div class="frame-note"><code>error_code</code> 非空时 <code>payload</code> 不作为结果。插件用 <code>pluginsdk.ErrCode</code> 产生稳定错误码，普通 error 映射为 <code>handler_error</code>。</div>';
    out += fieldsHTML(frame);
    $('#frameOut').innerHTML = out;
  }

  /* ---------- 03 pipeline ---------- */
  function renderPipe() {
    const p = LA.pipeline;
    const rows = [];
    rows.push(node(p.entry.name, 'priority ' + p.entry.priority, p.entry.note, true, null));
    p.consumers.forEach((c) => rows.push(node(c.name, 'priority ' + c.priority, c.note, en[c.name], c.name)));
    rows.push(node(p.owner.name, 'Route Owner · priority ' + p.owner.priority, p.owner.note, true, null));
    $('#pipeTrack').innerHTML = rows.join('');
    $('#pipeNote').innerHTML = (en['model-context']
      ? 'Request 从 Host 入口进入，先经 <code>' + p.method + '</code> 的 Consumer（priority 2000），Consumer 调 <code>Proceed</code> 后交给 Owner。'
      : '没有可用 Consumer 时，<code>agent</code> 的基线投影直接进入 <code>' + p.owner.name + '</code>。因为 <code>llm.complete</code> 的 Consumer 是可选注册，不是 Manifest 强制声明。') +
      '<br>' + p.rules.map((r) => '· ' + esc(r)).join('<br>');

    function node(name, prio, note, on, key) {
      return '<div class="pipe-node' + (on ? '' : ' off') + '"' + (key ? ' data-plugin="' + key + '"' : '') + '>' +
        '<div><div class="pn-name" style="color:' + colorOf(name) + '">' + esc(name) + '</div>' +
        '<div class="pn-prio">' + esc(prio) + '</div></div>' +
        '<div class="pn-note">' + esc(note) + '</div></div>';
    }
  }

  function colorOf(name) {
    const p = LA.plugins.find((x) => x.name === name);
    return p ? p.color : 'var(--tx)';
  }

  /* ---------- 04 graph ---------- */
  let pluginSel = 'agent';

  function initGraph() {
    const svg = $('#graph');
    const act = (t) => {
      const sw = t.closest('[data-toggle]');
      if (sw) { en[sw.dataset.toggle] = !en[sw.dataset.toggle]; broadcast(); return true; }
      const g = t.closest('[data-node]');
      if (g) { pluginSel = g.dataset.node; renderGraph(); renderDetail(); return true; }
      return false;
    };
    svg.addEventListener('click', (e) => act(e.target));
    svg.addEventListener('keydown', (e) => {
      if (e.key !== 'Enter' && e.key !== ' ') return;
      if (act(e.target)) e.preventDefault();
    });
    renderGraph(); renderDetail(); renderCombo();
  }

  function rect(name) {
    const p = LA.pluginPos[name];
    const host = name === 'host';
    const w = host ? 200 : NODE_W, h = host ? 54 : NODE_H;
    return { cx: p.x, cy: p.y, x: p.x - w / 2, y: p.y - h / 2, w: w, h: h };
  }

  function clip(a, b) {
    const dx = b.cx - a.cx, dy = b.cy - a.cy;
    const sx = (a.w / 2) / (Math.abs(dx) || 1e-6), sy = (a.h / 2) / (Math.abs(dy) || 1e-6);
    const s = Math.min(sx, sy);
    return [a.cx + dx * s, a.cy + dy * s];
  }

  function renderGraph() {
    const svg = $('#graph');
    const parts = ['<defs><marker id="ah" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L8,4 L0,8 z" fill="#4a6577"/></marker><marker id="ahp" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L8,4 L0,8 z" fill="#b79cff"/></marker></defs>'];

    const isOn = (n) => !en.hasOwnProperty(n) || en[n];
    LA.plugins.filter((p) => p.name !== 'host').forEach((p) => {
      const a = rect('host'), b = rect(p.name);
      const pt = clip(a, b), pb = clip(b, a);
      parts.push('<line class="gedge host' + (isOn(p.name) ? '' : ' off') + '" x1="' + pt[0] + '" y1="' + pt[1] + '" x2="' + pb[0] + '" y2="' + pb[1] + '"/>');
    });

    LA.edges.forEach((ed) => {
      const on = isOn(ed.from) && isOn(ed.to);
      const a = rect(ed.from), b = rect(ed.to);
      const ctl = ed.dip ? [(a.cx + b.cx) / 2, Math.max(a.cy, b.cy) + ed.dip] : null;
      const pa = ctl ? clip(a, { cx: ctl[0], cy: ctl[1], w: a.w, h: a.h }) : clip(a, b);
      const pb = ctl ? clip(b, { cx: ctl[0], cy: ctl[1], w: b.w, h: b.h }) : clip(b, a);
      const cls = 'gedge ' + ed.kind + (on ? '' : ' off');
      const mk = ed.kind === 'pipeline' ? 'url(#ahp)' : 'url(#ah)';
      const d = ctl
        ? 'M' + pa[0] + ',' + pa[1] + ' Q' + ctl[0] + ',' + ctl[1] + ' ' + pb[0] + ',' + pb[1]
        : 'M' + pa[0] + ',' + pa[1] + ' L' + pb[0] + ',' + pb[1];
      parts.push('<path class="' + cls + '" d="' + d + '" marker-end="' + mk + '"/>');
      if (ed.label) {
        const lx = ctl ? ctl[0] : (pa[0] + pb[0]) / 2;
        const ly = (ctl ? ctl[1] : (pa[1] + pb[1]) / 2) - 5;
        parts.push('<text class="gelabel" x="' + lx + '" y="' + ly + '" text-anchor="middle" style="paint-order:stroke;stroke:#080d13;stroke-width:3">' + esc(ed.label) + '</text>');
      }
    });

    LA.plugins.forEach((p) => {
      const r = rect(p.name), on = isOn(p.name);
      const sel = pluginSel === p.name;
      const sw = p.toggleable;
      parts.push('<g class="gnode' + (on ? '' : ' off') + '" data-node="' + p.name + '" role="button" tabindex="0" aria-pressed="' + sel + '">');
      parts.push('<rect x="' + r.x + '" y="' + r.y + '" width="' + r.w + '" height="' + r.h + '" rx="5"' +
        (sel ? ' style="stroke:' + p.color + ';stroke-width:1.8"' : '') + '/>');
      if (p.name !== 'host') parts.push('<rect x="' + r.x + '" y="' + r.y + '" width="3" height="' + r.h + '" fill="' + p.color + '" rx="1.5"/>');
      parts.push('<text class="nm" x="' + (r.cx) + '" y="' + (r.y + (p.name === 'host' ? 33 : 30)) + '" text-anchor="middle" style="font-size:' + (p.name === 'host' ? 16 : 15) + 'px" fill="' + p.color + '">' + esc(p.label) + '</text>');
      if (p.name === 'host') { parts.push('<text class="cp" x="' + r.cx + '" y="' + (r.y + 48) + '" text-anchor="middle">薄内核 · 装载与路由</text>'); }
      else {
        parts.push('<text class="cp" x="' + r.cx + '" y="' + (r.y + 48) + '" text-anchor="middle">provides ' + esc(p.caps.join(' · ') || '—') + '</text>');
        const pr = p.priority === null ? '—' : p.priority;
        parts.push('<text class="st" x="' + r.cx + '" y="' + (r.y + 64) + '" text-anchor="middle">' + esc(on ? 'active' : 'disabled') + ' · prio ' + esc(String(pr)) + '</text>');
        if (sw) {
          const bx = r.x + r.w - 46, by = r.y + r.h - 22;
          parts.push('<g data-toggle="' + p.name + '" role="button" tabindex="0" aria-label="启停 ' + esc(p.name) + '">');
          parts.push('<rect class="gswitch-bg' + (on ? ' on' : '') + '" x="' + bx + '" y="' + by + '" width="40" height="17" rx="3"/>');
          parts.push('<text class="gswitch" x="' + (bx + 20) + '" y="' + (by + 12.5) + '" text-anchor="middle" fill="' + (on ? '#43d9c0' : '#93a5b8') + '">' + (on ? '运行' : '停用') + '</text>');
          parts.push('</g>');
        }
      }
      parts.push('</g>');
    });
    svg.innerHTML = parts.join('');
  }

  function renderDetail() {
    const p = LA.plugins.find((x) => x.name === pluginSel);
    const on = !en.hasOwnProperty(p.name) || en[p.name];
    let h = '<h3 style="color:' + p.color + '">' + esc(p.label) + '</h3>' +
      '<p class="sub">' + esc(p.status) + (p.priority === null ? '' : ' · priority ' + p.priority) + ' · ' + (on ? 'active' : 'disabled') + '</p>' +
      '<p style="font-size:13.6px;color:var(--dim);margin:0 0 14px">' + esc(p.desc) + '</p>';
    const row = (k, v) => '<div class="mrow"><code>' + k + '</code><div class="md">' + v + '</div></div>';
    if (p.name !== 'host') {
      h += row('depends_on', p.dependsOn.length ? p.dependsOn.map((d) => '<span class="chip">' + esc(d) + '</span>').join('') : '<span class="chip">无</span>');
      h += row('requires', p.requires.length ? p.requires.map((d) => '<span class="chip">' + esc(d) + '</span>').join('') : '<span class="chip">无</span>');
      if (p.mounts.length) h += row('ui.components', p.mounts.map((m) => '<span class="chip">' + esc(m[0]) + ' → ' + esc(m[1]) + '</span>').join(''));
      else h += row('ui.components', '<span class="chip">无</span>');
      if (p.note) h += row('note', esc(p.note));
    }
    h += '<div style="margin-top:14px"><p class="mono-dim">注册的 capability.method</p>' +
      p.methods.map((m) => '<div class="mrow"><code>' + esc(m[0]) + '</code><div class="md">' + esc(m[1]) + '</div></div>').join('') + '</div>';
    $('#pluginDetail').innerHTML = h;
  }

  function renderCombo() {
    const mc = en['model-context'], tl = en['tools'], ft = en['file-tools'], st = en['shell-tools'];
    const chain = ['agent'];
    const h = [];
    h.push('<h3 class="mini-title">当前组合的调用链</h3>');
    h.push('<div class="combo-line">session-chat → loop.turn → agent → session.derive（基线投影）</div>');
    h.push('<div class="combo-line">llm.complete → ' + (mc ? 'model-context (prio 2000) → Proceed → llm-openai' : 'llm-openai（Consumer 缺席，基线直达）') + '</div>');
    h.push('<div class="combo-line' + (tl ? '' : ' dim') + '">工具阶段 → ' + (tl ? 'tools.list / tools.call → ' + [ft ? 'file.*' : null, st ? 'shell.*' : null].filter(Boolean).join(' + ') + (ft || st ? '' : '（无可用 Provider）') : '跳过，写入 request_header.meta.degraded') + '</div>');
    h.push('<div class="combo-line">落盘 → session.append（assistant / step_end / turn_end） → ui.state</div>');

    const deg = [];
    if (!mc) deg.push('<b>model-context 停用</b>：不写 system / runtime Facts，不做快照与确定性压缩；模型只看到普通消息投影。');
    if (!tl) deg.push('<b>tools 停用</b>：agent 跳过工具阶段并标记 degraded；file-tools、shell-tools 的注册表不再被查询。');
    if (!ft) deg.push('<b>file-tools 停用</b>：file.* 不再进入路由表，tools.call 打到该目标返回 route_failed。');
    if (!st) deg.push('<b>shell-tools 停用</b>：shell.* 不可用；shell_execute 是 RiskCritical 工具。');
    h.push('<div class="degrade">' + (deg.length ? deg.join('<br>') : '全部插件在线。降级只发生在弱依赖缺失时，硬依赖缺失会让插件启动或调用失败。') + '</div>');
    h.push('<p class="hint">启动顺序由 Manifest 的 <code>autostart</code> 与 <code>depends_on</code> 决定：Host 取全部 autostart 根插件，按 <code>depends_on</code> 做 DFS 后序装载。停止某个插件不会级联停止依赖方，但 Host 会摘掉它的路由键，依赖方的后续调用返回 <code>method_not_found</code>（路由表里没有属主）。</p>');
    $('#comboOut').innerHTML = h.join('');
  }

  function broadcast() {
    renderGraph(); renderDetail(); renderCombo(); renderPipe(); renderPlayer();
  }

  /* ---------- 05 turn player ---------- */
  let scen = 'normal', idx = 0, timer = null;

  function initPlayer() {
    const items = Object.keys(LA.scenarios).map((k) => ({ v: k, t: LA.scenarios[k].label }));
    segbar($('#scenarioBar'), items, scen, (v) => { scen = v; idx = 0; syncSeg($('#scenarioBar'), v); stop(); renderPlayer(); }, '场景');
    $('#stepPrev').addEventListener('click', () => { stop(); idx = Math.max(0, idx - 1); renderPlayer(); });
    $('#stepNext').addEventListener('click', () => { stop(); idx = Math.min(steps().length - 1, idx + 1); renderPlayer(); });
    $('#stepPlay').addEventListener('click', () => { timer ? stop() : play(); });
    $('#stepRange').addEventListener('input', (e) => { stop(); idx = +e.target.value - 1; renderPlayer(); });
    $('#turnNote').textContent = LA.turnNote;
    renderPlayer();
  }

  function steps() { return LA.scenarios[scen].steps; }

  function eff(st) {
    if (!st.needs) return { state: 'ok', to: st.to };
    const missing = st.needs.filter((n) => !en[n]);
    if (!missing.length) return { state: 'ok', to: st.to };
    if (st.altTo) return { state: 'alt', to: st.altTo, why: st.altNote };
    return { state: 'skip', to: st.to, why: st.skip };
  }

  function play() {
    $('#stepPlay').textContent = '暂停';
    if (idx >= steps().length - 1) idx = 0;
    renderPlayer();
    timer = setInterval(() => {
      if (idx >= steps().length - 1) { stop(); return; }
      idx++; renderPlayer();
    }, 1700);
  }
  function stop() {
    if (timer) clearInterval(timer);
    timer = null;
    $('#stepPlay').textContent = '播放';
  }

  function renderPlayer() {
    const list = steps();
    const tagLab = { req: 'req', res: 'res', evt: 'evt', ui: 'ui push', http: 'http', note: 'internal' };
    const used = {};
    list.forEach((s) => { used[s.from] = 1; used[eff(s).to] = 1; });

    $('#lanes').innerHTML = LA.actors.map((a) =>
      '<div class="lane-head' + (used[a.id] ? '' : ' dimmed') + '" data-lane="' + a.id + '">' +
      '<span class="lh-n">' + esc(a.label) + '</span><span class="lh-s">' + esc(a.sub) + '</span></div>'
    ).join('');

    const nameOf = (id) => (LA.actors.find((a) => a.id === id) || { label: id }).label;
    $('#laneBody').innerHTML = list.map((s, i) => {
      const e = eff(s);
      const st = i < idx ? 'done' : (i === idx ? 'cur' : '');
      const cls = st + (e.state === 'skip' ? ' skip' : '');
      const to = e.state === 'alt' ? s.altTo : s.to;
      return '<div class="msg ' + cls + '" data-i="' + i + '">' +
        '<span class="mn">' + (i + 1) + '</span>' +
        '<span><span class="mline"><b style="font-weight:500">' + esc(s.label) + '</b>' +
        '<span class="mto">' + esc(nameOf(s.from)) + ' → ' + esc(nameOf(to)) + '</span>' +
        '<span class="mtag">' + esc(tagLab[s.frame] || s.frame) + '</span>' +
        '<span class="mtag">' + esc(s.cap) + '.' + esc(s.method) + '</span></span>' +
        (s.note ? '<span class="mnote">' + esc(s.note) + '</span>' : '') +
        (e.state === 'alt' ? '<span class="mskip" style="display:block">改写：' + esc(e.why) + '</span>' : '') +
        (e.state === 'skip' ? '<span class="mskip">跳过：' + esc(e.why) + '</span>' : '') +
        '</span></div>';
    }).join('');

    $$('#laneBody .msg').forEach((m) => m.addEventListener('click', () => { stop(); idx = +m.dataset.i; renderPlayer(); }));
    const cur = list[idx];
    if (cur) {
      const e = eff(cur);
      $$('.lane-head').forEach((h) => h.classList.toggle('hit', h.dataset.lane === cur.from || h.dataset.lane === (e.state === 'alt' ? cur.altTo : cur.to)));
      const active = $('.msg.cur');
      if (active) active.scrollIntoView({ block: 'nearest' });
    }

    const range = $('#stepRange');
    range.max = String(list.length);
    range.value = String(idx + 1);
    $('#stepCount').textContent = (idx + 1) + ' / ' + list.length + ' · ' + LA.scenarios[scen].label;

    if (!cur) return;
    const e2 = eff(cur);
    $('#stepRead').innerHTML =
      blk('动作', cur.label) +
      blk('Frame', cur.cap + '.' + cur.method + ' · ' + (tagLab[cur.frame] || cur.frame)) +
      blk('链路', nameOf(cur.from) + ' → ' + nameOf(e2.state === 'alt' ? cur.altTo : cur.to)) +
      blk('降级', e2.state === 'ok' ? '无（当前插件组合下正常执行）' : e2.why);

    function blk(k, v) { return '<div class="sr"><b>' + esc(k) + '</b>' + esc(v) + '</div>'; }
  }

  /* ---------- 06 webui ---------- */
  let slotSel = 'center', scSel = 'state';

  function initShell() {
    $('#shellMock').addEventListener('click', (e) => {
      const s = e.target.closest('.slot');
      if (!s) return;
      slotSel = s.dataset.slot;
      renderShell();
    });
    segbar($('#scBar'), [{ v: 'state', t: '推 State' }, { v: 'command', t: '推 Command' }], scSel, (v) => {
      scSel = v; syncSeg($('#scBar'), v); renderSC();
    }, '推送类型');
    renderShell(); renderSC();
  }

  function renderShell() {
    $$('.slot').forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.slot === slotSel)));
    const s = LA.slots[slotSel];
    let h = '<h3>' + esc(s.tag) + '</h3><p class="sub">Slot <code>' + esc(slotSel) + '</code> · 属主 ' + esc(s.owner) + '</p>' +
      '<p style="font-size:13.6px;color:var(--dim);margin:0 0 12px">' + esc(s.body) + '</p>' +
      '<dl class="dl">' +
      '<dt>Mount 身份</dt><dd><code>' + esc(s.owner === '—' ? '—' : s.owner + ' + ' + s.mount) + '</code>，<code>mount_id</code> 只定位实例，不参与业务路由</dd>' +
      '<dt>插件侧发送</dt><dd><code>SetState(mountID, state)</code> / <code>SendCommand(mountID, command, payload)</code>，目标地址由 pluginsdk 生成</dd>';
    h += '<dt>Component 契约</dt><dd>' + LA.componentApi.map((c) =>
      '<div class="mrow"><code>' + esc(c[0]) + '</code><div class="md">' + esc(c[1]) + '</div></div>').join('') + '</dd>';
    h += '<dt class="neg">Host 不做</dt><dd>不解释 Component 的业务内容，不保存插件业务状态，不实现业务面板</dd></dl>';
    $('#shellDetail').innerHTML = h;
  }

  function renderSC() {
    const isState = scSel === 'state';
    const payload = isState ? LA.stateSample : LA.commandSample;
    $('#scOut').innerHTML = hlJSON(payload);
    const badge = $('#pushBadge');
    badge.hidden = false;
    badge.textContent = isState ? 'state rev=12' : 'command scroll';
    badge.style.borderColor = isState ? '#2f6d68' : '#6b5a35';
    badge.style.color = isState ? 'var(--teal)' : 'var(--amber)';
  }

  /* ---------- 07 context ---------- */
  let ctxMC = true, ctxCompact = false, hlSeq = 0;

  function initCtx() {
    $('#ctxMC').addEventListener('change', (e) => { ctxMC = e.target.checked; renderCtx(); });
    $('#ctxCompact').addEventListener('change', (e) => { ctxCompact = e.target.checked; renderCtx(); });
    $('#factList').addEventListener('click', (e) => {
      const f = e.target.closest('.fact');
      if (!f) return;
      hlSeq = hlSeq === +f.dataset.seq ? 0 : +f.dataset.seq;
      renderCtx();
    });
    $('#projBox').addEventListener('click', (e) => {
      const b = e.target.closest('.msgbox');
      if (!b) return;
      const s = +b.dataset.src;
      hlSeq = hlSeq === s ? 0 : s;
      renderCtx();
    });
    renderCtx();
  }

  function ctxVisible(f) {
    if (f.compact && (!ctxCompact || !ctxMC)) return false;
    if (f.mc && !ctxMC) return false;
    return true;
  }

  function renderCtx() {
    const compactOn = ctxCompact && ctxMC;
    const facts = LA.ctxFacts;
    $('#ctxCompact').disabled = !ctxMC;
$('#factList').innerHTML = facts.map((f) => {
      const vis = ctxVisible(f);
      const label = f.kind ? f.type + ' · ' + f.kind : f.type;
      return '<li class="fact' + (vis ? '' : ' muted') + (hlSeq === f.seq ? ' hl' : '') + '" data-seq="' + f.seq + '">' +
        '<span class="fs">seq ' + f.seq + '</span><span class="ft">' + esc(label) + '</span>' +
        '<span class="fx">' + esc(f.role) + ' · ' + esc(f.text) + '</span></li>';
    }).join('');

    const proj = [];
    const msgs = facts.filter((f) => f.type === 'message' && ctxVisible(f) && !f.kind);
    if (ctxMC) {
      proj.push({ role: 'system', text: 'system prompt 正文（来自 kind=system_prompt 的 message Fact）', src: 2 });
      proj.push({ role: 'user', text: 'runtime context：cwd、平台、时间、工具清单（kind=runtime_context 的 message Fact）', src: 3 });
    }
    const shadowed = compactOn ? [5, 7] : [];
    if (compactOn) proj.push({ role: 'system', text: '摘要：前一轮对话的确定性压缩结果（shadow range seq 4–10）', src: 14 });
    msgs.forEach((m) => {
      if (shadowed.indexOf(m.seq) >= 0) return;
      proj.push({ role: m.role, text: m.text, src: m.seq });
    });
    if (!ctxMC) proj.push({ role: 'system', text: '（无 system / runtime Fact，基线直接进 llm-openai）', src: 0 });

    $('#projBox').innerHTML = proj.map((p) =>
      '<div class="msgbox' + (hlSeq && p.src === hlSeq ? ' hl' : '') + '" data-role="' + esc(p.role) + '" data-src="' + p.src + '">' +
      '<span class="role">' + esc(p.role) + (p.src ? ' · seq ' + p.src : '') + '</span>' +
      '<span class="body">' + esc(p.text) + '</span></div>'
    ).join('');

    const tok = compactOn ? '压缩后 token 估算下降，摘要落在 shadow range 上，原始 Fact 不被改写' : '未触发压缩，投影覆盖全部普通消息';
    $('#projNote').innerHTML = '模型看到的是纯投影结果：' + (ctxMC ? 'Fact Log 含 system_prompt 与 runtime_context' : 'Fact Log 无上下文 Facts') +
      '。<code>projectMessages</code> 按 system / runtime / summary / shadow range 投影，' + tok + '。点击任一条可定位它的来源 Fact。';
  }

  /* ---------- 08 / 09 static ---------- */
  function initStatic() {
    $('#rulesGrid').innerHTML = LA.rules.map((r, i) =>
      '<div class="rule" data-n="' + String(i + 1).padStart(2, '0') + '"><p>' + esc(r[0]) + '</p><span>' + esc(r[1]) + '</span></div>').join('');
    $('#nongoals').innerHTML = LA.nongoals.map((n) => '<span class="chip">' + esc(n) + '</span>').join('');
    $('#removed').innerHTML = LA.removed.map((n) => '<span class="chip">' + esc(n) + '</span>').join('');
    $('#notImplemented').innerHTML = LA.notImplemented.map((n) => '<span class="chip">' + esc(n) + '</span>').join('');
    $('#security').innerHTML = LA.security.map((s) => '<li>' + esc(s) + '</li>').join('');
    $('#endpoints').innerHTML = LA.endpoints.map((e) =>
      '<li><span class="em em-' + e[0] + '">' + e[0] + '</span><span><span class="ep">' + esc(e[1]) + '</span><br><span class="mono-dim">' + esc(e[2]) + '</span></span></li>').join('');
    $('#cmds').innerHTML = LA.cmds.map((c) =>
      '<div class="cmd"><code>' + esc(c[0]) + '</code><span>' + esc(c[1]) + '</span></div>').join('');
    $('#searchOrder').innerHTML = LA.searchOrder.map((s) => '<li>' + esc(s) + '</li>').join('');
    $('#docMap').innerHTML = '<thead><tr><th>文档</th><th>内容</th></tr></thead><tbody>' +
      LA.docMap.map((d) => '<tr><td>' + esc(d[0]) + '</td><td>' + esc(d[1]) + '</td></tr>').join('') + '</tbody>';
    $('#reading').innerHTML = LA.reading.map((r) => '<li>' + esc(r) + '</li>').join('');
  }

  /* ---------- nav spy ---------- */
  function initNav() {
    const links = $$('.nav a');
    const map = {};
    links.forEach((a) => { map[a.getAttribute('href').slice(1)] = a; });
    const obs = new IntersectionObserver((es) => {
      es.forEach((e) => {
        if (e.intersectionRatio < 0.18) return;
        links.forEach((a) => a.classList.remove('here'));
        const l = map[e.target.id];
        if (l) l.classList.add('here');
      });
    }, { rootMargin: '-40% 0px -50% 0px', threshold: [0.18, 0.5] });
    ['overview'].concat(links.map((a) => a.getAttribute('href').slice(1))).forEach((id) => {
      const s = document.getElementById(id);
      if (s) obs.observe(s);
    });
  }

  document.addEventListener('DOMContentLoaded', () => {
    initHero(); initLayers(); initFrame(); renderErrTable(); renderPipe(); initGraph();
    initPlayer(); initShell(); initCtx(); initStatic(); initNav();
  });
})();
