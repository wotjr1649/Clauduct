// Clauduct records only routing choices in the native child label. The original
// prompt and all other options still go through native's Workflow VM and guards.
function agent(prompt, options = {}) {
  const catalogue = __CLAUDUCT_CATALOGUE__;
  const parent = __CLAUDUCT_PARENT__;
  if (!options || typeof options !== 'object' || Array.isArray(options)) throw Error('CLAUDUCT_WORKFLOW_OPTIONS_UNSUPPORTED');
  const opts = {...options};
  if (Object.hasOwn(opts, 'maxTurns')) throw Error('CLAUDUCT_WORKFLOW_OPTION_UNSUPPORTED: use a native agent definition for maxTurns; no agent started');
  const hasTools = Object.hasOwn(opts, 'tools');
  if (hasTools && (!Array.isArray(opts.tools) || opts.tools.length > 64 || opts.tools.some(v => typeof v !== 'string' || !/^[A-Za-z0-9_-]{1,200}$/.test(v)) || new Set(opts.tools).size !== opts.tools.length)) throw Error('CLAUDUCT_WORKFLOW_TOOLS_INVALID');
  const hasRole = Object.hasOwn(opts, 'agentType');
  if (hasRole && (typeof opts.agentType !== 'string' || !/^[A-Za-z0-9_:-]{1,200}$/.test(opts.agentType))) throw Error('CLAUDUCT_WORKFLOW_ROLE_INVALID');
  const hasModel = Object.hasOwn(opts, 'model'), hasEffort = Object.hasOwn(opts, 'effort');
  const model = hasModel ? opts.model : null, effort = hasEffort ? opts.effort : null;
  const explicit = hasModel && model !== 'inherit';
  const selected = explicit && typeof model === 'string' && Object.hasOwn(catalogue, model) ? catalogue[model] : explicit ? null : parent;
  if (!selected || hasEffort && !['low','medium','high','xhigh','max'].includes(effort)) throw Error('UNSUPPORTED_MODEL_OR_EFFORT: use a listed Clauduct model and effort');
  const chosenEffort = hasEffort ? effort : selected[1];
  const label = opts.label === undefined ? 'agent' : opts.label;
  if (typeof label !== 'string' || label.length > 1024) throw Error('CLAUDUCT_WORKFLOW_LABEL_UNSUPPORTED');
  const proof = [__CLAUDUCT_CALL__,model,effort];
  if (hasTools || hasRole) proof.push({...hasTools?{tools:opts.tools}:{},...hasRole?{agentType:opts.agentType}:{}});
  delete opts.tools; // Enforced by the gateway, not an ignored native VM option.
  if (hasRole && !hasModel) return globalThis.agent(prompt, {...opts,
    label:label+' [clauduct:'+JSON.stringify(proof)+']'});
  return globalThis.agent(prompt, {...opts, model:selected[0], effort:chosenEffort,
    label:label+' [clauduct:'+JSON.stringify(proof)+']'});
}
