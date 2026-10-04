# Tullips 🌷

A self-hostable, LinkedIn-first prospecting workspace. Analyze your product, define your audience, organize prospects, and prepare outreach with an AI assistant. MIT licensed. The hosted edition uses the same application with workspace subscriptions and AI credits.

## Run locally

Requirements: Docker with Compose, Python 3 for local secret generation.

```sh
python3 scripts/setup.py
docker compose up --build
```

Open <http://localhost:3000>, create an account, then create a workspace. Secrets are generated in an ignored, owner-readable `.env`; keep the encryption key backed up because changing it makes stored account credentials unreadable. PostgreSQL data persists in a Docker volume. API and database ports are private; only the web service is bound to localhost. For public hosting, use an HTTPS reverse proxy and set `APP_URL` to its canonical URL.

The application starts without provider credentials. AI operations report configuration errors until a model is configured. Real outbound delivery is off by default (`LIVE_SENDS=false`). There are no bundled LinkedIn credentials, HAR files, or live customer records.

## AI gateway

Set `LLM_BASE_URL`, `LLM_API_KEY`, and `LLM_MODEL` for an OpenAI-compatible gateway. To use the bundled LiteLLM service, configure the provider key (`OPENAI_API_KEY` or `OPENROUTER_API_KEY`), `LITELLM_MODEL` (provider/model), set `LLM_MODEL=tullips`, set `LLM_API_KEY` to the generated `LITELLM_MASTER_KEY`, and run:

```sh
docker compose --profile ai up --build
```

For OpenRouter, set `OPENROUTER_API_KEY` in your local `.env` and
`LITELLM_MODEL=openrouter/openai/gpt-6-luna-pro`. LiteLLM resolves the provider's
key automatically; the application uses the `tullips` alias and the LiteLLM
master key, not the upstream key. After changing `.env`, recreate the gateway,
API and worker: `docker compose --profile ai up -d litellm api worker`.

Self-hosted operators can change the gateway configuration to their provider. Hosted mode imposes an operator-configured model; users cannot supply their own keys. Model choice and prices are not preset commercial decisions. `LLM_INPUT_PRICE_PER_MILLION`, `LLM_OUTPUT_PRICE_PER_MILLION`, and `AI_CREDITS_PER_DOLLAR` define actual usage accounting.

## Accounts and workflows

Workspace owners manage membership, billing and workspace deletion. Members can manage prospects, campaigns and sender accounts. The CRM supports tags, notes, statuses, filters and CSV import/export. Campaign workflows are versioned; existing enrollments keep their snapshot. LinkedIn is the primary channel, email secondary.

Default schedule: weekdays 09:00–18:00 in the sender timezone. Invitations and messages share a daily account budget of 40, then 60 after 14 days and 100 after 60 days. Search/profile budgets are separate. These defaults do not imply approval or guaranteed deliverability by LinkedIn.

The default sequence waits for invitation acceptance and two business days from invitation before messaging, then waits three business days before a follow-up. Replies stop the sequence; unanswered invitations expire after 30 calendar days without withdrawal.

Read [integration coverage](integrations.md) before enabling any external delivery. LinkedIn's captured protocols are private and can change. Offline tests are not a substitute for real account testing; real LinkedIn and mailbox tests have not been authorized or performed.

## MCP and API access

Create a workspace API key in Settings. Keys default to `read`, with separate `write`, `launch` and `send` scopes. The MCP endpoint is `${APP_URL}/mcp`; send the key in `Authorization: Bearer ...`. Tools require the workspace ID associated with the credential. Revoking a key or workspace membership removes access.

OAuth discovery is served at `/.well-known/oauth-protected-resource` and `/.well-known/oauth-authorization-server`. Agents can register a public client and use authorization code flow with S256 PKCE, the exact registered redirect URI, and `resource=${APP_URL}/mcp`. The consent screen selects workspace and scopes. Access tokens expire after one hour; reauthorize to renew. Revoke through `/oauth/revoke` or the user's connected-agent settings.

## Hosted billing

Set `DEPLOYMENT_MODE=saas`. Configure Stripe restricted API credentials, webhook signing secret, workspace monthly Price ID, additional LinkedIn account Price ID and credit top-up Price ID. Set `MONTHLY_AI_CREDITS` and `TOPUP_AI_CREDITS`. No prices are invented by the application. Three LinkedIn accounts are included in the workspace plan.

Register `${APP_URL}/api/stripe/webhook` for `customer.subscription.created`, `customer.subscription.updated`, `customer.subscription.deleted`, `invoice.paid`, `invoice.payment_failed`, `checkout.session.completed`, and `checkout.session.async_payment_succeeded`. Signed webhooks update billing; the success page never grants credits. Configure applicable tax registrations and collection before live sales; automatic tax is not enabled by default.

## Development and checks

```sh
cd backend
go test ./...
go vet ./...
cd ../web
npm ci
npm run typecheck
npm run build
```

Use `docker compose logs --tail=100 api worker web` for local diagnostics. Do not publish logs containing account data. The agreed scope is recorded in [docs/SPEC.md](SPEC.md). Commercial settings and real-provider acceptance checks must be completed before public launch.

For native development, expose the database on loopback with `docker compose -f compose.yaml -f compose.dev.yaml up -d postgres`, then run `python3 scripts/dev.py migrate`, and start `api`, `worker`, and `web` in separate terminals using the same script. Run `python3 scripts/smoke.py` against the running web service for the auth/CRM/API-key/OAuth/MCP acceptance check. It creates a temporary workspace and removes it afterwards; its synthetic auth user remains in the local database.
