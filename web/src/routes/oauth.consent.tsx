import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { Flower2, ShieldCheck } from "lucide-react";
import { api, type Row } from "../lib/api";
export const Route = createFileRoute("/oauth/consent")({ component: Consent });
function Consent() {
  const [spaces, setSpaces] = useState<Row[]>([]),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [client, setClient] = useState({ id: "", host: "" });
  useEffect(() => {
    sessionStorage.setItem("oauth_return", location.pathname + location.search);
    const params = new URLSearchParams(location.search);
    let host = "Invalid callback";
    try {
      host = new URL(params.get("redirect_uri") || "").host;
    } catch {}
    setClient({ id: params.get("client_id") || "", host });
    api("/workspaces")
      .then(setSpaces)
      .catch((e) => setError(e.message));
  }, []);
  return (
    <div className="auth-page">
      <a className="brand" href="/">
        <Flower2 />
        tullips
      </a>
      <div className="auth-card">
        <ShieldCheck />
        <h1>Let your agent help.</h1>
        <p>
          <b>{client.host}</b>
          <br />
          Client: {client.id}
        </p>
        <p>
          Choose the workspace and permissions you want to share. You can revoke
          access at any time.
        </p>
        {error && (
          <div className="error">
            {error}
            <a href="/login">Sign in</a>
          </div>
        )}
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setBusy(true);
            try {
              const f = new FormData(e.currentTarget),
                params = Object.fromEntries(
                  new URLSearchParams(location.search),
                );
              const r = await api("/oauth/authorize", "POST", {
                ...params,
                workspace_id: f.get("workspace"),
                scope: f.getAll("scope").join(" "),
              });
              location.href = r.redirect_uri;
            } catch (e) {
              setError((e as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          <label>
            Workspace
            <select name="workspace" required>
              {spaces.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </label>
          <fieldset>
            <legend>Allow this agent to</legend>
            {["read", "write", "launch", "send"].map((s) => (
              <label className="check" key={s}>
                <input
                  type="checkbox"
                  name="scope"
                  value={s}
                  defaultChecked={s === "read"}
                />
                {
                  {
                    read: "Read your CRM and campaigns",
                    write: "Modify prospects and campaigns",
                    launch: "Launch campaigns",
                    send: "Send messages",
                  }[s]
                }
              </label>
            ))}
          </fieldset>
          <button className="primary" disabled={busy || !spaces.length}>
            Authorize agent
          </button>
        </form>
        <a className="button" href="/app">
          Cancel
        </a>
      </div>
    </div>
  );
}
