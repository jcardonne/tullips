CREATE TABLE IF NOT EXISTS workspaces (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL, settings jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS memberships (
 workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE, user_id text NOT NULL, role text NOT NULL CHECK(role IN ('owner','member')), PRIMARY KEY(workspace_id,user_id)
);
CREATE TABLE IF NOT EXISTS records (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN ('campaigns','prospects','accounts','conversations')), data jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS records_workspace_kind ON records(workspace_id,kind);
CREATE UNIQUE INDEX IF NOT EXISTS prospect_linkedin_unique ON records(workspace_id,lower(data->>'linkedin_url')) WHERE kind='prospects' AND coalesce(data->>'linkedin_url','')<>'';
CREATE UNIQUE INDEX IF NOT EXISTS prospect_email_unique ON records(workspace_id,lower(data->>'email')) WHERE kind='prospects' AND coalesce(data->>'email','')<>'';
CREATE TABLE IF NOT EXISTS api_keys (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE, user_id text NOT NULL,
 name text NOT NULL, token_hash text UNIQUE NOT NULL, scopes text[] NOT NULL DEFAULT '{read}', created_at timestamptz NOT NULL DEFAULT now(), last_used_at timestamptz
);
CREATE TABLE IF NOT EXISTS events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE, actor text NOT NULL, action text NOT NULL, record_id text, data jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS workflow_versions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, campaign_id uuid NOT NULL REFERENCES records(id) ON DELETE CASCADE, workflow jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 campaign_id uuid REFERENCES records(id) ON DELETE CASCADE, prospect_id uuid REFERENCES records(id) ON DELETE CASCADE, account_id uuid REFERENCES records(id) ON DELETE CASCADE,
 kind text NOT NULL, payload jsonb NOT NULL DEFAULT '{}', status text NOT NULL DEFAULT 'pending', run_at timestamptz NOT NULL DEFAULT now(), attempts integer NOT NULL DEFAULT 0,
 locked_at timestamptz, last_error text, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS jobs_due ON jobs(run_at) WHERE status='pending';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS idempotency_key text UNIQUE;
CREATE TABLE IF NOT EXISTS enrollments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,campaign_id uuid NOT NULL REFERENCES records(id) ON DELETE CASCADE,
 prospect_id uuid NOT NULL REFERENCES records(id) ON DELETE CASCADE,account_id uuid NOT NULL REFERENCES records(id) ON DELETE CASCADE,workflow jsonb NOT NULL,
 status text NOT NULL DEFAULT 'active',step integer NOT NULL DEFAULT 0,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(campaign_id,prospect_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS enrollment_active ON enrollments(prospect_id) WHERE status='active';
CREATE TABLE IF NOT EXISTS credit_usage(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,job_id uuid REFERENCES jobs(id),model text NOT NULL,input_tokens bigint NOT NULL,output_tokens bigint NOT NULL,credits numeric NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE UNIQUE INDEX IF NOT EXISTS credit_usage_job ON credit_usage(job_id);
