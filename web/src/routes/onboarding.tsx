import { createFileRoute } from '@tanstack/react-router';
import { useEffect, useRef, useState } from 'react';
import { ArrowLeft, ArrowRight, Check, Flower2, Globe2, UsersRound, Sparkles } from 'lucide-react';
import { api, aiAction, type Row } from '../lib/api';
import { analysisFields, normalizeWebsite, onboardingCampaign, onboardingDefaults, type OnboardingDraft } from '../lib/onboarding';
export const Route = createFileRoute('/onboarding')({ component: Onboarding });
const steps = ['Your product', 'Your audience', 'Your campaign', 'Your sender', 'Ready to grow'];
function Onboarding() {
  const [draft, setDraft] = useState<OnboardingDraft>({...onboardingDefaults});
  const [ws,setWs] = useState(''), [owner,setOwner] = useState(true), [demo,setDemo] = useState(false), [loading,setLoading] = useState(true), [busy,setBusy] = useState(false), [analyzing,setAnalyzing] = useState(false), [error,setError] = useState(''), [notice,setNotice] = useState(''), [accounts,setAccounts] = useState<Row[]>([]), [suggestion,setSuggestion] = useState<Partial<OnboardingDraft> | null>(null);
  const [audienceEditing,setAudienceEditing] = useState(false);
  const [documents,setDocuments] = useState<{name:string;text:string}[]>([]);
  const heading = useRef<HTMLHeadingElement>(null);
  const base = `/workspaces/${ws}`;
  const change = (key: keyof OnboardingDraft, value: unknown) => setDraft(d => ({...d,[key]:value}));
  useEffect(() => {
    const params = new URLSearchParams(location.search);
    if (params.get('demo') === 'true') { setDemo(true); setLoading(false); return; }
    (async () => {
      const workspaces = await api('/workspaces');
      const selected = workspaces.find((w:Row) => w.id === params.get('workspace')) || workspaces[0];
      if (!selected) return;
      setWs(selected.id); setOwner(selected.role === 'owner');
      const [settings, senders] = await Promise.all([api(`/workspaces/${selected.id}/settings`), api(`/workspaces/${selected.id}/accounts`)]);
      const local = localStorage.getItem(`tullips:onboarding:${selected.id}`);
      let saved = settings.onboarding;
      if (local) { try { saved = JSON.parse(local); } catch { /* Ignore an invalid browser draft. */ } }
      if (saved && !saved.complete) setDraft({...onboardingDefaults,...saved,step:Math.max(0,Math.min(4,Number(saved.step)||0))});
      else setDraft(d => ({...d,workspaceName:selected.name}));
      setAccounts(senders.filter((a:Row)=>a.channel==='linkedin'));
    })().catch(e=>setError(e.message)).finally(()=>setLoading(false));
  },[]);
  useEffect(() => { if (!loading) heading.current?.focus(); },[draft.step,loading]);
  useEffect(() => {
    if (!ws || loading) return;
    try { localStorage.setItem(`tullips:onboarding:${ws}`,JSON.stringify(draft)); }
    catch { setNotice('Browser storage is unavailable. Use Continue to save your progress to the workspace.'); }
  },[draft,ws,loading]);
  async function persist(next: OnboardingDraft, workspace = ws) {
    if (!demo && workspace && owner) await api(`/workspaces/${workspace}/settings`,'PATCH',{onboarding:next});
    setDraft(next);
  }
  async function advance() {
    setBusy(true); setError('');
    try {
      const url = normalizeWebsite(draft.url);
      let workspace = ws;
      if (!demo && !workspace) {
        const created = await api('/workspaces','POST',{name:draft.workspaceName.trim() || new URL(url).hostname.replace(/^www\./,'')});
        workspace=created.id; setWs(workspace); setOwner(true);
        history.replaceState(null,'',`/onboarding?workspace=${workspace}`);
      }
      await persist({...draft,url,step:Math.min(4,draft.step+1)},workspace);
      if (draft.step === 0) await analyze(workspace, url);
    } catch(e) { setError((e as Error).message); } finally {setBusy(false);}
  }
  async function analyze(workspace = ws, website = draft.url) {
    setAnalyzing(true); setError(''); setSuggestion(null);
    try {
      if(demo) throw new Error('This is an interactive preview. Create an account and configure an AI provider to analyze your website. You can still try every step manually.');
      const result = await aiAction(`/workspaces/${workspace}`,'website.analyze',{url:normalizeWebsite(website),urls:draft.urls.split('\n').map(x=>x.trim()).filter(Boolean).map(normalizeWebsite),context:draft.context,documents});
      const text = result.text || JSON.stringify(result);
      change('analysis',text);
      try {setSuggestion(analysisFields(text));} catch {setNotice('Analysis received. Review the source text below and fill in your audience.');}
    } catch(e) {setError((e as Error).message);} finally {setAnalyzing(false);}
  }
  async function finish() {
    setBusy(true); setError('');
    try {
      if(demo) {location.href='/app?demo=true';return;}
      const campaign = await api(`${base}/campaigns${draft.campaignId?`/${draft.campaignId}`:''}`,draft.campaignId?'PATCH':'POST',onboardingCampaign(draft));
      const next={...draft,campaignId:draft.campaignId||campaign.id,complete:true};
      setDraft({...next,complete:false});
      // Keep the campaign ID before saving progress so a retry updates the same draft.
      try {localStorage.setItem(`tullips:onboarding:${ws}`,JSON.stringify({...next,complete:false}));} catch { /* Server persistence below remains available. */ }
      await persist(next);
      location.href=`/app?workspace=${ws}&tab=campaigns`;
    } catch(e) {setError((e as Error).message);} finally {setBusy(false);}
  }
  const field = (key:keyof OnboardingDraft,label:string,placeholder='',required=false) => <label>{label}<input value={String(draft[key])} onChange={e=>change(key,e.target.value)} placeholder={placeholder} required={required}/></label>;
  const area = (key:keyof OnboardingDraft,label:string,placeholder='',required=false) => <label>{label}<textarea value={String(draft[key])} onChange={e=>change(key,e.target.value)} placeholder={placeholder} required={required}/></label>;
  return <div className="onboarding-shell">
    <header className="onboarding-header"><a className="brand" href="/"><Flower2/>tullips</a><a href={demo?'/app?demo=true':ws?`/app?workspace=${ws}`:'/'} onClick={async e=>{if(!ws||demo)return;e.preventDefault();try{await persist(draft);location.href=`/app?workspace=${ws}`;}catch(e){setError((e as Error).message);}}}>{ws?'Save for later':'Exit setup'} <ArrowRight size={15}/></a></header>
    <div className={`onboarding-layout ${draft.step===0?'onboarding-intro-layout':''}`}>
    <main className="onboarding-main">{loading?<p role="status">Preparing your workspace…</p>:<>
      <nav className="onboarding-progress" aria-label="Setup progress">{steps.map((name,i)=><span key={name} aria-current={draft.step===i?'step':undefined} title={name}><i/>{draft.step===i&&<span>{name}</span>}</span>)}</nav>
      {error&&<p className="error" role="alert">{error}</p>}{notice&&<p className="onboarding-notice" role="status">{notice}</p>}
      <form onSubmit={e=>{e.preventDefault();void(draft.step===4?finish():advance());}}>
      <section className="onboarding-step" key={draft.step}>
      <div className={`ai-seed ${analyzing?'is-thinking':''}`} aria-hidden="true"><Sparkles size={24}/></div><h2 ref={heading} tabIndex={-1}>{['Your next customers start here.','Let’s find your people.','Make the first move yours.','Give your campaign a sender.','A thoughtful start. Ready when you are.'][draft.step]}</h2>
      <p className="onboarding-intro">{['Drop your website. Tullips will get to know your business and find who you should be talking to.','A little understanding before the first hello. Review what fits, and make it yours.','Start with LinkedIn. Every message and delay remains editable after setup.','Select the LinkedIn accounts that will share the campaign. You can connect them later from Accounts.','Check the details below. We’ll save a draft so you can review it before launching.'][draft.step]}</p>
      {draft.step===0&&<>
        <label className="website-entry"><span className="sr-only">Your website</span><Globe2 size={20}/><input type="text" inputMode="url" autoCapitalize="none" autoCorrect="off" spellCheck={false} required value={draft.url} placeholder="https://your-company.com" onChange={e=>change('url',e.target.value)}/></label>
        <details className="onboarding-details"><summary>Add a little context <span>Optional</span></summary>{!ws&&field('workspaceName','Workspace name','Defaults to your website name')}{area('context','Anything we should know?','A product detail, a market you have in mind…')}{area('urls','Additional URLs','One URL per line, up to 5')}<label>Product documents<input type="file" multiple accept=".txt,.md" onChange={async e=>{const files=Array.from(e.target.files||[]);if(files.length>5||files.some(f=>f.size>100000||!/\.(txt|md)$/i.test(f.name))){setError('Choose up to 5 .txt or .md files, each under 100 KB.');e.target.value='';setDocuments([]);return;}try{setDocuments(await Promise.all(files.map(async f=>({name:f.name,text:await f.text()}))));setError('');}catch{setError('A document could not be read. Please select it again.');}}}/><small>Files are used for this analysis only. Reselect them if you leave this page.</small></label></details>
      </>}
      {draft.step===1&&<>
        {analyzing?<div className="analysis-stage" role="status"><span className="analysis-orbit" aria-hidden="true"><Globe2 size={28}/></span><h3>Getting to know your business</h3><p>{draft.url}</p><div className="analysis-tasks"><span>Explore your website</span><span>Understand your product</span><span>Find the right people</span></div><small>We’re preparing your audience. This may take a few minutes.</small></div>:<div className="onboarding-ai"><Globe2 size={24}/><div><strong>{draft.url}</strong><p>Explore relevant pages and propose an audience you can review.</p></div><button type="button" disabled={analyzing} aria-busy={analyzing} onClick={()=>void analyze()}><Sparkles size={16}/>{draft.analysis?'Analyze again':'Analyze website'}</button></div>}
        {suggestion&&<div className="onboarding-suggestion"><strong>Suggested positioning</strong><p>{suggestion.summary}</p><dl><dt>Ideal customers</dt><dd>{suggestion.customers}</dd><dt>Decision makers</dt><dd>{suggestion.buyers}</dd><dt>End users</dt><dd>{suggestion.users}</dd></dl><button type="button" onClick={()=>{setDraft(d=>({...d,...suggestion,roles:suggestion.buyers||d.roles,campaignName:d.campaignName||'First conversations'}));setSuggestion(null);setAudienceEditing(true);setNotice('Your audience is ready to refine.');}}>Use these suggestions</button></div>}
        {draft.analysis&&<details className="onboarding-details"><summary>Read the full analysis and evidence</summary><pre>{draft.analysis}</pre></details>}
        {!analyzing&&!audienceEditing&&<button className="text-button audience-manual" type="button" onClick={()=>setAudienceEditing(true)}>{draft.summary?'Refine your audience':'Define your audience yourself'} <ArrowRight size={14}/></button>}
        {!analyzing&&audienceEditing&&<div className="audience-editor">{area('summary','Product summary','What does your product do?',true)}{area('customers','Ideal customers','Which businesses have the problem you solve?',true)}
        <div className="onboarding-grid">{area('buyers','Who makes the buying decision?','Founders, VP of Sales…',true)}{area('users','Who will use the product?','Account executives, sales operations…',true)}</div>
        <h3>Turn your audience into a search</h3><div className="onboarding-grid">{field('roles','Job titles','Founder, Head of Sales',true)}{field('geography','Geography','France, United Kingdom')}{field('industry','Industry','B2B software')}<label>Company size<select value={draft.companySize} onChange={e=>change('companySize',e.target.value)}><option value="">Any size</option>{['1–10','11–50','51–200','201–500','501–1000','1001+'].map(s=><option key={s}>{s}</option>)}</select></label>{field('seniority','Seniority','Director, VP, C-suite')}</div>{area('instructions','Describe anything else that matters','Include companies hiring sales teams. Exclude agencies and existing customers.')}</div>}
      </>}
      {draft.step===2&&<>
        {field('campaignName','Campaign name','First conversations with SaaS founders',true)}
        <details className="onboarding-details"><summary>Campaign preferences <span>Language, volume & automation</span></summary><div className="onboarding-grid">{field('language','Message language','English',true)}<label>Target prospects<input type="number" min={1} max={10000} required value={draft.target} onChange={e=>change('target',e.target.valueAsNumber)}/></label><label>Discovery mode<select value={draft.searchMode} onChange={e=>change('searchMode',e.target.value)}><option value="once">One-time search</option><option value="continuous">Keep finding prospects</option></select></label><label>Automation level<select value={draft.autonomy} onChange={e=>change('autonomy',e.target.value)}><option value="manual">Manual — approve each action</option><option value="review">Review — approve a sample first</option><option value="automatic">Automatic — run the approved workflow</option></select></label></div></details>
        <div className="onboarding-sequence"><h3><UsersRound size={18}/> Your LinkedIn sequence</h3><p>1. Send a connection request</p><label>Business days after invitation<input type="number" min={0} max={90} required value={draft.firstDelay} onChange={e=>change('firstDelay',e.target.valueAsNumber)}/></label><p className="muted small">The first message waits for acceptance. Unaccepted invitations expire from this sequence after 30 calendar days.</p>{area('message','2. First message','Use {{first_name}} and {{company}} to personalize.',true)}<label>Business days before follow-up<input type="number" min={1} max={90} required value={draft.followupDelay} onChange={e=>change('followupDelay',e.target.valueAsNumber)}/></label>{area('followup','3. Follow-up','Keep it helpful and human.',true)}<small>A reply always stops the sequence. Sending follows each account’s working hours.</small></div>
      </>}
      {draft.step===3&&<>
        {accounts.length?accounts.map(account=><label className="onboarding-account" key={account.id}><input type="checkbox" checked={draft.accountIds.includes(account.id)} onChange={e=>change('accountIds',e.target.checked?[...draft.accountIds,account.id]:draft.accountIds.filter(id=>id!==account.id))}/><UsersRound/><span><strong>{account.name||account.email}</strong><small>{account.status||'Not connected'} · {account.timezone||'Timezone not set'}</small></span></label>):<div className="onboarding-empty"><UsersRound size={32}/><h3>No LinkedIn account yet</h3><p>Connect your account securely from Accounts in a separate tab, including two-factor authentication if needed. Or finish your draft and connect later.</p></div>}
        {!demo&&<div className="onboarding-connect"><a className="primary" href={`/app?workspace=${ws}&tab=accounts`} target="_blank" rel="noopener noreferrer">Connect an account ↗</a><button type="button" disabled={busy} onClick={async()=>{setBusy(true);setError('');try{const rows=await api(`${base}/accounts`);setAccounts(rows.filter((a:Row)=>a.channel==='linkedin'));setNotice('Account list refreshed. Select your campaign senders.');}catch(e){setError((e as Error).message);}finally{setBusy(false);}}}>Refresh accounts</button></div>}
        <div className="onboarding-suggestion"><h3>A steady start</h3><p>New accounts start at 40 invitations and messages per day, then 60 after 14 days and 100 after 60 days.</p><p>Default activity: Monday–Friday, 9 am–6 pm in the sender’s timezone. Search and profile views have separate limits.</p><small>You can adjust these settings per account. Quotas are ceilings, not delivery guarantees.</small></div>
      </>}
      {draft.step===4&&<>
        <div className="onboarding-review">{[['Product',draft.summary,0],['Audience',`${draft.roles}\n${draft.geography||'Any geography'} · ${draft.companySize||'Any company size'}\n${draft.customers}`,1],['Campaign',`${draft.campaignName}\n${draft.target} prospects · ${draft.language} · ${draft.autonomy}`,2],['Sender',draft.accountIds.length?`${draft.accountIds.length} LinkedIn account(s) selected`:'Connect a LinkedIn account after saving',3]].map(([label,value,step])=><div key={String(label)}><span>{label}</span><p>{value}</p><button type="button" aria-label={`Edit ${label}`} onClick={()=>change('step',step)}>Edit</button></div>)}</div><div className="onboarding-notice"><Check size={18}/> Your campaign will be saved as a draft. Nothing will be sent during setup.</div>
      </>}
      </section><footer className={`onboarding-footer ${draft.step===0?'first-step-footer':''}`}><button type="button" disabled={draft.step===0||busy||analyzing} onClick={()=>change('step',draft.step-1)}><ArrowLeft size={16}/>Back</button><button className="primary" disabled={busy||analyzing||(draft.step===1&&!audienceEditing)} aria-busy={busy||analyzing} type="submit">{analyzing?'Understanding your business…':busy?'Saving…':draft.step===0?'Find my audience':draft.step===4?demo?'Finish preview':'Create campaign draft':draft.step===3&&!draft.accountIds.length?'Connect later & continue':'Continue'}<ArrowRight size={16}/></button></footer>
      </form>
    </>}</main></div>
  </div>;
}
