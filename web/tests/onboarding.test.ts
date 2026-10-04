import { test } from 'node:test';
import assert from 'node:assert/strict';
import { analysisFields, normalizeWebsite, onboardingCampaign, onboardingDefaults } from '../src/lib/onboarding';
test('reviewable AI fields and campaign preserve audience, delays and draft status', () => {
  const fields=analysisFields('```json\n{"product_summary":"CRM","buying_decision_makers":["Founder"],"end_users":{"role":"Sales"}}\n```');
  assert.equal(fields.buyers,'Founder'); assert.equal(fields.users,'role: Sales');
  assert.throws(()=>analysisFields('not JSON'));
  const campaign=onboardingCampaign({...onboardingDefaults,...fields,campaignName:' Launch ',firstDelay:4,followupDelay:5,roles:'Founder',accountIds:['sender']});
  assert.equal(campaign.status,'draft'); assert.equal(campaign.name,'Launch');
  assert.equal(campaign.workflow[1].days,4); assert.equal(campaign.workflow[4].days,5);
  assert.deepEqual(campaign.account_ids,['sender']); assert.match(campaign.criteria.instructions,/Decision makers: Founder/);
});

test('website input accepts bare domains and validates HTTP URLs', () => {
  assert.equal(normalizeWebsite(' kissaki.io '), 'https://kissaki.io/');
  assert.equal(normalizeWebsite('www.kissaki.io/pricing?q=1'), 'https://www.kissaki.io/pricing?q=1');
  assert.equal(normalizeWebsite('http://kissaki.io/path'), 'http://kissaki.io/path');
  assert.equal(normalizeWebsite('//kissaki.io'), 'https://kissaki.io/');
  for (const input of ['', 'not a website', 'https://', 'ftp://kissaki.io', 'javascript:alert(1)', 'https://user:password@kissaki.io']) assert.throws(() => normalizeWebsite(input));
});
