/* lite-agent.js — Web Shell bridge for Panel Components (zero npm, ADR-0010).
 *
 * A plugin's UI Entry module (ui/main.js) is imported by the Shell from
 * /plugin-ui/<name>/main.js?plugin=<name>&v=<version> — derive your plugin
 * name from import.meta.url, not from location. Define custom elements named
 * "<plugin-name>-*"; they receive data via element properties (PanelOp props
 * or manifest mounts), render into their own Shadow DOM using --la-* design
 * tokens, and talk back through this global:
 *
 *   LiteAgent.call(cap, method, payload)          → Promise<any>   star-through Host call
 *   LiteAgent.emitUIAction(plugin, panel, event, value, props?) → Promise<any>
 *                                                                  ui.action → plugin handler
 *   LiteAgent.on(topic, fn)                       → unsubscribe    presentation|status|stream|panel
 *   LiteAgent.onSessionChange(fn)                 → unsubscribe    fires immediately with the current
 *                                                                  session id, then on every switch
 *   LiteAgent.sendMessage(text, sessionId?)       → Promise<any>   start a turn (POST /api/message)
 *   LiteAgent.runCommand(line)                    → Promise<any>   run a /command (POST /api/command)
 *   LiteAgent.complete(prefix)                    → string[]       slash-command completion
 */
(function(global){
  var listeners = {};
  function on(topic, fn){
    if(!listeners[topic]) listeners[topic] = [];
    listeners[topic].push(fn);
    return function(){
      var a = listeners[topic];
      var i = a.indexOf(fn);
      if(i>=0) a.splice(i,1);
    };
  }
  function emit(topic, data){
    (listeners[topic]||[]).slice().forEach(function(fn){
      try { fn(data); } catch(e){ /* one bad subscriber must not break the stream */ }
    });
  }
  async function call(cap, method, payload){
    var res = await fetch('/api/call', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body: JSON.stringify({cap:cap, method:method, payload: payload||{}})
    });
    return res.json();
  }
  async function emitUIAction(plugin, panel, event, value, props){
    var res = await fetch('/api/ui-action', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body: JSON.stringify({
        plugin: plugin,
        panel: panel,
        event: event,
        value: value===undefined?null:value,
        props: props||{}
      })
    });
    return res.json();
  }
  function complete(prefix){
    if(!prefix || prefix.charAt(0) !== '/') return [];
    var body = prefix.slice(1);
    if(body.indexOf(' ') >= 0) return [];
    var names = global.__liteCommands || ['help','lp','refresh'];
    return names.map(function(n){return '/'+n;}).filter(function(c){return c.indexOf(prefix)===0;});
  }
  async function sendMessage(text, sessionId){
    var res = await fetch('/api/message', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body: JSON.stringify({text: text, sessionId: sessionId||''})
    });
    return res.json();
  }
  async function runCommand(line){
    var res = await fetch('/api/command', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body: JSON.stringify({line: line})
    });
    return res.json();
  }

  // Session channel: the Shell keeps window.__liteSessionId current and
  // re-emits on every switch. Subscribing fires immediately so late-loaded
  // components never miss the initial state.
  function onSessionChange(fn){
    if(global.__liteSessionId !== undefined){
      try { fn(global.__liteSessionId); } catch(e){}
    }
    return on('__session', fn);
  }

  global.LiteAgent = { on:on, emit:emit, call:call, emitUIAction:emitUIAction,
                       complete:complete, onSessionChange:onSessionChange,
                       sendMessage:sendMessage, runCommand:runCommand };
})(window);
