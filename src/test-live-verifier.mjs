import assert from 'node:assert/strict';
import { verifyLive } from '../verification/verify-live.mjs';

let requests = 0, closed = 0;
function transport(text = 'CLAUDUCT_PUBLIC_CHECK_OK', fail = false) {
  return { diagnostics: () => ({ activeSockets: 0, activeRequests: 0, requestAttempts: requests }),
    close: async () => { closed++; }, send: async (body, _signal, callbacks) => {
      requests++;
      assert.equal(body.model, 'gpt-5.6-luna');
      assert.equal(body.reasoning.effort, 'low');
      assert.equal(body.tool_choice, 'none'); assert.deepEqual(body.tools, []);
      assert.equal(await callbacks.canRetry(), false);
      assert.ok(JSON.stringify(body).includes('Reply with exactly CLAUDUCT_PUBLIC_CHECK_OK.'));
      if (fail) throw new Error('SYNTHETIC_PRIVATE_ERROR');
      const item = { type: 'message', id: 'msg_public', role: 'assistant', status: 'completed',
        content: [{ type: 'output_text', text, annotations: [] }] };
      return [
        { type: 'response.created', response: { id: 'resp_public', status: 'in_progress' } },
        { type: 'response.output_item.added', output_index: 0, item: { ...item, status: 'in_progress', content: [] } },
        { type: 'response.output_text.delta', output_index: 0, item_id: item.id, content_index: 0, delta: text },
        { type: 'response.output_text.done', output_index: 0, item_id: item.id, content_index: 0, text },
        { type: 'response.output_item.done', output_index: 0, item },
        { type: 'response.completed', response: { id: 'resp_public', status: 'completed', model: body.model,
          output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }
      ];
    } };
}
const good = await verifyLive(transport());
assert.equal(good.passed, true); assert.equal(good.resourcesClosed, true);
const wrong = await verifyLive(transport('SYNTHETIC_PRIVATE_RESPONSE'));
assert.equal(wrong.passed, false); assert.equal(wrong.exactReply, false);
assert.ok(!JSON.stringify(wrong).includes('SYNTHETIC_PRIVATE_RESPONSE'));
const failed = await verifyLive(transport('', true));
assert.equal(failed.passed, false); assert.equal(failed.failed, 1); assert.equal(failed.resourcesClosed, true);
assert.ok(!JSON.stringify(failed).includes('SYNTHETIC_PRIVATE_ERROR'));
assert.equal(requests, 3); assert.equal(closed, 3);
console.log(JSON.stringify({ suite: 'live-verifier', checks: 3, actualClaudeExecutions: 0,
  actualCredentialReads: 0, externalRequests: 0 }));
