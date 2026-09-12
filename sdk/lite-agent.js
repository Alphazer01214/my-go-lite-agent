/* lite-agent.js — Web Shell bridge (zero npm). Trusted injection model (ADR-0009). */
(function(global){
  var listeners = { presentation: [], status: [], stream: [], panel: [] };
  function on(topic, fn){
    if(!listeners[topic]) listeners[topic] = [];
    listeners[topic].push(fn);
    return function(){
      var a = listeners[topic];
      var i = a.indexOf(fn);
      if(i>=0) a.splice(i,1);
    };
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
  function bind(root){
    (root||document).querySelectorAll('[data-la-plugin]').forEach(function(el){
      if(el.__laBound) return;
      el.__laBound = true;
      var run = function(){
        var value = el.getAttribute('data-la-value');
        if(value && value.trim().charAt(0)==='{'){
          try { value = JSON.parse(value); } catch(e){}
        }
        emitUIAction(
          el.getAttribute('data-la-plugin'),
          el.getAttribute('data-la-panel'),
          el.getAttribute('data-la-event') || el.tagName.toLowerCase(),
          value,
          {}
        );
      };
      el.addEventListener(el.tagName === 'SELECT' || el.tagName === 'INPUT' ? 'change' : 'click', run);
    });
  }
  global.LiteAgent = { on:on, call:call, emitUIAction:emitUIAction, complete:complete, bind:bind };
})(window);
