-- Restores the Vietnamese War Room content that scripts/seed.sql inserted before
-- the frontend became English-only.

UPDATE labs SET
    title = 'Ca Trực Đầu Tiên',
    description_md = $md$**23:41.** Điện thoại rung. Trang chủ trả lỗi, khách đang kêu trên mạng xã hội.

Bạn chỉ biết chừng đó — đúng như lúc trực thật.

### Việc của bạn

Dịch vụ web chạy ở `http://127.0.0.1:8080`, và nó có một đường
`/healthz` trả về đúng chữ `ok` khi mọi thứ bình thường:

```sh
curl -i http://127.0.0.1:8080/healthz
```

Làm cho câu lệnh đó trả `ok` trở lại. Xong thì bấm **Kiểm tra** — đó cũng là lúc
đồng hồ sự cố dừng, nên đừng sửa xong rồi ngồi đọc tiếp.

### Không ai nói bạn hỏng ở đâu

Cố ý. Mò ra hỏng ở đâu **là** bài học; biết trước thì phần còn lại chỉ là gõ.
Mấy chỗ đáng nhìn trước:

```sh
curl -v http://127.0.0.1:8080/healthz   # nó im lặng, hay nó trả lỗi?
ss -ltn                                  # có ai đang giữ cổng 8080 không?
ps aux                                   # tiến trình nào đang chạy, chạy với tham số gì?
ls -l ~/web                              # file còn đó không, quyền còn đọc được không?
```

Dịch vụ được dựng bằng `httpd` của busybox, phục vụ thư mục `~/web`:

```sh
httpd -p 127.0.0.1:8080 -h ~/web
```

> Mọi lệnh bạn gõ trong phiên này được ghi lại, và hiện ở trang kết quả sau khi
> kết thúc — để bạn thấy mình đã mất bao lâu ở hướng nào. Chỉ bạn và quản trị
> viên đọc được, và nó mất cùng lúc với phiên.$md$
WHERE slug = 'incident-lab-1';

UPDATE lab_tasks SET
    title = 'Khôi phục dịch vụ: /healthz ở cổng 8080 trả về ok',
    hint = 'Ba câu hỏi theo thứ tự đó: có ai nghe ở cổng 8080 không (ss -ltn), tiến trình đang nghe là cái gì và trỏ vào đâu (ps aux), thư mục nó phục vụ còn đọc được không (ls -l ~/web).'
FROM labs l
WHERE lab_tasks.lab_id = l.id AND l.slug = 'incident-lab-1' AND lab_tasks.order_idx = 0;

UPDATE lab_incidents SET
    title = 'Tiến trình web đã chết',
    reveal_md = $md$### Tiến trình `httpd` không còn chạy

Không ai nghe ở cổng 8080 cả, nên `curl` báo **Failed to connect** chứ không trả
về mã lỗi HTTP nào. Đó là dấu hiệu tách bạch nhất trong ba kịch bản: hỏng ở tầng
kết nối, không phải ở tầng ứng dụng.

Đường tìm ra: `ss -ltn` không thấy dòng nào cho 8080, `ps aux` không thấy
`httpd`. Sửa bằng cách chạy lại nó:

```sh
httpd -p 127.0.0.1:8080 -h ~/web
```

Ngoài đời không ai chạy tay như vậy — process manager (systemd, supervisor,
container restart policy) tự bật lại. Câu hỏi thật khi gặp cảnh này là **vì sao
nó chết**, và câu trả lời gần như luôn nằm trong log hoặc trong OOM killer.$md$
FROM labs l
WHERE lab_incidents.lab_id = l.id AND l.slug = 'incident-lab-1'
  AND lab_incidents.break_script = $sh$pkill httpd$sh$;

UPDATE lab_incidents SET
    title = 'Tiến trình khác đang giữ cổng 8080',
    break_script = $sh$pkill httpd
mkdir -p "$HOME/old-release"
printf 'ban cu, khong co healthz\n' > "$HOME/old-release/index.html"
httpd -p 127.0.0.1:8080 -h "$HOME/old-release"$sh$,
    reveal_md = $md$### Cổng 8080 bị một `httpd` khác chiếm, và nó phục vụ nhầm thư mục

Cổng vẫn có người nghe, nên `curl` **kết nối được** — chỉ là `/healthz` trả
**404**. Một dịch vụ trả lời sai khác hẳn một dịch vụ không trả lời, và đó là
thứ phân biệt kịch bản này với kịch bản "tiến trình đã chết".

Đường tìm ra: `ss -ltn` thấy 8080 đang LISTEN, `ps aux` thấy `httpd` chạy với
`-h /home/student/old-release` — sai thư mục. Sửa: giết nó rồi bật lại đúng chỗ.

```sh
pkill httpd
httpd -p 127.0.0.1:8080 -h ~/web
```

Ngoài đời đây là cảnh deploy hụt: bản cũ chưa tắt hẳn, bản mới không gắn được
cổng nên chết ngay lúc khởi động, và thứ đang phục vụ khách là bản đáng lẽ đã bị
thay. Bài học: **cổng có người nghe không có nghĩa là đúng người đang nghe.**$md$
FROM labs l
WHERE lab_incidents.lab_id = l.id AND l.slug = 'incident-lab-1'
  AND lab_incidents.break_script LIKE '%old-release%';

UPDATE lab_incidents SET
    title = 'Thư mục web mất quyền đọc',
    reveal_md = $md$### `~/web` bị `chmod 000`

`httpd` vẫn chạy, cổng vẫn LISTEN, nhưng nó không mở nổi file trong thư mục nên
mọi đường dẫn đều ra **404** — kể cả `/index.html` vốn vẫn nằm nguyên đó.

Đường tìm ra: `ls -ld ~/web` cho ra `d---------`. Sửa:

```sh
chmod 755 ~/web
```

Chỗ dễ mất thì giờ nhất ở kịch bản này là tin vào mã lỗi: 404 đọc ra là "file
không tồn tại", nên người ta đi tìm file trước khi nhìn quyền — mà file vẫn ở
đó. Ngoài đời cảnh này hay tới sau một lệnh `chmod`/`chown` chạy nhầm thư mục,
hoặc một tiến trình deploy chạy dưới user khác.$md$
FROM labs l
WHERE lab_incidents.lab_id = l.id AND l.slug = 'incident-lab-1'
  AND lab_incidents.break_script LIKE 'chmod 000%';
