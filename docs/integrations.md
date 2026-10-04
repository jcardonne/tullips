# Integration status and validation

The supplied HAR was inspected offline, including base64-encoded bodies. No captured session was replayed and no real message was sent. No captured credentials, cookies, personal identifiers or message contents are stored in this repository. Provider tests use synthetic fixtures.

## LinkedIn

The HAR contains 1,182 entries. Implemented protocol coverage:

| Capability | Implementation | Remaining validation |
| --- | --- | --- |
| Login | Extract password authentication action from fresh HTML/React Flight; bind fresh credentials to state keys | Real account login and dynamic anti-abuse state |
| Checkpoint | Parse fresh form tokens and submit verification code | Real 2FA; push approval and non-code challenges |
| Search | Resolve shared RSC text and extract target-scoped result names, headlines, locations and URLs | Pagination consistency, company enrichment, filter IDs |
| Invitation | Extract an action matching the exact target profile URL; reject application errors | Live invitation and changing RSC templates |
| Identity | `/me` referenced miniProfile resolves the authenticated `fsd_profile` mailbox | Real account verification |
| Messaging | Captured Voyager `createMessage` body with stable deduplication token | New conversation creation and real sends |
| Inbox | Captured conversation/message GraphQL queries, normalized message parser | Complete pagination and live sync |
| Acceptance | Explicit `DISTANCE_1` participant metadata from inbox | Profile probe fails closed when explicit target relationship metadata is unavailable |

All LinkedIn HTTP uses `bogdanfinn/tls-client`, with a separate cookie jar per account. Session export is encrypted by the core account store, along with retained passwords and pending challenge state. Cookie domains are validated before restoration. Redirects are restricted to LinkedIn HTTPS; requests and responses are bounded. Checkpoints pause the account instead of silently retrying.

RSC requests use fresh page actions, not recorded HAR state. The password action is distinguished from Google/passkey actions. An invitation action must match the requested canonical profile, preventing recommended profiles from being invited accidentally. Unknown protocol shapes return errors. HTTP 200 alone is not treated as confirmation of an RSC invitation.

Offline people-page validation yielded seven target-scoped result cards with complete names, headlines and locations; unrelated profile links are excluded instead of borrowing neighboring metadata. Synthetic fixtures test shared references and cross-profile isolation. Company is left empty where there is no explicit evidence.

The capture does not contain new-conversation creation: conversation POSTs only update read state or typing, and all message sends reference an existing conversation. To complete first-contact messaging, capture sending a first message to a newly accepted connection with no prior thread. Captured profile pages also lack explicit machine-readable relationship degree; the fresh profile probe only confirms degree when present and scoped to the target. Inbox first-degree metadata is available and parsed.

These are implemented, offline-tested adapters, not a claim of successful end-to-end LinkedIn integration. `LIVE_SENDS` remains disabled by default. Authentication, 2FA, acceptance, inbox and send behavior require validation using accounts and designated recipients supplied later by the operator. Protocol changes can require updates.

## AI and websites

OpenAI-compatible LiteLLM chat requests return actual prompt/completion tokens, with output capped at 2,048 tokens. Cost uses configured per-million input/output prices and rounds up into credits. `EstimateMaxCredits` conservatively reserves one input token per UTF-8 byte plus overhead, and the maximum output. The workspace ledger checks available credits and records actual usage transactionally.

Configure `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL`, `LLM_INPUT_PRICE_PER_MILLION`, `LLM_OUTPUT_PRICE_PER_MILLION`, and `AI_CREDITS_PER_DOLLAR`. SaaS pricing must be configured before AI consumption; self-hosters supply their gateway credentials.

Website analysis follows at most eight same-origin HTML pages (five by default), with response/time bounds. Public-address checks happen at connection time; validated DNS addresses are pinned, redirects are revalidated, proxy environment variables are ignored, and private/link-local/special-use IPs are rejected. Website text is untrusted data in AI prompts. The onboarding also accepts five additional URLs and five TXT/Markdown documents (100 KB each); analysis results are saved per workspace. PDF/Word extraction and JavaScript-only website rendering are not included.

## Scheduling

Defaults are weekdays 09:00–18:00 in the account timezone; custom working days are supported (Sunday = 0). Business-day calculations preserve local wall time over DST. First LinkedIn message is due at the later of invitation + two business days and acceptance. Unaccepted invitations expire after 30 calendar days without automatic withdrawal. Follow-ups use three business days. Shared per-account quotas are 40 initially, 60 after 14 days, 100 after 60 days; research has a separate budget. These are configurable product limits, not a provider safety guarantee.

Conditions cover acceptance, email availability, recorded email opens/link clicks/message-seen events, CRM status/tags, no reply, and explicit AI decisions. A missing AI decision fails closed. Reply detection always stops sending. Event collection must be enabled before corresponding conditions can become true; email tracking pixels and click redirect collection are not implemented by these adapters.

## Email

SMTP supports mandatory TLS (465) or STARTTLS (587), authenticated with a password or OAuth token. IMAP uses TLS (993), UID-based incremental inbox reads and UIDVALIDITY. MIME text extraction is bounded and ignores attachments. Email headers reject CRLF injection. SMTP failures after DATA are marked ambiguous: Message-ID is useful for threading but is not a provider idempotency guarantee.

Google and Microsoft OAuth configurations support offline tokens and XOAUTH2 for SMTP/IMAP. Configure provider client IDs/secrets and deployment callback URLs. Google may require app verification for mail scopes; Microsoft tenant policies and SMTP AUTH settings may prevent access. Configuring OAuth does not guarantee provider approval or deliverability. Both flows require real mailbox validation before production.

Mail server destinations are limited to public IPs. Self-hosting does not implicitly grant access to private-network SMTP/IMAP servers.

## Offline checks

Run `cd backend && go test ./internal/automation ./internal/integrations` for DST/custom-day scheduling, warmup boundaries, stop-on-reply, missing AI decisions, SSRF rejection, token accounting, MIME parsing, header injection, RSC action matching, redirect isolation, mailbox identity and cookie persistence.

References: [TLS client](https://github.com/bogdanfinn/tls-client), [LiteLLM proxy](https://docs.litellm.ai/docs/proxy/user_keys), [Google XOAUTH2](https://developers.google.com/workspace/gmail/imap/xoauth2-protocol), [Microsoft mail OAuth](https://learn.microsoft.com/en-us/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth).
