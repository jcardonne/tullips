import { UpdatePanel } from "../lib/update-panel";
import { BillingPanel } from "../lib/billing-panel";
import { Tulip as Flower2 } from "../lib/tulip";
import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState, type ReactNode } from "react";
import {
  LayoutDashboard,
  Users,
  Send,
  Inbox,
  Settings,
  ChevronDown,
  Plus,
  ArrowUpRight,
  ArrowRight,
  Search,
  Sparkles,
  X,
  Check,
  ContactRound as Linkedin,
  Mail,
  MoreHorizontal,
  Clock3,
  GitBranch,
  Play,
  Pause,
  Download,
  Upload,
  KeyRound,
  CreditCard,
  Sprout,
  CircleHelp,
  LogOut,
  PanelRightClose,
  Globe2,
} from "lucide-react";
import {
  api,
  aiAction,
  removeWorkflowStep,
  demoCampaigns,
  demoProspects,
  starterWorkflow,
  type Row,
} from "../lib/api";
export const Route = createFileRoute("/app")({ component: App });
const nav = [
  ["overview", "Overview", LayoutDashboard],
  ["campaigns", "Campaigns", Send],
  ["prospects", "Prospects", Users],
  ["inbox", "Inbox", Inbox],
  ["accounts", "Connected accounts", Linkedin],
  ["settings", "Workspace settings", Settings],
] as const;
function Badge({ children }: { children: ReactNode }) {
  return (
    <span className={`badge ${String(children).toLowerCase()}`}>
      {children}
    </span>
  );
}
function App() {
  const [installationAdmin, setInstallationAdmin] = useState(false);
  useEffect(() => {
    if (new URLSearchParams(location.search).get("demo") !== "true")
      api("/installation").then(r => setInstallationAdmin(r.administrator)).catch(() => {});
  }, []);
  const [demo, setDemo] = useState(false),
    [page, setPage] = useState("overview"),
    [workspaces, setWorkspaces] = useState<Row[]>([]),
    [ws, setWs] = useState(""),
    [campaigns, setCampaigns] = useState<Row[]>([]),
    [prospects, setProspects] = useState<Row[]>([]),
    [accounts, setAccounts] = useState<Row[]>([]),
    [conversations, setConversations] = useState<Row[]>([]),
    [keys, setKeys] = useState<Row[]>([]),
    [members, setMembers] = useState<Row[]>([]),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [loading, setLoading] = useState(true),
    [aiOpen, setAiOpen] = useState(true),
    [modal, setModal] = useState<{ kind: string; row?: Row } | null>(null),
    [query, setQuery] = useState(""),
    [filter, setFilter] = useState("all"),
    [settingsTab, setSettingsTab] = useState("workspace"),
    [aiText, setAiText] = useState(""),
    [aiAnswer, setAiAnswer] = useState(""),
    [proposal, setProposal] = useState<any>(null),
    [busy, setBusy] = useState(false),
    [secret, setSecret] = useState(""),
    [grants, setGrants] = useState<Row[]>([]),
    [verificationCode, setVerificationCode] = useState(""),
    [jobs, setJobs] = useState<Row[]>([]),
    [sampleReviewId, setSampleReviewId] = useState<string | null>(null),
    [workspaceSettings, setWorkspaceSettings] = useState<Record<string, any>>(
      {},
    ),
    [products, setProducts] = useState<
      { url: string; analysis: string; job_id: string }[]
    >([]);
  const base = `/workspaces/${ws}`;
  useEffect(() => {
    if (window.matchMedia("(max-width: 1000px)").matches) setAiOpen(false);
    const d = new URLSearchParams(location.search).get("demo") === "true";
    setDemo(d);
    if (d) {
      setWorkspaces([{ id: "demo", name: "The growth studio", role: "owner" }]);
      setWs("demo");
      setCampaigns(demoCampaigns);
      setProspects(demoProspects);
      setAccounts([
        {
          id: "a1",
          name: "Alex Morgan",
          channel: "linkedin",
          status: "simulated",
          email: "alex@example.com",
          timezone: "Europe/Paris",
          working_days: [1, 2, 3, 4, 5],
          start_hour: 9,
          end_hour: 18,
        },
      ]);
      setLoading(false);
    } else
      api("/workspaces")
        .then((r) => {
          setWorkspaces(r);
          const params = new URLSearchParams(location.search);
          setWs(r.find((w: Row) => w.id === params.get("workspace"))?.id || r[0]?.id || "");
          if (["campaigns", "accounts"].includes(params.get("tab") || "")) setPage(params.get("tab")!);
          if (!r.length) location.href = "/onboarding";
        })
        .catch((e) => setError(e.message))
        .finally(() => setLoading(false));
  }, []);
  const refresh = async () => {
    if (!ws || demo) return;
    try {
      const [c, p, a, i, k, m] = await Promise.all(
        [
          "campaigns",
          "prospects",
          "accounts",
          "conversations",
          "keys",
          "members",
        ].map((x) =>
          api(`${base}/${x}`).catch((e) => {
            if (["members", "keys"].includes(x)) return [];
            throw e;
          }),
        ),
      );
      setCampaigns(c);
      setProspects(p);
      setAccounts(a);
      setConversations(i);
      setKeys(k);
      setMembers(m);
      setJobs(await api(`${base}/actions`));
      const settings = await api(`${base}/settings`);
      setProducts(settings.products || []);
      setWorkspaceSettings(settings);
    } catch (e) {
      setError((e as Error).message);
    }
  };
  useEffect(() => {
    void refresh();
  }, [ws, demo]);
  async function save(resource: string, data: any, id?: string) {
    setError("");
    if (!demo && resource === "campaigns" && data.status === "active")
      delete data.status;
    if (demo) {
      const setters: any = {
        campaigns: setCampaigns,
        prospects: setProspects,
        accounts: setAccounts,
        keys: setKeys,
        members: setMembers,
      };
      setters[resource]?.((rows: Row[]) =>
        id
          ? rows.map((r) => (r.id === id ? { ...r, ...data } : r))
          : [...rows, { ...data, id: crypto.randomUUID() }],
      );
      setNotice(
        "Saved in this demo session. No external action was performed.",
      );
      setModal(null);
      return;
    }
    setBusy(true);
    try {
      const r = await api(
        `${base}/${resource}${id ? "/" + id : ""}`,
        id || resource === "settings" ? "PATCH" : "POST",
        data,
      );
      if (resource === "keys") setSecret(r.token || r.key || "");
      await refresh();
      setModal(null);
      setNotice("Changes saved.");
      return r;
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const run = async (fn: () => Promise<any>) => {
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  useEffect(() => {
    if (settingsTab === "MCP & API" && !demo)
      api("/oauth/grants")
        .then(setGrants)
        .catch((e) => setError(e.message));
  }, [settingsTab, demo]);
  useEffect(() => {
    if (!modal) return;
    const previous = document.activeElement as HTMLElement;
    const dialog = document.querySelector(".modal") as HTMLElement;
    const focusable = () =>
      Array.from(
        dialog.querySelectorAll<HTMLElement>(
          "button,input,select,textarea,a[href]",
        ),
      ).filter(
        (el) => !el.hasAttribute("disabled") && el.offsetParent !== null,
      );
    focusable()[0]?.focus();
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") setModal(null);
      if (e.key === "Tab") {
        const items = focusable(),
          first = items[0],
          last = items.at(-1);
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last?.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first?.focus();
        }
      }
    };
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("keydown", key);
      previous?.focus();
    };
  }, [modal?.kind, modal?.row?.id]);
  const selectedWorkspace = workspaces.find((w) => w.id === ws);
  useEffect(() => {
    if (
      selectedWorkspace?.role !== "owner" &&
      ["billing", "members"].includes(settingsTab)
    )
      setSettingsTab("workspace");
  }, [selectedWorkspace?.role, settingsTab]);
  return (
    <div className={`app-shell ${aiOpen ? "" : "no-ai"}`}>
      <aside className="sidebar">
        <a className="brand" href="/">
          <Flower2 size={30} />
          tullips
        </a>
        <div className="workspace-picker">
          <span className="workspace-icon">G</span>
          <select
            aria-label="Current workspace"
            value={ws}
            onChange={(e) => setWs(e.target.value)}
          >
            {workspaces.map((w) => (
              <option key={w.id} value={w.id}>
                {w.name}
              </option>
            ))}
          </select>
          <button
            aria-label="Create workspace"
            onClick={() => setModal({ kind: "workspace" })}
          >
            <Plus size={15} />
          </button>
        </div>
        <span className="nav-caption">YOUR WORKSPACE</span>
        <nav>
          {nav.map(([id, label, Icon], i) => (
            <button
              key={id}
              className={`${page === id ? "selected" : ""} ${i === 4 ? "nav-separator" : ""}`}
              onClick={() => {
                setPage(id);
                setQuery("");
                setFilter("all");
              }}
            >
              <Icon size={18} />
              {label}
              {id === "inbox" && conversations.length > 0 && (
                <span className="count">{conversations.length}</span>
              )}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="grow-note">
            <Sprout size={23} />
            <b>A little care. A lot of growth.</b>
            <p>Your next great connection starts with a conversation.</p>
            <button
              onClick={() => {
                setPage("settings");
                setSettingsTab("billing");
              }}
            >
              View your plan <ArrowUpRight size={14} />
            </button>
          </div>
          <button
            className="sidebar-help"
            onClick={() => {
              setAiOpen(true);
              setAiAnswer(
                "Start with your website, shape your audience, then create a LinkedIn campaign. You can review every prospect and message before launch.",
              );
            }}
          >
            <CircleHelp size={17} /> A little help
          </button>
          <div className="profile">
            <span className="avatar">{demo ? "AM" : "YO"}</span>
            <div>
              <b>{demo ? "Alex Morgan" : "Your account"}</b>
              <small>{selectedWorkspace?.role || "Workspace member"}</small>
            </div>
            <button
              aria-label="Refresh workspace"
              onClick={() => void refresh()}
            >
              <ArrowRight size={14} />
            </button>
            <button
              aria-label="Sign out"
              onClick={async () => {
                await fetch("/api/auth/sign-out", {
                  method: "POST",
                  headers: { "Content-Type": "application/json" },
                  body: "{}",
                });
                location.href = "/login";
              }}
            >
              <LogOut size={16} />
            </button>
          </div>
        </div>
      </aside>
      <div className="main-column">
        <header className="topbar">
          <span>{nav.find((n) => n[0] === page)?.[1] || "Your product"}</span>
          <div>
            {demo && <span className="demo-label">INTERACTIVE DEMO</span>}
            <span className="workspace-status">
              <span className="dot" /> Your workspace
            </span>
            <button
              className={`ai-toggle ${aiOpen ? "active" : ""}`}
              onClick={() => setAiOpen(!aiOpen)}
            >
              <Sparkles size={15} /> AI companion
            </button>
          </div>
        </header>
        <main className="content" key={page}>
          {error && (
            <div className="error" role="alert">
              {error}
              {error.includes("sign in") && <a href="/login"> Sign in →</a>}
              <button aria-label="Dismiss error" onClick={() => setError("")}>
                <X size={14} />
              </button>
            </div>
          )}
          {notice && (
            <div className="notice" role="status">
              <Check size={15} />
              {notice}
              <button
                aria-label="Dismiss notification"
                onClick={() => setNotice("")}
              >
                <X size={14} />
              </button>
            </div>
          )}
          {loading ? (
            <div className="empty">
              <Flower2 />
              Your workspace is taking root…
            </div>
          ) : (
            <>
              {page === "overview" && (
                <>
                  <div className="page-heading">
                    <div>
                      <span className="eyebrow">
                        A LITTLE INTENTION. A LOT OF POSSIBILITY.
                      </span>
                      <h1>
                        Good things are growing<span className="pink">.</span>
                      </h1>
                      <p>Here's how your connections are coming along.</p>
                    </div>
                    <button
                      className="primary"
                      onClick={() => setModal({ kind: "campaign" })}
                    >
                      <Plus size={17} /> New campaign
                    </button>
                  </div>
                  <div className="welcome-banner">
                    <div>
                      <span className="eyebrow">
                        YOUR NEXT CHAPTER STARTS HERE
                      </span>
                      <h2>
                        Find your people.
                        <br />
                        Make it personal.
                      </h2>
                      <p>
                        Turn your website into an audience worth talking to.
                        <br />
                        Your AI companion will help you plant the right seeds.
                      </p>
                      <button onClick={() => { location.href = demo ? "/onboarding?demo=true" : `/onboarding?workspace=${ws}`; }}>
                        Find my audience <ArrowUpRight size={16} />
                      </button>
                    </div>
                    <div className="banner-flower">
                      <Flower2 />
                      <span className="orbit o1" />
                      <span className="orbit o2" />
                      <span className="tiny-flower">✳</span>
                    </div>
                  </div>
                  <div className="stats">
                    {[
                      [
                        prospects.length,
                        "People discovered",
                        "A growing circle",
                        Users,
                      ],
                      [
                        prospects.filter((p) => p.status === "connected")
                          .length,
                        "New connections",
                        "A door opened",
                        Linkedin,
                      ],
                      [
                        prospects.filter((p) => p.status === "replied").length,
                        "Conversations started",
                        "Where possibility begins",
                        Inbox,
                      ],
                      [
                        campaigns.filter((c) => c.status === "active").length,
                        "Active campaigns",
                        "Growing at your pace",
                        Sprout,
                      ],
                    ].map(([n, l, s, I]: any) => (
                      <div className="stat" key={l}>
                        <div>
                          <span>{l}</span>
                          <I size={17} />
                        </div>
                        <strong>{n}</strong>
                        <small>{s}</small>
                      </div>
                    ))}
                  </div>
                  <div className="section-heading">
                    <h2>
                      Your campaigns <span>{campaigns.length}</span>
                    </h2>
                    <button
                      className="text-button"
                      onClick={() => setPage("campaigns")}
                    >
                      View all campaigns <ArrowRight size={15} />
                    </button>
                  </div>
                  <div className="campaign-list">
                    {campaigns.length ? (
                      campaigns
                        .slice(0, 3)
                        .map((c, i) => (
                          <CampaignCard
                            key={c.id}
                            c={c}
                            index={i}
                            onClick={() =>
                              setModal({ kind: "campaign", row: c })
                            }
                          />
                        ))
                    ) : (
                      <Empty
                        title="Your first campaign is a seed of possibility."
                        text="Define your audience and start a thoughtful conversation."
                        action={() => setModal({ kind: "campaign" })}
                      />
                    )}
                  </div>
                  <div className="section-heading">
                    <h2>People to get to know</h2>
                    <button
                      className="text-button"
                      onClick={() => setPage("prospects")}
                    >
                      Open your CRM <ArrowRight size={15} />
                    </button>
                  </div>
                  <ProspectTable
                    rows={prospects.slice(0, 4)}
                    onSelect={(p) => setModal({ kind: "prospect", row: p })}
                  />
                  <div className="footer-note">
                    <Flower2 size={15} /> Meaningful growth starts with
                    meaningful connections.
                  </div>
                </>
              )}
              {page === "campaigns" && (
                <>
                  {jobs.some((j) => j.status === "awaiting_approval") && (
                    <div className="panel">
                      <h2>Ready for your review</h2>
                      <p>
                        Approve each prepared action before it enters the
                        sending queue.
                      </p>
                      {jobs
                        .filter((j) => j.status === "awaiting_approval")
                        .map((j) => (
                          <div className="member-row" key={j.id}>
                            <div>
                              <b>{j.payload?.step?.type || j.kind}</b>
                              <p>
                                {j.payload?.step?.message ||
                                  "Connection request"}
                              </p>
                            </div>
                            <button
                              onClick={() =>
                                run(async () => {
                                  await api(
                                    `${base}/actions/${j.id}/approve`,
                                    "POST",
                                    {},
                                  );
                                  await refresh();
                                  setNotice("Action approved.");
                                })
                              }
                            >
                              Approve
                            </button>
                          </div>
                        ))}
                    </div>
                  )}
                  <Heading
                    title="Plant a conversation."
                    sub="Thoughtful outreach, one campaign at a time."
                    action="New campaign"
                    onClick={() => setModal({ kind: "campaign" })}
                  />
                  <div className="toolbar">
                    <Search size={17} />
                    <input
                      aria-label="Search campaigns"
                      placeholder="Search your campaigns…"
                      value={query}
                      onChange={(e) => setQuery(e.target.value)}
                    />
                    <select
                      aria-label="Campaign status"
                      value={filter}
                      onChange={(e) => setFilter(e.target.value)}
                    >
                      <option value="all">All campaigns</option>
                      <option>active</option>
                      <option>draft</option>
                      <option>paused</option>
                    </select>
                  </div>
                  <div className="campaign-list">
                    {campaigns
                      .filter(
                        (c) =>
                          (filter === "all" || c.status === filter) &&
                          c.name.toLowerCase().includes(query.toLowerCase()),
                      )
                      .map((c, i) => (
                        <CampaignCard
                          key={c.id}
                          c={c}
                          index={i}
                          onClick={() => setModal({ kind: "campaign", row: c })}
                        />
                      ))}
                  </div>
                  {!campaigns.length && (
                    <Empty
                      title="A fresh patch of possibility."
                      text="Build your first LinkedIn outreach sequence."
                      action={() => setModal({ kind: "campaign" })}
                    />
                  )}
                </>
              )}
              {page === "prospects" && (
                <>
                  <Heading
                    title="Your people, all together."
                    sub="Every connection has a story. Keep yours growing."
                    action="Add prospect"
                    onClick={() => setModal({ kind: "prospect" })}
                  />
                  <div className="toolbar">
                    <Search size={17} />
                    <input
                      aria-label="Search prospects"
                      placeholder="Search people, companies, roles…"
                      value={query}
                      onChange={(e) => setQuery(e.target.value)}
                    />
                    <select
                      aria-label="Prospect status"
                      value={filter}
                      onChange={(e) => setFilter(e.target.value)}
                    >
                      <option value="all">All statuses</option>
                      {[
                        "new",
                        "invited",
                        "connected",
                        "replied",
                        "qualified",
                        "won",
                        "lost",
                        "unsubscribed",
                      ].map((s) => (
                        <option key={s}>{s}</option>
                      ))}
                    </select>
                    <button
                      onClick={() =>
                        run(async () => {
                          if (demo)
                            throw new Error(
                              "CSV export is available in a signed-in workspace.",
                            );
                          const r = await fetch(
                            `/api/v1${base}/prospects/export`,
                          );
                          if (!r.ok) throw new Error("Export failed");
                          const u = URL.createObjectURL(await r.blob());
                          const a = document.createElement("a");
                          a.href = u;
                          a.download = "tullips-prospects.csv";
                          a.click();
                          URL.revokeObjectURL(u);
                        })
                      }
                    >
                      <Download size={15} />
                      Export
                    </button>
                    <label className="button">
                      <Upload size={15} />
                      Import CSV
                      <input
                        className="visually-hidden"
                        type="file"
                        accept=".csv"
                        onChange={(e) => {
                          const f = e.target.files?.[0];
                          if (f)
                            void run(async () => {
                              if (demo)
                                throw new Error(
                                  "Import is available in a signed-in workspace.",
                                );
                              const r = await fetch(
                                `/api/v1${base}/prospects/import`,
                                {
                                  method: "POST",
                                  headers: { "Content-Type": "text/csv" },
                                  body: await f.text(),
                                },
                              );
                              if (!r.ok)
                                throw new Error(
                                  "Import failed. Check the CSV columns.",
                                );
                              await refresh();
                              setNotice("Prospects imported.");
                            });
                        }}
                      />
                    </label>
                  </div>
                  <ProspectTable
                    rows={prospects.filter(
                      (p) =>
                        (filter === "all" || p.status === filter) &&
                        `${p.first_name} ${p.last_name} ${p.company} ${p.title}`
                          .toLowerCase()
                          .includes(query.toLowerCase()),
                    )}
                    onSelect={(p) => setModal({ kind: "prospect", row: p })}
                  />
                </>
              )}
              {page === "overview" && products.length > 0 && (
                <section className="panel saved-analyses">
                  <h2>Your product library</h2>
                  <p>Past analyses, ready to inform your next campaign.</p>
                  {products.map((p) => (
                    <details key={p.job_id}>
                      <summary>{p.url}</summary>
                      <p className="analysis-text">{p.analysis}</p>
                      <button
                        onClick={() => {
                          setAiAnswer(p.analysis);
                          setAiOpen(true);
                        }}
                      >
                        Discuss with your companion
                      </button>
                    </details>
                  ))}
                </section>
              )}
              {page === "inbox" && (
                <>
                  <Heading
                    title="Keep the conversation going."
                    sub="LinkedIn first. Email alongside. Every reply stops the sequence."
                  />
                  {conversations.length ? (
                    <div className="inbox-list">
                      {conversations.map((c) => (
                        <div key={c.id} className="panel">
                          <Badge>{c.channel}</Badge>
                          <h3>
                            {prospects.find((p) => p.id === c.prospect_id)
                              ?.first_name || "Conversation"}
                          </h3>
                          {(c.messages || []).map((m: any, i: number) => (
                            <p key={i} className="message">
                              {m.body || m.text}
                            </p>
                          ))}
                          <form
                            onSubmit={(e) => {
                              e.preventDefault();
                              const f = new FormData(e.currentTarget);
                              void run(async () => {
                                await api(
                                  `${base}/conversations/${c.id}/reply`,
                                  "POST",
                                  { text: f.get("body") },
                                );
                                await refresh();
                                setNotice(
                                  "Reply queued. Delivery follows the account connection and sending settings.",
                                );
                              });
                            }}
                          >
                            <label>
                              Your reply
                              <textarea
                                name="body"
                                value={c.draft || ""}
                                onChange={(e) =>
                                  setConversations((rows) =>
                                    rows.map((x) =>
                                      x.id === c.id
                                        ? { ...x, draft: e.target.value }
                                        : x,
                                    ),
                                  )
                                }
                                required
                              />
                            </label>
                            <div className="actions">
                              <button
                                type="button"
                                onClick={() =>
                                  run(async () => {
                                    const r = await aiAction(base, "ai.draft", {
                                      prompt: JSON.stringify(c.messages),
                                    });
                                    setConversations((rows) =>
                                      rows.map((x) =>
                                        x.id === c.id
                                          ? { ...x, draft: r.text }
                                          : x,
                                      ),
                                    );
                                  })
                                }
                              >
                                <Sparkles size={15} />
                                Suggest a reply
                              </button>
                              <button className="primary">
                                Send reply <Send size={15} />
                              </button>
                            </div>
                          </form>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <Empty
                      title="Good conversations have a home."
                      text="Replies from your connected LinkedIn and email accounts will appear here. AI suggests; you send."
                      icon={<Inbox size={30} />}
                    />
                  )}
                </>
              )}
              {page === "accounts" && (
                <>
                  <Heading
                    title="The people behind your outreach."
                    sub="Separate sessions, shared purpose. Each sender keeps their own limits."
                    action="Connect account"
                    onClick={() => setModal({ kind: "account" })}
                  />
                  <div className="account-grid">
                    {accounts.map((a) => (
                      <button
                        key={a.id}
                        className="panel account-card"
                        onClick={() => setModal({ kind: "account", row: a })}
                      >
                        <span className="icon-box">
                          {a.channel === "linkedin" ? <Linkedin /> : <Mail />}
                        </span>
                        <Badge>{a.status || "disconnected"}</Badge>
                        <h3>{a.name}</h3>
                        <p>{a.email}</p>
                        <hr />
                        <span>
                          <Clock3 size={15} />
                          Mon–Fri · {a.start_hour || 9}:00–{a.end_hour || 18}:00
                        </span>
                        <small>{a.timezone || "Europe/Paris"}</small>
                        <div className="quota-track">
                          <i style={{ width: "40%" }} />
                        </div>
                        <small>
                          {a.daily_limit
                            ? `${a.daily_limit} actions/day · custom cap`
                            : (workspaceSettings.ramp_limits || [40, 60, 100])
                                .map(
                                  (n: number, i: number) =>
                                    `${n} from day ${(workspaceSettings.ramp_days || [0, 14, 60])[i]}`,
                                )
                                .join(" → ")}
                        </small>
                      </button>
                    ))}
                  </div>
                  <div className="note">
                    <Sprout size={18} />
                    Invitations and messages share a daily budget. Search and
                    profile views have separate limits.
                  </div>
                </>
              )}
              {page === "settings" && (
                <>
                  <Heading
                    title="Make yourself at home."
                    sub="Your workspace, your way of growing."
                  />
                  <div className="tabs">
                    {[
                      "workspace",
                      ...(selectedWorkspace?.role === "owner"
                        ? ["members", "billing"]
                        : []),
                      "MCP & API",
                    ].map((t) => (
                      <button
                        key={t}
                        className={settingsTab === t ? "active" : ""}
                        onClick={() => setSettingsTab(t)}
                      >
                        {t}
                      </button>
                    ))}
                    {installationAdmin && <button className={settingsTab === "installation" ? "active" : ""} onClick={() => setSettingsTab("installation")}>Installation updates</button>}
                  </div>
                  {settingsTab === "installation" && installationAdmin && <UpdatePanel />}
                  {settingsTab === "workspace" && (
                    <div className="panel form-panel">
                      <h2>Workspace details</h2>
                      <form
                        onSubmit={(e) => {
                          e.preventDefault();
                          const f = new FormData(e.currentTarget);
                          void save(
                            "settings",
                            {
                              name: f.get("name"),
                              timezone: f.get("timezone"),
                              language: f.get("language"),
                              ramp_days: f.getAll("ramp_days").map(Number),
                              ramp_limits: f.getAll("ramp_limits").map(Number),
                            },
                            undefined,
                          );
                        }}
                      >
                        <label>
                          Workspace name
                          <input
                            name="name"
                            defaultValue={selectedWorkspace?.name}
                            required
                          />
                        </label>
                        <label>
                          Default timezone
                          <input
                            name="timezone"
                            defaultValue={
                              workspaceSettings.timezone || "Europe/Paris"
                            }
                          />
                        </label>
                        <label>
                          Default message language
                          <input
                            name="language"
                            defaultValue={
                              workspaceSettings.language || "English"
                            }
                          />
                        </label>
                        <fieldset>
                          <legend>Default LinkedIn sending ramp</legend>
                          <p className="small">
                            Days since account activation. A sender's explicit
                            daily cap overrides this schedule.
                          </p>
                          {(workspaceSettings.ramp_days || [0, 14, 60]).map(
                            (_: number, i: number) => (
                              <div className="form-grid" key={i}>
                                <label>
                                  Stage {i + 1}: activation day
                                  <input
                                    name="ramp_days"
                                    type="number"
                                    min={i === 0 ? 0 : 1}
                                    max="3650"
                                    readOnly={i === 0}
                                    defaultValue={
                                      workspaceSettings.ramp_days?.[i] ??
                                      [0, 14, 60][i]
                                    }
                                    required
                                  />
                                </label>
                                <label>
                                  Daily invitations + messages
                                  <input
                                    name="ramp_limits"
                                    type="number"
                                    min="1"
                                    max="10000"
                                    defaultValue={
                                      workspaceSettings.ramp_limits?.[i] ??
                                      [40, 60, 100][i]
                                    }
                                    required
                                  />
                                </label>
                              </div>
                            ),
                          )}
                        </fieldset>
                        <button className="primary">Save preferences</button>
                      </form>
                    </div>
                  )}
                  {settingsTab === "members" && (
                    <div className="panel">
                      <div className="section-heading">
                        <h2>A shared place to grow.</h2>
                        <button onClick={() => setModal({ kind: "member" })}>
                          <Plus size={15} />
                          Add member
                        </button>
                      </div>
                      <p>
                        Owners manage billing and membership. Members manage
                        campaigns, prospects, and sender accounts.
                      </p>
                      {members.map((m) => (
                        <div className="member-row" key={m.id}>
                          <span>{m.email || m.user_id}</span>
                          <Badge>{m.role}</Badge>
                          {m.role !== "owner" && (
                            <button
                              onClick={() =>
                                run(async () => {
                                  await api(
                                    `${base}/members/${m.id}`,
                                    "DELETE",
                                  );
                                  await refresh();
                                })
                              }
                            >
                              Remove
                            </button>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                  {settingsTab === "billing" && (
                    <BillingPanel base={base} demo={demo} />
                  )}
                  {settingsTab === "MCP & API" && (
                    <div className="panel">
                      <div className="section-heading">
                        <div>
                          <h2>Your agent, meet your workspace.</h2>
                          <p>
                            Connect an AI agent using a scoped API key or OAuth.
                          </p>
                        </div>
                        <button onClick={() => setModal({ kind: "key" })}>
                          <Plus size={15} />
                          Create API key
                        </button>
                      </div>
                      {secret && (
                        <div className="notice">
                          <div>
                            Copy this key now. It will not be shown again.
                            <code>{secret}</code>
                          </div>
                          <button
                            onClick={() =>
                              navigator.clipboard.writeText(secret)
                            }
                          >
                            Copy
                          </button>
                        </div>
                      )}
                      <label>
                        MCP server URL
                        <input
                          readOnly
                          value={
                            typeof window === "undefined"
                              ? ""
                              : `${window.location.origin}/mcp`
                          }
                        />
                      </label>
                      <p className="small">
                        Use this URL in your agent's remote MCP settings. OAuth
                        permissions are requested during connection.
                      </p>
                      <h3>Authorized OAuth agents</h3>
                      {grants
                        .filter((g) => g.workspace_id === ws)
                        .map((g) => (
                          <div className="member-row" key={g.client_id}>
                            <b>{g.name}</b>
                            <span>{(g.scopes || []).join(", ")}</span>
                            <button
                              onClick={() =>
                                run(async () => {
                                  await api(
                                    `/oauth/grants/${g.client_id}`,
                                    "DELETE",
                                  );
                                  setGrants((x) =>
                                    x.filter(
                                      (v) => v.client_id !== g.client_id,
                                    ),
                                  );
                                })
                              }
                            >
                              Revoke
                            </button>
                          </div>
                        ))}
                      <h3>API keys</h3>
                      {keys.map((k) => (
                        <div className="member-row" key={k.id}>
                          <KeyRound size={18} />
                          <b>{k.name}</b>
                          <span>{(k.scopes || []).join(", ")}</span>
                          <button
                            onClick={() =>
                              run(async () => {
                                if (demo) {
                                  setKeys((rows) =>
                                    rows.filter((x) => x.id !== k.id),
                                  );
                                  return;
                                }
                                await api(`${base}/keys/${k.id}`, "DELETE");
                                await refresh();
                              })
                            }
                          >
                            Revoke
                          </button>
                        </div>
                      ))}
                    </div>
                  )}
                </>
              )}
            </>
          )}
        </main>
      </div>
      {aiOpen && (
        <aside className="ai-panel">
          <div className="ai-header">
            <span className="ai-avatar">
              <Flower2 size={20} />
            </span>
            <div>
              <b>Your growth companion</b>
              <small>A little clarity. A little momentum.</small>
            </div>
            <button
              aria-label="Close AI companion"
              onClick={() => setAiOpen(false)}
            >
              <PanelRightClose size={16} />
            </button>
          </div>
          <div className="ai-body">
            <div className="ai-hello">
              <span className="eyebrow">LET'S GROW SOMETHING GOOD</span>
              <h2>
                A fresh perspective,
                <br />
                whenever you need it.
              </h2>
              <p>
                I can help you find the right people, shape your message, and
                make your next move feel a little easier.
              </p>
            </div>
            <div className="suggestions">
              {[
                "Help me define my ideal customer",
                "Make my first message more personal",
                "Review my campaign before launch",
              ].map((s, i) => (
                <button key={s} onClick={() => setAiText(s)}>
                  {i === 0 ? (
                    <Users size={16} />
                  ) : i === 1 ? (
                    <Mail size={16} />
                  ) : (
                    <Sprout size={16} />
                  )}
                  <span>{s}</span>
                  <ArrowUpRight size={14} />
                </button>
              ))}
            </div>
            {aiAnswer && (
              <div className="ai-answer">
                <span className="ai-avatar">
                  <Flower2 size={17} />
                </span>
                <p>{aiAnswer}</p>
              </div>
            )}
            {proposal && (
              <div className="proposal">
                <b>Suggested changes</b>
                <pre>{JSON.stringify(proposal, null, 2)}</pre>
                <button
                  className="primary"
                  onClick={() =>
                    run(async () => {
                      if (
                        !["campaigns", "prospects"].includes(
                          proposal.resource,
                        ) ||
                        !proposal.id ||
                        !proposal.data
                      )
                        throw new Error(
                          "Please apply this suggestion in the campaign or prospect editor.",
                        );
                      await api(
                        `${base}/${proposal.resource}/${proposal.id}`,
                        "PATCH",
                        proposal.data,
                      );
                      setProposal(null);
                      await refresh();
                      setNotice("Reviewed changes applied.");
                    })
                  }
                >
                  Apply reviewed changes
                </button>
                <button onClick={() => setProposal(null)}>Discard</button>
              </div>
            )}
          </div>
          <div className="ai-compose">
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void run(async () => {
                  if (demo) {
                    setAiAnswer(
                      "This is an interactive preview. In your connected workspace, I can analyze your product and propose campaign changes for you to review. No AI request was made.",
                    );
                    setAiText("");
                    return;
                  }
                  const r = await aiAction(base, "ai.chat", {
                    prompt: JSON.stringify({
                      request: aiText,
                      campaigns,
                      prospects: prospects.slice(0, 10),
                      instruction:
                        "Offer helpful advice. If proposing edits, return JSON {message,proposal:{resource:campaigns,id,data}}. Never claim to have changed data.",
                    }),
                  });
                  let parsed: any;
                  try {
                    parsed = JSON.parse(r.text);
                  } catch {}
                  setAiAnswer(parsed?.message || r.text || JSON.stringify(r));
                  setProposal(parsed?.proposal || null);
                  setAiText("");
                });
              }}
            >
              <textarea
                aria-label="Message your AI companion"
                placeholder="A thought, a question, an idea…"
                value={aiText}
                onChange={(e) => setAiText(e.target.value)}
                required
              />
              <div>
                <span>
                  <Sparkles size={12} /> Made for your next move
                </span>
                <button
                  className="primary"
                  aria-label="Send to AI companion"
                  disabled={busy || !aiText.trim()}
                >
                  <ArrowUpRight size={17} />
                </button>
              </div>
            </form>
            <small>You choose what changes. Always.</small>
          </div>
        </aside>
      )}
      {modal && (
        <div
          className="modal-backdrop"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) setModal(null);
          }}
        >
          <section
            className={`modal ${modal.kind === "campaign" ? "wide" : ""}`}
            role="dialog"
            aria-modal="true"
            aria-label={`Edit ${modal.kind}`}
          >
            <div className="modal-heading">
              <h2>
                {modal.row ? "Edit" : "New"}{" "}
                {modal.kind === "key" ? "API key" : modal.kind}
              </h2>
              <button aria-label="Close dialog" onClick={() => setModal(null)}>
                <X size={20} />
              </button>
            </div>
            {error && (
              <div className="error" role="alert">
                {error}
              </div>
            )}
            {notice && (
              <div className="notice" role="status">
                {notice}
              </div>
            )}
            {modal.kind === "campaign" && modal.row && (
              <div className="actions modal-actions">
                <button
                  type="button"
                  className="primary"
                  onClick={() =>
                    run(async () => {
                      if (demo) {
                        await save(
                          "campaigns",
                          { status: "active" },
                          modal.row!.id,
                        );
                        return;
                      }
                      await api(
                        `${base}/campaigns/${modal.row!.id}/launch`,
                        "POST",
                        {},
                      );
                      await refresh();
                      setModal(null);
                      setNotice(
                        "Campaign launched. Sender quotas and review settings apply.",
                      );
                    })
                  }
                >
                  <Play size={14} />
                  Launch campaign
                </button>
                <button
                  type="button"
                  onClick={() =>
                    save("campaigns", { status: "paused" }, modal.row!.id)
                  }
                >
                  <Pause size={14} />
                  Pause
                </button>
                <button
                  type="button"
                  onClick={() =>
                    run(async () => {
                      if (demo)
                        throw new Error(
                          "Discovery requires a connected LinkedIn account.",
                        );
                      const c = modal.row!;
                      if (!c.account_ids?.length)
                        throw new Error(
                          "Select a sender account in launch settings first.",
                        );
                      await api(`${base}/actions`, "POST", {
                        kind: "linkedin.search",
                        payload: {
                          account_id: c.account_ids[0],
                          campaign_id: c.id,
                          keywords: [
                            c.criteria?.roles,
                            c.criteria?.geography,
                            c.criteria?.industry,
                          ]
                            .filter(Boolean)
                            .join(" "),
                          start: 0,
                        },
                      });
                      setNotice(
                        "Prospect discovery queued. Refresh your workspace to see results.",
                      );
                      setModal(null);
                    })
                  }
                >
                  <Search size={14} />
                  Find prospects
                </button>
              </div>
            )}
            {modal.kind === "campaign" &&
              modal.row?.autonomy === "review" &&
              modal.row.status === "active" &&
              !modal.row.review_approved && (
                <section className="panel modal-actions">
                  {sampleReviewId !== modal.row.id ? (
                    <button
                      type="button"
                      onClick={() => setSampleReviewId(modal.row!.id)}
                    >
                      <Check size={15} />
                      Review sample and enable automation
                    </button>
                  ) : (
                    <>
                      <h3>Approve this sample and automate the campaign?</h3>
                      <p>
                        Review the saved sequence below. Approval allows current
                        and future prospects in this campaign to proceed without
                        individual message approval. Sender quotas, active
                        hours, and stop-on-reply still apply.
                      </p>
                      {(modal.row.workflow || [])
                        .filter((step: any) =>
                          ["invitation", "linkedin_message", "email"].includes(
                            step.type,
                          ),
                        )
                        .map((step: any, index: number) => (
                          <div className="message" key={index}>
                            <b>{step.type.replaceAll("_", " ")}</b>
                            <p>
                              {step.message ||
                                "Connection invitation without a note"}
                            </p>
                          </div>
                        ))}
                      <div className="actions">
                        <button
                          type="button"
                          className="primary"
                          disabled={busy}
                          onClick={() =>
                            run(async () => {
                              const id = modal.row!.id;
                              if (demo) {
                                setCampaigns((rows) =>
                                  rows.map((c) =>
                                    c.id === id
                                      ? { ...c, review_approved: true }
                                      : c,
                                  ),
                                );
                                setNotice(
                                  "Sample approved in this demo only. No messages will be sent.",
                                );
                              } else {
                                await api(
                                  `${base}/campaigns/${id}/approve_sample`,
                                  "POST",
                                  {},
                                );
                                await refresh();
                                setNotice(
                                  "Sample approved. This campaign now follows the saved sequence automatically.",
                                );
                              }
                              setSampleReviewId(null);
                              setModal(null);
                            })
                          }
                        >
                          Approve sample and automate
                        </button>
                        <button
                          type="button"
                          disabled={busy}
                          onClick={() => setSampleReviewId(null)}
                        >
                          Keep reviewing individually
                        </button>
                      </div>
                    </>
                  )}
                </section>
              )}
            {modal.kind === "account" && modal.row && (
              <div className="panel modal-actions">
                <div className="actions">
                  <button
                    type="button"
                    onClick={() =>
                      run(async () => {
                        if (demo)
                          throw new Error(
                            "Provider connections are unavailable in the demo.",
                          );
                        const r = await api(
                          `${base}/accounts/${modal.row!.id}/connect`,
                          "POST",
                          {},
                        );
                        await refresh();
                        setNotice(
                          r.status === "verification_required"
                            ? "Enter the verification code requested by LinkedIn."
                            : "Provider connection verified.",
                        );
                        setModal((m) =>
                          m
                            ? { ...m, row: { ...m.row!, status: r.status } }
                            : m,
                        );
                      })
                    }
                  >
                    Connect / reconnect
                  </button>
                  <button
                    type="button"
                    onClick={() =>
                      run(async () => {
                        if (demo)
                          throw new Error(
                            "Provider sync is unavailable in the demo.",
                          );
                        await api(
                          `${base}/accounts/${modal.row!.id}/sync`,
                          "POST",
                          {},
                        );
                        await refresh();
                        setNotice("Mailbox synchronized.");
                      })
                    }
                  >
                    Sync replies
                  </button>
                </div>
                {modal.row.status === "verification_required" && (
                  <>
                    <label>
                      Verification code
                      <input
                        value={verificationCode}
                        onChange={(e) => setVerificationCode(e.target.value)}
                        autoComplete="one-time-code"
                      />
                    </label>
                    <button
                      type="button"
                      onClick={() =>
                        run(async () => {
                          const r = await api(
                            `${base}/accounts/${modal.row!.id}/verify`,
                            "POST",
                            { code: verificationCode },
                          );
                          setNotice(`Connection status: ${r.status}`);
                          await refresh();
                          setModal(null);
                          setVerificationCode("");
                        })
                      }
                    >
                      Verify
                    </button>
                  </>
                )}
              </div>
            )}
            {["campaign", "sequence"].includes(modal.kind) ? (
              <CampaignForm
                row={modal.row}
                accounts={accounts}
                save={(d) =>
                  modal.kind === "sequence"
                    ? save("prospects", { workflow: d.workflow }, modal.row?.id)
                    : save("campaigns", d, modal.row?.id)
                }
              />
            ) : (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  const f = new FormData(e.currentTarget);
                  let d: any = Object.fromEntries(f);
                  if (modal.kind === "workspace") {
                    void run(async () => {
                      if (demo) {
                        const w = { id: crypto.randomUUID(), name: d.name };
                        setWorkspaces((x) => [...x, w]);
                        setWs(w.id);
                        setModal(null);
                        return;
                      }
                      const w = await api("/workspaces", "POST", d);
                      setWorkspaces((x) => [...x, w]);
                      setWs(w.id);
                      setModal(null);
                    });
                    return;
                  }
                  if (modal.kind === "prospect") {
                    d.tags = String(d.tags || "")
                      .split(",")
                      .map((s: string) => s.trim())
                      .filter(Boolean);
                    if (!d.campaign_id) delete d.campaign_id;
                  }
                  if (modal.kind === "account") {
                    d.start_hour = Number(d.start_hour);
                    d.end_hour = Number(d.end_hour);
                    for (const key of [
                      "daily_limit",
                      "search_daily_limit",
                      "profile_daily_limit",
                      "smtp_port",
                      "imap_port",
                    ]) {
                      if (d[key] !== "") d[key] = Number(d[key]);
                      else if (key === "daily_limit") d[key] = null;
                      else delete d[key];
                    }
                    d.working_days = f.getAll("working_days").map(Number);
                    if (!d.password) delete d.password;
                  }
                  if (modal.kind === "key") d.scopes = f.getAll("scopes");
                  void save(
                    (
                      {
                        prospect: "prospects",
                        account: "accounts",
                        key: "keys",
                        member: "members",
                      } as any
                    )[modal.kind],
                    d,
                    modal.row?.id,
                  );
                }}
              >
                {modal.kind === "workspace" && (
                  <label>
                    Workspace name
                    <input
                      name="name"
                      required
                      placeholder="The growth studio"
                      autoFocus
                    />
                  </label>
                )}
                {modal.kind === "prospect" && (
                  <>
                    {modal.row && (
                      <section className="panel qualification">
                        <div className="section-heading">
                          <h3>Audience match</h3>
                          <Badge>
                            {modal.row.status === "qualified" ||
                            modal.row.qualification === "manual"
                              ? "Approved"
                              : modal.row.qualification || "Not assessed"}
                          </Badge>
                        </div>
                        {typeof modal.row.ai_score === "number" && (
                          <p>AI fit score: {modal.row.ai_score}/100</p>
                        )}
                        <p>
                          {modal.row.qualification_reason ||
                            "No qualification assessment is available for this prospect."}
                        </p>
                        <p className="small">
                          Matching uses available profile evidence. Missing
                          company size, location, or other details stay unknown.
                          Uncertain LinkedIn discoveries require your review
                          before automatic enrollment.
                        </p>
                        {modal.row.qualification &&
                          modal.row.status !== "qualified" &&
                          modal.row.qualification !== "manual" && (
                            <button
                              type="button"
                              disabled={busy}
                              onClick={() =>
                                save(
                                  "prospects",
                                  { status: "qualified" },
                                  modal.row!.id,
                                )
                              }
                            >
                              <Check size={15} />
                              Approve qualification
                            </button>
                          )}
                      </section>
                    )}
                    <div className="form-grid">
                      <Field
                        name="first_name"
                        label="First name"
                        row={modal.row}
                        required
                      />
                      <Field
                        name="last_name"
                        label="Last name"
                        row={modal.row}
                      />
                      <Field name="title" label="Job title" row={modal.row} />
                      <Field name="company" label="Company" row={modal.row} />
                    </div>
                    <Field
                      name="linkedin_url"
                      label="LinkedIn profile URL"
                      row={modal.row}
                      type="url"
                    />
                    <Field
                      name="email"
                      label="Email (if available)"
                      row={modal.row}
                      type="email"
                    />
                    <label>
                      Status
                      <select
                        name="status"
                        defaultValue={modal.row?.status || "new"}
                      >
                        {[
                          "new",
                          "invited",
                          "connected",
                          "replied",
                          "qualified",
                          "won",
                          "lost",
                          "unsubscribed",
                        ].map((s) => (
                          <option key={s}>{s}</option>
                        ))}
                      </select>
                    </label>
                    <label>
                      Campaign
                      <select
                        name="campaign_id"
                        defaultValue={modal.row?.campaign_id || ""}
                      >
                        <option value="">No campaign</option>
                        {campaigns.map((c) => (
                          <option value={c.id} key={c.id}>
                            {c.name}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label>
                      Tags
                      <input
                        name="tags"
                        defaultValue={(modal.row?.tags || []).join(", ")}
                        placeholder="Decision maker, High intent"
                      />
                    </label>
                    <label>
                      Notes
                      <textarea
                        name="notes"
                        defaultValue={modal.row?.notes || ""}
                      />
                    </label>
                    {modal.row && (
                      <button
                        type="button"
                        onClick={() =>
                          setModal({
                            kind: "sequence",
                            row: {
                              ...campaigns.find(
                                (c) => c.id === modal.row?.campaign_id,
                              ),
                              ...modal.row!,
                              name: "Individual sequence",
                              description: "Individual prospect workflow",
                            },
                          })
                        }
                      >
                        Customize sequence
                      </button>
                    )}
                  </>
                )}
                {modal.kind === "account" && (
                  <>
                    <Field
                      name="name"
                      label="Sender name"
                      row={modal.row}
                      required
                    />
                    <label>
                      Channel
                      <select
                        name="channel"
                        defaultValue={modal.row?.channel || "linkedin"}
                      >
                        <option value="linkedin">LinkedIn</option>
                        <option value="email">Email — SMTP / IMAP</option>
                      </select>
                    </label>
                    <Field
                      name="email"
                      label="Email address"
                      row={modal.row}
                      type="email"
                      required
                    />
                    <label>
                      Password
                      <input
                        type="password"
                        name="password"
                        autoComplete="new-password"
                        placeholder={
                          modal.row
                            ? "Leave blank to keep existing"
                            : "Stored encrypted for reconnection"
                        }
                      />
                    </label>
                    <p className="small">
                      Connection may require a verification code. Creating an
                      account does not confirm a successful provider connection.
                    </p>
                    <Field
                      name="timezone"
                      label="Timezone"
                      row={modal.row || { id: "", timezone: "Europe/Paris" }}
                      required
                    />
                    <div className="form-grid">
                      <Field
                        name="start_hour"
                        label="Start hour"
                        type="number"
                        row={modal.row || { id: "", start_hour: 9 }}
                      />
                      <Field
                        name="end_hour"
                        label="End hour"
                        type="number"
                        row={modal.row || { id: "", end_hour: 18 }}
                      />
                    </div>
                    <fieldset>
                      <legend>Active days</legend>
                      <div className="checks">
                        {["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].map(
                          (d, i) => (
                            <label key={d}>
                              <input
                                type="checkbox"
                                name="working_days"
                                value={i}
                                defaultChecked={(
                                  modal.row?.working_days || [1, 2, 3, 4, 5]
                                ).includes(i)}
                              />
                              {d}
                            </label>
                          ),
                        )}
                      </div>
                    </fieldset>
                    <div className="form-grid">
                      <Field
                        name="daily_limit"
                        label="Daily send cap (optional)"
                        type="number"
                        row={modal.row}
                      />
                      <Field
                        name="search_daily_limit"
                        label="Daily searches"
                        type="number"
                        row={modal.row || { id: "", search_daily_limit: 20 }}
                      />
                      <Field
                        name="profile_daily_limit"
                        label="Daily profile views"
                        type="number"
                        row={modal.row || { id: "", profile_daily_limit: 100 }}
                      />
                    </div>
                    <p className="small">
                      Leave the send cap empty to use the automatic 40 → 60 →
                      100 default ramp (configurable in workspace settings).
                      Setting a cap overrides the automatic daily limit.
                    </p>
                    <details>
                      <summary>Email server settings</summary>
                      <Field
                        name="smtp_host"
                        label="SMTP host"
                        row={modal.row}
                      />
                      <Field
                        name="smtp_port"
                        label="SMTP port"
                        type="number"
                        row={modal.row || { id: "", smtp_port: 465 }}
                      />
                      <Field
                        name="imap_host"
                        label="IMAP host"
                        row={modal.row}
                      />
                      <Field
                        name="imap_port"
                        label="IMAP port"
                        type="number"
                        row={modal.row || { id: "", imap_port: 993 }}
                      />
                    </details>
                    <div className="actions">
                      <button
                        type="button"
                        onClick={() =>
                          run(async () => {
                            if (!modal.row?.id || demo)
                              throw new Error(
                                "Save this email account first, then connect Google.",
                              );
                            const r = await api(
                              `${base}/accounts/${modal.row.id}/oauth`,
                              "POST",
                              { provider: "google" },
                            );
                            location.href = r.url;
                          })
                        }
                      >
                        Google OAuth
                      </button>
                      <button
                        type="button"
                        onClick={() =>
                          run(async () => {
                            if (!modal.row?.id || demo)
                              throw new Error(
                                "Save this email account first, then connect Microsoft.",
                              );
                            const r = await api(
                              `${base}/accounts/${modal.row.id}/oauth`,
                              "POST",
                              { provider: "microsoft" },
                            );
                            location.href = r.url;
                          })
                        }
                      >
                        Microsoft OAuth
                      </button>
                    </div>
                  </>
                )}
                {modal.kind === "key" && (
                  <>
                    <label>
                      Key name
                      <input
                        name="name"
                        required
                        placeholder="My AI assistant"
                      />
                    </label>
                    <fieldset>
                      <legend>Permissions</legend>
                      {[
                        ["read", "Read prospects and campaigns"],
                        ["write", "Modify workspace data"],
                        ["launch", "Launch campaigns"],
                        ["send", "Send messages"],
                      ].map(([v, l]) => (
                        <label className="check" key={v}>
                          <input
                            type="checkbox"
                            name="scopes"
                            value={v}
                            defaultChecked={v === "read"}
                          />
                          {l}
                        </label>
                      ))}
                    </fieldset>
                    <p className="small">
                      Limited to this workspace and your member permissions.
                      Revoke access at any time.
                    </p>
                  </>
                )}
                {modal.kind === "member" && (
                  <>
                    <label>
                      Existing user's email
                      <input name="email" type="email" required />
                    </label>
                    <input name="role" type="hidden" value="member" />
                    <p className="small">
                      The person must first create a Tullips account.
                    </p>
                  </>
                )}
                <button className="primary" disabled={busy}>
                  Save {modal.kind}
                  <Check size={15} />
                </button>
              </form>
            )}
          </section>
        </div>
      )}
    </div>
  );
}
function Field({
  name,
  label,
  row,
  type = "text",
  required = false,
}: {
  name: string;
  label: string;
  row?: Row;
  type?: string;
  required?: boolean;
}) {
  return (
    <label>
      {label}
      <input
        name={name}
        type={type}
        defaultValue={row?.[name] || ""}
        required={required}
      />
    </label>
  );
}
function Heading({
  title,
  sub,
  action,
  onClick,
}: {
  title: string;
  sub: string;
  action?: string;
  onClick?: () => void;
}) {
  return (
    <div className="page-heading">
      <div>
        <span className="eyebrow">GROW WITH INTENTION</span>
        <h1>{title}</h1>
        <p>{sub}</p>
      </div>
      {action && (
        <button className="primary" onClick={onClick}>
          <Plus size={16} />
          {action}
        </button>
      )}
    </div>
  );
}
function Empty({
  title,
  text,
  action,
  icon,
}: {
  title: string;
  text: string;
  action?: () => void;
  icon?: ReactNode;
}) {
  return (
    <div className="empty">
      {icon || <Sprout size={30} />}
      <h3>{title}</h3>
      <p>{text}</p>
      {action && (
        <button onClick={action}>
          <Plus size={16} />
          Get started
        </button>
      )}
    </div>
  );
}
function CampaignCard({
  c,
  index,
  onClick,
}: {
  c: Row;
  index: number;
  onClick: () => void;
}) {
  return (
    <button className="campaign-card" onClick={onClick}>
      <span className={`campaign-symbol tone-${index % 3}`}>
        {index % 2 ? <Sprout size={23} /> : <Flower2 size={23} />}
      </span>
      <div className="campaign-info">
        <h3>{c.name}</h3>
        <p>{c.description || "Your next meaningful connections"}</p>
        <span className="small">
          <Linkedin size={12} /> LinkedIn first <span>·</span>{" "}
          {c.autonomy === "review"
            ? "Review before sending"
            : c.autonomy === "automatic"
              ? "Automatic"
              : "Manual approval"}
        </span>
      </div>
      <div className="campaign-count">
        <b>{c.prospects || c.target_count || 0}</b>
        <small>{c.prospects ? "prospects" : "target prospects"}</small>
      </div>
      <Badge>{c.status || "draft"}</Badge>
      <ArrowUpRight size={18} />
    </button>
  );
}
function ProspectTable({
  rows,
  onSelect,
}: {
  rows: Row[];
  onSelect: (p: Row) => void;
}) {
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>PERSON</th>
            <th>COMPANY</th>
            <th>STATUS</th>
            <th>MATCH</th>
            <th>RELATIONSHIP</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {rows.map((p, i) => (
            <tr key={p.id}>
              <td>
                <button className="person" onClick={() => onSelect(p)}>
                  <span className={`avatar tone-${i % 3}`}>
                    {p.first_name?.[0]}
                    {p.last_name?.[0]}
                  </span>
                  <span>
                    <b>
                      {p.first_name} {p.last_name}
                    </b>
                    <small>{p.title || "Add a role"}</small>
                  </span>
                </button>
              </td>
              <td>{p.company || "—"}</td>
              <td>
                <Badge>{p.status || "new"}</Badge>
              </td>
              <td className="match-cell">
                <Badge>
                  {p.status === "qualified" || p.qualification === "manual"
                    ? "Approved"
                    : p.qualification || "Not assessed"}
                </Badge>
                {typeof p.ai_score === "number" && (
                  <small>{p.ai_score}/100 fit</small>
                )}
              </td>
              <td>
                <span className="tag">{p.tags?.[0] || "Getting to know"}</span>
              </td>
              <td>
                <button
                  aria-label={`Edit ${p.first_name}`}
                  onClick={() => onSelect(p)}
                >
                  <MoreHorizontal size={17} />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {!rows.length && (
        <div className="empty">Your future connections will appear here.</div>
      )}
    </div>
  );
}
function CampaignForm({
  row,
  accounts,
  save,
}: {
  row?: Row;
  accounts: Row[];
  save: (d: any) => void;
}) {
  const [steps, setSteps] = useState<any[]>(row?.workflow || starterWorkflow),
    [tab, setTab] = useState("audience"),
    [formError, setFormError] = useState("");
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (steps.some((s) => s.type === "email" && !s.email_account_id)) {
          setFormError("Choose an email sender for every email step.");
          setTab("sequence");
          return;
        }
        setFormError("");
        const f = new FormData(e.currentTarget);
        save({
          name: f.get("name"),
          description: f.get("description"),
          status: f.get("status"),
          autonomy: f.get("autonomy"),
          language: f.get("language"),
          target_count: Number(f.get("target_count")),
          search_mode: f.get("search_mode"),
          account_ids: f.getAll("accounts"),
          criteria: {
            roles: f.get("roles"),
            geography: f.get("geography"),
            industry: f.get("industry"),
            company_size: f.get("company_size"),
            seniority: f.get("seniority"),
            instructions: f.get("instructions"),
          },
          workflow: steps,
        });
      }}
    >
      {formError && (
        <div className="error" role="alert">
          {formError}
        </div>
      )}
      <Field name="name" label="Campaign name" row={row} required />
      <Field name="description" label="Description" row={row} />
      <div className="tabs">
        {["audience", "sequence", "launch settings"].map((t) => (
          <button
            type="button"
            key={t}
            className={tab === t ? "active" : ""}
            onClick={() => setTab(t)}
          >
            {t}
          </button>
        ))}
      </div>
      <div hidden={tab !== "audience"}>
        <div className="form-grid">
          {[
            ["roles", "Job titles"],
            ["geography", "Locations"],
            ["industry", "Industry"],
            ["company_size", "Company size"],
            ["seniority", "Seniority / experience"],
          ].map(([n, l]) => (
            <label key={n}>
              {l}
              {n === "company_size" || n === "seniority" ? (
                <select name={n} defaultValue={row?.criteria?.[n] || ""}>
                  <option value="">Any</option>
                  {(n === "company_size"
                    ? [
                        "1–10",
                        "11–50",
                        "51–200",
                        "201–500",
                        "501–1,000",
                        "1,001–5,000",
                        "5,001+",
                      ]
                    : [
                        "Founder / Owner",
                        "C-level",
                        "Vice President",
                        "Director",
                        "Manager",
                        "Senior individual contributor",
                        "Individual contributor",
                      ]
                  ).map((v) => (
                    <option key={v}>{v}</option>
                  ))}
                </select>
              ) : (
                <input name={n} defaultValue={row?.criteria?.[n] || ""} />
              )}
            </label>
          ))}
        </div>
        <label>
          Describe your ideal prospect
          <textarea
            name="instructions"
            placeholder="Founders of early-stage B2B software companies, with an active growth team…"
            defaultValue={row?.criteria?.instructions || ""}
          />
        </label>
        <div className="form-grid">
          <label>
            Search mode
            <select
              name="search_mode"
              defaultValue={row?.search_mode || "once"}
            >
              <option value="once">One-time discovery</option>
              <option value="continuous">Continuous discovery</option>
            </select>
          </label>
          <label>
            Target prospects
            <input
              name="target_count"
              type="number"
              min="1"
              max="100000"
              defaultValue={row?.target_count || 100}
            />
          </label>
        </div>
      </div>
      <div hidden={tab !== "sequence"}>
        <p className="small">
          A reply always stops the sequence. Changes apply to new prospects;
          existing sequences retain their current steps.
        </p>
        <div className="workflow">
          {steps.map((s, i) => (
            <div className="workflow-step" key={i}>
              <span className="step-number">{i + 1}</span>
              <div>
                <select
                  aria-label={`Step ${i + 1} action`}
                  value={s.type}
                  onChange={(e) =>
                    setSteps((ss) =>
                      ss.map((x, j) =>
                        j === i
                          ? {
                              ...x,
                              type: e.target.value,
                              condition: x.condition || "accepted",
                            }
                          : x,
                      ),
                    )
                  }
                >
                  {[
                    "invitation",
                    "delay",
                    "condition",
                    "linkedin_message",
                    "email",
                    "tag",
                    "stop",
                  ].map((t) => (
                    <option key={t}>{t}</option>
                  ))}
                </select>
                {s.type === "delay" ? (
                  <label>
                    Business days
                    <input
                      type="number"
                      min="0"
                      value={s.days || 0}
                      onChange={(e) =>
                        setSteps((ss) =>
                          ss.map((x, j) =>
                            j === i
                              ? { ...x, days: Number(e.target.value) }
                              : x,
                          ),
                        )
                      }
                    />
                  </label>
                ) : s.type === "condition" ? (
                  <label>
                    Continue when
                    <select
                      value={s.condition || "accepted"}
                      onChange={(e) =>
                        setSteps((ss) =>
                          ss.map((x, j) =>
                            j === i ? { ...x, condition: e.target.value } : x,
                          ),
                        )
                      }
                    >
                      {[
                        "accepted",
                        "not_accepted",
                        "has_email",
                        "no_email",
                        "email_opened",
                        "email_clicked",
                        "message_seen",
                        "tag_present",
                        "status_matches",
                        "ai",
                        "no_reply",
                        "score",
                        "field",
                      ].map((t) => (
                        <option key={t}>{t}</option>
                      ))}
                    </select>
                    <input
                      type={s.condition === "score" ? "number" : "text"}
                      aria-label="Condition value"
                      placeholder="Tag or status value (when applicable)"
                      value={s.value || ""}
                      onChange={(e) =>
                        setSteps((ss) =>
                          ss.map((x, j) =>
                            j === i
                              ? {
                                  ...x,
                                  value:
                                    s.condition === "score"
                                      ? Number(e.target.value)
                                      : e.target.value,
                                }
                              : x,
                          ),
                        )
                      }
                    />
                  </label>
                ) : ["invitation", "linkedin_message", "email"].includes(
                    s.type,
                  ) ? (
                  <textarea
                    aria-label={`Step ${i + 1} message`}
                    value={s.message || ""}
                    placeholder="Write a message. Use {{first_name}} and {{company}}."
                    onChange={(e) =>
                      setSteps((ss) =>
                        ss.map((x, j) =>
                          j === i ? { ...x, message: e.target.value } : x,
                        ),
                      )
                    }
                  />
                ) : s.type === "tag" ? (
                  <input
                    aria-label="Tag to add"
                    value={s.value || ""}
                    onChange={(e) =>
                      setSteps((ss) =>
                        ss.map((x, j) =>
                          j === i ? { ...x, value: e.target.value } : x,
                        ),
                      )
                    }
                  />
                ) : null}
                {s.type === "email" && (
                  <label>
                    Email sender
                    <select
                      value={s.email_account_id || ""}
                      onChange={(e) =>
                        setSteps((ss) =>
                          ss.map((x, j) =>
                            j === i
                              ? { ...x, email_account_id: e.target.value }
                              : x,
                          ),
                        )
                      }
                    >
                      <option value="">Choose a connected email account</option>
                      {accounts
                        .filter((a) => a.channel === "email")
                        .map((a) => (
                          <option key={a.id} value={a.id}>
                            {a.name} · {a.email}
                          </option>
                        ))}
                    </select>
                  </label>
                )}
                {s.type === "email" && (
                  <input
                    aria-label="Email subject"
                    placeholder="Email subject"
                    value={s.subject || ""}
                    onChange={(e) =>
                      setSteps((ss) =>
                        ss.map((x, j) =>
                          j === i ? { ...x, subject: e.target.value } : x,
                        ),
                      )
                    }
                  />
                )}
                {s.type === "condition" && (
                  <>
                    <label hidden={s.condition !== "field"}>
                      Prospect field
                      <input
                        value={s.field || ""}
                        placeholder="company"
                        onChange={(e) =>
                          setSteps((ss) =>
                            ss.map((x, j) =>
                              j === i ? { ...x, field: e.target.value } : x,
                            ),
                          )
                        }
                      />
                    </label>
                    {s.condition === "accepted" && (
                      <label>
                        Stop waiting after (calendar days)
                        <input
                          type="number"
                          min="1"
                          max="365"
                          value={s.timeout_days ?? 30}
                          onChange={(e) =>
                            setSteps((ss) =>
                              ss.map((x, j) =>
                                j === i
                                  ? {
                                      ...x,
                                      timeout_days: Number(e.target.value),
                                    }
                                  : x,
                              ),
                            )
                          }
                        />
                      </label>
                    )}
                    <div className="form-grid">
                      {["then_step", "else_step"].map((branch) => (
                        <label key={branch}>
                          {branch === "then_step" ? "If true" : "If false"}
                          <select
                            value={s[branch] ?? ""}
                            onChange={(e) =>
                              setSteps((ss) =>
                                ss.map((x, j) => {
                                  if (j !== i) return x;
                                  const next = { ...x };
                                  if (e.target.value === "")
                                    delete next[branch];
                                  else next[branch] = Number(e.target.value);
                                  return next;
                                }),
                              )
                            }
                          >
                            <option value="">
                              {branch === "then_step"
                                ? "Continue to next step"
                                : "Wait until condition passes"}
                            </option>
                            {steps.map((next, k) =>
                              k > i ? (
                                <option key={k} value={k}>
                                  Step {k + 1}: {next.type}
                                </option>
                              ) : null,
                            )}
                          </select>
                        </label>
                      ))}
                    </div>
                  </>
                )}
              </div>
              <button
                type="button"
                aria-label={`Remove step ${i + 1}`}
                onClick={() => setSteps((ss) => removeWorkflowStep(ss, i))}
              >
                <X size={16} />
              </button>
            </div>
          ))}
        </div>
        <button
          type="button"
          onClick={() =>
            setSteps((s) => [...s, { type: "linkedin_message", message: "" }])
          }
        >
          <Plus size={15} />
          Add step
        </button>
      </div>
      <div hidden={tab !== "launch settings"}>
        <label>
          Autonomy
          <select name="autonomy" defaultValue={row?.autonomy || "review"}>
            <option value="manual">Approve every prospect and message</option>
            <option value="review">Review a sample, then automate</option>
            <option value="automatic">Automatic after launch</option>
          </select>
        </label>
        <label>
          Message language
          <input name="language" defaultValue={row?.language || "English"} />
        </label>
        <label>
          Campaign status
          <select name="status" defaultValue={row?.status || "draft"}>
            <option value="draft">Draft</option>
            {row?.status === "active" && <option value="active">Active</option>}
            <option value="paused">Paused</option>
          </select>
        </label>
        <fieldset>
          <legend>Sender accounts</legend>
          {accounts
            .filter((a) => a.channel === "linkedin")
            .map((a) => (
              <label className="check" key={a.id}>
                <input
                  name="accounts"
                  type="checkbox"
                  value={a.id}
                  defaultChecked={row?.account_ids?.includes(a.id)}
                />
                {a.name}
              </label>
            ))}
          {!accounts.length && (
            <p>Connect a LinkedIn account before launching.</p>
          )}
        </fieldset>
        <p className="small">
          Prospects are assigned according to available sender quotas. Each
          prospect keeps the same sender throughout their sequence.
        </p>
      </div>
      <div className="modal-footer">
        <span className="small">Your changes. Your call.</span>
        <button className="primary">
          Save campaign <Check size={15} />
        </button>
      </div>
    </form>
  );
}
