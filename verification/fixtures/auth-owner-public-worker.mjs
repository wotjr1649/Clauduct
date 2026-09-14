import { createInterface } from 'node:readline';

const mode = process.argv[2];
if (!['normal','wrong-home','wrong-account','approval','unsolicited','duplicate','truncated','oversize','timeout'].includes(mode)) throw new Error('PUBLIC_OWNER_MODE');
const input = createInterface({ input: process.stdin });
const timer = setTimeout(() => process.exit(7), 3000);
const emit = value => process.stdout.write(JSON.stringify(value) + '\n');
const account = id => ({ id, result: { requiresOpenaiAuth: true,
  account: { type: mode === 'wrong-account' ? 'apiKey' : 'chatgpt', email: 'public@example.com', planType: 'pro' } } });
let step = 0;
try {
  for await (const line of input) {
    if (line.length > 1024) throw new Error('PUBLIC_OWNER_INPUT');
    const value = JSON.parse(line);
    if (value.method !== ['initialize','initialized','account/read','account/read'][step++]) throw new Error('PUBLIC_OWNER_INPUT');
    if (step === 1) {
      if (mode === 'timeout') continue;
      if (mode === 'oversize') { process.stdout.write(' '.repeat(16385)); continue; }
      const initial = { id: 0, result: { codexHome: mode === 'wrong-home' ? 'C:\\OtherFixture\\codex-home' : 'C:\\PublicFixture\\codex-home',
        platformFamily: 'windows', platformOs: 'windows', userAgent: 'PUBLIC_DISCARDED_AGENT' } };
      if (mode === 'approval') emit({ method: 'item/commandExecution/requestApproval', id: 9, params: { command: 'PUBLIC_UNEXECUTED_COMMAND' } });
      else if (mode === 'truncated') { process.stdout.write(JSON.stringify(initial)); break; }
      else if (mode === 'unsolicited') process.stdout.write(JSON.stringify(initial) + '\n' + JSON.stringify(account(1)) + '\n');
      else emit(initial);
    } else if (step === 3) {
      if (value.id !== 1 || value.params.refreshToken !== false) throw new Error('PUBLIC_OWNER_INPUT');
      emit(account(1));
    } else if (step === 4) {
      if (value.id !== 2 || value.params.refreshToken !== true) throw new Error('PUBLIC_OWNER_INPUT');
      if (mode === 'duplicate') process.stdout.write(JSON.stringify(account(2)) + '\n' + JSON.stringify(account(2)) + '\n');
      else emit(account(2));
    }
  }
} finally { clearTimeout(timer); input.close(); }
