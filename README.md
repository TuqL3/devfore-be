# DevForge

Nền tảng học DevOps qua lab thực hành. Mỗi bài lab cấp cho học viên một **container Linux thật**, truy cập qua terminal trong trình duyệt, giới hạn 60 phút.

Production chạy trên hai VPS trả tiền (prod + staging) cộng một tên miền; mọi thứ còn lại 0₫ — xem [§9 Hạ tầng production](INFRA.md#9-hạ-tầng-production).

---

> **§8, §9 và §13 nằm ở [INFRA.md](INFRA.md).** Số mục giữ nguyên khi tách, nên
> mọi `§9.5`, `§13.2`… trong file này và trong comment của code vẫn trỏ đúng chỗ —
> chỉ là chúng nằm ở file bên cạnh. File này giữ §0–§7 và §10–§12.

## 0. Tóm tắt

```
Vite/React/TS/Tailwind ─HTTP+WS→ Go/Gin/GORM ─→ Postgres
                                      │
                             socket-proxy → Docker → lab container (hardened, TTL 20')

be:  Cloudflare(DNS+TLS) → nginx → Docker Compose → Hostinger VPS (amd64, Singapore)
fe:  cùng VPS, cùng origin — image devforge-web sau chính cái biên đó

Actions → GHCR (cả hai repo) → tag v* promote → ssh (chỉ repo be) → up.sh pull
```

Ba khối chức năng:

1. **Học viên** — landing, danh sách khoá học, chi tiết khoá học (4 tab), lab + terminal thật, chấm điểm, lịch sử, bảng xếp hạng, chat chung, bài mô phỏng, War Room
2. **Admin** — CRUD khoá học/lab/task, chạy thử `check_script`, ban user, kill session đang chạy, audit log, bảng sự kiện hệ thống
3. **Hạ tầng** — 2 môi trường local/production, CI/CD, cấu hình server, observability, backup + diễn tập restore

Đã chốt hạ tầng: **Hostinger VPS KVM2 (2 vCPU amd64 / 8 GB, Singapore)**, **nginx + Cloudflare Origin Certificate** ở biên, FE và API **cùng một origin** trên máy đó (§9.1.1 — quyết định Cloudflare Pages đã bị đảo, lý do là artifact chứ không phải giá). Hai repo, hai image, một đường deploy. Chi phí cố định = VPS + tên miền; xem §9.

**Phương án Oracle Always Free / Caddy là bản cũ và chưa từng dựng thật.** Khoảng cách giữa file trên đĩa và bản đã chốt: **§13**.

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
| **Prod — be** | Hostinger VPS KVM2, amd64, Singapore + Docker Compose | trả tiền   |
| **Prod — fe** | Cùng VPS, cùng origin — image riêng, xem §9.1.1    | 0₫         |
| Reverse proxy | nginx (container trong compose)                    | 0₫         |
| TLS           | Cloudflare Origin Certificate — 15 năm, không renew | 0₫        |
| DNS + proxy   | Cloudflare, **mây cam bắt buộc** (§9.5)            | 0₫         |
| Registry      | GHCR — build ở Actions, box chỉ `pull` (§9.2)      | 0₫         |
| CI/CD be      | Actions → GHCR → ssh (staging ở master, prod ở tag) | 0₫        |
| CI/CD fe      | Actions → GHCR (build ở master, promote ở tag)     | 0₫         |
| Staging       | VPS thứ hai, cùng compose khác `.env` (§8)          | trả tiền   |
| Config server | `deploy/bootstrap.sh` (một lần mỗi máy, không Ansible) | 0₫       |
| Secrets       | GitHub Secrets (`SSH_*`, `STAGING_SSH_*`) / env file `chmod 600` trên server | 0₫ |
| Backup        | snapshot Hostinger + `scripts/backup.sh` → Cloudflare R2 | 0₫  |
| Sinh kịch bản sim | OpenRouter (tuỳ chọn, tắt được)                | xem §9.6   |
| Scan secret   | gitleaks (job `secrets` trong cả hai `ci.yml`)     | 0₫         |
| Scan lỗ hổng  | Trivy — image + `fs`, chặn HIGH/CRITICAL có bản vá | 0₫         |
| Lint          | golangci-lint + lefthook (be), oxlint + tsc (fe)   | —          |
| Coverage      | `go test -coverprofile`, sàn 16.5% trong CI        | —          |
| **Uptime**    | UptimeRobot / BetterStack ping `/readyz` — **ngoài hạ tầng**, xem §9.9 | 0₫ — ⏳ chưa bật |
| **Error tracking** | Sentry (Go + React)                          | 0₫ — ⏳ chưa bật |

**Chưa có: uptime, error tracking, gom log, metric, alert, đo traffic.** Muốn xem log lúc này chỉ có `docker compose logs`. Thứ tự nên làm, theo giá trị trên mỗi phút bỏ ra:

1. **Uptime monitor ngoài** — 5 phút cấu hình, và là thứ duy nhất báo được "máy chết". Xem §9.9.
2. **Sentry** — lỗi runtime tự bay về kèm stacktrace, không phải grep log.
3. **Grafana Cloud + agent Alloy** — metric, log, alert.

**Không tự dựng Prometheus/Grafana/Loki trên máy.** Ba thứ đó ăn ~2 GB RAM và CPU thật; trên một box đang chạy lab container, chúng cạnh tranh tài nguyên với chính thứ chúng giám sát — và khi máy quá tải thì dashboard chết trước. Grafana Cloud free tier (10k series, 50 GB log, alert + contact point Telegram) làm đúng việc đó, 0₫, trên máy chỉ còn một agent. Umami cho traffic nằm cùng nhóm đó: đã chọn, chưa cài. Trivy thì không — nó chạy **trong Actions**, không phải trên box, nên không tranh tài nguyên với gì cả; xem §8 "Quét bảo mật và chất lượng mã". Cùng lý do đó mà SonarQube tự dựng bị loại thẳng: nó là thứ duy nhất trong danh sách đòi chỗ trên chính cái máy đang bán ghế lab.

**Không dùng Terraform, cũng không dùng Ansible.** Terraform để quản hai máy không bao giờ bị destroy là công cụ lớn hơn việc cần làm; Ansible cho hai host chạy một lần cũng vậy — không có inventory, không có drift để hội tụ. `deploy/bootstrap.sh` là bốn mươi dòng apt, chạy một lần trên mỗi máy.

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

### Admin — ràng buộc

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

### Sandbox lab — bề mặt tấn công

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

## 8, 9, 13. Hạ tầng & triển khai → [INFRA.md](INFRA.md)

Môi trường, biến cấu hình, routing, migration, CI/CD, quét bảo mật, staging, máy
chủ, chi phí, sức chứa, rủi ro, và danh sách việc còn phải làm — tất cả ở
[INFRA.md](INFRA.md), giữ nguyên số mục §8, §9 và §13.

Ba thứ hay cần nhất:

| Cần gì | Ở đâu |
| --- | --- |
| Cắt một bản phát hành | `make release v=v1.2.0` — [INFRA §8, CI/CD](INFRA.md#cicd--hai-repo-gặp-nhau-ở-registry) |
| Lùi bản đang chạy | `IMAGE_TAG=<tag cũ> … up -d --wait` — [deploy/DEPLOY.md](deploy/DEPLOY.md) |
| Việc còn phải làm để lên máy thật | [INFRA §13](INFRA.md#13-việc-cần-làm--mua-máy-caddy--nginx) |


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
| **P10**  | Backup pg_dump → R2 + diễn tập restore                                      | ⚠️ script + cron đã xong (13.0 #5, #7) — **diễn tập restore vẫn chưa ai chạy** |
| **P3.5** | Observability: metrics Go + Grafana Alloy → Grafana Cloud + alert           | ❌ |
| **P9**   | Deploy prod: compose prod, CD qua SSH, `TRUSTED_PROXIES`                    | ✅ pipeline xong (GHCR, Trivy, promote, staging) — **chưa có máy để chạy** |
| **P9.5** | Mua 2 VPS (prod + staging), Caddy → nginx, bootstrap, DNS                    | ❌ — danh sách việc ở §13 |
| **P11**  | _(tuỳ chọn)_ tách runner node riêng, gVisor, hoặc chuyển k3s                | — |

**Thứ tự: P10 → P3.5 → P9.** Backup là thứ duy nhất còn lại khi mọi thứ khác biến mất — làm nó trước khi có gì để mất. Rồi đến quan sát được, rồi mới tới tự động deploy.

Ngoài lộ trình, đã làm thêm: bài mô phỏng CI/CD (`SIM-CICD.md`), mô phỏng Linux / tìm kiếm / sắp xếp, War Room (lab sự cố có hạn giờ, link chia sẻ công khai, ca trực ngày, chuỗi ngày, bảng tuần), bảng `system_events` và bốn màn quản trị đi kèm, tin nhắn riêng trong chat, sổ tay ôn tập.

**Ước lượng còn lại: 5–7 ngày công.** §13 giờ là hai máy chứ không phải một, nên phần hạ tầng nhích lên ~2 ngày.

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
│   ├── .github/workflows/ci.yml   # lint/test/coverage + gitleaks + trivy + GHCR + deploy
│   ├── Dockerfile              # build → prod (distroless)
│   ├── docker-compose.yml      # postgres + redis + mailpit + docker-proxy (dev)
│   ├── docker-compose.prod.yml # api + web từ GHCR, nginx — mailpit tắt bằng profile
│   ├── .golangci.yml · .trivyignore · .dockerignore
│   ├── .air.toml · Makefile · lefthook.yml · .env.example
│
└── devforge-fe/                # REPO 2 — Vite + React (chạy standalone)
    ├── src/{pages,components,api,hooks,context,lib,sims}
    │   ├── pages/admin/
    │   └── lib/*.check.ts      # assert của node, không framework
    ├── Dockerfile · Caddyfile.static   # image devforge-web, VITE_API_URL ghim rỗng
    ├── .github/workflows/ci.yml   # oxlint/tsc/check/build + gitleaks + trivy + GHCR
    ├── vite.config.ts · lefthook.yml · .env.example
```

`deploy/monitoring/`, `deploy/terraform/` và `deploy/ansible/` đều đã bỏ, xem §1. Trên máy hiện không có agent quan sát nào — khi có sẽ là một service Alloy trong compose, không phải Prometheus tự dựng.

**Vì sao polyrepo:** be/fe tách sạch theo ranh giới repo — CI riêng, phân quyền riêng, mỗi bên tự build và tự promote image của mình. Chỗ duy nhất hai bên gặp nhau là GHCR: repo be sở hữu cả hai VPS và là đường deploy duy nhất, repo fe chỉ đẩy image. Cái giá là một quy ước — cùng một tên tag trên cả hai repo — và §8 mô tả cái cổng chặn khi ai đó quên.

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
gofmt -l . && golangci-lint run ./... && go test ./...
make cover                    # sàn coverage của CI là 16.5%, cần `make up` trước
make check-seed               # cần docker: mọi check_script phải TRƯỢT trên container mới,
                              # rồi phải ĐẬU sau lời giải trong scripts/seed-solutions.tsv
make check-sim                # cần DB, không cần docker: pipeline sai phải trượt, đúng phải đậu

# devforge-fe
npx tsc --noEmit && npx oxlint && npm run check
```

`npm run check` chạy 8 file assert (`json`, `clock`, `mdSummary`, `sim`, `wsRetry`, `linux`,
`search`, `sort`) bằng `node --experimental-strip-types`, không framework. Cả tám **đã nằm
trong CI** của FE (`lint → tsc → check → build`).

`make check-seed` và `make check-sim` cũng ngoài CI — chúng cần docker và database, chấp nhận được,
nhưng phải chạy tay khi sửa nội dung seed.

> Compose đặt tên `devforge-be`, tránh trùng volume/container project khác trên máy.
> Dev không có reverse proxy nào — FE gọi thẳng API. Edge (`deploy/nginx`) chỉ tồn tại ở **prod**,
> và đó là lý do hai lỗi ở §9.5 không bao giờ lộ ra lúc dev.
