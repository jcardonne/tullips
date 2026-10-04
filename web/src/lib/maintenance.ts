import { Pool } from "pg";
const pool = new Pool({ connectionString: process.env.DATABASE_URL, max: 2 });
export async function maintenanceResponse(): Promise<Response | null> {
  try {
    const result = await pool.query("SELECT maintenance FROM installation WHERE id");
    if (result.rows[0]?.maintenance === false) return null;
  } catch { /* Fail closed while migrations or recovery are in progress. */ }
  return Response.json({ error: "The installation is being updated. Please try again shortly." }, { status: 503, headers: { "Retry-After": "30" } });
}
