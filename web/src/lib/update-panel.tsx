import { useEffect, useState } from "react";
import { api } from "./api";

type Status = {
  phase: string; version: string; latest?: string; error?: string;
  automatic: boolean; busy: boolean; mode: string; retention: number;
  release_url: string; history: { action: string; actor: string; at: string }[];
};
export function UpdatePanel() {
  const [status, setStatus] = useState<Status | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [restore, setRestore] = useState(false);
  async function refresh() {
    try { setStatus(await api("/installation/updates")); setError(""); }
    catch (e) { setError(e instanceof Error ? e.message : "Unable to reach the updater"); }
  }
  useEffect(() => {
    void refresh();
    const timer = setInterval(refresh, 5000);
    return () => clearInterval(timer);
  }, []);
  async function action(path: string, body = {}, method = "POST") {
    setPending(true);
    try {
      await api("/installation/updates/" + path, method, body);
      await refresh();
    } catch (e) { setError(e instanceof Error ? e.message : "The operation failed"); }
    finally { setPending(false); }
  }
  const disabled = pending || status?.busy;
  const recovering = ["awaiting_restore", "recovery_failed", "restoring"].includes(status?.phase || "");
  return <section className="panel form-panel update-panel" aria-labelledby="updates-heading">
    <h2 id="updates-heading">Installation updates</h2>
    <p>These settings apply to every workspace on this server.</p>
    {error && <p role="alert">{error}. If the application is offline, use the server update CLI.</p>}
    {!status ? <button className="button" onClick={refresh}>Retry connection</button> : <>
      <p>Installed: <strong>{status.version}</strong> · Latest: <strong>{status.latest || "Not checked"}</strong></p>
      <p role="status" aria-live="polite">{status.busy ? "Operation in progress" : status.phase.replaceAll("_", " ")}</p>
      {status.error && <p role="alert">{status.error}</p>}
      <label className="update-toggle"><input type="checkbox" checked={status.automatic} disabled={disabled}
        onChange={e => action("settings", { automatic: e.target.checked }, "PATCH")} /> Automatically install stable patch and minor updates</label>
      <p>Major upgrades require your approval. Updates briefly interrupt service and always require a successful local backup. The latest {status.retention} successful backups are retained.</p>
      <p><a href={status.release_url} target="_blank" rel="noreferrer">Read release notes</a></p>
      <div className="actions">
        <button className="button" disabled={disabled} onClick={() => action("check")}>Check for updates</button>
        <button className="button primary" disabled={disabled || recovering || !status.latest || status.latest === status.version}
          onClick={() => setConfirmation(status.latest || "")}>Update now</button>
        {recovering && <button className="button" disabled={disabled} onClick={() => action("recover")}>Retry safe recovery</button>}
      </div>
      {confirmation && <div role="group" aria-label="Confirm update">
        <p>Install version {confirmation}? All workspaces will be briefly unavailable.</p>
        <button className="button primary" disabled={disabled} onClick={() => { action("apply", { version: confirmation }); setConfirmation(""); }}>Install {confirmation}</button>
        <button className="button" onClick={() => setConfirmation("")}>Cancel</button>
      </div>}
      {recovering && <div>
        <p>Database restoration replaces current data with the pre-update backup. Approve only after reviewing the failed update.</p>
        <label className="update-toggle"><input type="checkbox" checked={restore} onChange={e => setRestore(e.target.checked)} /> I approve restoring the pre-update database.</label>
        <button className="button" disabled={disabled || !restore} onClick={() => action("restore", { confirm_version: status.version })}>Restore version {status.version}</button>
      </div>}
      <h3>Recent activity</h3>
      <ul>{status.history?.slice(-10).reverse().map((event, i) => <li key={`${event.at}-${i}`}>
        <time dateTime={event.at}>{new Date(event.at).toLocaleString()}</time>: {event.action.replaceAll("_", " ")}
      </li>)}</ul>
    </>}
  </section>;
}
