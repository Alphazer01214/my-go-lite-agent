/* LiteAgent 数据流转 · mermaid sequenceDiagram 子集渲染器
 *
 * 站点不引 npm、不引 CDN。这里实现 mermaid sequenceDiagram 的一个子集：
 *   participant X as Label
 *   A->>B: 调用            实线 + 箭头
 *   A-->>B: 返回           虚线 + 箭头
 *   A->>A: 自调用
 *   Note over A,B: 文本
 *   Note right of A: 文本
 *
 * 输出 SVG。语法与仓库 docs/*.md 里的 ```mermaid 块一致，
 * 所以同一段源码既能在 GitHub 渲染，也能在本页渲染。
 */

(function () {
  'use strict';

  const CJK = /[\u2e80-\u9fff\uf900-\ufaff\uff00-\uffef\u3000-\u303f]/;

  /* 半角单位宽度：CJK 记 2，其余记 1。 */
  function units(s) {
    let u = 0;
    for (const ch of String(s)) u += CJK.test(ch) ? 2 : 1;
    return u;
  }

  /* 按词折行。中文没有空格，按字符断；英文优先在空格处断。 */
  function wrap(text, maxUnits) {
    const out = [];
    for (const para of String(text).split(/<br\s*\/?>/i)) {
      const words = para.split(/\s+/).filter(Boolean);
      let line = '';
      let w = 0;
      for (const word of words) {
        const ww = units(word);
        if (ww > maxUnits) {
          /* 单个词就超宽：先结算当前行，再按字符硬断。 */
          if (line) { out.push(line); line = ''; w = 0; }
          let buf = '';
          let bw = 0;
          for (const ch of word) {
            const cw = CJK.test(ch) ? 2 : 1;
            if (bw + cw > maxUnits && buf) { out.push(buf); buf = ''; bw = 0; }
            buf += ch;
            bw += cw;
          }
          line = buf;
          w = bw;
          continue;
        }
        const sp = line ? 1 : 0;
        if (w + sp + ww > maxUnits && line) {
          out.push(line);
          line = word;
          w = ww;
        } else {
          line += (line ? ' ' : '') + word;
          w += sp + ww;
        }
      }
      out.push(line);
    }
    return out.length ? out : [''];
  }

  function esc(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  /* ---------- 解析 ---------- */

  function parse(src) {
    const participants = [];
    const events = [];
    const byId = {};
    const lines = String(src).split(/\r?\n/);

    function ensure(id, label) {
      if (byId[id]) {
        if (label && byId[id].label === byId[id].id) byId[id].label = label;
        return byId[id];
      }
      const p = { id, label: label || id };
      byId[id] = p;
      participants.push(p);
      return p;
    }

    for (const raw of lines) {
      const line = raw.trim();
      if (!line || line.startsWith('%%')) continue;
      if (/^sequenceDiagram$/i.test(line)) continue;

      let m;
      if ((m = line.match(/^participant\s+([A-Za-z0-9_]+)(?:\s+as\s+(.+))?$/))) {
        ensure(m[1], (m[2] || '').trim());
        continue;
      }
      if ((m = line.match(/^Note\s+(over|right of|left of)\s+([A-Za-z0-9_]+)(?:\s*,\s*([A-Za-z0-9_]+))?\s*:\s*(.*)$/i))) {
        const a = ensure(m[2]);
        const b = m[3] ? ensure(m[3]) : null;
        events.push({ kind: 'note', mode: m[1].toLowerCase(), a, b, text: m[4] });
        continue;
      }
      if ((m = line.match(/^([A-Za-z0-9_]+)\s*(-->>|->>|-->|->|-\)|--\))\s*([A-Za-z0-9_]+)\s*:\s*(.*)$/))) {
        const from = ensure(m[1]);
        const to = ensure(m[3]);
        events.push({ kind: 'msg', from, to, arrow: m[2], text: m[4] });
        continue;
      }
      /* 未知行忽略，保持渲染器容错。 */
    }
    return { participants, events };
  }

  /* ---------- 布局 ---------- */

  const PAD_X = 52;
  const HEADER_H = 54;
  const LABEL_MAX = 34;
  const LINE_H = 17;
  const ROW_MIN = 30;
  const ROW_PAD = 14;
  const SELF_W = 34;

  function arrowKind(arrow) {
    if (arrow === '->>' || arrow === '->') return 'req';
    if (arrow === '-->>' || arrow === '-->') return 'res';
    return 'req';
  }

  function isDotted(arrow) {
    return arrow === '-->>' || arrow === '-->';
  }

  function render(src, opts) {
    const o = opts || {};
    const { participants, events } = parse(src);

    /* 等宽列，参与者名字过长时整体加宽。 */
    const uniform = Math.max.apply(null, participants.map((p) => units(p.label) * 7.6 + 34).concat([150]));
    const x = participants.map((_, i) => PAD_X + i * uniform + uniform / 2);
    const baseWidth = PAD_X * 2 + participants.length * uniform;

    /* 第一遍：排版，算出每行位置与总高度。 */
    const rows = [];
    let y = HEADER_H;
    let msgIndex = 0;
    events.forEach((ev) => {
      if (ev.kind === 'note') {
        const lines = wrap(ev.text, LABEL_MAX * 2);
        rows.push({ ev, lines, top: y + 2, h: lines.length * LINE_H + ROW_PAD - 4 });
        y += lines.length * LINE_H + ROW_PAD;
        return;
      }
      const lines = wrap(ev.text, LABEL_MAX);
      msgIndex++;
      rows.push({
        ev, lines, i: msgIndex,
        labelTop: y + 4,
        lineY: y + 4 + lines.length * LINE_H + 4,
        h: Math.max(ROW_MIN, lines.length * LINE_H + ROW_PAD),
      });
      y += Math.max(ROW_MIN, lines.length * LINE_H + ROW_PAD);
    });
    const height = y + 18;

    /* 第二遍：出图。生命线先画，压在消息下面。 */
    const parts = [];
    parts.push(
      '<defs>' +
      '<marker id="mk-req" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">' +
      '<path d="M0,1 L9,5 L0,9 z" fill="#43d9c0"/></marker>' +
      '<marker id="mk-res" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">' +
      '<path d="M0,1 L9,5 L0,9 z" fill="#8ce99a"/></marker>' +
      '<marker id="mk-self" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">' +
      '<path d="M0,1 L9,5 L0,9 z" fill="#93a5b8"/></marker>' +
      '</defs>'
    );

    participants.forEach((p, i) => {
      parts.push('<line class="sq-life" x1="' + x[i] + '" y1="' + HEADER_H + '" x2="' + x[i] + '" y2="' + (height - 10) + '"/>');
    });

    participants.forEach((p, i) => {
      const cx = x[i];
      const w = Math.max(uniform - 16, units(p.label) * 7.6 + 22);
      parts.push(
        '<rect class="sq-box" x="' + (cx - w / 2) + '" y="8" width="' + w + '" height="34" rx="4"/>' +
        '<text class="sq-title" x="' + cx + '" y="30" text-anchor="middle">' + esc(p.label) + '</text>'
      );
    });

    let contentRight = baseWidth;

    rows.forEach((row) => {
      const ev = row.ev;
      if (ev.kind === 'note') {
        const left0 = x[participants.indexOf(ev.a)];
        let left, right;
        const ia = participants.indexOf(ev.a);
        /* 文字宽度决定盒子最小宽度，Note over 至少要装得下整段文字。 */
        const textW = Math.max.apply(null, row.lines.map((l) => units(l) * 7.0)) + 22;
        if (ev.mode === 'over') {
          const ib = ev.b ? participants.indexOf(ev.b) : ia;
          left = x[Math.min(ia, ib)] - uniform / 2 + 8;
          right = x[Math.max(ia, ib)] + uniform / 2 - 8;
          if (right - left < textW) {
            const c = (left + right) / 2;
            left = c - textW / 2;
            right = c + textW / 2;
          }
        } else if (ev.mode === 'right of') {
          left = left0 + 14;
          right = left + Math.max(textW, 130);
        } else {
          right = left0 - 14;
          left = right - Math.max(textW, 130);
        }
        parts.push('<rect class="sq-note" x="' + left + '" y="' + row.top + '" width="' + (right - left) + '" height="' + row.h + '" rx="4"/>');
        row.lines.forEach((ln, li) => {
          parts.push('<text class="sq-notetext" x="' + (left + 10) + '" y="' + (row.top + 16 + li * LINE_H) + '">' + esc(ln) + '</text>');
        });
        if (right > contentRight) contentRight = right;
        return;
      }

      const ia = participants.indexOf(ev.from);
      const ib = participants.indexOf(ev.to);
      const kind = arrowKind(ev.arrow);
      const dotted = isDotted(ev.arrow);
      const cls = 'sq-msg sq-' + kind + (dotted ? ' sq-dash' : '') + (o.onSelect !== false ? ' clickable' : '');
      const attr = o.onSelect !== false ? ' data-i="' + row.i + '"' : '';

      if (ia === ib) {
        const sx = x[ia];
        parts.push(
          '<path class="' + cls + '"' + attr + ' d="M' + sx + ',' + (row.lineY - 15) + ' h' + SELF_W + ' v15 h-' + SELF_W + '"/>' +
          '<path class="' + cls + '" d="M' + (sx + SELF_W) + ',' + (row.lineY - 15) + ' v15" marker-end="url(#mk-self)"/>'
        );
        row.lines.forEach((ln, li) => {
          parts.push('<text class="sq-text sq-text-self" x="' + (sx + SELF_W + 10) + '" y="' + (row.labelTop + LINE_H - 4 + li * LINE_H) + '">' + esc(ln) + '</text>');
        });
      } else {
        const x1 = x[ia];
        const x2 = x[ib];
        const dir = x2 > x1 ? 1 : -1;
        parts.push(
          '<line class="' + cls + '"' + attr + ' x1="' + (x1 + dir * 3) + '" y1="' + row.lineY + '" x2="' + (x2 - dir * 3) + '" y2="' + row.lineY + '"' +
          ' marker-end="url(#mk-' + kind + ')"/>'
        );
        row.lines.forEach((ln, li) => {
          const ty = row.labelTop + LINE_H - 4 + li * LINE_H;
          parts.push('<text class="sq-text" x="' + ((x1 + x2) / 2) + '" y="' + ty + '" text-anchor="middle">' + esc(ln) + '</text>');
        });
      }
    });

    return (
      '<svg class="sq-svg" viewBox="0 0 ' + (contentRight + PAD_X) + ' ' + height + '" width="100%" ' +
      'preserveAspectRatio="xMidYMin meet" role="img">' +
      parts.join('') + '</svg>'
    );
  }

  window.LASeq = { render: render, parse: parse, wrap: wrap, units: units };
})();
