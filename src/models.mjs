// Claude 2.1.263 reserves 20K output tokens before applying the compact percentage.
export const CONTEXT_POLICY = Object.freeze({ window: 400000, compactAt: 320000, outputReserve: 20000,
  compactPercent: 320000 / (400000 - 20000) * 100 });
export const MODELS = Object.freeze({
  astra: Object.freeze({ model: 'gpt-6-astra', effort: 'medium' }),
  sol: Object.freeze({ model: 'gpt-5.6-sol', effort: 'xhigh' }),
  terra: Object.freeze({ model: 'gpt-5.6-terra', effort: 'high' }),
  luna: Object.freeze({ model: 'gpt-5.6-luna', effort: 'max' })
});
export const EFFORTS = Object.freeze(['low', 'medium', 'high', 'xhigh', 'max']);
// Main startup only; explicit model defaults and fixed agent definitions stay separate.
export const DEFAULT_SELECTION = Object.freeze({ model: MODELS.astra.model, effort: 'low' });
export function selectModel(value = 'astra', effort) {
  const selected = Object.hasOwn(MODELS, value) ? MODELS[value] : Object.values(MODELS).find(item => item.model === value);
  if (!selected || (effort !== undefined && !EFFORTS.includes(effort))) throw new Error('UNSUPPORTED_MODEL_OR_EFFORT');
  return { model: selected.model, effort: effort ?? selected.effort };
}
// Plan states its own route; it deliberately does not alias DEFAULT_SELECTION, which is the
// main startup value and must stay independently changeable.
export const ROLE_MODELS = Object.freeze({ Explore: MODELS.luna,
  Plan: Object.freeze({ model: MODELS.astra.model, effort: 'low' }), 'general-purpose': MODELS.luna });
