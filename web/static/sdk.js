/* lite-agent.js — Web Shell L0 bridge for Panel Components (zero npm, ADR-0010/0030).
 *
 * Shell keeps only: point-named call, UI action, event bus, slash complete.
 * Domain APIs (sendMessage / session faces) live in plugin ui/ modules.
 *
 *   LiteAgent.call(to, method, payload)     → Promise<{ok,result}>  POST /api/call {to,method,payload}
 *   LiteAgent.callCap(to, cap, method, payload) → same, with explicit cap for plugin dispatch
 *   LiteAgent.emitUIAction(plugin, panel, event, value, props?) → Promise
 *   LiteAgent.on(topic, fn)                 → unsubscribe  presentation|status|stream|panel|evt
 *   LiteAgent.emit(topic, data)             → local bus
 *   LiteAgent.complete(prefix)              → string[] slash suggestions
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
  async function callCap(to, cap, method, payload){
    var res = await fetch('/api/call', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body: JSON.stringify({to:to, cap:cap, method:method, payload: payload||{}})
    });
    return res.json();
  }
  async function call(to, method, payload){
    return callCap(to, to, method, payload);
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
  async function runCommand(line){
    var res = await fetch('/api/command', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body: JSON.stringify({line: line})
    });
    return res.json();
  }

  global.LiteAgent = { on:on, emit:emit, call:call, callCap:callCap,
                       emitUIAction:emitUIAction, complete:complete,
                       runCommand:runCommand };
})(window);
