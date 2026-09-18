# Runbook triển khai — chưa viết

Bản trước của file này là runbook cho **Oracle Always Free + Caddy + build trên
máy prod**. Không còn bước nào trong đó đúng: hạ tầng đã chốt là hai VPS
Hostinger (prod + staging), nginx ở biên, và image build một lần trong GitHub
Actions rồi pull từ GHCR. Một runbook sai nguy hiểm hơn một runbook thiếu, nên nó
bị gỡ thay vì để lại. `git log -- deploy/DEPLOY.md` còn nguyên bản cũ.

Runbook thật viết được **sau khi mua máy**, vì phần lớn nội dung là những thứ chỉ
biết khi đứng trên máy thật: đường dẫn cert Origin, hình dạng file nginx, và bước
nào trong `bootstrap.sh` cần sửa cho template Hostinger. Xem [INFRA.md](../INFRA.md) §13.2 dòng
`deploy/DEPLOY.md`.

Trong lúc chờ, thứ tự việc nằm ở **[INFRA.md](../INFRA.md) §13.4**, và từng việc tay ở **§13.1**.

## Ba thứ không nằm ở đâu khác

**Prod đang chạy bản nào:**

```bash
cat /opt/devforge/devforge-be/.image-tag        # tag đang phục vụ
docker compose -f docker-compose.yml -f docker-compose.prod.yml ps
```

`.image-tag` do `up.sh` ghi sau mỗi lần deploy khoẻ, và là đích rollback. Máy nằm
ở detached HEAD trên tag đang chạy — trạng thái đúng, đừng `git pull` bằng tay để
"sửa" nó.

**Lùi bản bằng tay:**

```bash
cd /opt/devforge/devforge-be
IMAGE_TAG=<tag cũ> docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --wait
```

Chỉ lùi image. Không `migrate down` — migration bắt buộc tương thích ngược đúng vì
lý do này ([INFRA.md](../INFRA.md) §8).

**Diễn tập restore** — bản backup chưa ai restore là một phỏng đoán:

```bash
./scripts/restore.sh            # nạp vào devforge_restore_check, không đụng DB thật
```

In số hàng của `users` / `courses` / `labs` / `lab_task_completions`. Số 0 nghĩa
là backup hỏng, không phải script hỏng.
