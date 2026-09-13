// Panel Components loader (ADR-0010/0011). One instance per layout page:
// it applies static mounts declared for that page and runtime PanelOps whose
// slot this page's layout provides. Ops may arrive before the entry module
// loads; the custom element upgrade then wires the already-inserted element
// without losing props.

// Layout page vocabulary (ADR-0011): the layout owns the page enum.
export const PAGES = ['main', 'trace'];

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
    node.style.display = 'block';
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
      (b.plugins || []).forEach(function (p) {
        if (!p.ui || !p.ui.entry) return;
        var url = p.ui.entry + '?plugin=' + encodeURIComponent(p.name) + '&v=' + encodeURIComponent(p.version || '');
        import(url).then(function () {
          (p.ui.mounts || []).forEach(function (m) {
            if (PAGES.indexOf(pageOf(m)) < 0) {
              report('plugin ' + p.name + ' mounts unknown page "' + pageOf(m) + '" (known: ' + PAGES.join(', ') + ')');
              return;
            }
            if (pageOf(m) !== page) return; // other layout page's mount
            applyPanel({ op: 'set', slot: m.slot, id: m.component, component: m.component, props: m.props });
          });
        }).catch(function (e) {
          report('plugin ui failed: ' + p.name + ' ' + e);
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
