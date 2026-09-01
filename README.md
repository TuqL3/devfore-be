# DevForge

Nền tảng học DevOps qua lab thực hành. Mỗi bài lab cấp cho học viên một **container Linux thật**, truy cập qua terminal trong trình duyệt, giới hạn 60 phút.

Chạy production ở mức **0₫/tháng** — xem [§9 Hạ tầng production](#9-hạ-tầng-production--chạy-0).

---

## 0. Tóm tắt

```
Vite/React/TS/Tailwind ─HTTP+WS→ Go/Gin/GORM ─→ Postgres
                                      │
                             socket-proxy → Docker → lab container (hardened, TTL 60')

Caddy(TLS) │ GHCR │ GitHub Actions │ Ansible │ Grafana Cloud │ Oracle Always Free (ARM64)
```

Ba khối chức năng:

1. **Học viên** — landing, danh sách khoá học, chi tiết khoá học (4 tab), lab + terminal thật, chấm điểm, lịch sử, bảng xếp hạng, chat chung, bài mô phỏng, War Room
2. **Admin** — CRUD khoá học/lab/task, chạy thử `check_script`, ban user, kill session đang chạy, audit log, bảng sự kiện hệ thống
3. **Hạ tầng** — 2 môi trường local/production, CI/CD, cấu hình server, observability, backup + diễn tập restore

Đã chốt hạ tầng: **Oracle Cloud Always Free, 4 OCPU ARM Ampere A1 / 24 GB RAM, vùng Singapore.** Chi phí duy nhất là tên miền (~300k₫/năm). Xem §9.

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

PostgreSQL 16. Backup `pg_dump` cron → nén → Cloudflare R2 (free tier 10 GB, egress 0₫). Diễn tập restore hàng tháng.

### Sandbox lab

| Lớp          | Chọn                                                                                                                   |
| ------------ | ---------------------------------------------------------------------------------------------------------------------- |
| Runtime      | Docker Engine API qua `tecnativa/docker-socket-proxy`                                                                  |
| Image lab    | `alpine:3.21`, user `student` non-root (4 image: linux, git, docker, net)                                               |
| Giới hạn     | `--memory=512m --cpus=0.5 --pids-limit=256 --cap-drop=ALL --security-opt=no-new-privileges --read-only` + tmpfs `/tmp` |
| Mạng         | `--network=none` — **mọi lab, không ngoại lệ**                                                                         |
| Nâng cấp sau | gVisor (`--runtime=runsc`)                                                                                             |

**Lab dạy Docker không có daemon và sẽ không bao giờ có.** Sandbox chạy không mạng, không capability, rootfs read-only — đó là toàn bộ lý do đưa shell cho người lạ mà vẫn an toàn. Image `labs/docker` cài một shim `docker` báo lỗi rồi thoát 1; khoá học chấm trên Dockerfile/compose học viên **viết** và lệnh học viên **gõ**, không chấm trên daemon thật. Không dùng `sysbox-runc`, không dùng rootless dind.

### Hạ tầng

| Hạng mục      | Chọn                                               | Giá        |
| ------------- | -------------------------------------------------- | ---------- |
| Local         | Docker Compose + `air` (hot reload Go)             | —          |
| Prod          | 1 VPS Oracle Always Free ARM64 + Docker Compose    | **0₫**     |
| Reverse proxy | Caddy                                              | 0₫         |
| DNS           | Cloudflare (DNS-only, xem §9.5)                    | 0₫         |
| Registry      | GHCR                                               | 0₫         |
| CI/CD         | GitHub Actions                                     | 0₫         |
| Config server | Ansible                                            | 0₫         |
| Secrets       | GitHub Secrets (CI) / env file `chmod 600` (server)| 0₫         |
| Metrics + Log + Alert | **Grafana Cloud free tier** + Alloy agent  | 0₫         |
| Traffic web   | Umami Cloud free tier                              | 0₫         |
| Backup        | Cloudflare R2                                      | 0₫         |
| Sinh kịch bản sim | OpenRouter (tuỳ chọn, tắt được)                | xem §9.6   |
| Scan          | trivy, gitleaks                                    | 0₫         |
| Lint          | golangci-lint, gofumpt, oxlint, lefthook           | —          |

**Không tự dựng Prometheus/Grafana/Loki.** Free tier của Grafana Cloud (10k series, 50GB log, alert + contact point Telegram) phủ hết nhu cầu của một node, và tiết kiệm ~2 GB RAM trên máy — đúng phần RAM lẽ ra phải trả tiền để có.

**Không dùng Terraform.** Terraform để quản đúng một máy Always Free không bao giờ bị destroy là công cụ lớn hơn việc cần làm. Dựng máy bằng tay một lần, `deploy/ansible/` lo phần cấu hình lặp lại được.

Không dùng: Jaeger/tracing, ELK, Vault, Consul, service mesh, SOPS, Kubernetes.

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

**Bài mô phỏng không tạo container nào.** Nhánh sim thoát sớm ngay sau khi ghi row session (`internal/labs/usecase/labs.go`), *trước* khi kiểm tra sức chứa — có chủ đích: lab sim không được bị từ chối vì lab container đã đầy. Xem §9.4, đây là đòn bẩy sức chứa lớn nhất.

### Chấm điểm

Mỗi lab có N task. Mỗi task = 1 shell script chạy bằng `docker exec` **trong chính container của học viên**, exit code 0 = đạt.

```
task: "Tạo thư mục /home/student/devforge"
check: test -d /home/student/devforge
```

Học viên bấm _Kiểm tra_ nhiều lần được. _Nộp bài_ chạy lần cuối rồi đóng session.

Kiểu bài thứ tư (`lab_tasks.kind = 'sim'`) chấm theo kết quả engine mô phỏng chứ không theo exit code — chi tiết ở `SIM-CICD.md`.

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
lab_tasks    (id, lab_id, title, points, check_script, kind, order_idx)
lab_images   (id, name, tag, description, active)    -- image đã build sẵn, admin chọn từ đây
reviews      (id, course_id, title, content_md, order_idx)   -- tab Ôn tập

enrollments  (user_id, course_id, created_at)        -- PK kép
lab_sessions (id, user_id, lab_id, container_id, status, started_at, expires_at, ended_at)
submissions  (id, session_id, user_id, lab_id, score, passed_tasks, total_tasks, submitted_at)

chat_messages (id, user_id, content, created_at)
audit_logs    (id, actor_id, action, target_type, target_id, diff JSONB, ip, created_at)
system_events (id, kind, severity, subject, detail, created_at)
```

`container_id` rỗng = session mô phỏng. Mọi nhánh sau đó (reaper, Stop, Submit, Live) đọc đúng cột này để phân biệt hai loại.

`audit_logs` trả lời *ai làm gì với ai*, chỉ ghi bởi hành động cố ý của admin. `system_events` trả lời *cái gì hỏng*, ghi bởi chính đường code thất bại. Hai câu hỏi khác nhau nên hai bảng khác nhau — gộp lại thì `actor_id` phải nullable và nửa số hàng không có người làm.

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

GET    /api/my-drill-streak            → chuỗi ngày liên tiếp của chính mình

# Bốn route KHÔNG cần đăng nhập — link chia sẻ phải mở được cho người lạ.
# Tất cả đều qua rate limit theo IP (PUBLIC_RATE_LIMIT, mặc định 60/phút).
# ⚠️ Rate limit này HỎNG khi chạy sau Caddy — xem §9.5 trước khi deploy.
GET    /api/shared-drills/:token       → số liệu + tên sự cố. Không timeline, không lời giải
GET    /api/shared-drills/:token/preview → HTML có thẻ og:* cho trình thu thập
GET    /api/shared-drills/:token/og.png  → ảnh 1200x630 vẽ từ số liệu
GET    /api/daily-drill[?day=]         → ca trực hôm nay + bảng ngày; `day` đọc ca đã qua (≤90 ngày, không nhận ngày mai)
GET    /api/daily-drill/:day/preview   → HTML có thẻ og:* cho một ngày đã đóng
GET    /api/daily-drill/:day/og.png    → ảnh 1200x630 của ngày đó
GET    /api/weekly-board               → bảng 7 ngày, xếp theo SỐ NGÀY giải được

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
GET    /api/admin/events               → system_events
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
| `/sim`                | Sân chơi mô phỏng — cùng engine với lab sim, không nhiệm vụ, không điểm, không lưu                                                                                                                                    |
| `/war-room`           | Thử thách sự cố có hạn giờ, ca trực hôm nay, bảng 7 ngày, chuỗi ngày                                                                                                                                                  |
| `/history`            | Lịch sử làm bài user hiện tại                                                                                                                                                                                         |
| `/chat`               | Phòng chat chung + tin nhắn riêng                                                                                                                                                                                     |

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
| `/admin/events`        | `system_events` — cái gì hỏng, tự động ghi                                             |

**Nút Chạy thử**: tạo 1 container lab thật, chạy `check_script`, trả exit code + stdout. Dùng lại đúng luồng `POST /api/labs/:id/start`.

Traffic web xem ở Umami, metrics hệ thống ở Grafana Cloud. Dashboard admin chỉ hiển thị số liệu nghiệp vụ: đăng ký khoá X, lab nào tỉ lệ rớt cao, thời gian trung bình hoàn thành.

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

> ⚠️ Cột `ip` trên `audit_logs` và trường `ip` trên màn hình thiết bị đều lấy từ `c.ClientIP()`. Sau Caddy, giá trị đó là IP container của Caddy chứ không phải của người dùng — xem §9.5.

### Sandbox lab

Container do học viên gõ lệnh = code lạ chạy trên máy chủ. Bắt buộc:

- **Không mount `/var/run/docker.sock` vào container lab.** API truy cập Docker qua `tecnativa/docker-socket-proxy`, chỉ mở `containers/create,start,exec,remove`.
- `--cap-drop=ALL`, `--security-opt=no-new-privileges`, user non-root trong container
- `--memory`, `--cpus`, `--pids-limit`, `--read-only` + tmpfs
- `--network=none` cho **mọi** lab
- Reaper luôn chạy, dựa trên `expires_at` trong DB (không dựa vào trạng thái trong RAM)
- **Không dùng `--privileged`** trong bất kỳ trường hợp nào, kể cả tạm để test.
- Trần `MAX_CONTAINERS` đếm chứ không đặt chỗ — hai lượt start cùng khoảnh khắc có thể cùng lọt và đẩy máy vượt một ghế. Đó là một ghế, không phải một sự cố; đánh đổi lấy việc không phải giữ lock qua một lời gọi docker.
- Nâng cấp trước khi mở công khai rộng: gVisor (`--runtime=runsc`)

### Admin

1. Admin đầu tiên tạo bằng CLI trên server: `make admin email=... password=...`. **Không có route đăng ký admin trong API.**
2. **TOTP 2FA bắt buộc** cho role admin.
3. Role lưu ở bảng `user_roles` riêng, không phải cột `is_admin` trên `users`.
4. `check_script` **chỉ chạy trong container lab đã hardened**, không bao giờ trên host.
5. Image lab chọn từ bảng `lab_images` (dropdown), admin **không gõ tên image tự do**.
6. Audit log append-only — không có `DELETE /api/admin/audit`.
7. Rate limit route admin, log mọi lần đăng nhập thất bại.

---

## 8. Môi trường

**Một `Dockerfile` cho mọi nơi — áp dụng cho API (Go).** Cùng file build ra image chạy local lẫn prod; khác nhau chỉ ở env var inject lúc chạy. Không có `Dockerfile.prod` riêng.

Nhưng **image không đi từ máy dev lên prod**: prod build lại từ nguồn ngay trên máy Oracle. Lý do ở §9.2 — runner của GitHub Actions là amd64, giả lập arm64 qua QEMU chậm gấp cả chục lần, trong khi 4 nhân ARM build image alpine trong vài giây. Hệ quả nằm ở rollback, xem ngay dưới bảng.

> **FE khác API:** Vite **bake `VITE_API_URL` lúc build**, không phải runtime. Build FE prod ≠ build FE local → mỗi môi trường một lần build riêng, truyền `VITE_API_URL` qua `--build-arg`. Không dùng chung một image FE cho cả 2 env.

|           | local                          | production                                                              |
| --------- | ------------------------------ | ----------------------------------------------------------------------- |
| Chạy bằng | `docker compose up`            | `docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d` |
| Kiến trúc | amd64 (máy dev)                | **arm64** (Ampere A1) — xem §9.2                                        |
| Config    | `.env` (từ `.env.example`)     | env file trên server, `chmod 600`, chủ sở hữu là user chạy compose      |
| DB        | postgres container, seed giả   | postgres volume + pg_dump cron → R2                                     |
| TLS       | không                          | Caddy + Let's Encrypt                                                   |
| Log       | stdout                         | slog JSON → Grafana Alloy → Grafana Cloud Loki                          |
| Deploy    | hot reload (`air`, `vite dev`) | GitHub Actions → SSH → `git fetch` + `compose build` + `up -d`          |

Cột production ở trên là đích, không phải hiện trạng: `docker-compose.prod.yml`, `deploy/Caddyfile.prod` và workflow CD **chưa có trên đĩa**. `deploy/` mới chỉ có `caddy/Caddyfile` (bản dev, `:80`, `auto_https off`) và một `monitoring/` rỗng.

**Rollback = checkout SHA cũ rồi build lại, không phải đổi tag.** Vì image sinh ra trên chính máy prod, không có tag cũ nào nằm sẵn ở registry để trỏ về. Giá phải trả: bản lỗi còn phục vụ thêm 1-2 phút trong lúc build.

Muốn rollback tính bằng giây thì đổi sang **self-hosted runner ARM ngay trên máy đó** — build native, push GHCR, rollback thành `IMAGE_TAG=<sha-cũ> compose up -d`. Đánh đổi: thêm một runner phải bảo trì, và runner đó có quyền trên host prod. Chưa làm; ghi ở đây để lần sau không phải suy luận lại.

### Biến môi trường

| Biến                                          | local                   | production             | Nguồn      |
| --------------------------------------------- | ----------------------- | ---------------------- | ---------- |
| `APP_ENV`                                     | development             | production             | compose    |
| `PORT`                                        | 8080                    | 8080                   | compose    |
| `LOG_LEVEL`                                   | debug                   | info                   | compose    |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_NAME` | (compose)               | (compose)              | compose    |
| `DB_PASSWORD`                                 | `.env` giả              | env file trên server   | **secret** |
| `REDIS_ADDR` / `REDIS_PASSWORD` / `REDIS_DB`  | localhost:6379          | (compose)              | compose    |
| `COOKIE_DOMAIN`                               | (rỗng)                  | (rỗng)                 | compose    |
| `JWT_SECRET`                                  | `.env` giả              | env file trên server   | **secret** |
| `ACCESS_TTL` / `REFRESH_TTL`                  | `15m` / `168h`          | `15m` / `168h`         | compose    |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`   | `.env`                  | env file trên server   | **secret** |
| `GOOGLE_REDIRECT_URL`                         | `http://localhost:8080/api/auth/google/callback` | `https://<domain>/api/auth/google/callback` | compose |
| `CORS_ORIGINS`                                | `http://localhost:5173` | `https://<domain>`     | compose    |
| `FRONTEND_URL`                                | `http://localhost:5173` | `https://<domain>`     | compose    |
| `PUBLIC_URL`                                  | `http://localhost:8080` | `https://<domain>` — **origin trần, không có `/api`** | compose |
| `UPLOAD_DIR`                                  | `./uploads`             | `./uploads` (bind mount, xem "Domain & routing") | compose |
| `TRUSTED_PROXIES`                             | `127.0.0.1,::1`         | `127.0.0.1,::1,172.16.0.0/12` — **bắt buộc, xem §9.5** | compose |
| `DATABASE_URL` (migrate)                      | `sslmode=disable`       | `sslmode=require`      | **secret** |
| `LAB_DOCKER_HOST`                             | `tcp://127.0.0.1:2375`  | `tcp://127.0.0.1:2375` | compose    |
| `LAB_SESSION_TTL`                             | `60m`                   | `30m` (xem §9.4)       | compose    |
| `MAX_CONTAINERS`                              | `40`                    | `40`                   | compose    |
| `PUBLIC_RATE_LIMIT`                           | `60`                    | `60`                   | compose    |
| `OPENROUTER_API_KEY`                          | (rỗng)                  | **(rỗng)** — xem §9.6  | **secret** |
| `OPENROUTER_MODEL`                            | —                       | `anthropic/claude-haiku-4-5` | compose |
| `AI_DAILY_LIMIT`                              | `10`                    | `3` (xem §9.6)         | compose    |
| `SMTP_HOST` / `SMTP_PORT`                     | `localhost` / `1025` (Mailpit) | Resend hoặc Brevo / `587` | compose |
| `SMTP_USER` / `SMTP_PASSWORD`                 | (rỗng — Mailpit không hỏi) | (bắt buộc)          | **secret** |
| `MAIL_FROM`                                   | `no-reply@devforge.local` | `no-reply@<domain>`  | compose    |
| `VERIFY_CODE_TTL` / `RESET_TOKEN_TTL` / `RESEND_COOLDOWN` | `10m` / `1h` / `60s` | như local        | compose    |
| `VITE_API_URL` (FE, **build-time**)           | `http://localhost:8080` | `https://<domain>/api` | build-arg  |

**Nhóm SMTP là chỗ hỏng im lặng.** Local có Mailpit nuốt mọi thư nên không ai thấy thiếu; prod không có gì đứng thay, và mã xác thực với link reset mật khẩu là hai thứ duy nhất đi qua đường đó. Thiếu `SMTP_HOST` ở prod = người đăng ký mới không bao giờ vào được, không có lỗi nào nổ ở phía server.

`.env.example` phải liệt kê đủ biến trên (giá trị giả).

**Secret prod nằm trong env file trên server, sửa bằng tay — CD không đẩy secret từ GitHub Secrets xuống.** Actions chỉ giữ ba secret để mở được cửa: `SSH_HOST`, `SSH_USER`, `SSH_KEY`. Đổi lại là ba thay vì hơn chục, và không secret prod nào đi ngang qua Actions. Giá phải trả: thêm một biến môi trường nghĩa là một lần ssh, không phải một lần commit — và **file đó không có bản sao ở đâu cả**: `scripts/backup.sh` dump database chứ không dump config. Dựng lại máy từ đầu mà không có bản `.env` trong tay thì phải sinh lại toàn bộ khoá, và mọi phiên đăng nhập hiện có mất theo `JWT_SECRET`.

### Healthcheck (điều kiện rollback)

> ⚠️ **Chưa làm.** `cmd/server/main.go` không đọc `os.Args` và không có `flag` nào — subcommand `healthcheck` chưa tồn tại. Cho tới khi nó có, bước deploy phải tự probe từ ngoài (xem CI/CD bên dưới), và cái compose healthcheck dưới đây là thứ phải viết chứ không phải thứ đang chạy.

Rollback tự động ở CD chỉ chạy được nếu `api` khai báo healthcheck trong compose:

```yaml
api:
  healthcheck:
    test: ["CMD", "/server", "healthcheck"] # binary tự gọi /healthz nội bộ
    interval: 10s
    timeout: 3s
    retries: 5
```

Image prod là **distroless (không shell/wget)** → không dùng được `CMD-SHELL "wget ..."`. Phải thêm subcommand `/server healthcheck` vào binary để tự probe `/healthz`.

Trong lúc chưa có, bước deploy kiểm tra từ phía ngoài container thay thế — `curl -fsS https://<domain>/readyz` lặp lại vài lần sau `up -d`. Kém hơn một bậc: nó không phân biệt được "API chưa boot xong" với "Caddy chưa route", nhưng nó không đòi sửa binary.

### Domain & routing

**1 domain, route bằng path** qua Caddy edge → không CORS ở prod, 1 cert TLS:

```
https://<domain>/          → FE static (Caddy serve)
https://<domain>/api/*     → Go API
https://<domain>/ws/*      → Go API (WebSocket → wss)
https://<domain>/uploads/* → Go API   ← KHÔNG nằm trong /api, xem dưới
https://<domain>/healthz   → Go API
https://<domain>/readyz    → Go API
```

**`/uploads/*` là cái bẫy của bảng này.** `cmd/server/router.go:32` gắn `r.Static("/uploads", cfg.UploadDir)` lên router gốc, ngoài group `/api`, và `internal/upload/image.go:100` dựng URL bằng `PUBLIC_URL + "/uploads/" + name`. Một Caddyfile chỉ route `/api/*`, `/ws/*` và `/` sẽ đẩy mọi ảnh bìa với avatar vào khối bắt-tất-cả của SPA — SPA trả `index.html` kèm **200**, nên thứ nhìn thấy là ảnh vỡ chứ không phải 404 để mà grep. Hai chỗ phải khớp nhau: route ở Caddy, và `PUBLIC_URL` phải là origin trần (`https://<domain>`), không kèm `/api`.

Thư mục `UPLOAD_DIR` phải là bind mount hoặc volume — nằm trong lớp ghi của container thì mỗi lần `up -d` là mất sạch ảnh đã upload.

Hệ quả khi chốt domain:

- FE build với `VITE_API_URL=https://<domain>/api`, WS dùng `wss://<domain>/ws/...`
- `CORS_ORIGINS` prod = same-origin → gần như chỉ còn cần cho dev (`:5173`)
- OAuth: đăng ký redirect `https://<domain>/api/auth/google/callback` ở Google Console + set `GOOGLE_REDIRECT_URL` khớp từng ký tự
- `PUBLIC_URL` và `FRONTEND_URL` cùng trỏ `https://<domain>` — cùng origin nên hai biến trùng giá trị ở prod, khác nhau chỉ ở local
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

Hai nửa, và chỉ nửa đầu có thật trên đĩa.

**CI — `.github/workflows/ci.yml`, đang chạy:**

```
push master | pull_request
  job be      → gofmt -l | (! grep .)  →  go vet  →  go build  →  go test
  job secrets → gitleaks (fetch-depth: 0, cần quyền pull-requests: read)
```

**CD — chưa có workflow nào.** Không có job build image, không trivy, không push registry, không bước deploy. Đây là hình dạng nó phải có, không phải hiện trạng:

```
merge master → ssh vào máy Oracle
             → git fetch && git checkout <sha>
             → docker compose build            (native arm64, vài giây)
             → migrate up                      (tương thích ngược, xem trên)
             → docker compose up -d
             → curl -fsS https://<domain>/readyz, lặp có timeout
             → đỏ  → checkout <sha trước> && build && up -d   (1-2 phút)
```

Ba điều kiện, và không cái nào là tuỳ chọn:

1. **`/readyz` phải trả đỏ khi API chưa nối được DB** — nếu không thì bước kiểm tra luôn xanh và rollback không bao giờ bắn. Cái này **đã có**: `cmd/server/router.go:74` ping database với timeout 2 giây và trả `503` khi ping hỏng. `/healthz` thì luôn `200` — nó chỉ nói tiến trình còn sống, nên **đừng dùng `/healthz` làm điều kiện rollback.**
2. **Migration chạy trước `up -d`, và phải tương thích ngược.** Rollback chỉ lùi code, không lùi schema — một migration drop cột trong một bước làm binary cũ chạy trên schema nó không đọc được, và lúc đó không còn đường về nào ngoài restore.
3. **Ba secret `SSH_HOST` / `SSH_USER` / `SSH_KEY`.** Khoá SSH riêng cho deploy, không dùng lại khoá cá nhân, và giới hạn được tới đâu thì giới hạn.

Nhánh mặc định là `master`. `ci.yml` theo dõi `push: [master]` + `pull_request` — trước đó nó theo dõi `main`, nghĩa là mọi lần merge đều vào mà CI không chạy lần nào.

Repo để public thì Actions không giới hạn phút và GHCR không giới hạn dung lượng — xem §9.3.

### Staging — phương án, chưa dựng

Lý do cần nằm ngay trong §9.5: hai lỗi mô tả ở đó *"chỉ nổ sau khi deploy (dev FE gọi thẳng `:8080` nên không thấy)"*. Cùng loại với chúng, và cũng chỉ lộ ra sau khi lên máy thật:

`TRUSTED_PROXIES` sau Caddy · `/uploads/*` với `PUBLIC_URL` · OAuth redirect URI thật · Caddy xin cert Let's Encrypt · `VITE_API_URL` bake lúc build · build arm64 · migration ba bước tương thích ngược · và chính cái rollback.

**Không dựng máy thứ hai.** Always Free là 4 OCPU / 24 GB **tổng cho cả tài khoản**, chia ra hai VM thì prod mất phần — mà §9.4 đã chốt trần của prod là CPU chứ không phải RAM. Cắt một OCPU cho staging là cắt một phần tư sức chứa lab.

Dựng **stack thứ hai trên cùng máy**: compose project khác tên, volume khác, một site block nữa trong Caddyfile.

```
https://<domain>/       → devforge-be   (prod)
https://stg.<domain>/   → devforge-stg  (staging)
```

Tốn thêm khoảng 1 GB RAM trong 24 GB, và gần như không CPU lúc rảnh. 0₫.

**Bẫy phải xử trước khi dựng — trần container đếm sai khi có hai stack.**

`internal/labs/adapter/repo/session.go:183` đếm ghế từ **database của chính nó**, không hỏi docker daemon:

```sql
SELECT count(*) FROM lab_sessions WHERE status = 'running' AND container_id <> ''
```

Hai stack dùng chung một daemon nhưng hai database riêng → mỗi bên tin rằng nó có đủ 40 ghế, và cùng nhau đẻ 80 container lên 4 nhân. `MAX_CONTAINERS` là trần trên *một cơ sở dữ liệu*, không phải trên *một cái máy*. Nên tổng hai bên phải bằng con số máy chịu được:

```
prod     MAX_CONTAINERS=37
staging  MAX_CONTAINERS=3
```

Reaper thì không có vấn đề tương tự: `DueForReaping` (`session.go:252`) quét theo `lab_sessions` của chính nó chứ không liệt kê container toàn daemon, nên hai stack không dọn nhầm container của nhau.

Staging cắt bớt cho rẻ:

| | prod | staging |
| --- | --- | --- |
| Postgres | volume riêng + `pg_dump` → R2 | volume riêng, **không backup** |
| Redis | riêng | dùng chung, khác `REDIS_DB` |
| `LAB_SESSION_TTL` | `30m` | `10m` |
| `OPENROUTER_API_KEY` | (rỗng, xem §9.6) | rỗng — đừng đốt tiền AI ở staging |
| Email | Resend / Brevo | Mailpit (đã có sẵn trong compose) |
| Grafana Alloy | có | không |
| Trigger deploy | merge `master` | push `develop` |

Thứ tự: **dựng prod cho xong trước.** Staging tồn tại để diễn tập một pipeline đã có; dựng nó trước là diễn tập cho thứ chưa viết.

---

## 9. Hạ tầng production — chạy 0₫

### 9.1 Máy chủ: Oracle Cloud Always Free

| | Always Free (vĩnh viễn) | Dự án cần |
| --- | --- | --- |
| CPU | 4 OCPU ARM Ampere A1 | 4+ |
| RAM | **24 GB** | ~10 GB |
| Disk | 200 GB block storage | ~60 GB |
| Egress | 10 TB/tháng | ~vài chục GB |
| Vùng | Singapore / Osaka / Tokyo | Singapore |
| Giá | **0₫ vĩnh viễn** | — |

Phải là VPS thật có root và cài được Docker — code đẻ container qua Docker Engine API, nên mọi PaaS (Vercel, Netlify, Render, Railway, Fly.io) đều loại ngay từ đầu.

Singapore → độ trễ tới VN 30–50ms. Đây là ràng buộc cứng, không phải sở thích: terminal là xterm.js qua WebSocket, mỗi phím gõ là một vòng round-trip. Hetzner ở EU rẻ hơn nhưng 250–300ms làm hỏng đúng tính năng cốt lõi.

**Ngay sau khi tạo tài khoản, nâng lên Pay As You Go.** Vẫn 0₫ nếu ở trong hạn mức free, nhưng được miễn cơ chế thu hồi tài nguyên Always Free để rảnh.

### 9.2 ARM64 — không phải sửa dòng nào

Ampere A1 là ARM64. Toàn bộ Dockerfile, compose và workflow **không ghim kiến trúc ở đâu cả**:

```bash
grep -rniE "amd64|x86_64|GOARCH|platform" Dockerfile labs/*/Dockerfile \
     ../devforge-fe/Dockerfile docker-compose.yml .github/workflows/*.yml
# → không có kết quả
```

Backend build `CGO_ENABLED=0` → Go tĩnh thuần, biên dịch chéo sạch. Các gói `apk` tự phân giải theo kiến trúc. Mười base image đều đã xác nhận có `linux/arm64`:

| Image | arm64 |
| --- | --- |
| `alpine:3.21` (4 image lab) | ✅ |
| `golang:1.25-alpine` | ✅ |
| `gcr.io/distroless/static-debian12:nonroot` | ✅ |
| `node:22-alpine` | ✅ |
| `caddy:2-alpine` | ✅ |
| `postgres:16-alpine` | ✅ |
| `redis:7-alpine` | ✅ |
| `migrate/migrate:v4.18.1` | ✅ |
| `axllent/mailpit:v1.21` (chỉ dev) | ✅ |
| `tecnativa/docker-socket-proxy:0.3.0` | ✅ |

Kiểm lại bất cứ lúc nào:

```bash
docker manifest inspect tecnativa/docker-socket-proxy:0.3.0 \
  | grep -A3 '"platform"' | grep architecture
```

**Build image ngay trên máy Oracle, không dùng `buildx` + QEMU trong GitHub Actions.** Runner của Actions là amd64; giả lập arm64 chậm gấp cả chục lần. Bốn nhân ARM build image alpine trong vài giây. Bước deploy chạy `docker compose build` qua SSH thay vì `pull` từ GHCR — hoặc dựng một self-hosted runner ngay trên chính máy đó.

### 9.3 Bảng free tier

| Khoản | Dịch vụ | Hạn mức free | Đủ không |
| --- | --- | --- | --- |
| Máy chủ | Oracle Always Free A1 | 4 OCPU / 24 GB | ✅ dư |
| TLS | Caddy + Let's Encrypt | — | ✅ |
| DNS | Cloudflare Free | — | ✅ |
| Metrics + log + alert | **Grafana Cloud Free** | 10k series, 50GB log, 14 ngày | ✅ và không tốn RAM trên máy |
| Backup | **Cloudflare R2** | 10 GB, egress 0₫ | ✅ dump nén thừa sức |
| Email | Resend 3000/tháng, hoặc Brevo 300/ngày | | ✅ chỉ dùng cho mã xác thực + reset |
| CI | GitHub Actions | 2000 phút/tháng private, **không giới hạn nếu repo public** | ✅ CI hiện ~5 phút/lần |
| Registry | GHCR | không giới hạn cho package public | ✅ |
| Analytics | Umami Cloud | 10k event/tháng | ✅ |
| Google OAuth | | | ✅ |
| Sinh kịch bản sim | OpenRouter | không có free tier dùng được — xem §9.6 | ⚠️ |

**Tổng: 0₫/tháng.** Khoản duy nhất phải trả là **tên miền ~300k₫/năm**. DuckDNS free nhưng phải build Caddy kèm plugin DNS-01 và tên miền trông không chuyên nghiệp — 300k/năm là chỗ đáng trả tiền nhất trong dự án.

### 9.4 Sức chứa: CPU là trần, không phải RAM

```
40 container × 512 MB (trần Docker)  = 20 GB   ← trần lý thuyết
40 container × ~150 MB (RSS thật)    = 6 GB    vs 21 GB còn trống  → thoải mái
40 container × 0.5 vCPU (nanoCPUs)   = 20 vCPU vs 4 nhân thật      → oversubscribe 5×
```

`Memory` của Docker là **trần, không phải đặt chỗ**; image nền là alpine chạy `sleep infinity` + shell, RSS thật 20–60 MB lúc rảnh. `NanoCPUs` cũng là hạn ngạch (cfs_quota) chứ không phải đặt chỗ, nên oversubscribe không sao khi container rảnh.

Nhưng **chỉ ~8 container có thể bận CPU cùng lúc** trên 4 nhân. Với lab dạy học (phần lớn thời gian học viên đang gõ và đọc) thì `MAX_CONTAINERS=40` là ổn. Thấy chậm thì **hạ xuống 25**, đừng nâng lên.

Ba đòn bẩy tăng số người phục vụ được, đều là biến môi trường hoặc thiết kế nội dung — không sửa code:

1. **Bài mô phỏng tốn 0 container.** Bốn engine sim (CI/CD, Linux, tìm kiếm, sắp xếp) không chiếm ghế nào; kiểm tra sức chứa nằm *sau* nhánh sim trong `labs.go` một cách có chủ đích. Xếp sim lên trước container trong lộ trình học → nhân đôi số người vào cùng lúc.
2. **`LAB_SESSION_TTL` 60m → 30m.** Reaper quét theo deadline này, nên vòng quay ghế gấp đôi ngay.
3. **`MAX_CONTAINERS`** điều chỉnh theo tải thật đo được ở Grafana, không theo cảm giác.

### 9.5 ⚠️ `TRUSTED_PROXIES` — bắt buộc set khi bật Caddy

Mọi thứ nhận dạng người gọi đều đọc `c.ClientIP()`: rate limit theo địa chỉ trên các route chia sẻ công khai (`PUBLIC_RATE_LIMIT`), cột `audit_logs.ip`, và địa chỉ trên màn hình thiết bị đang đăng nhập. Giá trị đó chỉ đúng khi server biết ai được phép nói thay người khác qua `X-Forwarded-For`.

`TRUSTED_PROXIES` mặc định là `127.0.0.1,::1` — đúng cho dev, vì không có gì đứng trước API nên không header nào đáng tin. **Prod phải khai báo lại**, vì `deploy/caddy/Caddyfile` proxy bằng `reverse_proxy api:8080` qua mạng bridge của compose: peer mà Gin thấy là IP container của Caddy (`172.x.x.x`), không nằm trong danh sách mặc định, nên `X-Forwarded-For` bị bỏ qua và mọi request trả về chính IP của Caddy.

Để mặc định ở prod thì hỏng hai chỗ, cả hai chỉ nổ sau khi deploy (dev FE gọi thẳng `:8080` nên không thấy):

| Chỗ hỏng | Triệu chứng |
| --- | --- |
| `internal/labs/adapter/ratelimit/redis.go` — key là `c.ClientIP()` | `PUBLIC_RATE_LIMIT` 60/phút **theo địa chỉ** biến thành 60/phút **toàn cục**. Một người xem link chia sẻ làm cạn quota của tất cả. Self-DoS trên đúng bốn route sinh ra để đón người lạ. |
| `audit_logs.ip`, `sess.ip` (màn hình thiết bị) | Mọi hàng ghi cùng một IP container. Màn hình "thiết bị đang đăng nhập" mất ý nghĩa, audit mất dấu vết IP. |

Cấu hình prod:

```bash
TRUSTED_PROXIES=127.0.0.1,::1,172.16.0.0/12
```

Chỉ mở tới dải bridge khi **API không publish cổng nào ra ngoài** và Caddy là lối vào duy nhất. Lab container chạy `--network=none` nên không tự nói chuyện được với API, nhưng điều kiện trên vẫn phải giữ. Cách khác: cho Caddy chạy `network_mode: host` và proxy tới `127.0.0.1:8080`, khi đó giữ nguyên mặc định.

**Không bao giờ đặt `0.0.0.0/0`.** Tin mọi peer nghĩa là người gọi tự chọn địa chỉ của mình bằng cách gửi header — rate limit và dấu vết audit giao lại cho bất cứ ai hỏi. Server ghi `slog.Warn` lúc khởi động nếu thấy giá trị này, nhưng vẫn chạy: một server không chịu boot vì cấu hình proxy thì làm sập site theo hướng ngược lại.

Hành vi được ghim bởi `cmd/server/trustedproxies_test.go`, gồm cả trường hợp tái hiện chính lỗi trên (`caddy unlisted swallows every caller into one address`).

**Giữ Cloudflare ở chế độ DNS-only (mây xám).** Bật proxy (mây cam) là thêm một tầng nữa vào cùng bài toán — lúc đó `TRUSTED_PROXIES` phải kể thêm dải IP của Cloudflare, và Caddy phải xin cert qua tầng đó. Ẩn IP gốc là điểm cộng, nhưng để sau.

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

### 9.7 Rủi ro thật của phương án free

1. **Oracle thu hồi tài nguyên Always Free để rảnh.** Chặn bằng cách nâng lên Pay As You Go (vẫn 0₫). Làm ngay khi tạo tài khoản.
2. **"Out of host capacity" khi tạo máy A1.** Rất phổ biến ở Singapore/Tokyo. Có thể phải thử lại nhiều ngày — viết script gọi API tạo máy lặp lại, hoặc chấp nhận vùng xa hơn và trả giá bằng độ trễ.
3. **Không có SLA. Oracle có thể khoá tài khoản free gần như không giải thích.** Với dự án dạy học / portfolio thì chấp nhận được. Với học viên trả tiền thì không — lúc đó chuyển sang VPS trả phí, cùng compose, cùng Ansible, đổi mỗi cái IP.
4. **Hệ quả của (3): backup là chặng ĐẦU TIÊN, không phải chặng cuối.** Nếu Oracle xoá tài khoản, bản `pg_dump` trên R2 là thứ duy nhất còn lại. Lộ trình bên dưới xếp lại theo đúng thứ tự này.

### 9.8 Nếu không lấy được máy A1

| Phương án | Giá | Đánh đổi |
| --- | --- | --- |
| **GitHub Student Pack** (nếu là sinh viên) | 0₫ | DigitalOcean $200 credit ≈ 4 tháng, + tên miền `.me` free. Hết credit là hết |
| **Oracle 2× AMD Micro** (1/8 OCPU, 1 GB, cũng always free) | 0₫ | Quá nhỏ — ~3 container. Chỉ đủ demo |
| **Contabo Singapore** 8 vCPU/24 GB | ~400k₫/tháng | Hay oversell, IO chậm. Rẻ nhất trong nhóm trả phí có độ trễ chấp nhận được |
| **VPS Việt Nam** 4 vCPU/8 GB, `MAX_CONTAINERS=15` | ~1,0tr₫/tháng | Độ trễ tốt nhất, hỗ trợ tiếng Việt |

---

## 10. Lộ trình

| Chặng    | Nội dung                                                                    | Trạng thái |
| -------- | --------------------------------------------------------------------------- | ---------- |
| **P0**   | Scaffold: vite app, Gin api, docker-compose, migration, lint, lefthook      | ✅ |
| **P1**   | Auth: register/login JWT + Google OAuth + middleware + Context ở FE         | ✅ |
| **P2**   | Landing + danh sách khoá học + chi tiết khoá học (4 tab)                    | ✅ |
| **P3**   | **Lab runtime**: container lifecycle, socket-proxy, WS terminal, reaper 60' | ✅ |
| **P4**   | Chấm điểm: check script, submit, submissions, trang Lịch sử                 | ✅ |
| **P4.5** | Roles, `RequireRole`, CLI tạo admin, TOTP 2FA, audit log                    | ✅ |
| **P5**   | Admin: CRUD khoá học/lab/task, dry-run check_script, draft→publish          | ✅ |
| **P6**   | Admin: users (ban/unban), sessions đang chạy (kill)                         | ✅ |
| **P7**   | Leaderboard + tab Trạng thái + chat WS                                      | ✅ |
| **P8**   | Nội dung: 3 khoá (Linux, Git, Docker) × 5 lab                               | ✅ |
| **P10**  | Backup pg_dump → R2 + diễn tập restore                                      | ✅ script — chờ bucket + cron, xem `deploy/DEPLOY.md` §G |
| **P3.5** | Observability: metrics Go + Grafana Alloy → Grafana Cloud + alert           | ❌ |
| **P9**   | Deploy prod: Caddy prod, CD qua SSH, set `TRUSTED_PROXIES` (§9.5)           | ✅ — bỏ Ansible, xem `deploy/DEPLOY.md` |
| **P11**  | _(tuỳ chọn)_ tách runner node riêng, gVisor, hoặc chuyển k3s                | — |

**Thứ tự đảo lại so với bản trước: P10 → P3.5 → P9.** Trên hạ tầng free không có SLA, backup là thứ duy nhất còn lại khi mọi thứ khác biến mất — làm nó trước khi có gì để mất. Rồi đến quan sát được, rồi mới tới tự động deploy.

Ngoài lộ trình, đã làm thêm: bài mô phỏng CI/CD (`SIM-CICD.md`), mô phỏng Linux / tìm kiếm / sắp xếp, War Room (lab sự cố có hạn giờ, link chia sẻ công khai, ca trực ngày, chuỗi ngày, bảng tuần), bảng `system_events` và bốn màn quản trị đi kèm, tin nhắn riêng trong chat, sổ tay ôn tập.

**Ước lượng còn lại: 4–6 ngày công** (giảm từ 6–9 nhờ bỏ Terraform và dùng Grafana Cloud thay stack tự dựng).

Metrics tối thiểu ở P3.5:

```
devforge_lab_sessions_active          gauge
devforge_lab_containers_running       gauge   # so với gauge trên → phát hiện rò rỉ
devforge_reaper_last_run_timestamp    gauge   # alert nếu > 5 phút
devforge_reaper_killed_total          counter
devforge_lab_start_duration_seconds   histogram
```

Alert quan trọng nhất: `lab_containers_running > lab_sessions_active` kéo dài → có container mồ côi. Trên máy 4 nhân, container mồ côi ăn RAM và ghế cho tới khi hết — đây là alert giữ cho máy free sống được.

---

## 11. Cấu trúc — polyrepo

Tách **2 repo độc lập** (kiểu outsource), gom trong 1 workspace để dễ quản lý:

```
devforge-workspace/
├── OPEN-QUESTIONS.md           # giả định đã dùng, sai thì vỡ ở đâu
├── SIM-CICD.md                 # thiết kế engine mô phỏng pipeline
│
├── devforge-be/                # REPO 1 — Go backend + orchestration
│   ├── cmd/server · cmd/createadmin · cmd/checksim
│   ├── internal/{audit,auth,chat,config,courses,db,events,i18n,labs,upload}
│   ├── migrations/             # SQL, golang-migrate
│   ├── labs/                   # Dockerfile 4 image lab + rc file dùng chung
│   ├── scripts/                # seed.sql, check-seed.sh, smoke-auth.sh, sim-pipelines/
│   ├── deploy/
│   │   ├── caddy/              # Caddyfile (dev) + Caddyfile.prod (TLS)
│   │   ├── bootstrap.sh        # dựng máy 1 lần — thay Ansible
│   │   ├── up.sh               # build → migrate → up → health → rollback
│   │   └── DEPLOY.md           # runbook triển khai
│   ├── .github/workflows/ci.yml   # gofmt/vet/build/test + gitleaks
│   ├── Dockerfile              # build → prod (distroless)
│   ├── docker-compose.yml      # postgres + redis + mailpit + docker-proxy (dev)
│   ├── docker-compose.prod.yml # + api, web, caddy — mailpit tắt bằng profile
│   ├── .air.toml · Makefile · lefthook.yml · .env.example
│
└── devforge-fe/                # REPO 2 — Vite + React (chạy standalone)
    ├── src/{pages,components,api,hooks,context,lib,sims}
    │   ├── pages/admin/
    │   └── lib/*.check.ts      # assert của node, không framework
    ├── .github/workflows/ci.yml   # oxlint/tsc/build + gitleaks
    ├── Dockerfile              # dev / build / prod (Caddy serve static)
    ├── Caddyfile.static · vite.config.ts · lefthook.yml · .env.example
```

`deploy/monitoring/` đã bỏ — Prometheus/Grafana/Loki chạy ở Grafana Cloud, trên máy chỉ còn Alloy agent khai báo trong compose. `deploy/terraform/` đã bỏ, xem §1.

**Vì sao polyrepo:** be/fe tách sạch theo ranh giới repo — CI riêng, deploy riêng, phân quyền riêng. `be` sở hữu orchestration (compose, DB, deploy) vì nó là hub triển khai cả stack. `fe` là app frontend thuần, dev chạy độc lập trỏ về API.

---

## 12. Chạy local

Hai repo độc lập, mở **2 terminal**:

```bash
# Terminal 1 — backend stack (postgres + redis + mailpit + docker-proxy + api)
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
| `localhost:6379` | Redis (refresh session)                   | devforge-be |
| `localhost:8025` | Mailpit — xem mọi mail app gửi            | devforge-be |
| `localhost:2375` | docker-socket-proxy                       | devforge-be |

Kiểm tra API: `curl localhost:8080/healthz` · `curl localhost:8080/readyz` · `curl localhost:8080/api/ping`

`make help` (trong `devforge-be`) xem toàn bộ lệnh.

`make migrate` gộp cả bốn bước và chạy lại được bao nhiêu lần cũng được: migration
đã chạy thì bỏ qua, seed chỉ chèn vào chỗ trống, image có cache. Tài khoản đầu
tiên (`superadmin` / `lukas`) do migration `000020` tạo chứ không do seed, nên
chúng có mặt kể cả khi bỏ qua dữ liệu demo.

Ở môi trường **không phải máy local** dùng `make migrate-schema` — chỉ chạy
migration, không nạp nội dung demo và không build image.

### Kiểm tra trước khi push

```bash
# devforge-be
gofmt -l . && go vet ./... && go test ./...
make check-seed               # cần docker: mọi check_script phải TRƯỢT trên container mới,
                              # rồi phải ĐẬU sau lời giải trong scripts/seed-solutions.tsv
make check-sim                # cần DB, không cần docker: pipeline sai phải trượt, đúng phải đậu

# devforge-fe
npx tsc --noEmit && npx oxlint && npm run check
```

`npm run check` chạy 7 file assert (`json`, `clock`, `mdSummary`, `sim`, `linux`, `search`, `sort`) bằng
`node --experimental-strip-types`, không framework. **Bảy file này chưa nằm trong CI** — `ci.yml` của FE
mới chỉ có `lint → tsc → build`, nên logic engine mô phỏng hỏng thì CI vẫn xanh. Thêm một dòng
`- run: npm run check` là xong.

`make check-seed` và `make check-sim` cũng ngoài CI — chúng cần docker và database, chấp nhận được,
nhưng phải chạy tay khi sửa nội dung seed.

> Compose đặt tên `devforge-be`, tránh trùng volume/container project khác trên máy.
> Caddy edge (`deploy/caddy`) chỉ dùng ở **prod** — dev thì FE gọi thẳng API, không qua proxy.
