import { useEffect, useState } from "react";
import { CreditCard, ArrowUpRight } from "lucide-react";
import { api } from "./api";
export function BillingPanel({ base, demo }: { base: string; demo: boolean }) {
  const [billing, setBilling] = useState<any>(null),
    [extra, setExtra] = useState(0),
    [confirm, setConfirm] = useState(false),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [notice, setNotice] = useState("");
  useEffect(() => {
    if (demo) {
      setBilling({ mode: "demo", configured: false });
      return;
    }
    api(`${base}/billing`)
      .then((b) => {
        setBilling(b);
        setExtra(Math.max(0, (b.account_limit || 3) - 3));
      })
      .catch((e) => setError(e.message));
  }, [base, demo]);
  async function action(path: string, body = {}) {
    setBusy(true);
    setError("");
    try {
      const r = await api(`${base}/billing/${path}`, "POST", body);
      if (r.url) location.href = r.url;
      else {
        setNotice(r.message || "Subscription updated.");
        setConfirm(false);
        setBilling(await api(`${base}/billing`));
      }
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="panel form-panel">
      <span className="icon-box">
        <CreditCard />
      </span>
      <h2>Space for your whole team.</h2>
      <p>
        One workspace subscription. Three LinkedIn accounts included. Add more
        as your team grows.
      </p>
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      {notice && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}
      <div className="price">
        {billing?.mode === "self_hosted"
          ? "Your workspace. Your infrastructure."
          : billing?.configured
            ? "Grow at your pace."
            : "Pricing coming soon"}
      </div>
      <p>
        {billing?.mode === "self_hosted"
          ? "All features are free under the MIT license. Configure your own AI provider keys."
          : `AI credits available: ${billing?.credits ?? "—"}. Prepared messages continue sending when credits run out.`}
      </p>
      <div className="actions">
        <button
          className="primary"
          disabled={busy || !billing?.configured}
          onClick={() =>
            action(billing.status === "active" ? "portal" : "checkout")
          }
        >
          Manage subscription <ArrowUpRight size={15} />
        </button>
        <button
          disabled={busy || !billing?.configured}
          onClick={() => action("topup")}
        >
          Add AI credits
        </button>
      </div>
      <hr />
      <h3>LinkedIn account allowance</h3>
      <p>
        Three accounts are included. Additional accounts are billed monthly at
        the configured per-account price.
      </p>
      <label>
        Additional paid account slots
        <input
          type="number"
          min="0"
          max="1000"
          value={extra}
          onChange={(e) => {
            setExtra(Number(e.target.value));
            setConfirm(false);
          }}
        />
      </label>
      {confirm ? (
        <div className="notice">
          <div>
            <b>Confirm subscription change</b>
            <p>
              Set your allowance to {extra + 3} LinkedIn accounts. Stripe will
              apply the configured price and prorate the subscription change.
            </p>
            <button
              disabled={busy}
              onClick={() => action("accounts", { additional_accounts: extra })}
            >
              Confirm paid change
            </button>
            <button onClick={() => setConfirm(false)}>Cancel</button>
          </div>
        </div>
      ) : (
        <button
          disabled={
            busy || !billing?.configured || billing?.status !== "active"
          }
          onClick={() => setConfirm(true)}
        >
          Review account allowance
        </button>
      )}
      {!billing?.configured && (
        <p className="small">
          Payments become available when the administrator has configured
          pricing.
        </p>
      )}
    </div>
  );
}
