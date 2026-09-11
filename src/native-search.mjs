// Claude Code's WebSearch is a server tool: the client issues an isolated side query carrying
// the search tool and reads web_search_tool_result blocks out of the reply. This gateway is that
// server, so it answers the side query itself from the backend's standalone search endpoint —
// the same endpoint, and the same credential, the reference client uses for its own web.run
// tool. No model turn is involved, so the answer costs no tokens and adds no upstream latency.
import { randomUUID } from 'node:crypto';
import { NativeError, need } from './native-protocol.mjs';

// The exact text the client puts in front of the query. Matching it is what distinguishes a
// side query from a conversation that merely mentions searching.
const PROMPT = 'Perform a web search for the query: ';
export const SEARCH_LIMITS = Object.freeze({ query: 2048, results: 20, title: 512, url: 2048, output: 100000 });

const text = value => typeof value === 'string' ? value
  : Array.isArray(value) && value.length === 1 && value[0]?.type === 'text' ? value[0].text : null;

// Returns the query when this request is the client's search side query, otherwise null. Every
// condition is one the client itself sets, so a near miss falls through to the ordinary path
// rather than being answered locally with something the caller did not ask for.
export function searchSideQuery(doc) {
  if (!Array.isArray(doc?.tools) || doc.tools.length !== 1) return null;
  const [tool] = doc.tools;
  if (tool?.type !== 'web_search_20250305' || tool.name !== 'web_search') return null;
  // Live requests carrying this tool reached the upstream stage, so the client does not always
  // name the tool in tool_choice. Accept the shapes it can send and refuse anything that points
  // at a different tool, which would be a conversation rather than the side query.
  const choice = doc.tool_choice;
  if (!(choice === undefined || choice?.type === 'auto'
    || (choice?.type === 'tool' && choice.name === 'web_search'))) return null;
  if (!Array.isArray(doc.messages) || doc.messages.length !== 1 || doc.messages[0]?.role !== 'user') return null;
  const body = text(doc.messages[0].content);
  if (typeof body !== 'string' || !body.startsWith(PROMPT)) return null;
  const query = body.slice(PROMPT.length).trim();
  if (!query || query.length > SEARCH_LIMITS.query) return null;
  return { query, allowed: domains(tool.allowed_domains), blocked: domains(tool.blocked_domains) };
}

const domains = value => Array.isArray(value) && value.length
  && value.every(item => typeof item === 'string' && item.length > 0 && item.length <= 253) ? value.slice(0, 32) : null;

// The reference client's own request shape. Only the query travels: the conversation tail it
// may send is deliberately left out, so nothing from this machine goes with the search.
export function searchRequestBody(id, model, { query, allowed, blocked }) {
  const filters = { ...(allowed && { allowed_domains: allowed }), ...(blocked && { blocked_domains: blocked }) };
  return { id, model,
    input: [{ type: 'message', role: 'user', content: [{ type: 'input_text', text: query }] }],
    commands: { search_query: [{ q: query }] },
    settings: { external_web_access: true, search_context_size: 'medium', allowed_callers: ['direct'],
      ...(Object.keys(filters).length && { filters }) },
    max_output_tokens: 2500 };
}

// Upstream search output is web content: attacker-influenced by definition. Bound and shape-check
// every field rather than trusting it, and never let a malformed result silently become no result.
const clean = (value, limit) => typeof value === 'string' && value.length > 0 && value.length <= limit
  && !/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/.test(value) ? value : null;
function link(item) {
  if (!item || typeof item !== 'object' || Array.isArray(item)) return null;
  const url = clean(item.url, SEARCH_LIMITS.url), title = clean(item.title, SEARCH_LIMITS.title);
  if (!url || !title) return null;
  let parsed;
  try { parsed = new URL(url); } catch { return null; }
  return parsed.protocol === 'https:' || parsed.protocol === 'http:' ? { type: 'web_search_result', title, url } : null;
}

export function searchResults(response) {
  need(response && typeof response === 'object' && !Array.isArray(response), 'SEARCH_RESPONSE_SHAPE');
  need(typeof response.output === 'string' && response.output.length <= SEARCH_LIMITS.output, 'SEARCH_RESPONSE_SHAPE');
  const absent = response.results === undefined || response.results === null;
  need(absent || Array.isArray(response.results), 'SEARCH_RESPONSE_SHAPE');
  const items = absent ? [] : response.results;
  const links = items.slice(0, SEARCH_LIMITS.results).map(link).filter(Boolean);
  // A reply with neither links nor text is a failed search, not an empty one. Say so.
  need(links.length > 0 || response.output.trim().length > 0, 'SEARCH_RESULTS_EMPTY');
  return { links, output: response.output };
}

// The client reads title and url out of the result block and takes everything else from the text
// blocks around it, so the endpoint's own digest goes into a text block after the result.
export function searchReply(model, request, response, id = randomUUID) {
  const { links, output } = searchResults(response);
  const useId = `srvtoolu_${id().replace(/-/g, '')}`;
  const content = [{ type: 'server_tool_use', id: useId, name: 'web_search', input: { query: request.query } },
    { type: 'web_search_tool_result', tool_use_id: useId, content: links }];
  if (output.trim()) content.push({ type: 'text', text: output });
  const usage = { input_tokens: 0, output_tokens: 0 };
  const message = { id: `msg_${id().replace(/-/g, '')}`, type: 'message', role: 'assistant', model,
    content, stop_reason: 'end_turn', stop_sequence: null, usage };
  const frames = [{ type: 'message_start',
    message: { ...message, content: [], stop_reason: null, usage: { ...usage, output_tokens: 0 } } }];
  content.forEach((block, index) => {
    frames.push({ type: 'content_block_start', index,
      content_block: block.type === 'text' ? { type: 'text', text: '' }
        : block.type === 'server_tool_use' ? { ...block, input: {} } : block });
    if (block.type === 'text') {
      frames.push({ type: 'content_block_delta', index, delta: { type: 'text_delta', text: block.text } });
    }
    if (block.type === 'server_tool_use') {
      frames.push({ type: 'content_block_delta', index,
        delta: { type: 'input_json_delta', partial_json: JSON.stringify(block.input) } });
    }
    frames.push({ type: 'content_block_stop', index });
  });
  frames.push({ type: 'message_delta', delta: { stop_reason: 'end_turn', stop_sequence: null }, usage },
    { type: 'message_stop' });
  return { message, frames, searchCalls: 1, links: links.length };
}
