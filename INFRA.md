# DevForge — hạ tầng & triển khai

Tách khỏi [README.md](README.md) vì nó chiếm hơn nửa tài liệu và gần như không ai
đọc nó cùng lúc với phần sản phẩm. Nội dung không đổi một chữ nào khi tách.

**Số mục giữ nguyên: §8, §9, §13.** Hai file là một tài liệu cắt làm đôi, nên mọi
tham chiếu `§9.2` trong code và trong README vẫn trỏ đúng chỗ. README giữ §0–§7 và
§10–§12.

| | |
| --- | --- |
| §8 | Môi trường, biến, routing, migration, CI/CD, quét bảo mật, staging |
| §9 | Hạ tầng production: máy, GHCR, chi phí, sức chứa, proxy, rủi ro, bẫy |
| §13 | Việc còn phải làm để lên được máy thật |

Runbook thao tác trên máy nằm ở [deploy/DEPLOY.md](deploy/DEPLOY.md).

---

## 8. Môi trường

**Một `Dockerfile` cho mọi nơi — áp dụng cho API (Go).** Cùng file build ra image chạy local lẫn prod; khác nhau chỉ ở env var inject lúc chạy. Không có `Dockerfile.prod` riêng.

Image **build một lần ở GitHub Actions rồi đẩy lên GHCR**, máy prod chỉ `pull`. Cả hai đều amd64 nên không có QEMU ở giữa (§9.2). Hệ quả nằm ở rollback, xem ngay dưới bảng.

> **FE cũng là một image, và cũng promote được.** Vite bake `VITE_API_URL` lúc build, nên bất kỳ giá trị nào khác rỗng đều làm image dính chặt vào một môi trường: bản build cho staging không bao giờ là bản lên production. Cách thoát nằm ở chỗ **không bake gì cả**.
>
> `devforge-fe/Dockerfile` ghim `ENV VITE_API_URL=""` — không phải `ARG`, nên không có gì để quên truyền. Bundle gọi đường dẫn tương đối, và biên đã route `/api`, `/ws`, `/uploads` sang API trên **cùng origin** (`deploy/nginx/devforge.conf`). Một image chạy được mọi môi trường; xem §9.1.1 vì sao phương án Cloudflare Pages bị đảo lại.
>
> ⚠️ **Rỗng thì `fetch` sống nhưng `new URL` chết.** `new URL(path, "")` ném `TypeError: Invalid base URL`, nên `src/api/labs.ts` dùng `||` chứ không `??`: `import.meta.env.VITE_API_URL || location.origin`. Dev không đổi — không có biến thì vẫn rơi về `http://localhost:8080`.

|           | local                          | production                                                              |
| --------- | ------------------------------ | ----------------------------------------------------------------------- |
| Chạy bằng | `docker compose up`            | `docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d` |
| Kiến trúc | amd64                          | amd64 (Hostinger KVM) — xem §9.2                                        |
| Nguồn image | build tại chỗ                | `ghcr.io/<owner>/devforge-api:<tag>` + `devforge-web` + 4 `devforge-lab-*` |
| Config    | `.env` (từ `.env.example`)     | env file trên server, `chmod 600`, chủ sở hữu là user chạy compose      |
| DB        | postgres container, seed giả   | postgres volume + snapshot Hostinger + pg_dump cron → R2                |
| TLS       | không                          | nginx + Cloudflare Origin Certificate (§9.5)                            |
| FE        | `vite dev` trên host           | image `devforge-web`, cùng box cùng origin (§9.1.1)                     |
| Log       | stdout                         | stdout → `docker compose logs` (chưa có gom log, §1)                    |
| Deploy    | hot reload (`air`, `vite dev`) | Actions → GHCR → promote theo tag → SSH → `up.sh` (pull + `up -d --wait`) |

⚠️ **`sha-<short>` của hai repo KHÔNG bằng nhau, nên một sha không gọi tên được
một stack.** `deploy/up.sh` kéo `api`, `web` và 4 image lab bằng **cùng một**
`IMAGE_TAG`; `api` và 4 lab mang sha của be, `web` mang sha của fe. Nghĩa là
`IMAGE_TAG=sha-<của be>` kéo được 5 image rồi chết ở `devforge-web:sha-<của be>`
— một tag không tồn tại và sẽ không bao giờ tồn tại.

Hệ quả có hai nửa, và cả hai đều từng sai trên đĩa:

- **Prod phải là tag `vX.Y.Z`** — thứ duy nhất cả hai repo cùng promote. Đó là
  lý do tag phiên bản tồn tại, chứ không phải để cho đẹp. `deploy/DEPLOY.md` §4
  từng bảo dùng một tag bất kỳ; đã sửa.
- **Staging dùng tag trôi `:master`.** Job `deploy-staging` từng truyền
  `IMAGE_TAG=sha-$(git rev-parse --short HEAD)` của chính repo be, nên nó sẽ
  chết đúng ở `devforge-web` **ngay lần đầu được bật** — tức là ngay sau khi
  thêm ba secret `STAGING_SSH_*`, lúc không ai nghi ngờ gì. Giờ cả hai repo đẩy
  thêm `:master` cho bản master mới nhất và staging deploy tag đó. Cái giá:
  `.image-tag` trên staging ghi `master`, nên staging không có đích rollback —
  chấp nhận được, staging là máy dùng xong bỏ.

**Rollback = đổi tag, không phải build lại.** `deploy/up.sh` ghi tag đang chạy vào `.image-tag` sau mỗi lần khoẻ, và lấy chính nó làm đích lùi: `IMAGE_TAG=<tag-cũ> docker compose up -d --wait` — vài giây, vì image cũ đã nằm sẵn ở GHCR và trong cache của box. Đây là lãi trực tiếp của việc rời ARM (§9.2); bản Oracle cũ phải build lại 1–2 phút trong lúc bản lỗi vẫn đang phục vụ.

Rollback **chỉ lùi image**, không lùi schema và không chạy lại migration — xem "Migration tương thích ngược" bên dưới, và ba chi tiết ở phần CI/CD.

### Biến môi trường

| Biến                                          | local                   | production             | Nguồn      |
| --------------------------------------------- | ----------------------- | ---------------------- | ---------- |
| `APP_ENV`                                     | development             | production             | compose    |
| `PORT`                                        | 8080                    | 8080                   | compose    |
| `LOG_LEVEL`                                   | debug                   | info                   | compose    |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_NAME` | (compose)               | (compose)              | compose    |
| `DB_PASSWORD`                                 | `.env` giả              | env file trên server   | **secret** |
| `REDIS_ADDR` / `REDIS_PASSWORD` / `REDIS_DB`  | localhost:6379          | (compose)              | compose    |
| `COOKIE_DOMAIN`                               | (rỗng)                  | **(rỗng)** — host-only, cùng origin nên không cần gì khác | compose   |
| `JWT_SECRET`                                  | `.env` giả              | env file trên server   | **secret** |
| `ACCESS_TTL` / `REFRESH_TTL`                  | `15m` / `168h`          | `15m` / `168h`         | compose    |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`   | `.env`                  | env file trên server   | **secret** |
| `GOOGLE_REDIRECT_URL`                         | `http://localhost:8080/api/auth/google/callback` | `https://<domain>/api/auth/google/callback` | compose |
| `CORS_ORIGINS`                                | `http://localhost:5173` | `https://<domain>` — **giờ là thứ chặn/mở cả REST lẫn WebSocket**, không còn là dead weight | compose |
| `FRONTEND_URL`                                | `http://localhost:5173` | `https://<domain>`      | compose   |
| `PUBLIC_URL`                                  | `http://localhost:8080` | `https://<domain>` — **origin trần, không có `/api`** | compose |
| `UPLOAD_DIR`                                  | `./uploads`             | `./uploads` (bind mount, xem "Domain & routing") | compose |
| `TRUSTED_PROXIES`                             | `127.0.0.1,::1`         | `127.0.0.1,::1,172.16.0.0/12` — **bắt buộc, và chỉ là một nửa: nginx phải khôi phục IP thật từ Cloudflare, xem §9.5** | compose |
| `DATABASE_URL` (migrate)                      | `sslmode=disable`       | `sslmode=disable` — xem ghi chú dưới bảng | **secret** |
| `LAB_DOCKER_HOST`                             | `tcp://127.0.0.1:2375`  | `tcp://127.0.0.1:2375` | compose    |
| `LAB_SESSION_TTL`                             | `60m`                   | `20m` (xem §9.4)       | compose    |
| `MAX_CONTAINERS`                              | `40`                    | `12` — 2 vCPU, xem §9.4 | compose   |
| `PUBLIC_RATE_LIMIT`                           | `60`                    | `60`                   | compose    |
| `OPENROUTER_API_KEY`                          | (rỗng)                  | **(rỗng)** — xem §9.6  | **secret** |
| `OPENROUTER_MODEL`                            | —                       | `anthropic/claude-haiku-4-5` | compose |
| `AI_DAILY_LIMIT`                              | `10`                    | `3` (xem §9.6)         | compose    |
| `SMTP_HOST` / `SMTP_PORT`                     | `localhost` / `1025` (Mailpit) | Resend hoặc Brevo / `587` | compose |
| `SMTP_USER` / `SMTP_PASSWORD`                 | (rỗng — Mailpit không hỏi) | (bắt buộc)          | **secret** |
| `MAIL_FROM`                                   | `no-reply@devforge.local` | `no-reply@<domain>`  | compose    |
| `VERIFY_CODE_TTL` / `RESET_TOKEN_TTL` / `RESEND_COOLDOWN` | `10m` / `1h` / `60s` | như local        | compose    |
| `VITE_API_URL` (FE, **build-time**)           | `http://localhost:8080` | **rỗng** — ghim `ENV VITE_API_URL=""` trong `devforge-fe/Dockerfile`, không phải biến của môi trường nào (§9.1.1) | Dockerfile |
| `IMAGE_REPO`                                  | —                       | `ghcr.io/<owner>` chữ thường | env file |
| `IMAGE_TAG`                                   | —                       | `<sha>` — CD truyền vào lúc gọi `up.sh` | CD |

⚠️ **`sslmode` là `disable` ở cả hai, và bảng này từng ghi `require` ở cột
production — sai.** Container `postgres:16-alpine` không được cấu hình TLS ở đâu
cả, nên `require` làm `migrate` chết ngay lần deploy đầu với `SSL is not enabled
on the server`. Cái làm nó an toàn không phải TLS mà là phạm vi: kết nối không
bao giờ rời mạng bridge của compose, và Postgres publish cổng ở `127.0.0.1` chứ
không ra ngoài. Muốn thật sự bật TLS thì phải cấp cert cho chính container
postgres trước — đó là một việc, không phải một chữ.

**Nhóm SMTP là chỗ hỏng im lặng.** Local có Mailpit nuốt mọi thư nên không ai thấy thiếu; prod không có gì đứng thay, và mã xác thực với link reset mật khẩu là hai thứ duy nhất đi qua đường đó. Thiếu `SMTP_HOST` ở prod = người đăng ký mới không bao giờ vào được, không có lỗi nào nổ ở phía server.

`.env.example` phải liệt kê đủ biến trên (giá trị giả).

**Secret prod nằm trong env file trên server, sửa bằng tay — CD không đẩy secret từ GitHub Secrets xuống.** Actions chỉ giữ ba secret để mở được cửa: `SSH_HOST`, `SSH_USER`, `SSH_KEY`. Đổi lại là ba thay vì hơn chục, và không secret prod nào đi ngang qua Actions. Giá phải trả: thêm một biến môi trường nghĩa là một lần ssh, không phải một lần commit — và **file đó không có bản sao ở đâu cả**: `scripts/backup.sh` dump database chứ không dump config. Dựng lại máy từ đầu mà không có bản `.env` trong tay thì phải sinh lại toàn bộ khoá, và mọi phiên đăng nhập hiện có mất theo `JWT_SECRET`.

### Healthcheck (điều kiện rollback)

Đã có: `cmd/server/healthcheck.go` + `cmd/server/main.go:30` đọc `os.Args[1] == "healthcheck"`, và `docker-compose.prod.yml` khai báo healthcheck bên dưới. `deploy/up.sh` đọc `docker inspect -f '{{.State.Health.Status}}'` để quyết định rollback.

Rollback tự động ở CD chỉ chạy được nếu `api` khai báo healthcheck trong compose:

```yaml
api:
  healthcheck:
    test: ["CMD", "/server", "healthcheck"] # binary tự gọi /healthz nội bộ
    interval: 10s
    timeout: 3s
    retries: 5
```

Image prod là **distroless (không shell/wget)** → không dùng được `CMD-SHELL "wget ..."`. Vì thế healthcheck là subcommand `/server healthcheck` của chính binary, tự probe `/readyz` qua loopback trong container.

Với `docker compose up -d --wait --wait-timeout 120` thì compose chính là thứ chờ healthcheck này, nên `deploy/up.sh` không cần vòng lặp `docker inspect` tự viết nữa (§13.2).

### Domain & routing — một origin, route theo path

```
https://<domain>/            → SPA (image devforge-web) và API, cùng một biên
    /api/*      REST
    /ws/*       WebSocket (wss)
    /uploads/*  ảnh bìa + avatar — KHÔNG nằm trong /api
    /r/:id · /war-room/day/:date  → chỉ bot: rewrite sang endpoint preview
    /healthz    /readyz
    còn lại     → SPA
```

Đây đúng là thứ `deploy/nginx/devforge.conf` đang làm, và `make check-edge`
dựng chính file đó trước hai upstream giả rồi hỏi từng đường dẫn — **cả năm**
route, không phải chỉ `location /`. Vì sao một origin chứ không phải hai:
§9.1.1.

`COOKIE_DOMAIN` để trống — cùng origin thì cookie host-only là đúng, và
`SameSite=Lax` không bao giờ thành vấn đề vì không có bên thứ hai nào.

⚠️ **`/uploads/*` phải có route riêng.** `cmd/server/router.go:32` gắn
`r.Static("/uploads", …)` lên router gốc, **ngoài** group `/api`, và
`internal/upload/image.go:100` dựng URL bằng `PUBLIC_URL + "/uploads/" + name`.
Thiếu route đó thì mọi ảnh nhận về `index.html` kèm **200** — ảnh vỡ, không phải
404 để mà grep. `UPLOAD_DIR` cũng phải là volume, nếu không mỗi `up -d` là mất
sạch ảnh đã upload.

⚠️ **Khối bot phải nằm TRƯỚC route bắt-tất-cả.** Facebook/Zalo/Slack không chạy
JavaScript nên link chia sẻ hiện ô trắng; khối `@crawler` rewrite `/r/:id` và
`/war-room/day/:date` sang endpoint preview. Giữ nguyên hai biểu thức chính quy
(`^[A-Za-z0-9_-]+$` cho id, `^\d{4}-\d{2}-\d{2}$` cho ngày) — chúng ở đó để
không đẩy rác vào một endpoint sinh ảnh. nginx thử location regex theo đúng thứ tự viết, và
regex luôn thắng prefix match `location /` — đó là thứ giữ hai khối này đứng
trước route bắt-tất-cả.

Khi chốt domain:

- `FRONTEND_URL` và `PUBLIC_URL` **trùng nhau**, cả hai là `https://<domain>`.
- Google Console: redirect `https://<domain>/api/auth/google/callback`, khớp từng ký tự — cộng một URI thứ hai cho staging (§13.1).
- DNS: một bản ghi `A` cho `<domain>`, mây cam (§9.5). Không có `api.` nào để tạo.
- `VITE_API_URL` không phải biến của môi trường nào — ghim rỗng trong `devforge-fe/Dockerfile`.

Cái phải trả cho một domain: FE và API chung một đường vào, tức chung một trần
băng thông, và một lần cấu hình biên sai làm sập cả hai.

### Migration tương thích ngược

Code mới phải chạy được với schema cũ. Đổi tên cột `user_name` → `username` làm 3 bước qua 3 lần deploy:

```
Deploy 1:  ADD COLUMN username; backfill; code đọc user_name, ghi CẢ HAI
Deploy 2:  code đọc username, ghi CẢ HAI
Deploy 3:  DROP COLUMN user_name; code chỉ dùng username
```

### CI/CD — hai repo, gặp nhau ở registry

**Hai nhánh dài: `develop` tích hợp, `master` production.**

Trước đây chỗ này chỉ có master, với lập luận: thêm `develop` là rước một cái
bẫy — merge `develop → master` đẻ merge commit, SHA đổi, image mang SHA cũ
không còn khớp commit được tag, nên phải ép `--ff-only` vĩnh viễn để chống một
vấn đề tự mình tạo ra.

Lập luận đó chỉ đúng nếu **develop build image**. Ở đây nó không build gì: mọi
job có guard `refs/heads/master` đều bỏ qua develop, nên develop không sinh ra
image nào để lệch. Merge commit trên master được build như mọi commit master
khác và mang SHA của chính nó; `promote` tra đúng SHA đó. Cái bẫy tan, và
`--ff-only` là thứ không cần tới.

Cái giá còn lại là thật và nhỏ, và nó không nằm ở build: một thay đổi vẫn chỉ
build đúng một lần, lúc master nhận merge. Thứ chạy hai lần là **phần kiểm** —
một lần ở PR vào develop, một lần nữa khi develop nhận merge đó. Vài phút
runner, đổi lấy việc develop luôn có trạng thái xanh của chính nó chứ không chỉ
của từng PR rời rạc.

```
pull_request                → chỉ kiểm, không build gì
push develop                → chỉ kiểm, không build gì   ← giống hệt PR
push master                 → kiểm → build → quét → push :sha-<short> + :master → deploy STAGING
push tag v* (CẢ HAI repo)   → promote :sha-<short> → :v1.2.0          → deploy PROD
```

Tag chỉ cắt trên master. `promote` tự chặn tag không thuộc master bằng
`git merge-base --is-ancestor`, nên quy ước này không dựa vào trí nhớ ai cả.

Repo `devforge-be`:

```
job be      → gofmt -l | (! grep .)  →  golangci-lint  →  go build
              →  go test -coverprofile  →  chặn nếu tụt dưới sàn 16.5%
job secrets → gitleaks (fetch-depth: 0, cần quyền pull-requests: read)
              →  trivy fs: go.sum

chỉ khi push master và cả hai xanh:
  job images         → docker build --target prod --load   ← --load, CHƯA push
                       →  trivy image devforge-api
                       →  build 4 lab image  →  trivy image từng cái
                       →  quét xong mới push :sha-<short> VÀ :master
                          ← đỏ thì registry không có gì
                          ← :master vì up.sh deploy cả stack bằng MỘT tag, mà
                            sha của be không phải sha của fe
  job deploy-staging → chưa có secret STAGING_SSH_HOST → bỏ qua, master vẫn xanh
                       →  ssh máy staging: checkout --force -B master origin/master
                          IMAGE_TAG=master ./deploy/up.sh

chỉ khi push tag `v*` và cả hai xanh:
  job promote → git merge-base --is-ancestor HEAD origin/master  ← tag phải trên master
                →  imagetools inspect :sha-<short>  thiếu → dừng, commit chưa từng build
                →  imagetools create :v1.2.0 từ :sha-<short>  ← copy theo digest, không build lại
  job deploy  → chờ tối đa 5' cho ghcr devforge-web:v1.2.0 xuất hiện
                     thiếu → dừng, CHƯA chạm vào box      ← cổng xuyên repo
                →  ssh: git fetch --tags --force
                        git checkout --force --detach <tag>   (chỉ clone be)
                        IMAGE_TAG=<tag> ./deploy/up.sh
                           compose pull api web
                           pull 4 lab image → docker tag về devforge/<tên>:latest
                           up -d --wait postgres redis
                           migrate up                   (tương thích ngược)
                           up -d --wait --wait-timeout 120
                           xanh → ghi .image-tag
                        đỏ → IMAGE_TAG=$(cat .image-tag) up -d --wait  ← KHÔNG migrate lại
```

Repo `devforge-fe`, cùng hình dạng, không có job deploy:

```
job fe      → oxlint  →  tsc --noEmit  →  npm run check (8 file assert)  →  build
job secrets → gitleaks  →  trivy fs (package-lock.json)

push master → job image   → build --load → trivy image → push devforge-web:sha-<short>
push tag v* → job promote → tag phải trên master
                          →  imagetools create devforge-web:v1.2.0 từ :sha-<short>
```

⚠️ **Image lab được pull rồi `docker tag` về `devforge/<tên>:latest` ngay trên máy.**
Bảng `lab_images` giữ đúng tên đó (`scripts/seed.sql`) và API gọi Docker API theo
tên trong bảng. Đổi tên trong DB là một migration; retag tại chỗ là một dòng, tức
thì, và giữ được nguyên tắc *quét cái gì thì ship cái đó*.

**Bốn điều kiện, không cái nào tuỳ chọn:**

1. **`/readyz` phải trả đỏ khi API chưa nối được DB** — nếu không thì bước kiểm luôn xanh và rollback không bao giờ bắn. Đã có: `cmd/server/router.go:74` ping database timeout 2 giây, trả `503` khi hỏng. `/healthz` luôn `200`, chỉ nói tiến trình còn sống — **đừng dùng nó làm điều kiện rollback.**
2. **Migration chạy trước `up -d`, và phải tương thích ngược.** Rollback chỉ lùi image, không lùi schema.
3. **Rollback không chạy lại `migrate`.** Bản deploy hỏng có thể đã apply xong một migration; `migrate up` với `migrations/` cũ sẽ chết vì version đã apply không còn file — và chết *trước* khi kịp đưa image cũ trở lại.
4. **Secret**: `SSH_HOST`/`SSH_USER`/`SSH_KEY` (prod) + `STAGING_SSH_*` (staging), chỉ ở repo be. Repo fe không cần secret nào — `GITHUB_TOKEN` mặc định đủ `packages: write` cho GHCR cùng owner.

**Chỉ một đường vào box, và nó nằm ở repo be.** `concurrency` của GitHub Actions
tính theo từng repo nên không xếp hàng hai repo với nhau được; job deploy thứ hai
là một đường đua, dù có cổng tag hay không.

Việc của repo fe trong một bản phát hành đúng một thứ: promote image của chính nó
lên `:v1.2.0`. Job `deploy` bên be **chờ** image đó, tối đa 5 phút, rồi mới ssh.
`needs:` không bắc qua hai repo được, nhưng registry thì bắc được — fe chưa promote
thì image không tồn tại và deploy dừng trước khi chạm vào box.

⚠️ **`flock` vẫn cần, và đặt trên thư mục checkout chứ không phải file khoá.**
`concurrency` không biết gì về **người** đang ssh vào box, mà §13.4 bảo chạy
`up.sh` bằng tay. `exec 9<"$PWD"` rồi `flock -n 9`: một file dưới `/run/lock` sẽ
do người deploy đầu tiên tạo, một lần `sudo ./deploy/up.sh` để lại file
`root:root 0644` và mọi lần CD sau đó chết ở `Permission denied`.

**Merge vào `master` build, quét và lên staging — nhưng không lên prod.** Prod chỉ
ra bản mới khi có người gắn tag (`make release v=v1.2.0`). Hệ quả:

- `master` là nhánh *deploy được*, không phải nhánh *đã deploy*. Cái đang chạy trên prod là tag gần nhất, không phải HEAD của `master`.
- `promote` đòi `merge-base --is-ancestor`, nên tag cắt từ nhánh phụ bị từ chối kể cả khi commit đó xanh.
- Chỉ còn **một** clone trên mỗi box: repo fe là image, không được clone ở đó nữa. Bẫy detached-HEAD của lỗi 13.0#3 biến mất theo — rollback đổi `IMAGE_TAG`, không `git checkout`.
- Sau rollback, clone be vẫn nằm ở tag hỏng. Không sao — lần sau `checkout --force` chứ không `pull`. Nhưng file nginx và `migrations/` trên đĩa là bản mới trong khi image là bản cũ, nên cả hai bắt buộc tương thích ngược.

Tên tag đi thẳng vào một lệnh shell qua ssh, nên `ci.yml` chặn trước khi gửi: chỉ
nhận `v[0-9]…` gồm `[A-Za-z0-9._-]`. Bộ lọc `tags: ["v*"]` của GitHub lọc tên,
không lọc ký tự shell.

**Một release = một tag, gắn lên CẢ HAI repo.** Hai không gian tag độc lập:
`v1.2.0` bên be trỏ commit của be, bên fe trỏ commit của fe, **chỉ cái tên trùng
nhau**. Chỗ hai bên gặp nhau là GHCR, không phải git.

```bash
make release v=v1.2.0     # tag + push cả hai repo; từ chối nếu tag đã có hoặc cây bẩn
```

FE không đổi dòng nào vẫn phải tag: thiếu nó thì `devforge-web:v1.2.0` không tồn
tại và deploy dừng. Số hiệu đi cùng nhau — be nhảy `v1.3.0` mà fe ở lại `v1.2.0`
thì một cái tên chỉ hai thứ khác nhau tuỳ chỗ đọc.

⚠️ **Không bao giờ dời một tag đã phát hành.** Git cho `-f`, GHCR cho ghi đè. Dời
rồi thì `v1.2.0` hết tái lập được và rollback thành đoán. Sai thì cắt `v1.2.1`.

Repo public thì Actions không giới hạn phút (§9.3). **Đặt package GHCR sang Public
một lần trong UI**, nếu không box phải `docker login ghcr.io` bằng PAT.

### Quét bảo mật và chất lượng mã

Bốn thứ, và thứ tự giữa chúng không đổi được.

**1. Artifact phải có trước — ✅ đã có.** Trước đây image build **trên box**, nên trong CI không tồn tại image nào để quét, và rollback build lại từ source tức là **khác bit** với thứ vừa test xanh. Cả hai đã đóng: sáu image (`devforge-api`, `devforge-web`, bốn `devforge-lab-*`) build một lần trong Actions, quét ở đó, đẩy lên GHCR dưới `:sha-<short>`, và tag `v*` chỉ **đổi tên** chúng bằng `imagetools create` — copy theo digest, không build lại.

Không `:latest` trên prod. `latest` là thứ làm rollback hết tái lập được, và `docker-compose.prod.yml` đòi `IMAGE_TAG` bằng `${IMAGE_TAG:?}` chứ không đặt mặc định, đúng vì một mặc định là cách `latest` lên máy mà không ai chọn.

**2. Trivy — bề mặt thật là lab image, không phải API image.**

`devforge-api` chạy distroless nonroot: không shell, không package manager, không coreutils. Bốn image lab thì ngược lại, và `labs/linux/Dockerfile` nói thẳng: *"Whatever a student types runs in here, so it carries a shell and coreutils"*. Đó là image bạn phát cho người lạ gõ lệnh vào. Quét chúng trước.

⚠️ **Không block `HIGH,CRITICAL` trần.** Alpine và distroless lúc nào cũng có CVE chưa ra bản vá; block trần là CI đỏ vì thứ không ai sửa được, và ba tuần sau có người tắt job. Cấu hình đúng:

```
trivy image --severity HIGH,CRITICAL --ignore-unfixed --exit-code 1 <image>
trivy fs    --severity HIGH,CRITICAL --ignore-unfixed --exit-code 1 .
```

`.trivyignore` **bắt buộc ghi ngày hết hạn cho từng dòng**. Một dòng bỏ qua không có hạn là một dòng vĩnh viễn.

Quét `fs` bắt `go.sum` và `package-lock.json` — lỗ hổng ở dependency gặp thường xuyên hơn ở base image.

**3. Coverage — đang không đo ở bất kỳ đâu.** `go test ./...` chạy nhưng không có `-coverprofile`, không ở CI, không ở `Makefile`, không ở `lefthook.yml`. Đây là lỗ thật, và nó chặn luôn mọi Quality Gate: gate trên coverage của code mới cần một con số để gate.

`golangci-lint` gộp sẵn `go vet` + `staticcheck` + `errcheck` + ~40 linter khác, nên nó **thay** dòng `go vet` chứ không thêm vào.

**4. SonarQube — không tự dựng, không bàn thêm.** Nó là app JVM cần Postgres riêng, sàn 2–4 GB. Box là KVM2 2 vCPU và §9.4 đã chốt **CPU là trần**: ghế lab chính là sản phẩm. Dựng Sonar ở đó là lấy ghế học viên nuôi một cái dashboard.

**SonarQube Cloud** (SaaS, 0₫ cho repo public) là đường duy nhất còn lại. **Job `sonar` đã có ở cả hai repo** (`SonarSource/sonarqube-scan-action@v8.2.2` + `sonar-project.properties`), và nó **tự bỏ qua khi chưa có `SONAR_TOKEN`** — cùng cách `deploy`/`deploy-staging` bỏ qua khi chưa có secret SSH. Còn lại đúng một việc tay: tạo tài khoản, bind repo, dán token.

Cổng là thật chứ không phải báo cáo: `sonar.qualitygate.wait=true` trong `sonar-project.properties` làm job đỏ khi Quality Gate trượt. Không có dòng đó thì phân tích vẫn upload, dashboard vẫn đỏ, mà CI vẫn xanh.

⚠️ `sonar.projectKey` và `sonar.organization` là **bắt buộc** với Cloud và không đặt được trong UI. Giá trị trong file đang là quy ước (`TuqL3_<repo>` / `tuql3`) — đối chiếu với UI ở lần chạy đầu.

Cái SonarQube Cloud hơn `golangci-lint` + ngưỡng coverage là lịch sử, duplication và biểu đồ theo thời gian, cộng khái niệm **new code**: gate chấm phần diff chứ không chấm cả quá khứ tích tụ.

---

### Staging — VPS thứ hai

Lý do cần nằm ngay trong §9.5: hai lỗi mô tả ở đó *"chỉ nổ sau khi deploy (dev FE
gọi thẳng `:8080` nên không thấy)"*. Cùng loại với chúng, và cũng chỉ lộ ra sau
khi lên máy thật:

`TRUSTED_PROXIES` sau nginx **và** sau Cloudflare · `/uploads/*` với `PUBLIC_URL`
· OAuth redirect URI thật · cert Origin + mây cam · khối bot đứng trước route
bắt-tất-cả · migration ba bước tương thích ngược · và chính cái rollback theo tag.

**Đã chốt: một VPS thứ hai, loại nhỏ nhất.** Hai phương án rẻ hơn đều hỏng theo
cách không sửa được bằng cấu hình: không dựng staging thì mọi thứ trong danh sách
trên vẫn chỉ lộ ở prod; dựng stack thứ hai trên cùng máy thì cắt thẳng vào ghế lab
của prod (KVM2 có 2 vCPU, §9.4 chốt trần là CPU) **và** dính bẫy đếm ghế dưới đây.

⚠️ **Bẫy đếm ghế — lý do chính loại phương án cùng máy.**
`internal/labs/adapter/repo/session.go:183` đếm ghế từ **database của chính nó**,
không hỏi docker daemon:

```sql
SELECT count(*) FROM lab_sessions WHERE status = 'running' AND container_id <> ''
```

Hai stack chung một daemon nhưng hai database riêng → mỗi bên tin nó còn chỗ, và
cùng nhau đẻ gấp đôi số container. `MAX_CONTAINERS` là trần trên *một cơ sở dữ
liệu*, không phải trên *một cái máy*. Hai máy riêng thì không có cách nào ghép
được. (Reaper không dính: `DueForReaping`, `session.go:252`, cũng quét theo
`lab_sessions` của chính nó nên hai bên không dọn nhầm container của nhau.)

Hai máy chạy **cùng một `docker-compose.yml` + `docker-compose.prod.yml` +
`deploy/up.sh`**, khác đúng ở file `.env`. Không có `docker-compose.staging.yml`
và không có `.env.staging.example` — hai template trăm dòng thì cái ít deploy hơn
sẽ lệch, và nó luôn là staging. Các dòng phải đổi nằm ở cuối `.env.prod.example`.

| | prod | staging |
| --- | --- | --- |
| Máy | KVM2, 2 vCPU / 8 GB | VPS nhỏ nhất, máy riêng |
| Domain | `<domain>` | `staging.<domain>` — bản ghi A riêng, mây cam |
| Postgres | volume riêng + `pg_dump` → R2 | volume riêng, **không backup** |
| `MAX_CONTAINERS` | `12` | `4` |
| `LAB_SESSION_TTL` | `20m` | `10m` |
| `OPENROUTER_API_KEY` | (rỗng, xem §9.6) | rỗng — đừng đốt tiền AI ở staging |
| Email | Resend / Brevo | Mailpit, `COMPOSE_PROFILES=dev` |
| Trigger deploy | push tag `v*` | push `master` |
| Image | `:v1.2.0` | `:master` — **cùng bit**, khác tên |

Dòng cuối là điểm quan trọng nhất: cả ba tên — `:sha-<short>`, `:master`,
`:v1.2.0` — trỏ vào **cùng một digest**. `promote` dùng `imagetools create`,
copy theo digest chứ không build lại. "Đã test ở staging" vì thế là câu đúng về
bit, không phải về commit.

Staging deploy bằng `:master` chứ không bằng `:sha-<short>` vì `up.sh` deploy cả
sáu image dưới **một** `IMAGE_TAG`, mà sha của repo be không phải sha của repo
fe — chi tiết ở §8 đầu mục.

⚠️ **Mailpit chỉ nghe `127.0.0.1`** (`docker-compose.yml`), vào hộp thư qua tunnel:

```bash
ssh -L 8025:127.0.0.1:8025 <user>@<staging host>    # rồi mở localhost:8025
```

Đừng đổi sang `0.0.0.0`. Mail catcher trên địa chỉ công khai là hộp thư ai cũng
đọc được, mà trong đó có mã xác minh và link đặt lại mật khẩu.

Thứ tự: **dựng prod cho xong trước.** Staging tồn tại để diễn tập một pipeline đã
có; dựng nó trước là diễn tập cho thứ chưa viết.

---

## 9. Hạ tầng production

> **Đã đổi phương án.** Oracle Always Free / Caddy là thiết kế cũ và **chưa từng
> dựng thật**. Quyết định hiện tại: **hai VPS Hostinger (prod + staging), nginx ở
> biên, FE và API cùng một origin**, image build ở Actions và pull từ GHCR. Việc
> phải làm để chuyển sang nằm ở §13.

### 9.0 Toàn cảnh

```
                 Cloudflare — DNS · TLS · WAF · DDoS
                 cache CHỈ cho assets tĩnh, KHÔNG cho /api /ws /uploads (§9.5)
                              │  HTTPS 443
        ┌─────────────────────┴─────────────────────┐
        ▼                                           ▼
   <domain>                                  staging.<domain>
   ┌────────────────────────────────────┐    ┌──────────────────────┐
   │ VPS prod — 2 vCPU / 8 GB, SG       │    │ VPS staging — nhỏ    │
   │                                    │    │ cùng compose,        │
   │  nginx  real_ip CF-Connecting-IP   │    │ khác .env            │
   │    │    /ws/: buffering off        │    │ image :sha-<short>   │
   │    ├──► web  (SPA tĩnh)            │    │ không backup         │
   │    └──► api  (Go, distroless)      │    │ mailpit ở 127.0.0.1  │
   │           ├── postgres  loopback   │    └──────────────────────┘
   │           ├── redis     loopback   │
   │           └── docker-socket-proxy  │
   │             CONTAINERS·POST·EXEC   │
   │             └── lab container ×N   │
   │                     --network=none │
   │                     --cap-drop=ALL │
   │                                    │
   │  (sau này) Alloy → Grafana Cloud   │
   └────────────────────────────────────┘
         │                    │
   pg_dump → R2         Sentry (chưa bật)
   + diễn tập restore

   NGOÀI hạ tầng:  UptimeRobot ping /readyz  ← không bao giờ đặt trên chính VPS (§9.9)
```

Luồng deploy:

```
push master → CI lint·test·coverage·gitleaks·trivy
            → build 6 image → quét → ghcr :sha-<short>
            → ssh staging → up.sh → pull, migrate, up -d --wait

tag v*      → promote :sha-<short> → :v1.2.0   (đổi tên, không build lại)
            → chờ image fe cùng tag → ssh prod → up.sh
            → không healthy trong 120s → IMAGE_TAG=$(cat .image-tag) up -d --wait
```

Ba thứ phân biệt sơ đồ này với một sơ đồ "VPS + Docker Compose" thông thường, và
cả ba đều là ràng buộc của chính sản phẩm chứ không phải sở thích:

1. **`docker-socket-proxy` + lab container.** App tự đẻ container — đó là sản phẩm. Kéo theo trần số container, reaper, và lý do không PaaS nào dùng được.
2. **Không có worker/queue.** Reaper là một goroutine, email gửi đồng bộ. Thêm BullMQ hay một service worker lúc này là thêm tiến trình phải giám sát cho việc chưa tồn tại.
3. **FE nằm cùng box, cùng origin.** Đây là lựa chọn, không phải mặc định: nó tốn gần như không CPU (phát file tĩnh, không build) và đổi lại là một image FE promote được từ staging lên prod — xem §9.1.1.

### 9.1 Máy chủ: Hostinger VPS

Lý do rời Oracle không phải tiền. Ba thứ phức tạp nhất của bản cũ **chỉ tồn tại vì
chọn ARM free tier**: image phải build trên chính máy prod (runner Actions là
amd64, giả lập arm64 qua QEMU chậm gấp cả chục lần) → không có registry → rollback
phải build lại → `deploy/up.sh` phải tự bắt health, tự `git reset`, tự prune, ~90
dòng bash tự viết. Bỏ Oracle thì cả ba tự biến mất, xem §9.2.

| | Hostinger KVM2 | Dự án cần |
| --- | --- | --- |
| CPU | 2 vCPU amd64 | 2+ |
| RAM | 8 GB | ~5 GB |
| Disk | 100 GB NVMe | ~60 GB |
| Vùng | **Singapore** | Singapore |
| Giá | kiểm lúc mua — giá khuyến mãi và giá gia hạn chênh nhau nhiều | — |

Vẫn phải là **VPS thật có root, cài được Docker**: code đẻ container qua Docker
Engine API, nên mọi PaaS (Vercel, Netlify, Render, Railway, Fly.io) loại ngay từ
đầu. Điều đó **chỉ đúng với backend** — FE là SPA tĩnh nên đi đường khác, xem
§9.1.1.

Singapore → VN 30–50ms. Ràng buộc cứng, không phải sở thích: terminal là xterm.js
qua WebSocket, mỗi phím gõ là một vòng round-trip. Hetzner rẻ hơn nhưng chỉ có
EU/US (250–300ms) → hỏng đúng tính năng cốt lõi.

Chọn **template OS có sẵn Docker** (Ubuntu 24.04 + Docker) → bỏ được nửa
`deploy/bootstrap.sh`.

⚠️ **Hostinger có tường lửa riêng trong hPanel.** Bài toán hai lớp cửa của Oracle
VCN quay lại nguyên vẹn: phải mở 80/443 ở hPanel **và** ở `ufw`. Quên lớp nào thì
máy cũng không ai vào được, và `ufw status` sẽ nói mọi thứ đều ổn.

**Bật snapshot/backup tuần của Hostinger** — nhưng nó *không* thay
`scripts/backup.sh` → R2. Snapshot nằm cùng nhà cung cấp: mất tài khoản là mất cả
hai. Snapshot để khôi phục nhanh, R2 để sống sót.

#### 9.1.1 Frontend ở lại trên box, cùng origin

> **Cloudflare Pages đã bị loại, và không phải vì giá — Pages rẻ hơn.** Ghi lại để
> không ai đề xuất lại: lý do là **artifact**.

Vite bake `VITE_API_URL` lúc build. Tách hai origin thì giá trị bake vào staging
khác giá trị bake vào production — hai bản build cho một commit, và "build một
lần, promote nhiều lần" ở nửa FE thành cái nhãn dán. Diễn tập ở staging xong thì
thứ lên prod vẫn là bit chưa ai chạy.

Cách thoát: **không bake gì cả.** `Dockerfile` ghim `ENV VITE_API_URL=""`, bundle
gọi đường dẫn tương đối, biên route `/api`, `/ws`, `/uploads` sang API trên cùng
origin — đúng thứ `deploy/nginx/devforge.conf` làm. Điều kiện là FE ở lại sau cùng cái
proxy đó.

Được thêm: cookie phiên hết bài SameSite/CORS/subdomain, và khối `@crawler` cho
thẻ `og:` không phải viết lại thành Pages Function. Static vẫn được Cloudflare
cam cache ở biên.

Mất: **preview mỗi pull request**. Đổi lại là staging thật (§8) — thứ preview của
Pages không thay được, vì nó chạy trên `*.pages.dev`, khác registrable domain với
API, nên cookie `Lax` không đi qua và **không đăng nhập được**.

Câu ở §9 *"mỗi vCPU dành cho FE là một vCPU lấy khỏi lab container"* đúng cho
**build**, không đúng cho **phát file tĩnh** — mà từ giờ không có build nào chạy
trên box nữa.

### 9.2 amd64 + GHCR — image không còn build trên máy prod

Hostinger là amd64, runner GitHub cũng amd64. Lý do duy nhất để build trên box đã
hết.

```
push master → Actions build → ghcr.io/<owner>/devforge-api:<sha>
            → ssh: docker compose pull && docker compose up -d --wait
rollback    → IMAGE_TAG=<sha cũ> docker compose up -d --wait     ← vài giây
```

Lợi, theo thứ tự quan trọng:

1. **Rollback từ 1–2 phút xuống vài giây.** Không build lại, chỉ đổi tag.
2. **Box 2 vCPU không phải vừa build vừa chạy lab.** Đây là điểm chết thấy rõ nhất
   trên máy nhỏ: học viên đang gõ trong terminal thì máy đi compile.
3. `up.sh` mất hẳn phần build, phần `healthy()` tự viết thay bằng
   `docker compose up -d --wait --wait-timeout 120`, còn khoảng 35 dòng.

Điều kiện: **đặt package GHCR sang Public một lần trong UI**, nếu không box phải
`docker login ghcr.io` bằng PAT. Repo đã public thì không có gì để giấu.

**Image lab cũng đi qua GHCR.** Bốn image `devforge/{linux,git,docker,net}` không
nằm trong compose — API tạo container từ tên image qua Docker API. Chúng build và
được Trivy quét trong Actions như hai image kia, rồi `up.sh` pull về và
`docker tag` lại thành đúng cái tên bảng `lab_images` đang giữ. Build lại trên box
thì nhanh hơn vài giây, nhưng là bit khác với bit đã quét — mà đây đúng là bốn
image có shell và người lạ gõ vào (§8 "Quét bảo mật").

Không chỗ nào trong repo ghim kiến trúc (`CGO_ENABLED=0`, Go tĩnh thuần, `apk` tự
phân giải), nên quay lại ARM sau này cũng không phải sửa dòng nào. Bảng đối chiếu
arm64 của bản cũ bỏ đi vì không còn tác dụng.

### 9.3 Bảng chi phí — không còn 0₫

| Khoản | Dịch vụ | Giá |
| --- | --- | --- |
| **Máy chủ prod** | Hostinger KVM2 Singapore | **trả tiền** — kiểm lúc mua |
| **Máy chủ staging** | VPS loại nhỏ nhất, máy riêng (§8 "Staging") | **trả tiền** — ⏳ chưa mua |
| **Tên miền** | | **~300k₫/năm** |
| TLS | Cloudflare Origin Certificate | 0₫ |
| DNS + proxy | Cloudflare Free | 0₫ |
| FE hosting | cùng VPS, cùng origin (§9.1.1) | 0₫ |
| Registry | GHCR (package public) | 0₫ |
| CI/CD | GitHub Actions (repo public, không giới hạn phút) | 0₫ |
| Quét lỗ hổng | Trivy (OSS, chạy trong Actions) | 0₫ |
| Lint + coverage | golangci-lint (OSS, chạy trong Actions) | 0₫ |
| Chất lượng mã | SonarQube Cloud, repo public — job `sonar` đã có, chờ `SONAR_TOKEN`, §8 | 0₫ |
| Backup | Cloudflare R2, 10 GB, egress 0₫ | 0₫ |
| Email | Resend 3000/tháng hoặc Brevo 300/ngày | 0₫ |
| Metrics + log + alert | Grafana Cloud Free | 0₫ — ⏳ chưa bật, §1 |
| Analytics | Umami Cloud | 0₫ — ⏳ chưa bật, §1 |
| Sinh kịch bản sim | OpenRouter | xem §9.6 |

⚠️ Không có dòng **SonarQube tự dựng** trong bảng này, và đó là chủ ý: giá của nó không phải tiền thuê mà là 2–4 GB cộng phần CPU lấy thẳng từ ghế lab — xem §8 "Quét bảo mật và chất lượng mã" và §9.4.

**Chi phí cố định = VPS + tên miền.** Mọi thứ còn lại vẫn 0₫. Đây là khoản đánh
đổi lấy: không còn chờ "out of host capacity", không còn nguy cơ bị thu hồi máy,
không còn ARM.

### 9.4 Sức chứa: CPU là trần, không phải RAM

2 vCPU thay cho 4 OCPU → cắt đôi mọi con số.

```
12 container × 512 MB (trần Docker) = 6 GB    ← trần lý thuyết
12 container × ~150 MB (RSS thật)   = 1,8 GB  vs ~6 GB còn trống → thoải mái
12 container × 0.5 vCPU (nanoCPUs)  = 6 vCPU  vs 2 nhân thật     → oversubscribe 3×
```

`Memory` của Docker là **trần, không phải đặt chỗ**; image nền là alpine chạy
`sleep infinity` + shell, RSS thật 20–60 MB lúc rảnh. `NanoCPUs` cũng là hạn ngạch
(cfs_quota), nên oversubscribe không sao khi container rảnh.

Nhưng **chỉ ~4 container bận CPU cùng lúc** trên 2 nhân. Với lab dạy học (phần lớn
thời gian học viên đang gõ và đọc) thì `MAX_CONTAINERS=12` là ổn. Thấy chậm thì hạ
xuống 8, đừng nâng lên.

Ba đòn bẩy tăng số người phục vụ được, đều là biến môi trường hoặc thiết kế nội
dung — không sửa code:

1. **Bài mô phỏng tốn 0 container.** Bốn engine sim (CI/CD, Linux, tìm kiếm, sắp
   xếp) không chiếm ghế nào; kiểm tra sức chứa nằm *sau* nhánh sim trong `labs.go`
   một cách có chủ đích. Xếp sim lên trước container trong lộ trình học → nhân đôi
   số người vào cùng lúc.
2. **`LAB_SESSION_TTL` 60m → 20m.** Reaper quét theo deadline này, nên vòng quay
   ghế nhanh gấp ba.
3. **`MAX_CONTAINERS`** điều chỉnh theo tải thật đo được, không theo cảm giác.

Cần hơn nữa thì lên **KVM4 (4 vCPU / 16 GB)** → `MAX_CONTAINERS=25`. Đây là cái
núm đúng, không phải nới trần trên máy cũ.

### 9.5 ⚠️ `TRUSTED_PROXIES` — giờ có HAI tầng proxy, không phải một

Mọi thứ nhận dạng người gọi đều đọc `c.ClientIP()`: rate limit theo địa chỉ trên
các route chia sẻ công khai (`PUBLIC_RATE_LIMIT`), cột `audit_logs.ip`, và địa chỉ
trên màn hình thiết bị đang đăng nhập. Giá trị đó chỉ đúng khi server biết ai được
phép nói thay người khác qua `X-Forwarded-For`.

**Bản cũ dặn giữ Cloudflare ở chế độ DNS-only (mây xám). Điều đó nay ngược lại:**
TLS là Cloudflare Origin Certificate, mà cert đó **chỉ Cloudflare tin**. Tắt mây
cam là trình duyệt báo cert không hợp lệ ngay lập tức. Mây cam trở thành thành
phần bắt buộc, và chuỗi request dài thêm một chặng:

```
browser → Cloudflare edge → nginx (container) → api (container)
```

Hai việc phải làm, thiếu một cái là hỏng:

1. **nginx phải khôi phục IP thật.** `set_real_ip_from <các dải IP Cloudflare>` +
   `real_ip_header CF-Connecting-IP`. Không có thì `X-Forwarded-For` nginx gửi
   sang API mang địa chỉ của edge node Cloudflare, không phải của người dùng. Dải
   IP đổi theo thời gian: `curl -s https://www.cloudflare.com/ips-v4`.
2. **`TRUSTED_PROXIES=127.0.0.1,::1,172.16.0.0/12`.** nginx nói chuyện với api qua
   mạng bridge của compose nên peer mà Gin thấy là `172.x.x.x`.

Để mặc định thì hỏng hai chỗ, cả hai chỉ nổ sau khi deploy (dev FE gọi thẳng
`:8080` nên không thấy):

| Chỗ hỏng | Triệu chứng |
| --- | --- |
| `internal/labs/adapter/ratelimit/redis.go` — key là `c.ClientIP()` | `PUBLIC_RATE_LIMIT` 60/phút **theo địa chỉ** biến thành 60/phút **toàn cục**. Một người xem link chia sẻ làm cạn quota của tất cả. Self-DoS trên đúng bốn route sinh ra để đón người lạ. |
| `audit_logs.ip`, `sess.ip` (màn hình thiết bị) | Mọi hàng ghi cùng một IP. Màn hình "thiết bị đang đăng nhập" mất ý nghĩa, audit mất dấu vết IP. |

Chỉ mở tới dải bridge khi **API không publish cổng nào ra ngoài** và nginx là lối
vào duy nhất. Lab container chạy `--network=none` nên không tự nói chuyện được với
API, nhưng điều kiện trên vẫn phải giữ.

**Không bao giờ đặt `0.0.0.0/0`.** Tin mọi peer nghĩa là người gọi tự chọn địa chỉ
của mình bằng cách gửi header — rate limit và dấu vết audit giao lại cho bất cứ ai
hỏi. Server ghi `slog.Warn` lúc khởi động nếu thấy giá trị này, nhưng vẫn chạy:
một server không chịu boot vì cấu hình proxy thì làm sập site theo hướng ngược
lại.

Hành vi được ghim bởi `cmd/server/trustedproxies_test.go`, và nửa còn lại —
`set_real_ip_from` + `real_ip_header` — bởi `make check-edge`, ca "a forged
X-Forwarded-For is replaced, not passed through".

⚠️ **Cache rule phải theo PATH, không theo host.** FE và API dùng chung một tên miền (§9.1.1), nên không còn cách tách bằng host nữa. Mây
cam bật lên là có CDN, và đó là thứ tốt cho `assets/*` của SPA. Với đường dẫn API thì
ngược lại:

- Bật "Cache Everything" trên đường API là **phát response có `Set-Cookie` của
  người này cho người khác**. Mặc định Cloudflare không cache response có
  `Set-Cookie`, nên đây là tai nạn do người tự bật rule, không phải mặc định — mà
  đó chính là loại tai nạn khó tin nhất khi nó xảy ra.
- Cache rule sai còn phá được bước nâng cấp lên WebSocket của `/ws/*`.

Quy tắc: cache rule chỉ phủ `assets/*` và `index.html`. Trên `/api/*`, `/ws/*`, `/uploads/*` để
mặc định (bypass), và chỉ bật WAF chứ không bật cache.

### 9.6 Chi phí AI — khoản duy nhất có thể vượt mặt hạ tầng

Tính năng nhờ AI dựng kịch bản mô phỏng gọi OpenRouter. Đo được: prompt hệ thống 12,4 KB → ~4k token input; `maxTokens = 16000`, kịch bản thực tế ~3k token output.

| Model | Input $/1M | Output $/1M | 1 lượt sinh |
| --- | --- | --- | --- |
| `anthropic/claude-opus-5` | $5 | $25 | ~$0,095 ≈ **2.500₫** |
| `anthropic/claude-sonnet-5` | $3 | $15 | ~$0,057 ≈ 1.500₫ |
| `anthropic/claude-haiku-4-5` | $1 | $5 | ~$0,019 ≈ **500₫** |

Với 100 học viên, 20% dùng, ~3 lượt/ngày: opus-5 ≈ **4,4tr₫/tháng**, haiku-4.5 ≈ **900k₫/tháng**. Ở trần cứng (`AI_DAILY_LIMIT=10`, tất cả dùng hết) opus-5 lên tới **74tr₫/tháng**. Tiền AI vượt tiền server ở mọi kịch bản.

Ba việc trước khi bật:

1. `OPENROUTER_MODEL=anthropic/claude-haiku-4-5`. Sinh kịch bản là điền JSON theo schema — không cần model mạnh nhất. Model phải hỗ trợ `response_format` (structured outputs), nếu không request bị từ chối định tuyến; phần lớn model gắn `:free` trên OpenRouter không hỗ trợ, nên không có đường đi hoàn toàn miễn phí ở đây.
2. `AI_DAILY_LIMIT=3` cho tới khi nhìn thấy số thật một ngày.
3. **Để trống `OPENROUTER_API_KEY` khi mới lên.** Code đã xử lý: thiếu key hoặc thiếu model thì tính năng tắt, riêng endpoint đó trả 503, server chạy bình thường. Đây là mặc định đúng cho lần deploy đầu.

### 9.7 Rủi ro của phương án này

1. **Một máy prod, không HA.** Máy chết là cả FE lẫn API chết cùng lúc — cùng
   origin nghĩa là không còn nửa nào ở hạ tầng khác để sống sót và hiện một trang
   lỗi tử tế. Staging không đỡ được: nó là máy diễn tập, không phải máy dự phòng.
2. **Backup là chặng ĐẦU TIÊN, không phải chặng cuối.** Snapshot Hostinger phục
   hồi nhanh nhưng nằm cùng nhà cung cấp; `pg_dump` → R2 là thứ sống sót khi mất
   tài khoản. Cron đã nằm trong `bootstrap.sh`, nhưng **diễn tập restore vẫn là
   việc phải làm bằng tay** — `./scripts/restore.sh` — và bản backup chưa ai
   restore là một phỏng đoán.
3. **Cloudflare thành điểm chết đơn.** Mây cam tắt (hoặc tài khoản có vấn đề) là
   cert Origin không còn hợp lệ và site đứt. Đường lùi có sẵn: đổi sang certbot +
   Let's Encrypt, xem §9.8.
4. **Giá gia hạn Hostinger, nhân hai.** Giá khuyến mãi chu kỳ đầu và giá gia hạn
   chênh nhau nhiều, và giờ có hai máy với hai ngày hết hạn. Ghi cả hai vào lịch.
5. **Máy staging là bề mặt tấn công thứ hai.** Cùng codebase, cùng cổng mở, nhưng
   không ai theo dõi và không có backup. Nó phải được `bootstrap.sh` và `ufw` đối
   xử đúng như prod — một máy staging bị chiếm vẫn là một máy trong tay người lạ,
   và nó có khoá SSH của CD.

### 9.8 Các đường lùi

| Nếu | Đổi sang | Đánh đổi |
| --- | --- | --- |
| Không muốn phụ thuộc Cloudflare proxy | **certbot + Let's Encrypt**, mây xám | Được giữ DNS-only, nhưng thêm ba phần động phải bảo trì: client, cron gia hạn, hook reload nginx |
| Muốn nhà cung cấp nhiều tài liệu hơn | **DigitalOcean / Vultr Singapore** | Đắt hơn ~1,5–2×, bù lại tài liệu và snapshot 1 nút |
| Cần độ trễ thấp nhất | **VPS Việt Nam** (AZDIGI/Vietnix) | <20ms, thanh toán nội địa, đắt hơn |
| Cần nhiều CPU nhất trên mỗi đồng | **Contabo Singapore** 4 vCPU/8 GB | Rẻ nhất nhóm, nhưng hay oversell và support chậm |
| Chấp nhận lại 0₫ | Quay về **Oracle Always Free A1** | Kéo cả ba thứ phức tạp ở §9.1 quay lại: build trên box, không registry, rollback build lại |

### 9.9 Bẫy của việc tự dựng trên một VPS

Năm cái dưới đây không phải kiến trúc — chúng là những chỗ một sơ đồ đúng vẫn hỏng lúc dựng thật.

#### 1. 🔴 Uptime monitor phải nằm NGOÀI hạ tầng nó giám sát

Đây là lỗi phổ biến nhất khi tự dựng: cài Uptime Kuma bằng docker ngay trên máy prod. VPS chết → monitor chết theo → **không ai báo gì**. Thứ duy nhất nó phát hiện được là "một container chết trong khi máy vẫn sống", tức đúng trường hợp ít nghiêm trọng nhất.

Dùng dịch vụ ngoài: **UptimeRobot / BetterStack / Cloudflare Health Check**, bản free, ping `https://<domain>/readyz` mỗi 5 phút, báo về Telegram.

`/readyz` chứ không phải `/healthz` — cùng lý do như điều kiện rollback ở §8: `/healthz` xanh cả khi database đã chết.

#### 2. 🔴 Docker publish port thì `ufw` không chặn được

Docker chèn luật iptables **trước** ufw. Nghĩa là:

```yaml
ports: ["5432:5432"]              # ❌ Postgres mở ra internet, dù `ufw status` nói đã chặn
ports: ["127.0.0.1:5432:5432"]    # ✅ chỉ loopback của host
expose: ["5432"]                  # ✅ chỉ trong mạng compose, tốt hơn nữa
```

Đây là cách rò rỉ database phổ biến nhất trên VPS tự dựng, và `ufw status` không hề nói dối — nó chỉ không phải nơi luật đó được áp.

`docker-compose.yml` hiện tại **đã đúng**: postgres, redis và docker-proxy đều ghim `127.0.0.1:`. Đừng bỏ tiền tố đó đi để "cho tiện debug từ máy khác" — dùng SSH tunnel.

#### 3. 🟠 Log của Docker mặc định không có trần

`json-file` không giới hạn dung lượng. Một lab container log loạn ăn hết đĩa và kéo Postgres chết theo. `deploy/bootstrap.sh` **đã xử lý**:

```json
{ "log-driver": "json-file", "log-opts": { "max-size": "10m", "max-file": "3" } }
```

Ghi lại ở đây vì nó là thứ dễ bị bỏ khi dựng máy bằng tay thay vì chạy script.

#### 4. 🟠 Luôn có hai tường lửa

Tường lửa của nhà cung cấp (Hostinger hPanel, Oracle VCN, DO Cloud Firewall) là cửa thứ nhất; `ufw` là cửa thứ hai. Phải mở 80/443 ở **cả hai**. Quên cửa nào thì máy cũng không ai vào được, và lớp còn lại sẽ báo là mọi thứ đều ổn.

#### 5. 🟡 Một proxy, và một server tĩnh — không phải hai proxy

Sơ đồ tự dựng hay có nginx ở biên (TLS + routing) **và** một nginx nữa bên trong làm "proxy cho frontend/backend". Hai tầng proxy trên cùng một máy không thêm gì ngoài một hop, một file config nữa phải đồng bộ, và một chỗ nữa để `X-Forwarded-For` bị đứt.

Ở đây **có hai tiến trình phục vụ HTTP, nhưng chỉ một cái proxy**: biên làm TLS và routing, còn image `devforge-web` chỉ `file_server` cho `dist` trên loopback nội bộ — nó không proxy đi đâu và không đọc `X-Forwarded-For`. Cái giá là một hop loopback; cái được là FE ship dưới dạng một artifact tự chứa, promote được như image api. Đừng biến `devforge-web` thành proxy — lúc đó nó mới thành cái bẫy ở trên.

---

## 13. Việc cần làm — mua máy

Đây là danh sách quyết định. Mọi thứ ở §8 và §9 mô tả **đích**. Khoảng cách phần mềm đã đóng: Caddy → nginx xong, `deploy/caddy/` đã xoá, `bootstrap.sh` và `.env.prod.example` khớp máy Hostinger. **Còn lại đúng một thứ chặn: chưa có máy.**

### 13.0 Lỗi đã có sẵn trên đĩa — đã sửa hết

Mười một lỗi (mười trong lần rà đầu, #3 lộ ra khi rà lại), sửa xong 2026-09-06.
Giữ lại đây một dòng mỗi lỗi vì lý do vẫn còn giá trị; chi tiết nằm trong comment
ở chính file đã sửa.

| # | Chỗ | Lỗi | Trạng thái |
| --- | --- | --- | --- |
| 1 | `devforge-fe/Dockerfile` | Thiếu `ARG VITE_API_URL` → Docker bỏ qua build-arg không khai báo → bundle rơi về `http://localhost:8080`, trang https gọi http localhost, mixed-content chặn, app chết | ✅ vá **mạnh hơn bản đầu**: `ENV VITE_API_URL=""` cố định, không còn build-arg nào để quên |
| 2 | `docker-compose.prod.yml` | Truyền `VITE_API_URL: https://${DOMAIN}/api` mà `path` đã có sẵn `/api/...` → mọi request thành `/api/api/...` → 404 | ✅ compose không truyền biến này nữa; không còn giá trị nào để đặt sai |
| 3 | `deploy/up.sh` | Rollback `git checkout <sha>` để lại detached HEAD → `git pull --ff-only` lần sau từ chối chạy → CD hỏng vĩnh viễn | ✅ rollback đổi sang `IMAGE_TAG`, không `git checkout`; CD `checkout --force` chứ không `pull` |
| 4 | `deploy/up.sh` | Rollback chạy lại `migrate up` với `migrations/` cũ → chết vì version đã apply không còn file → đứt giữa sự cố | ✅ cờ `skip-migrate` trên nhánh rollback |
| 5 | `scripts/restore.sh` | Dòng cuối `[ ... ] && echo` → nhánh `--into-live` cho false → exit 1 dù restore thành công, đúng lúc đang có sự cố | ✅ đổi thành `if ... fi` |
| 6 | Cả hai repo | Không có `.dockerignore` → `COPY . .` nuốt `.env` prod vào layer cache | ✅ thêm cả hai |
| 7 | `scripts/backup.sh` | Cron chỉ nằm trong comment, không ai cài → làm đúng quy trình vẫn ra không có backup nào | ✅ `/etc/cron.d/devforge-backup` trong `bootstrap.sh` |
| 8 | `.env.prod.example` | Nói "CI writes it from GitHub Secrets" — ngược với §8 và với `ci.yml` | ✅ xoá |
| 9 | `.env.prod.example` | `DB_PASSWORD` ở hai chỗ → điền một quên một → `migrate` chết trên máy không ai nhìn | ✅ `DATABASE_URL` nội suy từ `DB_*` |
| 10 | `ci.yml` job deploy | `ssh` không timeout → box treo giữ runner 6 tiếng | ✅ `timeout 1800`, `ConnectTimeout=10`, `BatchMode=yes` |
| 11 | `devforge-fe` terminal | `ws.onclose` không thử lại → mỗi lần deploy đá mọi học viên khỏi terminal đang mở | ✅ reconnect có backoff, phân biệt close frame với đứt 1006 |

### 13.1 Việc phải làm bằng tay (cần đăng nhập, không script hoá được)

| | Việc | Ghi chú |
| --- | --- | --- |
| ☐ | Mua VPS Hostinger **KVM2, vùng Singapore**, template Ubuntu 24.04 **có sẵn Docker** | Máy prod. Kiểm giá gia hạn, không chỉ giá khuyến mãi |
| ☐ | Mua **VPS thứ hai, loại nhỏ nhất**, cùng vùng | Máy staging (§8 "Staging"). Máy riêng chứ không phải stack thứ hai — lý do ở đúng mục đó |
| ☐ | Mở **80 và 443 trong tường lửa hPanel** | Lớp cửa thứ nhất — `ufw` là lớp thứ hai, quên lớp nào cũng không vào được (§9.1) |
| ☐ | Bật **snapshot/backup tuần** của Hostinger | Không thay R2, xem §9.7 |
| ☐ | DNS Cloudflare: `<domain>` A → IP VPS prod, `staging.<domain>` A → IP VPS staging | **Cả hai mây cam.** Không còn bản ghi `api.` nào để tạo (§9.1.1) |
| ☐ | Tạo **Cloudflare Origin Certificate** cho `<domain>` + `*.<domain>` | Lưu vào `deploy/nginx/certs/`, key `chmod 600` |
| ☐ | Đặt package GHCR (`devforge-api`, `devforge-web`, 4 `devforge-lab-*`) sang **Public** | ❗Không làm thì box phải `docker login ghcr.io` bằng PAT chỉ để pull bản phát hành của chính mình |
| ☐ | Google Console: đăng ký **hai** redirect URI — `https://<domain>/api/auth/google/callback` và `https://staging.<domain>/...` | Khớp từng ký tự. Thiếu cái thứ hai thì OAuth chết đúng ở staging |
| ☐ | Tạo bucket **R2** + lifecycle xoá sau 30 ngày | Nếu chưa có |
| ☐ | Secrets repo **be**: `SSH_HOST`, `SSH_USER`, `SSH_KEY` (prod) | Khoá deploy riêng, không dùng lại khoá cá nhân |
| ☐ | Secrets repo **be**: `STAGING_SSH_HOST`, `STAGING_SSH_USER`, `STAGING_SSH_KEY` | Job `deploy-staging` **tự bỏ qua** khi chưa có `STAGING_SSH_HOST`, nên master vẫn xanh trước lúc mua máy |
| ☐ | Repo **fe**: không cần secret nào | `GITHUB_TOKEN` mặc định đủ quyền `packages: write` cho GHCR cùng owner |
| ☐ | **Uptime monitor ngoài** ping `https://<domain>/readyz`, báo Telegram | 5 phút, và là thứ duy nhất báo được "máy chết" (§9.9). Đừng cài trên chính VPS. Không cần monitor cho staging |
| ☐ | Kiểm cache rule Cloudflare **không phủ `/api/*`, `/ws/*`, `/uploads/*`** | Cùng origin nên cache rule phải theo path chứ không theo host. Cache `/api/*` là phát dữ liệu người này cho người kia (§9.5) |
| ☐ | Bật **Authenticated Origin Pulls** ở Cloudflare, rồi bỏ comment hai dòng trong `deploy/nginx/devforge.conf` | Origin Cert chỉ chứng minh server với Cloudflare; nó **không** ngăn ai tìm ra IP thật rồi gọi thẳng, bỏ qua WAF và rate limit. Thứ tự bắt buộc: bật ở dashboard **trước**, xác nhận site còn phục vụ, rồi mới sửa nginx. Ngược lại là 400 cho tất cả |
| ☐ | Sentry cho Go + React | Lỗi runtime kèm stacktrace, rẻ hơn nhiều so với dựng cả stack quan sát |

### 13.2 Repo `devforge-be` — file phải sửa

| | File | Việc |
| --- | --- | --- |
| ✅ | `deploy/nginx/devforge.conf` | **Mới, đã viết.** Một site duy nhất (`server_name _`, box chỉ phục vụ API): 80 → 301 sang 443; cert Origin; `set_real_ip_from` các dải Cloudflare + `real_ip_header CF-Connecting-IP`; `location /ws/` với `proxy_buffering off` + timeout 3600s; `location /`; `client_max_body_size` cho upload ảnh. ⚠️ Upstream phải đi qua biến + `resolver 127.0.0.11` — nginx cache DNS vĩnh viễn, container `api` mới sau mỗi deploy sẽ nhận 502 nếu không |
| ✅ | `deploy/nginx/certs/.gitignore` | **Mới.** `*` + `!.gitignore` — khoá riêng không bao giờ vào git |
| ✅ | `deploy/caddy/` | Đã xoá cả thư mục. Bản dev trong đó vốn đã mồ côi — không compose file nào mount nó |
| ✅ | `docker-compose.prod.yml` | `api` và `web` đổi từ `build:` sang `image: ${IMAGE_REPO}/devforge-{api,web}:${IMAGE_TAG}`, **không còn khoá `build:` nào** — có nó thì `up` lặng lẽ build lại khi thiếu image, tức là box quay về compile bản phát hành. `IMAGE_TAG` dùng `${IMAGE_TAG:?}` chứ không mặc định. Service `web` **ở lại** (§9.1.1 đảo quyết định Pages). Service `caddy` đã đổi thành `nginx`: mount `devforge.conf` + `certs/`, hết `caddy_data`/`caddy_config` (Origin Cert không có state để giữ), và có healthcheck riêng trên `127.0.0.1:81` vì `up -d --wait` tính một container đang crash-loop là đang chạy |
| ✅ | `deploy/up.sh` | Viết lại: `pull` thay `build`, `up -d --wait --wait-timeout 120` thay hàm `healthy()` tự viết, rollback đổi sang `IMAGE_TAG=$(cat .image-tag)` và **không chạy lại migrate** (lỗi 13.0#4). Đòi `IMAGE_TAG`. Pull 4 lab image rồi `docker tag` về `devforge/<tên>:latest` cho khớp bảng `lab_images`. Lần deploy đầu chưa có `.image-tag` → nói thẳng là không có gì để lùi thay vì lùi bừa |
| ✅ | `deploy/bootstrap.sh` | Docker: **kiểm rồi thoát**, không cài — hai bản cài trên một máy là chỗ `docker ps` và compose bất đồng ý ai giữ container. Bỏ clone fe; ghi chú tường lửa hPanel; swap 2 GB; lời nhắn cuối nói cert Origin thay vì ACME |
| ✅ | `.env.prod.example` | `IMAGE_REPO` có; `PUBLIC_URL`/`FRONTEND_URL`/`CORS_ORIGINS`/`GOOGLE_REDIRECT_URL` đã khớp bảng §8; `ACME_EMAIL` đã bỏ; `MAX_CONTAINERS=12`; `LAB_SESSION_TTL=20m` |
| ✅ | `.github/workflows/ci.yml` | Job `images` (master: build → quét → push `:sha-<short>`), job `promote` (tag: `imagetools create` sang `:v1.2.0`, đòi tag nằm trên master), job `deploy` (chờ `devforge-web:<tag>` rồi ssh, truyền `IMAGE_TAG`). Chỉ checkout clone be — repo fe không còn trên box |
| ✅ | `.github/workflows/ci.yml` | `trivy image` chạy trên image `--load` **trước** bước push, cộng 4 lab image; `trivy fs` và `trivy edge image` trong job `secrets`. Cờ: `--severity HIGH,CRITICAL --ignore-unfixed --exit-code 1`. ⚠️ Bước này **chưa từng chạy** cho tới 2026-09-18 vì tag action sai — xem §13.6 |
| ✅ | `.github/workflows/ci.yml` | `go vet` → `golangci-lint run`; `go test` → `-coverprofile` + sàn **16.5%** (số của cây lúc thêm bước này là 16.8%). Bánh cóc: nâng khi coverage lên, không hạ để build xanh |
| ✅ | `.trivyignore` | Rỗng có chủ ý, chỉ còn quy tắc: mỗi dòng bỏ qua **phải có `exp:<ngày>`** kèm lý do. Không hạn = tắt scanner cho CVE đó vĩnh viễn |
| ✅ | `.golangci.yml` | v2, `default: standard` + `bodyclose`, `rowserrcheck`, `sqlclosecheck`, `errorlint`. Dùng preset loại trừ có sẵn thay vì danh sách tự chế. Cây đang **0 issue** — 5 lỗi thật đã vá: field `total` chết ở `streak.go`, `%v` → `%w` ở `simgen.go`, `client.IsErrNotFound` đã deprecated → `cerrdefs.IsNotFound`, thứ tự trả về của helper test |
| ◐ | `deploy/DEPLOY.md` | Đã viết lại theo thứ tự của §13.4, tách nhỏ hơn, cộng mục 502-sau-deploy. **Chưa ai chạy nó trên máy thật** — những chỗ chỉ biết khi đứng trên máy có dấu ⚠️ *kiểm trên máy*, sửa ngay trong lần dựng đầu |
| ✅ | `Makefile` | `release` (tag + push cả hai repo, chặn tag trùng và cây bẩn), `cover`, và `check-edge` — xem dòng dưới |
| ✅ | `scripts/edge-routes.check.sh` | **Mới.** Dựng `devforge.conf` thật trước hai upstream giả tên `api`/`web` rồi hỏi 14 đường dẫn qua https. `nginx -t` chỉ nói file cú pháp đúng; nó không nói `/uploads/*` có rơi vào SPA hay không, mà đó là lỗi trả **200 kèm ảnh vỡ** chứ không phải 404 để grep. Đã bắt một lỗi thật lúc viết: `{4}` trong regex ngày bị nginx đọc là mở block, phải quote |
| ✅ | Comment trong code | Đổi hết sang nginx: `cmd/server/router.go`, `internal/config/config.go`, `internal/labs/adapter/rest/preview.go`, `deploy/up.sh`, `ci.yml`, `.env.example`; `behindCaddy` → `behindEdge` trong `trustedproxies_test.go`. Chỉ là chữ, `go build ./...` và test xanh |

### 13.3 Repo `devforge-fe` — file phải sửa

> **Bảng này đã đổi hẳn nội dung.** Bản trước là danh sách việc chuyển sang
> Cloudflare Pages (Pages Function, `_redirects`, `_headers`, xoá `Dockerfile`).
> §9.1.1 đảo quyết định đó, nên toàn bộ những dòng ấy **không còn là việc** — giữ
> lại đây một câu để người đọc sau không đi tìm chúng.

| | File | Việc |
| --- | --- | --- |
| ✅ | `Dockerfile` | `ENV VITE_API_URL=""` thay cho `ARG VITE_API_URL` — không còn build-arg nào để quên truyền, và image hết dính môi trường. Vá luôn lớp lỗi 13.0#1 tận gốc thay vì vá triệu chứng |
| ✅ | `src/api/labs.ts` | `??` → `\|\|` cho `VITE_API_URL`: `new URL(path, "")` ném `TypeError: Invalid base URL`, nên base rỗng phải rơi về `location.origin` |
| ✅ | `src/components/LabTerminal.tsx` | Reconnect có backoff khi socket đứt bất thường (mã 1006). Không có nó thì **mỗi lần deploy đá cả lớp ra khỏi terminal** |
| ✅ | `src/lib/wsRetry.ts` + `.check.ts` | Quy tắc "đóng sạch = phiên kết thúc, đóng bất thường = thử lại" tách ra khỏi component và có assert riêng. Đăng ký trong `npm run check` |
| ✅ | `.github/workflows/ci.yml` | Thêm `trivy fs`, job `image` (master: build → `trivy image` → push `devforge-web:sha-<short>`), job `promote` (tag: `imagetools create` sang `:v1.2.0`). Vẫn **không có** job deploy |
| ✅ | `.env` local | `VITE_API_URL=http://localhost:8888` khớp `PORT=8888` trong `devforge-be/.env`. Hai file `.env.example` vẫn ghi 8080 và cũng khớp nhau — không đụng, đổi một bên là làm hỏng máy của người đang dùng bên kia |

### 13.4 Thứ tự

**Đã xong, không phụ thuộc máy:**

```
✅ reconnect terminal (fe)             ← thứ đáng làm nhất, và không nằm trong pipeline
✅ pipeline artifact: build một lần trên master → GHCR :sha-<short>
✅ trivy image (api + 4 lab) trước bước push, trivy fs cho dependency
✅ golangci-lint thay go vet + sàn coverage
✅ promote theo tag + cổng xuyên repo qua registry + make release
✅ Caddy → nginx: devforge.conf + certs/, compose đổi service, deploy/caddy/ xoá
✅ bootstrap.sh cho template Hostinger, .env.prod.example khớp bảng §8
✅ make check-edge — 14 route của biên, chạy được trên máy dev
✅ deploy/DEPLOY.md viết lại  ← còn phải sửa lại lúc đứng trên máy thật
```

**Còn lại, theo thứ tự bắt buộc. Từ đây mọi bước đều cần máy:**

```
0. Đặt 6 package GHCR sang Public        ← làm được NGAY, không cần máy
1. Mua máy + DNS + Origin Cert           (13.1, phần hạ tầng)
2. bootstrap.sh trên máy mới + .env + cert
3. make release v=v0.1.0, rồi IMAGE_TAG=v0.1.0 up.sh chạy tay trên máy
   ← phải là tag PHIÊN BẢN, không phải sha: §8 đầu mục nói vì sao
   ← job deploy tự bỏ qua vì chưa có SSH_HOST, nên master vẫn xanh
4. R2 + chạy backup tay + DIỄN TẬP RESTORE   ← đừng để sau
5. Bật CD = thêm 3 secret SSH, rồi make release v=v0.1.1, xem nó chạy hết đường
6. Diễn tập rollback bằng tay: IMAGE_TAG=<tag cũ> docker compose up -d --wait
7. Bật Authenticated Origin Pulls  ← sau khi site đã chạy, thứ tự ở DEPLOY.md §1
8. VPS thứ hai cho staging       ← đã chốt mua, §8 "Staging"
   bootstrap.sh + .env theo khối STAGING ở cuối .env.prod.example
   DNS staging.<domain> + Google redirect URI thứ hai
   3 secret STAGING_SSH_* → job deploy-staging tự bật (deploy `:master`)
9. SonarQube Cloud: tạo tài khoản, bind 2 repo, dán SONAR_TOKEN
   ← job `sonar` đã có sẵn và đang tự bỏ qua; token là thứ bật nó lên
   ← đối chiếu sonar.projectKey/sonar.organization với UI ở lần chạy đầu
```

Runbook từng bước: [deploy/DEPLOY.md](deploy/DEPLOY.md). Nó đánh số theo đúng
danh sách này, cộng một mục cho lỗi 502-sau-deploy.

Bước 4 và bước 6 là hai bước hay bị bỏ nhất và cũng là hai bước duy nhất chứng minh được lưới an toàn có thật.

⚠️ **Bước 3 có một cái bẫy của lần đầu.** `promote` đòi image `:sha-<short>` của commit được tag phải có sẵn trên GHCR, mà image chỉ được build khi push **master**. Nên tag đầu tiên phải cắt trên một commit đã đi qua master **sau khi** job `images` tồn tại. Tag một commit cũ hơn thì `promote` dừng và nói thẳng lý do.

### 13.5 Kiểm chứng sau khi lên

```bash
curl -sf https://<domain>/healthz              # tiến trình sống
curl -sf https://<domain>/readyz               # + nối được DB
curl -sI https://<domain>/ | grep -i cf-       # FE đi qua Cloudflare
```

Trên trình duyệt, những thứ chỉ hỏng ở prod nên phải nhìn tận mắt:

- **Đăng nhập rồi F5** — còn đăng nhập nghĩa là cookie same-site đúng.
- **Mở một lab, gõ vài phím** — không khựng nghĩa là `proxy_buffering off` đúng.
- **Xem một ảnh bìa** — hiện được nghĩa là biên có route riêng cho `/uploads/*`; nếu nó trả `index.html` kèm 200 thì ảnh vỡ chứ không 404.
- **Mở một lab rồi `docker restart` container api** — terminal phải tự nối lại chứ không chết hẳn. Đây là thứ duy nhất kiểm được reconnect, và nó là thứ mỗi lần deploy sẽ làm.
- **Dán một link `/r/<id>` vào Zalo/Slack** — ra thẻ xem trước nghĩa là khối `@crawler` ở biên chạy, và nó đứng đúng trước route bắt-tất-cả.
- **Xem `audit_logs.ip` của lần đăng nhập vừa rồi** — ra IP thật của bạn, không phải IP Cloudflare, nghĩa là §9.5 đã đúng cả hai tầng.

### 13.6 Nợ lại có chủ ý

| Việc | Vì sao chưa làm |
| --- | --- |
| Gom log / metric / alert (P3.5) | Chưa chặn việc lên prod. Làm ngay sau khi có người dùng thật |
| ~~Uptime monitor ngoài~~ | Đã chuyển lên 13.1 — quá rẻ để xếp vào nợ |
| ~~Sentry~~ | Đã chuyển lên 13.1, cùng lý do |
| ~~trivy quét image~~ | **Đã làm — và lần chạy thật đầu tiên là 2026-09-18.** Từ lúc thêm cho tới hôm đó nó chưa từng thực thi: `aquasecurity/trivy-action@0.28.0` không phải tag có thật (upstream có `v` ở đầu), nên job chết ở "Set up job" trên mọi nhánh kể cả master. Lần chạy thật đầu tiên ra 3 CVE Go (2 CRITICAL ở `pgx`), 1 HIGH npm, 17 HIGH trong binary Caddy của image `devforge-web`, và 39 HIGH trong `nginx:1.27-alpine` mà biên đang ghim. Bài học không phải về trivy: **một bước CI chưa từng thấy đỏ cũng chưa từng thấy xanh** |
| ~~SonarCloud~~ | **Hết là nợ.** Job `sonar` đã có ở cả hai repo, gate chặn bằng `sonar.qualitygate.wait=true`, và nó tự bỏ qua tới khi có `SONAR_TOKEN` — §13.4 bước 9. Tự dựng SonarQube thì **không bao giờ** trên box này, §9.4 |
| Reconnect cho WebSocket chat | `LabTerminal` đã có; `src/api/chat.ts` dùng chung `terminalURL` nhưng chưa dùng chung phần thử lại. Ít đau hơn: mất một socket chat không giết một phiên lab |
| ~~Staging BE~~ | **Hết là nợ.** Đã chốt mua VPS thứ hai và job `deploy-staging` đã có; còn lại là mua máy — §13.4 bước 7 |
| Bỏ Redis (dồn session vào Postgres) | Đang chạy, 0 cấu hình. Lãi một container, không đáng ưu tiên |
