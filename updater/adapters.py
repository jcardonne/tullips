"""The four supported deployment layouts. No arbitrary commands accepted from the UI."""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import tarfile
import time
from update import download, fetch, run, sha, atomic, read

COMPONENTS = ("api", "web", "worker")


class Deployment:
    def __init__(self, updater):
        self.u, self.c = updater, updater.c
        self.mode = self.c["mode"]
        self.migration_label = "tullips.updater=" + hashlib.sha256(str(self.u.root).encode()).hexdigest()[:16]
        if self.mode not in {"compose", "coolify", "kubernetes", "systemd"}:
            raise ValueError("Unsupported installation type")

    def compose(self, *args, env=None):
        return run(["docker", "compose", "--project-directory", self.c["project_dir"],
                    "--env-file", self.c["env_file"], "-f", Path(self.c["project_dir"]) / "compose.yaml",
                    "-f", self.u.root / "release.compose.json", *args], env=env or self.u.env)

    def kube(self, *args, input=None):
        return run(["kubectl", "--namespace", self.c["namespace"], *args], input=input)

    def coolify(self, path, method="GET", data=None):
        return json.loads(fetch(self.c["coolify_url"].rstrip("/") + "/api/v1/" + path,
            headers={"Authorization": "Bearer " + self.u.env["COOLIFY_TOKEN"]}, method=method, data=data, limit=8*1024**2))

    def preflight(self):
        required = ["psql", "pg_dump", "pg_restore", "gh"]
        required += {"compose": ["docker"], "coolify": ["docker"], "kubernetes": ["kubectl"], "systemd": ["systemctl", "git", "go", "npm", "node"]}[self.mode]
        for binary in required:
            if not shutil.which(binary): raise ValueError(f"Required executable not found: {binary}")
        for component in COMPONENTS:
            if not self.c.get(component + "_url"):
                raise ValueError(f"Configure the {component} readiness URL")
        if self.u.sql("SELECT count(*) FROM installation;") != "1":
            raise ValueError("Run the installation migration first")
        server_major = int(self.u.sql("SHOW server_version_num")) // 10000
        client_major = int(re.search(r"(\d+)\.", run(["pg_dump", "--version"]))[1])
        if client_major < server_major: raise ValueError("Install PostgreSQL client tools at least as new as the database server")
        size = int(self.u.sql("SELECT pg_database_size(current_database());"))
        if shutil.disk_usage(self.u.root).free < max(size * 2, 1024**3):
            raise ValueError("Not enough free space for the database backup and release")
        if self.mode == "compose":
            self.compose("config", "--quiet")
        elif self.mode == "kubernetes":
            for verb, resource in (("patch", "deployments/tullips-api"), ("create", "jobs"), ("delete", "jobs"), ("list", "pods")):
                if self.kube("auth", "can-i", verb, resource) != "yes":
                    raise ValueError("The updater service account is missing deployment permissions")
        elif self.mode == "coolify":
            for component in COMPONENTS:
                app = self.coolify("applications/" + self.c["applications"][component])
                if app["build_pack"] != "dockerimage":
                    raise ValueError("Migrate this Coolify installation to the official Docker-image layout first")
        else:
            self.check_source(Path(self.c["project_dir"]) / "current")

    def check_source(self, source):
        source = source.resolve()
        checksums = read(source / "source-files.json")
        if not checksums:
            raise ValueError("Custom source installation: update manually or install an official source release")
        actual = {}
        for file in source.rglob("*"):
            relative = file.relative_to(source)
            if any(part in {"node_modules", ".output", ".git", ".tanstack", "bin"} for part in relative.parts) or str(relative) == "source-files.json":
                continue
            if file.is_file(): actual[str(relative)] = sha(file)
        if actual != checksums:
            raise ValueError("Local source changes detected. Update manually to preserve your changes.")

    def snapshot(self):
        if self.mode == "compose":
            config = json.loads(self.compose("config", "--format", "json"))
            services = {}
            for component in COMPONENTS:
                ids = self.compose("ps", "--all", "--quiet", component).splitlines()
                if len(ids) != 1: raise ValueError("Expected one container per Compose application service")
                container = json.loads(run(["docker", "inspect", ids[0]]))[0]
                # Keep immutable local IDs even if a mutable tag moves during a pull.
                services[component] = {"image": container["Image"], "pull_policy": "never"}
            return {"override": {"services": services}, "config": config}
        if self.mode == "kubernetes":
            return {component: json.loads(self.kube("get", "deployment", "tullips-" + component, "-o", "json"))["spec"] for component in COMPONENTS}
        if self.mode == "coolify":
            return {component: {"application": self.coolify("applications/" + self.c["applications"][component]),
                    "envs": self.coolify("applications/" + self.c["applications"][component] + "/envs")} for component in COMPONENTS}
        return {"source": str((Path(self.c["project_dir"]) / "current").resolve())}

    def prepare(self, m):
        if self.mode in {"compose", "coolify"}:
            for image in set(m["images"].values()): run(["docker", "pull", image])
        if self.mode != "systemd": return
        release = Path(self.c["project_dir"]) / "releases" / m["version"]
        if release.exists():
            self.check_source(release)
            return
        archive = self.u.root / "downloads" / m["version"] / "source.tar.gz"
        download(m["download_base"] + "/source.tar.gz", archive)
        if sha(archive) != m["source_sha256"]: raise ValueError("Source archive checksum does not match the signed release")
        release.mkdir(parents=True, mode=0o755)
        release.chmod(0o755)
        release.parent.chmod(0o755)
        try:
            with tarfile.open(archive) as tar:
                if sum(member.size for member in tar.getmembers()) > 1024**3:
                    raise ValueError("Source archive is too large")
                tar.extractall(release, filter="data")
            self.check_source(release)
            (release / "bin").mkdir(exist_ok=True)
            for binary in ("api", "worker", "migrate"):
                run(["go", "build", "-o", release / "bin" / binary, "./cmd/" + binary], cwd=release / "backend")
            run(["npm", "ci"], cwd=release / "web")
            run(["npm", "run", "build"], cwd=release / "web")
        except Exception:
            shutil.rmtree(release)
            raise

    def stop(self):
        if self.mode in {"compose", "coolify"}:
            ids = run(["docker", "ps", "-q", "--filter", "label=" + self.migration_label]).splitlines()
            if ids: run(["docker", "stop", "--time", "330", *ids])
        if self.mode == "compose":
            self.compose("stop", "--timeout", "330", "web", "api", "worker")
            ids = self.compose("ps", "--all", "--quiet", *COMPONENTS).splitlines()
            if ids:
                for container in json.loads(run(["docker", "inspect", *ids])):
                    if container["State"]["Running"]: raise ValueError("An application container did not stop")
        elif self.mode == "kubernetes":
            self.kube("delete", "job", "tullips-migrate-api", "tullips-migrate-web", "--ignore-not-found=true", "--wait=true")
            self.kube("scale", *["deployment/tullips-" + c for c in COMPONENTS], "--replicas=0")
            deadline = time.monotonic() + 360
            while json.loads(self.kube("get", "pods", "-l", "tullips.io/application=true", "-o", "json"))["items"]:
                if time.monotonic() > deadline: raise ValueError("Kubernetes application pods did not stop")
                time.sleep(2)
        elif self.mode == "coolify":
            for component in COMPONENTS:
                uuid = self.c["applications"][component]
                # Coolify manages the resource; allow its active containers to drain first.
                ids = run(["docker", "ps", "-q", "--filter", "label=coolify.applicationId=" + str(self.coolify("applications/" + uuid)["id"])]).splitlines()
                if ids: run(["docker", "stop", "--time", "330", *ids])
                self.coolify("applications/" + uuid + "/stop?docker_cleanup=false", "POST")
            deadline = time.monotonic() + 360
            while time.monotonic() < deadline:
                if all(str(self.coolify("applications/" + self.c["applications"][c]).get("status", "")).startswith("exited") for c in COMPONENTS): return
                time.sleep(3)
            raise ValueError("Coolify applications did not stop before the timeout")
        else:
            run(["systemctl", "stop", "tullips-migrate-*.service"], timeout=400)
            run(["systemctl", "stop", *["tullips-" + c for c in COMPONENTS]], timeout=400)
            for component in COMPONENTS:
                state = run(["systemctl", "show", "-p", "ActiveState", "--value", "tullips-" + component])
                if state not in {"inactive", "failed"}: raise ValueError("A systemd application service did not stop")

    def migrate(self, m):
        if self.mode == "compose":
            self.write_compose(m)
            self.compose("run", "--rm", "--label", self.migration_label, "--no-deps", "api", "/app/migrate")
            self.compose("run", "--rm", "--label", self.migration_label, "--no-deps", "web", "npm", "run", "auth:migrate")
        elif self.mode == "coolify":
            # The migration containers use the same saved environment as the managed apps.
            for image, command in ((m["images"]["backend"], ["/app/migrate"]), (m["images"]["web"], ["npm", "run", "auth:migrate"])):
                run(["docker", "run", "--rm", "--label", self.migration_label, "--network", self.c.get("docker_network", "host"), "--env-file", self.c["env_file"],
                     "-e", "RELEASE_VERSION=" + m["version"], image, *command])
        elif self.mode == "kubernetes":
            for component, command in (("api", ["/app/migrate"]), ("web", ["npm", "run", "auth:migrate"])):
                spec = self.u.state()["previous"][component]["template"]["spec"]
                spec = json.loads(json.dumps(spec))
                container = spec["containers"][0]
                container["image"] = m["images"]["web" if component == "web" else "backend"]
                container["command"] = command
                for key in ("args", "livenessProbe", "readinessProbe", "startupProbe", "ports"):
                    container.pop(key, None)
                spec["restartPolicy"] = "Never"
                name = "tullips-migrate-" + component
                self.kube("delete", "job", name, "--ignore-not-found=true", "--wait=true")
                job = {"apiVersion": "batch/v1", "kind": "Job", "metadata": {"name": name}, "spec": {"backoffLimit": 0, "activeDeadlineSeconds": 600, "template": {"spec": spec}}}
                self.kube("create", "-f", "-", input=json.dumps(job).encode())
                self.kube("wait", "--for=condition=complete", "job/" + name, "--timeout=610s")
        else:
            release = Path(self.c["project_dir"]) / "releases" / m["version"]
            for name, command, cwd in (("api", [release / "bin/migrate"], release), ("web", [shutil.which("npm"), "run", "auth:migrate"], release / "web")):
                run(["systemd-run", "--unit=tullips-migrate-" + name, "--wait", "--collect", "--service-type=exec",
                     "--property=EnvironmentFile=" + self.c["env_file"], "--property=WorkingDirectory=" + str(cwd),
                     "--setenv=RELEASE_VERSION=" + m["version"], *command])

    def write_compose(self, m):
        atomic(self.u.root / "release.compose.json", {"services": {c: {"image": m["images"]["web" if c in ("web", "auth-migrate") else "backend"], "pull_policy": "never"} for c in (*COMPONENTS, "migrate", "auth-migrate")}})

    def deploy(self, m):
        if self.mode == "compose":
            self.write_compose(m)
            self.compose("up", "-d", "--no-build", "--no-deps", "--wait", "--wait-timeout", "180", *COMPONENTS)
        elif self.mode == "kubernetes":
            for c in COMPONENTS:
                image = m["images"]["web" if c == "web" else "backend"]
                self.kube("set", "image", "deployment/tullips-" + c, c + "=" + image)
                self.kube("scale", "deployment/tullips-" + c, "--replicas=" + str(self.u.state()["previous"][c].get("replicas", 1)))
                self.kube("rollout", "status", "deployment/tullips-" + c, "--timeout=180s")
        elif self.mode == "coolify":
            for c in COMPONENTS:
                image = m["images"]["web" if c == "web" else "backend"]
                name, digest = image.split("@sha256:")
                self.coolify("applications/" + self.c["applications"][c], "PATCH", {"docker_registry_image_name": name, "docker_registry_image_tag": "sha256-" + digest})
                self.coolify_deploy(self.c["applications"][c])
        else:
            self.switch_source(Path(self.c["project_dir"]) / "releases" / m["version"])

    def coolify_deploy(self, uuid):
        result = self.coolify("deploy", "POST", {"uuid": uuid, "force": False})
        deployments = result.get("deployments", [])
        if not deployments: raise ValueError("Coolify did not return a deployment ID")
        for deployment in deployments:
            deadline = time.monotonic() + 600
            while time.monotonic() < deadline:
                status = self.coolify("deployments/" + deployment["deployment_uuid"]).get("status")
                if status == "finished": break
                if status in {"failed", "cancelled"}: raise ValueError("Coolify deployment failed")
                time.sleep(3)
            else: raise ValueError("Coolify deployment timed out")

    def switch_source(self, source):
        root = Path(self.c["project_dir"])
        temporary = root / "current.next"
        temporary.unlink(missing_ok=True)
        temporary.symlink_to(source)
        os.replace(temporary, root / "current")
        run(["systemctl", "start", *["tullips-" + c for c in COMPONENTS]])

    def restore(self, previous):
        if self.mode == "compose":
            atomic(self.u.root / "release.compose.json", previous["override"])
            self.compose("up", "-d", "--no-build", "--no-deps", "--wait", "--wait-timeout", "180", *COMPONENTS)
        elif self.mode == "kubernetes":
            for c in COMPONENTS:
                self.kube("patch", "deployment", "tullips-" + c, "--type=merge", "-p", json.dumps({"spec": previous[c]}))
                self.kube("rollout", "status", "deployment/tullips-" + c, "--timeout=180s")
        elif self.mode == "coolify":
            for c in COMPONENTS:
                app = previous[c]["application"]
                self.coolify("applications/" + self.c["applications"][c], "PATCH", {k: app[k] for k in ("docker_registry_image_name", "docker_registry_image_tag")})
                self.coolify_deploy(self.c["applications"][c])
        else:
            self.switch_source(Path(previous["source"]))

    def healthy(self):
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            try:
                for c in COMPONENTS:
                    fetch(self.c[c + "_url"], limit=65536)
                if self.u.sql('SELECT count(*) >= 0 FROM "user";') != "t": raise ValueError("Authentication database unavailable")
                return
            except Exception:
                time.sleep(2)
        raise ValueError("Application readiness checks timed out")
