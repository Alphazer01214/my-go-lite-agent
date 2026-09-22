// Session fact projection shared by the chat face and the trace column.

// Consecutive type=reasoning facts (legacy fragmented flushes) become one Thinking row.
// Merged row keeps the first non-zero ts (thinking start) so UI can paint it.
export function mergeReasoningFacts(facts) {
  var out = [];
  for (var i = 0; i < facts.length; i++) {
    var f = facts[i];
    if (!f || f.type !== 'reasoning') { out.push(f); continue; }
    var parts = [];
    var meta = f.meta;
    var seq = f.seq;
    var sid = f.sessionId;
    var ts = f.ts;
    while (i < facts.length && facts[i] && facts[i].type === 'reasoning') {
      if (facts[i].content) parts.push(facts[i].content);
      if (facts[i].meta) meta = facts[i].meta;
      if (facts[i].seq !== undefined) seq = facts[i].seq;
      if (facts[i].sessionId !== undefined) sid = facts[i].sessionId;
      if (!ts && facts[i].ts) ts = facts[i].ts;
      i++;
    }
    i--;
    out.push({ type: 'reasoning', role: f.role || 'assistant', content: parts.join(''), meta: meta, seq: seq, sessionId: sid, ts: ts });
  }
  return out;
}
