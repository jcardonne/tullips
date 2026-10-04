import { starterWorkflow } from './api';
export const onboardingDefaults = {
  step: 0, workspaceName: '', url: '', context: '', urls: '', analysis: '',
  summary: '', customers: '', buyers: '', users: '', roles: '', geography: '', industry: '', companySize: '', seniority: '', instructions: '',
  campaignName: '', language: 'English', autonomy: 'review', target: 100, searchMode: 'once', accountIds: [] as string[],
  invitation: '', message: String(starterWorkflow[3].message || ''), followup: String(starterWorkflow[5].message || ''), firstDelay: 2, followupDelay: 3,
  campaignId: '', complete: false,
};
export type OnboardingDraft = typeof onboardingDefaults;
export function analysisFields(text: string): Partial<OnboardingDraft> {
  const value = JSON.parse(text.trim().replace(/^```(?:json)?\s*/i, '').replace(/\s*```$/, ''));
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('The analysis is available below. Copy the relevant details into your audience fields.');
  const display = (v: unknown): string => v == null ? '' : typeof v === 'string' ? v : Array.isArray(v) ? v.map(display).join('\n') : typeof v === 'object' ? Object.entries(v).map(([k,v]) => `${k}: ${display(v)}`).join('\n') : String(v);
  return { summary: display(value.product_summary), customers: display(value.ideal_customers), buyers: display(value.buying_decision_makers), users: display(value.end_users), instructions: display(value.targeting_criteria) };
}
export function onboardingCampaign(d: OnboardingDraft) {
  return { name: d.campaignName.trim(), description: d.summary, status: 'draft', language: d.language, autonomy: d.autonomy, target_count: Number(d.target), search_mode: d.searchMode, account_ids: d.accountIds,
    criteria: { roles: d.roles, geography: d.geography, industry: d.industry, company_size: d.companySize, seniority: d.seniority, instructions: [d.instructions, `Ideal customers: ${d.customers}`, `Decision makers: ${d.buyers}`, `End users: ${d.users}`].join('\n') },
    workflow: starterWorkflow.map((s, i) => ({ ...s, ...(i === 0 ? {message:d.invitation} : i === 1 ? {days:Number(d.firstDelay),label:`Wait ${d.firstDelay} business days from invitation`} : i === 3 ? {message:d.message} : i === 4 ? {days:Number(d.followupDelay),label:`Wait ${d.followupDelay} business days`} : i === 5 ? {message:d.followup} : {}) })),
  };
}

export function normalizeWebsite(value: string): string {
  const input = value.trim();
  try {
    const url = new URL(input.startsWith('//') ? `https:${input}` : /^[a-z][a-z\d+.-]*:\/\//i.test(input) ? input : `https://${input}`);
    if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password || !url.hostname.includes('.')) throw new Error();
    return url.href;
  } catch {
    throw new Error('Enter a valid website, like kissaki.io or https://kissaki.io.');
  }
}
