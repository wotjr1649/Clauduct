import assert from 'node:assert/strict';
import { prepareNative } from './native-protocol.mjs';
import { MODELS } from './models.mjs';
import { interactiveLaunch } from './clauduct.mjs';

const compact = `CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.

- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.
- You already have all the context you need in the conversation above.
- Tool calls will be REJECTED and will waste your only turn — you will fail the task.
- Your entire response must be plain text: an <analysis> block followed by a <summary> block.

Your task is to create a detailed summary of the conversation so far, paying close attention to the user's explicit requests and your previous actions.
PUBLIC_FIXTURE

REMINDER: Do NOT call any tools. Respond with plain text only — an <analysis> block followed by a <summary> block. Tool calls will be rejected and you will fail the task.`;
const doc = (model, effort, content) => ({ model, output_config: { effort }, max_tokens: 10000, stream: true,
  messages: [{ role: 'user', content }] });
let checks = 0;
for (const verificationSelection of [{ model: 'gpt-5.6-luna', effort: 'max' }, { model: 'gpt-5.6-sol', effort: 'low' }]) {
  const gateway = { port: 12345, clientHeaders: () => ({ Authorization: 'Bearer PUBLIC_FIXTURE' }) };
  const launch = interactiveLaunch(gateway, {}, process.cwd(), verificationSelection, [], { verifyModelRoute: true });
  const settings = JSON.parse(launch.args[launch.args.indexOf('--settings') + 1]);
  for (const env of [launch.options.env, settings.env]) {
    assert.equal(env.ANTHROPIC_DEFAULT_HAIKU_MODEL, verificationSelection.model);
    assert.equal(env.CLAUDE_CODE_EFFORT_LEVEL, verificationSelection.effort); checks++;
  }
  const normal = interactiveLaunch(gateway, {}, process.cwd(), verificationSelection);
  assert.equal(normal.options.env.ANTHROPIC_DEFAULT_HAIKU_MODEL, MODELS.luna.model);
  assert.equal(normal.options.env.CLAUDE_CODE_EFFORT_LEVEL, undefined); checks++;
  for (const content of ['PUBLIC_TASK', compact]) {
    for (const subagent of [false, true]) {
      const route = subagent ? verificationSelection : undefined;
      const request = doc(verificationSelection.model, verificationSelection.effort, content);
      const prepared = prepareNative(request, { route, subagent, verificationSelection });
      assert.deepEqual(prepared.selected, verificationSelection);
      assert.equal(prepared.purpose, content === compact ? 'compact-template' : 'conversation');
      assert.equal(prepared.body.model, verificationSelection.model);
      assert.equal(prepared.body.reasoning.effort, verificationSelection.effort); checks++;
      assert.throws(() => prepareNative(request, { route: { ...verificationSelection, effort: 'medium' },
        subagent, verificationSelection }), error => error.code === 'VERIFICATION_ROUTE_MISMATCH'); checks++;
    }
  }
  for (const { model } of Object.values(MODELS).filter(item => item.model !== verificationSelection.model)) {
    assert.throws(() => prepareNative(doc(model, verificationSelection.effort, 'PUBLIC_TASK'), { verificationSelection }),
      error => error.code === 'VERIFICATION_ROUTE_MISMATCH'); checks++;
  }
}
// Default policy remains medium for compaction; the verification request must opt in.
assert.equal(prepareNative(doc('luna', 'max', compact)).selected.effort, 'medium'); checks++;
assert.throws(() => prepareNative({ ...doc('luna', 'max', compact), verificationSelection: MODELS.luna }),
  error => error.requestFailure === 'REQUEST_FIELDS'); checks++;
console.log(JSON.stringify({ suite: 'verification-route', checks, externalRequests: 0, actualCredentialReads: 0 }));
