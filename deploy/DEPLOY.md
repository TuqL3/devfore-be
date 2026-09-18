# Runbook triển khai

> **Chưa ai chạy tài liệu này trên một máy thật.** Máy chưa mua (INFRA.md
> §13.1), nên mọi lệnh dưới đây đọc từ file trong repo chứ không phải chép lại
> từ một phiên SSH đã chạy thành công. Chỗ nào chỉ biết khi đứng trên máy —
> đường dẫn Hostinger đặt sẵn, tên service Docker của template — đều có dấu
> ⚠️ **kiểm trên máy**. Sửa tài liệu này ngay trong lần dựng đầu tiên, lúc còn
> nhớ; một runbook chép lại từ lần chạy thật đáng giá gấp nhiều lần bản suy ra
> từ file.
>
> Bản trước là runbook cho Oracle Always Free + Caddy + build trên máy prod.
> Không còn bước nào trong đó đúng; `git log -- deploy/DEPLOY.md` còn nguyên.

Hình dạng của hệ: image build một lần trong GitHub Actions → GHCR → máy prod
`pull`. Không có gì được build trên máy. Biên là nginx với Cloudflare Origin
Certificate, một domain, route theo path (INFRA.md §8).

```
trình duyệt → Cloudflare (mây cam) → nginx :443 → api:8080 / web:80
```

---

## 0. Thứ tự

Chín bước, và thứ tự là bắt buộc — INFRA.md §13.4 là bản gốc của danh sách này.

```
1. Mua máy · DNS · Origin Cert                 §1
2. bootstrap.sh trên máy mới                   §2
3. .env + cert                                 §3
4. make release v0.1.0 → deploy tay lần đầu    §4
5. Kiểm chứng                                  §5
6. R2 + backup tay + DIỄN TẬP RESTORE          §6   ← đừng để sau
7. Bật CD: thêm 3 secret SSH, cắt tag tiếp     §7
8. Diễn tập rollback                           §8
9. Máy staging                                 §9
```

Bước 6 và bước 8 là hai bước hay bị bỏ nhất, và là hai bước duy nhất chứng minh
được lưới an toàn có thật.

---

## 1. Trước khi SSH

Việc tay, cần đăng nhập, không script hoá được. Danh sách đầy đủ ở INFRA.md
§13.1 — đây là phần phải xong **trước** khi chạm vào máy.

**Máy.** VPS Hostinger **KVM2, vùng Singapore**, template **Ubuntu 24.04 có sẵn
Docker**. Singapore là ràng buộc cứng chứ không phải sở thích: terminal là
xterm.js qua WebSocket, mỗi phím là một vòng round-trip, và EU/US 250–300ms làm
hỏng đúng tính năng cốt lõi. Template có sẵn Docker vì `bootstrap.sh` **từ chối
chạy** khi không thấy Docker — nó không tự cài nữa (hai bản cài Docker trên một
máy là chỗ `docker ps` và compose bất đồng ý về việc ai đang giữ container).

**Tường lửa hPanel.** Mở 80 và 443 **trong hPanel**. `ufw` mà `bootstrap.sh` đặt
là lớp cửa **thứ hai**. Quên lớp hPanel thì máy không ai vào được, và
`ufw status` vẫn báo mọi thứ ổn — đó là lý do nó đứng ở đây, trước mọi thứ khác.

**Snapshot.** Bật backup/snapshot tuần của Hostinger. Nó **không** thay
`scripts/backup.sh` → R2: snapshot nằm cùng tài khoản với máy, mất tài khoản là
mất cả hai. Snapshot để khôi phục nhanh, R2 để sống sót.

**DNS Cloudflare.** `<domain>` A → IP máy prod. **Mây cam**, bắt buộc, không
phải tuỳ chọn — xem mục cert ngay dưới. Không có bản ghi `api.` nào để tạo: FE
và API chung một origin (INFRA.md §9.1.1).

**SSL mode ở Cloudflare: Full (strict).** Không phải Flexible. Flexible nghĩa là
Cloudflare → origin đi bằng http, tức là cookie phiên đi trần trên đoạn cuối, và
`X-Forwarded-Proto` nói dối.

**Origin Certificate.** Cloudflare → SSL/TLS → Origin Server → Create
Certificate, cho `<domain>` **và** `*.<domain>` (dấu sao là thứ để cùng một file
phục vụ luôn `staging.<domain>`). Lưu hai phần:

```
deploy/nginx/certs/origin.pem     # certificate
deploy/nginx/certs/origin.key     # private key, chmod 600
```

Cert này **chỉ Cloudflare tin**. Tắt mây cam là trình duyệt báo cert không hợp
lệ ngay lập tức — đó chính là lý do mây cam thành bắt buộc, và cũng là lý do bỏ
được Caddy: thứ Caddy bán là HTTPS tự động, mà sau mây cam thì không dùng được.
Hạn 15 năm, không có ACME, không có challenge nào để hỏng.

**Cache rule Cloudflare.** Kiểm rằng **không** rule nào phủ `/api/*`, `/ws/*`,
`/uploads/*`. Cùng origin nên rule phải theo path chứ không theo host. Cache
`/api/*` là phát response có `Set-Cookie` của người này cho người kia; rule sai
còn phá được bước nâng cấp WebSocket của `/ws/*`. Chỉ cache `assets/*` và
`index.html`.

**Authenticated Origin Pulls** — nên bật, và bật **sau** khi site đã chạy. Nó
chặn người tìm ra IP thật của VPS rồi gọi thẳng, bỏ qua WAF và rate limit;
Origin Certificate không làm được việc đó vì client có quyền không kiểm cert.
Cách bật nằm trong comment ở `deploy/nginx/devforge.conf`. Thứ tự quan trọng:
bật ở dashboard Cloudflare **trước**, xác nhận site còn phục vụ, rồi mới bỏ
comment hai dòng nginx và reload. Ngược lại thì nginx đòi client cert trong khi
Cloudflare chưa gửi, và site trả 400 cho tất cả, kể cả bạn.

**GHCR sang Public.** 6 package: `devforge-api`, `devforge-web`, và 4
`devforge-lab-{linux,git,docker,net}`. ❗Không làm thì máy phải
`docker login ghcr.io` bằng PAT chỉ để pull bản phát hành của chính mình.

**Google Console.** Redirect URI `https://<domain>/api/auth/google/callback`,
khớp từng ký tự. Đăng ký luôn cái thứ hai cho staging ở bước §9.

**R2.** Tạo bucket + lifecycle xoá sau 30 ngày.

**Secrets repo `devforge-be`.** `SSH_HOST`, `SSH_USER`, `SSH_KEY` — khoá deploy
**riêng**, không dùng lại khoá cá nhân. Repo `devforge-fe` không cần secret nào.

**Uptime monitor ngoài** ping `https://<domain>/readyz` 5 phút một lần, báo
Telegram. Đừng cài trên chính VPS — nó là thứ duy nhất báo được "máy chết".

---

## 2. Dựng máy

```bash
scp deploy/bootstrap.sh root@<ip>:/tmp/
ssh root@<ip> 'bash /tmp/bootstrap.sh'
```

Script làm: kiểm Docker (không cài), giới hạn log container 10m×3, tạo user
`devforge` trong nhóm docker, chép `authorized_keys` của root sang cho user đó,
`ufw` 22/80/443, tắt đăng nhập root bằng mật khẩu, swap 2 GB, cron backup 3:15
sáng + logrotate, unattended-upgrades.

⚠️ **kiểm trên máy:** template Hostinger có thể đã có sẵn user, swap, hoặc một
`ufw` đang bật với luật khác. Script `ufw --force reset` nên nó ghi đè — đọc
output, đừng chỉ nhìn exit code.

Giới hạn log không phải trang trí: lab container là con vật chính của sản phẩm
và chúng là gia súc; một lab chạy loạn không có trần log sẽ ghi đầy 200 GB và
kéo database chết theo.

---

## 3. Cấu hình

Từ đây trở đi, đăng nhập bằng user `devforge`, không phải root.

```bash
git clone <be repo> /opt/devforge/devforge-be
cd /opt/devforge/devforge-be
cp .env.prod.example .env && chmod 600 .env
```

Chỉ clone repo **be**. FE không còn trên máy — nó là image `devforge-web`.

Điền `.env`. Bảng đầy đủ ở INFRA.md §8; những dòng dễ sai:

| Biến | Giá trị | Sai thì sao |
| --- | --- | --- |
| `IMAGE_REPO` | `ghcr.io/<owner>` **chữ thường** | GHCR từ chối chữ hoa |
| `DB_PASSWORD` | sinh mới | `DATABASE_URL` nội suy từ đây, chỉ một chỗ để sửa |
| `JWT_SECRET` | sinh mới | mất nó là mất mọi phiên đăng nhập |
| `PUBLIC_URL` | `https://<domain>` — **không có `/api`** | thêm `/api` → ảnh upload thành `/api/uploads`, không route nào phục vụ |
| `TRUSTED_PROXIES` | `127.0.0.1,::1,172.16.0.0/12` | để mặc định → rate limit thành 60/phút cho cả internet, `audit_logs.ip` một giá trị cho mọi hàng |
| `SMTP_*` | Resend hoặc Brevo | **hỏng im lặng**: người đăng ký mới không bao giờ nhận được mã, không lỗi nào nổ ở server |
| `OPENROUTER_API_KEY` | **để rỗng** lần đầu | đây là khoản duy nhất có thể vượt mặt tiền máy (§9.6). Điền sau khi xem một ngày tải thật |

Sinh secret:

```bash
openssl rand -base64 32     # JWT_SECRET
openssl rand -base64 24     # DB_PASSWORD
```

`.env` **không có bản sao ở đâu cả** — `scripts/backup.sh` dump database chứ
không dump config. Dựng lại máy mà không có file này trong tay thì phải sinh lại
toàn bộ khoá.

Cert, dán từ Cloudflare:

```bash
install -m 644 /dev/null deploy/nginx/certs/origin.pem
install -m 600 /dev/null deploy/nginx/certs/origin.key
nano deploy/nginx/certs/origin.pem   # dán certificate
nano deploy/nginx/certs/origin.key   # dán private key
```

`deploy/nginx/certs/.gitignore` là `*` + `!.gitignore`, nên khoá riêng không bao
giờ vào git kể cả khi ai đó `git add -A`.

**Cert và `.env` sống qua mọi lần deploy.** CD chạy `git fetch` +
`git checkout --force --detach <tag>` và **không** `git clean`, nên file
untracked bị gitignore ở lại nguyên. Đừng thêm `git clean` vào đường CD: nó sẽ
xoá đúng hai thứ không có bản sao ở đâu.

---

## 4. Deploy tay lần đầu

⚠️ **Phải là tag phiên bản `vX.Y.Z`, không phải `sha-<short>`.** `up.sh` kéo
`api`, `web` và 4 image lab bằng **cùng một** `IMAGE_TAG`. `api` và 4 image lab
build ở repo be nên mang sha của be; `web` build ở repo fe nên mang sha của fe —
hai con số khác nhau:

```
be  master  →  devforge-api:sha-226ed8d  +  4 × devforge-lab-*:sha-226ed8d
fe  master  →  devforge-web:sha-33c8d37
```

`IMAGE_TAG=sha-<của be>` kéo được api và 4 lab rồi chết ở `devforge-web` không
tồn tại. Tag phiên bản là **thứ duy nhất cả hai repo cùng promote**, và đó đúng
là lý do nó tồn tại.

Nên trước bước này phải cắt một tag:

```bash
make release v=v0.1.0        # gắn tag CẢ HAI repo rồi push
```

Đợi `promote` xanh ở cả hai repo — nó tạo `:v0.1.0` từ `:sha-<short>` có sẵn.
Job `deploy` cũng chạy theo tag, nhưng **tự bỏ qua khi chưa có secret
`SSH_HOST`**, nên lúc này nó không làm gì và master vẫn xanh. Đó là điều kiện để
bạn tự tay làm lần deploy đầu thay vì giao nó cho một job.

⚠️ Bẫy của tag đầu tiên: `promote` đòi image `:sha-<short>` của commit được tag
phải có sẵn trên GHCR, mà image chỉ build khi push **master**. Tag một commit cũ
hơn job `images` thì `promote` dừng và nói thẳng lý do.

Rồi trên máy:

```bash
IMAGE_TAG=v0.1.0 ./deploy/up.sh
```

`up.sh` làm, theo thứ tự: khoá chống hai lần deploy chồng nhau → pull `api`,
`web` và 4 image lab rồi `docker tag` chúng về `devforge/<tên>:latest` cho khớp
bảng `lab_images` → `up -d --wait postgres redis` → chạy migration trong
container `migrate/migrate` → `up -d --wait --wait-timeout 120` cả stack → ghi
tag vào `.image-tag`.

Lần đầu **chưa có `.image-tag`**, nên nếu bản này không lên khoẻ thì script nói
thẳng là không có gì để lùi và để nguyên stack cho bạn đọc log — chứ không lùi
bừa.

Migration chạy **trước** khi container api mới khởi động. Điều đó an toàn vì
migration bắt buộc tương thích ngược (INFRA.md §8): schema mới phải chạy được
với binary cũ, không phải ngược lại.

### Admin đầu tiên

`make admin` **không dùng được trên máy prod**: nó chạy `go run ./cmd/createadmin`,
mà image prod là distroless và `Dockerfile` chỉ build `cmd/server`. Trên máy
không có Go, không có binary đó, và không có shell trong container api.

Đường đi thật, hai bước:

1. **Đăng ký một tài khoản bình thường qua UI** ở `https://<domain>`. Không có
   cổng xác minh email nào chặn đăng nhập, nên bước này chạy được cả khi `SMTP_*`
   chưa điền.
2. **Cấp quyền admin bằng SQL** — quyền nằm ở bảng nối `user_roles`, không phải
   một cột trên `users`:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml \
  exec -T postgres psql -U "$DB_USER" -d "$DB_NAME" <<'SQL'
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u, roles r
WHERE u.email = 'you@example.com' AND r.name = 'admin'
ON CONFLICT DO NOTHING;
SQL
```

Đăng xuất rồi đăng nhập lại để token mang vai trò mới.

Không đưa `createadmin` vào image prod có chủ ý: đó là một binary đúc được thông
tin đăng nhập, và nó chỉ cần một lần cho mỗi máy.

---

## 5. Kiểm chứng

Từ máy của bạn, không phải từ trên VPS:

```bash
curl -sf https://<domain>/healthz              # tiến trình sống
curl -sf https://<domain>/readyz               # + nối được DB
curl -sI https://<domain>/ | grep -i cf-       # đi qua Cloudflare
```

Rồi những thứ **chỉ hỏng ở prod** nên phải nhìn tận mắt trên trình duyệt:

- **Đăng nhập rồi F5** — còn đăng nhập nghĩa là cookie same-site đúng.
- **Mở một lab, gõ vài phím** — không khựng nghĩa là `proxy_buffering off` đúng.
- **Xem một ảnh bìa** — hiện được nghĩa là biên có route riêng cho `/uploads/*`.
  Hỏng thì nó trả `index.html` kèm **200**, tức ảnh vỡ chứ không phải 404 để mà
  grep.
- **Mở một lab rồi `docker restart` container api** — terminal phải tự nối lại.
  Đây là thứ duy nhất kiểm được reconnect, và nó là thứ **mỗi lần deploy** sẽ
  làm với mọi học viên đang mở terminal.
- **Dán một link `/r/<id>` vào Zalo/Slack** — ra thẻ xem trước nghĩa là khối
  crawler ở biên chạy và nó đứng đúng trước route bắt-tất-cả.
- **Xem `audit_logs.ip` của lần đăng nhập vừa rồi** — phải ra IP thật của bạn,
  không phải IP Cloudflare. Đó là lúc biết `set_real_ip_from` +
  `TRUSTED_PROXIES` đúng cả hai tầng.

Sáu route của biên có check chạy được, và nó chạy trên máy dev chứ không cần
VPS:

```bash
make check-edge
```

Nó dựng chính `deploy/nginx/devforge.conf` trước hai upstream giả và hỏi từng
đường dẫn. Chạy nó mỗi lần sửa file edge — `nginx -t` chỉ nói file cú pháp đúng.

---

## 6. Backup và diễn tập restore

Cron đã được `bootstrap.sh` cài (3:15 sáng). Đừng chờ nó:

```bash
./scripts/backup.sh                 # chạy tay một lần, xem nó lên R2
./scripts/restore.sh                # nạp vào devforge_restore_check
```

`restore.sh` in số hàng của `users` / `courses` / `labs` /
`lab_task_completions`. **Số 0 nghĩa là backup hỏng, không phải script hỏng.**

Bản backup chưa ai restore là một phỏng đoán. Chạy lại diễn tập mỗi khi
migration đổi hình dạng schema.

---

## 7. Bật CD

Tới đây máy đã chạy `v0.1.0` do bạn tự deploy. Bật CD là **thêm ba secret**
`SSH_HOST`, `SSH_USER`, `SSH_KEY` (§1) — job `deploy` đang tự bỏ qua vì thiếu
chúng, và sự xuất hiện của `SSH_HOST` là thứ bật nó lên.

Rồi cắt tag tiếp theo và xem nó chạy hết đường:

```bash
make release v=v0.1.1
```

`images` → `promote` → `deploy` (chờ `devforge-web:v0.1.1` xuất hiện) → ssh →
`up.sh`. Job `deploy` có `timeout 1800`, `ConnectTimeout=10`, `BatchMode=yes`,
nên một máy treo không giữ runner sáu tiếng.

Lần này bạn đang xem CD làm đúng việc bạn vừa làm tay ở §4. So được hai bên là
điểm của thứ tự này.

---

## 8. Diễn tập rollback

Làm lúc không có sự cố. Đó là toàn bộ ý nghĩa của chữ "diễn tập".

```bash
cd /opt/devforge/devforge-be
cat .image-tag                      # tag đang phục vụ
IMAGE_TAG=<tag cũ> docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --wait
```

**Chỉ lùi image.** Không `migrate down` — một down migration giữa lúc sự cố là
cách một cú rollback biến thành mất dữ liệu. Migration bắt buộc tương thích
ngược đúng vì lý do này.

Sau rollback, checkout trên máy vẫn nằm ở tag hỏng. Không sao: CD
`checkout --force` chứ không `pull`, nên không có trạng thái nhánh nào để kẹt.
Nhưng nó có nghĩa là **file nginx và `migrations/` trên đĩa là bản mới trong khi
image là bản cũ** — cả hai bắt buộc tương thích ngược.

---

## 9. Máy staging

VPS thứ hai, **loại nhỏ nhất**, cùng vùng. Máy riêng chứ không phải stack thứ
hai trên máy prod.

Lặp lại §2 và §3 với những dòng khác nằm ở khối `STAGING` cuối
`.env.prod.example`: `DOMAIN`, `PUBLIC_URL`, `FRONTEND_URL`, `CORS_ORIGINS`,
`GOOGLE_REDIRECT_URL` đổi sang `staging.<domain>`; `MAX_CONTAINERS=4`;
`LAB_SESSION_TTL=10m`; `OPENROUTER_API_KEY` rỗng và `AI_DAILY_LIMIT=0`;
`R2_BUCKET` rỗng — staging không được backup, có chủ ý.

Cùng cert: Origin Cert đã có `*.<domain>`.

DNS `staging.<domain>` A → IP máy staging, **cũng mây cam**.

Google Console: URI thứ hai. Thiếu nó thì OAuth chết đúng ở staging.

Ba secret `STAGING_SSH_HOST`, `STAGING_SSH_USER`, `STAGING_SSH_KEY` → job
`deploy-staging` **tự bật**. Trước khi có chúng nó tự bỏ qua, nên master vẫn
xanh trong lúc chưa mua máy.

Staging deploy bằng tag trôi **`:master`**, không phải `:sha-<short>` — cả hai
repo đẩy tag đó cho bản master mới nhất của mình, và nó là thứ duy nhất gọi tên
được cả sáu image cùng lúc (§4 giải thích vì sao sha không làm được). Hệ quả:
`.image-tag` trên staging ghi `master`, nên **staging không có đích rollback**.
Đúng chủ ý — staging là máy dựng lại được, prod mới là máy cần lùi bản.

---

## Ba thứ không nằm ở đâu khác

**Prod đang chạy bản nào:**

```bash
cat /opt/devforge/devforge-be/.image-tag
docker compose -f docker-compose.yml -f docker-compose.prod.yml ps
```

`.image-tag` do `up.sh` ghi sau mỗi lần deploy khoẻ, và là đích rollback. Máy
nằm ở detached HEAD trên tag đang chạy — trạng thái đúng, đừng `git pull` bằng
tay để "sửa" nó.

**Lùi bản bằng tay:** §8.

**Diễn tập restore:** §6.

---

## Khi biên trả 502

Triệu chứng duy nhất đáng nghi sau một lần deploy. nginx cache DNS vĩnh viễn với
tên viết thẳng trong `proxy_pass`, mà container `api` nhận địa chỉ bridge mới
sau mỗi lần deploy.

`deploy/nginx/devforge.conf` đã phòng: upstream đi qua biến + `resolver
127.0.0.11`. Nếu vẫn 502, kiểm theo thứ tự:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml ps        # api có healthy không
docker compose -f docker-compose.yml -f docker-compose.prod.yml logs --tail 50 api
docker compose -f docker-compose.yml -f docker-compose.prod.yml exec nginx wget -qO- http://api:8080/healthz
```

Dòng thứ ba tách được "nginx không thấy api" khỏi "api không khoẻ" — hai nguyên
nhân trông giống hệt nhau từ ngoài.
