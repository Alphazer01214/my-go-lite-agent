/* uidemo UI Entry (ADR-0010 reference, multi-file assets per ADR-0011).
 *
 * Demonstrates the full Panel Component contract in one plugin:
 *   1. static mount      — manifest ui.mounts places uidemo-mode-panel in sidebar
 *   2. Shadow DOM + taps — styles/templates are separate ui/ files; theme via --la-*
 *   3. UI Action         — buttons call LiteAgent.emitUIAction → plugin's ui.action
 *   4. dynamic PanelOp   — plugin replies with EmitPanel set of uidemo-echo-panel
 *   5. session awareness — LiteAgent.onSessionChange keeps the footer current
 *
 * Asset loading: ui.assets are declared in plugin.json and fetched relative
 * to this module's URL; CSS is adopted, templates are cloned into the shadow.
 */
const NAME = new URL(import.meta.url).searchParams.get('plugin') || 'uidemo';

let sheetPromise = null;
function sharedSheet() {
  if (!sheetPromise) {
    sheetPromise = fetch(new URL('panels.css', import.meta.url))
      .then(r => r.text())
      .then(text => {
        const sheet = new CSSStyleSheet();
        sheet.replaceSync(text);
        return sheet;
      });
  }
  return sheetPromise;
}

async function template(file) {
  const res = await fetch(new URL(file, import.meta.url));
  const doc = new DOMParser().parseFromString(await res.text(), 'text/html');
  return doc.querySelector('template');
}

// mode panel: static mount target; emits UI Actions when the mode changes.
class UIDemoModePanel extends HTMLElement {
  constructor() {
    super();
    this._mode = '';
  }
  async connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    root.adoptedStyleSheets = [await sharedSheet()];
    root.appendChild((await template('mode-panel.html')).content.cloneNode(true));
    if (!this._mode) this._mode = (this.props && this.props.initial) || 'chat';
    this._render();
    this._offSession = LiteAgent.onSessionChange(id => this._session(id));
    this._session(window.__liteSessionId);
  }
  disconnectedCallback() { if (this._offSession) this._offSession(); }
  // The Shell property-assigns props after createElement; re-render if live.
  set props(v) {
    this._props = v || {};
    if (this._mode === '') this._mode = this._props.initial || 'chat';
    if (this.shadowRoot && this.shadowRoot.querySelector('.row')) this._render();
  }
  get props() { return this._props || {}; }
  _render() {
    const row = this.shadowRoot.querySelector('.row');
    row.textContent = '';
    for (const m of ['chat', 'agent']) {
      const b = document.createElement('button');
      b.textContent = m[0].toUpperCase() + m.slice(1);
      if (m === this._mode) b.classList.add('on');
      b.onclick = () => {
        this._mode = m;
        this._render();
        LiteAgent.emitUIAction(NAME, 'mode', 'set', m);
      };
      row.appendChild(b);
    }
    this.shadowRoot.querySelector('.mode').textContent = this._mode;
  }
  _session(id) {
    const el = this.shadowRoot && this.shadowRoot.querySelector('.sess');
    if (el) el.textContent = id || '(default)';
  }
}

// echo panel: mounted by the plugin's dynamic PanelOp reply (toolbar-right).
class UIDemoEchoPanel extends HTMLElement {
  constructor() {
    super();
  }
  async connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    root.adoptedStyleSheets = [await sharedSheet()];
    root.appendChild((await template('echo-panel.html')).content.cloneNode(true));
    this._render();
  }
  set props(v) {
    this._props = v || {};
    if (this.shadowRoot && this.shadowRoot.querySelector('.meta')) this._render();
  }
  get props() { return this._props || {}; }
  _render() {
    const meta = this.shadowRoot.querySelector('.meta');
    const p = this._props || {};
    meta.textContent = 'echo · mode=' + (p.mode || '?') + ' event=' + (p.event || '?');
  }
}

// Reload-safe registration: define throws on duplicate names.
if (!customElements.get('uidemo-mode-panel')) customElements.define('uidemo-mode-panel', UIDemoModePanel);
if (!customElements.get('uidemo-echo-panel')) customElements.define('uidemo-echo-panel', UIDemoEchoPanel);
