CREATE TABLE IF NOT EXISTS installation (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    admin_user_id text,
    bootstrap_open boolean,
    maintenance boolean NOT NULL DEFAULT false
);
INSERT INTO installation(id) VALUES(true) ON CONFLICT DO NOTHING;
