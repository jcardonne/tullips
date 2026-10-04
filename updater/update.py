#!/usr/bin/env python3
"""Tullips updater. Stdlib only; deployment tools remain the source of truth."""
import argparse
import contextlib
import datetime
import fcntl
import hashlib
import hmac
import http.server
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

VERSION = re.compile(r"^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$")
REPO = re.compile(r"^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")
DIAGNOSTICS = None
TERMINAL = {"idle", "complete", "rolled_back", "failed"}


def version(value):
    if not isinstance(value, str) or not VERSION.fullmatch(value):
        raise ValueError("Expected a stable release version such as 1.2.3")
    return tuple(map(int, value.split(".")))


def atomic(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd, temp = tempfile.mkstemp(dir=path.parent)
    try:
        with os.fdopen(fd, "w") as out:
            json.dump(value, out, indent=2)
            out.flush()
            os.fsync(out.fileno())
        os.replace(temp, path)
        fd = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        if os.path.exists(temp):
            os.unlink(temp)


def read(path, default=None):
    return json.loads(Path(path).read_text()) if Path(path).exists() else default


def run(args, *, env=None, cwd=None, input=None, timeout=900, stdout=None):
    # Never expose commands or stderr through the API; private diagnostics are operator-only.
    result = subprocess.run(list(map(str, args)), env=env, cwd=cwd, input=input,
                            stdout=stdout or subprocess.PIPE, stderr=subprocess.PIPE,
                            timeout=timeout)
    if result.returncode:
        if DIAGNOSTICS is not None:
            try:
                with open(DIAGNOSTICS, "ab") as log:
                    os.chmod(DIAGNOSTICS, 0o600)
                    log.write(("\n" + Path(str(args[0])).name + " failed\n").encode() + result.stderr)
            except OSError: pass
        raise RuntimeError(f"{Path(str(args[0])).name} failed (exit {result.returncode}); inspect the service logs locally")
    return result.stdout.decode().strip() if result.stdout else ""


def env_file(path):
    values = {}
    for line in Path(path).read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, sep, value = line.partition("=")
        if not sep or not re.fullmatch(r"[A-Z][A-Z0-9_]*", key):
            raise ValueError("Invalid environment file")
        if value[:1] in ("'", '"'):
            if len(value) < 2 or value[-1] != value[0]:
                raise ValueError("Invalid quoted environment value")
            value = value[1:-1]
        values[key] = value
    return values


def database_env(environment):
    parsed = urllib.parse.urlsplit(environment["DATABASE_URL"])
    if parsed.scheme not in {"postgres", "postgresql"} or not parsed.hostname or not parsed.path.strip("/"):
        raise ValueError("DATABASE_URL must be a PostgreSQL connection URL with a database name")
    values = {"PGHOST": parsed.hostname, "PGPORT": str(parsed.port or 5432),
              "PGDATABASE": urllib.parse.unquote(parsed.path[1:]),
              "PGUSER": urllib.parse.unquote(parsed.username or "postgres"),
              "PGPASSWORD": urllib.parse.unquote(parsed.password or ""), "PGCONNECT_TIMEOUT": "10"}
    for key, value in urllib.parse.parse_qsl(parsed.query):
        if key not in {"sslmode", "sslrootcert", "sslcert", "sslkey", "connect_timeout", "options", "application_name"}:
            raise ValueError("Unsupported PostgreSQL connection parameter")
        values["PG" + key.upper().replace("_", "")] = value
    return environment | values


def fetch(url, *, headers=None, data=None, method=None, limit=2**20):
    request = urllib.request.Request(url, data=None if data is None else json.dumps(data).encode(),
        headers={"Accept": "application/json", "Content-Type": "application/json", **(headers or {})}, method=method)
    with urllib.request.urlopen(request, timeout=30) as response:
        raw = response.read(limit + 1)
    if len(raw) > limit:
        raise ValueError("Response exceeded the size limit")
    return raw


def download(url, path, limit=512 * 1024**2):
    if urllib.parse.urlparse(url).scheme != "https":
        raise ValueError("Release downloads require HTTPS")
    with urllib.request.urlopen(url, timeout=60) as response, open(path, "wb") as output:
        size = 0
        while chunk := response.read(1024**2):
            size += len(chunk)
            if size > limit:
                raise ValueError("Release artifact exceeded the size limit")
            output.write(chunk)


def sha(path):
    with open(path, "rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def validate_manifest(m, repo):
    version(m["version"])
    version(m["minimum_version"])
    if m.get("format") != 1 or m.get("updater_protocol") != 1:
        raise ValueError("This release requires a newer updater; follow its manual upgrade instructions")
    if m.get("repository") != repo:
        raise ValueError("Release repository does not match the trusted repository")
    for component in ("backend", "web"):
        if not re.fullmatch(r"ghcr\.io/" + re.escape(repo.lower()) + "-" + component + r"@sha256:[a-f0-9]{64}", m["images"][component]):
            raise ValueError("Invalid or unpinned release image")
    if not re.fullmatch(r"[a-f0-9]{64}", m["source_sha256"]):
        raise ValueError("Invalid source checksum")
    for compatible in m.get("rollback_versions", []):
        version(compatible)
    if version(m["minimum_version"]) > version(m["version"]):
        raise ValueError("Invalid upgrade range")
    return m


class Updater:
    def __init__(self, config):
        self.config_path = Path(config).resolve()
        self.c = read(self.config_path)
        if not self.c or not REPO.fullmatch(self.c.get("repository", "")):
            raise ValueError("Configure the trusted release repository")
        self.root = Path(self.c["state_dir"]).resolve()
        self.root.mkdir(parents=True, exist_ok=True, mode=0o700)
        global DIAGNOSTICS
        DIAGNOSTICS = self.root / "diagnostics.log"
        self.env = os.environ | env_file(self.c["env_file"])
        self.token = self.env.get("UPDATER_TOKEN", "")
        if len(self.token) < 32:
            raise ValueError("UPDATER_TOKEN must contain at least 32 characters")
        if self.env.get("DEPLOYMENT_MODE") == "saas":
            raise ValueError("Self-hosted updater is disabled in SaaS mode")
        from adapters import Deployment
        self.deployment = Deployment(self)
        if not (self.root / "state.json").exists():
            atomic(self.root / "state.json", {"phase": "idle", "version": self.c["version"], "history": []})

    @contextlib.contextmanager
    def lock(self):
        with open(self.root / "update.lock", "a") as lock:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                raise RuntimeError("Another update operation is already running") from None
            yield

    def state(self):
        return read(self.root / "state.json")

    def save(self, **changes):
        state = self.state() | changes
        state["updated_at"] = datetime.datetime.now(datetime.timezone.utc).isoformat()
        atomic(self.root / "state.json", state)
        return state

    def event(self, action, actor):
        history = self.state().get("history", [])
        history.append({"action": action, "actor": actor, "at": datetime.datetime.now(datetime.timezone.utc).isoformat()})
        self.save(history=history[-100:])

    def status(self):
        state = self.state()
        # Never expose local config, credentials, deployment snapshots, or backup contents.
        return {k: state.get(k) for k in ("phase", "version", "latest", "error", "updated_at", "history", "backup", "failed_version")} | {
            "automatic": read(self.root / "settings.json", {}).get("automatic", False),
            "mode": self.c["mode"], "retention": self.c.get("retention", 3),
            "release_url": f"https://github.com/{self.c['repository']}/releases",
        }

    def sql(self, query):
        return run(["psql", "-X", "-A", "-t", "-v", "ON_ERROR_STOP=1"],
                   env=database_env(self.env), input=query.encode(), timeout=60)

    def maintenance(self, active):
        self.sql("UPDATE installation SET maintenance=" + ("true" if active else "false") + " WHERE id;")

    def check(self):
        repo = self.c["repository"]
        metadata = json.loads(fetch(f"https://api.github.com/repos/{repo}/releases?per_page=100"))
        candidates = [r for r in metadata if not r["draft"] and not r["prerelease"] and VERSION.fullmatch(r["tag_name"].removeprefix("v"))]
        if not candidates:
            self.save(latest=None, available=[], error="No stable releases have been published")
            return
        current = version(self.state()["version"])
        # Keep both the newest release and the newest within the installed major.
        newest = max(candidates, key=lambda r: version(r["tag_name"].removeprefix("v")))
        same_major = [r for r in candidates if version(r["tag_name"].removeprefix("v"))[0] == current[0]]
        selected = {newest["tag_name"]: newest}
        if same_major:
            r = max(same_major, key=lambda r: version(r["tag_name"].removeprefix("v")))
            selected[r["tag_name"]] = r
        manifests = []
        for tag, release in selected.items():
            v = tag.removeprefix("v")
            folder = self.root / "downloads" / v
            folder.mkdir(parents=True, exist_ok=True, mode=0o700)
            base = f"https://github.com/{repo}/releases/download/{tag}"
            download(base + "/release.json", folder / "release.json", 1024**2)
            download(base + "/release.sigstore.json", folder / "release.sigstore.json", 8*1024**2)
            run(["gh", "attestation", "verify", folder / "release.json", "--bundle", folder / "release.sigstore.json",
                 "--repo", repo, "--signer-workflow", repo + "/.github/workflows/release.yml", "--deny-self-hosted-runners", "--source-ref", "refs/tags/" + tag], timeout=90)
            m = validate_manifest(read(folder / "release.json"), repo)
            if m["version"] != v:
                raise ValueError("Release tag does not match signed metadata")
            m["download_base"] = base
            manifests.append(m)
        self.save(latest=newest["tag_name"].removeprefix("v"), available=manifests, error=None)

    def backup(self):
        folder = self.root / "backups" / (str(time.time_ns()))
        folder.mkdir(parents=True, mode=0o700)
        self.save(backup=str(folder))
        env = database_env(self.env)
        run(["pg_dump", "--format=custom", "--no-owner", "--file", folder / "database.dump"], env=env)
        run(["pg_restore", "--list", folder / "database.dump"])
        shutil.copy2(self.c["env_file"], folder / "environment")
        shutil.copy2(self.config_path, folder / "updater.json")
        atomic(folder / "deployment.json", self.state()["previous"])
        for item in folder.iterdir():
            item.chmod(0o600)
            with item.open("rb") as stream: os.fsync(stream.fileno())
        atomic(folder / "checksums.json", {p.name: sha(p) for p in folder.iterdir()})
        self.save(backup_complete=True)

    def verify_backup(self):
        folder = Path(self.state()["backup"])
        if folder.parent != self.root / "backups":
            raise ValueError("Invalid backup path")
        checksums = read(folder / "checksums.json")
        for name in ("database.dump", "environment", "updater.json", "deployment.json"):
            if checksums.get(name) != sha(folder / name):
                raise ValueError("Backup verification failed")
        run(["pg_restore", "--list", folder / "database.dump"])
        return folder

    def apply(self, target, actor="cli", automatic=False):
        state = self.state()
        if state["phase"] not in TERMINAL:
            raise ValueError("Resolve the interrupted update before starting another")
        current, target_version = version(state["version"]), version(target)
        if target_version <= current:
            raise ValueError("Choose a newer version")
        if automatic and (target_version[0] != current[0] or state.get("failed_version") == target):
            raise ValueError("This release requires explicit approval")
        m = next((m for m in state.get("available", []) if m["version"] == target), None)
        if not m:
            raise ValueError("Check for updates before selecting a release")
        if current < version(m["minimum_version"]):
            raise ValueError("An intermediate upgrade is required")
        self.event("update_requested:" + target, actor)
        self.save(phase="preparing", target=m, error=None, migration_started=False, backup_complete=False,
                  previous_version=state["version"], previous=None, backup=None, maintenance_entered=False, restore_started=False)
        try:
            self.deployment.preflight()
            self.save(previous=self.deployment.snapshot())
            self.deployment.prepare(m)
            self.save(phase="stopping", maintenance_entered=True)
            self.maintenance(True)
            self.deployment.stop()
            self.save(phase="backing_up")
            self.backup()
            self.save(phase="migrating", migration_started=True)
            self.deployment.migrate(m)
            self.save(phase="deploying")
            self.deployment.deploy(m)
            self.save(phase="checking_health")
            self.deployment.healthy()
            self.maintenance(False)
            self.save(phase="complete", version=target, failed_version=None, error=None)
            self.event("update_complete:" + target, actor)
            try: self.prune()
            except Exception: self.save(error="Update succeeded, but old backups could not be pruned. Check local storage permissions.")
        except Exception as exc:
            self.save(error=safe_error(exc), failed_version=target)
            self.recover(actor)

    def recover(self, actor="cli"):
        state = self.state()
        if state["phase"] in TERMINAL:
            return
        self.event("recovery_requested", actor)
        safe = not state.get("restore_started") and (not state.get("migration_started") or state.get("previous_version") in state.get("target", {}).get("rollback_versions", []))
        if not state.get("maintenance_entered"):
            self.save(phase="failed")
            return
        try:
            self.maintenance(True)
            self.deployment.stop()
            if not safe:
                self.save(phase="awaiting_restore", error="The update failed after database migration. Database restoration requires administrator approval.")
                return
            self.save(phase="rolling_back")
            self.deployment.restore(state["previous"])
            self.deployment.healthy()
            self.maintenance(False)
            self.save(phase="rolled_back", version=state["previous_version"])
            self.event("rollback_complete", actor)
        except Exception as exc:
            self.save(phase="recovery_failed", error=safe_error(exc))

    def restore(self, actor="cli", confirm=None):
        state = self.state()
        if state["phase"] not in {"awaiting_restore", "recovery_failed", "restoring"} or not state.get("backup_complete"):
            raise ValueError("There is no failed update with a complete backup to restore")
        if confirm != state["previous_version"]:
            raise ValueError("Confirm the previous version to approve database restoration")
        folder = self.verify_backup()
        self.event("database_restore_approved", actor)
        self.save(phase="restoring", restore_started=True)
        try:
            self.deployment.stop()
            # Replace the entire public schema, including tables introduced by failed migrations.
            # All application writers are stopped and this is an explicitly approved restore.
            self.sql("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
            run(["pg_restore", "--exit-on-error", "--single-transaction", "--no-owner", "--no-privileges", "--dbname", database_env(self.env)["PGDATABASE"], folder / "database.dump"], env=database_env(self.env))
            self.maintenance(True)
            self.deployment.restore(state["previous"])
            self.deployment.healthy()
            self.maintenance(False)
            self.save(phase="rolled_back", version=state["previous_version"], error=None)
            self.event("database_restore_complete", actor)
        except Exception as exc:
            self.save(phase="recovery_failed", error=safe_error(exc))

    def prune(self):
        backups = sorted((self.root / "backups").glob("*/checksums.json"), reverse=True)
        for path in backups[max(1, int(self.c.get("retention", 3))):]:
            if str(path.parent) != self.state().get("backup"):
                shutil.rmtree(path.parent)

    def settings(self, automatic):
        if type(automatic) is not bool:
            raise ValueError("automatic must be true or false")
        atomic(self.root / "settings.json", {"automatic": automatic})

    def admin(self, email):
        if not email or len(email) > 320 or any(ord(c) < 32 for c in email):
            raise ValueError("Provide an existing user's email address")
        literal = "'" + email.replace("'", "''") + "'"
        result = self.sql(f'UPDATE installation SET admin_user_id=(SELECT id FROM "user" WHERE lower(email)=lower({literal})), bootstrap_open=false WHERE id AND EXISTS(SELECT 1 FROM "user" WHERE lower(email)=lower({literal})) RETURNING admin_user_id;')
        if not result or result == "UPDATE 0":
            raise ValueError("User not found")
        self.event("administrator_assigned", "cli")


def safe_error(exc):
    # Detailed subprocess/HTTP errors may contain URLs, tokens, or database passwords.
    if isinstance(exc, (ValueError, RuntimeError)):
        return str(exc)[:500]
    return f"{type(exc).__name__}: operation failed; inspect local service logs"


def serve(updater):
    gate = threading.Lock()

    def operation(action, data, actor):
        if action == "check": updater.check()
        elif action == "apply": updater.apply(data["version"], actor)
        elif action == "recover": updater.recover(actor)
        elif action == "restore": updater.restore(actor, data.get("confirm_version"))
        elif action == "settings":
            updater.settings(data["automatic"])
            updater.event("automatic_updates:" + str(data["automatic"]), actor)

    class Handler(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_): pass

        def do_GET(self): self.handle_request()
        def do_POST(self): self.handle_request()
        def do_PATCH(self): self.handle_request()

        def respond(self, code, payload):
            raw = json.dumps(payload).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        def handle_request(self):
            if not hmac.compare_digest(self.headers.get("Authorization", ""), "Bearer " + updater.token):
                self.respond(401, {"error": "Authentication required"}); return
            if self.command == "GET" and self.path == "/updates":
                self.respond(200, updater.status() | {"busy": gate.locked()}); return
            routes = {("POST", "/updates/" + name): name for name in ("check", "apply", "restore", "recover")}
            routes[("PATCH", "/updates/settings")] = "settings"
            action = routes.get((self.command, self.path))
            if action is None:
                self.respond(404, {"error": "Not found"}); return
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 <= length <= 4096 or self.headers.get("Transfer-Encoding"):
                    raise ValueError("Invalid request body")
                self.connection.settimeout(5)
                data = json.loads(self.rfile.read(length) or b"{}")
                if not isinstance(data, dict): raise ValueError("Object required")
                if action == "apply": version(data.get("version"))
                if action == "settings" and type(data.get("automatic")) is not bool: raise ValueError("automatic must be true or false")
                if action == "restore" and not data.get("confirm_version"): raise ValueError("Confirm the previous version")
            except (ValueError, TypeError):
                self.respond(400, {"error": "Invalid request body"}); return
            if not gate.acquire(blocking=False):
                self.respond(409, {"error": "An operation is already running"}); return
            disk_lock = updater.lock()
            try: disk_lock.__enter__()
            except RuntimeError:
                gate.release()
                self.respond(409, {"error": "Another update operation is already running"}); return
            actor = self.headers.get("X-Tullips-Actor", "api")[:200]
            def background():
                try: operation(action, data, actor)
                except Exception as exc: updater.save(error=safe_error(exc))
                finally:
                    disk_lock.__exit__(None, None, None)
                    gate.release()
            threading.Thread(target=background, daemon=True).start()
            self.respond(202, {"accepted": True})

    def record_error(exc):
        try:
            with updater.lock(): updater.save(error=safe_error(exc))
        except RuntimeError: pass  # A CLI operation owns the state, so leave it untouched.

    def scheduled():
        # Recover interrupted operations before allowing another automatic release.
        with gate:
            try:
                with updater.lock(): updater.recover("startup")
            except Exception as exc: record_error(exc)
        while True:
            if gate.acquire(blocking=False):
                try:
                    with updater.lock():
                        if updater.state()["phase"] in TERMINAL:
                            updater.check()
                            if read(updater.root / "settings.json", {}).get("automatic", False):
                                state = updater.state()
                                current = version(state["version"])
                                candidates = [m for m in state.get("available", []) if version(m["version"])[0] == current[0] and version(m["version"]) > current and version(m["minimum_version"]) <= current and m["version"] != state.get("failed_version")]
                                if candidates: updater.apply(max(candidates, key=lambda m: version(m["version"]))["version"], "scheduler", True)
                except Exception as exc: record_error(exc)
                finally: gate.release()
            time.sleep(3600)

    threading.Thread(target=scheduled, daemon=True).start()
    host, port = updater.c.get("listen", "127.0.0.1:8090").rsplit(":", 1)
    http.server.ThreadingHTTPServer((host, int(port)), Handler).serve_forever()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", default=os.getenv("UPDATER_CONFIG", "/etc/tullips/updater.json"))
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("serve", "status", "check", "recover"):
        sub.add_parser(name)
    sub.add_parser("update").add_argument("version")
    sub.add_parser("restore").add_argument("--confirm-version", required=True)
    sub.add_parser("admin").add_argument("email")
    sub.add_parser("automatic").add_argument("value", choices=("on", "off"))
    args = parser.parse_args()
    os.umask(0o077)
    updater = Updater(args.config)
    if args.command == "serve": serve(updater); return
    if args.command == "status":
        print(json.dumps(updater.status(), indent=2)); return
    with updater.lock():
        if args.command == "check": updater.check()
        elif args.command == "update": updater.check(); updater.apply(args.version)
        elif args.command == "restore": updater.restore(confirm=args.confirm_version)
        elif args.command == "recover": updater.recover()
        elif args.command == "admin": updater.admin(args.email)
        elif args.command == "automatic": updater.settings(args.value == "on")
        print(json.dumps(updater.status(), indent=2))
        if updater.state()["phase"] in {"failed", "rolled_back", "awaiting_restore", "recovery_failed"} and args.command in {"update", "restore", "recover"}:
            raise SystemExit(1)


if __name__ == "__main__":
    try: main()
    except Exception as exc:
        print(safe_error(exc), file=sys.stderr)
        raise SystemExit(1)
