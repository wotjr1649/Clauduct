import assert from 'node:assert/strict';
import { prepareNative, prepareFileReview, prepareReviewContext, verifyFileReviewStep, fileReviewCommand } from './native-protocol.mjs';

const target = 'D:/SYNTHETIC_PROJECT/example.mjs';
const doc = () => ({ model: 'luna', stream: true, max_tokens: 1000,
  messages: [{ role: 'user', content: `Review target: \`${target}\`\nReview the file.` }],
  tools: ['Read', 'Bash'].map(name => ({ name, input_schema: { type: 'object', properties: {} } })) });
const diffCall = { type: 'tool_use', id: 'diff_target', name: 'Bash', input: { command: fileReviewCommand(target) } };
for (const level of ['low', 'medium', 'high', 'xhigh', 'max']) {
  const first = doc(); first.messages[0].content += `\n${level} effort`;
  const prepared = prepareNative(first);
  prepareFileReview(prepared, first);
  assert.deepEqual(prepared.body.tool_choice, { type: 'function', name: 'Bash' });
  assert.equal(prepared.body.parallel_tool_calls, false);
  const bash = prepared.body.tools.find(tool => tool.name === 'Bash');
  assert.equal(bash.strict, true);
  assert.deepEqual(bash.parameters, { type: 'object', properties: {
    command: { type: 'string', enum: [diffCall.input.command] }
  }, required: ['command'], additionalProperties: false });
  assert.equal(first.tools.find(tool => tool.name === 'Bash').input_schema.properties.command, undefined);
  assert.throws(() => verifyFileReviewStep({ content: [{ type: 'text', text: '(none)' }] }, prepared), /REVIEW_DIFF_REQUIRED/);
  assert.throws(() => verifyFileReviewStep({ content: [{ ...diffCall, name: 'Read' }] }, prepared), /REVIEW_DIFF_REQUIRED/);
  assert.throws(() => verifyFileReviewStep({ content: [{ ...diffCall, input: { command: fileReviewCommand('D:/OTHER/file.mjs') } }] }, prepared));
  assert.throws(() => verifyFileReviewStep({ content: [{ ...diffCall, input: { command: diffCall.input.command + '; echo extra' } }] }, prepared));
  verifyFileReviewStep({ content: [diffCall] }, prepared);
  for (const [calls, reason] of [[[], 'call-count'], [[diffCall, diffCall], 'call-count'],
    [[{ ...diffCall, name: 'Read' }], 'tool-name'],
    [[{ ...diffCall, input: { command: 'SYNTHETIC_WRONG_COMMAND' } }], 'command'],
    [[{ ...diffCall, input: { ...diffCall.input, run_in_background: true } }], 'background']]) {
    assert.throws(() => verifyFileReviewStep({ content: calls }, prepared), error =>
      error.code === 'REVIEW_DIFF_REQUIRED' && error.reviewDiffMismatch === reason);
  }

  const following = doc(); following.messages[0].content += `\n${level} effort`;
  following.messages.push({ role: 'assistant', content: [diffCall] },
    { role: 'user', content: [{ type: 'tool_result', tool_use_id: diffCall.id, content: JSON.stringify({ diagnosticVersion: 1, target, kind: 'untracked-added', diff: '@@ -0,0 +1,1 @@\n+SYNTHETIC\n' }) }] });
  const next = prepareNative(following); prepareFileReview(next, following);
  assert.equal(next.requiredReviewDiff, undefined); assert.equal(next.body.tool_choice, 'auto');
  assert.equal(next.body.tools.find(tool => tool.name === 'Bash').strict, false);
  verifyFileReviewStep({ content: [{ type: 'text', text: '[]' }] }, next);
  following.messages.at(-1).content[0].is_error = true;
  assert.throws(() => prepareFileReview(prepareNative(following), following), /REVIEW_DIFF_FAILED/);
  for (const blocked of ['none', 'removed']) {
    const request = doc();
    if (blocked === 'none') request.tool_choice = { type: 'none' };
    else request.tools = request.tools.filter(tool => tool.name !== 'Bash');
    assert.throws(() => prepareFileReview(prepareNative(request), request), /REVIEW_DIFF_UNAVAILABLE/);
  }
}
const branch = doc(); branch.messages[0].content = 'Review target: `main`';
const branchPrepared = prepareNative(branch); prepareFileReview(branchPrepared, branch);
assert.equal(branchPrepared.body.tool_choice, 'auto');
const ordinary = prepareNative(doc());
const unchanged = JSON.stringify(ordinary);
prepareReviewContext(ordinary, { reviewContext: false });
assert.equal(JSON.stringify(ordinary), unchanged);
const member = prepareNative(doc());
prepareReviewContext(member, { reviewContext: true });
assert.match(member.body.input.at(-1).content, /already executing native code-review/);
assert.equal(member.requiredReviewDiff, undefined);
assert.deepEqual(member.body.tools, ordinary.body.tools);
assert.equal(member.body.tool_choice, ordinary.body.tool_choice);
const compact = prepareNative(doc()); compact.purpose = 'compact-template';
const beforeCompact = JSON.stringify(compact);
prepareReviewContext(compact, { reviewContext: true });
assert.equal(JSON.stringify(compact), beforeCompact);
verifyFileReviewStep({ content: [{ type: 'text', text: 'ordinary reply' }] }, ordinary);
assert.equal(ordinary.body.tool_choice, 'auto');
console.log(JSON.stringify({ suite: 'file-review', passed: true, actualClaude: 0, externalRequests: 0 }));
