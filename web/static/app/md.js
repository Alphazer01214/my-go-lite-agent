// Markdown rendering for the chat face (extracted verbatim from shell.html).

export var BT = String.fromCharCode(96);
export var FENCE = BT + BT + BT;

export function esc(s) {
  return String(s).replace(/[&<>]/g, function (c) {
    return { '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c];
  });
}

export function renderMath(root) {
  if (!window.katex || !window.renderMathInElement) return;
  try {
    window.renderMathInElement(root, {
      delimiters: [
        { left: '$$', right: '$$', display: true },
        { left: '\\[', right: '\\]', display: true },
        { left: '$', right: '$', display: false },
        { left: '\\(', right: '\\)', display: false }
      ],
      throwOnError: false
    });
  } catch (e) { }
}

export function md(src) {
  if (!src) return '';
  var s = String(src).replace(/\r\n/g, '\n');
  var fenceRe = new RegExp(FENCE + '([\\\\w-]*)\\\\n([\\\\s\\\\S]*?)' + FENCE, 'g');
  s = s.replace(fenceRe, function (_, lang, code) { return '<pre><code>' + esc(code) + '</code></pre>'; });
  s = s.replace(/^### (.*)$/gm, '<h3>$1</h3>');
  s = s.replace(/^## (.*)$/gm, '<h2>$1</h2>');
  s = s.replace(/^# (.*)$/gm, '<h1>$1</h1>');
  s = s.replace(/^---$/gm, '<hr/>');
  s = s.replace(/^> (.*)$/gm, '<blockquote>$1</blockquote>');
  s = s.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  var codeRe = new RegExp(BT + '([^' + BT + ']+)' + BT, 'g');
  s = s.replace(codeRe, '<code>$1</code>');
  s = s.replace(/\*([^*]+)\*/g, '<em>$1</em>');
  s = s.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  s = s.replace(/^(?:- |\* )(.*)(?:\n(?:- |\* ).*)*/gm, function (block) {
    var items = block.split(/\n/).map(function (l) { return l.replace(/^(?:- |\* )/, ''); }).map(function (t) { return '<li>' + t + '</li>'; }).join('');
    return '<ul>' + items + '</ul>';
  });
  return s.split(/\n{2,}/).map(function (p) {
    p = p.trim();
    if (!p) return '';
    if (/^<(h\d|ul|pre|blockquote|hr)/.test(p)) return p;
    return '<p>' + p.replace(/\n/g, '<br/>') + '</p>';
  }).join('\n');
}
