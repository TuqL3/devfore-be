# DevForge — infrastructure & deployment

The one document for infrastructure: environments, configuration, CI/CD, the
production box, and the runbook that takes the project from "no server" to
"running, monitored and backed up". It replaces the former `INFRA.md` +
`deploy/DEPLOY.md` pair; both are still in `git log -- INFRA.md deploy/DEPLOY.md`.

**Section numbers §8, §9, §13 and §14 are fixed.** README holds §0–§7 and §10–§12, and
comments in `deploy/up.sh`, `deploy/bootstrap.sh`, `.github/workflows/ci.yml`,
`docker-compose.prod.yml`, `deploy/nginx/devforge.conf`, `.env.prod.example`,
`scripts/backup.sh` and `scripts/edge-routes.check.sh` cite them. Renumber
anything here → `grep -rn 'INFRA.md §' .` and fix every hit.

| I need to…                          | Go to                                      |
| ----------------------------------- | ------------------------------------------ |
| Run the stack on my machine         | §8.1, then README §12                      |
| Know what a variable does           | §8 "Environment variables"                 |
| Cut a release                       | `make release v=vX.Y.Z` — §13.11           |
| Get from zero to production         | §13.4 (the ordered list), then §13.1–§13.7 |
| Roll back                           | §13.8                                      |
| Restore the database                | §13.7                                      |
| Fix a 502, a red Trivy, a full disk | §13.9                                      |
| See what is still open              | §13.0, §13.10                              |
| Name a branch, open a PR            | §8 "Branching"                             |
| Plan a sprint release               | §13.11                                     |
| Production has a bug                | §13.12                                     |
| Know why it is built this way       | §8.0, §14                                  |

---

## 8. Environments, configuration, CI/CD

### 8.0 Design principles

Six rules nearly every team that ships software follows, and where each one
lives here. Everything else in this document is a consequence of them.

| # | Principle                                   | Here                                                                                      |
| - | ------------------------------------------- | ----------------------------------------------------------------------------------------- |
| 1 | **Build once, promote everywhere**          | images built only on a push to `develop`; a release re-tags the image dev ran (§8 "CI/CD") |
| 2 | **Same artifact, different config**         | one image per service for dev and prod; only `.env` differs (§8.1)                        |
| 3 | **Production is isolated**                  | its own Linode, its own SSH key, secrets only in the `production` environment, approval to deploy |
| 4 | **No real data outside production**         | dev runs demo seed only; production is never copied down (§8 "Dev environment")          |
| 5 | **Non-production is private**               | `dev.<domain>` behind Cloudflare Access                                                   |
| 6 | **Mitigate first, fix second**              | a bad release is rolled back in seconds; the fix goes through the normal flow (§13.12)    |

Where DevForge sits among real-world setups:

| Team size        | Typical environments                                  | Typical infrastructure                         |
| ---------------- | ----------------------------------------------------- | ---------------------------------------------- |
| **1–5 devs**     | local → staging/dev → prod                            | PaaS, or 1–2 VPS + Compose — **this project**  |
| 10–50 devs       | local → preview per PR → staging → prod               | Kubernetes, Terraform, GitOps (Argo CD)        |
| Enterprise       | dev → QA → staging/UAT → prod (+ perf, DR)            | one cloud account per environment, IaC         |

Not adopted, on purpose, until the team or the service count grows:
Kubernetes, Terraform, per-PR preview environments, feature flags, a monorepo.

### 8.1 Three environments

One `Dockerfile` per repo, used everywhere. The same image runs on dev and in
production; only the env injected at run time differs. There is no
`Dockerfile.prod`. Dev and production also share one compose setup and one
`up.sh` — they differ in their `.env` and nothing else.

|              | local                                   | dev                                                      | production                                              |
| ------------ | --------------------------------------- | -------------------------------------------------------- | ------------------------------------------------------- |
| Purpose      | write code                              | integrate and test everything merged to `develop`        | serve users                                             |
| Where        | laptop                                  | own Linode, Shared 2 GB, Singapore                       | own Linode, Shared 4 vCPU / 8 GB, Singapore (§9.1)      |
| URL          | `localhost`                             | `https://dev.<domain>` behind Cloudflare Access          | `https://<domain>`                                      |
| Runs with    | `docker compose --profile dev` + `air` + Vite | base + prod compose via `up.sh`                    | base + prod compose via `up.sh`                         |
| Images       | built locally (`make lab-images`)       | GHCR `:develop` (= newest `dev-<sha>`)                   | GHCR `:vX.Y.Z` = a `dev-<sha>` that ran on dev          |
| Deployed by  | hot reload                              | every push to `develop`, either repo, automatically      | `make release` → tag → approval → CI                    |
| Rollback     | —                                       | none, fix forward                                        | automatic on unhealthy; by hand §13.8                   |
| Config       | `.env` from `.env.example`              | `.env` from the DEV block of `.env.prod.example`         | `.env` from `.env.prod.example`                         |
| Data         | demo seed                               | demo seed — **never a copy of production**               | real                                                    |
| Mail         | Mailpit, `http://localhost:8025`        | Mailpit, through an ssh tunnel                           | Resend or Brevo                                         |
| AI           | optional                                | off (`AI_DAILY_LIMIT=0`)                                 | haiku, 3/day (§9.6)                                     |
| Lab seats    | 40 (code default)                       | 3                                                        | 16 (§9.4)                                               |
| Backup       | —                                       | none, on purpose                                         | Linode Backups + nightly `pg_dump` → R2                 |
| TLS / edge   | none                                    | nginx + Origin cert (`*.<domain>`), orange cloud         | nginx + Origin cert, orange cloud (§9.5)                |
| CI secrets   | —                                       | GitHub Environment `development` (both repos)            | GitHub Environment `production` (be only, reviewer)     |

Local quick start (full walkthrough in README §12):

```bash
# devforge-be
cp .env.example .env
make migrate        # postgres, redis, mailpit, docker-proxy; migrations; demo seed; 4 lab images
make air            # API with hot reload on $PORT

# devforge-fe
cp .env.example .env
npm ci && npm run dev
```

- **Ports come in pairs.** Both `.env.example` files say 8080. A working copy may
  run 8888 in both `.env` files instead. Either works as long as
  `devforge-be/.env` `PORT` and `devforge-fe/.env` `VITE_API_URL` match.
- **Labs need the four local images.** If `docker images 'devforge/*'` is empty,
  starting any container lab fails. Run `make lab-images` (it is part of
  `make migrate`, not `make up`).
- Every port in `docker-compose.yml` is bound to `127.0.0.1`. Keep it that way
  (§9.9 #2). Reach them from another machine through an SSH tunnel.

Make targets in `devforge-be`:

| Target                     | Does                                                                         |
| -------------------------- | ---------------------------------------------------------------------------- |
| `up` / `down`              | start / stop the dev compose stack                                           |
| `migrate`                  | `up` + migrations + demo seed + lab images — local only, the seed adds demo accounts |
| `migrate-schema`           | migrations only, no seed, no images                                          |
| `migrate-down`             | roll back one migration (local only — never during a prod incident, §8 "Migrations") |
| `migrate-new name=x`       | create the next numbered migration pair                                      |
| `admin email= password=`   | create or promote an admin via `go run` — local only, see §13.5 for prod     |
| `lab-images`               | build `devforge/{linux,git,docker,net}:latest`                               |
| `test` / `cover`           | `go test ./...` / coverage as CI measures it (floor 16.5%)                   |
| `check-seed`               | run every seed `check_script` inside a real lab container                    |
| `check-sim`                | grade every simulated task: wrong pipeline must fail, right one must pass    |
| `check-edge`               | 14 route checks against the real `deploy/nginx/devforge.conf` (needs Docker) |
| `check-release-guard`      | pin which tag names may become a release                                     |
| `release v=vX.Y.Z`         | tag both repos and push — §8 "CI/CD"                                         |

### Environment variables

Source of truth for values: `.env.example` (local) and `.env.prod.example`
(production, with a `STAGING` block at the bottom). Code defaults live in
`internal/config/config.go`. **Secret** = only ever in the `.env` on the box.

| Variable                                                  | Local                                   | Production                                                        | Notes                                                                                           |
| --------------------------------------------------------- | --------------------------------------- | ----------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `DOMAIN`                                                  | —                                       | `example.com`                                                     | Read only by `deploy/up.sh`, which refuses to run without it                                    |
| `IMAGE_REPO`                                              | —                                       | `ghcr.io/<owner>`, **lowercase**                                  | GHCR rejects uppercase                                                                          |
| `IMAGE_TAG`                                               | —                                       | **not in `.env`** — `develop` on dev, `vX.Y.Z` in production      | `up.sh` writes the healthy one to `.image-tag`, the commits to `.deployed`                      |
| `APP_ENV` / `PORT` / `LOG_LEVEL`                          | `development` / `8080` / `debug`        | `production` / `8080` / `info`                                    |                                                                                                 |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_NAME`             | `localhost` / `5432` / `devforge` ×2    | `postgres` / `5432` / `devforge` ×2                               |                                                                                                 |
| `DB_PASSWORD`                                             | fake                                    | `openssl rand -base64 24`                                         | **Secret**                                                                                      |
| `DATABASE_URL`                                            | spelled out                             | interpolated from `DB_*`                                          | Used by `migrate` and the Makefile only, not by the Go binary. `sslmode=disable` — see below     |
| `REDIS_ADDR` / `REDIS_PASSWORD` / `REDIS_DB`              | `localhost:6379` / empty / `0`          | `redis:6379` / empty / `0`                                        | Refresh sessions only; losing Redis signs everyone out, nothing else                            |
| `JWT_SECRET`                                              | fake                                    | `openssl rand -base64 32`                                         | **Secret.** Changing it signs everyone out                                                      |
| `ACCESS_TTL` / `REFRESH_TTL`                              | `15m` / `168h`                          | same                                                              |                                                                                                 |
| `COOKIE_DOMAIN`                                           | empty                                   | **empty**                                                         | Host-only cookie; correct for one origin                                                        |
| `CORS_ORIGINS`                                            | `http://localhost:5173`                 | `https://<domain>`                                                | Also the WebSocket `Origin` allow-list (terminal, chat). Wrong → terminal never connects         |
| `FRONTEND_URL`                                            | `http://localhost:5173`                 | `https://<domain>`                                                |                                                                                                 |
| `PUBLIC_URL`                                              | `http://localhost:8080`                 | `https://<domain>` — **no `/api`**                                | Code appends `/uploads/…` and `/api/shared-drills/…` itself                                     |
| `UPLOAD_DIR`                                              | `./uploads`                             | `/uploads`                                                        | Named volume `uploads` in `docker-compose.prod.yml`; distroless nonroot cannot write elsewhere  |
| `TRUSTED_PROXIES`                                         | `127.0.0.1,::1`                         | `127.0.0.1,::1,172.16.0.0/12`                                     | **Mandatory.** Half of §9.5; the other half is in nginx. Never `0.0.0.0/0`                      |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`               | `.env`                                  | `.env` on the box                                                 | **Secret**                                                                                      |
| `GOOGLE_REDIRECT_URL`                                     | `http://localhost:8080/api/auth/google/callback` | `https://<domain>/api/auth/google/callback`              | Must match Google Console character for character                                               |
| `SMTP_HOST` / `SMTP_PORT`                                 | Mailpit `localhost` / `1025`            | Resend or Brevo / `587`                                           | **Fails silently** — see below                                                                  |
| `SMTP_USER` / `SMTP_PASSWORD`                             | empty                                   | required                                                          | **Secret**                                                                                      |
| `MAIL_FROM`                                               | `no-reply@devforge.local`               | `no-reply@<domain>`                                               |                                                                                                 |
| `VERIFY_CODE_TTL` / `RESET_TOKEN_TTL` / `RESEND_COOLDOWN` | `10m` / `1h` / `60s`                    | same                                                              |                                                                                                 |
| `LAB_DOCKER_HOST`                                         | `tcp://127.0.0.1:2375`                  | `tcp://docker-proxy:2375` (forced by `docker-compose.prod.yml`)   | Never `DOCKER_HOST` — the docker CLI reads that name too                                        |
| `LAB_SESSION_TTL`                                         | `60m`                                   | `20m`                                                             | §9.4                                                                                            |
| `MAX_CONTAINERS`                                          | `40` (code default, not in `.env.example`) | `16`                                                           | Seat cap **per database**, §9.4                                                                 |
| `PUBLIC_RATE_LIMIT`                                       | `60`                                    | `60`                                                              | Per client IP per minute on public share routes — only per-IP if §9.5 is right                  |
| `OPENROUTER_API_KEY`                                      | empty                                   | **empty on first deploy**                                         | **Secret.** Empty key or model → that one endpoint answers 503, server runs normally (§9.6)    |
| `OPENROUTER_MODEL`                                        | `anthropic/claude-opus-5`               | `anthropic/claude-haiku-4-5`                                      | Must support `response_format`                                                                  |
| `AI_DAILY_LIMIT`                                          | `10`                                    | `3`                                                               | Hard cap per user per UTC day                                                                   |
| `R2_BUCKET` / `R2_ENDPOINT` / `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` | —                       | required                                                          | **Secret.** `backup.sh` exits loudly until filled                                               |
| `COMPOSE_PROFILES`                                        | `dev`                                   | **absent**                                                        | Gates Mailpit. A mail catcher on a public box is a mailbox anyone can read                     |
| `VITE_API_URL` (FE, build time)                           | `http://localhost:8080`                 | **empty, pinned** by `ENV VITE_API_URL=""` in `devforge-fe/Dockerfile` | Not a per-environment variable — §9.1.1                                                  |
| `VITE_UMAMI_SRC` / `VITE_UMAMI_ID` (FE)                   | empty                                   | empty                                                             | Analytics off until set                                                                         |

⚠️ **`sslmode=disable` is correct in production, and only there.** The
`postgres:16-alpine` container has no TLS configured; `require` kills `migrate`
on the first deploy with `SSL is not enabled on the server`. What makes it safe
is scope: the connection never leaves the compose bridge, and Postgres publishes
only on `127.0.0.1`.

⚠️ **SMTP is where production breaks silently.** Locally Mailpit swallows every
mail, so nobody notices it missing. In production, verification codes and
password resets are the only mail sent; without `SMTP_HOST` new users never get
a code and nothing errors on the server.

⚠️ **The production `.env` exists in exactly one place.** CD does not ship it
(Actions holds only `SSH_HOST`, `SSH_USER`, `SSH_KEY`, and optionally
`SONAR_TOKEN`), and `backup.sh` dumps the database, not config. Keep a copy in a
password manager. Rebuilding the box without it means new keys everywhere, and
everyone is signed out with the old `JWT_SECRET`.

### Domain & routing

One origin, routed by path. `deploy/nginx/devforge.conf` implements this, and
`make check-edge` asks the real config 14 questions to prove it.

```
https://<domain>/
    /api/*      REST
    /ws/*       WebSocket (wss) — proxy_buffering off, read timeout 3600s
    /uploads/*  cover images + avatars — NOT under /api
    /r/:id · /war-room/day/:date   → crawlers only: rewritten to the preview endpoint
    /healthz    /readyz
    everything else → SPA (image devforge-web)
```

- **`/uploads/*` needs its own route.** `cmd/server/router.go` mounts
  `r.Static("/uploads", …)` on the root router, outside `/api`. Without the
  route every image gets `index.html` with **200** — a broken image, not a 404
  anyone can grep for.
- **The crawler blocks must stay ahead of the catch-all.** Facebook, Zalo and
  Slack run no JavaScript, so shared links render blank without them. nginx
  tries regex locations in written order and they always beat the `location /`
  prefix. Keep both regexes (`^[A-Za-z0-9_-]+$` for ids, `^\d{4}-\d{2}-\d{2}$`
  for dates) — they keep junk out of an image-generating endpoint. The date
  regex is quoted because nginx reads `{4}` as a block opener.
- **Upstreams go through variables + `resolver 127.0.0.11`.** nginx otherwise
  resolves `api` once at start, and the new `api` container after a deploy gets
  a new bridge address → 502.
- `COOKIE_DOMAIN` stays empty: same origin, host-only cookie, `SameSite=Lax`
  never comes into play.
- `client_max_body_size 4m` caps uploads at the edge.

When the domain is chosen:

- `FRONTEND_URL`, `PUBLIC_URL` and `CORS_ORIGINS` are all `https://<domain>`.
- Google Console redirect URI: `https://<domain>/api/auth/google/callback`.
- DNS: one `A` record for `<domain>`, orange cloud. There is no `api.` record.

The price of one domain: FE and API share one way in, so one bad edge config
takes both down.

### Healthcheck and readiness

| Endpoint                         | Means                                                  | Use for                              |
| -------------------------------- | ------------------------------------------------------ | ------------------------------------ |
| `/healthz`                       | process is alive, always 200                           | nothing that decides anything        |
| `/readyz`                        | pings the database (2 s timeout), 503 when it cannot   | rollback condition, uptime monitor   |
| `http://127.0.0.1:81/nginx-alive` (inside nginx) | the edge is serving                    | nginx container healthcheck          |

The `api` image is distroless (no shell, no `wget`), so its compose healthcheck
is the binary probing itself: `["CMD", "/server", "healthcheck"]` →
`cmd/server/healthcheck.go` → `GET /readyz` over loopback. Interval 10 s,
timeout 5 s, 5 retries, 20 s start period. `docker compose up -d --wait
--wait-timeout 120` in `up.sh` blocks on exactly this, which is what makes
automatic rollback trustworthy. Without the nginx healthcheck, `--wait` would
count a crash-looping edge as running.

### Migrations

Rollback moves the image back, never the schema. Therefore **every migration
must be backward compatible**: the previous binary has to run against the new
schema. Renaming `user_name` → `username` takes three releases:

```
Release 1:  ADD COLUMN username; backfill; code reads user_name, writes BOTH
Release 2:  code reads username, writes BOTH
Release 3:  DROP COLUMN user_name; code uses username only
```

Rules `up.sh` relies on:

1. Migrations run **before** the new `api` starts (`migrate/migrate:v4.18.1` on
   the compose network).
2. The rollback path **does not run `migrate` again** — the failed release may
   have applied a migration whose file the old checkout does not have, and
   `migrate up` would die before the old image is back.
3. **No down migration during an incident.** That is how a rollback becomes data
   loss.

### Branching

Long-lived branches, in both repos:

| Branch    | Role                                                    | Receives                          |
| --------- | ------------------------------------------------------- | --------------------------------- |
| `develop` | default branch; integration; deploys to dev on every push | PRs from `feat/*` and `fix/*`, Dependabot |
| `master`  | what releases are cut from; never deployed by itself    | PR `develop → master` before a release |

Short-lived branches, always from `develop` and back into it by PR:

| Prefix        | For                  | Example                 |
| ------------- | -------------------- | ----------------------- |
| `feat/<name>` | a feature            | `feat/lab-reconnect`    |
| `fix/<name>`  | a bug, any severity  | `fix/upload-200-broken` |

- **No `hotfix/*` branches** (decided 2026-10-07, §13.11). Every bug goes
  through `develop` and the next release.
- **Merge `develop → master` with a merge commit**, not squash or rebase:
  `make release` tags the commits dev is serving, and those must stay
  ancestors of `master`.
- **Never merge unfinished work into `develop`.** Every release ships all of
  `develop`; half-done work stays on its branch.
- A change touching both repos is two PRs, merged into both `develop` branches;
  each repo deploys its own push to dev.

### CI/CD — two repos, meeting at the registry

**Branches: `develop` integrates, `master` is what releases are cut from.** Both
repos' default branch is `develop`, so new PRs and Dependabot target it.

**Build once, promote.** Images are built exactly once — on a push to
`develop` — scanned, and run on dev. A release does not build: it gives the
image dev ran a version name, so production serves the same bits that were
tested.

```
pull_request          → check only
push develop          → check → build + scan ONCE → dev-<sha> + :develop → deploy dev
push master           → check only
make release v=vX.Y.Z → tags the commits dev is serving, in BOTH repos
push tag v* (BOTH)    → check → re-scan dev-<sha> → copy to :vX.Y.Z → approval → deploy prod
```

How a commit is followed from dev to production:

1. A push to `develop` builds `devforge-*:dev-<full sha>` (never moves) and
   `:develop` (moves), with the label `org.opencontainers.image.revision=<sha>`.
2. Dev deploys `IMAGE_TAG=develop`. After the stack is healthy, `up.sh` writes
   `.deployed` on the box: `be=<sha>` and `fe=<sha>`, read from the `api` and
   `web` image labels. It deletes the file before every deploy, so a broken
   deploy leaves nothing a release could name.
3. `make release v=vX.Y.Z` reads `.deployed` over ssh, refuses unless both
   commits are on `master`, and tags **those commits** — not master's HEAD — in
   both repos (`scripts/release.sh`, pinned by `make check-release`).
4. Each repo's `promote` job pulls `dev-<sha>` for its tagged commit, re-scans it
   (CVEs published since the dev build count), and pushes the same image as
   `:vX.Y.Z`. A commit that never built on `develop` has no `dev-<sha>` and is
   refused.
5. be's `deploy` waits for `devforge-web:vX.Y.Z`, waits for approval in the
   `production` environment, then ssh → `up.sh`.

`devforge-be` (`.github/workflows/ci.yml`):

```
be          gofmt -l → golangci-lint v2.12.2 → go build → go test -coverprofile
            → fail if coverage < 16.5% (a ratchet: raise it, never lower it)
sonar       needs be; skips itself without SONAR_TOKEN
secrets     gitleaks → trivy fs (go.sum) → trivy on the nginx tag in docker-compose.prod.yml

push develop only:
images-dev  build api + 4 labs --load (labelled) → trivy each → push dev-<sha> + :develop
deploy-dev  env `development`; skips without SSH_HOST
            ssh: checkout --detach <this sha>; IMAGE_TAG=develop DEPLOY_WAIT=900 up.sh

tag v* only:
promote     release-guard.sh → tag must be on master → pull dev-<sha> (missing → refuse)
            → trivy api + 4 labs → push the same image as :vX.Y.Z
deploy      env `production` (required reviewer); wait ≤ 5 min for devforge-web:<tag>
            → skip without SSH_HOST → release-guard.sh
            → ssh (timeout 1800, ConnectTimeout=10, BatchMode=yes):
                git fetch --tags --force && git checkout --force --detach <tag>
                IMAGE_TAG=<tag> ./deploy/up.sh
```

`devforge-fe`, same shape:

```
fe          oxlint → tsc → npm run check (8 assert files) → build
sonar       skips itself without SONAR_TOKEN
secrets     gitleaks → trivy fs (package-lock.json)
image-dev   push develop: build --load (labelled) → trivy → push dev-<sha> + :develop
deploy-dev  push develop, env `development`: ssh, be clone → origin/develop, same up.sh call
promote     tag v*: guard → tag on master → pull dev-<sha> → trivy → push :vX.Y.Z
            (no production deploy here — only be's deploy job waits for both repos)
```

What `deploy/up.sh` does on either box:

```
flock on the checkout dir — refuse a concurrent deploy, or wait DEPLOY_WAIT seconds (dev)
→ source .env; require DOMAIN, DATABASE_URL, IMAGE_REPO, IMAGE_TAG
→ delete .deployed
→ pull api + web; pull 4 lab images and `docker tag` them to devforge/<name>:latest
→ up -d --wait postgres redis → migrate up
→ up -d --wait --wait-timeout 120 (whole stack)
   healthy   → write .image-tag and .deployed, prune dangling images, exit 0
   unhealthy → print api logs, then
               no .image-tag (first deploy)        → leave the stack, exit 1
               previous tag == this tag (:develop) → nothing to roll back to, exit 1
               otherwise → pull + start the previous tag WITHOUT migrating, exit 1
```

Rules:

- **One release = one version name on both repos, tagging the commits dev
  serves.** `up.sh` pulls `api`, `web` and 4 lab images with one `IMAGE_TAG`, and
  only a name both repos share can address all six. `make release` writes both
  tags; never tag by hand.
- **Merge `develop` into `master` before releasing.** The dev commits must be
  ancestors of `master` (a merge commit is fine — the tag goes on the dev commit,
  not on the merge).
- **Never move a released tag.** Git allows `-f` and GHCR allows overwrite; doing
  either makes the release unreproducible and rollback a guess. Cut the next
  patch version.
- **Tag names reach a remote shell.** `tags: ["v*"]` filters names, not shell
  metacharacters. `scripts/release-guard.sh` accepts only `v[0-9]…` spelled with
  `[A-Za-z0-9._-]`; it runs in `make release`, `promote` and `deploy`, and
  `make check-release-guard` pins it.
- **Dev rolls forward only.** `:develop` moves, so a failed dev deploy is fixed
  by the next push. Production keeps tag rollback.
- **Two repos deploy to dev, one deploys to production.** Dev has no cross-repo
  gate, so each repo deploys its own pushes and the box serialises them
  (`DEPLOY_WAIT`). Production needs both images, so only be's job deploys it.
- **Lab images are retagged on the box** to `devforge/<name>:latest`, the name
  the `lab_images` table stores. Renaming in the database would be a migration.
- **The flock is on the directory, not a lock file.** A file under `/run/lock`
  created by one `sudo ./deploy/up.sh` stays root-owned and breaks every later
  deploy with `Permission denied`.
- **Never add `git clean` to the deploy path.** The checkout holds the two
  untracked files with no copy anywhere: `.env` and `deploy/nginx/certs/*`.

The cost of promoting: nothing reaches production that is not running on dev
right now, so a release needs dev healthy and `develop` frozen while it is
tested (§13.11). A hotfix goes through `develop` like everything else.

### Security scanning and code quality

| Gate            | Where                          | Fails on                                                        |
| --------------- | ------------------------------ | --------------------------------------------------------------- |
| gitleaks        | `secrets` job, both repos      | any secret in history (`.gitleaks.toml` for allow-lists)        |
| Trivy fs        | `secrets` job, both repos      | fixable HIGH/CRITICAL in `go.sum` / `package-lock.json`         |
| Trivy edge      | `secrets` job, be              | fixable HIGH/CRITICAL in the nginx image production pulls       |
| Trivy image     | `images-dev` / `image-dev`; re-run in `promote` | same, on api, web and the 4 lab images — **before** push, and again before a release |
| golangci-lint   | `be` job                       | `.golangci.yml` (v2, standard + bodyclose, rowserrcheck, sqlclosecheck, errorlint); replaces `go vet` |
| Coverage floor  | `be` job                       | total < 16.5%                                                   |
| SonarQube Cloud | `sonar` job, both repos        | Quality Gate red (`sonar.qualitygate.wait=true`) — skipped until `SONAR_TOKEN` exists |

Trivy flags everywhere: `--severity HIGH,CRITICAL --ignore-unfixed --exit-code 1`.
`--ignore-unfixed` is not optional — without it CI goes red on CVEs nobody can
patch, and someone deletes the job three weeks later.

`.trivyignore` is empty on purpose. Every line added must carry `exp:<date>` and
a reason; an ignore with no expiry disables the scanner for that CVE forever.

The lab images are the real attack surface: they carry a shell and coreutils on
purpose and strangers type into them. `devforge-api` is distroless nonroot.

Dependabot (both repos, monthly) bumps Dockerfile base images and Actions. It
does **not** read compose files — the Trivy edge scan is what watches the nginx
pin in `docker-compose.prod.yml`.

SonarQube is never self-hosted on the box: it is a JVM app needing 2–4 GB, and
§9.4 says CPU is the ceiling and lab seats are the product. `sonar.projectKey`
(`TuqL3_<repo>`) and `sonar.organization` (`tuql3`) in
`sonar-project.properties` are guesses from Cloud's naming convention; check them
against the UI on the first real run.

### Dev environment

A second, small Linode that runs exactly what is on `develop`, for testing
before a release. Same compose files, same `up.sh`, same images as production;
only its `.env` differs (the DEV block at the end of `.env.prod.example`).

**Its own box, never a second stack on the production box:**

- Both stacks want ports 80/443; sharing would mean pulling nginx out of the
  compose files into a shared edge.
- Unreleased code with access to the Docker daemon would run on the box that
  holds production's data.
- Dev's labs would take CPU from production's students (§9.4).
- **The seat-count trap.** `internal/labs/adapter/repo/session.go` counts seats
  from its own database
  (`SELECT count(*) FROM lab_sessions WHERE status = 'running' AND container_id <> ''`),
  not from the Docker daemon. Two stacks on one daemon each believe they have
  room and together start twice `MAX_CONTAINERS`.

**Keep it private.** A Cloudflare Access application on `dev.<domain>` lets only
the team in (free up to 50 users). Strangers and search engines never see
unreleased features or demo accounts. Side effect: link previews (`/r/:id` in
Zalo/Slack) cannot be tested on dev — crawlers are kept out too.

**Data is demo data.** Seed it once after the first deploy (§13.5). Never copy
production's database to dev: it holds real users' personal data.

**Mail goes to Mailpit** (`COMPOSE_PROFILES=dev`, `SMTP_HOST=mailpit`). Its
inbox is bound to `127.0.0.1` on the box; read it through a tunnel, never by
binding it to `0.0.0.0` — it holds verification codes and reset links:

```bash
ssh -L 8025:127.0.0.1:8025 devforge@<dev host>   # then open localhost:8025
```

**What dev catches and what it does not.** Same nginx config, Origin cert,
orange cloud, `TRUSTED_PROXIES`, OAuth flow and migrations as production, so
edge and proxy bugs show on dev first. What only production has — real
traffic, real data, the SMTP provider, the AI key — still only shows there.

---

## 9. Production infrastructure

### 9.0 Overview

```
                Cloudflare — DNS · TLS · WAF · DDoS
                cache ONLY static assets, never /api /ws /uploads (§9.5)
                             │ HTTPS 443, Full (strict)
                             ▼
   ┌──────────────── VPS prod — Linode 4 shared vCPU / 8 GB, Singapore ────────────────┐
   │  nginx :80/:443  (only published ports)   real_ip from CF-Connecting-IP            │
   │    ├──► web    SPA static files (nginx, image devforge-web)                         │
   │    └──► api    Go, distroless nonroot, healthcheck → /readyz                        │
   │           ├── postgres  volume pgdata,  127.0.0.1:5432                              │
   │           ├── redis     no persistence, 127.0.0.1:6379                              │
   │           ├── volume uploads → /uploads                                             │
   │           └── docker-socket-proxy  CONTAINERS · POST · EXEC only                    │
   │                 └── lab containers × MAX_CONTAINERS                                 │
   │                     network none · cap-drop ALL · no-new-privileges · read-only     │
   │                     rootfs · 512 MB · 0.5 CPU · 256 pids                            │
   └────────────────────────────────────────────────────────────────────────────────────┘
          │ cron 03:15                                   ▲
          ▼                                              │ every 5 min
   pg_dump → Cloudflare R2 (30-day lifecycle)     UptimeRobot / BetterStack → /readyz
                                                  (outside the box, always — §9.9 #1)
```

The dev box (§8 "Dev environment") is the same picture on a Linode 2 GB at
`dev.<domain>`, behind Cloudflare Access, with Mailpit, demo data, 3 lab seats,
no backups and no uptime monitor.

Three things make this more than a generic "VPS + Compose" setup, and all three
come from the product:

1. **The app starts containers.** Hence `docker-socket-proxy`, the seat cap, the
   reaper, and why no PaaS (Vercel, Render, Railway, Fly.io) can host the API.
2. **No worker or queue.** The reaper is a goroutine; email is sent
   synchronously. Do not add a worker service for work that does not exist.
3. **FE on the same box, same origin** — a choice, §9.1.1.

### 9.1 Server: Linode

|          | The box we have                                   | Project needs |
| -------- | ------------------------------------------------- | ------------- |
| Provider | Linode (Akamai), Shared CPU plan                  | —             |
| CPU      | 4 vCPU amd64 (AMD EPYC 7713), **shared**          | 2+            |
| RAM      | 8 GB                                              | ~5 GB         |
| Disk     | 160 GB                                            | ~60 GB        |
| Region   | **`ap-south` — Singapore**                        | Singapore     |
| OS       | **Ubuntu 24.04 LTS** (the box shipped with Arch — rebuild, §13.1) | Ubuntu LTS |

- **Must be a real VPS with root and Docker** — the API drives Docker Engine.
- **Singapore is a hard constraint.** The terminal is xterm.js over WebSocket;
  every keystroke is a round trip. Singapore → Vietnam is 30–50 ms; EU/US is
  250–300 ms and breaks the core feature.
- **Ubuntu LTS, not a rolling distro.** `bootstrap.sh` is apt-based, relies on
  `unattended-upgrades` for security patches, and refuses anything that is not
  Ubuntu. A rolling release moves the kernel and Docker on every upgrade, which
  is the wrong property for the one box that serves everything.
- **Linode's Ubuntu image has no Docker.** `bootstrap.sh` installs Docker Engine
  + the compose plugin from Docker's own apt repository (not Ubuntu's `docker.io`).
- **Firewalls.** `ufw` (set by bootstrap) is always on. A **Linode Cloud
  Firewall** is optional and free; if one is attached it is the first door and
  must allow 22, 80 and 443 too — forget it and the box is unreachable while
  `ufw status` looks fine. Recommended: attach one allowing only 22/80/443, so
  a mistake in `ufw` alone does not expose anything.
- **Turn on Linode Backups** (paid add-on, per Linode). It does not replace R2:
  same provider, same account — lose the account, lose both. Linode Backups
  restores fast and is the only copy of the `uploads` volume; R2 survives.
- **Never publish the box's IP** (in docs, commits or issues). Behind the
  orange cloud the IP is what lets someone bypass WAF and rate limits (§9.5).

#### 9.1.1 Frontend on the box, same origin

Cloudflare Pages was rejected — not on price (Pages is cheaper) but on the
artifact. Vite bakes `VITE_API_URL` at build time; two origins mean two builds
per commit, and the bits tested anywhere are never the bits shipped.

The fix is to bake nothing: `devforge-fe/Dockerfile` pins `ENV VITE_API_URL=""`
(an `ENV`, not an `ARG`, so nothing can be forgotten), the bundle calls relative
paths, and the edge routes `/api`, `/ws`, `/uploads` to the API on the same
origin. One FE image runs in every environment.

- Empty base breaks `new URL(path, "")` (`TypeError: Invalid base URL`), so
  `src/api/labs.ts` uses `import.meta.env.VITE_API_URL || location.origin` —
  `||`, not `??`.
- Gained: no CORS, no cross-subdomain cookies, crawler blocks stay in nginx.
- Lost: per-PR previews. They could not log in anyway (`*.pages.dev` is a
  different registrable domain, so the `Lax` cookie does not cross).
- `devforge-web` only serves files (`nginx.static.conf`). It must never become a
  second proxy (§9.9 #5).

### 9.2 amd64 + GHCR — nothing is built on the box

Linode and GitHub runners are both amd64, so images are built once in
Actions, scanned there, pushed to GHCR, and only pulled on the box.

1. **Rollback takes seconds** — start an image already on disk.
2. **The box never compiles while students are typing.**
3. **`up.sh` stays small** — `pull` + `up -d --wait`, no hand-written health loop.

Six packages: `devforge-api`, `devforge-web`, `devforge-lab-{linux,git,docker,net}`.
**Set each to Public once in the GitHub UI**, or the box needs
`docker login ghcr.io` with a PAT just to pull its own release.

Nothing pins the architecture (`CGO_ENABLED=0`, static Go, `apk`), so moving to
ARM later changes no file — only the build runner.

### 9.3 Cost

| Item                 | Service                                             | Price                         |
| -------------------- | --------------------------------------------------- | ----------------------------- |
| **Prod server**      | Linode Shared 4 vCPU / 8 GB, Singapore              | **paid**, monthly             |
| Server backups       | Linode Backups add-on                               | **paid**, scales with plan    |
| **Dev server**       | Linode Shared 2 GB, Singapore                       | **paid**, monthly             |
| **Domain**           |                                                     | **~300k₫/year**               |
| TLS                  | Cloudflare Origin Certificate                       | 0                             |
| DNS + proxy + WAF    | Cloudflare Free                                     | 0                             |
| FE hosting           | same VPS                                            | 0                             |
| Registry             | GHCR, public packages                               | 0                             |
| CI/CD                | GitHub Actions, public repos                        | 0                             |
| Scanning + lint      | Trivy, gitleaks, golangci-lint                      | 0                             |
| Code quality         | SonarQube Cloud, public repos                       | 0 — waiting for `SONAR_TOKEN` |
| Backup               | Cloudflare R2, 10 GB, zero egress                   | 0                             |
| Email                | Resend 3000/month or Brevo 300/day                  | 0                             |
| Uptime               | UptimeRobot / BetterStack free                      | 0                             |
| Errors / metrics     | Sentry, Grafana Cloud free                          | 0 — not enabled               |
| Analytics            | Umami Cloud                                         | 0 — not enabled               |
| AI scenario builder  | OpenRouter                                          | §9.6 — the only variable cost |

Fixed cost = two Linodes + Linode Backups (prod) + domain.

### 9.4 Capacity: CPU is the ceiling, RAM is the backstop

Each lab container is capped at 512 MB, 0.5 CPU, 256 pids
(`internal/labs/adapter/dockerx/runtime.go`). Those are ceilings, not
reservations.

```
16 containers × 512 MB cap       = 8 GB     ← theoretical, = all RAM
16 containers × ~150 MB real RSS = 2.4 GB   vs ~6 GB free → fine
16 containers × 0.5 vCPU         = 8 vCPU   vs 4 shared   → 2× oversubscribed
```

Only ~8 containers can be CPU-busy at once, and **shared** vCPU can lose time
to neighbours on the same host. For teaching labs (mostly reading and typing)
`MAX_CONTAINERS=16` is the starting point — an assumption, not a measurement.
**Slow under real load → lower it to 12. Never raise it on this box.**

Watch two numbers after launch:

- **CPU steal** — `st` in `top` / `vmstat 5`. Sustained above ~10% means
  neighbours are taking the CPU the seat count assumes.
- **Memory** — the 512 MB caps add up to all of RAM at 16 seats; the 2 GB swap
  is what keeps a burst from becoming an OOM kill of Postgres.

Levers that need no code:

1. **Simulated labs cost zero containers** — the capacity check sits after the
   sim branch in `labs.go`. Put sims ahead of container labs in the learning
   path.
2. **`LAB_SESSION_TTL` 60m → 20m** turns seats over three times faster.
3. **Tune `MAX_CONTAINERS` from measured load**, not feel.

More users or high steal → **Linode Dedicated CPU** (same vCPU count, no
neighbours) or a 16 GB plan, then raise the cap. A bigger box is the right knob.

### 9.5 Cloudflare and `TRUSTED_PROXIES` — two proxy layers

The Origin Certificate is trusted only by Cloudflare, so the **orange cloud is
mandatory**: turn it off and browsers reject the cert immediately. The request
path therefore has two proxies:

```
browser → Cloudflare edge → nginx (container) → api (container)
```

Everything that identifies a caller reads `c.ClientIP()`: the per-IP rate limit
on public share routes, `audit_logs.ip`, and the logged-in-devices screen. Both
halves are required:

1. **nginx restores the real IP:** `set_real_ip_from <Cloudflare ranges>` (22
   lines, dated in the file) + `real_ip_header CF-Connecting-IP`.
2. **The API trusts nginx:** `TRUSTED_PROXIES=127.0.0.1,::1,172.16.0.0/12`, because
   nginx reaches the API across the compose bridge.

Get either wrong and both symptoms appear, only in production:

| Where                                                  | Symptom                                                                        |
| ------------------------------------------------------ | ------------------------------------------------------------------------------ |
| `internal/labs/adapter/ratelimit/redis.go` (key = IP)  | `PUBLIC_RATE_LIMIT` 60/min per IP becomes 60/min for the whole internet        |
| `audit_logs.ip`, devices screen                        | every row shows the same address                                               |

- Widening to the bridge range is safe only because the API publishes no port
  and nginx is the sole way in. **Never `0.0.0.0/0`** — any caller could pick
  their own IP. The server logs `slog.Warn` at start if it sees that, but still
  boots.
- Pinned by `cmd/server/trustedproxies_test.go` and by `make check-edge`
  ("a forged X-Forwarded-For is replaced, not passed through").
- **Cloudflare ranges change.** Refresh from `https://www.cloudflare.com/ips-v4`
  and `/ips-v6` (§13.9). Nothing detects a missing range, and the symptom is the
  table above for only some users.

Cloudflare settings:

- **SSL/TLS mode: Full (strict).** Never Flexible — that sends session cookies
  in plain HTTP on the last hop.
- **Cache rules by path, not host** (FE and API share the host). Cache only
  `assets/*` and `index.html`. Leave `/api/*`, `/ws/*`, `/uploads/*` on default
  bypass, WAF only. A "Cache Everything" rule on `/api/*` serves one user's
  `Set-Cookie` response to another, and can break the `/ws/*` upgrade.
- **Authenticated Origin Pulls** — prepared, not enabled. It stops anyone who
  finds the VPS IP from bypassing WAF and rate limits. Two switches, strict
  order: enable in the Cloudflare dashboard **first**, confirm the site still
  serves, **then** uncomment `ssl_client_certificate` + `ssl_verify_client on` in
  `deploy/nginx/devforge.conf`, put `cloudflare-origin-pull-ca.pem` in
  `deploy/nginx/certs/`, and reload. Reverse the order and every request gets 400.

### 9.6 AI cost — the only line that can outspend the server

The AI scenario builder calls OpenRouter: ~4k input tokens (12.4 KB system
prompt), ~3k output (`maxTokens = 16000`).

| Model                        | $/1M in | $/1M out | Per generation       |
| ---------------------------- | ------- | -------- | -------------------- |
| `anthropic/claude-opus-5`    | $5      | $25      | ~$0.095 ≈ **2,500₫** |
| `anthropic/claude-sonnet-5`  | $3      | $15      | ~$0.057 ≈ 1,500₫     |
| `anthropic/claude-haiku-4-5` | $1      | $5       | ~$0.019 ≈ **500₫**   |

100 students, 20% using it, ~3 runs/day: opus-5 ≈ 4.4M₫/month, haiku-4.5 ≈
900k₫/month. At the hard cap (`AI_DAILY_LIMIT=10`, everyone maxing out) opus-5
reaches ~74M₫/month.

Before turning it on: `OPENROUTER_MODEL=anthropic/claude-haiku-4-5`,
`AI_DAILY_LIMIT=3`, and leave `OPENROUTER_API_KEY` empty on the first deploy.
Fill it in after watching one real day of usage. Most `:free` models lack
`response_format`, so there is no fully free path.

### 9.7 Risks

1. **One box, no HA.** Box down = FE and API down together.
2. **Backup is the first thing to make work, not the last.** A backup nobody has
   restored is a guess (§13.7).
3. **Cloudflare is a single point of failure.** Orange cloud off or account
   trouble → the Origin cert is invalid and the site is down. Fallback: certbot +
   Let's Encrypt (§9.8).
4. **Dev catches most, not all.** Real traffic, real data, the SMTP provider
   and the AI key exist only in production. Run §13.6 on production after every
   release that touched the edge, auth or mail.
5. **Shared CPU.** Neighbours can take CPU at the worst moment; watch steal
   (§9.4) and move to Dedicated CPU if it stays high.
6. **The `.env` and the cert have no automatic copy** (§8 "Environment
   variables").

### 9.8 Fallbacks

| If                                 | Switch to                            | Trade-off                                                            |
| ---------------------------------- | ------------------------------------ | -------------------------------------------------------------------- |
| No dependence on Cloudflare proxy  | certbot + Let's Encrypt, grey cloud  | three moving parts: client, renewal cron, nginx reload hook          |
| Shared CPU steal too high          | Linode Dedicated CPU, same region    | more per month; Linode "Resize" keeps the disk and IP               |
| Another provider                   | DigitalOcean / Vultr / Hostinger SG  | rebuild + DNS change; everything in §13 applies unchanged            |
| Lowest latency                     | Vietnamese VPS (AZDIGI, Vietnix)     | <20 ms, local payment, pricier                                       |
| Most CPU per đồng                  | Contabo Singapore 4 vCPU / 8 GB      | oversold, slow support                                               |
| Back to 0₫                         | Oracle Always Free A1 (ARM)          | build on box, no registry, rollback = rebuild — everything §9.2 removed |

### 9.9 Self-hosting traps

1. 🔴 **The uptime monitor must live outside what it monitors.** Uptime Kuma on
   the prod box dies with the box and reports nothing. Use UptimeRobot,
   BetterStack or a Cloudflare Health Check on `https://<domain>/readyz` every
   5 min, alerting to Telegram. `/readyz`, not `/healthz`.
2. 🔴 **Docker-published ports bypass `ufw`.** Docker inserts iptables rules ahead
   of ufw. `"5432:5432"` exposes Postgres to the internet while `ufw status`
   says blocked. Use `127.0.0.1:5432:5432` or `expose:`. The compose files are
   correct today; do not drop the `127.0.0.1:` prefix "to debug".
3. 🟠 **Docker logs are unbounded by default.** `bootstrap.sh` sets
   `json-file` 10 MB × 3 in `/etc/docker/daemon.json`. A box set up by hand
   without it fills its disk from one noisy lab.
4. 🟠 **Two firewalls** — provider (Linode Cloud Firewall, if attached) and
   `ufw`. Open 22/80/443 in both.
5. 🟡 **One proxy, one static server — never two proxies.** nginx at the edge
   does TLS and routing; `devforge-web` only serves files. A second proxy adds a
   hop, a config to sync, and another place where `X-Forwarded-For` breaks.

---

## 13. Runbook — from no box to production

Every command on the box runs as user `devforge` in
`/opt/devforge/devforge-be` unless it says root. Define this once per shell:

```bash
cd /opt/devforge/devforge-be
dc() { IMAGE_TAG="${IMAGE_TAG:-$(cat .image-tag)}" docker compose -f docker-compose.yml -f docker-compose.prod.yml "$@"; }
```

⚠️ **Plain `docker compose -f … -f docker-compose.prod.yml` fails on the box**
for every subcommand, `ps` and `logs` included:
`required variable IMAGE_TAG is missing a value`. `IMAGE_TAG` is required on
purpose (no accidental `latest`), so every manual command must pass it — `dc`
reads it from `.image-tag`. `scripts/backup.sh` and `restore.sh` use the base file
only and are unaffected.

### 13.0 Status

Checked 2026-10-07. Update this block whenever a line changes.

| Item                                        | State                                                                                                   |
| ------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Production box                              | ◐ Linode bought — `ap-south` (Singapore), 4 shared vCPU / 8 GB / 160 GB. Still **Arch Linux**, password SSH login **on**, no Docker → rebuild to Ubuntu 24.04 (§13.1) |
| Dev box                                     | ❌ not bought                                                                                            |
| Pipeline (build on develop, promote on tag, deploy-dev) | ✅ written, actionlint clean; ⚠️ never run in Actions                                          |
| `make release` (`scripts/release.sh`)       | ✅ `make check-release` passes (6 cases, temp repos); never run against a real dev box                  |
| Release tags                                | ❌ none in either repo                                                                                   |
| GitHub Environments / secrets               | ❌ none — `sonar`, `deploy-dev` and `deploy` skip themselves                                             |
| Repo visibility                             | ✅ both repos **public** (2026-10-07) — Environments, branch protection, unlimited Actions minutes, free public GHCR packages and SonarQube Cloud all depend on it |
| be CI                                       | 🔴 `secrets` job red: `trivy edge image` finds 2 HIGH in `nginx:1.31-alpine` — CVE-2026-93990 (`libexpat` 2.8.4-r0 → 2.8.5-r0) and CVE-2026-103111 (`pcre2` 10.48-r0 → 10.49-r0). Blocks every be build, **including the dev build**. Fix: §13.9 |
| fe CI                                       | ✅ branches green. The same nginx base is in `devforge-fe/Dockerfile`, so `image-dev` will hit the same CVE |
| Default branch                              | ✅ `develop` on both repos                                                                               |
| Software side (compose, up.sh, bootstrap, nginx) | ✅ written; ⚠️ never run on a real box — lines marked ⚠️ *check on the box* below                  |

### 13.1 Manual work before touching the boxes

Needs logins; cannot be scripted.

| ☐ | Task                                                                                          | Why / notes                                                                         |
| - | --------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| ✅ | Production Linode, **Singapore (`ap-south`)**, 4 shared vCPU / 8 GB                          | done                                                                                |
| ☐ | **Rebuild** the production Linode from **Ubuntu 24.04 LTS**, SSH public key in *Authorized Keys* | wipes the disk (it is empty); `bootstrap.sh` refuses non-Ubuntu                  |
| ☐ | Dev Linode: **Shared 2 GB, Singapore, Ubuntu 24.04 LTS**, SSH key in *Authorized Keys*        | its own box (§8 "Dev environment")                                                  |
| ☐ | (recommended) **Linode Cloud Firewall** on both: inbound 22, 80, 443 only                     | first door; `ufw` is the second                                                     |
| ☐ | **Linode Backups** on production only                                                         | fast restore, only copy of `uploads`; not a replacement for R2                      |
| ☐ | Cloudflare DNS: `<domain>` and `dev.<domain>` **A** records, **orange cloud** both            | no `api.` record                                                                    |
| ☐ | Cloudflare SSL/TLS mode **Full (strict)**                                                      | never Flexible                                                                      |
| ☐ | Cloudflare **Origin Certificate** for `<domain>` **and** `*.<domain>`; save cert + key         | one cert for both boxes, 15 years                                                   |
| ☐ | Cloudflare **Access** application on `dev.<domain>`, allow the team's emails                   | keeps dev private (§8 "Dev environment")                                            |
| ☐ | Cloudflare cache rules: only `assets/*`, `index.html`; nothing on `/api/*`, `/ws/*`, `/uploads/*` | §9.5                                                                             |
| ☐ | Google Console redirect URIs: `https://<domain>/api/auth/google/callback` **and** `https://dev.<domain>/…` | exact match                                                            |
| ☐ | SMTP account (Resend or Brevo), sending domain verified                                        | production only; dev uses Mailpit                                                   |
| ☐ | Cloudflare **R2** bucket + lifecycle rule "delete after 30 days" + API token                   | production backups                                                                  |
| ☐ | **Two new** SSH key pairs for CD — one per box, never a personal key                           | a leaked dev key must not open production                                           |
| ☐ | GitHub **Environment `development`** in **both** repos: `SSH_HOST`, `SSH_USER`=`devforge`, `SSH_KEY` (dev key) | turns `deploy-dev` on                                      |
| ☐ | GitHub **Environment `production`** in **devforge-be** only: same three names (prod key), **required reviewer** = you | add the secrets after the first manual deploy (§13.4) |
| ☐ | Set the 6 GHCR packages to **Public** (after the first develop build creates them)            | otherwise both boxes need a PAT to pull                                             |
| ☐ | Your laptop: `DEV_SSH=devforge@<dev host>` in `devforge-be/.env`, and your key on the dev box  | `make release` reads `.deployed` over ssh                                           |
| ☐ | External **uptime monitor** on `https://<domain>/readyz`, 5 min, Telegram alert                | production only; never on the VPS itself                                            |
| ☐ | (optional) SonarQube Cloud: create org, bind both repos, add `SONAR_TOKEN` to both             | the `sonar` job turns itself on                                                     |
| ☐ | (optional) Sentry for Go + React                                                               | runtime errors with stack traces                                                    |

### 13.2 Provision the box

From your machine, as root on the new box:

```bash
scp deploy/bootstrap.sh root@<ip>:/tmp/
ssh root@<ip> 'bash /tmp/bootstrap.sh'
```

`bootstrap.sh` (idempotent where it matters):

- installs `ca-certificates curl git ufw fail2ban unattended-upgrades cron`;
- refuses to run on anything but Ubuntu;
- installs Docker Engine + compose plugin from Docker's apt repo if Docker is
  missing; refuses an existing Docker without the compose plugin;
- writes `/etc/docker/daemon.json` (json-file 10 MB × 3) and restarts Docker;
- creates user `devforge` in group `docker`, `/opt/devforge` owned by it, and
  copies root's `authorized_keys` to it;
- `ufw --force reset`, deny incoming, allow 22/80/443, enable;
- writes `/etc/ssh/sshd_config.d/00-devforge.conf` (`PermitRootLogin
  prohibit-password`, `PasswordAuthentication no`), validates with `sshd -t`,
  reloads, prints the effective values — a drop-in because the first value
  wins and a cloud-init drop-in could otherwise keep passwords on;
- tops swap up with a 2 GB `/swapfile` when total swap is under 2 GB (Linode's
  image has a ~512 MB swap disk);
- nightly backup cron `/etc/cron.d/devforge-backup` at 03:15 → `/var/log/devforge-backup.log`, weekly logrotate × 8;
- unattended security upgrades.

⚠️ *Check on the box:* tested in an `ubuntu:24.04` container up to the Docker
install (the OS guard and the apt repository work); the systemd, sshd, swap and
`ufw` steps have only run on paper. Keep the root SSH session open until a
**second** session logs in as `devforge` with the key. `ufw --force reset`
overwrites any existing rules — read the output, not just the exit code.

Then add the CD public key for `devforge`:

```bash
ssh root@<ip> 'cat >> /home/devforge/.ssh/authorized_keys' < cd_deploy_key.pub
```

### 13.3 Configure: `.env` and the certificate

From here on, log in as `devforge`, not root.

```bash
git clone https://github.com/<owner>/devfore-be.git /opt/devforge/devforge-be
cd /opt/devforge/devforge-be
cp .env.prod.example .env && chmod 600 .env
openssl rand -base64 32     # → JWT_SECRET
openssl rand -base64 24     # → DB_PASSWORD
nano .env
```

Only the be repo is cloned; the FE ships as an image. The directory must be
`/opt/devforge/devforge-be` — the deploy job and the backup cron hard-code it.

Lines that go wrong most often:

| Variable          | Value                                    | If wrong                                                       |
| ----------------- | ---------------------------------------- | -------------------------------------------------------------- |
| `DOMAIN`          | the real domain                          | `up.sh` refuses to run                                         |
| `IMAGE_REPO`      | `ghcr.io/<owner>`, lowercase             | pull fails                                                     |
| `DB_PASSWORD`     | generated                                | `DATABASE_URL` interpolates it — edit only here                |
| `PUBLIC_URL`      | `https://<domain>`, **no `/api`**        | uploads become `/api/uploads/…`, which no route serves         |
| `TRUSTED_PROXIES` | `127.0.0.1,::1,172.16.0.0/12`            | rate limit global, all audit IPs identical                     |
| `SMTP_*`          | provider values                          | new users never receive a code; no server error                |
| `OPENROUTER_API_KEY` | **empty** at first                    | §9.6                                                           |
| `R2_*`            | bucket, endpoint, key id, secret         | `backup.sh` exits with `parameter null or not set`             |

Certificate, pasted from Cloudflare:

```bash
install -m 644 /dev/null deploy/nginx/certs/origin.pem
install -m 600 /dev/null deploy/nginx/certs/origin.key
nano deploy/nginx/certs/origin.pem   # certificate
nano deploy/nginx/certs/origin.key   # private key
```

`deploy/nginx/certs/.gitignore` is `*` + `!.gitignore`, so the key never reaches
git. Both `.env` and the cert survive every deploy (CD force-checks-out a tag and
never cleans). Copy both into a password manager now.

### 13.4 Order to production

Steps 9 and 11 are skipped most often, and they are the only two that prove the
safety net exists.

```
0. Fix be CI (§13.9 "Trivy is red")                ← now; it blocks every build, dev included
1. §13.1 dashboard work (both Linodes on Ubuntu 24.04, DNS, cert, Access, Google, R2, SMTP)
2. §13.2 bootstrap BOTH boxes
3. §13.3 .env + cert on both — dev from the DEV block of .env.prod.example
4. Dev on: add the `development` environment to both repos → push to develop
   → watch images-dev / image-dev + deploy-dev → set the 6 GHCR packages Public
5. Seed dev with demo data (§13.5) and verify dev (§13.6 against dev.<domain>)
6. Merge develop into master; make release v=v0.1.0
   → promote runs in both repos; `deploy` skips (no production secrets yet)
7. §13.5 deploy v0.1.0 to production BY HAND; first admin
8. §13.6 verify production
9. §13.7 first backup by hand + RESTORE DRILL         ← do not postpone
10. Production CD on: add the `production` environment (secrets + required reviewer)
    → next sprint's release goes through CD end to end (§13.11)
11. §13.8 rollback drill
12. Authenticated Origin Pulls (§9.5), after the site has run a while
13. SonarQube Cloud token, Sentry (optional)
```

The production `deploy` job skips itself until `SSH_HOST` exists in the
`production` environment, which is what lets step 7 be done by hand and step 10
be compared against it.

### 13.5 First deploys

**Dev, first time.** After step 4 has deployed dev once, seed demo data — the
pipeline only migrates. On the dev box:

```bash
cd /opt/devforge/devforge-be
set -a; . ./.env; set +a
dc exec -T postgres psql -U "$DB_USER" -d "$DB_NAME" < scripts/seed.sql
```

`seed.sql` creates demo accounts. That is fine behind Cloudflare Access and is
exactly why it never runs in production.

**Production, first release.** Dev must be healthy on the commits you mean to
ship (`ssh devforge@<dev host> cat /opt/devforge/devforge-be/.deployed`), and
`develop` merged into `master` in both repos. On your machine:

```bash
cd devforge-be
make release v=v0.1.0
```

Wait until **both** repos' `promote` jobs are green. Then on the production box:

```bash
cd /opt/devforge/devforge-be
git fetch --tags --force && git checkout --force --detach v0.1.0
IMAGE_TAG=v0.1.0 ./deploy/up.sh
```

Expected end: `==> healthy on v0.1.0`. On the first release there is no
`.image-tag`, so a failure prints `NO PREVIOUS TAG TO ROLL BACK TO` and leaves the
stack up for you to read logs (`dc logs --tail 100 api`).

**First admin.** `make admin` cannot work on a box: it runs
`go run ./cmd/createadmin`, there is no Go there, and the distroless image
contains only `/server`. Instead:

1. Register a normal account at `https://<domain>` (no email gate blocks login,
   so this works even before SMTP is set).
2. Grant the role in SQL — roles live in `user_roles`, not on `users`:

   ```bash
   set -a; . ./.env; set +a
   dc exec -T postgres psql -U "$DB_USER" -d "$DB_NAME" <<'SQL'
   INSERT INTO user_roles (user_id, role_id)
   SELECT u.id, r.id FROM users u, roles r
   WHERE u.email = 'you@example.com' AND r.name = 'admin'
   ON CONFLICT DO NOTHING;
   SQL
   ```

3. Log out and back in so the token carries the role.

### 13.6 Verify after going live

From your machine, not from the box:

```bash
curl -sf https://<domain>/healthz            # process alive
curl -sf https://<domain>/readyz             # + database reachable
curl -sI https://<domain>/ | grep -i '^cf-'  # served through Cloudflare
```

In a browser — these only break in production:

| Check                                                     | Proves                                                     |
| --------------------------------------------------------- | ---------------------------------------------------------- |
| Log in, press F5, still logged in                         | cookie + same origin                                       |
| Open a lab, type quickly, no stutter                      | `/ws/` with `proxy_buffering off`                          |
| A cover image renders                                     | `/uploads/*` route + `PUBLIC_URL` without `/api`           |
| Open a lab, then `dc restart api` — terminal reconnects   | reconnect with backoff; every deploy does this to students |
| Paste a `/r/<id>` link into Zalo/Slack → preview card     | crawler block ahead of the catch-all                       |
| Your last login in `audit_logs.ip` is your real IP        | §9.5, both layers                                          |
| Sign up a new account → code arrives by email             | `SMTP_*`                                                   |
| Google login completes                                    | redirect URI                                               |

Edge routing can also be checked on a dev machine, before any box:
`make check-edge`. Run it after every edit to `deploy/nginx/devforge.conf` —
`nginx -t` only proves the file parses.

### 13.7 Backup and restore drill

The cron from `bootstrap.sh` runs `scripts/backup.sh` at 03:15 daily. Do not
wait for it:

```bash
./scripts/backup.sh       # pg_dump | gzip → s3://$R2_BUCKET/pg/devforge-<UTC>.sql.gz
./scripts/restore.sh      # newest dump → scratch DB devforge_restore_check, prints row counts
```

- `backup.sh` refuses to upload a dump under 1 KB (that is a gzipped error).
- `restore.sh` prints counts for `users`, `courses`, `labs`,
  `lab_task_completions`. **Zeros mean the backup is broken, not the script.**
  Drop `devforge_restore_check` afterwards.
- Re-run the drill whenever a migration changes the schema shape.
- Watch the cron: `tail /var/log/devforge-backup.log`.

**Restoring for real.** `restore.sh --into-live` loads the newest dump into
`$DB_NAME` **without dropping it** (it asks you to type the name). The dump
contains the whole schema plus `schema_migrations`, so it must land in an
**empty** database — loading it over a migrated one collides on every
`CREATE TABLE` and on the rows migrations seed into `roles`.

⚠️ Never run on a real box yet — drill it on a scratch box first.

Lost the whole box:

```bash
# new box: §13.2, then §13.3 with .env and the cert from your password manager
git fetch --tags --force && git checkout --force --detach <last good tag>
IMAGE_TAG=<last good tag> dc up -d --wait postgres   # fresh, empty database
./scripts/restore.sh --into-live
IMAGE_TAG=<last good tag> ./deploy/up.sh              # migrate up is a no-op now
```

Damaged database on a live box (destructive — take a fresh `./scripts/backup.sh`
first if the database still answers):

```bash
set -a; . ./.env; set +a
dc stop api
dc exec -T postgres psql -U "$DB_USER" -d postgres \
  -c "DROP DATABASE \"$DB_NAME\" WITH (FORCE)" -c "CREATE DATABASE \"$DB_NAME\""
./scripts/restore.sh --into-live
dc up -d --wait
```

Uploaded images live in the `uploads` volume and are **not** in the dump — only
Linode Backups covers them (§13.10).

### 13.8 Rollback

**Automatic:** `up.sh` rolls back by itself when a release is not healthy within
120 s, to the tag in `.image-tag`, without migrating.

**By hand** (bad release that passed its healthcheck):

```bash
cd /opt/devforge/devforge-be
cat .image-tag                                  # what is serving now
git tag --sort=-v:refname | head               # pick the previous release
IMAGE_TAG=<previous tag> ./deploy/up.sh
```

- **Do not `git checkout` the old tag first.** Stay on the current checkout:
  its `migrations/` contains every applied version, so `migrate up` is a no-op.
  An older checkout lacks the newest file and `migrate` dies with "no migration
  found for version N".
- Use `up.sh`, not a bare `dc up`: `up.sh` also retags the four lab images and
  rewrites `.image-tag`. A bare `dc up` leaves labs on the bad release and
  `.image-tag` pointing at it.
- Image only — never `migrate down` during an incident.
- Afterwards the checkout (nginx config, migrations) is newer than the image.
  That is fine only because both must be backward compatible. The next release
  force-checks-out its own tag.

**Drill it** (step 9 of §13.4) while nothing is on fire.

### 13.9 Day-2 operations

**What is running**

```bash
cat .image-tag                 # tag serving
cat .deployed                  # be and fe commits serving (what `make release` reads on dev)
dc ps
git describe --tags            # checkout (detached HEAD on the tag is correct — never `git pull`)
```

**Logs**

```bash
dc logs -f --tail 100 api
dc logs --tail 100 nginx
docker ps --filter ancestor=devforge/linux:latest     # live lab containers (also git/docker/net)
```

**Change a variable** (no new release needed):

```bash
nano .env
dc up -d --wait --force-recreate api
```

`MAX_CONTAINERS`, `LAB_SESSION_TTL`, `AI_DAILY_LIMIT` and the like are tuned this
way. Changing `JWT_SECRET` signs everyone out. Changing `DB_PASSWORD` here does
not change it inside an existing Postgres volume — run `ALTER USER` first.

**Edge returns 502 after a deploy**

```bash
dc ps                                           # is api healthy?
dc logs --tail 50 api
dc exec nginx wget -qO- http://api:8080/healthz # can nginx reach api?
```

The third line separates "nginx cannot see api" from "api is unhealthy" — they
look identical from outside. The resolver-variable pattern in `devforge.conf`
should prevent stale DNS; if someone wrote a literal `proxy_pass http://api:8080`,
that is the bug.

**Trivy is red on a base image** (the current state, §13.0)

1. Read the finding: image, package, installed → fixed version.
2. Upstream rebuilt the same tag? Re-run the job (Trivy pulls fresh). If green,
   done.
3. Otherwise bump the pin to a tag that carries the fix, in **every** place it
   lives: `docker-compose.prod.yml` (edge), `devforge-fe/Dockerfile` (web), and
   any `labs/*/Dockerfile` affected. Keep pinning to a version line, not to
   `alpine` or `latest`.
4. No fixed image published yet and the release cannot wait → one
   `.trivyignore` line with `exp:<date ≤ 30 days>` and the reason. Never without
   an expiry.
5. On the box, the edge picks up a new tag on the next `up.sh`.

**Refresh Cloudflare IP ranges** (every few months, or when an `audit_logs.ip`
shows a Cloudflare address):

```bash
curl -s https://www.cloudflare.com/ips-v4; curl -s https://www.cloudflare.com/ips-v6
```

Compare with the `set_real_ip_from` lines in `deploy/nginx/devforge.conf`, update
the date comment, `make check-edge`, release.

**Disk**

```bash
df -h /; docker system df
```

`up.sh` prunes dangling images after each healthy release. Old release images
stay (they are rollback targets); remove ones older than the last few tags with
`docker image rm` when space is tight.

**Rotate a secret**

- `JWT_SECRET`: edit `.env`, recreate `api`. Everyone is signed out.
- Google client secret: rotate in Google Console, edit `.env`, recreate `api`.
- CD key: new key pair → replace line in `/home/devforge/.ssh/authorized_keys` →
  update `SSH_KEY` secret.

**Origin certificate** expires 15 years after creation. Put the date in a
calendar anyway.

### 13.10 Deliberate debt

| Item                                         | Why not yet                                                                                  |
| -------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Dev cannot roll back                         | It deploys the moving `:develop`; fixed forward by the next push                             |
| Promote prints digests, does not compare them | The version is a re-tag of the pulled `dev-<sha>`; the log shows both. Add a hard check if they ever differ |
| Log / metric aggregation (Grafana Cloud)     | Not blocking launch; do it once there are real users                                         |
| Sentry                                       | Cheap; do with the first real users                                                          |
| `make check-edge` in CI                      | Needs Docker (CI has it); ~6-line job, not added yet — run it by hand after edge edits        |
| Uploads not backed up off-provider           | `backup.sh` dumps Postgres only; the `uploads` volume relies on Linode Backups               |
| Cloudflare IP ranges hard-coded              | Dated 2026-09-18; refreshed by hand (§13.9). Nothing detects drift                           |
| Authenticated Origin Pulls off               | Two-sided switch; enable after launch (§9.5)                                                 |
| nginx pinned by minor line, not digest       | Consistent with `postgres:16-alpine`; Trivy edge scan is the safety net                      |
| Chat WebSocket has no reconnect              | Terminal has it; chat shares `terminalURL` but not the retry. Losing chat does not kill a lab |
| Drop Redis (sessions into Postgres)          | Works, zero config; saves one container — not worth it now                                   |

### 13.11 Sprint release process

A sprint is the planning rhythm; a release is whenever `develop` on dev is worth
shipping — usually once per sprint, earlier when a finished feature carries a
migration (a three-step migration is three releases, §8 "Migrations").

```
Sprint days    feature branch → PR into develop → CI → dev deploys it
               test each feature on dev.<domain> as it lands

Freeze         stop merging features into develop; fixes only
               unfinished features stay on their branches — never merged half-done

Test on dev    each feature's acceptance criteria + §13.6
               bug → fix PR into develop → dev redeploys → test again

Release        PR develop → master in BOTH repos (merge commit is fine)
               make release v=vX.Y.0
               approve `deploy` in the production environment (Actions)
               §13.6 on production, watch logs for 30–60 min
               release note

Unfreeze       merging into develop resumes
```

Prerequisites on your machine: both repos cloned side by side, `DEV_SSH` in
`devforge-be/.env`, your key on the dev box as `devforge`.

What `make release` refuses, before tagging anything:

| Refusal                                        | Meaning                                                            |
| ---------------------------------------------- | ------------------------------------------------------------------ |
| `refusing tag`                                 | version name fails `release-guard.sh`                              |
| `dev has no healthy, labelled deploy recorded` | dev is mid-deploy, its last deploy failed, or it runs pre-label images — push to develop and wait |
| `… is not on master`                           | merge develop into master first                                    |
| `… already has vX.Y.Z`                         | that version exists — cut the next one                             |

Versions: a release with features bumps the minor (`v1.3.0`); a release with
only fixes bumps the patch (`v1.3.1`).

**Bug policy (decided 2026-10-07): no hotfixes.** Every bug, whatever its
severity, is fixed on `develop`, verified on dev, and reaches production with the
next release. There is no `hotfix/*` branch and no path that patches production
outside a release.

```
bug on production → branch fix/<name> from develop → PR into develop
→ dev deploys it → verify on dev → ships with the next release
```

- Branch names: `feat/<name>` for features, `fix/<name>` for bugs, both from and
  into `develop`.
- A bug in production that cannot wait: if the last release brought it, **roll
  back** to the previous tag (§13.8) — seconds, no code. The fix still goes
  through `develop` and the next release. If rolling back does not help, the bug
  waits for the next release.
- Releases ship everything on `develop`, so unfinished work is never merged and
  every merged change is tested on dev as it lands.

### 13.12 When production has a bug

Rule: **mitigate first, fix second.** There are no hotfixes (§13.11): the fix
always goes through `develop` and the next release. What can happen right away
is a rollback.

```
Bug reported on production
│
├─ 1. Confirm it on production, and check whether dev has it too
│
├─ 2. Did the latest release bring it?   (worked on the previous version?)
│     ├─ yes, and users are blocked → ROLL BACK now (below), then go to 3
│     └─ no / not sure / not blocking → go to 3
│
├─ 3. fix/<name> from develop → PR → dev → verify on dev
│
└─ 4. Ships with the next release (§13.11)
```

**Severity decides only whether to roll back** — every fix takes the same path:

| Severity | Examples                                                        | Action now                                          |
| -------- | --------------------------------------------------------------- | --------------------------------------------------- |
| **SEV1** | site down, nobody can log in, labs cannot start, data loss, security hole | roll back if the last release caused it; tell users |
| **SEV2** | a main feature broken, a workaround exists                      | roll back only if the last release caused it and the workaround is painful |
| **SEV3** | minor or cosmetic                                               | nothing; `fix/` into the sprint                     |

**Rolling back** (details and pitfalls in §13.8):

```bash
cd /opt/devforge/devforge-be
cat .image-tag                          # the bad release
git tag --sort=-v:refname | head        # pick the one before it
IMAGE_TAG=<previous tag> ./deploy/up.sh
```

- Image only. Never `migrate down`. Never `git checkout` the old tag first.
- When the bad release carried a migration the old image cannot live with, a
  rollback will not help — which is why every migration must be backward
  compatible (§8 "Migrations").
- After a rollback, production runs an older version than `master`. The next
  release restores order: it carries everything on `develop`, including the fix.

**After a SEV1 or SEV2**, before closing it:

1. A test that fails on the bug (the fix PR carries it).
2. A short postmortem in the issue: what happened, why it was not caught on dev,
   what stops it next time. No blame.
3. If dev could not have caught it (real traffic, real data, SMTP, AI key), say
   so — that is a gap in §8 "Dev environment", not in the code.

---

## 14. Decision log

Newest first. Each line: what was decided, what was considered instead, and
why. Change a decision by adding a line, not by editing an old one.

| Date       | Decision                                                                 | Considered instead                                   | Why                                                                                         |
| ---------- | ------------------------------------------------------------------------ | ---------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| 2026-10-07 | **Both repos public**                                                    | private + GitHub Pro; private on Free                | The pipeline needs Environments, unlimited Actions minutes and free GHCR storage; private on Free has none of them. History scanned first: gitleaks clean on all commits; the two author emails in history were accepted as public |
| 2026-10-07 | **No hotfixes.** Bugs go `fix/*` → `develop` → dev → next release         | GitFlow `hotfix/*` from the prod tag; break-glass direct deploy | One path to production, nothing ships untested on dev; urgent cases are covered by rollback |
| 2026-10-07 | **Keep rollback** as the only immediate action on production             | dropping it with hotfixes                            | Seconds, no code, already in `up.sh`; mitigates a bad release without bypassing dev         |
| 2026-10-07 | **Sprint releases with a freeze**; early releases allowed                | release train; continuous delivery                   | Fits a small team's sprint rhythm; freeze keeps dev stable while it is tested               |
| 2026-10-07 | **Build once on `develop`, promote the dev image on release**            | build again at the tag (previous design)             | Production runs the exact bits tested on dev; no base-image drift between test and release   |
| 2026-10-07 | **Keep `develop` + `master`**                                            | trunk-based (`main` only)                            | Matches sprint releases; `master` stays the record of what was released                      |
| 2026-10-07 | **Three environments: local, dev, production**; dev on its **own** Linode | dev as a second stack on the production box          | Port 80/443 conflict, unreleased code on the prod box, CPU contention, seat-count trap (§8 "Dev environment") |
| 2026-10-07 | **Dev behind Cloudflare Access, demo data only**                         | public dev; copy of production data                  | Unreleased features and demo accounts stay private; no real personal data outside production |
| 2026-10-07 | **GitHub Environments** `development` / `production`, reviewer on production | repo-wide `SSH_*` + `DEV_SSH_*` secrets           | Each job sees only its box's key; production needs a person's approval                      |
| 2026-10-07 | **Linode** Shared 4 vCPU / 8 GB, Singapore, as production                | Hostinger KVM2 (previous plan)                       | Already owned; amd64 and Singapore keep every other part of the design                      |
| 2026-10-07 | **Ubuntu 24.04 LTS**, rebuild from Arch                                   | keep Arch, port `bootstrap.sh` to pacman             | LTS + unattended security upgrades on a single box; Arch moves kernel and Docker on every upgrade |
| 2026-10-07 | `bootstrap.sh` **installs Docker** from Docker's apt repo                 | require a Docker template                            | Linode's Ubuntu image has none; Ubuntu's `docker.io` lacks the compose plugin                |
| 2026-10-07 | `MAX_CONTAINERS=16` on production                                        | 12 (2 vCPU plan), 25 (4 vCPU / 16 GB plan)           | 4 shared vCPU but 8 GB — an assumption until real load is measured (§9.4)                    |
| 2026-10-07 | `INFRA.md` + `deploy/DEPLOY.md` merged into one file                     | keep two files                                       | One runbook; the two had drifted (wrong `IMAGE_TAG`, `UPLOAD_DIR`, commands that fail without `IMAGE_TAG`) |
| 2026-09-21 | Staging deferred                                                         | —                                                    | Superseded 2026-10-07 by the dev environment                                                 |
| 2026-09-18 | Trivy gates every image before push; edge image scanned too              | report-only scanning                                 | A scan after the push is a report, not a gate                                                |
| earlier    | nginx + Cloudflare Origin Certificate at the edge                        | Caddy with automatic HTTPS                           | Behind the orange cloud, Caddy's ACME is the one feature that cannot be used (§9.5)         |
| earlier    | FE image on the same box, same origin                                    | Cloudflare Pages                                     | One environment-free FE image promotable everywhere; no CORS or cross-site cookies (§9.1.1)  |
| earlier    | Images in GHCR, pulled by the boxes                                      | build on the box                                     | Fast rollback, no compiling on the box students use (§9.2)                                   |
| earlier    | SonarQube Cloud, never self-hosted                                       | SonarQube on the box                                 | 2–4 GB and CPU taken from lab seats (§9.4)                                                   |
