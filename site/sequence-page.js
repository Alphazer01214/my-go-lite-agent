/* LiteAgent 数据流转 · 页面逻辑 */

(function () {
  'use strict';

  const $ = (s, r) => (r || document).querySelector(s);
  const $$ = (s, r) => Array.prototype.slice.call((r || document).querySelectorAll(s));
  const esc = (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  function heroFacts() {
    const facts = [
      { k: '15', t: '张序列图', d: '从完整数据流到 Frame 闭环、确认与委派' },
      { k: 'fwd-N', t: 'pending 键', d: 'Host 分配的内部 id，不是插件的 id' },
      { k: '1', t: '条终态 Response', d: '每条 Request 最终只得到一条' },
      { k: '0', t: '插件直连', d: '任何跨插件调用都经 Host 路由' },
    ];
    $('#heroFacts').innerHTML = facts.map((f) =>
      '<li><b>' + esc(f.k) + '</b><span>' + esc(f.t) + '</span><span class="fd">' + esc(f.d) + '</span></li>'
    ).join('');
  }

  function table(host, head, rows) {
    $(host).innerHTML =
      '<thead><tr>' + head.map((h) => '<th>' + esc(h) + '</th>').join('') + '</tr></thead><tbody>' +
      rows.map((r) => '<tr>' + r.map((c) => '<td>' + esc(c) + '</td>').join('') + '</tr>').join('') +
      '</tbody>';
  }

  const DEFAULT_DETAIL =
    '<div class="sq-step"><span class="sq-stepno">?</span><div>' +
    '<p class="sq-stept">怎么读这张图</p>' +
    '<p class="sq-stepb">实线是一次请求发出，虚线是一次返回或归因事件。' +
    '点图里任意一条消息，这里会展开那一跳：谁写的 Frame、id 怎么被改写、pending 键是什么、失败时会落到哪个 error_code。</p>' +
    '</div></div>';

  function detailPanel(diag, steps, i) {
    const s = steps && steps[i];
    if (!s) return DEFAULT_DETAIL;
    return '<div class="sq-step">' +
      '<span class="sq-stepno">' + i + '</span>' +
      '<div><p class="sq-stept">' + esc(s.t) + '</p>' +
      '<p class="sq-stepb">' + esc(s.b) + '</p>' +
      (s.k ? '<p class="sq-stepk"><code>' + esc(s.k) + '</code></p>' : '') +
      '</div></div>';
  }

  function renderDiagram(key, diag) {
    const steps = diag.steps || {};
    const card = document.createElement('article');
    card.className = 'seq-card';
    card.id = 'd-' + key;

    const plot = document.createElement('div');
    plot.className = 'seq-plot';
    plot.innerHTML =
      '<div class="sq-legend">' +
      '<span><i class="sq-ln sq-ln-req"></i>调用</span>' +
      '<span><i class="sq-ln sq-ln-res"></i>返回 / 事件</span>' +
      '<span class="sq-legend-hint">点任意一条消息看那一跳</span>' +
      '</div>' +
      window.LASeq.render(diag.mermaid, { onSelect: true });

    const detail = document.createElement('aside');
    detail.className = 'seq-detail';
    detail.setAttribute('aria-live', 'polite');
    detail.innerHTML = detailPanel(diag, steps, 0);

    const src = document.createElement('details');
    src.className = 'seq-src';
    src.innerHTML = '<summary>mermaid 源码</summary><pre class="code">' + esc(diag.mermaid) + '</pre>';

    const head = document.createElement('header');
    head.className = 'seq-head';
    head.innerHTML =
      '<div class="seq-headtext"><h3>' + esc(diag.title) + '</h3>' +
      '<p class="where mono">' + esc(diag.where) + '</p>' +
      '<p class="lead">' + esc(diag.lead) + '</p></div>' +
      '<span class="seq-count">' + Object.keys(steps).length + ' 跳</span>';

    const body = document.createElement('div');
    body.className = 'seq-body';
    body.appendChild(plot);
    body.appendChild(detail);

    card.appendChild(head);
    card.appendChild(body);
    card.appendChild(src);

    /* 点消息 → 填细节面板，并高亮该行。 */
    plot.addEventListener('click', (e) => {
      const el = e.target.closest('[data-i]');
      if (!el) return;
      const i = +el.getAttribute('data-i');
      detail.innerHTML = detailPanel(diag, steps, i);
      $$('[data-i]', plot).forEach((n) => n.classList.remove('sel'));
      el.classList.add('sel');
    });

    return card;
  }

  function renderParts() {
    const host = $('#parts');
    LA_SEQ.parts.forEach((part) => {
      const sec = document.createElement('section');
      sec.className = 'seq-part';
      sec.id = 'part-' + part.id;

      const head = document.createElement('header');
      head.className = 'block-head';
      head.innerHTML = '<span class="idx">' + esc(part.n) + '</span><div><h3 class="seq-parttitle">' +
        esc(part.title) + '</h3><p class="block-lede">' + esc(part.lead) + '</p></div>';
      sec.appendChild(head);

      Object.keys(LA_SEQ.diagrams).forEach((key) => {
        if (LA_SEQ.diagramParts[key] !== part.id) return;
        sec.appendChild(renderDiagram(key, LA_SEQ.diagrams[key]));
      });

      host.appendChild(sec);
    });
  }

  function renderIndex() {
    const items = [];
    LA_SEQ.parts.forEach((part) => {
      items.push('<li class="grp">' + esc(part.n + ' · ' + part.title) + '</li>');
      Object.keys(LA_SEQ.diagrams).forEach((key) => {
        if (LA_SEQ.diagramParts[key] !== part.id) return;
        items.push('<li><a href="#d-' + key + '">' + esc(LA_SEQ.diagrams[key].title) + '</a></li>');
      });
    });
    $('#diagramIndex').innerHTML = items.join('');
  }

  function initNav() {
    const links = $$('.nav a[href^="#"]');
    const map = {};
    links.forEach((a) => { map[a.getAttribute('href').slice(1)] = a; });
    const obs = new IntersectionObserver((es) => {
      es.forEach((e) => {
        if (e.intersectionRatio < 0.15) return;
        links.forEach((a) => a.classList.remove('here'));
        const l = map[e.target.id];
        if (l) l.classList.add('here');
      });
    }, { rootMargin: '-40% 0px -55% 0px', threshold: [0.15, 0.5] });
    ['overview'].concat(Object.keys(map)).forEach((id) => {
      const s = document.getElementById(id);
      if (s) obs.observe(s);
    });
  }

  document.addEventListener('DOMContentLoaded', () => {
    heroFacts();
    table('#frameFields', ['字段', '取值', '谁写', '含义'], LA_SEQ.frameFields);
    table('#pendingKeys', ['场景', 'pending 键', 'Frame.id 还原成什么', '说明'], LA_SEQ.pendingKeys);
    table('#errPoints', ['error_code', '在哪一跳被写', '说明'], LA_SEQ.errPoints);
    table('#pluginCodes', ['插件', '错误码', '什么时候出现'], LA_SEQ.pluginCodes);
    renderParts();
    renderIndex();
    initNav();
  });
})();
