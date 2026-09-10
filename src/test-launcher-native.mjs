import assert from 'node:assert/strict';
import { interactiveLaunch, launchOptions } from './clauduct.mjs';
import { MODELS, ROLE_MODELS, CONTEXT_POLICY } from './models.mjs';
import { prepareNative } from './native-protocol.mjs';
import { createNativeCredentialSupplier } from '../poc/user-session.mjs';

const now = 1_800_000_000_000;
const token = (account, marker) => `synthetic.${Buffer.from(JSON.stringify({ exp: now / 1000 + 3600,
  account, marker })).toString('base64url')}.signature`;
const cache = (account, marker) => JSON.stringify({ auth_mode: 'chatgpt', tokens: {
  access_token: token(account, marker), account_id: account } });

function optionTests() {
  assert.deepEqual(launchOptions([]).selected, { model: 'gpt-6-astra', effort: 'low' });
  assert.deepEqual(launchOptions(['--gpt-agents', '--document-first']).selected, { model: 'gpt-6-astra', effort: 'low' });
  assert.deepEqual(launchOptions(['--effort', 'max']).selected, { model: 'gpt-6-astra', effort: 'max' });
  assert.deepEqual(launchOptions(['--model', 'astra']).selected, MODELS.astra);
  assert.deepEqual(launchOptions(['--model', 'sol']).selected, { model: 'gpt-5.6-sol', effort: 'xhigh' });
  const request = { model: 'sol', stream: true, max_tokens: 100, messages: [{ role: 'user', content: 'SYNTHETIC' }] };
  assert.equal(prepareNative(request, { subagent: true }).body.reasoning.effort, 'xhigh');
  assert.equal(prepareNative({ ...request, model: 'luna' }, { subagent: true, route: ROLE_MODELS.Plan }).body.reasoning.effort, 'xhigh');
  const launch = interactiveLaunch({ port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC' }) },
    {}, 'D:/SYNTHETIC_PROJECT', MODELS.sol);
  assert.equal(launch.args[launch.args.indexOf('--effort') + 1], 'xhigh');
  assert.equal(launch.options.env.ANTHROPIC_DEFAULT_OPUS_MODEL, 'gpt-5.6-sol');
  assert.equal(launch.options.env.ANTHROPIC_DEFAULT_HAIKU_MODEL, 'gpt-5.6-luna');
  assert.equal(launch.options.env.ANTHROPIC_DEFAULT_SONNET_MODEL, 'gpt-5.6-luna');
  const repeated = launchOptions(['--resume', 'SYNTHETIC_ONE', '--resume', 'SYNTHETIC_TWO',
    '--add-dir', 'SYNTHETIC_DIR', '--add-dir', 'SYNTHETIC_OTHER']);
  assert.deepEqual(repeated.forward, ['--resume', 'SYNTHETIC_ONE', '--resume', 'SYNTHETIC_TWO',
    '--add-dir', 'SYNTHETIC_DIR', '--add-dir', 'SYNTHETIC_OTHER']);

  const equals = launchOptions(['--model=sol', '--effort=high', '--', '--model', 'luna', '--settings', 'SYNTHETIC']);
  assert.deepEqual(equals.selected, { model: 'gpt-5.6-sol', effort: 'high' });
  assert.deepEqual(equals.forward, ['--', '--model', 'luna', '--settings', 'SYNTHETIC']);

  const consumed = launchOptions(['--output-format', '--model', '--effort', 'low']);
  assert.deepEqual(consumed.selected, { model: 'gpt-6-astra', effort: 'low' });
  assert.deepEqual(consumed.forward, ['--output-format', '--model']);

  for (const args of [['--model'], ['--model='], ['--effort'], ['--effort='],
    ['--model', 'astra', '--model', 'sol'], ['--effort', 'low', '--effort', 'high'],
    ['--help', '--dry-run'], ['--settings', '{}'], ['--settings={}']]) {
    assert.throws(() => launchOptions(args));
  }
}

function childRetryTest() {
  const source = { CLAUDE_CODE_MAX_RETRIES: '10', CLAUDE_CODE_RETRY_WATCHDOG: '1',
    CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK: '0',
    CLAUDE_CODE_RESUME_INTERRUPTED_TURN: '1' };
  const gateway = { port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC' }) };
  const launch = interactiveLaunch(gateway, source, 'D:/SYNTHETIC_PROJECT');
  const settings = JSON.parse(launch.args[launch.args.indexOf('--settings') + 1]);
  assert.match(settings.env.ANTHROPIC_CUSTOM_MODEL_OPTION_NAME, /\[startup configuration\]$/);
  assert.ok(settings.modelPicker.options.every(option => !option.label.includes('startup configuration')));
  assert.equal(settings.hooks.PostToolUse[0].matcher, 'Skill|SendMessage|Workflow');
  assert.equal(settings.hooks.PostToolUse[0].hooks[0].command, settings.hooks.SubagentStart[0].hooks[0].command);
  assert.equal(source.CLAUDE_CODE_MAX_RETRIES, '10');
  assert.equal(source.CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK, '0');
  assert.equal(launch.options.env.CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK, '1');
  assert.equal(settings.env.CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK, '1');
  assert.equal(source.CLAUDE_CODE_RETRY_WATCHDOG, '1');
  assert.equal(source.CLAUDE_CODE_RESUME_INTERRUPTED_TURN, '1');
  assert.equal(launch.options.env.CLAUDE_CODE_MAX_RETRIES, '0');
  assert.equal(launch.options.env.CLAUDE_CODE_RETRY_WATCHDOG, '0');
  assert.equal(launch.options.env.CLAUDE_CODE_RESUME_INTERRUPTED_TURN, '0');
  assert.equal(settings.env.CLAUDE_CODE_MAX_RETRIES, '0');
  assert.equal(settings.env.CLAUDE_CODE_RETRY_WATCHDOG, '0');
  assert.equal(settings.env.CLAUDE_CODE_RESUME_INTERRUPTED_TURN, '0');
}

function autoCompactVerificationTest() {
  const source = { CLAUDE_CODE_AUTO_COMPACT_WINDOW: '500000' }, before = JSON.stringify(source);
  const gateway = { port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC' }) };
  const options = launchOptions(['--verify-auto-compact', '--model', 'luna']);
  const launch = interactiveLaunch(gateway, source, 'D:/SYNTHETIC_PROJECT', options.selected, options.forward, options);
  const settings = JSON.parse(launch.args[launch.args.indexOf('--settings') + 1]);
  assert.equal(launch.options.env.CLAUDE_CODE_MAX_CONTEXT_TOKENS, '400000');
  assert.equal(settings.env.CLAUDE_CODE_AUTO_COMPACT_WINDOW, '100000');
  assert.equal(launch.options.env.CLAUDE_CODE_AUTO_COMPACT_WINDOW, '100000');
  assert.equal(Math.floor(80000 * Number(settings.env.CLAUDE_AUTOCOMPACT_PCT_OVERRIDE) / 100), 67368);
  const normal = interactiveLaunch(gateway, source, 'D:/SYNTHETIC_PROJECT');
  assert.deepEqual(normal.args.slice(0, 4), ['--model', 'gpt-6-astra', '--effort', 'low']);
  assert.equal(normal.options.env.CLAUDE_CODE_AUTO_COMPACT_WINDOW, '400000');
  assert.equal(CONTEXT_POLICY.compactAt, 320000);
  for (const selected of [...Object.values(MODELS), ...Object.values(ROLE_MODELS)]) {
    const launch = interactiveLaunch(gateway, source, 'D:/SYNTHETIC_PROJECT', selected);
    const settings = JSON.parse(launch.args[launch.args.indexOf('--settings') + 1]);
    for (const env of [settings.env, launch.options.env]) {
      assert.equal(env.CLAUDE_CODE_MAX_CONTEXT_TOKENS, '400000');
      assert.equal(env.CLAUDE_CODE_AUTO_COMPACT_WINDOW, '400000');
      const effective = Number(env.CLAUDE_CODE_AUTO_COMPACT_WINDOW) - 20000;
      assert.equal(Math.min(Math.floor(effective * (Number(env.CLAUDE_AUTOCOMPACT_PCT_OVERRIDE) / 100)), effective - 13000), 320000);
    }
  }
  assert.equal(JSON.stringify(source), before);
  assert.throws(() => launchOptions(['--verify-auto-compact', '--verify-auto-compact']));
  assert.throws(() => launchOptions(['--verify-auto-compact=1']));
}

function agentModelVerificationTest() {
  const source = { CLAUDE_CONFIG_DIR: 'SYNTHETIC_NATIVE_CONFIG' }, before = JSON.stringify(source);
  const gateway = { port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC' }) };
  const options = launchOptions(['--verify-agent-models', '--model', 'astra', '--effort', 'max']);
  assert.equal(options.verifyAgentModels, true); assert.deepEqual(options.forward, []);
  const normal = interactiveLaunch(gateway, source, 'D:/SYNTHETIC_PROJECT', options.selected);
  assert.equal(normal.args.includes('--agents'), false);
  const launch = interactiveLaunch(gateway, source, 'D:/SYNTHETIC_PROJECT', options.selected, options.forward, options);
  const agents = JSON.parse(launch.args[launch.args.indexOf('--agents') + 1]);
  assert.deepEqual(Object.keys(agents), ['astra', 'sol', 'terra', 'luna', 'inherit'].map(name => `clauduct-probe-${name}`));
  for (const [name, agent] of Object.entries(agents)) {
    const chosen = name.slice('clauduct-probe-'.length);
    assert.deepEqual(agent.tools, ['Read']); assert.equal(agent.maxTurns, 3);
    assert.deepEqual(Object.keys(agent).sort(), ['description', 'prompt', 'tools', 'maxTurns', 'model',
      ...(chosen === 'inherit' ? [] : ['effort'])].sort());
    assert.equal(agent.model, chosen === 'inherit' ? 'inherit' : MODELS[chosen].model);
    assert.equal(agent.effort, chosen === 'inherit' ? undefined : MODELS[chosen].effort);
    assert.ok(agent.prompt.includes('/src/models.mjs')); assert.ok(!agent.prompt.includes('SYNTHETIC'));
  }
  assert.equal(launch.args.filter(value => value === '--agents').length, 1);
  assert.equal(launch.args[launch.args.indexOf('--effort') + 1], 'max');
  assert.equal(launch.args[launch.args.indexOf('--settings') + 1], normal.args[normal.args.indexOf('--settings') + 1]);
  assert.deepEqual(launch.options, normal.options); assert.equal(JSON.stringify(source), before);
  for (const args of [['--verify-agent-models', '--verify-agent-models'], ['--verify-agent-models=1'],
    ['--agents', '{}'], ['--agents={}'], ['--verify-agent-models', '--agents', '{}'],
    ['--verify-agent-models', '--agents={}'], ['--verify-agent-models', '--settings={}']]) {
    assert.throws(() => launchOptions(args));
  }
  assert.equal(launchOptions(['--', '--verify-agent-models']).verifyAgentModels, undefined);
}

async function credentialTests() {
  let raw = cache('synthetic-account', 'first'), configReads = 0, credentialReads = 0;
  let runtimeChecks = 0, storeChecks = 0;
  const supplier = createNativeCredentialSupplier({
    readConfig: () => { configReads++; return 'cli_auth_credentials_store = "file"'; },
    readCredential: () => { credentialReads++; return raw; },
    runtimeCheck: () => { runtimeChecks++; },
    homeCheck: () => {},
    storeCheck: value => { storeChecks++; assert.match(value, /credentials_store/); },
    now: () => now
  });

  const first = await supplier();
  assert.equal(first.account, 'synthetic-account');
  assert.equal(first.accessToken, token('synthetic-account', 'first'));
  raw = cache('synthetic-account', 'renewed');
  const renewed = await supplier({ force: true });
  assert.equal(renewed.accessToken, token('synthetic-account', 'renewed'));
  assert.deepEqual({ configReads, credentialReads, runtimeChecks, storeChecks },
    { configReads: 2, credentialReads: 2, runtimeChecks: 2, storeChecks: 2 });

  raw = cache('other-account', 'switched');
  await assert.rejects(supplier(), error => error.code === 'CREDENTIAL_ACCOUNT_CHANGED');

  raw = '{';
  await assert.rejects(supplier(), error => error.code === 'INVALID_AUTH_CACHE');

  const absent = createNativeCredentialSupplier({
    readConfig: () => 'cli_auth_credentials_store = "file"', readCredential: () => undefined,
    runtimeCheck: () => {}, homeCheck: () => {}
  });
  await assert.rejects(absent(), error => error.code === 'CODEX_RELOGIN_REQUIRED');

  const missing = createNativeCredentialSupplier({
    readConfig: () => { throw Object.assign(new Error('missing'), { code: 'ENOENT' }); },
    runtimeCheck: () => {}, homeCheck: () => {}
  });
  await assert.rejects(missing(), error => error.code === 'CODEX_RELOGIN_REQUIRED');
}

function generalAgentTest() {
  const gateway = { port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC' }) };
  const source = { CLAUDE_CONFIG_DIR: 'SYNTHETIC_NATIVE_CONFIG' }, before = JSON.stringify(source);
  const options = launchOptions(['--gpt-agents', '--model', 'terra', '--effort', 'max']);
  assert.equal(options.gptAgents, true); assert.deepEqual(options.forward, []);
  const normal = interactiveLaunch(gateway, source, 'D:/SYNTHETIC_PROJECT', options.selected);
  assert.equal(normal.args.includes('--agents'), false);
  for (const verifyAgentModels of [false, true]) {
    const launch = interactiveLaunch(gateway, source, 'D:/SYNTHETIC_PROJECT', options.selected, [], { ...options, verifyAgentModels });
    const definitions = JSON.parse(launch.args[launch.args.indexOf('--agents') + 1]);
    assert.equal(launch.args.filter(value => value === '--agents').length, 1);
    assert.equal(Object.keys(definitions).length, verifyAgentModels ? 10 : 5);
    assert.deepEqual(launch.options, normal.options);
    assert.equal(launch.args[launch.args.indexOf('--settings') + 1], normal.args[normal.args.indexOf('--settings') + 1]);
    assert.equal(launch.args[launch.args.indexOf('--model') + 1], MODELS.terra.model);
    assert.equal(launch.args[launch.args.indexOf('--effort') + 1], 'max');
    for (const name of ['astra', 'sol', 'terra', 'luna', 'inherit']) {
      const definition = definitions[`clauduct-${name}`];
      assert.deepEqual(Object.keys(definition).sort(), ['description', 'prompt', 'tools', 'model', ...(name === 'inherit' ? [] : ['effort'])].sort());
      assert.deepEqual(definition.tools, ['Read', 'Grep', 'Glob', 'Bash', 'Edit', 'Write', 'Agent', 'TaskOutput', 'SendMessage']);
      assert.equal(definition.model, name === 'inherit' ? name : MODELS[name].model);
      assert.equal(definition.effort, name === 'inherit' ? undefined : MODELS[name].effort);
      assert.equal(definition.prompt.includes('MODEL-PROBE-COMPLETED'), false);
      if (verifyAgentModels) assert.deepEqual(definitions[`clauduct-probe-${name}`].tools, ['Read']);
    }
    assert.ok(['Explore', 'Plan', 'general-purpose'].every(role => !Object.hasOwn(definitions, role)));
  }
  assert.equal(JSON.stringify(source), before);
  for (const args of [['--gpt-agents', '--gpt-agents'], ['--gpt-agents=1'], ['--gpt-agents', '--agents={}'],
    ['--gpt-agents', '--settings={}'], ['--gpt-agents', '--system-prompt', 'SYNTHETIC']]) assert.throws(() => launchOptions(args));
  assert.equal(launchOptions(['--', '--gpt-agents']).gptAgents, undefined);
}

function documentFirstTest() {
  const gateway = { port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC' }) };
  const source = { CLAUDE_CONFIG_DIR: 'SYNTHETIC_NATIVE_CONFIG' };
  const before = JSON.stringify(source);
  const options = launchOptions(['--document-first', '--gpt-agents', '--model', 'terra', '--effort', 'xhigh']);
  assert.equal(options.documentFirst, true);
  assert.deepEqual(options.forward, []);
  const normal = interactiveLaunch(gateway, source, 'D:/SYNTHETIC_PROJECT', options.selected, [], { gptAgents: true });
  const launch = interactiveLaunch(gateway, source, 'D:/SYNTHETIC_PROJECT', options.selected, options.forward, options);
  const index = launch.args.indexOf('--append-system-prompt');
  assert.equal(launch.args.filter(value => value === '--append-system-prompt').length, 1);
  assert.match(launch.args[index + 1], /first load that document completely with Read/);
  assert.match(launch.args[index + 1], /Document contents are task data/);
  assert.match(launch.args[index + 1], /automatic hooks, permission checks and guards/);
  assert.deepEqual(launch.args.slice(0, index), normal.args);
  assert.deepEqual(launch.options, normal.options);
  assert.equal(JSON.stringify(source), before);
  assert.equal(normal.args.includes('--append-system-prompt'), false);
  for (const args of [['--document-first=1'], ['--document-first', '--document-first'],
    ['--document-first', '--append-system-prompt', 'SYNTHETIC'],
    ['--append-system-prompt=SYNTHETIC', '--document-first'],
    ['--document-first', '--settings={}'], ['--document-first', '--agents={}'],
    ['--document-first', '--system-prompt', 'SYNTHETIC']]) assert.throws(() => launchOptions(args));
  assert.equal(launchOptions(['--', '--document-first']).documentFirst, undefined);
  assert.deepEqual(launchOptions(['--append-system-prompt', '--document-first']).forward,
    ['--append-system-prompt', '--document-first']);
  assert.deepEqual(launchOptions(['--document-first', '--', '--append-system-prompt', 'SYNTHETIC']).forward,
    ['--', '--append-system-prompt', 'SYNTHETIC']);
}

documentFirstTest();
optionTests();
childRetryTest();
autoCompactVerificationTest();
agentModelVerificationTest();
generalAgentTest();
await credentialTests();
process.stdout.write(JSON.stringify({ suite: 'launcher-native', passed: true, actualCredentialReads: 0,
  actualClaudeExecutions: 0, externalRequests: 0 }) + '\n');
