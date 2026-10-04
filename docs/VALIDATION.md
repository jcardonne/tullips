# Validation and launch requirements

## Reproducible local checks

- `cd backend && go test ./... && go vet ./...`: deterministic workflow, security, parser, mail and billing checks.
- Set `TEST_DATABASE_URL` to a disposable PostgreSQL database and run Go tests to exercise durable enrollment, acceptance waiting and stop-on-reply. The test creates and removes its own workspace.
- `cd web && npm run typecheck && npm test && npm run build`: types, proxy isolation and production build.
- `python3 scripts/smoke.py`: running web + API + PostgreSQL acceptance test covering sign-up, persistence, API-key scopes and revocation, MCP tool calls, OAuth PKCE, single-use codes and token revocation. No external providers are invoked.
- `docker compose up --build`: complete local stack, with optional `--profile ai` for LiteLLM.

The interactive demo at `/app?demo=true` is explicitly separate from persisted authenticated workspaces. It uses synthetic records and never sends messages.

## Provider acceptance still required

No live LinkedIn accounts, email accounts, AI provider keys, or Stripe credentials were supplied for end-to-end provider verification. No captured credentials have been replayed. Offline parser success is not evidence that LinkedIn currently accepts the complete protocol.

The supplied LinkedIn HAR includes message creation inside an **existing conversation**, but no creation of a **new conversation**. To implement that missing request accurately, capture sending a first message to a designated, newly accepted test connection with no existing thread. Capture the preceding page load and request/response bodies. Keep the HAR outside the repository; it can contain passwords, cookies and private messages. Until then, Tullips must report that first-conversation creation is unsupported rather than fabricate a successful send.

Later acceptance checks should cover fresh login, each encountered 2FA/checkpoint type, invitation acceptance without an existing thread, new and existing conversations, reply synchronization, quota exhaustion, session expiry and reconnect. Google/Microsoft registrations require the exact `${APP_URL}/oauth/mail/callback` URL and provider-approved permissions. SMTP/IMAP requires a public TLS endpoint.

For SaaS launch, choose the hosted model, model pricing conversion, workspace/extra-account/top-up Stripe Prices and credit allowances. Register and test signed Stripe webhooks and configure applicable tax collection. Complete these settings before taking live payments.

## Operational limits

The worker deliberately serializes dispatch through a PostgreSQL advisory lock to keep quotas correct. Per-account concurrency can replace this when throughput warrants it. Ambiguous outbound delivery is held for review rather than automatically retried. The crawler reads public HTML; JavaScript-only rendering and PDF/Word parsing are not included. TXT/Markdown attachments are supported. Remote MCP OAuth access lasts one hour and currently requires reauthorization rather than refresh tokens.

## Onboarding and interaction update

- Web build and TypeScript validation passed; 4 web tests passed, including reviewable AI field parsing and draft campaign construction.
- Browser verified password visibility toggle, five onboarding stages, manual targeting, persistence across reload, and creation of a real local draft campaign (6 workflow steps). Synthetic workspace removed after validation.
- Mobile review screen checked at 390 px with no horizontal overflow. Landing preview tabs verified interactively. Motion CSS respects reduced-motion preferences.
- Live AI website analysis and LinkedIn connection remain dependent on configured providers and real account validation; the onboarding supports manual setup and deferred sender connection.
