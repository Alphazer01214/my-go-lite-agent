// Panel Components loader (ADR-0010/0011). One instance per layout page:
// it applies static mounts declared for that page and runtime PanelOps whose
// slot this page's layout provides. Ops may arrive before the entry module
// loads; the custom element upgrade then wires the already-inserted element
// without losing props.

// Layout page vocabulary comes from the merged layout (ADR-0012/0031).
// Fallback for tests / early boot before /api/layout lands.
export var PAGES = ['main'];

export function setPages(list) {
  if (list && list.length) PAGES = list.slice();
}

function pageOf(mount) {
  return mount.page || 'main';
}

export function createLoader(page, panelHost, onError) {
  function report(msg) {
    try { onError(msg); } catch (e) { /* reporting must never throw */ }
  }

  function applyPanel(op) {
    if (!op || !op.id) return;
    var node = document.getElementById('panel-' + op.id);
    if (op.op === 'clear') { if (node) node.remove(); return; }
    if (!op.component) return;
    if (node) node.remove();
    node = document.createElement(op.component);
    node.id = 'panel-' + op.id;
    node.className = 'panel-root';
    // Do not force display:block — it overrides component :host{display:flex}
    // and breaks scrolling inside Shadow DOM (chat/rail).
    var props = null;
    if (op.props !== undefined && op.props !== null) {
      try { props = (typeof op.props === 'string') ? JSON.parse(op.props) : op.props; }
      catch (e) { props = {}; }
    }
    if (props !== null) {
      node.props = props;
      // If the entry module has not landed yet, the assignment above becomes
      // an own data property that would shadow the class accessor after the
      // custom element upgrades. Re-apply once defined so the setter runs.
      if (!customElements.get(op.component)) {
        customElements.whenDefined(op.component).then(function () {
          if (node.isConnected) { delete node.props; node.props = props; }
        });
      }
    }
    var host = panelHost(op.slot);
    if (host) host.appendChild(node);
  }

  // Import every mounted plugin's UI Entry and apply this page's static
  // mounts. Query params: plugin name (components derive it from
  // import.meta.url) and version (cache-busting across plugin upgrades).
  function loadPluginUIs() {
    fetch('/api/plugins').then(function (r) { return r.json(); }).then(function (b) {
      // Deterministic slot order (BUG-14): chips/mounts must land in a stable
      // plugin-name order — the bottom region has no ordering contract, so
      // import-completion order used to shuffle it per load. Imports still run
      // in parallel; only the mount phase is ordered.
      var plugins = (b.plugins || []).filter(function (p) {
        if (!p.ui || !p.ui.entry) return false;
        // /api/plugins reports the whole Discovery catalog (the plugin graph
        // draws from it), but only mounted plugins have live Panel Components:
        // an unmounted plugin's UI dir is not even served under /plugin-ui/.
        // Disabled (ADR-0032) is never mounted.
        if (p.disabled) return false;
        if (p.state && p.state !== 'mounted') return false;
        return true;
      }).sort(function (a, c) { return (a.name || '').localeCompare(c.name || ''); });

      function mountsOf(p) {
        return (p.ui.mounts || []).filter(function (m) {
          if (PAGES.indexOf(pageOf(m)) < 0) {
            report('plugin ' + p.name + ' mounts unknown page "' + pageOf(m) + '" (known: ' + PAGES.join(', ') + ')');
            return false;
          }
          return pageOf(m) === page;
        });
      }

      Promise.all(plugins.map(function (p) {
        var url = p.ui.entry + '?plugin=' + encodeURIComponent(p.name) + '&v=' + encodeURIComponent(p.version || '');
        return import(url).catch(function (e) {
          report('plugin ui failed: ' + p.name + ' ' + e);
          return null;
        });
      })).then(function () {
        // Ordered mount: each plugin's statics land in sorted-name order while
        // module compilation stayed concurrent.
        plugins.forEach(function (p) {
          mountsOf(p).forEach(function (m) {
            applyPanel({ op: 'set', slot: m.slot, id: m.component, component: m.component, props: m.props });
          });
        });
      });
    }).catch(function (e) {
      report('plugin list failed: ' + e);
    });
  }

  return { applyPanel: applyPanel, loadPluginUIs: loadPluginUIs };
}

// The main shell page's loader instance; events.js routes runtime PanelOps
// through it. setPageLoader wires the instance created by main.js.
var pageApplyPanel = function () { };

export function setPageLoader(loader) {
  pageApplyPanel = loader.applyPanel;
}

export function applyPanel(op) {
  pageApplyPanel(op);
}
