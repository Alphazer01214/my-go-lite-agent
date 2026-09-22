// Unit check for mergeReasoningFacts ts projection. Run: node facts_test.mjs
import { mergeReasoningFacts } from './facts.js';

function assert(cond, msg) {
  if (!cond) {
    console.error('FAIL:', msg);
    process.exit(1);
  }
}

const merged = mergeReasoningFacts([
  { type: 'message', role: 'user', content: 'hi', seq: 1, ts: 100 },
  { type: 'reasoning', role: 'assistant', content: 'a', seq: 2, ts: 0 },
  { type: 'reasoning', role: 'assistant', content: 'b', seq: 3, ts: 200 },
  { type: 'reasoning', role: 'assistant', content: 'c', seq: 4, ts: 300 },
  { type: 'message', role: 'assistant', content: 'ok', seq: 5, ts: 400 },
]);

assert(merged.length === 3, 'want 3 rows after merge, got ' + merged.length);
assert(merged[1].type === 'reasoning', 'want middle reasoning row');
assert(merged[1].content === 'abc', 'want merged content abc, got ' + merged[1].content);
assert(merged[1].ts === 200, 'want first non-zero ts 200, got ' + merged[1].ts);
assert(merged[1].seq === 4, 'want last seq 4, got ' + merged[1].seq);

const single = mergeReasoningFacts([
  { type: 'reasoning', role: 'assistant', content: 'only', seq: 1, ts: 999 },
]);
assert(single[0].ts === 999, 'single reasoning keeps ts, got ' + single[0].ts);

const noTs = mergeReasoningFacts([
  { type: 'reasoning', role: 'assistant', content: 'x', seq: 1 },
  { type: 'reasoning', role: 'assistant', content: 'y', seq: 2 },
]);
assert(noTs[0].ts === undefined, 'missing ts stays undefined, got ' + noTs[0].ts);

console.log('facts.js mergeReasoningFacts ts ok');
