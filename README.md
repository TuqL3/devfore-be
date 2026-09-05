# DevForge

Nền tảng học DevOps qua lab thực hành. Mỗi bài lab cấp cho học viên một **container Linux thật**, truy cập qua terminal trong trình duyệt, giới hạn 60 phút.

Chạy production ở mức **0₫/tháng** — xem [§9 Hạ tầng production](#9-hạ-tầng-production--chạy-0).

---

## 0. Tóm tắt

```
Vite/React/TS/Tailwind ─HTTP+WS→ Go/Gin/GORM ─→ Postgres
                                      │
                             socket-proxy → Docker → lab container (hardened, TTL 20')

be:  Cloudflare(DNS+TLS) → nginx → Docker Compose → Hostinger VPS (amd64, Singapore)
fe:  Cloudflare Pages                                   ← hạ tầng riêng, không đụng VPS

Actions → GHCR → ssh (repo be)      ·      Actions → wrangler pages deploy (repo fe)
```

Ba khối chức năng:

1. **Học viên** — landing, danh sách khoá học, chi tiết khoá học (4 tab), lab + terminal thật, chấm điểm, lịch sử, bảng xếp hạng, chat chung, bài mô phỏng, War Room
2. **Admin** — CRUD khoá học/lab/task, chạy thử `check_script`, ban user, kill session đang chạy, audit log, bảng sự kiện hệ thống
3. **Hạ tầng** — 2 môi trường local/production, CI/CD, cấu hình server, observability, backup + diễn tập restore

Đã chốt hạ tầng: **Hostinger VPS KVM2 (2 vCPU amd64 / 8 GB, Singapore)** cho backend, **Cloudflare Pages** cho frontend, **nginx + Cloudflare Origin Certificate** ở biên. Hai repo, hai hạ tầng, không giao nhau. Chi phí cố định = VPS + tên miền; xem §9.

**Phương án Oracle Always Free / Caddy / một domain là bản cũ và chưa từng dựng thật.** Việc phải làm để chuyển sang bản đã chốt: **§13**.

---

## 1. Stack

### Frontend

| Thành phần   | Chọn                               |
| ------------ | ---------------------------------- |
| Build        | Vite + React 19 + TypeScript       |
| Router       | React Router v7                    |
| Server state | TanStack Query                     |
| Client state | React Context                      |
| UI           | TailwindCSS v4 (`@tailwindcss/vite`) |
| Terminal     | xterm.js + `@xterm/addon-fit`      |
| Form         | `<form>` + validate tay (chưa cần lib) |
| Markdown     | react-markdown + remark-gfm        |
| Ngày giờ     | `Intl.RelativeTimeFormat` (native) |

Không dùng: Next.js, Redux, Zustand, axios, dayjs, i18n, Storybook, shadcn/ui, react-hook-form, zod, shiki.

### Backend

| Thành phần | Chọn                                              |
| ---------- | ------------------------------------------------- |
| Ngôn ngữ   | Go 1.25                                           |
| Router     | Gin                                               |
| ORM        | GORM — **chỉ để query, không dùng `AutoMigrate`** |
| Driver     | pgx v5                                            |
| Migration  | golang-migrate (file SQL)                         |
| WebSocket  | gorilla/websocket                                 |
| Docker     | docker/docker/client                              |
| Auth       | golang-jwt/jwt v5 + golang.org/x/oauth2 + bcrypt  |
| 2FA        | TOTP RFC 6238 tự viết (`internal/auth/adapter/totp`, chỉ stdlib) + rsc.io/qr |
| YAML       | goccy/go-yaml (chấm pipeline mô phỏng)            |
| Ảnh OG     | `image/*` stdlib + golang.org/x/image + x/text (`internal/labs/adapter/rest/og.go`) |
| Log        | log/slog (JSON)                                   |
| Validate   | binding của Gin (go-playground/validator, gián tiếp) |
| Test       | stdlib testing (`go test ./...`)                  |

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
| Local         | Docker Compose + `air` (Go) + `vite dev`           | —          |
| **Prod — be** | Hostinger VPS KVM2, amd64, Singapore + Docker Compose | trả tiền |
| **Prod — fe** | Cloudflare Pages — **hạ tầng riêng** (§9.1.1)      | 0₫         |
| Reverse proxy | nginx (container trong compose)                    | 0₫         |
| TLS           | Cloudflare Origin Certificate — 15 năm, không renew | 0₫        |
| DNS + proxy   | Cloudflare, **mây cam bắt buộc** (§9.5)            | 0₫         |
| Registry      | GHCR — build ở Actions, box chỉ `pull` (§9.2)      | 0₫         |
| CI/CD be      | Actions → GHCR → ssh → `deploy/up.sh`              | 0₫         |
| CI/CD fe      | Actions → `wrangler pages deploy`                  | 0₫         |
| Config server | `deploy/bootstrap.sh` (một máy, một lần, không Ansible) | 0₫    |
| Secrets       | GitHub Secrets (`SSH_*`, `CLOUDFLARE_*`) / env file `chmod 600` trên server | 0₫ |
| Backup        | snapshot Hostinger + `scripts/backup.sh` → Cloudflare R2 | 0₫  |
| Sinh kịch bản sim | OpenRouter (tuỳ chọn, tắt được)                | xem §9.6   |
| Scan secret   | gitleaks (job `secrets` trong cả hai `ci.yml`)     | 0₫         |
| Lint          | gofmt + go vet + lefthook (be), oxlint + tsc (fe)  | —          |
| **Uptime**    | UptimeRobot / BetterStack ping `/readyz` — **ngoài hạ tầng**, xem §9.9 | 0₫ — ⏳ chưa bật |
| **Error tracking** | Sentry (Go + React)                          | 0₫ — ⏳ chưa bật |

**Chưa có: uptime, error tracking, gom log, metric, alert, đo traffic.** Muốn xem log lúc này chỉ có `docker compose logs`. Thứ tự nên làm, theo giá trị trên mỗi phút bỏ ra:

1. **Uptime monitor ngoài** — 5 phút cấu hình, và là thứ duy nhất báo được "máy chết". Xem §9.9.
2. **Sentry** — lỗi runtime tự bay về kèm stacktrace, không phải grep log.
3. **Grafana Cloud + agent Alloy** — metric, log, alert.

**Không tự dựng Prometheus/Grafana/Loki trên máy.** Ba thứ đó ăn ~2 GB RAM và CPU thật; trên một box đang chạy lab container, chúng cạnh tranh tài nguyên với chính thứ chúng giám sát — và khi máy quá tải thì dashboard chết trước. Grafana Cloud free tier (10k series, 50 GB log, alert + contact point Telegram) làm đúng việc đó, 0₫, trên máy chỉ còn một agent. Umami cho traffic và trivy cho scan image cũng nằm ở đây: đã chọn, chưa cài.

**Không dùng Terraform, cũng không dùng Ansible.** Terraform để quản đúng một máy Always Free không bao giờ bị destroy là công cụ lớn hơn việc cần làm; Ansible cho một host chạy một lần cũng vậy — không có inventory, không có drift để hội tụ. `deploy/bootstrap.sh` là bốn mươi dòng apt, chạy một lần lúc dựng máy.

Không dùng: Jaeger/tracing, ELK, Vault, Consul, service mesh, SOPS, Kubernetes.

---

## 2. Kiến trúc

```
Browser (Vite SPA)
   │  REST  /api/*             → auth, courses, labs, history, leaderboard, admin
   │  WS    /ws/terminal/:sid  → PTY container
   │  WS    /ws/chat           → phòng chat chung
   ▼
Cloudflare ──> nginx (TLS) ──> Go API (Gin) ──── Postgres
                                   │
                                   └── docker-socket-proxy ──> Docker Engine
                                                                 └── devforge-lab-<sid>
                                                                     (ephemeral, TTL 20')
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
# ⚠️ Rate limit này HỎNG khi chạy sau proxy — và prod có HAI tầng (Cloudflare
#    rồi nginx). Xem §9.5 trước khi deploy.
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

Traffic web và metrics hệ thống nằm ngoài app (Umami / Grafana Cloud — cả hai chưa bật, xem §1). Dashboard admin chỉ hiển thị số liệu nghiệp vụ: đăng ký khoá X, lab nào tỉ lệ rớt cao, thời gian trung bình hoàn thành.

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

> ⚠️ Cột `ip` trên `audit_logs` và trường `ip` trên màn hình thiết bị đều lấy từ `c.ClientIP()`. Sau proxy, giá trị đó là IP của nginx (hoặc của edge node Cloudflare) chứ không phải của người dùng — xem §9.5, phải sửa ở **cả hai** tầng.

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

Image **build một lần ở GitHub Actions rồi đẩy lên GHCR**, máy prod chỉ `pull`. Cả hai đều amd64 nên không có QEMU ở giữa (§9.2). Hệ quả nằm ở rollback, xem ngay dưới bảng.

> **FE khác API ở hai điểm.** Thứ nhất, Vite **bake `VITE_API_URL` lúc build**, không phải runtime — mỗi môi trường một lần build riêng, không dùng chung artifact. Thứ hai, từ khi tách hạ tầng thì FE **không còn image nào cả**: nó là thư mục `dist` đẩy thẳng lên Cloudflare Pages (§9.1.1).
>
> ⚠️ **`VITE_API_URL` là origin trần, KHÔNG có `/api` ở cuối.** `src/lib/api.ts:46` gọi `fetch(BASE + path)` mà `path` đã chứa sẵn `/api/...`; thêm `/api` vào biến là ra `/api/api/...` và mọi request 404. Bản `docker-compose.prod.yml` hiện tại đang truyền `https://${DOMAIN}/api` — sai, nằm trong danh sách sửa ở §13.

|           | local                          | production                                                              |
| --------- | ------------------------------ | ----------------------------------------------------------------------- |
| Chạy bằng | `docker compose up`            | `docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d` |
| Kiến trúc | amd64                          | amd64 (Hostinger KVM) — xem §9.2                                        |
| Nguồn image | build tại chỗ                | `ghcr.io/<owner>/devforge-api:<sha>`                                    |
| Config    | `.env` (từ `.env.example`)     | env file trên server, `chmod 600`, chủ sở hữu là user chạy compose      |
| DB        | postgres container, seed giả   | postgres volume + snapshot Hostinger + pg_dump cron → R2                |
| TLS       | không                          | nginx + Cloudflare Origin Certificate (§9.5)                            |
| FE        | `vite dev` trên host           | Cloudflare Pages, **hạ tầng riêng** (§9.1.1)                            |
| Log       | stdout                         | stdout → `docker compose logs` (chưa có gom log, §1)                    |
| Deploy    | hot reload (`air`, `vite dev`) | Actions → GHCR → SSH → `compose pull` + `up -d --wait`                  |

**Rollback = đổi tag, không phải build lại.** `IMAGE_TAG=<sha-cũ> docker compose up -d --wait` — vài giây, vì image cũ đã nằm sẵn ở GHCR và trong cache của box. Đây là lãi trực tiếp của việc rời ARM (§9.2); bản Oracle cũ phải build lại 1–2 phút trong lúc bản lỗi vẫn đang phục vụ.

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
| `COOKIE_DOMAIN`                               | (rỗng)                  | **(rỗng)** — host-only trên `api.<domain>`, xem "Domain & routing" | compose |
| `JWT_SECRET`                                  | `.env` giả              | env file trên server   | **secret** |
| `ACCESS_TTL` / `REFRESH_TTL`                  | `15m` / `168h`          | `15m` / `168h`         | compose    |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`   | `.env`                  | env file trên server   | **secret** |
| `GOOGLE_REDIRECT_URL`                         | `http://localhost:8080/api/auth/google/callback` | `https://api.<domain>/api/auth/google/callback` | compose |
| `CORS_ORIGINS`                                | `http://localhost:5173` | `https://<domain>` — **giờ là thứ chặn/mở cả REST lẫn WebSocket**, không còn là dead weight | compose |
| `FRONTEND_URL`                                | `http://localhost:5173` | `https://<domain>` (Pages) | compose |
| `PUBLIC_URL`                                  | `http://localhost:8080` | `https://api.<domain>` — **origin trần, không có `/api`** | compose |
| `UPLOAD_DIR`                                  | `./uploads`             | `./uploads` (bind mount, xem "Domain & routing") | compose |
| `TRUSTED_PROXIES`                             | `127.0.0.1,::1`         | `127.0.0.1,::1,172.16.0.0/12` — **bắt buộc, và chỉ là một nửa: nginx phải khôi phục IP thật từ Cloudflare, xem §9.5** | compose |
| `DATABASE_URL` (migrate)                      | `sslmode=disable`       | `sslmode=require`      | **secret** |
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
| `VITE_API_URL` (FE, **build-time**)           | `http://localhost:8080` | `https://api.<domain>` — **origin trần, KHÔNG có `/api`** | biến của repo FE trên GitHub |
| `GHCR_OWNER`                                  | —                       | chủ package GHCR       | env file   |
| `IMAGE_TAG`                                   | —                       | `<sha>` — CD truyền vào lúc gọi `up.sh` | CD |

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

### Domain & routing — hai origin, không còn path routing

**Hai subdomain của MỘT tên miền.** Đây là ràng buộc cứng, không phải sở thích:

```
https://<domain>/            → SPA trên Cloudflare Pages     (hạ tầng riêng)
https://api.<domain>/        → Go API sau nginx trên VPS     (hạ tầng riêng)
    /api/*      REST
    /ws/*       WebSocket (wss)
    /uploads/*  ảnh bìa + avatar — KHÔNG nằm trong /api
    /healthz    /readyz
```

#### ⚠️ Ràng buộc cookie — đọc trước khi chốt tên miền

Cookie phiên là `SameSite=Lax` (`internal/auth/adapter/rest/cookies.go:42`), `HttpOnly`. Lax **được gửi** khi hai bên cùng một *registrable domain*:

```
✅  devforge.vn        +  api.devforge.vn      → same-site, Lax đi qua, KHÔNG sửa code
❌  devforge.pages.dev +  api.devforge.vn      → cross-site, Lax bị chặn, ĐĂNG NHẬP CHẾT
```

Nghĩa là **phải gắn domain riêng cho Pages**, không dùng `*.pages.dev`. Nếu bắt buộc phải khác tên miền thì phải đổi code sang `SameSite=None; Secure` — và lúc đó Safari cùng các trình chặn tracker bắt đầu ăn cookie. Đường đó đắt hơn nhiều so với việc trỏ một bản ghi CNAME.

`COOKIE_DOMAIN` để **trống**: cookie host-only trên `api.<domain>` là đủ, vì FE không bao giờ đọc nó (`HttpOnly`) — nó chỉ cần được trình duyệt gửi kèm khi gọi API.

#### Code đã sẵn sàng, chỉ đổi env

Tách origin **không cần sửa một dòng code nào**, đây là phần đã kiểm:

| Thứ | Ở đâu | Trạng thái |
| --- | --- | --- |
| `credentials: "include"` trên mọi request | `src/lib/api.ts:51` | ✅ có sẵn |
| CORS phản chiếu origin + `Allow-Credentials: true` | `cmd/server/middleware.go:25` | ✅ có sẵn |
| WebSocket kiểm `Origin` theo `CORS_ORIGINS` | `internal/labs/adapter/rest/terminal.go:41`, `internal/chat/ws.go:322` | ✅ có sẵn |
| URL tuyệt đối + tự đổi `http→ws` | `src/api/labs.ts:112` | ✅ có sẵn |

Cái phải đổi là **giá trị env**, xem bảng trên. `CORS_ORIGINS` từ chỗ gần như thừa trở thành thứ duy nhất chặn/mở cả REST lẫn WebSocket — sai giá trị là app trắng, không phải lỗi 500 để mà grep.

#### `/uploads/*` — cái bẫy vẫn còn, chỉ đổi chỗ

`cmd/server/router.go:32` gắn `r.Static("/uploads", cfg.UploadDir)` lên router gốc, **ngoài** group `/api`, và `internal/upload/image.go:100` dựng URL bằng `PUBLIC_URL + "/uploads/" + name`. Với nginx thì cả hai đều rơi vào `location /` nên không cần route riêng — nhưng `PUBLIC_URL` bây giờ phải là **`https://api.<domain>`**, không phải domain của FE. Đặt nhầm sang domain FE thì mọi ảnh trỏ về Pages, Pages trả `index.html` kèm **200**, và thứ nhìn thấy là ảnh vỡ chứ không phải 404.

Thư mục `UPLOAD_DIR` phải là volume — nằm trong lớp ghi của container thì mỗi lần `up -d` là mất sạch ảnh đã upload.

#### Cái mất khi tách: thẻ `og:` cho trình thu thập

Facebook/X/Slack/Zalo không chạy JavaScript, nên link chia sẻ dán vào đâu cũng hiện ô trắng. Bản một-domain giải quyết bằng khối `@crawler` trong Caddyfile: bot hỏi `/r/:id` hay `/war-room/day/:date` thì rewrite sang endpoint preview của API.

Hai đường dẫn đó là **route của FE**, mà FE giờ nằm ở Pages — không còn proxy nào đứng giữa để rewrite. Phải dựng lại bằng **Pages Function**: đọc `User-Agent`, nếu là bot thì `fetch` sang `https://api.<domain>/api/shared-drills/<id>/preview`, còn lại `next()` để trả file tĩnh. Giữ nguyên hai biểu thức chính quy cũ (`^[A-Za-z0-9_-]+$` cho id, `^\d{4}-\d{2}-\d{2}$` cho ngày) — chúng ở đó để không đẩy rác vào một endpoint sinh ảnh.

#### Hệ quả khi chốt domain

- Google Console: đăng ký redirect `https://api.<domain>/api/auth/google/callback` và set `GOOGLE_REDIRECT_URL` khớp từng ký tự. Luồng OAuth là điều hướng cấp cao nhất nên cookie `Lax` vẫn được set bình thường trên `api.<domain>`.
- `FRONTEND_URL` = domain FE (nơi API redirect về sau callback), `PUBLIC_URL` = domain API. **Ở bản một-domain hai biến này trùng nhau; giờ thì không.** Đây là chỗ dễ chép nhầm nhất.
- DNS: `<domain>` CNAME về Pages, `api.<domain>` A về IP VPS. Cả hai **mây cam** — cert Origin chỉ hợp lệ sau Cloudflare (§9.5).

Không gộp về một domain nữa. Đổi lại là mất path routing, phải trả bằng CORS + một Pages Function; đổi được là hai hạ tầng độc lập thật, deploy bên nào cũng không đụng bên kia.

### Migration tương thích ngược

Code mới phải chạy được với schema cũ. Đổi tên cột `user_name` → `username` làm 3 bước qua 3 lần deploy:

```
Deploy 1:  ADD COLUMN username; backfill; code đọc user_name, ghi CẢ HAI
Deploy 2:  code đọc username, ghi CẢ HAI
Deploy 3:  DROP COLUMN user_name; code chỉ dùng username
```

### CI/CD — hai repo, hai đường, không giao nhau

**Repo `devforge-be` → VPS Hostinger:**

```
push master | pull_request
  job be      → gofmt -l | (! grep .)  →  go vet  →  go build  →  go test
  job secrets → gitleaks (fetch-depth: 0, cần quyền pull-requests: read)

chỉ khi push master và cả hai xanh:
  job image   → docker build --target prod  →  push ghcr.io/<owner>/devforge-api:<sha>
  job deploy  → ssh: git pull --ff-only
                     IMAGE_TAG=<sha> ./deploy/up.sh
                        docker compose pull api
                        build 4 image lab (alpine, vài giây)
                        up -d --wait postgres redis
                        migrate up                    (tương thích ngược)
                        up -d --wait --wait-timeout 120
                     đỏ → IMAGE_TAG=<sha trước> up -d --wait   ← KHÔNG migrate lại
```

**Repo `devforge-fe` → Cloudflare Pages:**

```
push master | pull_request
  job fe      → oxlint  →  tsc --noEmit  →  npm run check (7 file assert)  →  build
  job secrets → gitleaks

chỉ khi push master:
  job deploy  → npm ci && npm run build   với VITE_API_URL từ biến repo
              → wrangler pages deploy dist
```

Không có ssh, không có Docker, không đụng VPS. Hai repo không còn chia sẻ thứ gì ngoài một hợp đồng HTTP.

Bốn điều kiện, và không cái nào là tuỳ chọn:

1. **`/readyz` phải trả đỏ khi API chưa nối được DB** — nếu không thì bước kiểm tra luôn xanh và rollback không bao giờ bắn. Cái này **đã có**: `cmd/server/router.go:74` ping database với timeout 2 giây và trả `503` khi ping hỏng. `/healthz` thì luôn `200` — nó chỉ nói tiến trình còn sống, nên **đừng dùng `/healthz` làm điều kiện rollback.**
2. **Migration chạy trước `up -d`, và phải tương thích ngược.** Rollback chỉ lùi image, không lùi schema — một migration drop cột trong một bước làm binary cũ chạy trên schema nó không đọc được, và lúc đó không còn đường về nào ngoài restore.
3. **Rollback không chạy lại `migrate`.** Bản deploy hỏng có thể đã apply xong một migration; `migrate up` với thư mục `migrations/` cũ sẽ chết vì version đã apply không còn file tương ứng — và chết *trước* khi kịp đưa image cũ trở lại.
4. **Ba secret `SSH_HOST` / `SSH_USER` / `SSH_KEY`** (repo be) và **`CLOUDFLARE_API_TOKEN` / `CLOUDFLARE_ACCOUNT_ID`** (repo fe). Khoá SSH riêng cho deploy, không dùng lại khoá cá nhân.

**Vấn đề khoá deploy tự biến mất.** Bản hiện tại có một lỗ: hai repo cùng deploy lên một box, mà `concurrency` của GitHub Actions tính **theo từng repo** — merge cả hai cùng lúc là hai `up.sh` build đè nhau và chạy `migrate` hai lần, và không có khoá nào chặn. Sau khi tách hạ tầng thì chỉ repo be chạm vào VPS, `concurrency: deploy-prod` trong chính repo đó là đủ, không cần `flock`.

Nhánh mặc định là `master`. `ci.yml` theo dõi `push: [master]` + `pull_request` — trước đó nó theo dõi `main`, nghĩa là mọi lần merge đều vào mà CI không chạy lần nào.

Repo để public thì Actions không giới hạn phút — xem §9.3. **Đặt package GHCR sang Public một lần trong UI**, nếu không box phải `docker login ghcr.io` bằng PAT.

### Staging — nửa đã có sẵn, nửa còn lại phải trả tiền

Lý do cần nằm ngay trong §9.5: hai lỗi mô tả ở đó *"chỉ nổ sau khi deploy (dev FE gọi thẳng `:8080` nên không thấy)"*. Cùng loại với chúng, và cũng chỉ lộ ra sau khi lên máy thật:

`TRUSTED_PROXIES` sau nginx **và** sau Cloudflare · `/uploads/*` với `PUBLIC_URL` · cookie same-site giữa hai subdomain · OAuth redirect URI thật · cert Origin + mây cam · `VITE_API_URL` bake lúc build · migration ba bước tương thích ngược · và chính cái rollback theo tag.

**Nửa FE đã miễn phí và tự động.** Cloudflare Pages tạo **preview deployment cho mỗi pull request**, mỗi cái một URL riêng, không phải dựng gì. Đây là lãi kèm của việc tách hạ tầng — bản một-domain cũ không có.

⚠️ Nhưng preview đó chạy trên `*.pages.dev`, tức là **khác registrable domain với `api.<domain>` → cookie `Lax` không đi qua → không đăng nhập được trên bản preview.** Preview chỉ dùng được để xem giao diện và các trang công khai. Muốn preview đăng nhập được thì phải trỏ preview vào một subdomain riêng (`*.stg.<domain>`), đó là việc phải cấu hình chứ không phải mặc định.

**Nửa BE thì phải trả tiền.** Bản Oracle cũ dựng stack thứ hai trên cùng máy vì có 4 OCPU / 24 GB không dùng hết. KVM2 chỉ có 2 vCPU, mà §9.4 đã chốt trần là CPU: cắt cho staging là cắt thẳng vào sức chứa lab của prod. Ba lựa chọn, không cái nào miễn phí:

| Cách | Giá | Đánh đổi |
| --- | --- | --- |
| Không dựng staging BE | 0₫ | Mọi lỗi ở danh sách trên vẫn chỉ lộ ra ở prod |
| Stack thứ hai trên cùng máy | 0₫ tiền, ~1 GB RAM + phần CPU cắt ra | Phải lên KVM4 trước, nếu không prod chịu |
| VPS thứ hai loại nhỏ nhất | + giá 1 VPS | Sạch nhất, không đụng prod |

**Bẫy phải xử trước khi dựng stack thứ hai trên cùng máy — trần container đếm sai.**

`internal/labs/adapter/repo/session.go:183` đếm ghế từ **database của chính nó**, không hỏi docker daemon:

```sql
SELECT count(*) FROM lab_sessions WHERE status = 'running' AND container_id <> ''
```

Hai stack dùng chung một daemon nhưng hai database riêng → mỗi bên tin rằng nó có đủ ghế, và cùng nhau đẻ gấp đôi số container lên đúng số nhân đó. `MAX_CONTAINERS` là trần trên *một cơ sở dữ liệu*, không phải trên *một cái máy*. Nên tổng hai bên phải bằng con số máy chịu được:

```
prod     MAX_CONTAINERS=10
staging  MAX_CONTAINERS=2
```

Reaper thì không có vấn đề tương tự: `DueForReaping` (`session.go:252`) quét theo `lab_sessions` của chính nó chứ không liệt kê container toàn daemon, nên hai stack không dọn nhầm container của nhau.

Staging cắt bớt cho rẻ:

| | prod | staging |
| --- | --- | --- |
| Postgres | volume riêng + `pg_dump` → R2 | volume riêng, **không backup** |
| Redis | riêng | dùng chung, khác `REDIS_DB` |
| `LAB_SESSION_TTL` | `20m` | `10m` |
| `OPENROUTER_API_KEY` | (rỗng, xem §9.6) | rỗng — đừng đốt tiền AI ở staging |
| Email | Resend / Brevo | Mailpit (đã có sẵn trong compose) |
| Trigger deploy | merge `master` | push `develop` |

Thứ tự: **dựng prod cho xong trước.** Staging tồn tại để diễn tập một pipeline đã có; dựng nó trước là diễn tập cho thứ chưa viết.

---

## 9. Hạ tầng production

> **Đã đổi phương án.** Oracle Always Free / Caddy / một domain là thiết kế cũ và
> **chưa từng dựng thật**. Quyết định hiện tại: **Hostinger VPS + nginx + hai hạ
> tầng tách rời cho be và fe**. Việc phải làm để chuyển sang nằm ở §13.

### 9.0 Toàn cảnh

```
                    Cloudflare   DNS · TLS · WAF · DDoS
                    cache CHỈ cho <domain>, KHÔNG cho api.<domain>  (§9.5)
                          │
        ┌─────────────────┴──────────────────┐
        ▼                                    ▼
   <domain>                            api.<domain>
   Cloudflare Pages                          │  HTTPS 443
   SPA + Pages Function (thẻ og:)            ▼
   ── hạ tầng riêng, §9.1.1 ──      ┌────────────────────────────────────┐
                                    │ VPS Ubuntu — 2 vCPU / 8 GB, SG     │
                                    │                                    │
                                    │  nginx   real_ip CF-Connecting-IP  │
                                    │    │     /ws/: buffering off       │
                                    │    ▼                               │
                                    │  api  (Go, distroless, expose)     │
                                    │    ├── postgres   127.0.0.1 only   │
                                    │    ├── redis      127.0.0.1 only   │
                                    │    └── docker-socket-proxy         │
                                    │          CONTAINERS·POST·EXEC      │
                                    │            └── lab container ×N    │
                                    │                --network=none      │
                                    │                --cap-drop=ALL      │
                                    │  (sau này) Alloy → Grafana Cloud   │
                                    └────────────────────────────────────┘
                                          │                │
                       pg_dump → R2 ──────┘                └──── Sentry
                       + diễn tập restore                        (chưa bật)

   NGOÀI hạ tầng:  UptimeRobot ping /readyz   ← không bao giờ đặt trên chính VPS (§9.9)
```

Luồng deploy:

```
push master
  → CI: lint · test · gitleaks
  → build → ghcr.io/<owner>/devforge-api:<sha>
  → ssh → git pull → compose pull → migrate up → up -d --wait
  → không healthy trong 120s → IMAGE_TAG=<sha trước> up -d --wait
```

Ba thứ phân biệt sơ đồ này với một sơ đồ "VPS + Docker Compose" thông thường, và cả ba đều là ràng buộc của chính sản phẩm chứ không phải sở thích:

1. **`docker-socket-proxy` + lab container.** App tự đẻ container — đó là sản phẩm. Kéo theo trần số container, reaper, và lý do không PaaS nào dùng được.
2. **Không có worker/queue.** Reaper là một goroutine, email gửi đồng bộ. Thêm BullMQ hay một service worker lúc này là thêm tiến trình phải giám sát cho việc chưa tồn tại.
3. **FE không nằm trên VPS.** Mỗi vCPU dành cho FE là một vCPU lấy khỏi lab container — phần duy nhất thật sự cần CPU.

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

#### 9.1.1 Frontend đi hạ tầng riêng — Cloudflare Pages

be và fe là hai repo, và từ đây là **hai hạ tầng không liên quan gì nhau**: deploy
FE không đụng VPS, deploy BE không đụng FE. Kéo theo: service `web` biến mất khỏi
compose, `context: ../devforge-fe` biến mất, và lỗ "hai repo cùng deploy lên một
box, không khoá" (§8, CI/CD) biến mất theo — không cần thêm `flock` vào nữa.

| | **Cloudflare Pages** (chốt) | VPS thứ hai + nginx |
| --- | --- | --- |
| Giá | 0₫ | + giá một VPS nữa |
| Deploy | git push → tự build tự lên | tự viết ssh/rsync |
| TLS | sẵn | certbot hoặc Origin Cert |
| CDN | có, toàn cầu | không |
| Phải bảo trì | ~0 | vá OS, ufw, cert, đĩa |
| Khối `og:` cho bot | cần 1 Pages Function | `map $http_user_agent` trong nginx |

SPA tĩnh không có lý do gì để có máy chủ riêng.

⚠️ **Cái mất khi tách origin: khối `@crawler`.** `deploy/caddy/Caddyfile.prod` đang
rewrite `/r/:id` và `/war-room/day/:date` sang endpoint preview của API để
Facebook/Zalo/Slack đọc được thẻ `og:*` — hai đường dẫn đó là **route của FE**.
Tách hai origin thì Caddy không còn đứng giữa, phải dựng lại bằng **Pages
Function** sniff `User-Agent` rồi `fetch` sang API. Giữ nguyên hai biểu thức chính
quy cũ (`^[A-Za-z0-9_-]+$` cho id chia sẻ, `^\d{4}-\d{2}-\d{2}$` cho ngày), nếu
không là đẩy rác vào một endpoint sinh ảnh.

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

**Image lab vẫn build trên box.** Bốn image `devforge/{linux,git,docker,net}`
không nằm trong compose — API tạo container từ tên image qua Docker API — chúng là
alpine, build vài giây, và gần như không đổi.

Không chỗ nào trong repo ghim kiến trúc (`CGO_ENABLED=0`, Go tĩnh thuần, `apk` tự
phân giải), nên quay lại ARM sau này cũng không phải sửa dòng nào. Bảng đối chiếu
arm64 của bản cũ bỏ đi vì không còn tác dụng.

### 9.3 Bảng chi phí — không còn 0₫

| Khoản | Dịch vụ | Giá |
| --- | --- | --- |
| **Máy chủ** | Hostinger KVM2 Singapore | **trả tiền** — kiểm lúc mua |
| **Tên miền** | | **~300k₫/năm** |
| TLS | Cloudflare Origin Certificate | 0₫ |
| DNS + proxy | Cloudflare Free | 0₫ |
| FE hosting | Cloudflare Pages | 0₫ |
| Registry | GHCR (package public) | 0₫ |
| CI/CD | GitHub Actions (repo public, không giới hạn phút) | 0₫ |
| Backup | Cloudflare R2, 10 GB, egress 0₫ | 0₫ |
| Email | Resend 3000/tháng hoặc Brevo 300/ngày | 0₫ |
| Metrics + log + alert | Grafana Cloud Free | 0₫ — ⏳ chưa bật, §1 |
| Analytics | Umami Cloud | 0₫ — ⏳ chưa bật, §1 |
| Sinh kịch bản sim | OpenRouter | xem §9.6 |

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

Hành vi được ghim bởi `cmd/server/trustedproxies_test.go`. Tên ca kiểm thử còn nói
"caddy", đổi tên là việc trong §13 — nội dung nó ghim vẫn đúng nguyên vẹn với
nginx.

⚠️ **Cache của Cloudflare thuộc về `<domain>`, KHÔNG thuộc về `api.<domain>`.** Mây
cam bật lên là có CDN, và đó là thứ tốt cho file tĩnh của Pages. Trên host API thì
ngược lại:

- Bật "Cache Everything" trên đường API là **phát response có `Set-Cookie` của
  người này cho người khác**. Mặc định Cloudflare không cache response có
  `Set-Cookie`, nên đây là tai nạn do người tự bật rule, không phải mặc định — mà
  đó chính là loại tai nạn khó tin nhất khi nó xảy ra.
- Cache rule sai còn phá được bước nâng cấp lên WebSocket của `/ws/*`.

Quy tắc: page rule / cache rule chỉ đặt trên `<domain>`. Trên `api.<domain>` để
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

1. **Một máy, không HA.** VPS chết là toàn bộ backend chết. FE vẫn sống trên Pages
   nhưng hiện màn hình lỗi vì không gọi được API — nhìn thấy được là điểm cộng duy
   nhất của việc tách origin ở đây.
2. **Backup là chặng ĐẦU TIÊN, không phải chặng cuối.** Snapshot Hostinger phục
   hồi nhanh nhưng nằm cùng nhà cung cấp; `pg_dump` → R2 là thứ sống sót khi mất
   tài khoản. ⚠️ **Cron chưa được cài ở đâu cả** — nó mới chỉ là một dòng comment
   trong `scripts/backup.sh` (lỗi 13.0#7). Và **diễn tập restore là việc phải làm
   bằng tay** — `./scripts/restore.sh`, vốn cũng đang trả sai mã lỗi (13.0#5).
3. **Cloudflare thành điểm chết đơn.** Mây cam tắt (hoặc tài khoản có vấn đề) là
   cert Origin không còn hợp lệ và site đứt. Đường lùi có sẵn: đổi sang certbot +
   Let's Encrypt, xem §9.8.
4. **Giá gia hạn Hostinger.** Giá khuyến mãi chu kỳ đầu và giá gia hạn chênh nhau
   nhiều. Ghi ngày hết hạn vào lịch, đừng để nó là bất ngờ.

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

Dùng dịch vụ ngoài: **UptimeRobot / BetterStack / Cloudflare Health Check**, bản free, ping `https://api.<domain>/readyz` mỗi 5 phút, báo về Telegram.

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

#### 5. 🟡 Một nginx, không phải hai

Sơ đồ tự dựng hay có nginx ở biên (TLS + static) **và** một nginx nữa bên trong làm "proxy cho frontend/backend". Hai tầng trên cùng một máy không thêm gì ngoài một hop, một file config nữa phải đồng bộ, và một chỗ nữa để `X-Forwarded-For` bị đứt. Ở đây chỉ có một, và nó là service `nginx` trong compose.

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
| **P10**  | Backup pg_dump → R2 + diễn tập restore                                      | ⚠️ có script — nhưng **cron chưa ai cài** và `restore.sh` sai mã lỗi, xem §13.0 |
| **P3.5** | Observability: metrics Go + Grafana Alloy → Grafana Cloud + alert           | ❌ |
| **P9**   | Deploy prod bản Oracle/Caddy: compose prod, CD qua SSH, `TRUSTED_PROXIES`   | ✅ trên đĩa — nhưng **đổi phương án**, xem §13 |
| **P9.5** | Chuyển sang Hostinger + nginx + tách hạ tầng fe/be                          | ❌ — danh sách việc ở §13 |
| **P11**  | _(tuỳ chọn)_ tách runner node riêng, gVisor, hoặc chuyển k3s                | — |

**Thứ tự đảo lại so với bản trước: P10 → P3.5 → P9.** Trên hạ tầng free không có SLA, backup là thứ duy nhất còn lại khi mọi thứ khác biến mất — làm nó trước khi có gì để mất. Rồi đến quan sát được, rồi mới tới tự động deploy.

Ngoài lộ trình, đã làm thêm: bài mô phỏng CI/CD (`SIM-CICD.md`), mô phỏng Linux / tìm kiếm / sắp xếp, War Room (lab sự cố có hạn giờ, link chia sẻ công khai, ca trực ngày, chuỗi ngày, bảng tuần), bảng `system_events` và bốn màn quản trị đi kèm, tin nhắn riêng trong chat, sổ tay ôn tập.

**Ước lượng còn lại: 5–7 ngày công**, trong đó §13 (chuyển hạ tầng) chiếm 1–1,5 ngày.

Metrics tối thiểu ở P3.5:

```
devforge_lab_sessions_active          gauge
devforge_lab_containers_running       gauge   # so với gauge trên → phát hiện rò rỉ
devforge_reaper_last_run_timestamp    gauge   # alert nếu > 5 phút
devforge_reaper_killed_total          counter
devforge_lab_start_duration_seconds   histogram
```

Alert quan trọng nhất: `lab_containers_running > lab_sessions_active` kéo dài → có container mồ côi. Trên máy 2 nhân, container mồ côi ăn ghế cho tới khi hết — càng ít nhân thì alert này càng đáng giá.

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
│   │   ├── nginx/              # devforge.conf + certs/ (Origin Cert, không commit)
│   │   ├── bootstrap.sh        # dựng máy 1 lần — thay Ansible
│   │   ├── up.sh               # pull → migrate → up --wait → rollback theo tag
│   │   └── DEPLOY.md           # runbook triển khai
│   ├── .github/workflows/ci.yml   # gofmt/vet/build/test + gitleaks
│   ├── Dockerfile              # build → prod (distroless)
│   ├── docker-compose.yml      # postgres + redis + mailpit + docker-proxy (dev)
│   ├── docker-compose.prod.yml # + api (từ GHCR), nginx — mailpit tắt bằng profile
│   ├── .air.toml · Makefile · lefthook.yml · .env.example
│
└── devforge-fe/                # REPO 2 — Vite + React (chạy standalone)
    ├── src/{pages,components,api,hooks,context,lib,sims}
    │   ├── pages/admin/
    │   └── lib/*.check.ts      # assert của node, không framework
    ├── functions/              # Pages Function: thẻ og: cho trình thu thập
    ├── public/_headers · public/_redirects   # cache + fallback SPA của Pages
    ├── .github/workflows/ci.yml   # oxlint/tsc/check/build + gitleaks + deploy Pages
    ├── vite.config.ts · lefthook.yml · .env.example
```

`deploy/monitoring/`, `deploy/terraform/` và `deploy/ansible/` đều đã bỏ, xem §1. Trên máy hiện không có agent quan sát nào — khi có sẽ là một service Alloy trong compose, không phải Prometheus tự dựng.

**Vì sao polyrepo:** be/fe tách sạch theo ranh giới repo — CI riêng, deploy riêng, phân quyền riêng. Từ §9.1.1 thì tách cả **hạ tầng**: `be` sở hữu VPS (compose, DB, nginx, deploy), `fe` sở hữu project Pages. Hai bên không còn chia sẻ thứ gì ngoài một hợp đồng HTTP và một tên miền gốc.

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
`node --experimental-strip-types`, không framework. Bảy file này **đã nằm trong CI** của FE
(`lint → tsc → check → build`).

`make check-seed` và `make check-sim` cũng ngoài CI — chúng cần docker và database, chấp nhận được,
nhưng phải chạy tay khi sửa nội dung seed.

> Compose đặt tên `devforge-be`, tránh trùng volume/container project khác trên máy.
> Dev không có reverse proxy nào — FE gọi thẳng API. Edge (`deploy/nginx`) chỉ tồn tại ở **prod**,
> và đó là lý do hai lỗi ở §9.5 không bao giờ lộ ra lúc dev.

---

## 13. Việc cần làm — chuyển sang Hostinger + nginx + tách hạ tầng

Đây là danh sách quyết định. Mọi thứ ở §8 và §9 mô tả **đích**; phần lớn file trên đĩa vẫn đang là bản Oracle/Caddy. Dưới đây là khoảng cách giữa hai bên.

### 13.0 Lỗi đang có sẵn trên đĩa

Chưa cái nào được sửa. Hai cái đầu làm prod trắng ngay lần deploy đầu, và cả hai độc lập với việc đổi hạ tầng — sửa hay không sửa trước khi chuyển sang Hostinger đều phải sửa.

| # | Chỗ | Lỗi | Sửa | Còn đúng sau §13? |
| --- | --- | --- | --- | --- |
| 1 | 🔴 `devforge-fe/Dockerfile` không có dòng `ARG VITE_API_URL` | Docker **bỏ qua build-arg không khai báo**, Vite không thấy biến, bundle rơi về `http://localhost:8080` → trang https gọi http localhost → chặn mixed-content, app chết | `ARG VITE_API_URL` + `ENV VITE_API_URL=$VITE_API_URL` trong stage `build` | Không — file bị xoá ở 13.3, biến chuyển sang biến repo của Pages |
| 2 | 🔴 `docker-compose.prod.yml` truyền `VITE_API_URL: https://${DOMAIN}/api` | `src/lib/api.ts:46` gọi `fetch(BASE + path)` mà `path` đã có sẵn `/api/...` → mọi request thành `/api/api/...` → 404 | Bỏ `/api` ở cuối | Có — giá trị mới là `https://api.<domain>`, vẫn phải là origin trần |
| 3 | 🟠 `deploy/up.sh:71` rollback dùng `git checkout <sha>` | Clone ở lại **detached HEAD**; lần deploy sau mở đầu bằng `git pull --ff-only` → từ chối chạy → CD hỏng vĩnh viễn tới khi có người ssh vào | `git reset --hard` | Không — `up.sh` viết lại ở 13.2, rollback đổi sang `IMAGE_TAG` |
| 4 | 🟠 `deploy/up.sh` rollback chạy lại `migrate up` | Bản hỏng có thể đã apply một migration; `migrate up` với `migrations/` cũ chết vì version đã apply không còn file → rollback đứt giữa chừng | Bỏ qua migrate trên nhánh rollback | Không — cùng lý do trên |
| 5 | 🟠 `scripts/restore.sh` dòng cuối `[ ... ] && echo` | Nhánh `--into-live` cho biểu thức false → **script exit 1 dù restore thành công**, đúng lúc đang có sự cố | Đổi thành `if ... fi` | **Có** — file này không đổi trong §13 |
| 6 | 🟠 Không repo nào có `.dockerignore` | `COPY . .` nuốt cả `.env` prod (chmod 600) vào layer cache của builder trên box | Thêm `.dockerignore`: `.env* .git dist node_modules uploads tmp` | **Có** cho be (image vẫn build từ context này ở Actions); không cho fe |
| 7 | 🟠 Cron backup chỉ nằm trong comment của `scripts/backup.sh`, không ai cài | Làm đúng quy trình dựng máy vẫn ra **không có backup nào** | Cài `/etc/cron.d/devforge-backup` trong `bootstrap.sh` | **Có** |
| 8 | 🟡 `.env.prod.example` dòng 3 nói "CI writes it from GitHub Secrets on every deploy" | Ngược với README §8 và với `ci.yml` — CD không đẩy secret xuống bao giờ | Xoá câu đó | **Có** |
| 9 | 🟡 `.env.prod.example` để `DB_PASSWORD` hai chỗ (biến riêng + nhúng trong `DATABASE_URL`) | Điền một chỗ quên chỗ kia → `migrate` chết trên máy không ai nhìn | `DATABASE_URL=postgres://${DB_USER}:${DB_PASSWORD}@...` — API không đọc biến này nên nội suy an toàn | **Có** |
| 10 | 🟡 `ssh` trong job deploy không có timeout | Box treo là giữ runner tới 6 tiếng | `timeout 1800 ssh -o ConnectTimeout=10 -o BatchMode=yes` | **Có** |

### 13.1 Việc phải làm bằng tay (cần đăng nhập, không script hoá được)

| | Việc | Ghi chú |
| --- | --- | --- |
| ☐ | Mua VPS Hostinger **KVM2, vùng Singapore**, template Ubuntu 24.04 **có sẵn Docker** | Kiểm giá gia hạn, không chỉ giá khuyến mãi |
| ☐ | Mở **80 và 443 trong tường lửa hPanel** | Lớp cửa thứ nhất — `ufw` là lớp thứ hai, quên lớp nào cũng không vào được (§9.1) |
| ☐ | Bật **snapshot/backup tuần** của Hostinger | Không thay R2, xem §9.7 |
| ☐ | DNS Cloudflare: `<domain>` CNAME → Pages, `api.<domain>` A → IP VPS | **Cả hai mây cam** |
| ☐ | Tạo **Cloudflare Origin Certificate** cho `<domain>` + `*.<domain>` | Lưu vào `deploy/nginx/certs/`, key `chmod 600` |
| ☐ | Tạo project **Cloudflare Pages**, gắn **domain riêng** | ❗Không dùng `*.pages.dev` — cookie sẽ chết, xem §8 "Ràng buộc cookie" |
| ☐ | Đặt **package GHCR sang Public** | Không thì box phải `docker login` bằng PAT (§9.2) |
| ☐ | Google Console: đổi redirect URI sang `https://api.<domain>/api/auth/google/callback` | Khớp từng ký tự |
| ☐ | Tạo bucket **R2** + lifecycle xoá sau 30 ngày | Nếu chưa có |
| ☐ | Secrets repo **be**: `SSH_HOST`, `SSH_USER`, `SSH_KEY` | |
| ☐ | Secrets repo **fe**: `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`; biến `VITE_API_URL` | |
| ☐ | **Uptime monitor ngoài** ping `https://api.<domain>/readyz`, báo Telegram | 5 phút, và là thứ duy nhất báo được "máy chết" (§9.9). Đừng cài trên chính VPS |
| ☐ | Kiểm cache rule Cloudflare **không phủ `api.<domain>`** | Chỉ bật WAF, không bật cache trên host API (§9.5) |
| ☐ | Sentry cho Go + React | Lỗi runtime kèm stacktrace, rẻ hơn nhiều so với dựng cả stack quan sát |

### 13.2 Repo `devforge-be` — file phải sửa

| | File | Việc |
| --- | --- | --- |
| ☐ | `deploy/nginx/devforge.conf` | **Mới.** Một site duy nhất (`server_name _`, box chỉ phục vụ API): 80 → 301 sang 443; cert Origin; `set_real_ip_from` các dải Cloudflare + `real_ip_header CF-Connecting-IP`; `location /ws/` với `proxy_buffering off` + timeout 3600s; `location /`; `client_max_body_size` cho upload ảnh. ⚠️ Upstream phải đi qua biến + `resolver 127.0.0.11` — nginx cache DNS vĩnh viễn, container `api` mới sau mỗi deploy sẽ nhận 502 nếu không |
| ☐ | `deploy/nginx/certs/.gitignore` | **Mới.** `*` + `!.gitignore` — khoá riêng không bao giờ vào git |
| ☐ | `deploy/caddy/` | Xoá cả thư mục |
| ☐ | `docker-compose.prod.yml` | Bỏ service `web` và `caddy` (+ volume `caddy_data`, `caddy_config`); thêm `nginx`; `api` đổi từ `build:` sang `image: ghcr.io/${GHCR_OWNER}/devforge-api:${IMAGE_TAG}`; bỏ `context: ../devforge-fe` |
| ☐ | `deploy/up.sh` | Bỏ toàn bộ phần build api/web; `pull` thay cho `build`; `up -d --wait` thay cho hàm `healthy()` tự viết; rollback đổi từ `git checkout <sha>` (lỗi 13.0#3) sang `IMAGE_TAG=<tag trước> up -d --wait`, và **không chạy lại migrate** (lỗi 13.0#4). ~90 dòng → ~35 |
| ☐ | `deploy/bootstrap.sh` | Bỏ phần cài Docker nếu dùng template có sẵn; bỏ dòng hướng dẫn clone repo fe; ghi chú tường lửa hPanel; swap 4 GB → 2 GB (không còn build trên box) |
| ☐ | `.env.prod.example` | `PUBLIC_URL`/`FRONTEND_URL`/`CORS_ORIGINS`/`GOOGLE_REDIRECT_URL` theo bảng §8; thêm `GHCR_OWNER`; bỏ `ACME_EMAIL` (không còn Caddy); `MAX_CONTAINERS=12`; `LAB_SESSION_TTL=20m` |
| ☐ | `.github/workflows/ci.yml` | Thêm job `image` (build + push GHCR, `permissions: packages: write`); job `deploy` truyền `IMAGE_TAG=<sha>` |
| ☐ | `deploy/DEPLOY.md` | Viết lại toàn bộ — đang là runbook Oracle |
| ☐ | Comment trong code | `cmd/server/router.go:53`, `internal/config/config.go:32`, `internal/labs/adapter/rest/preview.go:25` còn nói "Caddy"; `cmd/server/trustedproxies_test.go` còn tên ca `behindCaddy`. Chỉ là chữ, không đổi hành vi — đổi tên cho khỏi lạc hướng người đọc sau |

### 13.3 Repo `devforge-fe` — file phải sửa

| | File | Việc |
| --- | --- | --- |
| ☐ | `.github/workflows/ci.yml` | Bỏ job deploy qua ssh; thay bằng `npm run build` (với `VITE_API_URL` từ biến repo) + `wrangler pages deploy dist`. ⚠️ Thêm một bước **fail sớm nếu `VITE_API_URL` rỗng** — đây đúng là lớp lỗi vừa mắc ở 13.0 |
| ☐ | `functions/r/[id].js` | **Mới.** Pages Function: `User-Agent` khớp danh sách bot → `fetch(API + /api/shared-drills/<id>/preview)`, còn lại `next()`. Giữ regex `^[A-Za-z0-9_-]+$` |
| ☐ | `functions/war-room/day/[date].js` | **Mới.** Như trên với `/api/daily-drill/<date>/preview`, regex `^\d{4}-\d{2}-\d{2}$` |
| ☐ | `public/_redirects` | `/* /index.html 200` — fallback SPA của Pages |
| ☐ | `public/_headers` | `assets/*` cache 1 năm immutable, `index.html` no-cache (thay `Caddyfile.static`) |
| ☐ | `Dockerfile`, `Caddyfile.static`, `.dockerignore` | Xoá — FE không còn image nào |

### 13.4 Thứ tự

```
1. Mua máy + DNS + Origin Cert          (13.1, phần hạ tầng)
2. Sửa repo be                          (13.2)         ← chưa deploy được nếu thiếu bước 1
3. bootstrap.sh trên máy mới + .env + up.sh chạy tay lần đầu
4. R2 + chạy backup tay + DIỄN TẬP RESTORE   ← đừng để sau
5. Sửa repo fe + tạo Pages + gắn domain (13.3)
6. Bật CD cả hai repo, đẩy thử một commit vô hại
7. Diễn tập rollback bằng tay: IMAGE_TAG=<sha cũ> up -d --wait
```

Bước 4 và bước 7 là hai bước hay bị bỏ nhất và cũng là hai bước duy nhất chứng minh được lưới an toàn có thật.

### 13.5 Kiểm chứng sau khi lên

```bash
curl -sf https://api.<domain>/healthz          # tiến trình sống
curl -sf https://api.<domain>/readyz           # + nối được DB
curl -sI https://<domain>/ | grep -i cf-       # FE đi qua Cloudflare
```

Trên trình duyệt, ba thứ chỉ hỏng ở prod nên phải nhìn tận mắt:

- **Đăng nhập rồi F5** — còn đăng nhập nghĩa là cookie same-site đúng.
- **Mở một lab, gõ vài phím** — không khựng nghĩa là `proxy_buffering off` đúng.
- **Xem một ảnh bìa** — hiện được nghĩa là `PUBLIC_URL` trỏ đúng domain API.
- **Dán một link `/r/<id>` vào Zalo/Slack** — ra thẻ xem trước nghĩa là Pages Function chạy.
- **Xem `audit_logs.ip` của lần đăng nhập vừa rồi** — ra IP thật của bạn, không phải IP Cloudflare, nghĩa là §9.5 đã đúng cả hai tầng.

### 13.6 Nợ lại có chủ ý

| Việc | Vì sao chưa làm |
| --- | --- |
| Gom log / metric / alert (P3.5) | Chưa chặn việc lên prod. Làm ngay sau khi có người dùng thật |
| ~~Uptime monitor ngoài~~ | Đã chuyển lên 13.1 — quá rẻ để xếp vào nợ |
| ~~Sentry~~ | Đã chuyển lên 13.1, cùng lý do |
| trivy quét image | Image build từ base chính thức, gitleaks đã chặn phần rủi ro lớn hơn |
| Staging BE | Xem §8 "Staging" — trên KVM2 là cắt vào sức chứa prod |
| Bỏ Redis (dồn session vào Postgres) | Đang chạy, 0 cấu hình. Lãi một container, không đáng ưu tiên |
