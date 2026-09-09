// Claude 2.1.263 _0e/yPn wrap compact requests in these exact text boundaries.
// This is template compatibility, not authenticated origin or a tool/permission decision.
const prefix = 'CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.\n\n'
  + '- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.\n'
  + '- You already have all the context you need in the conversation above.\n'
  + '- Tool calls will be REJECTED and will waste your only turn — you will fail the task.\n'
  + '- Your entire response must be plain text: an <analysis> block followed by a <summary> block.\n\n';
const suffix = '\n\nREMINDER: Do NOT call any tools. Respond with plain text only — '
  + 'an <analysis> block followed by a <summary> block. Tool calls will be rejected and you will fail the task.';
const foldWhitespace = value => value.replace(/\s+/g, ' ').trim();
const foldedPrefix = foldWhitespace(prefix), foldedSuffix = foldWhitespace(suffix);

export function inspectCompactTemplate(messages) {
  // Never inspect historical summaries, assistant text, or tool outputs for routing.
  const last = messages.findLast(message => message.role !== 'system');
  const shape = { lastRole: ['user', 'assistant'].includes(last?.role) ? last.role : 'other',
    textBlocks: 0, mixedBlocks: false, prefixMatches: false, suffixMatches: false, matches: false };
  if (last?.role !== 'user') return shape;
  const blocks = typeof last.content === 'string' ? [{ type: 'text', text: last.content }] : last.content;
  if (!Array.isArray(blocks)) return shape;
  shape.mixedBlocks = blocks.some(block => block.type !== 'text');
  // Native _ys appends a newline when merging adjacent user messages. ghr can
  // retain earlier tool_result blocks in that same message; never inspect those.
  const texts = blocks.filter(block => block.type === 'text' && typeof block.text === 'string')
    .map(block => block.text.trimEnd())
    .filter(value => !(value.startsWith('<system-reminder>') && value.endsWith('</system-reminder>')));
  shape.textBlocks = texts.length;
  const final = texts.at(-1);
  // Compare wording, not terminal paste indentation or native message line wrapping.
  // The original prompt sent upstream is never rewritten.
  const folded = typeof final === 'string' ? foldWhitespace(final) : '';
  shape.prefixMatches = folded.startsWith(foldedPrefix + ' ');
  shape.suffixMatches = folded.endsWith(' ' + foldedSuffix);
  shape.matches = shape.prefixMatches && shape.suffixMatches
    && folded.length > foldedPrefix.length + foldedSuffix.length + 2;
  return shape;
}
