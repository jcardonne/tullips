# Self-hosted updates

Tullips has one update engine with Docker Compose, Coolify, Kubernetes, and Linux/systemd deployment integrations. The updater runs separately from the application, so stopping the application does not stop recovery.

Automatic updates are opt-in. The updater checks GitHub hourly, installs stable patch/minor releases within the installed major, and requires an explicit action for major upgrades. Unsupported upgrade paths and modified source installations are blocked. PostgreSQL and optional third-party services are not upgraded by this mechanism.

## Administrator controls

The first registration on a fresh installation becomes installation administrator, atomically, even if registrations overlap. On an existing database, nobody is automatically promoted. Assign the administrator by email from the CLI. Workspace owners and workspace API keys cannot control installation updates.

Use **Workspace settings → Installation updates** for release information, automatic-update settings, installation, and recovery. The CLI remains available if the web application cannot start.

For Compose, run commands from the installation directory:

```sh
docker compose exec updater python3 /opt/updater/update.py --config "$PWD/.tullips-updater/updater.json" status
docker compose exec updater python3 /opt/updater/update.py --config "$PWD/.tullips-updater/updater.json" check
docker compose exec updater python3 /opt/updater/update.py --config "$PWD/.tullips-updater/updater.json" automatic on
```

Install a specific version returned by `check` with `update VERSION`. Use `automatic off` to disable unattended updates. An update failure exits nonzero on the CLI and is recorded in the UI. Failed versions are not automatically retried.

On Linux/systemd and Coolify, substitute:

```sh
sudo python3 /opt/tullips-updater/update.py --config /opt/tullips/.tullips-updater/updater.json status
```

On Kubernetes:

```sh
kubectl -n tullips exec deploy/tullips-updater -- python3 /opt/updater/update.py --config /etc/tullips/updater.json status
```

## Update lifecycle

1. Acquire an installation-wide file lock and validate the upgrade path, tooling, permissions, and free space.
2. Verify release metadata against GitHub's attestation, trusted repository, release workflow, and tag. Image digests and source checksums come from that verified metadata.
3. Download images or build the new source release before downtime.
4. Enable maintenance mode, drain active work, and stop every application process. Cancel leftover migration jobs before recovery.
5. Write a PostgreSQL custom-format dump and save essential configuration, keys, and the previous deployment. Check dump readability and record file checksums.
6. Apply ordered backend migrations and serialized authentication migrations. Startup checks schema compatibility but does not run migrations.
7. Deploy matching API, web, and worker versions. Verify readiness and authentication database access before clearing maintenance mode.

The backup reader check is not a substitute for a restore drill. A real dump/restore regression runs against a disposable database in CI. Backups remain on local persistent storage, so loss of that storage still requires an operator-managed off-machine backup.

Jobs already making an external request get time to finish during shutdown. Uncertain outbound delivery retains the existing manual-review behavior instead of being resent automatically.

## Backup and recovery

Backups live under the updater's `state_dir/backups`, with owner-only permissions. They include `database.dump`, the environment file, updater configuration, deployment snapshot, and checksums. Treat the whole directory as secret. The default retention is three successful backups. Incomplete backups and backups required by unresolved recovery are not automatically removed.

Each release declares exact previous versions that can safely run against its migrated schema. An empty compatibility list means database restoration requires approval. This declaration must cover partial migration failure as well as the fully migrated schema.

If a release fails and rollback is compatible, the updater restores the previous deployment automatically. Otherwise it stops the application and enters `awaiting_restore`. The CLI `recover` retries safe recovery without authorizing database replacement.

To approve replacement of the database with the pre-update backup:

```sh
# Replace 0.1.0 with the previous version shown by status.
docker compose exec updater python3 /opt/updater/update.py --config "$PWD/.tullips-updater/updater.json" restore --confirm-version 0.1.0
```

Restoration verifies the saved files, stops application and migration processes, replaces the application's public schema, restores the dump, redeploys the previous application, and checks health. Use a dedicated PostgreSQL database for Tullips. The updater never automatically restores the database after a crash. An interrupted restoration stays blocked until explicitly retried.

If the updater process itself is down, run the same command in a one-off container with `docker compose run --rm --no-deps updater --config ... recover`, or run its Python CLI directly on the server. Preserve the state directory and previous images/release directories. Do not prune them during recovery.

## Installation layouts

### Docker Compose

`python3 scripts/setup.py` generates the updater configuration and secrets as part of the normal installation. The Compose updater has the Docker socket and a bind mount of the installation directory. The application containers do not receive the socket. Failed subprocess output is retained only in the owner-readable `state_dir/diagnostics.log`. This file may contain secrets and must not be published. Protect the updater token and restrict Docker access to trusted operators.

The `.env` file sets `COMPOSE_FILE` to include the updater's release override. Keep that override when running Compose commands, or an ordinary restart could select the original local build. Additional custom Compose files must be integrated into the managed configuration before enabling updates. Do not move the directory without updating `TULLIPS_PROJECT_DIR` and the absolute paths in `updater.json`.

### Linux with systemd

Prerequisites: Python 3.12+, GitHub CLI with `gh attestation verify`, Git, Go compatible with `backend/go.mod`, Node.js 22, npm, and PostgreSQL client tools at least as new as the server. Configure a dedicated PostgreSQL database and the required secrets in an environment file. Keep that file outside the source tree.

```sh
sudo python3 scripts/install.py systemd --env-file /etc/tullips.env
```

The installer verifies the published source release, builds it into `/opt/tullips/releases/VERSION`, creates the `tullips` service account and systemd units, runs migrations, and starts application and updater services. The application runs as `tullips`. The updater controls only the named Tullips services through predefined commands.

Updates use separate release directories and an atomic `current` symlink. Modifying a release's source files blocks further automatic updates. Migrations run as named transient systemd units so recovery can stop an orphaned migration.

### Kubernetes

Prerequisites: Python 3.12+, GitHub CLI, kubectl, a namespace-capable installer identity, a default storage class, and a dedicated PostgreSQL database reachable from the cluster. Set `DATABASE_URL` and application secrets in an environment file.

```sh
python3 scripts/install.py kubernetes --env-file /secure/tullips.env --directory ./tullips-install --namespace tullips
```

The installer creates application Deployments/Services, environment Secrets, the updater, namespace-scoped RBAC, and a 20 GiB backup PVC. Choose another size with `--backup-storage`. Database provisioning and your public Ingress belong to normal cluster setup. No database or cloud-provider operator is installed.

Managed deployment names are `tullips-api`, `tullips-web`, and `tullips-worker`. The updater has one replica and a persistent state/backup volume. Update jobs inherit the relevant application's environment and security context. Existing replica counts are restored after an update. If a GitOps controller manages these resources, give it ownership of the same desired versions or it will undo updater changes.

### Coolify

Run the installer on the Linux server hosting the applications. It uses that server's Docker engine for migration containers and graceful draining, and the Coolify API for deployment configuration and status. Install Python 3.12+, GitHub CLI, Docker, and PostgreSQL client tools first.

Supply `DATABASE_URL` reachable from both the server and application containers, application secrets, and `COOLIFY_TOKEN` in the environment file. Use a token with access to the intended Coolify project.

```sh
sudo python3 scripts/install.py coolify \
  --env-file /etc/tullips.env \
  --coolify-url https://coolify.example.com \
  --coolify-project PROJECT_UUID \
  --coolify-server SERVER_UUID
```

The installer creates three Docker-image applications, configures their environment and readiness checks, and installs the independent updater service. Coolify applications reference immutable digests using Coolify's `sha256-...` tag representation. The updater binds to the Docker bridge address. Publish the web application through your normal HTTPS proxy.

Do not enable a second independent deployment automation for the same application. Updates must share one maintenance/backup/migration sequence.

## Existing installations

Adopting the updater is a one-time operator migration. Preserve a verified database backup and `.env` before changing the deployment layout or upgrading from a version that predates the updater.

For the original Compose layout, update the checkout, then:

```sh
python3 scripts/setup.py --migrate
docker compose up -d --build
docker compose exec updater python3 /opt/updater/update.py --config "$PWD/.tullips-updater/updater.json" admin owner@example.com
```

`--migrate` first stops the application and creates a checked adoption backup. If backup fails, it restarts the previous containers and aborts. Secrets are preserved. Migrations adopt existing application tables. Authentication initialization detects existing users and leaves administrator assignment to the CLI.

For existing Coolify Docker-image applications, pass `--applications applications.json` to the installer instead of project/server creation arguments. The JSON object maps `api`, `web`, and `worker` to their application UUIDs. Their environment must match the saved environment file. Existing Compose-as-a-service resources must first move to the three-application layout.

Existing Kubernetes deployments must adopt the documented names, labels, environment, RBAC, and backup volume before the updater can safely manage them. The installer refuses to overwrite an existing managed API deployment. For source installations, migrate service ownership to the supplied systemd units and preserve local changes before adopting verified release directories. Arbitrary custom deployment layouts are not silently rewritten.

After adoption, use the platform's updater CLI `admin EMAIL` to assign an existing account. Automatic updates remain disabled until the operator enables them.

## Publishing a release

1. Run checks and update `VERSION` to the stable semantic version.
2. Review `release-policy.json`. Set `minimum_version` to the oldest supported direct upgrade. List a previous version in `rollback_versions` only after testing that version against both complete and partially applied new migrations.
3. Push a matching `vVERSION` tag. The release workflow runs checks, publishes AMD64/ARM64 images, packages the exact Git source, and attests `release.json` using GitHub OIDC.
4. The workflow publishes the release only after all artifacts and the attestation bundle are uploaded. Ensure the GHCR packages are publicly readable before announcing the release.
5. Run an upgrade and approved restore drill on a disposable installation.

The trusted repository is `jcardonne/tullips`. No signing private key is shipped or configured on installations. Verification is tied to `.github/workflows/release.yml` and the exact release tag. Future changes to deployment layout or updater protocol must declare a new `updater_protocol` and provide manual migration instructions. Protocol 1 does not replace the running updater itself.

## Validation

`python3 -m unittest discover -s updater -p 'test_*.py' -v` checks update ordering, failed backups, incompatible migrations, rollback, interrupted restoration, locking, release validation, source changes, and settings validation.

`UPDATER_TEST_DATABASE_URL` enables the destructive PostgreSQL dump/restore check. Use only a disposable database. Backend migration tests use `TEST_DATABASE_URL` and isolated schemas.

`updater/compose_smoke.py` is a destructive end-to-end drill guarded to run only in a Compose project named `tullips-updater-qa`. It uses locally built images to test orchestration without publishing fake releases. Set `TULLIPS_DISPOSABLE_UPDATE_TEST=yes` inside that project's updater container and supply its config path.

Live acceptance on operator-owned Coolify, Kubernetes, and systemd infrastructure remains a separate deployment check. Command-contract tests alone do not establish compatibility with every platform version, policy, or custom layout.
