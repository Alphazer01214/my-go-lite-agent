// Floating plugin-relationship panel (shell overlay). Opened from Chat header
// "Plugins". The graph is drawn from the whole Discovery catalog — every
// discovered plugin gets a node, coloured by mount state — so it is complete at
// page load: with Autostart+dependsOn (ADR-0021) most plugins mount lazily via
// an Agent Scheme (ADR-0023), and a live-only graph would look half empty until
// the first Turn.
//
// The snapshot is prefetched at Shell boot (prefetchPlugins) so opening the
// panel paints synchronously from cache; a background refetch repaints when the
// Host gained plugins (e.g. right after a scheme switch ensured them).

function el(tag, attrs, children) {
  const n = document.createElement(tag);
  if (attrs) Object.keys(attrs).forEach(k => {
    if (k === 'text') n.textContent = attrs[k];
    else if (k === 'html') n.innerHTML = attrs[k];
    else if (k === 'class') n.className = attrs[k];
    else n.setAttribute(k, attrs[k]);
  });
  (children || []).forEach(c => c && n.appendChild(c));
  return n;
}

function svgEl(tag, attrs) {
  const n = document.createElementNS('http://www.w3.org/2000/svg', tag);
  if (attrs) Object.keys(attrs).forEach(k => n.setAttribute(k, attrs[k]));
  return n;
}

// Node palette per kind (frozen dark idiom of the shell: explicit fills, never
// theme-class names).
const KIND = {
  host: { fill: '#1a2438', stroke: '#7aa2f7', text: '#e8eaed' },
  plugin: { fill: '#161a22', stroke: '#9ece6a', text: '#e8eaed' },
  capability: { fill: '#1a1620', stroke: '#bb9af7', text: '#e8eaed' },
  slot: { fill: '#12161f', stroke: '#e0af68', text: '#e8eaed' }
};

// Mount states (server: StateMounted / StateAvailable / StateDegraded / StateMissing).
const STATE = {
  mounted: { stroke: '#9ece6a', fill: '#161a22', text: '#e8eaed', dash: '', label: 'mounted' },
  available: { stroke: '#4b5563', fill: '#12161f', text: '#9aa0a6', dash: '5 3', label: 'available' },
  degraded: { stroke: '#f7768e', fill: '#2a1620', text: '#f7768e', dash: '3 2', label: 'degraded' },
  missing: { stroke: '#e0af68', fill: '#1a1620', text: '#e0af68', dash: '2 3', label: 'missing' },
  disabled: { stroke: '#6b7280', fill: '#1a1a1a', text: '#6b7280', dash: '1 4', label: 'disabled' }
};

const EDGE = {
  provides: { stroke: '#9ece6a', dash: '' },
  'host-uses': { stroke: '#7aa2f7', dash: '' },
  consumes: { stroke: '#bb9af7', dash: '4 3' },
  'ui-mount': { stroke: '#e0af68', dash: '2 3' },
  'depends-on': { stroke: '#56b6c2', dash: '' },
  scheme: { stroke: '#c678dd', dash: '1 4' }
};

function layeredLayout(nodes, W, H) {
  const plugins = nodes.filter(n => n.kind === 'plugin');
  const caps = nodes.filter(n => n.kind === 'capability');
  const slots = nodes.filter(n => n.kind === 'slot');
  const hosts = nodes.filter(n => n.kind === 'host');
  const pos = {};
  const pad = 36;
  const colW = (W - pad * 2) / 3;

  function stack(list, x, y0, y1) {
    if (!list.length) return;
    const n = list.length;
    const span = Math.max(y1 - y0, 40);
    list.forEach((item, i) => {
      const y = n === 1 ? (y0 + y1) / 2 : y0 + (span * i) / (n - 1);
      pos[item.id] = { x, y, kind: item.kind, label: item.label, node: item };
    });
  }

  stack(plugins, pad + colW * 0.5, pad + 20, H - pad - 20);
  stack(caps, pad + colW * 1.5, pad + 20, H - pad - 20);
  // Host + UI slots share the right column
  const right = hosts.concat(slots);
  stack(right, pad + colW * 2.5, pad + 20, H - pad - 20);
  return pos;
}

function nodeSize(kind) {
  if (kind === 'host') return { w: 88, h: 40 };
  if (kind === 'capability') return { w: 100, h: 32 };
  if (kind === 'slot') return { w: 120, h: 32 };
  return { w: 110, h: 36 };
}

function edgeEndpoints(a, b, sa, sb) {
  // Horizontal-ish layered edges: leave from right of a, enter left of b (or reverse).
  const dx = b.x - a.x;
  const sx = dx >= 0 ? 1 : -1;
  return {
    x1: a.x + sx * (sa.w / 2),
    y1: a.y,
    x2: b.x - sx * (sb.w / 2 + 4),
    y2: b.y
  };
}

function relatedTo(edges, id) {
  return edges.filter(e => e.from === id || e.to === id);
}

function drawGraph(host, graph, selected, onSelect) {
  const nodes = graph.nodes || [];
  const edges = graph.edges || [];
  const nPlugin = Math.max(nodes.filter(n => n.kind === 'plugin').length, 1);
  const nCap = Math.max(nodes.filter(n => n.kind === 'capability').length, 1);
  const W = Math.max(760, 220 + nPlugin * 20);
  const H = Math.max(420, Math.max(nPlugin, nCap, 4) * 52 + 80);
  const pos = layeredLayout(nodes, W, H);

  const svg = svgEl('svg', { width: W, height: H, viewBox: `0 0 ${W} ${H}` });
  // Column captions
  [['Plugins', W * 0.166], ['Capabilities', W * 0.5], ['Host / UI slots', W * 0.833]].forEach(([t, x]) => {
    const label = svgEl('text', {
      x, y: 18, 'text-anchor': 'middle', fill: '#9aa0a6',
      'font-size': '11', 'font-family': 'ui-monospace, Menlo, Consolas, monospace'
    });
    label.textContent = t;
    svg.appendChild(label);
  });

  const defs = svgEl('defs');
  Object.keys(EDGE).forEach(k => {
    const marker = svgEl('marker', {
      id: 'm-' + k, viewBox: '0 0 10 10', refX: '9', refY: '5',
      markerWidth: '5', markerHeight: '5', orient: 'auto-start-reverse'
    });
    marker.appendChild(svgEl('path', { d: 'M0,0 L10,5 L0,10 z', fill: EDGE[k].stroke }));
    defs.appendChild(marker);
  });
  svg.appendChild(defs);

  const hotSet = new Set();
  if (selected) {
    hotSet.add(selected);
    relatedTo(edges, selected).forEach(e => { hotSet.add(e.from); hotSet.add(e.to); });
  }

  edges.forEach(e => {
    const a = pos[e.from], b = pos[e.to];
    if (!a || !b) return;
    const sa = nodeSize(a.kind), sb = nodeSize(b.kind);
    const { x1, y1, x2, y2 } = edgeEndpoints(a, b, sa, sb);
    const mx = (x1 + x2) / 2;
    const hot = selected && (e.from === selected || e.to === selected);
    const style = EDGE[e.kind] || EDGE.provides;
    const path = svgEl('path', {
      d: `M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}`,
      fill: 'none',
      stroke: style.stroke,
      'stroke-width': hot ? 2.2 : 1.3,
      'stroke-dasharray': style.dash || '',
      'stroke-opacity': selected && !hot ? 0.2 : 0.95,
      'marker-end': `url(#m-${e.kind})`
    });
    const title = svgEl('title');
    const cap = e.capability ? ` [${e.capability}]` : '';
    const comp = e.component ? ` (${e.component})` : '';
    const sch = e.scheme ? ` scheme=${e.scheme}` : '';
    title.textContent = `${e.from} —${e.kind}→ ${e.to}${cap}${comp}${sch}`;
    path.appendChild(title);
    svg.appendChild(path);

    // Edge label near midpoint
    const lx = (x1 + x2) / 2;
    const ly = (y1 + y2) / 2 - 6;
    const lab = svgEl('text', {
      x: lx, y: ly, 'text-anchor': 'middle',
      fill: hot ? style.stroke : '#6b7280',
      'font-size': '9',
      'font-family': 'ui-monospace, Menlo, Consolas, monospace',
      'fill-opacity': selected && !hot ? 0.25 : 1
    });
    lab.textContent = e.kind === 'ui-mount' ? (e.component || 'mount')
      : e.kind === 'scheme' ? ('scheme:' + (e.scheme || ''))
        : (e.capability || e.kind);
    svg.appendChild(lab);
  });

  nodes.forEach(n => {
    const p = pos[n.id];
    if (!p) return;
    const sz = nodeSize(n.kind);
    const base = KIND[n.kind] || KIND.plugin;
    const st = n.kind === 'plugin' ? (STATE[n.state] || STATE.mounted) : null;
    const colors = st || base;
    const hot = selected === n.id;
    const dim = selected && !hotSet.has(n.id);
    const g = svgEl('g', { style: 'cursor:pointer', opacity: dim ? 0.28 : 1 });
    g.appendChild(svgEl('rect', {
      x: p.x - sz.w / 2, y: p.y - sz.h / 2,
      width: sz.w, height: sz.h, rx: 8,
      fill: hot ? '#1a2438' : colors.fill,
      stroke: hot ? '#7aa2f7' : colors.stroke,
      'stroke-width': hot ? 2 : (n.autostart && n.kind === 'plugin' ? 2 : 1.2),
      'stroke-dasharray': (st && st.dash) || '',
      'fill-opacity': st && n.state === 'available' ? 0.55 : 1
    }));
    const t = svgEl('text', {
      x: p.x, y: p.y, 'text-anchor': 'middle', 'dominant-baseline': 'central',
      fill: colors.text, 'font-size': n.kind === 'plugin' ? 12 : 11,
      'font-family': 'system-ui, sans-serif',
      'font-weight': n.kind === 'plugin' || n.kind === 'host' ? '600' : '400'
    });
    const label = n.label || n.id;
    t.textContent = label.length > 16 ? label.slice(0, 15) + '…' : label;
    g.appendChild(t);
    const title = svgEl('title');
    const lines = [n.label || n.id, 'kind: ' + n.kind];
    if (n.state) lines.push('state: ' + n.state);
    if (n.version) lines.push('version: ' + n.version);
    if (n.description) lines.push(n.description);
    if (n.autostart) lines.push('autostart: true（Host 启动根）');
    if (n.provides && n.provides.length) lines.push('provides: ' + n.provides.join(', '));
    if (n.consumes && n.consumes.length) lines.push('consumes: ' + n.consumes.join(', '));
    if (n.dependsOn && n.dependsOn.length) lines.push('dependsOn: ' + n.dependsOn.join(', '));
    if (n.schemes && n.schemes.length) lines.push('pulled by scheme: ' + n.schemes.join(', '));
    if (n.tools && n.tools.length) lines.push('tools: ' + n.tools.join(', '));
    if (n.unmet && n.unmet.length) lines.push('unmet: ' + n.unmet.join(', '));
    if (n.mounts && n.mounts.length) {
      lines.push('ui: ' + n.mounts.map(m => m.page + '/' + m.slot + '→' + m.component).join('; '));
    }
    title.textContent = lines.join('\n');
    g.appendChild(title);
    g.addEventListener('click', () => onSelect(hot ? null : n.id));
    svg.appendChild(g);
  });

  host.innerHTML = '';
  host.appendChild(svg);
}

function chip(text, color, border) {
  return el('span', {
    text,
    style: `font-size:10px;font-family:var(--la-mono);padding:2px 6px;border-radius:4px;border:1px solid ${border};color:${color};display:inline-block`
  });
}

function chips(list, color, border) {
  const box = el('div', { style: 'display:flex;flex-wrap:wrap;gap:4px;margin-top:4px' });
  if (!list || !list.length) {
    box.appendChild(el('span', { style: 'color:var(--la-dim);font-size:11px', text: '—' }));
    return box;
  }
  list.forEach(c => box.appendChild(chip(c, color, border)));
  return box;
}

function stateBadge(state) {
  const st = STATE[state] || STATE.mounted;
  return el('span', {
    text: st.label,
    style: `font-size:9px;font-family:var(--la-mono);text-transform:uppercase;letter-spacing:.05em;
            padding:1px 5px;border-radius:4px;border:1px dashed ${st.stroke};color:${st.stroke};margin-left:6px`
  });
}

function renderDetail(side, data, selected, onSelect) {
  const plugins = data.plugins || [];
  const graph = data.graph || { nodes: [], edges: [] };
  const summary = data.summary || {};
  side.innerHTML = '';

  side.appendChild(el('div', {
    style: 'font-size:11px;text-transform:uppercase;letter-spacing:.06em;color:var(--la-dim);margin-bottom:8px;font-family:var(--la-mono)',
    text: 'Mounted ' + (summary.mounted != null ? summary.mounted : plugins.length) +
      ' / discovered ' + (summary.discovered != null ? summary.discovered : plugins.length)
  }));

  if (data.scheme) {
    const sch = el('div', {
      style: 'border:1px solid var(--la-line);border-radius:8px;padding:8px 10px;margin-bottom:10px;background:var(--la-panel2)'
    });
    sch.appendChild(el('div', {
      style: 'font-size:10px;color:var(--la-dim);font-family:var(--la-mono)', text: 'agent scheme'
    }));
    sch.appendChild(el('div', {
      style: 'font-size:13px;color:#c678dd;font-weight:600;font-family:var(--la-mono)', text: data.scheme
    }));
    const pulls = (data.schemePulls || {})[data.scheme] || [];
    sch.appendChild(el('div', {
      style: 'font-size:10px;color:var(--la-dim);margin-top:4px;font-family:var(--la-mono)',
      text: pulls.length ? 'pulls: ' + pulls.join(', ') : 'pulls: (no extra plugin)'
    }));
    side.appendChild(sch);
  }

  plugins.forEach(p => {
    const isSel = selected === p.name;
    const card = el('div', {
      style: 'border:1px solid ' + (isSel ? 'var(--la-accent)' : 'var(--la-line)') +
        ';border-radius:8px;padding:8px 10px;margin-bottom:8px;background:var(--la-panel);cursor:pointer'
    });
    const nameRow = el('div', { style: 'font-weight:600;font-size:13px;display:flex;align-items:center' });
    nameRow.appendChild(document.createTextNode(p.name));
    if (p.version) nameRow.appendChild(el('span', {
      style: 'color:var(--la-dim);font-size:11px;font-family:var(--la-mono);margin-left:6px;font-weight:400',
      text: 'v' + p.version
    }));
    nameRow.appendChild(stateBadge(p.state));
    if (p.autostart) nameRow.appendChild(el('span', {
      text: 'autostart',
      style: 'font-size:9px;font-family:var(--la-mono);padding:1px 5px;border-radius:4px;border:1px solid #7aa2f766;color:#7aa2f7;margin-left:6px'
    }));
    // Plugin enable switch (ADR-0032). Autostart cores may be turned off — warn, do not block.
    const enabled = p.state !== 'disabled' && !p.disabled;
    const sw = el('button', {
      type: 'button',
      text: enabled ? 'On' : 'Off',
      title: enabled
        ? '关闭插件：立即卸载并禁止再挂（Autostart/ensure 均生效）'
        : '启用插件：清除禁用标记；何时再挂载取决于 Autostart / scheme',
      style: 'margin-left:auto;cursor:pointer;border-radius:6px;padding:2px 8px;font-size:11px;font-family:var(--la-mono);' +
        (enabled
          ? 'background:#16281a;border:1px solid #9ece6a66;color:#9ece6a'
          : 'background:#1a1a1a;border:1px solid #4b5563;color:#9aa0a6')
    });
    sw.addEventListener('click', async (e) => {
      e.stopPropagation();
      if (enabled && p.autostart) {
        if (!window.confirm('「' + p.name + '」是 autostart 核心插件，关闭后聊天/装配可能不可用。仍要关闭？')) return;
      }
      sw.disabled = true;
      try {
        const res = await LiteAgent.callCap('host', 'host', 'setPluginEnabled', {
          name: p.name,
          enabled: !enabled
        });
        if (!res || res.ok === false) {
          window.alert('切换失败：' + ((res && res.error) || 'unknown'));
          return;
        }
        LiteAgent.emit('__notice', {
          text: 'plugin ' + p.name + ' → ' + (!enabled ? 'on' : 'off (disabled)'),
          cls: 'message'
        });
        LiteAgent.emit('__plugin-switch', { name: p.name, enabled: !enabled });
        invalidatePluginsCache();
        // Re-open the panel so the graph/state refreshes immediately.
        const rootEl = document.getElementById('main-overlay');
        if (rootEl) rootEl.innerHTML = '';
        openPluginsPanel();
      } catch (err) {
        window.alert('切换失败：' + err);
      } finally {
        sw.disabled = false;
      }
    });
    nameRow.appendChild(sw);
    card.appendChild(nameRow);
    if (p.description) {
      card.appendChild(el('div', { style: 'font-size:11px;color:var(--la-dim);margin-top:4px;line-height:1.35', text: p.description }));
    }
    card.appendChild(el('div', { style: 'font-size:10px;color:var(--la-dim);margin-top:6px;font-family:var(--la-mono)', text: 'provides' }));
    card.appendChild(chips(p.provides, 'var(--la-ok)', '#9ece6a44'));
    card.appendChild(el('div', { style: 'font-size:10px;color:var(--la-dim);margin-top:6px;font-family:var(--la-mono)', text: 'consumes' }));
    card.appendChild(chips(p.consumes, 'var(--la-accent)', '#7aa2f744'));

    const node = (graph.nodes || []).find(n => n.id === p.name);
    if (node && node.dependsOn && node.dependsOn.length) {
      card.appendChild(el('div', { style: 'font-size:10px;color:var(--la-dim);margin-top:6px;font-family:var(--la-mono)', text: 'dependsOn' }));
      card.appendChild(chips(node.dependsOn, '#56b6c2', '#56b6c255'));
    }
    if (p.schemes && p.schemes.length) {
      card.appendChild(el('div', { style: 'font-size:10px;color:var(--la-dim);margin-top:6px;font-family:var(--la-mono)', text: 'pulled by scheme' }));
      card.appendChild(chips(p.schemes, '#c678dd', '#c678dd55'));
    }
    if (p.tools && p.tools.length) {
      card.appendChild(el('div', { style: 'font-size:10px;color:var(--la-dim);margin-top:6px;font-family:var(--la-mono)', text: 'tools' }));
      card.appendChild(chips(p.tools, '#e0af68', '#e0af6855'));
    }
    if (node && node.unmet && node.unmet.length) {
      card.appendChild(el('div', { style: 'font-size:10px;color:var(--la-dim);margin-top:6px;font-family:var(--la-mono)', text: 'unmet' }));
      card.appendChild(chips(node.unmet, 'var(--la-err)', '#f7768e55'));
    }
    if (node && node.mounts && node.mounts.length) {
      card.appendChild(el('div', { style: 'font-size:10px;color:var(--la-dim);margin-top:6px;font-family:var(--la-mono)', text: 'UI mounts' }));
      node.mounts.forEach(m => {
        card.appendChild(el('div', {
          style: 'font-size:11px;color:var(--la-ink);font-family:var(--la-mono);margin-top:2px',
          text: m.page + '/' + m.slot + ' → ' + m.component
        }));
      });
    }
    if (p.commands && p.commands.length) {
      card.appendChild(el('div', { style: 'font-size:10px;color:var(--la-dim);margin-top:6px;font-family:var(--la-mono)', text: 'commands' }));
      p.commands.forEach(c => {
        card.appendChild(el('div', {
          style: 'font-size:11px;color:var(--la-dim);font-family:var(--la-mono)',
          text: '/' + c.name
        }));
      });
    }
    card.addEventListener('click', () => onSelect(isSel ? null : p.name));
    side.appendChild(card);
  });

  // Relationship list for selection (any node kind)
  const rel = selected ? relatedTo(graph.edges || [], selected) : [];
  if (selected) {
    const box = el('div', { style: 'margin-top:12px;border-top:1px solid var(--la-line);padding-top:10px' });
    box.appendChild(el('div', {
      style: 'font-size:11px;text-transform:uppercase;letter-spacing:.06em;color:var(--la-dim);margin-bottom:6px;font-family:var(--la-mono)',
      text: 'Relations · ' + selected
    }));
    if (!rel.length) {
      box.appendChild(el('div', { style: 'font-size:12px;color:var(--la-dim)', text: 'No edges.' }));
    }
    rel.forEach(e => {
      const dir = e.from === selected ? '→' : '←';
      const other = e.from === selected ? e.to : e.from;
      const meta = [e.kind, e.capability, e.scheme, e.component].filter(Boolean).join(' · ');
      box.appendChild(el('div', {
        style: 'font-size:11px;font-family:var(--la-mono);color:var(--la-ink);margin-bottom:4px;line-height:1.35',
        text: dir + ' ' + other + (meta ? '  (' + meta + ')' : '')
      }));
    });
    const clear = el('button', {
      type: 'button', text: 'Clear selection',
      style: 'margin-top:8px;background:transparent;border:1px solid var(--la-line);color:var(--la-dim);border-radius:6px;padding:4px 8px;font-size:11px;cursor:pointer'
    });
    clear.onclick = () => onSelect(null);
    box.appendChild(clear);
    side.appendChild(box);
  }

  const legend = el('div', { style: 'display:flex;flex-direction:column;gap:6px;font-size:11px;color:var(--la-dim);margin-top:12px;font-family:var(--la-mono)' });
  legend.innerHTML =
    '<span style="color:var(--la-dim)">state</span>' +
    '<span><i style="display:inline-block;width:10px;height:10px;border:1px solid #9ece6a;background:#161a22;vertical-align:middle;margin-right:5px"></i>mounted（已挂载）</span>' +
    '<span><i style="display:inline-block;width:10px;height:10px;border:1px dashed #4b5563;background:#12161f;vertical-align:middle;margin-right:5px"></i>available（待 scheme 拉起）</span>' +
    '<span><i style="display:inline-block;width:10px;height:10px;border:1px dashed #f7768e;background:#2a1620;vertical-align:middle;margin-right:5px"></i>degraded（consumes 未满足）</span>' +
    '<span><i style="display:inline-block;width:10px;height:10px;border:1px dashed #6b7280;background:#1a1a1a;vertical-align:middle;margin-right:5px"></i>disabled（用户关闭）</span>' +
    '<span style="color:var(--la-dim);margin-top:4px">edges</span>' +
    '<span><i style="display:inline-block;width:18px;height:2px;background:#9ece6a;vertical-align:middle;margin-right:4px"></i>provides (plugin → cap)</span>' +
    '<span><i style="display:inline-block;width:18px;height:2px;background:#7aa2f7;vertical-align:middle;margin-right:4px"></i>host-uses (cap → Host)</span>' +
    '<span><i style="display:inline-block;width:18px;height:0;border-top:2px dashed #bb9af7;vertical-align:middle;margin-right:4px"></i>consumes (cap → plugin)</span>' +
    '<span><i style="display:inline-block;width:18px;height:2px;background:#56b6c2;vertical-align:middle;margin-right:4px"></i>depends-on (plugin → plugin)</span>' +
    '<span><i style="display:inline-block;width:18px;height:0;border-top:2px dotted #c678dd;vertical-align:middle;margin-right:4px"></i>scheme (agent → plugin)</span>' +
    '<span><i style="display:inline-block;width:18px;height:0;border-top:2px dotted #e0af68;vertical-align:middle;margin-right:4px"></i>ui-mount (plugin → slot)</span>';
  side.appendChild(legend);
}

function paint(root, data, close) {
  const graph = data.graph || { nodes: [], edges: [] };
  const summary = data.summary || {};
  let selected = null;

  const body = el('div', { class: 'pg-modal-body' });
  const side = el('div', {
    style: 'width:300px;flex-shrink:0;border-right:1px solid var(--la-line);overflow-y:auto;padding:12px;background:var(--la-panel2)'
  });
  const canvas = el('div', {
    style: 'flex:1;min-width:0;overflow:auto;padding:8px;display:flex;align-items:flex-start;justify-content:center'
  });
  body.appendChild(side);
  body.appendChild(canvas);

  function refresh() {
    renderDetail(side, data, selected, (id) => { selected = id; refresh(); });
    drawGraph(canvas, graph, selected, (id) => { selected = id; refresh(); });
  }

  root.innerHTML = '';
  const backdrop = el('div', { class: 'pg-backdrop' });
  const modal = el('div', { class: 'pg-modal', style: 'width:min(1080px,100%)' });
  const head = el('div', { class: 'pg-modal-head' });
  const title = el('span', { style: 'display:flex;align-items:center;gap:8px' });
  title.appendChild(document.createTextNode('Plugins · capabilities · Host'));
  title.appendChild(el('span', {
    style: 'font-size:11px;font-weight:400;color:var(--la-dim);font-family:var(--la-mono)',
    text: 'mounted ' + (summary.mounted != null ? summary.mounted : '?') +
      ' / ' + (summary.discovered != null ? summary.discovered : '?') +
      (summary.disabled && summary.disabled.length ? ' · off ' + summary.disabled.length : '') +
      (data.scheme ? ' · scheme ' + data.scheme : '')
  }));
  const btnClose = el('button', { type: 'button', text: 'Close' });
  btnClose.onclick = close;
  head.appendChild(title);
  head.appendChild(btnClose);
  modal.appendChild(head);
  modal.appendChild(body);
  backdrop.appendChild(modal);
  backdrop.addEventListener('click', (e) => { if (e.target === backdrop) close(); });
  root.appendChild(backdrop);

  try {
    refresh();
  } catch (e) {
    // A render bug must never leave a blank canvas behind.
    canvas.innerHTML = '';
    canvas.appendChild(el('div', {
      style: 'padding:16px;color:var(--la-err);font-size:12px;font-family:var(--la-mono);white-space:pre-wrap',
      text: 'graph render failed: ' + ((e && e.message) || e)
    }));
  }
}

// The last /api/plugins snapshot, prefetched at Shell boot so the panel paints
// synchronously when opened.
let snapshot = null;
let keyHandler = null;

function fetchPlugins(force) {
  if (snapshot && !force) return Promise.resolve(snapshot);
  return fetch('/api/plugins').then(r => r.json()).then(data => {
    if (data && data.graph) snapshot = data;
    return snapshot;
  });
}

// prefetchPlugins warms the cache during Shell boot: the dependency graph is
// ready before the first Turn, not after it.
export function prefetchPlugins() {
  return fetchPlugins(true).catch(() => null);
}

// invalidatePluginsCache drops the snapshot so the next open/refresh refetches.
// Called when scheme or session changes — ensurePlugins may have mounted more.
export function invalidatePluginsCache() {
  snapshot = null;
}

// Live invalidation: scheme switch / session select / plugin switch can mount or unmount.
if (window.LiteAgent && window.LiteAgent.on) {
  window.LiteAgent.on('__scheme', () => { invalidatePluginsCache(); prefetchPlugins(); });
  window.LiteAgent.on('__session', () => { invalidatePluginsCache(); prefetchPlugins(); });
  window.LiteAgent.on('__plugin-switch', () => { invalidatePluginsCache(); prefetchPlugins(); });
}

export function openPluginsPanel() {
  const root = document.getElementById('main-overlay');
  if (!root) return;
  const closePanel = () => {
    root.innerHTML = '';
    if (keyHandler) document.removeEventListener('keydown', keyHandler);
    keyHandler = null;
  };
  if (root.querySelector('.pg-backdrop')) {
    closePanel();
    return;
  }
  keyHandler = (e) => { if (e.key === 'Escape') closePanel(); };
  document.addEventListener('keydown', keyHandler);

  // Paint from the prefetched snapshot immediately; refresh in the background
  // so newly ensured plugins show up without waiting for a reload.
  if (snapshot) {
    paint(root, snapshot, closePanel);
    fetchPlugins(true).then(fresh => {
      if (!fresh || !root.querySelector('.pg-backdrop')) return;
      paint(root, fresh, closePanel);
    }).catch(() => { });
    return;
  }
  root.innerHTML = '';
  root.appendChild(el('div', { class: 'pg-backdrop' }, [
    el('div', { class: 'pg-modal' }, [
      el('div', { class: 'pg-modal-head' }, [
        el('span', { text: 'Plugins · capabilities · Host' }),
        el('span', { style: 'color:var(--la-dim);font-size:12px;font-weight:400', text: 'loading…' })
      ])
    ])
  ]));
  fetchPlugins(true).then(data => {
    if (data) paint(root, data, closePanel);
  }).catch(e => {
    root.innerHTML = '';
    const backdrop = el('div', { class: 'pg-backdrop' });
    const modal = el('div', { class: 'pg-modal' });
    const close = el('button', { type: 'button', text: 'Close' });
    close.onclick = closePanel;
    modal.appendChild(el('div', { class: 'pg-modal-head' }, [
      el('span', { text: 'Plugins · capabilities · Host' }), close
    ]));
    modal.appendChild(el('div', { style: 'padding:20px;color:var(--la-err);font-size:13px', text: 'Failed to load /api/plugins: ' + e }));
    backdrop.appendChild(modal);
    root.appendChild(backdrop);
  });
}
