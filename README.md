# DevForge

Nền tảng học DevOps qua lab thực hành. Mỗi bài lab cấp cho học viên một **container Linux thật**, truy cập qua terminal trong trình duyệt, giới hạn 60 phút.

---

## 0. Tóm tắt

```
Vite/React/TS/Tailwind ─HTTP+WS→ Go/Gin/GORM ─→ Postgres
                                      │
                             socket-proxy → Docker → lab container (hardened, TTL 60')

Caddy(TLS) │ GHCR │ GitHub Actions │ Terraform+Ansible │ Prometheus/Grafana/Loki │ SOPS
```

Ba khối chức năng:

1. **Học viên** — landing, danh sách khoá học, chi tiết khoá học (4 tab), lab + terminal thật, chấm điểm, lịch sử, bảng xếp hạng, chat chung
2. **Admin** — CRUD khoá học/lab/task, chạy thử `check_script`, ban user, kill session đang chạy, audit log
3. **Hạ tầng** — 2 môi trường local/production, CI/CD, IaC, observability, backup + diễn tập restore

Chưa chốt (chỉ cần trước P9, không chặn P0–P8): nhà cung cấp VPS, cấu hình máy, tên miền, ngân sách/tháng.

---

## 1. Stack

### Frontend

| Thành phần   | Chọn                               |
| ------------ | ---------------------------------- |
| Build        | Vite + React 19 + TypeScript       |
| Router       | React Router v7                    |
| Server state | TanStack Query                     |
| Client state | React Context                      |
| UI           | TailwindCSS + shadcn/ui            |
| Terminal     | xterm.js + `@xterm/addon-fit`      |
| Form         | react-hook-form + zod              |
| Markdown     | react-markdown + shiki             |
| Ngày giờ     | `Intl.RelativeTimeFormat` (native) |

Không dùng: Next.js, Redux, Zustand, axios, dayjs, i18n, Storybook.

### Backend

| Thành phần | Chọn                                              |
| ---------- | ------------------------------------------------- |
| Ngôn ngữ   | Go 1.25                                           |
| Router     | Gin                                               |
| ORM        | GORM — **chỉ để query, không dùng `AutoMigrate`** |
| Driver     | pgx v5                                            |
| Migration  | golang-migrate (file SQL)                         |
| WebSocket  | coder/websocket                                   |
| Docker     | docker/docker/client                              |
| Auth       | golang-jwt/jwt v5 + golang.org/x/oauth2 + bcrypt  |
| 2FA        | pquerna/otp (TOTP cho admin)                      |
| Log        | log/slog (JSON)                                   |
| Validate   | go-playground/validator                           |
| Test       | stdlib testing + testcontainers-go                |

Redis giữ **duy nhất** refresh session (xem [Phiên đăng nhập](#phiên-đăng-nhập)) — mất volume Redis = mọi người phải đăng nhập lại, không mất dữ liệu gì khác.

Không dùng: gRPC, message queue, DI framework.

### Dữ liệu

PostgreSQL 16. Backup `pg_dump` cron → nén → S3/R2. Diễn tập restore hàng tháng.

### Sandbox lab

| Lớp            | Chọn                                                                                                                   |
| -------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Runtime        | Docker Engine API qua `tecnativa/docker-socket-proxy`                                                                  |
| Image lab      | Alpine / Debian slim, user `student` non-root                                                                          |
| Giới hạn       | `--memory=512m --cpus=0.5 --pids-limit=256 --cap-drop=ALL --security-opt=no-new-privileges --read-only` + tmpfs `/tmp` |
| Mạng           | `--network=none` mặc định                                                                                              |
| Lab dạy Docker | `sysbox-runc`                                                                                                          |
| Nâng cấp sau   | gVisor (`--runtime=runsc`)                                                                                             |

### Hạ tầng

| Hạng mục      | Chọn                                               |
| ------------- | -------------------------------------------------- |
| Local         | Docker Compose + `air` (hot reload Go)             |
| Prod          | 1 VPS + Docker Compose (P11 tách 2 VPS)            |
| Reverse proxy | Caddy                                              |
| Registry      | GHCR                                               |
| CI/CD         | GitHub Actions                                     |
| Provision     | Terraform                                          |
| Config server | Ansible                                            |
| Secrets       | SOPS + age (git) / GitHub Secrets (CI)             |
| Metrics       | Prometheus + Grafana                               |
| Log           | Loki + Promtail                                    |
| Alert         | Alertmanager → Telegram                            |
| Traffic web   | Umami (self-host)                                  |
| Scan          | trivy, gitleaks                                    |
| Lint          | golangci-lint, gofumpt, eslint, prettier, lefthook |

Không dùng: Jaeger/tracing, ELK, Vault, Consul, service mesh.

---

## 2. Kiến trúc

```
Browser (Vite SPA)
   │  REST  /api/*             → auth, courses, labs, history, leaderboard, admin
   │  WS    /ws/terminal/:sid  → PTY container
   │  WS    /ws/chat           → phòng chat chung
   ▼
Caddy (TLS) ──> Go API (Gin) ──── Postgres
                    │
                    └── docker-socket-proxy ──> Docker Engine
                                                  └── devforge-lab-<sid>
                                                      (ephemeral, TTL 60')
```

### Vòng đời lab session

1. `POST /api/labs/:slug/start` → tạo container từ image của lab
2. Giới hạn: `--memory=512m --cpus=0.5 --pids-limit=256 --cap-drop=ALL --security-opt=no-new-privileges --read-only` + tmpfs `/tmp`, user `student` non-root
3. Trả `sessionID` + `expires_at` → FE mở `WS /ws/terminal/:sessionID`
4. Backend `ContainerExecAttach` với TTY → pipe stdin/stdout qua WS
5. **Reaper goroutine** quét mỗi 30s theo `expires_at` **trong DB** → `ContainerRemove(force)`. Server restart vẫn dọn được, user đóng tab vẫn bị dọn.
6. Nộp bài → chạy check script trong container → chấm → lưu `submissions` → xoá container

### Chấm điểm

Mỗi lab có N task. Mỗi task = 1 shell script chạy bằng `docker exec` **trong chính container của học viên**, exit code 0 = đạt.

```
task: "Tạo thư mục /home/student/devforge"
check: test -d /home/student/devforge
```

Học viên bấm _Kiểm tra_ nhiều lần được. _Nộp bài_ chạy lần cuối rồi đóng session.

---

## 3. Nguồn sự thật của nội dung

| Loại                                                                | Sửa ở đâu               |
| ------------------------------------------------------------------- | ----------------------- |
| Nội dung khoá học/lab (tiêu đề, markdown, thứ tự, điểm, thời lượng) | Admin UI → DB           |
| Image lab (Dockerfile, gói cài sẵn)                                 | Git + CI build          |
| `check_script`                                                      | Admin UI + nút Chạy thử |

**DB là nguồn sự thật duy nhất.** YAML chỉ dùng `make seed` cho môi trường trống lúc dựng local — không seed đè lúc app khởi động.

Hệ quả: tạo khoá học mới không cần deploy. Tạo môi trường lab kiểu mới (lab cần `kubectl` cài sẵn) thì cần commit Dockerfile + đợi CI build image.

---

## 4. Data model

```sql
users        (id, username, email, password_hash NULL, google_id NULL, avatar_url,
              status DEFAULT 'active', banned_reason, banned_at, banned_by, created_at)
roles        (id, name)                              -- 'student' | 'admin'
user_roles   (user_id, role_id)                      -- PK kép
totp_secrets (user_id, secret_enc, confirmed_at)     -- 2FA admin

courses      (id, slug, title, description, image_url, level,
              status DEFAULT 'draft', published_at, updated_at)
labs         (id, course_id, slug, title, description_md, duration_minutes,
              lab_image_id, order_idx)
lab_tasks    (id, lab_id, title, points, check_script, order_idx)
lab_images   (id, name, tag, description, active)    -- image đã build sẵn, admin chọn từ đây
reviews      (id, course_id, title, content_md, order_idx)   -- tab Ôn tập

enrollments  (user_id, course_id, created_at)        -- PK kép
lab_sessions (id, user_id, lab_id, container_id, status, started_at, expires_at, ended_at)
submissions  (id, session_id, user_id, lab_id, score, passed_tasks, total_tasks, submitted_at)

chat_messages (id, user_id, content, created_at)
audit_logs    (id, actor_id, action, target_type, target_id, diff JSONB, ip, created_at)
```

Bảng xếp hạng = query `SUM(điểm cao nhất mỗi lab) GROUP BY user`. Không cần bảng riêng.

---

## 5. API

### Public / student

```
POST   /api/auth/register              {username, email, password}
POST   /api/auth/login
GET    /api/auth/google                → redirect OAuth
GET    /api/auth/google/callback
POST   /api/auth/refresh               → xoay session, không body, không trả token
POST   /api/auth/logout                → thoát máy này
POST   /api/auth/logout-all            → thoát mọi thiết bị (cần access token)
GET    /api/auth/sessions              → thiết bị đang đăng nhập, mới nhất trước
DELETE /api/auth/sessions/:id          → thoát 1 thiết bị (chỉ phiên của chính mình)
GET    /api/me

GET    /api/courses                    → chỉ status=published
GET    /api/courses/:slug              → detail + labs + tiến độ user
POST   /api/courses/:slug/enroll
GET    /api/courses/:slug/reviews      → tab Ôn tập
GET    /api/courses/:slug/leaderboard  → tab Bảng xếp hạng
GET    /api/courses/:slug/status       → tab Trạng thái

GET    /api/labs/:slug
POST   /api/labs/:slug/start           → tạo container, trả sessionID + expires_at
                                         ?incident=<id> xin đúng 1 kịch bản thay vì bốc ngẫu nhiên
POST   /api/sessions/:id/check         → chấm tạm, chạy nhiều lần
POST   /api/sessions/:id/submit        → chấm cuối, đóng session, xoá container
GET    /api/history                    → lịch sử làm bài user hiện tại

POST   /api/lab-sessions/:id/share     → đăng báo cáo ca trực công khai, trả token
DELETE /api/lab-sessions/:id/share     → gỡ xuống, link cũ chết hẳn

# Hai route KHÔNG cần đăng nhập — link chia sẻ phải mở được cho người lạ
GET    /api/shared-drills/:token       → số liệu + tên sự cố. Không timeline, không lời giải
GET    /api/daily-drill                → ca trực hôm nay (chọn từ ngày, UTC) + bảng xếp hạng ngày

WS     /ws/terminal/:sessionID
WS     /ws/chat
```

### Admin — middleware `RequireRole("admin")`, ghi audit tự động

```
GET    /api/admin/stats
GET    /api/admin/courses              ?status=
POST   /api/admin/courses
PATCH  /api/admin/courses/:id
POST   /api/admin/courses/:id/publish
POST   /api/admin/labs
PATCH  /api/admin/labs/:id
POST   /api/admin/labs/:id/tasks
POST   /api/admin/labs/:id/dry-run     → chạy thử check_script trong container thật
GET    /api/admin/lab-images
GET    /api/admin/users                ?q=&status=
POST   /api/admin/users/:id/ban        {reason}
POST   /api/admin/users/:id/unban
GET    /api/admin/sessions/active
DELETE /api/admin/sessions/:id         → kill container thủ công
GET    /api/admin/audit                ?actor=&action=
```

---

## 6. Màn hình

### Học viên

| Route                 | Nội dung                                                                                                                                                                                                              |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `/login`, `/register` | form + nút "Đăng nhập với Google"                                                                                                                                                                                     |
| `/`                   | Landing 2 cột. Trái: "Làm Chủ Quy Trình DevOps & Cloud-Native" + mô tả + 4 chip (Thực hành trực tiếp / Môi trường Linux / Quản lý Git / Container hóa) + 2 nút _Khám phá khóa học_, _Tìm hiểu thêm_. Phải: ảnh DevOps |
| `/courses`            | Lưới card khoá học                                                                                                                                                                                                    |
| `/courses/:slug`      | Phải: ảnh, cấp độ, số lab, học viên, cập nhật, nút Đăng ký. Trái: 4 tab — Nội dung khoá học \| Ôn tập \| Bảng xếp hạng \| Trạng thái                                                                                  |
| `/labs/:slug`         | Trái: đề bài + checklist task + đồng hồ đếm ngược. Phải: terminal xterm.js. Nút _Kiểm tra_ / _Nộp bài_                                                                                                                |
| `/history`            | Lịch sử làm bài user hiện tại                                                                                                                                                                                         |
| `/chat`               | Phòng chat chung                                                                                                                                                                                                      |

_Bắt đầu làm bài thực hành_ → modal xác nhận ("Bạn có 60 phút, container sẽ bị xoá khi hết giờ") → `POST /start`.

### Admin

| Route                  | Nội dung                                                                               |
| ---------------------- | -------------------------------------------------------------------------------------- |
| `/admin`               | Dashboard: user mới 7 ngày, session đang chạy, lab hoàn thành, tỉ lệ pass theo lab     |
| `/admin/courses`       | CRUD khoá học / lab / task, kéo thả sắp xếp, draft → publish                           |
| `/admin/labs/:id/edit` | Soạn đề bài markdown, thêm task, chọn image từ dropdown, **nút Chạy thử** check_script |
| `/admin/users`         | Danh sách, tìm kiếm, tiến độ, ban/unban                                                |
| `/admin/sessions`      | Session đang chạy: ai, lab gì, còn bao lâu, nút Kill                                   |
| `/admin/audit`         | Audit log, append-only                                                                 |

**Nút Chạy thử**: tạo 1 container lab thật, chạy `check_script`, trả exit code + stdout. Dùng lại đúng luồng `POST /api/labs/:id/start`.

Traffic web xem ở Umami, metrics hệ thống ở Grafana. Dashboard admin chỉ hiển thị số liệu nghiệp vụ: đăng ký khoá X, lab nào tỉ lệ rớt cao, thời gian trung bình hoàn thành.

---

## 7. Bảo mật

### Phiên đăng nhập

Không token nào chạm tới JavaScript. Đăng nhập trả về **user**, phần chứng thực đi bằng 2 cookie `HttpOnly`:

| Cookie       | Là gì                        | Path        | Sống  |
| ------------ | ---------------------------- | ----------- | ----- |
| `df_access`  | JWT ngắn hạn, mang `sid`     | `/`         | 15m   |
| `df_session` | id session ngẫu nhiên 256bit | `/api/auth` | 168h  |

- **Refresh là tra Redis, không phải kiểm chữ ký** — nên thu hồi được. Đó là toàn bộ lý do refresh không còn là JWT.
- **Xoay vòng mỗi lần refresh**: id cũ chết ngay. Cookie bị trộm chỉ dùng được tới lần refresh kế tiếp của máy thật.
- **Khoá `usess:<uid>`** là set id session của một user → `logout-all` là một lần đọc set, không quét keyspace.
- **Đổi mật khẩu** đá mọi thiết bị *khác*, giữ lại thiết bị vừa thao tác (`sid` nằm trong access token nên biết được đâu là phiên hiện tại).
- **Chống CSRF 2 lớp**: `SameSite=Lax` trên cookie, cộng với chặn ở tầng HTTP — request đổi trạng thái mà `Origin` không thuộc `CORS_ORIGINS` và không same-origin → 403.
- **Màn hình thiết bị** (`GET /api/auth/sessions`): `sess:<id>` là hash chứa `uid`, `ua`, `ip`, `created`, `seen`. Refresh xoay id nhưng **khiêng `created` sang phiên mới** — nếu không, mọi thiết bị sẽ luôn báo "vừa đăng nhập". `DELETE /api/auth/sessions/:id` kiểm chủ sở hữu và trả **404** khi id thuộc người khác — báo 403 tức là xác nhận id đó có thật.
- Thu hồi phiên chỉ chặn được **refresh**. Access token đã ký vẫn sống tới hết 15 phút — cái giá của việc không tra DB mỗi request.
- `Secure` bật theo `APP_ENV=production`; local không có TLS nên cookie `Secure` sẽ không được lưu.

### Sandbox lab

Container do học viên gõ lệnh = code lạ chạy trên máy chủ. Bắt buộc:

- **Không mount `/var/run/docker.sock` vào container lab.** API truy cập Docker qua `tecnativa/docker-socket-proxy`, chỉ mở `containers/create,start,exec,remove`.
- `--cap-drop=ALL`, `--security-opt=no-new-privileges`, user non-root trong container
- `--memory`, `--cpus`, `--pids-limit`, `--read-only` + tmpfs
- `--network=none` mặc định; chỉ mở mạng cho lab cần
- Tối đa 1 session đồng thời mỗi user
- Reaper luôn chạy, dựa trên `expires_at` trong DB (không dựa vào trạng thái trong RAM)
- **Không dùng `--privileged`** trong bất kỳ trường hợp nào, kể cả tạm để test. Lab dạy Docker dùng `sysbox-runc` hoặc rootless dind.
- Nâng cấp trước khi mở công khai: gVisor (`--runtime=runsc`)

### Admin

1. Admin đầu tiên tạo bằng CLI trên server: `./devforge admin create --email=...`. **Không có route đăng ký admin trong API.**
2. **TOTP 2FA bắt buộc** cho role admin.
3. Role lưu ở bảng `user_roles` riêng, không phải cột `is_admin` trên `users`.
4. `check_script` **chỉ chạy trong container lab đã hardened**, không bao giờ trên host.
5. Image lab chọn từ bảng `lab_images` (dropdown), admin **không gõ tên image tự do**.
6. Audit log append-only — không có `DELETE /api/admin/audit`.
7. Rate limit route admin, log mọi lần đăng nhập thất bại.

---

## 8. Môi trường

**Build 1 lần, chạy mọi nơi — áp dụng cho API (Go).** Cùng image `ghcr.io/<user>/devforge-api:<git-sha>` chạy ở local lẫn prod. Khác nhau chỉ ở env var inject lúc chạy. Không có `Dockerfile.prod` riêng.

> **FE khác API:** Vite **bake `VITE_API_URL` lúc build**, không phải runtime. Image FE prod ≠ image FE local → phải build riêng mỗi môi trường (CI truyền `VITE_API_URL` qua `--build-arg`, tag theo git-sha). Không dùng chung 1 image FE cho cả 2 env. Câu "build 1 lần chạy mọi nơi" chỉ đúng cho API.

|           | local                          | production                                                              |
| --------- | ------------------------------ | ----------------------------------------------------------------------- |
| Chạy bằng | `docker compose up`            | `docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d` |
| Config    | `.env` (từ `.env.example`)     | SOPS-encrypted / env trên server                                        |
| DB        | postgres container, seed giả   | postgres volume + pg_dump cron                                          |
| TLS       | không                          | Caddy + Let's Encrypt                                                   |
| Log       | stdout                         | slog JSON → Promtail → Loki                                             |
| Deploy    | hot reload (`air`, `vite dev`) | GitHub Actions → SSH → `compose pull && up -d`                          |

Rollback = trỏ về image tag SHA cũ, không revert code rồi build lại.

### Biến môi trường

| Biến                                          | local                   | production             | Nguồn      |
| --------------------------------------------- | ----------------------- | ---------------------- | ---------- |
| `APP_ENV`                                     | development             | production             | compose    |
| `PORT`                                        | 8080                    | 8080                   | compose    |
| `LOG_LEVEL`                                   | debug                   | info                   | compose    |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_NAME` | (compose)               | (compose)              | compose    |
| `DB_PASSWORD`                                 | `.env` giả              | SOPS / GitHub Secrets  | **secret** |
| `REDIS_ADDR` / `REDIS_PASSWORD` / `REDIS_DB`  | localhost:6379          | (compose)              | compose    |
| `COOKIE_DOMAIN`                               | (rỗng)                  | (rỗng)                 | compose    |
| `JWT_SECRET`                                  | `.env` giả              | SOPS / GitHub Secrets  | **secret** |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`   | `.env`                  | SOPS / GitHub Secrets  | **secret** |
| `CORS_ORIGINS`                                | `http://localhost:5173` | `https://<domain>`     | compose    |
| `DATABASE_URL` (migrate)                      | `sslmode=disable`       | `sslmode=require`      | **secret** |
| `VITE_API_URL` (FE, **build-time**)           | `http://localhost:8080` | `https://<domain>/api` | build-arg  |

`.env.example` phải liệt kê đủ biến trên (giá trị giả). Secret prod **không bao giờ vào git thô** — chỉ SOPS-encrypted (age) hoặc GitHub Secrets.

### Healthcheck (điều kiện rollback)

Rollback tự động ở CI chỉ chạy được nếu `api` khai báo healthcheck trong compose:

```yaml
api:
  healthcheck:
    test: ["CMD", "/server", "healthcheck"] # binary tự gọi /healthz nội bộ
    interval: 10s
    timeout: 3s
    retries: 5
```

Image prod là **distroless (không shell/wget)** → không dùng được `CMD-SHELL "wget ..."`. Phải thêm subcommand `/server healthcheck` vào binary để tự probe `/healthz`.

### Domain & routing (P9)

**1 domain, route bằng path** qua Caddy edge → không CORS ở prod, 1 cert TLS:

```
https://<domain>/         → FE static (Caddy serve)
https://<domain>/api/*    → Go API
https://<domain>/ws/*     → Go API (WebSocket → wss)
```

Hệ quả khi chốt domain:

- FE build với `VITE_API_URL=https://<domain>/api`, WS dùng `wss://<domain>/ws/...`
- `CORS_ORIGINS` prod = same-origin → gần như chỉ còn cần cho dev (`:5173`)
- OAuth: đăng ký redirect `https://<domain>/api/auth/google/callback` ở Google Console + set env
- Caddy auto Let's Encrypt: cần DNS A record trỏ VPS **trước khi** `up`

Không tách `api.` / `app.` subdomain → tránh CORS + cookie cross-site (`SameSite`) rắc rối. Muốn subdomain thì phải bật CORS credentials + set cookie domain — đắt hơn, không cần cho quy mô này.

### Migration tương thích ngược

Code mới phải chạy được với schema cũ. Đổi tên cột `user_name` → `username` làm 3 bước qua 3 lần deploy:

```
Deploy 1:  ADD COLUMN username; backfill; code đọc user_name, ghi CẢ HAI
Deploy 2:  code đọc username, ghi CẢ HAI
Deploy 3:  DROP COLUMN user_name; code chỉ dùng username
```

### CI/CD

```
push branch → lint + test + gitleaks
            → build image, trivy scan
            → push GHCR, tag = git SHA
merge main  → deploy prod qua SSH
            → health check → đỏ thì rollback tag cũ
```

---

## 9. Lộ trình

| Chặng    | Nội dung                                                                    |
| -------- | --------------------------------------------------------------------------- |
| **P0**   | Scaffold: vite app, Gin api, docker-compose, migration, lint, lefthook      |
| **P1**   | Auth: register/login JWT + Google OAuth + middleware + Context ở FE         |
| **P2**   | Landing + danh sách khoá học + chi tiết khoá học (4 tab)                    |
| **P3**   | **Lab runtime**: container lifecycle, socket-proxy, WS terminal, reaper 60' |
| **P3.5** | Observability: Prometheus + Grafana + Loki + alert                          |
| **P4**   | Chấm điểm: check script, submit, submissions, trang Lịch sử                 |
| **P4.5** | Roles, `RequireRole`, CLI tạo admin, TOTP 2FA, audit log                    |
| **P5**   | Admin: CRUD khoá học/lab/task, dry-run check_script, draft→publish          |
| **P6**   | Admin: users (ban/unban), sessions đang chạy (kill)                         |
| **P7**   | Leaderboard + tab Trạng thái + chat WS                                      |
| **P8**   | Nội dung: 3 khoá (Linux, Git, Docker) × 5 lab                               |
| **P9**   | Deploy prod: Terraform + Ansible + Caddy + GitHub Actions                   |
| **P10**  | Backup pg_dump + diễn tập restore, Umami, dashboard admin                   |
| **P11**  | _(tuỳ chọn)_ tách runner node riêng, gVisor, hoặc chuyển k3s                |

**P3 là phần rủi ro nhất — làm sớm**, trước khi đầu tư nhiều vào UI.

Metrics tối thiểu ở P3.5:

```
devforge_lab_sessions_active          gauge
devforge_lab_containers_running       gauge   # so với gauge trên → phát hiện rò rỉ
devforge_reaper_last_run_timestamp    gauge   # alert nếu > 5 phút
devforge_reaper_killed_total          counter
devforge_lab_start_duration_seconds   histogram
```

Alert quan trọng nhất: `lab_containers_running > lab_sessions_active` kéo dài → có container mồ côi.

---

## 10. Cấu trúc — polyrepo

Tách **2 repo độc lập** (kiểu outsource), gom trong 1 workspace để dễ quản lý:

```
devforge-workspace/
├── README.md                   # tài liệu tổng (file này)
│
├── devforge-be/                # REPO 1 — Go backend + orchestration
│   ├── cmd/server/main.go
│   ├── cmd/cli/main.go         # devforge admin create ...
│   ├── internal/{config,db,auth,courses,labs,sandbox,admin,chat,ws,metrics}
│   ├── migrations/             # SQL, golang-migrate
│   ├── labs/                   # Dockerfile image lab + seed YAML
│   ├── deploy/
│   │   ├── caddy/              # edge proxy (prod)
│   │   ├── terraform/          # VPS, DNS, firewall        (P9)
│   │   ├── ansible/            # harden, docker, user       (P9)
│   │   └── monitoring/         # prometheus, grafana, loki  (P3.5)
│   ├── .github/workflows/ci.yml   # go lint/test/build + gitleaks
│   ├── Dockerfile              # dev (air) / build / prod (distroless)
│   ├── docker-compose.yml      # postgres + api (dev)
│   ├── .air.toml · Makefile · lefthook.yml · .env.example
│
└── devforge-fe/                # REPO 2 — Vite + React (chạy standalone)
    ├── src/{pages,components,api,hooks,context,lib}
    │   └── pages/admin/
    ├── .github/workflows/ci.yml   # node lint/tsc/build + gitleaks
    ├── Dockerfile              # dev / build / prod (Caddy serve static)
    ├── Caddyfile.static · vite.config.ts · lefthook.yml · .env.example
```

**Vì sao polyrepo:** be/fe tách sạch theo ranh giới repo — CI riêng, deploy riêng, phân quyền riêng. `be` sở hữu orchestration (compose, DB, deploy) vì nó là hub triển khai cả stack. `fe` là app frontend thuần, dev chạy độc lập trỏ về API.

---

## 11. Chạy local

Hai repo độc lập, mở **2 terminal**:

```bash
# Terminal 1 — backend stack (postgres + api)
cd devforge-be
cp .env.example .env
make migrate                  # một lệnh: bật postgres, chạy migration, nạp nội dung, build 4 image lab
npx lefthook install          # git hook (1 lần)

# Terminal 2 — frontend (Vite dev, hot reload)
cd devforge-fe
cp .env.example .env          # VITE_API_URL=http://localhost:8080
npm ci
npm run dev                   # vite :5173
npx lefthook install          # git hook (1 lần)
```

| Cổng             | Dịch vụ                                   | Repo        |
| ---------------- | ----------------------------------------- | ----------- |
| `localhost:5173` | Vite dev server                           | devforge-fe |
| `localhost:8080` | Go API (FE trỏ trực tiếp, CORS cho :5173) | devforge-be |
| `localhost:5432` | Postgres                                  | devforge-be |

Kiểm tra API: `curl localhost:8080/healthz` · `curl localhost:8080/readyz` · `curl localhost:8080/api/ping`

`make help` (trong `devforge-be`) xem toàn bộ lệnh.

`make migrate` gộp cả bốn bước và chạy lại được bao nhiêu lần cũng được: migration
đã chạy thì bỏ qua, seed chỉ chèn vào chỗ trống, image có cache. Tài khoản đầu
tiên (`superadmin` / `lukas`) do migration `000020` tạo chứ không do seed, nên
chúng có mặt kể cả khi bỏ qua dữ liệu demo.

Ở môi trường **không phải máy local** dùng `make migrate-schema` — chỉ chạy
migration, không nạp nội dung demo và không build image.

Sửa nội dung seed thì chạy `make check-seed`: nó nạp mọi `check_script` vào đúng
image lab, chạy trên container mới (phải **trượt**) rồi chạy lại sau lời giải
trong `scripts/seed-solutions.tsv` (phải **đậu**). Một script chấm sai chỉ lộ ra
ở đây, không lộ ra khi đọc.

> Compose đặt tên `devforge-be`, tránh trùng volume/container project khác trên máy.
> Caddy edge (`deploy/caddy`) chỉ dùng ở **prod** (P9) — dev thì FE gọi thẳng API, không qua proxy.
