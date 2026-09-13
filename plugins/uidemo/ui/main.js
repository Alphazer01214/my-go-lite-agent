/* uidemo UI Entry (ADR-0010 reference implementation).
 *
 * Demonstrates the full Panel Component contract in one plugin:
 *   1. static mount      — manifest ui.mounts places uidemo-mode-panel in sidebar
 *   2. Shadow DOM + taps — styles are local; theme comes from --la-* tokens
 *   3. UI Action         — buttons call LiteAgent.emitUIAction → plugin's ui.action
 *   4. dynamic PanelOp   — plugin replies with EmitPanel set of uidemo-echo-panel
 *   5. session awareness — LiteAgent.onSessionChange keeps the footer current
 */
const NAME = new URL(import.meta.url).searchParams.get('plugin') || 'uidemo';

const CSS = `
  :host { display:block; font:12px/1.5 var(--la-sans, system-ui); color:var(--la-ink,#e8eaed); padding:4px 2px; }
  strong { font-size:12px; }
  .row { display:flex; gap:6px; flex-wrap:wrap; margin-top:6px; }
  button {
    background:var(--la-panel2,#12161f); color:var(--la-dim,#9aa0a6);
    border:1px solid var(--la-line,#2a2f3a); border-radius:8px;
    padding:5px 10px; font:inherit; cursor:pointer;
  }
  button:hover { color:var(--la-ink,#e8eaed); }
  button.on { background:var(--la-accent,#7aa2f7); color:#0b1020; border-color:transparent; font-weight:600; }
  .meta { margin-top:8px; font-size:11px; color:var(--la-dim,#9aa0a6); font-family:var(--la-mono,monospace); }
`;

// mode panel: static mount target; emits UI Actions when the mode changes.
class UIDemoModePanel extends HTMLElement {
  constructor(){
    super();
    this._mode = '';
    const root = this.attachShadow({mode:'open'});
    const style = document.createElement('style');
    style.textContent = CSS;
    root.appendChild(style);
  }
  connectedCallback(){
    if(!this._mode) this._mode = (this.props && this.props.initial) || 'chat';
    this._render();
    this._offSession = LiteAgent.onSessionChange(id => this._session(id));
    this._session(window.__liteSessionId);
  }
  disconnectedCallback(){ if(this._offSession) this._offSession(); }
  // The Shell property-assigns props after createElement; re-render if live.
  set props(v){
    this._props = v || {};
    if(this._mode === '') this._mode = this._props.initial || 'chat';
    if(this.shadowRoot && this.shadowRoot.querySelector('.row')) this._render();
  }
  get props(){ return this._props || {}; }
  _render(){
    const row = this.shadowRoot.querySelector('.row');
    if(row) row.remove();
    const rowEl = document.createElement('div');
    rowEl.className = 'row';
    for(const m of ['chat','agent']){
      const b = document.createElement('button');
      b.textContent = m[0].toUpperCase()+m.slice(1);
      if(m === this._mode) b.classList.add('on');
      b.onclick = () => {
        this._mode = m;
        this._render();
        LiteAgent.emitUIAction(NAME, 'mode', 'set', m);
      };
      rowEl.appendChild(b);
    }
    this.shadowRoot.appendChild(rowEl);
    if(!this.shadowRoot.querySelector('.meta')){
      const meta = document.createElement('div');
      meta.className = 'meta';
      meta.innerHTML = 'mode: <span class="mode"></span><br/>sess: <span class="sess"></span>';
      this.shadowRoot.appendChild(meta);
    }
    this.shadowRoot.querySelector('.mode').textContent = this._mode;
  }
  _session(id){
    const el = this.shadowRoot && this.shadowRoot.querySelector('.sess');
    if(el) el.textContent = id || '(default)';
  }
}

// echo panel: mounted by the plugin's dynamic PanelOp reply (toolbar-right).
class UIDemoEchoPanel extends HTMLElement {
  constructor(){
    super();
    const root = this.attachShadow({mode:'open'});
    const style = document.createElement('style');
    style.textContent = CSS;
    root.appendChild(style);
  }
  connectedCallback(){ this._render(); }
  set props(v){
    this._props = v || {};
    if(this.shadowRoot && this.shadowRoot.querySelector('.meta')) this._render();
  }
  get props(){ return this._props || {}; }
  _render(){
    let meta = this.shadowRoot.querySelector('.meta');
    if(!meta){
      meta = document.createElement('div');
      meta.className = 'meta';
      this.shadowRoot.appendChild(meta);
    }
    const p = this._props || {};
    meta.textContent = 'echo · mode=' + (p.mode || '?') + ' event=' + (p.event || '?');
  }
}

// Reload-safe registration: define throws on duplicate names.
if(!customElements.get('uidemo-mode-panel')) customElements.define('uidemo-mode-panel', UIDemoModePanel);
if(!customElements.get('uidemo-echo-panel')) customElements.define('uidemo-echo-panel', UIDemoEchoPanel);
