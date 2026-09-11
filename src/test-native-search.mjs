// Offline only: pure translation checks. Never opens a socket or reads a credential.
import assert from 'node:assert/strict';
import { searchSideQuery, searchRequestBody, searchResults, searchReply, SEARCH_LIMITS } from './native-search.mjs';

let count = 0;
const check = (name, action) => { action(); count++; };
const tool = extra => ({ type: 'web_search_20250305', name: 'web_search', ...extra });
const side = (query = 'SYNTHETIC_QUERY', extra = {}, overrides = {}) => ({
  model: 'claude-sonnet-5', max_tokens: 1024, tools: [tool(extra)],
  tool_choice: { type: 'tool', name: 'web_search' },
  messages: [{ role: 'user', content: `Perform a web search for the query: ${query}` }], ...overrides });

check('the client side query is recognised', () => {
  assert.deepEqual(searchSideQuery(side()), { query: 'SYNTHETIC_QUERY', allowed: null, blocked: null });
  // Content may arrive as a single text block instead of a bare string.
  assert.equal(searchSideQuery(side('SYNTHETIC_QUERY', {}, {
    messages: [{ role: 'user', content: [{ type: 'text', text: 'Perform a web search for the query: SYNTHETIC_QUERY' }] }]
  })).query, 'SYNTHETIC_QUERY');
  assert.deepEqual(searchSideQuery(side('q', { allowed_domains: ['example.com'] })).allowed, ['example.com']);
});
check('anything the client would not send falls through to the model', () => {
  const cases = [
    { tools: [] },
    { tools: [tool(), tool()] },
    { tools: [{ type: 'web_search_20250305', name: 'other' }] },
    { tool_choice: 'auto' },
    { tool_choice: { type: 'tool', name: 'Bash' } },
    { messages: [{ role: 'user', content: 'search the web for cats' }] },
    { messages: [{ role: 'assistant', content: 'Perform a web search for the query: x' }] },
    { messages: [{ role: 'user', content: 'Perform a web search for the query: x' },
      { role: 'assistant', content: 'y' }] },
    { messages: [{ role: 'user', content: 'Perform a web search for the query:    ' }] },
    { messages: [{ role: 'user', content: `Perform a web search for the query: ${'x'.repeat(SEARCH_LIMITS.query + 1)}` }] }
  ];
  for (const overrides of cases) {
    assert.equal(searchSideQuery(side('q', {}, overrides)), null, JSON.stringify(overrides).slice(0, 60));
  }
  assert.equal(searchSideQuery(undefined), null);
  assert.equal(searchSideQuery({}), null);
});
check('only the query travels upstream', () => {
  const body = searchRequestBody('session-1', 'gpt-5.6-luna', { query: 'SYNTHETIC_QUERY', allowed: null, blocked: null });
  assert.deepEqual(body.commands, { search_query: [{ q: 'SYNTHETIC_QUERY' }] });
  assert.equal(body.input.length, 1);
  assert.equal(JSON.stringify(body).includes('Perform a web search'), false);
  assert.equal(body.settings.filters, undefined);
  const filtered = searchRequestBody('s', 'm', { query: 'q', allowed: ['a.com'], blocked: ['b.com'] });
  assert.deepEqual(filtered.settings.filters, { allowed_domains: ['a.com'], blocked_domains: ['b.com'] });
});
const upstream = (extra = {}) => ({ encrypted_output: 'opaque', output: 'SYNTHETIC_DIGEST',
  results: [{ type: 'text_result', ref_id: 'turn0search0', url: 'https://example.com/a', title: 'A',
    snippet: 'SYNTHETIC_SNIPPET', domain: 'example.com' }], ...extra });

check('a result keeps only what the client reads', () => {
  const { links, output } = searchResults(upstream());
  assert.deepEqual(links, [{ type: 'web_search_result', title: 'A', url: 'https://example.com/a' }]);
  assert.equal(output, 'SYNTHETIC_DIGEST');
});
check('web content is bounded and shape checked, never trusted', () => {
  const bad = [{ url: 'https://example.com/a' }, { title: 'A' }, { url: 'javascript:alert(1)', title: 'A' },
    { url: 'not a url', title: 'A' }, { url: 'https://example.com/a', title: `A${String.fromCharCode(0)}B` },
    { url: 'https://example.com/a', title: 'x'.repeat(SEARCH_LIMITS.title + 1) },
    { url: `https://e.com/${'x'.repeat(SEARCH_LIMITS.url)}`, title: 'A' }, null, 'string', []];
  assert.deepEqual(searchResults(upstream({ results: bad })).links, []);
  // More results than the cap are dropped rather than passed through.
  const many = Array.from({ length: SEARCH_LIMITS.results + 5 },
    (value, index) => ({ url: `https://example.com/${index}`, title: `T${index}` }));
  assert.equal(searchResults(upstream({ results: many })).links.length, SEARCH_LIMITS.results);
});
check('a malformed or empty search fails with its own name', () => {
  for (const doc of [null, [], 'text', { output: 1 }, { output: 'x', results: 'no' },
    { results: [] }, { output: 'x'.repeat(SEARCH_LIMITS.output + 1) }]) {
    assert.throws(() => searchResults(doc), error => error.code === 'SEARCH_RESPONSE_SHAPE', JSON.stringify(doc));
  }
  assert.throws(() => searchResults({ output: '   ', results: [] }), error => error.code === 'SEARCH_RESULTS_EMPTY');
  // Links with no digest still count as a search that worked.
  assert.equal(searchResults({ output: '', results: upstream().results }).links.length, 1);
});
check('the reply is the shape the client reduces', () => {
  const request = { query: 'SYNTHETIC_QUERY', allowed: null, blocked: null };
  const { message, frames, links } = searchReply('claude-sonnet-5', request, upstream());
  assert.equal(links, 1);
  assert.deepEqual(message.content.map(block => block.type),
    ['server_tool_use', 'web_search_tool_result', 'text']);
  const [use, result, digest] = message.content;
  assert.equal(use.name, 'web_search');
  assert.deepEqual(use.input, { query: 'SYNTHETIC_QUERY' });
  assert.equal(result.tool_use_id, use.id);
  assert.deepEqual(result.content, [{ type: 'web_search_result', title: 'A', url: 'https://example.com/a' }]);
  assert.equal(digest.text, 'SYNTHETIC_DIGEST');
  assert.equal(message.stop_reason, 'end_turn');
  // The frame order the client's stream reader expects.
  assert.deepEqual(frames.map(frame => frame.type), ['message_start',
    'content_block_start', 'content_block_delta', 'content_block_stop',
    'content_block_start', 'content_block_stop',
    'content_block_start', 'content_block_delta', 'content_block_stop',
    'message_delta', 'message_stop']);
  assert.equal(frames[1].content_block.type, 'server_tool_use');
  assert.deepEqual(frames[1].content_block.input, {});
  assert.equal(JSON.parse(frames[2].delta.partial_json).query, 'SYNTHETIC_QUERY');
  assert.equal(frames[4].content_block.type, 'web_search_tool_result');
  assert.equal(frames.at(-2).delta.stop_reason, 'end_turn');
});
check('a digest-free search still returns its links', () => {
  const { message } = searchReply('m', { query: 'q' }, { output: '', results: upstream().results });
  assert.deepEqual(message.content.map(block => block.type), ['server_tool_use', 'web_search_tool_result']);
});
console.log(JSON.stringify({ suite: 'native-search', passed: count, externalRequests: 0, credentialReads: 0 }));
