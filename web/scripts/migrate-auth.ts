import { getMigrations } from "better-auth/db/migration";
import { Pool } from "pg";
import { auth } from "../src/lib/auth";
const pool = new Pool({ connectionString: process.env.DATABASE_URL });
const client = await pool.connect();
try {
  await client.query("SELECT pg_advisory_lock(7483100)");
  const version = process.env.RELEASE_VERSION || "dev";
  const name = `auth/${version}`;
  const applied = await client.query("SELECT 1 FROM schema_migrations WHERE name=$1", [name]);
  if (version === "dev" || applied.rowCount === 0) {
    const { runMigrations } = await getMigrations(auth.options);
    await runMigrations();
    // The insert trigger serializes concurrent first registrations inside their transactions.
    // Existing users are deliberately never promoted automatically.
    await client.query(`
      UPDATE installation SET bootstrap_open=NOT EXISTS(SELECT 1 FROM "user")
        WHERE id AND bootstrap_open IS NULL;
      CREATE OR REPLACE FUNCTION claim_installation_admin() RETURNS trigger LANGUAGE plpgsql AS $$
      BEGIN
        UPDATE installation SET admin_user_id=NEW.id, bootstrap_open=false
          WHERE id AND bootstrap_open=true;
        RETURN NEW;
      END $$;
      DROP TRIGGER IF EXISTS installation_first_user ON "user";
      CREATE TRIGGER installation_first_user AFTER INSERT ON "user"
        FOR EACH ROW EXECUTE FUNCTION claim_installation_admin();
    `);
    await client.query("INSERT INTO schema_migrations(name,checksum) VALUES($1,'better-auth') ON CONFLICT DO NOTHING", [name]);
  }
  console.log("Database authentication migrations complete");
} finally {
  await client.query("SELECT pg_advisory_unlock(7483100)");
  client.release();
  await pool.end();
}
process.exit(0);
