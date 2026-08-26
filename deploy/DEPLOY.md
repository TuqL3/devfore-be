# Triển khai production — việc cần làm

Mục tiêu: **0₫/tháng**, trừ tên miền. Hạ tầng theo README §9.

Trạng thái: bạn **đã có tên miền**, **chưa có máy chủ**. Ảnh build **trên chính máy đó**,
không qua GHCR, không QEMU.

Ba việc cần bạn tự làm vì phải đăng nhập: tạo máy Oracle, trỏ DNS, tạo bucket R2.
Phần còn lại đã thành script trong repo này.

---

## 0. Tóm tắt thứ tự

```
A. Oracle A1        → có IP
B. DNS A record     → domain trỏ về IP        ← PHẢI xong trước bước E
C. bootstrap.sh     → docker, ufw, user, swap
D. .env             → điền secret, chmod 600
E. up.sh            → build, migrate, up, Caddy xin cert
F. tài khoản admin
G. backup R2 + cron + diễn tập restore        ← đừng để sau
H. GitHub Secrets   → CD tự chạy từ lần sau
```

Ước lượng: 2–3 giờ, phần lớn là chờ Oracle cấp máy.

---

## A. Máy chủ Oracle Always Free

1. Tạo tài khoản tại `cloud.oracle.com`, chọn **home region = Singapore**
   (đổi region sau là không được). Singapore → VN 30–50ms; terminal lab là
   WebSocket mỗi phím một vòng round-trip nên đây là ràng buộc cứng, không phải
   sở thích.
2. **Nâng ngay lên Pay As You Go.** Vẫn 0₫ khi ở trong hạn mức free, nhưng
   được miễn cơ chế thu hồi tài nguyên máy để rảnh (README §9.7).
3. Tạo instance:
   - Shape: **VM.Standard.A1.Flex**, **4 OCPU / 24 GB** (toàn bộ hạn mức free)
   - Image: **Ubuntu 24.04 (aarch64)**
   - Boot volume: 100–200 GB
   - Dán SSH public key của bạn
4. **"Out of host capacity"** là chuyện thường ở Singapore. Cứ thử lại; nếu vài
   ngày không được thì viết script gọi API tạo máy lặp lại, hoặc đổi sang
   Osaka/Tokyo (README §9.8 có phương án dự phòng trả phí).
5. Trong **VCN → Security List**, mở ingress **80** và **443** từ `0.0.0.0/0`.
   `ufw` ở bước C là cửa thứ hai, không phải cửa đầu tiên — quên bước này thì
   máy không ai vào được dù ufw nói gì.

Xong bước này bạn có `<IP>`.

## B. DNS

Cloudflare Free (hoặc chỗ nào đang giữ domain):

```
A    @    <IP>    DNS only (mây XÁM)
```

**Giữ mây xám.** Bật proxy cam là thêm một tầng vào đúng bài toán
`TRUSTED_PROXIES` ở §9.5, và Caddy phải xin cert xuyên qua tầng đó. Ẩn IP gốc
là điểm cộng, để sau.

Đợi DNS lan xong (`dig +short <domain>` ra đúng IP) **trước** bước E: Caddy xin
Let's Encrypt qua HTTP-01, challenge trượt sẽ retry vào rate limit 5 lần/tuần.

## C. Dựng máy

```bash
scp deploy/bootstrap.sh ubuntu@<IP>:/tmp/
ssh ubuntu@<IP> 'sudo bash /tmp/bootstrap.sh'
```

Script làm: docker engine + compose plugin, giới hạn log 10m×3 (một lab chạy
loạn không được phép ăn hết 200 GB đĩa), user `devforge` trong nhóm docker,
ufw 22/80/443, tắt đăng nhập mật khẩu SSH, 4 GB swap (build Go + Vite chạy
ngay trên máy này, build OOM giết Postgres là kết cục tệ hơn build chậm),
unattended security upgrades.

Không dùng Ansible: một host, chạy một lần, không có inventory và không có
drift để hội tụ. Playbook ở đây là một dependency và một ngôn ngữ thứ hai cho
bốn mươi dòng `apt`.

Rồi clone hai repo:

```bash
ssh devforge@<IP>
git clone <be repo> /opt/devforge/devforge-be
git clone <fe repo> /opt/devforge/devforge-fe
```

Hai repo phải **cạnh nhau** đúng tên đó: `docker-compose.prod.yml` build FE với
`context: ../devforge-fe`.

## D. File môi trường

```bash
cd /opt/devforge/devforge-be
cp .env.prod.example .env
chmod 600 .env
nano .env
```

Bắt buộc điền:

| Biến | Lấy ở đâu |
| --- | --- |
| `DOMAIN`, `ACME_EMAIL` | của bạn |
| `DB_PASSWORD` | `openssl rand -base64 32` — **và dán lại vào `DATABASE_URL`** |
| `JWT_SECRET` | `openssl rand -base64 48` |
| `GOOGLE_CLIENT_ID` / `_SECRET` | Google Cloud Console |
| `SMTP_*` | Resend (3000 mail/tháng) hoặc Brevo (300/ngày) |
| `R2_*` | bước G |

Ba giá trị đừng đổi, chúng là lý do bước này tồn tại:

- `TRUSTED_PROXIES=127.0.0.1,::1,172.16.0.0/12` — **bắt buộc** (README §9.5).
  Để mặc định thì Gin thấy peer là IP container của Caddy, `X-Forwarded-For` bị
  bỏ, `PUBLIC_RATE_LIMIT` 60/phút **theo địa chỉ** biến thành 60/phút **toàn
  cầu** (một người xem link chia sẻ làm cạn quota của tất cả), và mọi hàng
  `audit_logs.ip` ghi cùng một IP. Chỉ mở rộng tới dải bridge được vì API không
  publish cổng nào và Caddy là lối vào duy nhất. **Không bao giờ `0.0.0.0/0`.**
- `PUBLIC_URL=https://<domain>` — **không có `/api` ở cuối.** Hai chỗ dùng biến
  này đều tự nối path riêng: `upload.Saver` nối `/uploads/...` (route ở root),
  handler preview nối `/api/shared-drills/...`.
- `OPENROUTER_API_KEY=` **để trống** cho lần lên đầu. Thiếu key thì tính năng
  sinh kịch bản tắt, riêng endpoint đó trả 503, server chạy bình thường. Tiền
  AI là khoản duy nhất vượt được tiền hạ tầng — ở trần cứng với opus-5 lên tới
  hàng chục triệu/tháng (README §9.6). Bật sau khi đã nhìn số thật một ngày,
  và bật với `anthropic/claude-haiku-4-5`.

Google Console: thêm redirect URI `https://<domain>/api/auth/google/callback`.

## E. Lên lần đầu

```bash
cd /opt/devforge/devforge-be
./deploy/up.sh
```

`up.sh` làm theo thứ tự: build ảnh api + web → build 4 ảnh lab → bật
postgres/redis → chạy migration trên mạng compose → `up -d` → chờ healthcheck
của `api` tối đa 120s. Trượt thì tự `git checkout` về commit trước, build lại,
và in 50 dòng log cuối.

Healthcheck là `/server healthcheck` — ảnh prod là distroless, không có shell
lẫn wget nên binary tự probe `/readyz` của chính nó. `/readyz` chứ không phải
`/healthz`: tiến trình trả lời được nhưng không với tới Postgres thì không đáng
giữ lại.

Rollback **không** chạy migration down. Migration bắt buộc tương thích ngược
(README §8), nên binary cũ chạy được với schema mới; hạ migration giữa lúc sự
cố là cách biến rollback thành mất dữ liệu.

Kiểm tra:

```bash
curl -sf https://<domain>/healthz
curl -sf https://<domain>/readyz
docker compose -f docker-compose.yml -f docker-compose.prod.yml ps
```

Cert chưa ra thì xem `docker compose ... logs caddy` — gần như luôn là DNS
chưa trỏ hoặc cổng 80 chưa mở ở Security List của Oracle.

## F. Tài khoản admin

Migration `000020` đã tạo sẵn `superadmin`. Đổi mật khẩu, hoặc tạo tài khoản
riêng:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml \
  exec -T postgres psql -U devforge -d devforge -c '\dt'   # kiểm tra schema đã lên
```

Lệnh `make admin` chạy `go run` trên host — trên máy prod không có Go toolchain,
nên tạo admin bằng `cmd/createadmin` phải build trong container hoặc đổi mật
khẩu cho `superadmin` qua giao diện.

## G. Backup — làm TRƯỚC khi có gì để mất

Hạ tầng free không có SLA và Oracle khoá tài khoản gần như không giải thích.
Nếu chuyện đó xảy ra, bản `pg_dump` trên R2 là thứ duy nhất còn lại. Đây là lý
do lộ trình đảo P10 lên trước P9.

1. Cloudflare R2 → tạo bucket `devforge-backup` (10 GB free, egress 0₫).
2. API token với quyền Object Read & Write → điền `R2_*` vào `.env`.
3. Ở bucket, đặt **lifecycle rule xoá sau 30 ngày** (retention là luật của R2,
   không phải code trong script).
4. Chạy tay một lần:

```bash
./scripts/backup.sh
```

5. Cron:

```bash
crontab -e
15 3 * * * cd /opt/devforge/devforge-be && ./scripts/backup.sh >> /var/log/devforge-backup.log 2>&1
```

6. **Diễn tập restore ngay, đừng để sau.** Bản backup chưa ai restore là một
   phỏng đoán:

```bash
./scripts/restore.sh          # nạp vào devforge_restore_check, không đụng DB thật
```

Script in số hàng của `users` / `courses` / `labs` / `lab_task_completions`.
Số 0 nghĩa là backup hỏng, không phải là script hỏng.

## H. CD tự động

Trong **cả hai** repo GitHub → Settings → Secrets → Actions:

| Secret | Giá trị |
| --- | --- |
| `SSH_HOST` | `<IP>` |
| `SSH_USER` | `devforge` |
| `SSH_KEY` | private key khớp với `authorized_keys` của user `devforge` |

Từ đó `push` vào `master` → CI (lint/test/gitleaks) xanh → job `deploy` ssh vào
máy, `git pull` cả hai repo, chạy `up.sh`. Secret prod không đi qua CI: `.env`
nằm trên máy ở `chmod 600`.

CD của repo FE và của repo BE cùng gọi một `up.sh`, nên đẩy repo nào cũng ra
được bản mới của cả hai.

---

## Còn nợ (cố ý)

| Việc | Vì sao chưa làm | Làm khi nào |
| --- | --- | --- |
| **P3.5 observability** — Grafana Alloy → Grafana Cloud | Cần tài khoản Grafana Cloud và thêm một agent thường trú ăn RAM. Không chặn việc lên prod. | Ngay sau khi có người dùng thật. Alert quan trọng nhất: `lab_containers_running > lab_sessions_active` kéo dài = có container mồ côi ăn ghế cho tới khi hết máy. |
| Ansible | Một host, chạy một lần. `bootstrap.sh` là 40 dòng `apt`. | Khi có máy thứ hai. |
| GHCR + rollback theo tag SHA | Build trên máy nhanh hơn QEMU cả chục lần; rollback hiện là build lại (~1–2 phút). | Khi 1–2 phút downtime lúc rollback là không chấp nhận được. |
| gVisor / tách runner node (P11) | Lab container đã `--network=none` + socket-proxy. | Khi mở cho người lạ ngoài lớp học. |

## Núm vặn khi máy chậm

Cả ba là biến môi trường, không phải code (README §9.4):

1. `LAB_SESSION_TTL` 30m → 20m: reaper quét theo hạn này, vòng quay ghế nhanh gấp rưỡi.
2. `MAX_CONTAINERS` 40 → 25. Bốn nhân chỉ gánh nổi ~8 container **bận CPU** cùng lúc.
3. Xếp **bài mô phỏng** lên trước bài container trong lộ trình học — sim tốn 0
   container, kiểm tra sức chứa nằm *sau* nhánh sim trong `labs.go` một cách có
   chủ đích.
