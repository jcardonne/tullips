import { createFileRoute } from "@tanstack/react-router";
import { Pool } from "pg";
const pool = new Pool({ connectionString: process.env.DATABASE_URL, max: 1 });
export const Route = createFileRoute("/healthz")({
  server: { handlers: { GET: async () => {
    try {
      await pool.query('SELECT id FROM "user" LIMIT 1');
      return Response.json({ status: "ok", version: process.env.RELEASE_VERSION || "dev" });
    } catch {
      return Response.json({ status: "unavailable" }, { status: 503 });
    }
  } } },
});
