# Tullips — agreed product specification

## Distribution and architecture
MIT licensed, identical self-hosted and hosted functionality. Self-hosting is free with operator-owned model keys; hosted billing is per workspace, including three LinkedIn accounts with paid additional accounts. No hosted BYOK. Prices, monthly included credits, and hosted model provider are intentionally configurable and undecided.

TanStack Start/React English frontend, Better Auth email/password authentication, Go API and worker, PostgreSQL persistence and durable queue, LiteLLM gateway, Docker Compose. Cream, leaf green, tulip pink; a restrained floral identity. No live account testing or replay of HAR credentials authorized. Simulated tests first, real integration validation later.

## Workspaces and CRM
Multiple workspaces, sites/products, members and LinkedIn accounts. Owner manages billing, membership and deletion; members manage campaigns, prospects and senders. Table CRM: filters, tags, notes, history, statuses, CSV import/export, prospect edits and individual workflow overrides. Deduplicate within workspace; prevent concurrent sequences for the same prospect. Keep each prospect's assigned sender throughout a sequence.

## AI and discovery
Analyze supplied landing page and relevant same-site pages; accept additional URLs and documents. Propose editable customer profiles, buyers, decision-makers and actual product users. Use explicit geography, company, seniority and experience filters alongside natural-language instructions. Never infer age. LinkedIn classic accounts only, no Sales Navigator. Discover emails only from LinkedIn or CSV imports, no paid enrichment or external email discovery.

Campaigns choose one-off discovery or continuous discovery up to a target with daily search budget. User selects sender accounts; allocate by available account quota. Language per campaign with prospect overrides. AI side panel proposes visible changes before application. Three selectable autonomy levels: review every prospect/message; approve sample/instructions then automate; automatic after launch.

## Sequences
Editable ordered steps with delays and branches, both forms and AI. Actions include LinkedIn invitation/message and secondary email. Conditions include acceptance, email presence, no reply, CRM tags/status/fields/AI score and custom AI instructions. LinkedIn is primary everywhere.

Default: invite; first message at two business days after invitation if accepted, otherwise next allowed activity window following acceptance; follow-up three business days after first message without reply. Expire unaccepted invitation after 30 calendar days without withdrawing it. Any reply stops the sequence. Unified inbox: AI drafts, human sends. Campaign edits affect new enrollments by default; explicit opt-in updates remaining unexecuted steps for existing prospects, preserving individual overrides.

## Scheduling and accounts
Per-account invitation+message budget: 40/day initially, 60 after 14 calendar days, 100 after 60 days. Separately configurable search/profile budgets. Weekdays 09:00–18:00 in account timezone, adjustable and manually pausable. Shared budgets across campaigns. These are product settings, not platform guarantees.

LinkedIn email/password and interactive 2FA, password and isolated cookie/session jars encrypted at rest. Reconnect automatically where supported; ask for 2FA/checkpoint handling when needed. All LinkedIn HTTP uses bogdanfinn/tls-client. No HAR secrets or personal records committed. Google/Microsoft mailbox OAuth and other providers SMTP/IMAP, encrypted credentials.

## Credits and billing
Stripe workspace subscription, extra LinkedIn account quantity above three, prepaid credit top-ups. Only real AI consumption spends credits; configurable rate and batch estimates. Exhaustion pauses AI work; already prepared messages continue. Signed idempotent webhooks control entitlements, never browser checkout completion alone. Taxes and commercial settings must be configured before live billing.

## MCP
Remote MCP via API keys or OAuth. Read-only default, independent read/write/launch/send scopes, selected workspace only, current membership checked, revocable. External agent actions obey the same permissions, approvals, scheduling and quotas as the UI.

## Delivery validation
Offline tests for schedule, quotas, workflow conditions, security boundaries and provider fixtures. Build/typecheck frontend, Go tests, Compose startup and browser walkthrough. Real LinkedIn/email/OAuth/provider/Stripe end-to-end tests remain gated by operator setup and dedicated test accounts; incomplete captured protocols must fail explicitly rather than simulate success.
