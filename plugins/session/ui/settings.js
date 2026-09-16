/* session plugin Settings face (independent render). */
const NAME = new URL(import.meta.url).searchParams.get('plugin') || 'session';

const CSS = `
:host { display:block; }
h3 { margin:0 0 4px; font-size:14px; color:var(--la-ink); }
.desc { font-size:12px; color:var(--la-dim); margin:0 0 14px; }
pre {
  background:var(--la-bg); border:1px solid var(--la-line); border-radius:8px;
  padding:10px 12px; font-size:11px; font-family:var(--la-mono); color:var(--la-dim);
  overflow:auto; max-height:280px;
}
`;

class SessionSettings extends HTMLElement {
  async connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    const style = document.createElement('style');
    style.textContent = CSS;
    root.appendChild(style);
    const h = document.createElement('h3');
    h.textContent = 'session';
    root.appendChild(h);
    const d = document.createElement('p');
    d.className = 'desc';
    d.textContent = 'Session Log 插件。数据目录由进程环境决定；会话列表见左侧 rail。';
    root.appendChild(d);
    const pre = document.createElement('pre');
    pre.textContent = 'loading…';
    root.appendChild(pre);
    try {
      const res = await fetch('/api/call', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ plugin: NAME, cap: 'session', method: 'list', payload: {} })
      });
      const b = await res.json();
      const r = b && b.result;
      const sessions = (r && r.sessions) || [];
      pre.textContent = 'sessions: ' + sessions.length + '\n' +
        sessions.slice(0, 20).map(s => '  ' + (s.id || '') + (s.workspace ? '  ws=' + s.workspace : '')).join('\n');
    } catch (e) {
      pre.textContent = String(e);
    }
  }
}

if (!customElements.get('session-settings')) customElements.define('session-settings', SessionSettings);
