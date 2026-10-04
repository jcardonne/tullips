export type Row = { id: string; [key: string]: any };
export async function api(path: string, method = "GET", body?: unknown) {
  const response = await fetch(`/api/v1${path}`, {
    method,
    headers:
      body === undefined
        ? {}
        : {
            "Content-Type": "application/json",
            "Idempotency-Key": crypto.randomUUID(),
          },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok)
    throw new Error(data.error || `Request failed (${response.status})`);
  return data;
}
export const starterWorkflow = [
  { type: "invitation", label: "Send a connection request", message: "" },
  { type: "delay", label: "Wait 2 business days from invitation", days: 2 },
  {
    type: "condition",
    label: "Wait until invitation accepted",
    condition: "accepted",
    timeout_days: 30,
  },
  {
    type: "linkedin_message",
    label: "Start a conversation",
    message:
      "Hi {{first_name}}, I noticed your work at {{company}}. Would you be open to exchanging ideas?",
  },
  { type: "delay", label: "Wait 3 business days", days: 3 },
  {
    type: "linkedin_message",
    label: "Follow up if no reply",
    message:
      "Hi {{first_name}}, just following up. Would this be relevant to your team?",
  },
];
export const demoCampaigns: Row[] = [
  {
    id: "c1",
    name: "Founders, meet your next chapter",
    description: "B2B SaaS founders · United Kingdom & France",
    status: "active",
    autonomy: "review",
    language: "English",
    target_count: 250,
    search_mode: "continuous",
    criteria: {
      roles: "Founder, CEO",
      geography: "United Kingdom, France",
      company_size: "11–50",
      industry: "Software",
    },
    workflow: starterWorkflow,
    account_ids: [],
    prospects: 128,
  },
  {
    id: "c2",
    name: "A little more room to grow",
    description: "Heads of Growth · European tech companies",
    status: "draft",
    autonomy: "manual",
    language: "English",
    target_count: 150,
    search_mode: "once",
    criteria: {},
    workflow: starterWorkflow,
    account_ids: [],
    prospects: 64,
  },
];
export const demoProspects: Row[] = [
  {
    id: "p1",
    first_name: "Olivia",
    last_name: "Bennett",
    title: "Co-founder & CEO",
    company: "Forma",
    status: "connected",
    tags: ["Decision maker"],
    email: "",
    notes: "Interested in building a more thoughtful outbound process.",
  },
  {
    id: "p2",
    first_name: "Lucas",
    last_name: "Martin",
    title: "Head of Growth",
    company: "Layers",
    status: "replied",
    tags: ["High intent"],
    email: "",
    notes: "",
  },
  {
    id: "p3",
    first_name: "Amelia",
    last_name: "Clarke",
    title: "Founder",
    company: "Offscript",
    status: "new",
    tags: ["Decision maker"],
    email: "",
    notes: "",
  },
  {
    id: "p4",
    first_name: "Noah",
    last_name: "Dubois",
    title: "VP Marketing",
    company: "Northstar",
    status: "invited",
    tags: ["Champion"],
    email: "",
    notes: "",
  },
  {
    id: "p5",
    first_name: "Sofia",
    last_name: "Rossi",
    title: "Co-founder",
    company: "Goodkind",
    status: "connected",
    tags: ["Decision maker"],
    email: "",
    notes: "",
  },
];

export async function aiAction(base: string, kind: string, payload: unknown) {
  const job = await api(`${base}/actions`, "POST", { kind, payload });
  for (let i = 0; i < 150; i++) {
    await new Promise((resolve) => setTimeout(resolve, 2000));
    const jobs = await api(`${base}/actions`);
    const current = jobs.find((j: Row) => j.id === job.id);
    if (current?.status === "completed") return current.payload.result;
    if (["failed", "blocked", "delivery_uncertain"].includes(current?.status))
      throw new Error(
        current.last_error || "The operation could not complete.",
      );
  }
  throw new Error("Still processing. Check back in your workspace activity.");
}

export function removeWorkflowStep(
  steps: Record<string, any>[],
  index: number,
) {
  return steps
    .filter((_, i) => i !== index)
    .map((step) => {
      const updated = { ...step };
      for (const key of ["then_step", "else_step"]) {
        if (updated[key] === index) delete updated[key];
        else if (updated[key] > index) updated[key]--;
      }
      return updated;
    });
}
