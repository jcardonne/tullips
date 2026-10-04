import { createFileRoute } from "@tanstack/react-router";
import { createAuthClient } from "better-auth/react";
import { useEffect, useState } from "react";
import { Flower2, ArrowRight, Eye, EyeOff } from "lucide-react";
const authClient = createAuthClient();
export const Route = createFileRoute("/login")({ component: Login });
function Login() {
  const [signup, setSignup] = useState(false),
    [showPassword, setShowPassword] = useState(false),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  useEffect(
    () =>
      setSignup(new URLSearchParams(location.search).get("signup") === "true"),
    [],
  );
  return (
    <div className="auth-page">
      <a href="/" className="brand">
        <Flower2 />
        tullips
      </a>
      <div className="auth-card">
        <span className="eyebrow">A FRESH START</span>
        <h1>{signup ? "Room to grow." : "Welcome back."}</h1>
        <p>
          {signup
            ? "Create your account and make your next meaningful connection."
            : "Your next good conversation is waiting."}
        </p>
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setBusy(true);
            setError("");
            const f = new FormData(e.currentTarget);
            try {
              const data = {
                email: String(f.get("email")),
                password: String(f.get("password")),
                name: String(f.get("name") || ""),
              };
              const result = signup
                ? await authClient.signUp.email(data)
                : await authClient.signIn.email(data);
              if (result.error) throw new Error(result.error.message);
              const returnPath = sessionStorage.getItem("oauth_return");
              sessionStorage.removeItem("oauth_return");
              location.href = returnPath?.startsWith("/oauth/consent?")
                ? returnPath
                : signup ? "/onboarding" : "/app";
            } catch (e) {
              setError((e as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          {signup && (
            <label>
              Your name
              <input name="name" required autoComplete="name" />
            </label>
          )}
          <label>
            Email address
            <input
              name="email"
              type="email"
              required
              autoComplete="email"
              placeholder="you@company.com"
            />
          </label>
          <label>
            Password
            <span className="password-field"><input
              id="password"
              name="password"
              type={showPassword ? "text" : "password"}
              required
              minLength={signup ? 12 : 1}
              autoComplete={signup ? "new-password" : "current-password"}
            /><button type="button" className="password-toggle" aria-controls="password" aria-label={showPassword ? "Hide password" : "Show password"} aria-pressed={showPassword} onClick={() => setShowPassword(!showPassword)}>{showPassword ? <EyeOff size={18} /> : <Eye size={18} />}</button></span>
          </label>
          {signup && <small>At least 12 characters.</small>}
          {error && (
            <p className="error" role="alert">
              {error}
            </p>
          )}
          <button className="primary" disabled={busy} aria-busy={busy}>
            {busy ? "One moment…" : signup ? "Create account" : "Log in"}
            <ArrowRight size={16} />
          </button>
        </form>
        <button className="text-button" onClick={() => { setSignup(!signup); setShowPassword(false); }}>
          {signup
            ? "Already growing with us? Log in"
            : "New to Tullips? Create an account"}
        </button>
        <a className="muted small" href="/app?demo=true">
          Explore the interactive demo
        </a>
      </div>
    </div>
  );
}
