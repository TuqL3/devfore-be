-- Demo content for a fresh local database: `make up && make migrate && make seed`.
--
-- The database is the source of truth (README §3) — this file only fills an
-- empty environment, and the admin UI owns everything after that. So it never
-- deletes and never overwrites: a course, lab, task or review is written only
-- where nothing is there yet. Re-running it is safe, and anything edited in the
-- admin UI stays edited.
--
-- Every check_script here runs through `/bin/sh -c` inside the lab container,
-- which is busybox ash, with no $HOME guarantees worth leaning on — hence
-- absolute /home/student paths and no bashisms. scripts/check-seed.sh runs the
-- lot against a real container.

-- ---------------------------------------------------------------------------
-- Lab images. Built by `make lab-images`; the admin UI picks a lab's image
-- from this list.
-- ---------------------------------------------------------------------------
INSERT INTO lab_images (name, tag, description)
VALUES
    ('devforge/linux',  'latest', 'Alpine + coreutils cho lab Linux'),
    ('devforge/git',    'latest', 'Alpine + git cho lab Git (không mạng, remote là bare repo trong home)'),
    ('devforge/docker', 'latest', 'Alpine cho lab Docker — không có daemon, chấm theo file và lệnh đã gõ'),
    ('devforge/net',    'latest', 'Alpine + ip/ss/curl/httpd cho lab Mạng — chỉ có loopback')
ON CONFLICT (name, tag) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Courses. A course only goes out published once it has labs — a published
-- course with nothing in it is a dead end for whoever clicks it.
-- ---------------------------------------------------------------------------
INSERT INTO courses (slug, title, description, level, status, published_at, image_url)
VALUES
    ('linux-co-ban', 'Linux Cơ Bản',
     'Làm chủ dòng lệnh Linux: filesystem, quyền, tìm kiếm, tiến trình và shell script.',
     'beginner', 'published', now(),
     'https://images.unsplash.com/photo-1629654297299-c8506221ca97?w=800'),
    ('git-thuc-hanh', 'Git Thực Hành',
     'Quản lý mã nguồn với Git: commit, branch, merge, sửa lịch sử, remote.',
     'beginner', 'published', now(),
     'https://images.unsplash.com/photo-1618401471353-b98afee0b2eb?w=800'),
    ('docker-nhap-mon', 'Docker Nhập Môn',
     'Container hoá ứng dụng: image, layer, Dockerfile, volume, network, compose.',
     'intermediate', 'published', now(),
     'https://images.unsplash.com/photo-1605745341112-85968b19335b?w=800'),
    ('mang-may-tinh', 'Mạng Máy Tính Cơ Bản',
     'Từ địa chỉ IP tới HTTP: giao diện, cổng, socket, mô hình phân tầng, DNS, client–server và chia mạng con — làm tay trên loopback.',
     'beginner', 'published', now(),
     'https://images.unsplash.com/photo-1544197150-b99a580bb7a8?w=800'),
    ('ci-cd-co-ban', 'CI/CD Cơ Bản',
     'Vì sao pipeline 8 phút xuống còn 4: job phụ thuộc nhau, chạy song song, cache và artifact — học trên một trình mô phỏng, không cần runner thật.',
     'intermediate', 'published', now(),
     'https://images.unsplash.com/photo-1667372393119-3d4c48d07fc9?w=800')
ON CONFLICT (slug) DO NOTHING;

-- ===========================================================================
-- LINUX CƠ BẢN
-- ===========================================================================

INSERT INTO labs (course_id, slug, title, description_md, duration_minutes, lab_image_id, order_idx)
SELECT c.id, v.slug, v.title, v.body, v.minutes,
       (SELECT id FROM lab_images WHERE name = 'devforge/linux' AND tag = 'latest'), v.idx
FROM courses c
JOIN (VALUES
    ('linux-lab-1', 'Điều Hướng Filesystem', 45, 0, $md$# Điều hướng filesystem

Sandbox này là một máy Linux thu nhỏ: **không có mạng**, và mọi thứ bạn tạo sẽ
biến mất khi phiên kết thúc. Cứ thoải mái thử, không hỏng được gì.

Bốn lệnh đủ để đi khắp nơi:

| Lệnh | Việc |
| --- | --- |
| `pwd` | đang đứng ở thư mục nào |
| `ls -l` | liệt kê chi tiết; thêm `-a` để thấy cả file ẩn |
| `cd /etc` | nhảy theo đường dẫn **tuyệt đối** (bắt đầu bằng `/`) |
| `cd docs` | nhảy theo đường dẫn **tương đối**, tính từ chỗ đang đứng |
| `mkdir -p a/b` | tạo thư mục, `-p` tạo luôn cả cây |

Thư mục nhà của bạn là `/home/student`, viết tắt là `~`.$md$),

    ('linux-lab-2', 'Tạo Và Thao Tác File', 45, 1, $md$# Tạo và thao tác file

Ở Linux gần như mọi thứ là file. Lab này làm quen với vòng đời của một file:
tạo, ghi, sao chép, đổi tên, xoá.

- `echo "chữ" > f` — ghi đè, tạo mới nếu chưa có
- `echo "chữ" >> f` — **ghi thêm** vào cuối, giữ nguyên nội dung cũ
- `cat f` — xem toàn bộ; `head -n 5 f` / `tail -n 5 f` — xem đầu / cuối
- `cp a b` — sao chép; `mv a b` — chuyển hoặc đổi tên; `rm f` — xoá

`rm` không có thùng rác. Xoá là mất.$md$),

    ('linux-lab-3', 'Quyền Và Sở Hữu', 45, 2, $md$# Quyền và sở hữu

`ls -l` mở đầu mỗi dòng bằng chuỗi kiểu `-rwxr-xr-x`. Đọc nó theo ba nhóm:

```
-  rwx  r-x  r-x
   chủ  nhóm  người khác
```

`r` đọc = 4, `w` ghi = 2, `x` chạy = 1. Cộng lại thành số bát phân mà `chmod`
nhận: `chmod 755 f` là `rwxr-xr-x`, `chmod 600 f` là `rw-------`.

Muốn chạy một script bạn viết, nó phải có `x`. Không có `x` thì vẫn chạy được
gián tiếp bằng `sh script.sh`, nhưng `./script.sh` sẽ báo *Permission denied*.

> Trong sandbox này bạn **luôn là user `student`** và container bỏ toàn bộ đặc
> quyền, nên `chown` sẽ báo lỗi. Đó là chủ ý — phần đổi chủ sở hữu học bằng lý
> thuyết.$md$),

    ('linux-lab-4', 'Tìm Kiếm Và Xử Lý Văn Bản', 60, 3, $md$# Tìm kiếm và xử lý văn bản

Sức mạnh của dòng lệnh nằm ở chỗ nối các lệnh nhỏ lại với nhau bằng `|`.

| Lệnh | Việc |
| --- | --- |
| `grep chữ f` | lọc dòng chứa `chữ` (`-i` bỏ qua hoa thường, `-v` lấy dòng **không** chứa, `-c` đếm) |
| `find /etc -name "*.conf"` | tìm file theo tên |
| `wc -l f` | đếm số dòng |
| `cut -d: -f1 f` | cắt cột, ngăn cách bởi `:` |
| `sort` / `uniq -c` | sắp xếp / đếm dòng trùng (phải sort trước) |

File luyện tập là `/etc/passwd` — mỗi dòng một tài khoản, các cột ngăn bằng `:`,
cột 1 là tên đăng nhập, cột 7 là shell.$md$),

    ('linux-lab-5', 'Tiến Trình Và Shell Script', 60, 4, $md$# Tiến trình và shell script

Một script là một file văn bản mở đầu bằng dòng *shebang* nói ai sẽ chạy nó:

```sh
#!/bin/sh
echo "chào"
```

Cấp quyền chạy bằng `chmod +x script.sh` rồi gọi `./script.sh`.

Tiến trình:

- `lệnh &` — chạy nền, shell trả prompt lại ngay; `$!` là PID của nó
- `ps` / `ps aux` — đang có gì chạy
- `kill PID` — gửi **SIGTERM**, lịch sự: chương trình được dọn dẹp rồi thoát
- `kill -9 PID` — gửi **SIGKILL**, kernel giết ngay, không dọn dẹp gì$md$),

    ('linux-lab-6', 'Ống Dẫn, Chuyển Hướng Và Mã Thoát', 60, 5, $md$# Ống dẫn, chuyển hướng và mã thoát

Mỗi tiến trình sinh ra đã có sẵn ba luồng:

| Số | Tên | Là gì |
| --- | --- | --- |
| `0` | stdin | đầu vào |
| `1` | stdout | kết quả bình thường |
| `2` | stderr | thông báo lỗi |

Tách riêng stdout và stderr là chủ ý: bạn lọc được kết quả mà **không** lọc mất
lời báo lỗi.

```sh
lenh > ket-qua.txt          # chỉ stdout, ghi đè
lenh 2> loi.txt             # chỉ stderr
lenh > tat-ca.txt 2>&1      # gộp cả hai vào một file
lenh > /dev/null 2>&1       # vứt sạch, chỉ quan tâm mã thoát
lenh | tee f.txt            # vừa hiện ra màn hình vừa ghi file
```

> Thứ tự `> f 2>&1` có nghĩa: chuyển stdout vào `f`, **rồi** cho stderr đi theo
> chỗ stdout đang trỏ. Viết ngược `2>&1 > f` thì stderr vẫn ra màn hình.

## Mã thoát

Mọi lệnh kết thúc đều trả một con số: **0 là thành công**, khác 0 là thất bại.
`$?` giữ mã của lệnh vừa chạy.

```sh
lenh-a && lenh-b     # chỉ chạy b nếu a thành công
lenh-a || lenh-b     # chỉ chạy b nếu a thất bại
lenh-a ; lenh-b      # chạy b bất kể a ra sao
```

Đây là nền của mọi script tự động: script biết mình hỏng ở đâu là nhờ con số đó.$md$),

    ('linux-lab-7', 'Liên Kết, Nén Và Sao Lưu', 60, 6, $md$# Liên kết, nén và sao lưu

## Hai kiểu liên kết

```sh
ln -s /duong/dan/goc lien-ket     # liên kết mềm (symbolic)
ln /duong/dan/goc cung            # liên kết cứng (hard)
```

**Liên kết mềm** là một file riêng chứa *đường dẫn* tới file gốc — giống lối tắt.
Xoá gốc thì nó gãy, trỏ vào hư vô.

**Liên kết cứng** là **tên thứ hai của cùng một inode**. Không có bản gốc và bản
sao, chỉ có hai cái tên ngang hàng cùng trỏ vào một khối dữ liệu. Xoá một tên,
dữ liệu vẫn còn vì tên kia còn giữ.

```sh
stat -c %i file      # số inode — hai liên kết cứng có cùng con số này
readlink lien-ket    # liên kết mềm đang trỏ đi đâu
ls -l                # l ở đầu dòng là liên kết mềm
```

## Nén và sao lưu

`tar` gom nhiều file thành một; `gzip` nén cái đó lại. Cờ `-z` làm cả hai:

```sh
tar -czf sao-luu.tar.gz -C /home/student du-lieu   # đóng gói
tar -tzf sao-luu.tar.gz                            # xem bên trong, chưa giải
tar -xzf sao-luu.tar.gz -C khoi-phuc               # giải ra
```

Nhớ `c`reate · `t`able of contents · e`x`tract, đi kèm `z`ip và `f`ile.

## Xem chỗ trống

- `du -sh <thư mục>` — thư mục này chiếm bao nhiêu
- `df -h` — cả phân vùng còn bao nhiêu$md$),

    ('linux-lab-8', 'Biến Môi Trường Và Tuỳ Biến Shell', 60, 7, $md$# Biến môi trường và tuỳ biến shell

```sh
TEN=devforge              # biến của riêng shell hiện tại
export TEN                # cho tiến trình con thừa hưởng luôn
TEN=devforge lenh         # đặt biến chỉ cho đúng một lần chạy lệnh
echo "$TEN"               # đọc, luôn để trong nháy kép
```

Không `export` thì tiến trình con **không thấy** biến đó — đây là lỗi hay gặp
nhất khi viết script.

## PATH

`PATH` là danh sách thư mục, ngăn bằng dấu hai chấm, shell tìm lệnh theo đúng thứ
tự đó. Muốn gọi script của mình từ bất cứ đâu thì thêm thư mục chứa nó vào:

```sh
export PATH="$HOME/bin:$PATH"
which chao       # tìm thấy ở đường dẫn nào
type cd          # là lệnh ngoài, hàm, hay lệnh dựng sẵn của shell
```

## Tham số của script

```sh
#!/bin/sh
echo "tham số đầu: $1"    # $2, $3… là các tham số sau
echo "số tham số: $#"
echo "tất cả: $@"
```

> Biến môi trường chỉ sống trong phiên shell hiện tại. Mở terminal mới là mất —
> muốn giữ thì viết vào file cấu hình shell.$md$)
) AS v(slug, title, minutes, idx, body) ON true
WHERE c.slug = 'linux-co-ban'
ON CONFLICT (slug) DO NOTHING;

-- --- linux-lab-1 -----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Tạo thư mục làm việc ~/devforge', 'Dấu ~ chính là /home/student.', 'script',
     'test -d /home/student/devforge', '', '[]'),

    (1, 'Tạo ba thư mục con bin, docs, tmp trong ~/devforge',
     'mkdir nhận nhiều tên một lúc, và -p tạo luôn thư mục cha còn thiếu.', 'script',
     'test -d /home/student/devforge/bin && test -d /home/student/devforge/docs && test -d /home/student/devforge/tmp',
     '', '[]'),

    (2, 'Hiện đường dẫn thư mục hiện tại', '', 'command', '', 'pwd', '[]'),

    (3, 'Liệt kê chi tiết mọi thứ trong /etc, kể cả file ẩn',
     'Gộp hai cờ lại: một cờ cho “chi tiết”, một cờ cho “kể cả file ẩn”.', 'command', '',
     E'ls -la /etc\nls -al /etc\nls -a -l /etc', '[]'),

    (4, 'Đường dẫn nào là đường dẫn tuyệt đối?', '', 'choice', '', '',
     '[{"text": "/etc/hosts", "correct": true},
       {"text": "devforge/docs", "correct": false},
       {"text": "./ghi-chu.txt", "correct": false},
       {"text": "../etc/hosts", "correct": false}]'),

    (5, 'Lệnh nào đưa bạn về thư mục nhà? (chọn mọi đáp án đúng)', '', 'choice', '', '',
     '[{"text": "cd", "correct": true},
       {"text": "cd ~", "correct": true},
       {"text": "cd /home/student", "correct": true},
       {"text": "cd ..", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'linux-lab-1'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- linux-lab-2 -----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Tạo ~/devforge/docs/ghi-chu.txt với đúng một dòng: xin chao devforge',
     'Dấu > ghi nội dung vào file và tạo file nếu chưa có. Thư mục docs phải tồn tại trước.', 'script',
     'grep -qx "xin chao devforge" /home/student/devforge/docs/ghi-chu.txt && test "$(wc -l < /home/student/devforge/docs/ghi-chu.txt)" = "1"',
     '', '[]'),

    (1, 'Thêm dòng thứ hai “dong thu hai” mà không mất dòng cũ',
     'Một dấu > ghi đè, hai dấu >> ghi thêm.', 'script',
     'test "$(wc -l < /home/student/devforge/docs/ghi-chu.txt)" = "2" && head -n 1 /home/student/devforge/docs/ghi-chu.txt | grep -qx "xin chao devforge" && tail -n 1 /home/student/devforge/docs/ghi-chu.txt | grep -qx "dong thu hai"',
     '', '[]'),

    (2, 'Sao lưu thành ghi-chu.txt.bak trong cùng thư mục, nội dung giống hệt', '', 'script',
     'cmp -s /home/student/devforge/docs/ghi-chu.txt /home/student/devforge/docs/ghi-chu.txt.bak',
     '', '[]'),

    (3, 'Chuyển bản sao sang ~/devforge/tmp và đổi tên thành ghi-chu-cu.txt',
     'mv làm cả hai việc cùng lúc: chuyển chỗ và đổi tên. Bản .bak không được còn ở docs.', 'script',
     'test -f /home/student/devforge/tmp/ghi-chu-cu.txt && ! test -e /home/student/devforge/docs/ghi-chu.txt.bak',
     '', '[]'),

    (4, 'Lệnh nào ghi thêm vào cuối file mà giữ nguyên nội dung cũ?', '', 'choice', '', '',
     '[{"text": "echo \"chu\" >> f.txt", "correct": true},
       {"text": "echo \"chu\" > f.txt", "correct": false},
       {"text": "cat \"chu\" > f.txt", "correct": false},
       {"text": "touch f.txt", "correct": false}]'),

    (5, 'Xem 5 dòng đầu của /etc/passwd', '', 'command', '',
     E'head -n 5 /etc/passwd\nhead -n5 /etc/passwd\nhead -5 /etc/passwd', '[]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'linux-lab-2'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- linux-lab-3 -----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Viết ~/devforge/bin/hello.sh in ra đúng dòng: hello devforge',
     'Mở bằng nano, dòng đầu là #!/bin/sh, dòng sau là lệnh echo. Chưa cần quyền chạy.', 'script',
     'test -f /home/student/devforge/bin/hello.sh && test "$(sh /home/student/devforge/bin/hello.sh)" = "hello devforge"',
     '', '[]'),

    (1, 'Cấp quyền 755 cho hello.sh rồi chạy trực tiếp bằng ./hello.sh',
     'chmod nhận số bát phân: rwx cho chủ, r-x cho phần còn lại.', 'script',
     'test "$(stat -c %a /home/student/devforge/bin/hello.sh)" = "755" && test "$(/home/student/devforge/bin/hello.sh)" = "hello devforge"',
     '', '[]'),

    (2, 'Tạo ~/devforge/docs/bi-mat.txt mà chỉ chủ sở hữu đọc và ghi được',
     'Chỉ chủ sở hữu: đọc 4 + ghi 2 = 6. Nhóm và người khác không có gì.', 'script',
     'test -f /home/student/devforge/docs/bi-mat.txt && test "$(stat -c %a /home/student/devforge/docs/bi-mat.txt)" = "600"',
     '', '[]'),

    (3, 'Với chuỗi quyền -rwxr-xr-x, ai được phép ghi vào file?', '', 'choice', '', '',
     '[{"text": "Chỉ chủ sở hữu", "correct": true},
       {"text": "Chủ sở hữu và nhóm", "correct": false},
       {"text": "Mọi người", "correct": false},
       {"text": "Không ai cả", "correct": false}]'),

    (4, 'chmod 644 ghi-chu.txt cho ra quyền gì?', '', 'choice', '', '',
     '[{"text": "Chủ: đọc + ghi. Nhóm và người khác: chỉ đọc", "correct": true},
       {"text": "Chủ: đọc + ghi + chạy. Nhóm và người khác: chỉ đọc", "correct": false},
       {"text": "Mọi người đọc và ghi được", "correct": false},
       {"text": "Chỉ chủ sở hữu đọc được, không ai khác thấy gì", "correct": false}]'),

    (5, 'Xem umask hiện tại của shell', '', 'command', '', 'umask', '[]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'linux-lab-3'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- linux-lab-4 -----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Lọc mọi dòng chứa “root” trong /etc/passwd, lưu vào ~/ket-qua.txt',
     'grep in ra màn hình; dấu > chuyển phần in ra đó vào file.', 'script',
     'grep root /etc/passwd | cmp -s - /home/student/ket-qua.txt', '', '[]'),

    (1, 'Ghi số dòng của /etc/passwd (chỉ con số) vào ~/so-dong.txt',
     'wc -l < file cho ra đúng con số, không kèm tên file.', 'script',
     'test -f /home/student/so-dong.txt && test "$(tr -d " \t\n" < /home/student/so-dong.txt)" = "$(wc -l < /etc/passwd | tr -d " ")"',
     '', '[]'),

    (2, 'Lấy cột tên đăng nhập của /etc/passwd, sắp xếp A→Z, lưu vào ~/users.txt',
     'Cột ngăn bằng dấu hai chấm: cut -d: -f1. Nối sang sort bằng dấu |.', 'script',
     'cut -d: -f1 /etc/passwd | sort | cmp -s - /home/student/users.txt', '', '[]'),

    (3, 'Đếm mỗi shell trong /etc/passwd xuất hiện bao nhiêu lần, lưu vào ~/shells.txt',
     'Shell là cột 7. uniq -c chỉ đếm được các dòng giống nhau nằm cạnh nhau, nên phải sort trước.', 'script',
     'cut -d: -f7 /etc/passwd | sort | uniq -c | cmp -s - /home/student/shells.txt', '', '[]'),

    (4, 'Tìm mọi file có đuôi .conf trong /etc', '', 'command', '',
     E'find /etc -name "*.conf"\nfind /etc -name ''*.conf''\nfind /etc -name *.conf', '[]'),

    (5, 'Cờ nào của grep đếm số dòng khớp thay vì in chúng ra?', '', 'choice', '', '',
     '[{"text": "grep -c", "correct": true},
       {"text": "grep -v", "correct": false},
       {"text": "grep -i", "correct": false},
       {"text": "grep -n", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'linux-lab-4'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- linux-lab-5 -----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Tạo ~/du-lieu chứa hai file a.txt và b.txt, cả hai đều có nội dung', '', 'script',
     'test -s /home/student/du-lieu/a.txt && test -s /home/student/du-lieu/b.txt', '', '[]'),

    (1, 'Viết ~/backup.sh chép mọi thứ trong ~/du-lieu sang ~/backup, cấp quyền chạy rồi chạy nó',
     'Dòng đầu phải là shebang #!/bin/sh. Sau khi chmod +x, gọi ./backup.sh.', 'script',
     'test -x /home/student/backup.sh && head -n 1 /home/student/backup.sh | grep -q "^#!" && test -f /home/student/backup/a.txt && test -f /home/student/backup/b.txt',
     '', '[]'),

    (2, 'Chạy nền lệnh sleep 300 và lưu PID của nó vào ~/pid.txt',
     'Dấu & đẩy lệnh xuống nền; ngay sau đó $! là PID của lệnh vừa đẩy xuống.', 'script',
     'p=$(tr -d " \n" < /home/student/pid.txt); test -n "$p" && kill -0 "$p" 2>/dev/null && ps -o pid,args | grep "^ *$p " | grep -q sleep',
     '', '[]'),

    (3, 'Liệt kê mọi tiến trình đang chạy', '', 'command', '',
     E'ps aux\nps -ef\nps w\nps -A', '[]'),

    (4, 'Khác nhau giữa kill PID và kill -9 PID là gì?', '', 'choice', '', '',
     '[{"text": "kill gửi SIGTERM để chương trình tự dọn dẹp rồi thoát; kill -9 gửi SIGKILL, kernel giết ngay và không dọn gì", "correct": true},
       {"text": "kill chỉ tạm dừng, kill -9 mới thật sự dừng hẳn", "correct": false},
       {"text": "kill dành cho tiến trình nền, kill -9 dành cho tiến trình nền trước", "correct": false},
       {"text": "Không khác nhau, -9 chỉ là viết tắt", "correct": false}]'),

    (5, 'Vì sao phải export một biến trước khi lệnh con nhìn thấy nó?', '', 'choice', '', '',
     '[{"text": "Không export thì biến chỉ tồn tại trong shell hiện tại, tiến trình con không được thừa hưởng", "correct": true},
       {"text": "export ghi biến xuống đĩa để phiên sau vẫn còn", "correct": false},
       {"text": "export biến thành hằng số, không sửa được nữa", "correct": false},
       {"text": "Không cần export, mọi biến đều tự động lan sang tiến trình con", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'linux-lab-5'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- linux-lab-6 -----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Chạy ls trên /etc và một đường dẫn không tồn tại, gộp CẢ kết quả lẫn lỗi vào ~/log.txt',
     'Gộp hai luồng: > log.txt 2>&1 — đúng thứ tự đó.', 'script',
     $chk$test -f /home/student/log.txt && grep -qi "no such file" /home/student/log.txt && grep -q "passwd" /home/student/log.txt$chk$,
     '', '[]'),

    (1, 'Chạy lại lệnh đó nhưng vứt stdout đi, chỉ giữ lỗi trong ~/loi.txt',
     'Vứt stdout vào /dev/null, chuyển riêng stderr bằng 2>.', 'script',
     $chk$test -s /home/student/loi.txt && grep -qi "no such file" /home/student/loi.txt && ! grep -q "passwd" /home/student/loi.txt$chk$,
     '', '[]'),

    (2, 'Đếm số dòng /etc/passwd, dùng tee ghi kết quả vào ~/tee.txt',
     'wc -l < /etc/passwd | tee ~/tee.txt', 'script',
     $chk$test "$(tr -d " \n" < /home/student/tee.txt)" = "$(wc -l < /etc/passwd | tr -d " ")"$chk$,
     '', '[]'),

    (3, 'Chạy một lệnh chắc chắn thất bại rồi ghi mã thoát của nó vào ~/ma-thoat.txt',
     'Lệnh false luôn trả về mã khác 0. Đọc mã vừa rồi bằng $?', 'script',
     $chk$test "$(tr -d " \n" < /home/student/ma-thoat.txt)" != "0" && test -n "$(tr -d " \n" < /home/student/ma-thoat.txt)"$chk$,
     '', '[]'),

    (4, 'Đếm xem /etc có bao nhiêu mục, bằng cách nối hai lệnh qua ống dẫn', '', 'command', '',
     E'ls /etc | wc -l\nls -1 /etc | wc -l', '[]'),

    (5, 'Câu lệnh `lenh-a || lenh-b` chạy lenh-b khi nào?', '', 'choice', '', '',
     '[{"text": "Chỉ khi lenh-a thất bại, tức trả mã thoát khác 0", "correct": true},
       {"text": "Chỉ khi lenh-a thành công", "correct": false},
       {"text": "Luôn luôn, sau khi lenh-a chạy xong", "correct": false},
       {"text": "Song song với lenh-a, ngay lập tức", "correct": false}]'),

    (6, 'Vì sao stderr được tách riêng khỏi stdout?', '', 'choice', '', '',
     '[{"text": "Để lọc hoặc chuyển hướng kết quả mà không nuốt mất thông báo lỗi", "correct": true},
       {"text": "Vì stderr nhanh hơn stdout", "correct": false},
       {"text": "Vì stdout chỉ chứa được văn bản, stderr chứa được nhị phân", "correct": false},
       {"text": "Vì ống dẫn | không hoạt động với một luồng duy nhất", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'linux-lab-6'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- linux-lab-7 -----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Tạo ~/goc.txt rồi tạo liên kết MỀM ~/lien-ket.txt trỏ tới nó',
     'ln -s <đường dẫn gốc> <tên liên kết>. Nên dùng đường dẫn tuyệt đối cho gốc.', 'script',
     $chk$test -L /home/student/lien-ket.txt && test -s /home/student/goc.txt && cmp -s /home/student/lien-ket.txt /home/student/goc.txt$chk$,
     '', '[]'),

    (1, 'Tạo liên kết CỨNG ~/cung.txt tới cùng dữ liệu của ~/goc.txt (cùng số inode)',
     'ln không có cờ -s. Kiểm tra bằng stat -c %i trên cả hai.', 'script',
     $chk$test -f /home/student/cung.txt && test "$(stat -c %i /home/student/goc.txt)" = "$(stat -c %i /home/student/cung.txt)"$chk$,
     '', '[]'),

    (2, 'Tạo ~/du-lieu chứa a.txt và b.txt, cả hai đều có nội dung', '', 'script',
     'test -s /home/student/du-lieu/a.txt && test -s /home/student/du-lieu/b.txt', '', '[]'),

    (3, 'Đóng gói ~/du-lieu thành ~/sao-luu.tar.gz rồi giải nén ra ~/khoi-phuc, nội dung phải khớp',
     'tar -czf ... -C /home/student du-lieu để đóng, tar -xzf ... -C khoi-phuc để mở.', 'script',
     $chk$test -f /home/student/sao-luu.tar.gz && cmp -s /home/student/du-lieu/a.txt /home/student/khoi-phuc/du-lieu/a.txt && cmp -s /home/student/du-lieu/b.txt /home/student/khoi-phuc/du-lieu/b.txt$chk$,
     '', '[]'),

    (4, 'Xem thư mục /etc chiếm bao nhiêu dung lượng, ở dạng dễ đọc', '', 'command', '',
     E'du -sh /etc\ndu -hs /etc', '[]'),

    (5, 'Xoá file gốc thì chuyện gì xảy ra với hai loại liên kết?', '', 'choice', '', '',
     '[{"text": "Liên kết mềm gãy vì nó chỉ giữ đường dẫn; liên kết cứng vẫn dùng được vì nó là tên thứ hai của cùng inode", "correct": true},
       {"text": "Cả hai đều gãy, dữ liệu mất theo file gốc", "correct": false},
       {"text": "Cả hai đều còn dùng được", "correct": false},
       {"text": "Liên kết cứng gãy, liên kết mềm tự trỏ sang bản sao", "correct": false}]'),

    (6, 'Cờ nào của tar dùng để XEM nội dung gói mà chưa giải nén?', '', 'choice', '', '',
     '[{"text": "-t (table of contents)", "correct": true},
       {"text": "-x (extract)", "correct": false},
       {"text": "-c (create)", "correct": false},
       {"text": "-v (verbose)", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'linux-lab-7'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- linux-lab-8 -----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Ghi giá trị biến PATH hiện tại vào ~/path.txt',
     'echo "$PATH" > ~/path.txt — nhớ nháy kép.', 'script',
     $chk$test -s /home/student/path.txt && grep -q "/usr/bin" /home/student/path.txt$chk$,
     '', '[]'),

    (1, 'Viết ~/in-ten.sh in ra giá trị biến TEN, rồi chạy nó với TEN=devforge, kết quả ghi vào ~/ket-qua.txt',
     'Đặt biến chỉ cho một lần chạy: TEN=devforge sh ~/in-ten.sh > ~/ket-qua.txt', 'script',
     $chk$test -f /home/student/in-ten.sh && grep -qx "devforge" /home/student/ket-qua.txt$chk$,
     '', '[]'),

    (2, 'Viết ~/chao-ten.sh in ra tham số thứ nhất, chạy với tham số “xin chao”, ghi ra ~/tham-so.txt',
     'Trong script, $1 là tham số đầu tiên. Nhớ đặt "xin chao" trong nháy kép khi gọi.', 'script',
     $chk$test -f /home/student/chao-ten.sh && grep -qx "xin chao" /home/student/tham-so.txt$chk$,
     '', '[]'),

    (3, 'Tạo lệnh riêng ~/bin/chao (in “chao devforge”), thêm ~/bin vào PATH rồi gọi nó bằng tên trần, ghi ra ~/goi.txt',
     'chmod +x ~/bin/chao, export PATH="$HOME/bin:$PATH", rồi chạy: chao > ~/goi.txt', 'script',
     $chk$test -x /home/student/bin/chao && grep -qx "chao devforge" /home/student/goi.txt$chk$,
     '', '[]'),

    (4, 'Xem lệnh ls đang được lấy từ đường dẫn nào', '', 'command', '',
     E'which ls\ntype ls\ncommand -v ls', '[]'),

    (5, 'PATH là gì?', '', 'choice', '', '',
     '[{"text": "Danh sách thư mục, ngăn bằng dấu hai chấm, shell tìm lệnh lần lượt theo đúng thứ tự đó", "correct": true},
       {"text": "Đường dẫn tới thư mục hiện tại", "correct": false},
       {"text": "Đường dẫn tới thư mục nhà của người dùng", "correct": false},
       {"text": "Danh sách mọi lệnh đã cài trên máy", "correct": false}]'),

    (6, 'Đặt biến rồi chạy script mà script không thấy biến — vì sao?', '', 'choice', '', '',
     '[{"text": "Biến chưa được export nên chỉ tồn tại trong shell hiện tại, tiến trình con không thừa hưởng", "correct": true},
       {"text": "Script phải khai báo lại biến ở dòng đầu tiên", "correct": false},
       {"text": "Biến viết thường không truyền được, phải viết hoa", "correct": false},
       {"text": "Phải khởi động lại shell thì biến mới có hiệu lực", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'linux-lab-8'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- Hai câu bổ sung cho lab 1: cây thư mục chuẩn và cách tự tra cứu — thứ mà
-- người mới thiếu nhất khi rời khỏi bài học đầu tiên.
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, '', 10, v.kind, '', v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (6, 'Thư mục nào chứa file cấu hình của hệ thống?', 'choice', '',
     '[{"text": "/etc", "correct": true},
       {"text": "/var", "correct": false},
       {"text": "/usr", "correct": false},
       {"text": "/tmp", "correct": false}]'),

    (7, 'Tra cứu hướng dẫn sử dụng của lệnh ls ngay trong terminal', 'command',
     E'ls --help\nman ls\nbusybox ls --help', '[]')
) AS v(idx, title, kind, cmds, opts) ON true
WHERE l.slug = 'linux-lab-1'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- Ôn tập ----------------------------------------------------------------
INSERT INTO reviews (course_id, title, content_md, order_idx)
SELECT c.id, v.title, v.body, v.idx
FROM courses c
JOIN (VALUES
    (0, 'Bảng lệnh cần nhớ', $md$## Đi lại và xem

| Lệnh | Việc |
| --- | --- |
| `pwd` | đang ở đâu |
| `ls -la` | liệt kê chi tiết, kể cả file ẩn |
| `cd -` | quay lại thư mục vừa rời |
| `cat` / `head -n 5` / `tail -n 5` | xem cả file / đầu / cuối |

## Tạo và sửa

| Lệnh | Việc |
| --- | --- |
| `mkdir -p a/b` | tạo cả cây thư mục |
| `cp -r a b` | sao chép, `-r` cho thư mục |
| `mv a b` | chuyển chỗ **hoặc** đổi tên |
| `rm -r a` | xoá, không có thùng rác |$md$),

    (1, 'Đọc một chuỗi quyền', $md$```
-rwxr-xr-x  1 student student  42 Jan  1 10:00 hello.sh
│└┬┘└┬┘└┬┘
│ │  │  └── người khác: r-x = 5
│ │  └───── nhóm:       r-x = 5
│ └──────── chủ:        rwx = 7
└────────── kiểu: - file thường, d thư mục, l liên kết
```

`r`=4, `w`=2, `x`=1, cộng lại theo từng nhóm → `755`.

Hay dùng:

- `chmod 755` script chạy được cho mọi người
- `chmod 644` file dữ liệu bình thường
- `chmod 600` file riêng tư, chỉ mình đọc ghi

Không có `x` thì `./script.sh` báo *Permission denied*, nhưng `sh script.sh` vẫn chạy.$md$),

    (2, 'Chuyển hướng và pipe', $md$## Chuyển hướng

- `lệnh > f` — ghi đè kết quả vào file
- `lệnh >> f` — ghi thêm vào cuối
- `lệnh < f` — lấy file làm đầu vào
- `lệnh 2> f` — chuyển riêng thông báo lỗi

## Pipe

`|` nối đầu ra của lệnh trái vào đầu vào của lệnh phải:

```sh
cut -d: -f7 /etc/passwd | sort | uniq -c
#   lấy cột shell      → xếp   → đếm trùng
```

`uniq` chỉ gộp được các dòng **giống nhau và nằm cạnh nhau**, nên gần như lúc
nào cũng phải `sort` trước.$md$),

    (3, 'Cây thư mục chuẩn', $md$Linux nào cũng cùng một bố cục. Biết chỗ nào đựng gì thì đỡ phải đoán:

| Thư mục | Đựng gì |
| --- | --- |
| `/etc` | file cấu hình của hệ thống — toàn văn bản, sửa được bằng editor |
| `/home/<tên>` | thư mục nhà của từng người dùng |
| `/var` | dữ liệu biến động: log (`/var/log`), hàng đợi, cache |
| `/tmp` | file tạm, thường bị dọn khi khởi động lại |
| `/usr/bin` · `/bin` | file thực thi của các lệnh |
| `/dev` | thiết bị dưới dạng file (`/dev/null` là cái thùng rác) |
| `/proc` · `/sys` | không nằm trên đĩa — kernel bày trạng thái ra dưới dạng file |

Hai dòng cuối là chỗ hay bị bất ngờ: `cat /proc/cpuinfo` đọc ra thông tin CPU,
nhưng file đó không tồn tại trên ổ cứng nào cả.

## Tự tra cứu

```sh
ls --help          # nhanh nhất, hầu như lệnh nào cũng có
man ls             # trang hướng dẫn đầy đủ, q để thoát
type cd            # cd là lệnh dựng sẵn của shell, không phải file
```$md$),

    (4, 'Ba luồng và mã thoát', $md$```
        ┌───────────┐
stdin 0 →│   lệnh    │→ 1 stdout   (kết quả)
        └───────────┘→ 2 stderr   (lỗi)
                     ↘ mã thoát: 0 = thành công
```

| Muốn | Viết |
| --- | --- |
| Giữ kết quả | `lenh > f` |
| Giữ lỗi | `lenh 2> f` |
| Giữ cả hai chung một chỗ | `lenh > f 2>&1` |
| Vứt hết, chỉ cần mã thoát | `lenh > /dev/null 2>&1` |
| Vừa xem vừa lưu | `lenh \| tee f` |

**Bẫy thứ tự:** `> f 2>&1` gộp đúng; `2>&1 > f` thì stderr vẫn ra màn hình, vì
lúc nó đi theo stdout thì stdout còn đang trỏ vào màn hình.

## Nối lệnh theo kết quả

```sh
make build && make test      # test chỉ chạy khi build xong xuôi
lenh || echo "hong roi"      # chỉ báo khi lệnh thất bại
```

`$?` là mã thoát của lệnh vừa xong — đọc ngay, vì lệnh kế tiếp sẽ ghi đè nó.$md$)
) AS v(idx, title, body) ON true
WHERE c.slug = 'linux-co-ban'
AND NOT EXISTS (SELECT 1 FROM reviews r WHERE r.course_id = c.id AND r.order_idx = v.idx);

-- ===========================================================================
-- GIT THỰC HÀNH
--
-- Không có mạng trong sandbox, nên mọi remote ở đây là một kho bare nằm ngay
-- trong home. Và vì mỗi lab là một container mới, lab 2 trở đi mở đầu bằng lệnh
-- `kho-mau` dựng lại kho ~/duan — gõ lại ba commit ở đầu mỗi lab không dạy thêm
-- điều gì.
-- ===========================================================================

INSERT INTO labs (course_id, slug, title, description_md, duration_minutes, lab_image_id, order_idx)
SELECT c.id, v.slug, v.title, v.body, v.minutes,
       (SELECT id FROM lab_images WHERE name = 'devforge/git' AND tag = 'latest'), v.idx
FROM courses c
JOIN (VALUES
    ('git-lab-1', 'Khởi Tạo Kho Và Commit Đầu Tiên', 45, 0, $md$# Khởi tạo kho và commit đầu tiên

Git theo dõi thay đổi qua ba khu vực:

```
thư mục làm việc  →  vùng staging (index)  →  kho (.git)
      sửa file          git add                git commit
```

Sửa file thôi thì Git chưa quan tâm. `git add` nói "cái này tôi muốn ghi lại",
`git commit` mới thật sự ghi.

Trước commit đầu tiên Git cần biết bạn là ai:

```sh
git config user.name  "Tên Bạn"
git config user.email "ban@vidu.com"
```

Không có hai dòng đó, `git commit` sẽ từ chối chạy.

> Sandbox **không có mạng**. Mọi thứ trong lab này diễn ra hoàn toàn trên máy.$md$),

    ('git-lab-2', 'Lịch Sử Và Hoàn Tác', 60, 1, $md$# Lịch sử và hoàn tác

Mở đầu bằng `kho-mau` để dựng lại `~/duan` với 3 commit sẵn.

Xem:

- `git log --oneline` — mỗi commit một dòng
- `git show HEAD` — commit gần nhất có gì
- `git diff` — sửa mà **chưa** `git add`; `git diff --staged` — đã add mà chưa commit

Hoàn tác, ba mức khác nhau:

| Lệnh | Hoàn tác cái gì |
| --- | --- |
| `git restore f` | bỏ sửa đổi trong thư mục làm việc |
| `git restore --staged f` | bỏ file khỏi vùng staging, giữ nguyên nội dung |
| `git revert <commit>` | tạo commit **mới** đảo ngược một commit cũ |

`revert` không xoá lịch sử, nó viết thêm — đó là cách an toàn khi commit đã chia
sẻ cho người khác.$md$),

    ('git-lab-3', 'Nhánh Và Gộp', 60, 2, $md$# Nhánh và gộp

Nhánh chỉ là một cái tên trỏ vào một commit. Tạo nhánh rẻ như tạo file.

```sh
git switch -c tinh-nang    # tạo nhánh mới và nhảy sang luôn
git switch main            # quay lại
git merge tinh-nang        # gộp nhánh kia vào nhánh đang đứng
git branch -d tinh-nang    # xoá nhánh đã gộp xong
```

Nếu `main` không có commit nào mới kể từ lúc rẽ nhánh, Git chỉ cần **dời con
trỏ** tới trước — gọi là *fast-forward*, không sinh commit gộp. Ngược lại, khi
hai bên cùng tiến, Git tạo một **commit gộp** có hai cha.

Đụng độ (*conflict*) xảy ra khi hai nhánh sửa **cùng một chỗ** trong cùng một
file. Git dừng lại và để bạn chọn.$md$),

    ('git-lab-4', 'Sửa Lịch Sử', 60, 3, $md$# Sửa lịch sử

Mở đầu bằng `kho-mau`.

| Lệnh | Việc |
| --- | --- |
| `git commit --amend -m "..."` | sửa commit gần nhất (message hoặc nội dung) |
| `git cherry-pick <commit>` | bê **một** commit từ nhánh khác sang đây |
| `git rebase main` | phát lại các commit của nhánh lên trên đầu `main` |

`.gitignore` liệt kê thứ Git phải làm ngơ — file log, thư mục build, file cấu
hình chứa mật khẩu. Một dòng một mẫu, `*.log` khớp mọi file đuôi `.log`.

> **Quy tắc vàng:** chỉ sửa lịch sử **chưa** chia sẻ cho ai. `amend` và `rebase`
> tạo commit mới với mã băm mới; ai đã kéo commit cũ về sẽ thấy lịch sử tách đôi.$md$),

    ('git-lab-5', 'Remote Cục Bộ', 60, 4, $md$# Remote cục bộ

Sandbox không có mạng, nhưng remote của Git **không nhất thiết phải ở trên
mạng** — một đường dẫn thư mục cũng là remote hợp lệ. Đủ để thấy toàn bộ vòng
đời push/clone/pull.

Kho **bare** là kho không có thư mục làm việc, chỉ có dữ liệu. Đó chính là thứ
nằm trên máy chủ GitHub:

```sh
git init --bare ~/remote.git        # dựng "máy chủ"
git remote add origin ~/remote.git  # trỏ kho của bạn vào đó
git push origin main                # đẩy lên
git clone ~/remote.git ~/ban-sao    # người khác kéo về
git pull origin main                # lấy thay đổi mới nhất
```

`origin` chỉ là cái tên mặc định cho remote, không có gì thần thánh.$md$)
) AS v(slug, title, minutes, idx, body) ON true
WHERE c.slug = 'git-thuc-hanh'
ON CONFLICT (slug) DO NOTHING;

-- --- git-lab-1 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Tạo thư mục ~/duan và khởi tạo kho Git trong đó',
     'git init biến một thư mục thường thành kho Git bằng cách tạo thư mục .git bên trong.', 'script',
     'test -d /home/student/duan/.git', '', '[]'),

    (1, 'Khai báo user.name và user.email cho kho',
     'git config user.name "..." — chạy khi đang đứng trong ~/duan.', 'script',
     'git -C /home/student/duan config user.name >/dev/null && git -C /home/student/duan config user.email >/dev/null',
     '', '[]'),

    (2, 'Tạo README.md chứa dòng “# Du an dau tien” rồi commit với message “commit dau tien”',
     'Ba bước: tạo file, git add README.md, rồi git commit -m "commit dau tien".', 'script',
     'git -C /home/student/duan log -1 --pretty=%s | grep -qx "commit dau tien" && git -C /home/student/duan show HEAD:README.md | grep -qx "# Du an dau tien"',
     '', '[]'),

    (3, 'Xem trạng thái hiện tại của kho', '', 'command', '', 'git status', '[]'),

    (4, 'git add làm gì?', '', 'choice', '', '',
     '[{"text": "Đưa thay đổi vào vùng staging, chờ commit ghi lại", "correct": true},
       {"text": "Ghi thẳng thay đổi vào lịch sử kho", "correct": false},
       {"text": "Tải thay đổi lên máy chủ", "correct": false},
       {"text": "Tạo một nhánh mới cho thay đổi", "correct": false}]'),

    (5, 'Một thay đổi đi qua những khu vực nào trước khi vào lịch sử?', '', 'choice', '', '',
     '[{"text": "Thư mục làm việc → vùng staging → kho", "correct": true},
       {"text": "Thư mục làm việc → kho → vùng staging", "correct": false},
       {"text": "Vùng staging → thư mục làm việc → kho", "correct": false},
       {"text": "Thư mục làm việc → kho, không có khu vực nào ở giữa", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'git-lab-1'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- git-lab-2 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Dựng kho mẫu: chạy lệnh kho-mau',
     'Gõ đúng một chữ: kho-mau. Nó tạo lại ~/duan với 3 commit — và xoá ~/duan cũ nếu có.', 'script',
     'test -d /home/student/duan/.git && test "$(git -C /home/student/duan log --oneline | wc -l)" = "3"',
     '', '[]'),

    (1, 'Sửa alpha.txt thành “alpha sua” rồi commit với message “sua alpha”',
     'git commit -am "..." gộp bước add cho các file Git đã theo dõi.', 'script',
     'git -C /home/student/duan log -1 --pretty=%s | grep -qx "sua alpha" && git -C /home/student/duan show HEAD:alpha.txt | grep -qx "alpha sua"',
     '', '[]'),

    (2, 'Tạo rac.txt, git add nó, rồi bỏ nó khỏi vùng staging mà vẫn giữ file trên đĩa',
     'git restore --staged <file> gỡ khỏi staging; file quay lại trạng thái chưa được theo dõi.', 'script',
     'test -f /home/student/duan/rac.txt && git -C /home/student/duan status --porcelain rac.txt | grep -q "^??"',
     '', '[]'),

    (3, 'Hoàn tác commit “them beta” bằng git revert (beta.txt biến mất, lịch sử dài thêm)',
     'git log tìm mã băm của commit đó, rồi git revert <mã> --no-edit.', 'script',
     'test ! -e /home/student/duan/beta.txt && git -C /home/student/duan log -1 --pretty=%s | grep -qi "^revert"',
     '', '[]'),

    (4, 'Xem lịch sử, mỗi commit gọn trên một dòng', '', 'command', '', 'git log --oneline', '[]'),

    (5, 'git diff (không tham số) so sánh cái gì với cái gì?', '', 'choice', '', '',
     '[{"text": "Thư mục làm việc so với vùng staging", "correct": true},
       {"text": "Vùng staging so với commit gần nhất", "correct": false},
       {"text": "Commit gần nhất so với commit trước đó", "correct": false},
       {"text": "Nhánh hiện tại so với nhánh main", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'git-lab-2'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- git-lab-3 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Dựng kho mẫu: chạy lệnh kho-mau', '', 'script',
     'test -d /home/student/duan/.git && test "$(git -C /home/student/duan log --oneline | wc -l)" = "3"',
     '', '[]'),

    (1, 'Tạo nhánh tinh-nang, sang nhánh đó và commit file tinh-nang.txt',
     'git switch -c <tên> vừa tạo vừa nhảy sang nhánh mới.', 'script',
     'git -C /home/student/duan rev-parse --verify tinh-nang >/dev/null 2>&1 && git -C /home/student/duan cat-file -e tinh-nang:tinh-nang.txt',
     '', '[]'),

    (2, 'Quay về nhánh main và gộp tinh-nang vào',
     'Đứng ở nhánh nhận rồi mới merge: git switch main, sau đó git merge tinh-nang.', 'script',
     'test "$(git -C /home/student/duan rev-parse --abbrev-ref HEAD)" = "main" && git -C /home/student/duan branch --merged main | grep -q "tinh-nang"',
     '', '[]'),

    (3, 'Xoá nhánh tinh-nang sau khi đã gộp xong',
     'git branch -d chỉ xoá nhánh đã gộp — đó là cái lưới an toàn của nó.', 'script',
     '! git -C /home/student/duan rev-parse --verify tinh-nang >/dev/null 2>&1', '', '[]'),

    (4, 'Khi nào một lần merge là fast-forward?', '', 'choice', '', '',
     '[{"text": "Khi nhánh nhận không có commit mới nào kể từ lúc rẽ nhánh — Git chỉ dời con trỏ tới trước", "correct": true},
       {"text": "Khi hai nhánh cùng sửa một file", "correct": false},
       {"text": "Khi nhánh kia chỉ có đúng một commit", "correct": false},
       {"text": "Khi merge bằng cờ --no-edit", "correct": false}]'),

    (5, 'Xem lịch sử mọi nhánh dưới dạng đồ thị, mỗi commit một dòng', '', 'command', '',
     E'git log --oneline --graph --all\ngit log --graph --oneline --all\ngit log --all --graph --oneline', '[]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'git-lab-3'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- git-lab-4 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Dựng kho mẫu: chạy lệnh kho-mau', '', 'script',
     'test -d /home/student/duan/.git && test "$(git -C /home/student/duan log --oneline | wc -l)" = "3"',
     '', '[]'),

    (1, 'Sửa message của commit gần nhất thành “them beta va gamma”, số commit giữ nguyên 3',
     'git commit --amend -m "..." viết lại commit gần nhất thay vì tạo commit mới.', 'script',
     'test "$(git -C /home/student/duan log -1 --pretty=%s)" = "them beta va gamma" && test "$(git -C /home/student/duan log --oneline | wc -l)" = "3"',
     '', '[]'),

    (2, 'Tạo .gitignore bỏ qua mọi file .log, rồi tạo test.log — git status không được nhắc tới nó',
     'Một dòng *.log trong .gitignore là đủ.', 'script',
     'test -f /home/student/duan/.gitignore && test -f /home/student/duan/test.log && test -z "$(git -C /home/student/duan status --porcelain test.log)"',
     '', '[]'),

    (3, 'Trên nhánh thu-nghiem tạo commit thêm delta.txt, rồi cherry-pick commit đó sang main',
     'Tạo nhánh và commit ở đó, git switch main, rồi git cherry-pick <mã commit>.', 'script',
     'test "$(git -C /home/student/duan rev-parse --abbrev-ref HEAD)" = "main" && git -C /home/student/duan cat-file -e main:delta.txt && git -C /home/student/duan rev-parse --verify thu-nghiem >/dev/null 2>&1',
     '', '[]'),

    (4, 'Khác nhau cơ bản giữa merge và rebase là gì?', '', 'choice', '', '',
     '[{"text": "merge giữ nguyên hai nhánh và tạo commit gộp; rebase phát lại các commit lên đầu nhánh kia, cho lịch sử thẳng nhưng mã băm đổi", "correct": true},
       {"text": "merge dành cho nhánh cục bộ, rebase dành cho nhánh trên remote", "correct": false},
       {"text": "rebase nhanh hơn vì không phải ghi commit mới", "correct": false},
       {"text": "Không khác nhau, rebase chỉ là tên gọi khác của merge", "correct": false}]'),

    (5, 'Khi nào tuyệt đối không nên amend hay rebase?', '', 'choice', '', '',
     '[{"text": "Khi những commit đó đã được đẩy lên và người khác đã kéo về", "correct": true},
       {"text": "Khi nhánh có nhiều hơn ba commit", "correct": false},
       {"text": "Khi đang đứng trên nhánh main", "correct": false},
       {"text": "Khi trong kho có file .gitignore", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'git-lab-4'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- git-lab-5 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Dựng kho mẫu: chạy lệnh kho-mau', '', 'script',
     'test -d /home/student/duan/.git && test "$(git -C /home/student/duan log --oneline | wc -l)" = "3"',
     '', '[]'),

    (1, 'Dựng một kho bare tại ~/remote.git để đóng vai máy chủ',
     'git init --bare <đường dẫn>. Kho bare không có thư mục làm việc, chỉ có dữ liệu.', 'script',
     'test "$(git -C /home/student/remote.git rev-parse --is-bare-repository 2>/dev/null)" = "true"',
     '', '[]'),

    (2, 'Trong ~/duan thêm remote tên origin trỏ tới ~/remote.git rồi push nhánh main lên',
     'git remote add origin /home/student/remote.git, sau đó git push origin main.', 'script',
     'test "$(git -C /home/student/remote.git rev-parse main 2>/dev/null)" = "$(git -C /home/student/duan rev-parse main)"',
     '', '[]'),

    (3, 'Clone ~/remote.git thành ~/ban-sao, đóng vai người thứ hai kéo dự án về',
     'git clone <nguồn> <đích>.', 'script',
     'test -d /home/student/ban-sao/.git && test "$(git -C /home/student/ban-sao log --oneline | wc -l)" = "3"',
     '', '[]'),

    (4, 'Trong ~/ban-sao commit thêm gamma.txt và push, rồi từ ~/duan pull về — hai kho phải trỏ cùng một commit',
     'Push từ ban-sao, sau đó về ~/duan chạy git pull origin main.', 'script',
     'test -f /home/student/duan/gamma.txt && test "$(git -C /home/student/duan rev-parse main)" = "$(git -C /home/student/ban-sao rev-parse main)"',
     '', '[]'),

    (5, 'origin/main khác main ở chỗ nào?', '', 'choice', '', '',
     '[{"text": "main là nhánh của bạn; origin/main là ghi nhớ vị trí nhánh main trên remote lần cuối bạn liên lạc với nó", "correct": true},
       {"text": "Hai cái là một, origin/ chỉ là tiền tố cho đẹp", "correct": false},
       {"text": "origin/main luôn mới hơn main", "correct": false},
       {"text": "main nằm trên máy chủ, origin/main nằm ở máy bạn", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'git-lab-5'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- Ôn tập ----------------------------------------------------------------
INSERT INTO reviews (course_id, title, content_md, order_idx)
SELECT c.id, v.title, v.body, v.idx
FROM courses c
JOIN (VALUES
    (0, 'Vòng đời một thay đổi', $md$```
   sửa file          git add           git commit
thư mục làm việc ──→ vùng staging ──→ lịch sử kho
      ↑                   │                │
      └── git restore ────┘                │
      └───────── git revert ───────────────┘
```

| Câu hỏi | Lệnh |
| --- | --- |
| Đang có gì thay đổi? | `git status` |
| Thay đổi cụ thể ra sao? | `git diff` (chưa add) · `git diff --staged` (đã add) |
| Lịch sử thế nào? | `git log --oneline --graph --all` |
| Commit này sửa gì? | `git show <mã>` |$md$),

    (1, 'Hoàn tác — chọn đúng mức', $md$| Muốn | Lệnh | Có đổi lịch sử không |
| --- | --- | --- |
| Bỏ sửa đổi chưa add | `git restore f` | không |
| Gỡ khỏi staging, giữ nội dung | `git restore --staged f` | không |
| Đảo ngược một commit đã có | `git revert <mã>` | không — nó **thêm** commit mới |
| Sửa commit gần nhất | `git commit --amend` | **có** |
| Vứt hẳn commit gần nhất | `git reset --hard HEAD~1` | **có**, và mất luôn thay đổi |

Cột cuối là cột quan trọng. Hai dòng cuối viết lại mã băm, nên chỉ dùng khi
commit đó **chưa** chia sẻ cho ai.$md$),

    (2, 'Nhánh và remote', $md$## Nhánh

```sh
git switch -c tinh-nang     # tạo và sang nhánh mới
git switch main             # quay lại
git merge tinh-nang         # gộp vào nhánh đang đứng
git branch -d tinh-nang     # xoá nhánh đã gộp (-D để ép xoá)
```

Nhánh chỉ là con trỏ tới một commit. Xoá nhánh không xoá commit.

## Remote

```sh
git remote -v                       # đang trỏ đi đâu
git push origin main                # đẩy nhánh main lên remote tên origin
git pull origin main                # kéo về và gộp luôn
git clone <nguồn> <đích>            # lấy toàn bộ kho về
```

`origin` chỉ là cái tên. Một kho có thể có nhiều remote với tên khác nhau.$md$)
) AS v(idx, title, body) ON true
WHERE c.slug = 'git-thuc-hanh'
AND NOT EXISTS (SELECT 1 FROM reviews r WHERE r.course_id = c.id AND r.order_idx = v.idx);

-- ===========================================================================
-- DOCKER NHẬP MÔN
--
-- Không có daemon trong sandbox và không thể có: không mạng, không capability,
-- rootfs read-only — chính những thứ khiến việc đưa shell cho người lạ là an
-- toàn. Nên khoá này chấm bằng thứ không cần daemon: file học viên viết
-- (Dockerfile, .dockerignore, compose.yml), lệnh họ gõ, và lý thuyết.
-- ===========================================================================

INSERT INTO labs (course_id, slug, title, description_md, duration_minutes, lab_image_id, order_idx)
SELECT c.id, v.slug, v.title, v.body, v.minutes,
       (SELECT id FROM lab_images WHERE name = 'devforge/docker' AND tag = 'latest'), v.idx
FROM courses c
JOIN (VALUES
    ('docker-lab-1', 'Image, Container Và Registry', 45, 0, $md$# Image, container và registry

**Image** là bản đóng gói bất biến: hệ thống tệp + siêu dữ liệu về cách chạy.
**Container** là một tiến trình chạy từ image đó, kèm một lớp ghi mỏng bên trên.
Một image sinh ra được trăm container; xoá container không đụng gì tới image.

**Registry** là nơi chứa image. `docker run nginx` thật ra là
`docker run docker.io/library/nginx:latest` — Docker Hub, kho `library`, thẻ
`latest`.

`latest` **không** có nghĩa là mới nhất. Nó chỉ là thẻ mặc định khi bạn không
ghi thẻ nào, và người phát hành có thể trỏ nó đi đâu tuỳ ý.

> **Sandbox này không có daemon Docker** — không mạng, không đặc quyền. Gõ
> `docker ...` sẽ báo lỗi và **vẫn được chấm**: bài kiểu “gõ lệnh” chấm theo
> lịch sử shell, còn bài kiểu “viết file” chấm theo Dockerfile bạn soạn.$md$),

    ('docker-lab-2', 'Dockerfile Đầu Tiên', 60, 1, $md$# Dockerfile đầu tiên

Mỗi dòng là một chỉ thị:

```dockerfile
FROM alpine:3.21          # nền
WORKDIR /app              # thư mục làm việc bên trong
COPY app.sh .             # chép file từ máy build vào image
RUN apk add --no-cache bash   # chạy lệnh lúc build
CMD ["sh", "app.sh"]      # chạy gì khi container khởi động
```

Ba thói quen tốt ngay từ đầu:

1. **Ghim thẻ cụ thể** — `alpine:3.21`, đừng `alpine`. `latest` đổi dưới chân bạn
   và build hôm nay khác build hôm qua.
2. **`--no-cache` khi cài gói** — không để lại chỉ mục gói trong layer.
3. **`CMD` dạng danh sách** — `["sh", "app.sh"]` chạy thẳng, không qua shell.

Soạn file trong `~/app` bằng `nano`.$md$),

    ('docker-lab-3', 'Layer, Cache Và Multi-stage', 60, 2, $md$# Layer, cache và multi-stage

Mỗi `FROM`, `RUN`, `COPY`, `ADD` tạo một **layer**. Build lại thì Docker dùng lại
layer cũ cho tới dòng đầu tiên có thay đổi — từ đó trở xuống build lại hết.

Hệ quả về thứ tự: **thứ ít đổi để trên, thứ hay đổi để dưới**.

```dockerfile
COPY go.mod go.sum ./     # hiếm khi đổi → cache còn dùng được
RUN go mod download       # bước đắt, nhờ trên mà khỏi chạy lại
COPY . .                  # đổi mỗi lần sửa code
```

Đảo hai cái đó lại thì mỗi lần sửa một dòng code là tải lại toàn bộ thư viện.

**Multi-stage** — build ở một stage, chỉ bê kết quả sang stage cuối:

```dockerfile
FROM golang:1.25 AS build
WORKDIR /src
COPY . .
RUN go build -o /app

FROM alpine:3.21
COPY --from=build /app /app
CMD ["/app"]
```

Image cuối không chứa trình biên dịch, không chứa mã nguồn. Vài chục MB thay vì
vài trăm.

`.dockerignore` giữ những thứ không nên vào build context: `.git`,
`node_modules`, file bí mật.$md$),

    ('docker-lab-4', 'Volume, Cổng Và Network', 60, 3, $md$# Volume, cổng và network

**Dữ liệu.** Lớp ghi của container chết cùng container.

- **Bind mount** `-v /duong/dan/host:/trong/container` — thư mục thật của máy,
  hợp cho code lúc dev.
- **Named volume** `-v du-lieu:/var/lib/postgresql/data` — Docker quản lý, hợp
  cho dữ liệu production.

**Cổng.** `-p 8080:80` đọc là **host:container** — gõ `localhost:8080` ở máy bạn
thì gói tin đi vào cổng 80 trong container. Nhớ sai chiều là lỗi phổ biến nhất.

**Network.**

| Chế độ | Nghĩa |
| --- | --- |
| `bridge` | mặc định, container có IP riêng, ra ngoài qua NAT |
| `host` | dùng thẳng network của máy, `-p` vô nghĩa |
| `none` | không có mạng — **chính là sandbox bạn đang ngồi** |

Container trong cùng một network do người dùng tạo gọi được nhau **bằng tên**.$md$),

    ('docker-lab-5', 'Docker Compose', 60, 4, $md$# Docker Compose

Một ứng dụng thật hiếm khi chỉ có một container. Compose mô tả cả cụm trong một
file YAML, thay cho một xâu lệnh `docker run` dài dằng dặc.

```yaml
services:
  web:
    build: .
    ports:
      - "8080:80"
    depends_on:
      - db
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: matkhau
    volumes:
      - du-lieu:/var/lib/postgresql/data

volumes:
  du-lieu:
```

`docker compose up -d` dựng tất cả; `docker compose down` dẹp tất cả.

`depends_on` chỉ đảm bảo **thứ tự khởi động**, không đảm bảo dịch vụ kia đã sẵn
sàng nhận kết nối — muốn thế phải có `healthcheck`.

YAML dùng **thụt lề bằng space**, không bao giờ dùng tab.$md$)
) AS v(slug, title, minutes, idx, body) ON true
WHERE c.slug = 'docker-nhap-mon'
ON CONFLICT (slug) DO NOTHING;

-- --- docker-lab-1 ----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Hỏi phiên bản Docker đang cài', 'Sandbox sẽ báo lỗi — lệnh vẫn được ghi nhận và vẫn được chấm.', 'command', '',
     E'docker --version\ndocker version', '[]'),

    (1, 'Liệt kê mọi image đang có trên máy', '', 'command', '',
     E'docker images\ndocker image ls', '[]'),

    (2, 'Liệt kê mọi container, kể cả container đã dừng', '', 'command', '',
     E'docker ps -a\ndocker ps --all\ndocker container ls -a', '[]'),

    (3, 'Image và container khác nhau thế nào?', '', 'choice', '', '',
     '[{"text": "Image là bản đóng gói bất biến; container là một tiến trình chạy từ image đó, kèm lớp ghi riêng", "correct": true},
       {"text": "Image đang chạy, container thì đã dừng", "correct": false},
       {"text": "Container là image đã được nén lại để đẩy lên registry", "correct": false},
       {"text": "Mỗi image chỉ tạo được đúng một container", "correct": false}]'),

    (4, 'Thẻ latest nghĩa là gì?', '', 'choice', '', '',
     '[{"text": "Chỉ là thẻ mặc định khi không ghi thẻ nào — không đảm bảo đó là bản mới nhất", "correct": true},
       {"text": "Luôn là bản phát hành mới nhất của image", "correct": false},
       {"text": "Bản ổn định đã qua kiểm thử của nhà phát hành", "correct": false},
       {"text": "Bản nhẹ nhất trong các thẻ của image", "correct": false}]'),

    (5, 'Chạy docker run alpine khi máy chưa có image alpine thì điều gì xảy ra?', '', 'choice', '', '',
     '[{"text": "Docker tự kéo image từ registry rồi mới tạo và chạy container", "correct": true},
       {"text": "Báo lỗi, phải docker pull thủ công trước", "correct": false},
       {"text": "Tạo một image rỗng tên alpine", "correct": false},
       {"text": "Chạy được nhưng container không có hệ thống tệp", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'docker-lab-1'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- docker-lab-2 ----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Tạo ~/app/app.sh và ~/app/Dockerfile có đủ FROM alpine, WORKDIR /app, COPY và CMD',
     'Soạn bằng nano ~/app/Dockerfile. Bốn chỉ thị, mỗi thứ một dòng.', 'script',
     $chk$f=/home/student/app/Dockerfile; test -f /home/student/app/app.sh && test -f "$f" && grep -qiE "^[[:space:]]*FROM[[:space:]]+alpine" "$f" && grep -qiE "^[[:space:]]*WORKDIR[[:space:]]+/app" "$f" && grep -qiE "^[[:space:]]*COPY[[:space:]]+" "$f" && grep -qiE "^[[:space:]]*CMD[[:space:]]+" "$f"$chk$,
     '', '[]'),

    (1, 'Ghim thẻ cụ thể cho image nền: FROM alpine:3.21 thay vì alpine trần',
     'Sửa đúng dòng FROM, thêm dấu hai chấm và số phiên bản.', 'script',
     $chk$grep -qiE "^[[:space:]]*FROM[[:space:]]+alpine:3\.[0-9]+" /home/student/app/Dockerfile$chk$,
     '', '[]'),

    (2, 'Thêm một dòng RUN cài gói bằng apk kèm cờ --no-cache',
     'RUN apk add --no-cache <tên gói>. Cờ --no-cache để không đọng chỉ mục gói trong layer.', 'script',
     $chk$grep -qiE "^[[:space:]]*RUN[[:space:]]+apk[[:space:]]+add[[:space:]].*--no-cache" /home/student/app/Dockerfile$chk$,
     '', '[]'),

    (3, 'Build image từ thư mục hiện tại, đặt tên myapp', '', 'command', '',
     E'docker build -t myapp .\ndocker build --tag myapp .', '[]'),

    (4, 'CMD khác ENTRYPOINT ở chỗ nào?', '', 'choice', '', '',
     '[{"text": "CMD là lệnh mặc định, bị thay hoàn toàn khi docker run truyền lệnh khác; ENTRYPOINT luôn chạy, phần truyền vào trở thành đối số của nó", "correct": true},
       {"text": "CMD chạy lúc build, ENTRYPOINT chạy lúc khởi động container", "correct": false},
       {"text": "ENTRYPOINT chỉ dùng được một lần, CMD viết bao nhiêu dòng cũng được", "correct": false},
       {"text": "Không khác nhau, ENTRYPOINT là tên cũ của CMD", "correct": false}]'),

    (5, 'Vì sao nên dùng COPY thay vì ADD?', '', 'choice', '', '',
     '[{"text": "COPY chỉ chép file, làm đúng một việc rõ ràng; ADD còn tự giải nén tar và tải URL — những hành vi ngầm dễ gây bất ngờ", "correct": true},
       {"text": "ADD đã bị gỡ khỏi Docker", "correct": false},
       {"text": "COPY nhanh hơn ADD vì không nén", "correct": false},
       {"text": "ADD không tạo layer mới nên khó gỡ lỗi", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'docker-lab-2'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- docker-lab-3 ----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Viết ~/app/Dockerfile.multi dạng multi-stage: từ hai FROM trở lên và có COPY --from=',
     'Stage đầu đặt tên bằng AS build, stage cuối bê kết quả sang bằng COPY --from=build.', 'script',
     $chk$f=/home/student/app/Dockerfile.multi; test -f "$f" && test "$(grep -ciE "^[[:space:]]*FROM[[:space:]]" "$f")" -ge 2 && grep -qiE "^[[:space:]]*COPY[[:space:]]+--from=" "$f"$chk$,
     '', '[]'),

    (1, 'Viết ~/app/Dockerfile.cache đặt COPY go.mod TRƯỚC dòng COPY . — để cache còn dùng được',
     'Thứ ít đổi để trên, thứ hay đổi để dưới. Dòng COPY go.mod phải nằm ở số dòng nhỏ hơn dòng COPY . .', 'script',
     $chk$f=/home/student/app/Dockerfile.cache; test -f "$f" || exit 1; a=$(grep -niE "^[[:space:]]*COPY[[:space:]]+go\.mod" "$f" | head -1 | cut -d: -f1); b=$(grep -niE "^[[:space:]]*COPY[[:space:]]+\.[[:space:]]" "$f" | head -1 | cut -d: -f1); test -n "$a" && test -n "$b" && test "$a" -lt "$b"$chk$,
     '', '[]'),

    (2, 'Tạo ~/app/.dockerignore loại .git và node_modules khỏi build context',
     'Mỗi mẫu một dòng, viết đúng tên thư mục.', 'script',
     $chk$f=/home/student/app/.dockerignore; test -f "$f" && grep -qE "^[[:space:]]*\.git[/[:space:]]*$" "$f" && grep -qE "^[[:space:]]*node_modules[/[:space:]]*$" "$f"$chk$,
     '', '[]'),

    (3, 'Build lại nhưng bỏ qua toàn bộ cache', '', 'command', '',
     E'docker build --no-cache -t myapp .\ndocker build --no-cache .', '[]'),

    (4, 'Vì sao gộp nhiều lệnh vào một RUN bằng && lại làm image nhỏ hơn?', '', 'choice', '', '',
     '[{"text": "Mỗi RUN là một layer, và file đã nằm trong layer trước thì layer sau xoá đi vẫn không lấy lại được dung lượng", "correct": true},
       {"text": "Docker chỉ chạy được tối đa 10 lệnh RUN mỗi Dockerfile", "correct": false},
       {"text": "&& khiến lệnh chạy song song nên nhanh và nhẹ hơn", "correct": false},
       {"text": "Gộp lại thì Docker nén layer bằng thuật toán tốt hơn", "correct": false}]'),

    (5, 'Sửa một dòng code rồi build lại — Docker chạy lại từ đâu?', '', 'choice', '', '',
     '[{"text": "Từ chỉ thị đầu tiên bị ảnh hưởng bởi thay đổi, và mọi chỉ thị nằm dưới nó", "correct": true},
       {"text": "Từ đầu Dockerfile, luôn luôn", "correct": false},
       {"text": "Chỉ chạy lại đúng dòng bị đổi", "correct": false},
       {"text": "Không chạy lại gì cho tới khi dùng --no-cache", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'docker-lab-3'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- docker-lab-4 ----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Chạy nền container nginx tên web, mở cổng 8080 của máy vào cổng 80 của container', '', 'command', '',
     E'docker run -d -p 8080:80 --name web nginx\ndocker run --detach --publish 8080:80 --name web nginx', '[]'),

    (1, 'Chạy container alpine gắn named volume du-lieu vào /data', '', 'command', '',
     E'docker run -v du-lieu:/data alpine\ndocker run --volume du-lieu:/data alpine\ndocker run -it -v du-lieu:/data alpine', '[]'),

    (2, 'Ghi vào ~/lenh-chay.txt đúng một dòng lệnh chạy nền nginx, tên web, ánh xạ 8080:80',
     'Dòng đó phải có đủ: docker run, -d, -p 8080:80, --name web, và tên image nginx.', 'script',
     $chk$f=/home/student/lenh-chay.txt; test -f "$f" && grep -q "docker run" "$f" && grep -qE "(^| )-d( |$)|--detach" "$f" && grep -qE "(-p|--publish)[[:space:]]+8080:80" "$f" && grep -qE "\-\-name[[:space:]]+web" "$f" && grep -q "nginx" "$f"$chk$,
     '', '[]'),

    (3, 'Trong -p 8080:80 thì số nào là cổng của máy chủ?', '', 'choice', '', '',
     '[{"text": "8080 — dạng viết là host:container", "correct": true},
       {"text": "80 — dạng viết là container:host", "correct": false},
       {"text": "Cả hai đều là cổng trong container", "correct": false},
       {"text": "Tuỳ chế độ network đang dùng", "correct": false}]'),

    (4, 'Bind mount khác named volume ở chỗ nào?', '', 'choice', '', '',
     '[{"text": "Bind mount trỏ vào một đường dẫn có thật trên máy chủ; named volume do Docker tạo và quản lý ở khu vực riêng của nó", "correct": true},
       {"text": "Bind mount chỉ đọc, named volume đọc ghi được", "correct": false},
       {"text": "Named volume mất khi container dừng, bind mount thì còn", "correct": false},
       {"text": "Bind mount chỉ dùng được với Linux", "correct": false}]'),

    (5, 'Container chạy với --network host thì cờ -p có tác dụng gì?', '', 'choice', '', '',
     '[{"text": "Không có tác dụng — container dùng thẳng network của máy chủ nên không có gì để ánh xạ", "correct": true},
       {"text": "Vẫn ánh xạ như bình thường", "correct": false},
       {"text": "Ánh xạ ngược lại, từ container ra máy chủ", "correct": false},
       {"text": "Docker báo lỗi và không cho chạy", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'docker-lab-4'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- docker-lab-5 ----------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Tạo ~/app/docker-compose.yml khai báo services với hai dịch vụ: web và db',
     'YAML thụt lề bằng space. services: ở cột 0, tên dịch vụ thụt vào hai space.', 'script',
     $chk$f=/home/student/app/docker-compose.yml; test -f "$f" && grep -qE "^services:" "$f" && grep -qE "^[[:space:]]+web:" "$f" && grep -qE "^[[:space:]]+db:" "$f"$chk$,
     '', '[]'),

    (1, 'Cho web mở cổng 8080:80 và phụ thuộc db bằng depends_on',
     'ports: là một danh sách, mỗi phần tử một dòng bắt đầu bằng dấu gạch ngang.', 'script',
     $chk$f=/home/student/app/docker-compose.yml; grep -qE "^[[:space:]]+ports:" "$f" && grep -qE "^[[:space:]]+-[[:space:]]+\"?8080:80\"?" "$f" && grep -qE "^[[:space:]]+depends_on:" "$f"$chk$,
     '', '[]'),

    (2, 'Cho db dùng image postgres và đặt biến môi trường POSTGRES_PASSWORD',
     'image: postgres:16-alpine, rồi environment: với biến bên dưới.', 'script',
     $chk$f=/home/student/app/docker-compose.yml; grep -qE "^[[:space:]]+image:[[:space:]]+postgres" "$f" && grep -qE "^[[:space:]]+environment:" "$f" && grep -q "POSTGRES_PASSWORD" "$f"$chk$,
     '', '[]'),

    (3, 'Dựng toàn bộ cụm dịch vụ ở chế độ nền', '', 'command', '',
     E'docker compose up -d\ndocker-compose up -d\ndocker compose up --detach', '[]'),

    (4, 'depends_on đảm bảo điều gì?', '', 'choice', '', '',
     '[{"text": "Chỉ đảm bảo thứ tự khởi động, không đảm bảo dịch vụ kia đã sẵn sàng nhận kết nối", "correct": true},
       {"text": "Đảm bảo dịch vụ kia đã sẵn sàng nhận kết nối rồi mới khởi động dịch vụ này", "correct": false},
       {"text": "Đảm bảo hai dịch vụ nằm cùng một máy chủ", "correct": false},
       {"text": "Đảm bảo dịch vụ kia khởi động lại khi dịch vụ này lỗi", "correct": false}]'),

    (5, 'Hai container trong cùng một network do Compose tạo gọi nhau bằng gì?', '', 'choice', '', '',
     '[{"text": "Bằng tên dịch vụ — Compose dựng sẵn DNS nội bộ cho network đó", "correct": true},
       {"text": "Bằng địa chỉ IP, phải tra thủ công mỗi lần khởi động", "correct": false},
       {"text": "Bằng localhost, vì chung một network", "correct": false},
       {"text": "Không gọi nhau được nếu chưa mở cổng bằng -p", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'docker-lab-5'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- Ôn tập ----------------------------------------------------------------
INSERT INTO reviews (course_id, title, content_md, order_idx)
SELECT c.id, v.title, v.body, v.idx
FROM courses c
JOIN (VALUES
    (0, 'Chỉ thị Dockerfile hay dùng', $md$| Chỉ thị | Việc | Bẫy thường gặp |
| --- | --- | --- |
| `FROM` | chọn image nền | để `latest` → build đổi dưới chân bạn |
| `WORKDIR` | đặt thư mục làm việc | dùng `RUN cd ...` thay thế — không có tác dụng sang dòng sau |
| `COPY` | chép file vào image | dùng `ADD` cho việc mà `COPY` làm đủ |
| `RUN` | chạy lệnh lúc build | mỗi `RUN` một layer; xoá file ở layer sau không lấy lại được dung lượng |
| `CMD` | lệnh mặc định khi chạy | viết dạng chuỗi thay vì danh sách |
| `ENTRYPOINT` | lệnh luôn chạy | nhầm với `CMD` |
| `EXPOSE` | ghi chú cổng | tưởng nó tự mở cổng — không, phải `-p` |$md$),

    (1, 'Cache và kích thước image', $md$## Thứ tự quyết định tốc độ

```dockerfile
COPY package.json .        # ít đổi  → để trên
RUN npm ci                 # đắt     → nhờ dòng trên mà khỏi chạy lại
COPY . .                   # hay đổi → để dưới
```

Đảo lại thì mỗi lần sửa một dòng code là cài lại toàn bộ thư viện.

## Ba cách giảm kích thước

1. **Multi-stage** — build một stage, stage cuối chỉ nhận sản phẩm
2. **Image nền nhỏ** — `alpine`, `distroless` thay vì `ubuntu`
3. **Gộp RUN + dọn trong cùng layer** — `apk add --no-cache`,
   `rm -rf /var/lib/apt/lists/*` phải nằm **cùng** `RUN` với lệnh cài$md$),

    (2, 'Từ docker run sang compose', $md$Cùng một việc, hai cách viết:

```sh
docker network create app-net
docker volume create du-lieu
docker run -d --name db --network app-net \
  -e POSTGRES_PASSWORD=matkhau -v du-lieu:/var/lib/postgresql/data postgres:16-alpine
docker run -d --name web --network app-net -p 8080:80 myapp
```

```yaml
services:
  web:
    image: myapp
    ports: ["8080:80"]
    depends_on: [db]
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: matkhau
    volumes: ["du-lieu:/var/lib/postgresql/data"]
volumes:
  du-lieu:
```

Compose tự tạo network riêng cho cụm, nên `web` gọi `db` bằng đúng chữ `db`.$md$)
) AS v(idx, title, body) ON true
WHERE c.slug = 'docker-nhap-mon'
AND NOT EXISTS (SELECT 1 FROM reviews r WHERE r.course_id = c.id AND r.order_idx = v.idx);

-- ===========================================================================
-- MẠNG MÁY TÍNH THỰC HÀNH
--
-- Sandbox chạy với `--network none`, nên chỉ có một giao diện: loopback. Đó là
-- ít hơn nghe tưởng: một socket gắn vào 127.0.0.1 vẫn là socket thật, cổng
-- thật, bắt tay TCP thật — client/server, cổng đang lắng nghe và HTTP đều học
-- được. Ngoài tầm với: máy thứ hai, ICMP (ping cần CAP_NET_RAW, đã bỏ hết
-- capability), DNS, và mọi tuyến đi xa hơn `lo`. Những phần đó dạy bằng lý
-- thuyết và bằng lệnh đã gõ.
-- ===========================================================================

INSERT INTO labs (course_id, slug, title, description_md, duration_minutes, lab_image_id, order_idx)
SELECT c.id, v.slug, v.title, v.body, v.minutes,
       (SELECT id FROM lab_images WHERE name = 'devforge/net' AND tag = 'latest'), v.idx
FROM courses c
JOIN (VALUES
    ('net-lab-1', 'Giao Diện Mạng Và Địa Chỉ IP', 45, 0, $md$# Giao diện mạng và địa chỉ IP

Máy nói chuyện qua **giao diện mạng**. Xem bằng:

```sh
ip addr           # địa chỉ của từng giao diện
ip -o -4 addr     # gọn một dòng mỗi giao diện, chỉ IPv4
ip link           # trạng thái lên/xuống
ip route          # đi ra ngoài theo đường nào
```

Sandbox này chỉ có `lo` — **loopback**, `127.0.0.1/8`. Mọi máy Linux đều có nó,
và nó không đi ra khỏi máy: gói tin gửi tới `127.0.0.1` quay ngược vào chính
máy đó. Đủ để một tiến trình nói chuyện với tiến trình khác qua TCP thật.

Ký hiệu `/8` là **độ dài tiền tố**: 8 bit đầu là phần mạng.

Ba dải dành riêng cho mạng nội bộ, không định tuyến ra Internet:

| Dải | CIDR |
| --- | --- |
| 10.0.0.0 – 10.255.255.255 | `10.0.0.0/8` |
| 172.16.0.0 – 172.31.255.255 | `172.16.0.0/12` |
| 192.168.0.0 – 192.168.255.255 | `192.168.0.0/16` |$md$),

    ('net-lab-2', 'Cổng Và Socket Đang Lắng Nghe', 60, 1, $md$# Cổng và socket đang lắng nghe

Địa chỉ IP đưa gói tin tới đúng **máy**. **Cổng** đưa nó tới đúng **tiến trình**.

Một socket đang phục vụ được xác định bởi bộ ba: giao thức + địa chỉ + cổng.

```sh
ss -ltn      # l = listening, t = TCP, n = giữ nguyên số, đừng dịch tên
ss -ltnp     # thêm p = tiến trình nào đang giữ
nc -l -p 8080   # tự mở một cổng để nhìn
nc -z 127.0.0.1 8080   # thử xem cổng có ai nghe không
```

Cổng **dưới 1024** là cổng đặc quyền — chỉ root mới gắn vào được. Vì thế web
server chạy dev hay dùng 8080 chứ không phải 80.

| Cổng | Dịch vụ |
| --- | --- |
| 22 | SSH |
| 53 | DNS |
| 80 | HTTP |
| 443 | HTTPS |
| 5432 | PostgreSQL |$md$),

    ('net-lab-3', 'Client – Server Với Netcat', 60, 2, $md$# Client – server với netcat

`nc` là con dao Thuỵ Sĩ của TCP: một bên lắng nghe, một bên gọi tới.

```sh
nc -l -p 9000 > nhan.txt   # phía server: nghe cổng 9000, ghi những gì nhận được
echo "xin chao" | nc 127.0.0.1 9000   # phía client: gửi rồi đóng
```

Đây là **TCP thật**: bắt tay ba bước (SYN → SYN/ACK → ACK), truyền dữ liệu, rồi
đóng. Loopback không làm nó bớt thật, chỉ làm nó nhanh.

**TCP** đảm bảo tới nơi, đúng thứ tự, có phát lại khi mất — trả giá bằng độ trễ.
**UDP** bắn đi là xong, không đảm bảo gì — được cái nhanh và nhẹ, hợp cho gọi
thoại, game, DNS.

> Dấu `&` để đẩy phía server xuống nền, nếu không shell sẽ đứng đó chờ.$md$),

    ('net-lab-4', 'HTTP Tận Mắt', 60, 3, $md$# HTTP tận mắt

HTTP là văn bản thuần trên nền TCP. Một yêu cầu gồm: **method + đường dẫn +
phiên bản**, các **header**, rồi (có thể) **body**.

Dựng một web server thật trong sandbox:

```sh
mkdir -p ~/web && echo "<h1>chao</h1>" > ~/web/index.html
httpd -p 8080 -h ~/web          # busybox httpd, chạy nền sẵn
curl http://127.0.0.1:8080/     # lấy nội dung
curl -I http://127.0.0.1:8080/  # chỉ lấy header phản hồi
curl -v http://127.0.0.1:8080/  # xem cả hai chiều
```

Mã trạng thái đọc theo chữ số đầu:

| Nhóm | Nghĩa | Ví dụ |
| --- | --- | --- |
| 2xx | thành công | 200 OK |
| 3xx | chuyển hướng | 301 chuyển vĩnh viễn |
| 4xx | lỗi phía client | 404 không có, 403 cấm |
| 5xx | lỗi phía server | 500 nội bộ |

`curl -o /dev/null -w "%{http_code}"` chỉ in ra đúng mã — tiện để ghi vào file.$md$),

    ('net-lab-5', 'Subnet Và CIDR', 60, 4, $md$# Subnet và CIDR

`192.168.1.0/26` đọc là: 26 bit đầu cố định (phần mạng), 6 bit còn lại tự do
(phần host).

```
tổng địa chỉ    = 2^(32-26) = 2^6 = 64
địa chỉ mạng    = 192.168.1.0      (bit host toàn 0)
địa chỉ quảng bá = 192.168.1.63    (bit host toàn 1)
host dùng được  = 64 - 2 = 62      (trừ hai địa chỉ trên)
```

Công thức chung với tiền tố `/n`: `2^(32-n)` địa chỉ, `2^(32-n) - 2` host dùng
được.

| CIDR | Mặt nạ | Host dùng được |
| --- | --- | --- |
| `/24` | 255.255.255.0 | 254 |
| `/25` | 255.255.255.128 | 126 |
| `/26` | 255.255.255.192 | 62 |
| `/30` | 255.255.255.252 | 2 |

Chia mạng con để làm gì: gom máy theo vai trò, chặn giữa các nhóm bằng firewall,
và giữ miền quảng bá đủ nhỏ.$md$),

    ('net-lab-6', 'Mô Hình Phân Tầng', 45, 5, $md$# Mô hình phân tầng

Không ai thiết kế mạng thành một khối. Mỗi tầng chỉ lo đúng việc của mình và
nhận từ tầng dưới một dịch vụ đã sẵn sàng.

| TCP/IP | Đơn vị | Địa chỉ | Ví dụ |
| --- | --- | --- | --- |
| Ứng dụng | dữ liệu | tên miền / URL | HTTP, DNS, SSH |
| Giao vận | segment | **cổng** | TCP, UDP |
| Internet | packet | **địa chỉ IP** | IP, ICMP |
| Liên kết | frame | **địa chỉ MAC** | Ethernet, Wi-Fi |

**Đóng gói** (encapsulation): dữ liệu của bạn đi xuống, mỗi tầng bọc thêm một
lớp header:

```
[ Ethernet [ IP [ TCP [ HTTP: GET / ] ] ] ]
   MAC        IP    cổng    dữ liệu thật
```

Bên nhận bóc ngược lại từ ngoài vào. Router chỉ cần đọc tới tầng IP để biết đẩy
đi đâu — nó không quan tâm bên trong là HTTP hay SSH.

Thiết bị theo tầng: **switch** làm việc ở tầng liên kết (MAC), **router** ở tầng
internet (IP), **firewall ứng dụng** đọc tới tầng trên cùng.

Trong sandbox này xem được tầng liên kết và tầng internet của `lo`:

```sh
cat /sys/class/net/lo/address   # địa chỉ MAC
cat /sys/class/net/lo/mtu       # gói lớn nhất tầng liên kết chở được
ip -s link show lo              # đếm gói vào/ra
```$md$),

    ('net-lab-7', 'Phân Giải Tên Và DNS', 60, 6, $md$# Phân giải tên và DNS

Người nhớ tên, máy cần số. Phân giải tên là bước dịch giữa hai thứ đó, và nó xảy
ra **trước** khi có bất kỳ gói tin nào được gửi đi.

Thứ tự tra cứu trên Linux (do `/etc/nsswitch.conf` quy định), gần như luôn là:

1. **`/etc/hosts`** — bảng tĩnh ngay trên máy, thắng tất cả
2. **DNS** — hỏi máy chủ ghi trong `/etc/resolv.conf`

Nên sửa `/etc/hosts` là cách nhanh nhất để trỏ một tên miền đi chỗ khác trên
đúng một máy — mẹo quen thuộc khi test.

Định dạng `/etc/hosts`: mỗi dòng một địa chỉ IP rồi tới các tên trỏ về nó, `#` là
chú thích.

## Các loại bản ghi

| Bản ghi | Trả về |
| --- | --- |
| `A` | địa chỉ IPv4 |
| `AAAA` | địa chỉ IPv6 |
| `CNAME` | bí danh, trỏ sang một tên khác |
| `MX` | máy chủ nhận thư của tên miền |
| `NS` | máy chủ DNS quản lý tên miền |

## Công cụ

```sh
getent hosts localhost      # tra theo đúng thứ tự hệ thống dùng
nslookup example.com        # hỏi thẳng DNS
dig example.com A           # hỏi thẳng DNS, đầy đủ hơn
```

> Sandbox **không có mạng**, nên `nslookup`/`dig` sẽ không ra kết quả — phần DNS
> thật học bằng lý thuyết. Còn `/etc/hosts` là file có thật ngay trong container,
> đọc và phân tích được như mọi file khác.$md$),

    ('net-lab-8', 'HTTP Thô Bằng Tay', 60, 7, $md$# HTTP thô bằng tay

`curl` tiện, nhưng nó giấu mất thứ đáng xem nhất: HTTP chỉ là **văn bản** gửi
qua một socket TCP. Tự gõ một yêu cầu là cách nhanh nhất để tin điều đó.

```sh
printf "GET / HTTP/1.0\r\nHost: 127.0.0.1\r\n\r\n" | nc 127.0.0.1 8080
```

Từng phần một:

```
GET / HTTP/1.0        ← dòng yêu cầu: method, đường dẫn, phiên bản
Host: 127.0.0.1       ← header, mỗi dòng một cặp tên: giá trị
                      ← MỘT DÒNG TRỐNG báo hết header
(body nếu có)
```

Kết thúc dòng phải là `\r\n` (CR LF), không phải `\n` — đó là quy định của giao
thức. Dòng trống cuối cùng là thứ hay quên nhất: thiếu nó, server ngồi đợi mãi.

Phản hồi cũng đúng khuôn đó:

```
HTTP/1.1 200 OK
Content-type: text/html
                      ← dòng trống
<h1>chao</h1>
```

## Method

| Method | Ý nghĩa | Có sửa dữ liệu không |
| --- | --- | --- |
| `GET` | lấy về | không (an toàn) |
| `POST` | tạo mới / gửi dữ liệu | có |
| `PUT` | thay thế toàn bộ | có |
| `DELETE` | xoá | có |
| `HEAD` | như GET nhưng chỉ lấy header | không |

## 1.0 và 1.1

HTTP/1.1 bắt buộc có header `Host` — nhờ nó một địa chỉ IP mới phục vụ được nhiều
tên miền — và mặc định **giữ kết nối** cho nhiều yêu cầu liên tiếp thay vì đóng
sau mỗi lần.$md$)
) AS v(slug, title, minutes, idx, body) ON true
WHERE c.slug = 'mang-may-tinh'
ON CONFLICT (slug) DO NOTHING;

-- --- net-lab-1 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Ghi địa chỉ IPv4 kèm tiền tố của giao diện lo vào ~/dia-chi.txt (ví dụ dạng a.b.c.d/n)',
     'ip -o -4 addr show lo rồi lấy cột thứ 4 bằng awk.', 'script',
     $chk$test -f /home/student/dia-chi.txt && test "$(tr -d " \n" < /home/student/dia-chi.txt)" = "$(ip -o -4 addr show lo | awk "{print \$4}")"$chk$,
     '', '[]'),

    (1, 'Ghi tên mọi giao diện mạng đang có vào ~/giao-dien.txt, mỗi tên một dòng',
     'ip -o link show in mỗi giao diện một dòng; tên nằm giữa dấu hai chấm.', 'script',
     $chk$test -f /home/student/giao-dien.txt && ip -o link show | awk -F": " "{print \$2}" | cut -d@ -f1 | cmp -s - /home/student/giao-dien.txt$chk$,
     '', '[]'),

    (2, 'Xem địa chỉ của mọi giao diện mạng', '', 'command', '',
     E'ip addr\nip address\nip a\nip -4 addr', '[]'),

    (3, 'Xem bảng định tuyến', '', 'command', '', E'ip route\nip r\nip route show', '[]'),

    (4, '127.0.0.1 là gì?', '', 'choice', '', '',
     '[{"text": "Địa chỉ loopback — gói tin gửi tới đó quay ngược vào chính máy này, không ra khỏi máy", "correct": true},
       {"text": "Địa chỉ của router mặc định trong mạng nội bộ", "correct": false},
       {"text": "Địa chỉ máy chủ DNS công cộng", "correct": false},
       {"text": "Địa chỉ quảng bá của mạng nội bộ", "correct": false}]'),

    (5, 'Dải nào KHÔNG phải dải IP nội bộ (private)?', '', 'choice', '', '',
     '[{"text": "8.8.0.0/16", "correct": true},
       {"text": "10.0.0.0/8", "correct": false},
       {"text": "172.16.0.0/12", "correct": false},
       {"text": "192.168.0.0/16", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'net-lab-1'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- net-lab-2 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Mở một cổng TCP 8080 đang lắng nghe, chạy dưới nền',
     'nc -l -p 8080 & — dấu & đẩy xuống nền, nếu không shell đứng chờ.', 'script',
     $chk$ss -ltn | grep -qE ":8080[[:space:]]"$chk$, '', '[]'),

    (1, 'Ghi danh sách socket TCP đang lắng nghe vào ~/cong.txt (phải thấy 8080 trong đó)',
     'ss -ltn > ~/cong.txt', 'script',
     $chk$test -f /home/student/cong.txt && grep -qE ":8080[[:space:]]" /home/student/cong.txt$chk$,
     '', '[]'),

    (2, 'Liệt kê mọi socket TCP đang lắng nghe, giữ nguyên số hiệu cổng', '', 'command', '',
     E'ss -ltn\nss -tln\nss -lnt', '[]'),

    (3, 'Kiểm tra xem cổng 8080 trên 127.0.0.1 có ai lắng nghe không', '', 'command', '',
     E'nc -z 127.0.0.1 8080\nnc -zv 127.0.0.1 8080', '[]'),

    (4, 'Cặp nào ghép đúng cổng với dịch vụ?', '', 'choice', '', '',
     '[{"text": "22 = SSH, 80 = HTTP, 443 = HTTPS, 53 = DNS", "correct": true},
       {"text": "22 = HTTP, 80 = SSH, 443 = DNS, 53 = HTTPS", "correct": false},
       {"text": "21 = SSH, 80 = HTTPS, 443 = HTTP, 25 = DNS", "correct": false},
       {"text": "22 = DNS, 53 = SSH, 80 = HTTPS, 443 = HTTP", "correct": false}]'),

    (5, 'Vì sao web server lúc dev hay dùng cổng 8080 thay vì 80?', '', 'choice', '', '',
     '[{"text": "Cổng dưới 1024 là cổng đặc quyền, chỉ tiến trình chạy bằng root mới gắn vào được", "correct": true},
       {"text": "Cổng 80 chỉ dành riêng cho HTTPS", "correct": false},
       {"text": "Cổng 8080 nhanh hơn vì số lớn hơn", "correct": false},
       {"text": "Trình duyệt chặn cổng 80 khi chạy trên localhost", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'net-lab-2'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- net-lab-3 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Dựng server nc nghe cổng 9000 ghi vào ~/nhan.txt, rồi từ client gửi đúng dòng “xin chao devforge”',
     'Hai bước: (nc -l -p 9000 > ~/nhan.txt &) rồi echo "xin chao devforge" | nc 127.0.0.1 9000', 'script',
     $chk$grep -qx "xin chao devforge" /home/student/nhan.txt$chk$, '', '[]'),

    (1, 'Gửi nguyên một file qua netcat: tạo ~/goi-di.txt, truyền sang cổng 9001, nhận ra ~/nhan-file.txt giống hệt',
     'Phía nhận nghe trước, phía gửi dùng: nc 127.0.0.1 9001 < ~/goi-di.txt', 'script',
     $chk$test -s /home/student/goi-di.txt && cmp -s /home/student/goi-di.txt /home/student/nhan-file.txt$chk$,
     '', '[]'),

    (2, 'Mở một cổng TCP 9002 đang lắng nghe để quan sát', '', 'command', '',
     E'nc -l -p 9002\nnc -l 9002', '[]'),

    (3, 'TCP khác UDP ở chỗ nào?', '', 'choice', '', '',
     '[{"text": "TCP có bắt tay, đảm bảo tới nơi và đúng thứ tự, phát lại khi mất; UDP bắn đi là xong, không đảm bảo gì nhưng nhẹ và nhanh", "correct": true},
       {"text": "TCP nhanh hơn UDP vì không phải kiểm tra lỗi", "correct": false},
       {"text": "UDP có bắt tay ba bước, TCP thì không", "correct": false},
       {"text": "TCP dùng cho mạng nội bộ, UDP dùng cho Internet", "correct": false}]'),

    (4, 'Bắt tay ba bước của TCP diễn ra theo thứ tự nào?', '', 'choice', '', '',
     '[{"text": "SYN → SYN/ACK → ACK", "correct": true},
       {"text": "ACK → SYN → SYN/ACK", "correct": false},
       {"text": "SYN → ACK → FIN", "correct": false},
       {"text": "HELLO → OK → READY", "correct": false}]'),

    (5, 'Gọi tới một cổng không có ai lắng nghe thì phía gọi nhận được gì?', '', 'choice', '', '',
     '[{"text": "Bị từ chối ngay (connection refused) — hệ điều hành phía kia trả về RST", "correct": true},
       {"text": "Chờ vô hạn, không bao giờ có phản hồi", "correct": false},
       {"text": "Kết nối thành công nhưng không nhận được dữ liệu", "correct": false},
       {"text": "Gói tin tự động chuyển sang cổng gần nhất đang mở", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'net-lab-3'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- net-lab-4 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Tạo ~/web/index.html chứa chuỗi “chao devforge” rồi chạy httpd phục vụ thư mục đó ở cổng 8080',
     'httpd -p 8080 -h /home/student/web — nó tự chạy nền, không cần dấu &.', 'script',
     $chk$curl -s --max-time 5 http://127.0.0.1:8080/ | grep -q "chao devforge"$chk$, '', '[]'),

    (1, 'Lưu header phản hồi của trang chủ vào ~/header.txt (phải có dòng trạng thái 200)',
     'curl -I http://127.0.0.1:8080/ > ~/header.txt', 'script',
     $chk$test -f /home/student/header.txt && grep -qiE "^HTTP/1\.[01] 200" /home/student/header.txt$chk$,
     '', '[]'),

    (2, 'Gọi một đường dẫn không tồn tại và ghi ĐÚNG mã trạng thái nhận được vào ~/ma-loi.txt',
     'curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8080/khong-co > ~/ma-loi.txt', 'script',
     $chk$test "$(tr -d " \n" < /home/student/ma-loi.txt)" = "404"$chk$, '', '[]'),

    (3, 'Lấy chỉ phần header của trang chủ trên cổng 8080', '', 'command', '',
     E'curl -I http://127.0.0.1:8080/\ncurl --head http://127.0.0.1:8080/\ncurl -I 127.0.0.1:8080', '[]'),

    (4, 'Mã trạng thái nào ghép đúng với ý nghĩa?', '', 'choice', '', '',
     '[{"text": "200 thành công · 301 chuyển vĩnh viễn · 404 không tìm thấy · 500 lỗi phía server", "correct": true},
       {"text": "200 thành công · 301 lỗi client · 404 lỗi server · 500 chuyển hướng", "correct": false},
       {"text": "200 chuyển hướng · 301 thành công · 404 lỗi server · 500 không tìm thấy", "correct": false},
       {"text": "200 thành công · 301 không tìm thấy · 404 chuyển hướng · 500 lỗi client", "correct": false}]'),

    (5, 'Một yêu cầu HTTP gồm những phần nào?', '', 'choice', '', '',
     '[{"text": "Dòng đầu (method + đường dẫn + phiên bản), các header, rồi body nếu có", "correct": true},
       {"text": "Chỉ có đường dẫn và body, header là của phản hồi", "correct": false},
       {"text": "Một chuỗi nhị phân đã nén, không đọc bằng mắt được", "correct": false},
       {"text": "Địa chỉ IP nguồn và đích, giống hệt một gói TCP", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'net-lab-4'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- net-lab-5 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Với 192.168.1.0/26: ghi ~/subnet.txt gồm 3 dòng — địa chỉ mạng, địa chỉ quảng bá, số host dùng được',
     '26 bit mạng, còn 6 bit host: 2^6 = 64 địa chỉ. Dòng 3 chỉ ghi con số.', 'script',
     $chk$test -f /home/student/subnet.txt && test "$(sed -n 1p /home/student/subnet.txt | tr -d " ")" = "192.168.1.0" && test "$(sed -n 2p /home/student/subnet.txt | tr -d " ")" = "192.168.1.63" && test "$(sed -n 3p /home/student/subnet.txt | tr -d " ")" = "62"$chk$,
     '', '[]'),

    (1, 'Ghi vào ~/so-host.txt số host dùng được của một mạng /22 (chỉ con số)',
     '32 - 22 = 10 bit host. 2^10 rồi trừ đi địa chỉ mạng và địa chỉ quảng bá.', 'script',
     $chk$test "$(tr -d " \n" < /home/student/so-host.txt)" = "1022"$chk$, '', '[]'),

    (2, 'Xem địa chỉ IPv4 của riêng giao diện lo', '', 'command', '',
     E'ip -4 addr show lo\nip addr show lo\nip -4 addr show dev lo', '[]'),

    (3, 'Mặt nạ 255.255.255.0 tương ứng tiền tố nào?', '', 'choice', '', '',
     '[{"text": "/24", "correct": true},
       {"text": "/16", "correct": false},
       {"text": "/25", "correct": false},
       {"text": "/32", "correct": false}]'),

    (4, 'Một mạng /30 dùng được bao nhiêu host?', '', 'choice', '', '',
     '[{"text": "2 — đủ cho một liên kết điểm-điểm giữa hai router", "correct": true},
       {"text": "4", "correct": false},
       {"text": "30", "correct": false},
       {"text": "1", "correct": false}]'),

    (5, 'Vì sao mỗi mạng con phải bỏ ra hai địa chỉ không gán cho máy nào?', '', 'choice', '', '',
     '[{"text": "Một là địa chỉ mạng (bit host toàn 0), một là địa chỉ quảng bá (bit host toàn 1)", "correct": true},
       {"text": "Một dành cho router, một dành cho máy chủ DNS", "correct": false},
       {"text": "Hai địa chỉ đó dự phòng cho việc mở rộng mạng sau này", "correct": false},
       {"text": "Đó là quy ước của nhà cung cấp dịch vụ Internet", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'net-lab-5'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- net-lab-6 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Ghi địa chỉ MAC của giao diện lo vào ~/mac.txt (thông tin tầng liên kết)',
     'Kernel bày nó ra dưới dạng file: /sys/class/net/lo/address', 'script',
     $chk$test -f /home/student/mac.txt && test "$(tr -d " \n" < /home/student/mac.txt)" = "$(cat /sys/class/net/lo/address)"$chk$,
     '', '[]'),

    (1, 'Ghi MTU của lo vào ~/mtu.txt — kích thước gói lớn nhất tầng liên kết chở được',
     '/sys/class/net/lo/mtu, hoặc đọc từ ip link show lo.', 'script',
     $chk$test "$(tr -d " \n" < /home/student/mtu.txt)" = "$(cat /sys/class/net/lo/mtu)"$chk$,
     '', '[]'),

    (2, 'Xem thống kê gói tin vào/ra của giao diện lo', '', 'command', '',
     E'ip -s link show lo\nip -s link\nip -s l show lo', '[]'),

    (3, 'Cổng (port) là khái niệm của tầng nào?', '', 'choice', '', '',
     '[{"text": "Tầng giao vận — TCP và UDP", "correct": true},
       {"text": "Tầng liên kết — Ethernet", "correct": false},
       {"text": "Tầng internet — IP", "correct": false},
       {"text": "Tầng ứng dụng — HTTP", "correct": false}]'),

    (4, 'Một yêu cầu HTTP đi xuống dây mạng thì được bọc theo thứ tự nào?', '', 'choice', '', '',
     '[{"text": "Ethernet [ IP [ TCP [ HTTP ] ] ] — tầng dưới bọc ngoài tầng trên", "correct": true},
       {"text": "HTTP [ TCP [ IP [ Ethernet ] ] ] — tầng trên bọc ngoài tầng dưới", "correct": false},
       {"text": "IP [ Ethernet [ HTTP [ TCP ] ] ]", "correct": false},
       {"text": "Không bọc gì cả, mỗi tầng gửi riêng một gói", "correct": false}]'),

    (5, 'Switch và router khác nhau ở tầng làm việc thế nào?', '', 'choice', '', '',
     '[{"text": "Switch chuyển frame theo địa chỉ MAC ở tầng liên kết; router chuyển packet theo địa chỉ IP ở tầng internet", "correct": true},
       {"text": "Switch làm việc với IP, router làm việc với MAC", "correct": false},
       {"text": "Cả hai đều đọc tới tầng ứng dụng để quyết định", "correct": false},
       {"text": "Switch nhanh hơn vì nó đọc ít tầng hơn router, ngoài ra giống nhau", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'net-lab-6'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- net-lab-7 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Ghi địa chỉ IPv4 mà /etc/hosts gán cho localhost vào ~/localhost-ip.txt',
     'grep localhost /etc/hosts rồi lấy dòng có địa chỉ IPv4, cột đầu tiên.', 'script',
     $chk$test "$(tr -d " \n" < /home/student/localhost-ip.txt)" = "127.0.0.1"$chk$,
     '', '[]'),

    (1, 'Đếm số bản ghi thật trong /etc/hosts (bỏ dòng trống và dòng chú thích), ghi vào ~/so-ban-ghi.txt',
     'grep -cvE để đếm ngược: loại dòng bắt đầu bằng # và dòng rỗng.', 'script',
     $chk$test "$(tr -d " \n" < /home/student/so-ban-ghi.txt)" = "$(grep -cvE "^[[:space:]]*(#|$)" /etc/hosts)"$chk$,
     '', '[]'),

    (2, 'Tra localhost bằng đúng cơ chế phân giải của hệ thống, lưu kết quả vào ~/getent.txt',
     'getent hosts localhost > ~/getent.txt', 'script',
     $chk$test -s /home/student/getent.txt && grep -q "localhost" /home/student/getent.txt$chk$,
     '', '[]'),

    (3, 'Hỏi DNS về tên miền example.com', 'Sandbox không có mạng nên sẽ không ra kết quả — cú pháp mới là thứ được chấm.', 'command', '',
     E'nslookup example.com\ndig example.com\ndig example.com A\nhost example.com', '[]'),

    (4, 'Bản ghi nào trả về địa chỉ IPv4 của một tên miền?', '', 'choice', '', '',
     '[{"text": "A", "correct": true},
       {"text": "AAAA", "correct": false},
       {"text": "CNAME", "correct": false},
       {"text": "MX", "correct": false}]'),

    (5, 'Một tên có trong /etc/hosts và cũng có bản ghi DNS khác — máy dùng cái nào?', '', 'choice', '', '',
     '[{"text": "/etc/hosts, vì nó được tra trước theo thứ tự trong nsswitch.conf", "correct": true},
       {"text": "DNS, vì đó là nguồn chính thức trên Internet", "correct": false},
       {"text": "Cái nào trả lời nhanh hơn thì dùng", "correct": false},
       {"text": "Báo lỗi xung đột, phải xoá một trong hai", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'net-lab-7'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- net-lab-8 -------------------------------------------------------------
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, v.script, v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Dựng lại web server: ~/web/index.html chứa “chao devforge”, httpd phục vụ ở cổng 8080', '', 'script',
     $chk$curl -s --max-time 5 http://127.0.0.1:8080/ | grep -q "chao devforge"$chk$, '', '[]'),

    (1, 'Tự gõ một yêu cầu HTTP thô bằng nc, lưu TOÀN BỘ phản hồi (cả header) vào ~/tho.txt',
     'printf "GET / HTTP/1.0\r\nHost: 127.0.0.1\r\n\r\n" | nc 127.0.0.1 8080 > ~/tho.txt — nhớ dòng trống cuối.', 'script',
     $chk$grep -qE "^HTTP/1\.[01] 200" /home/student/tho.txt && grep -q "chao devforge" /home/student/tho.txt$chk$,
     '', '[]'),

    (2, 'Cũng bằng nc, xin một đường dẫn không tồn tại và lưu phản hồi vào ~/tho-404.txt',
     'Đổi đường dẫn trong dòng đầu tiên của yêu cầu.', 'script',
     $chk$grep -qE "^HTTP/1\.[01] 404" /home/student/tho-404.txt$chk$, '', '[]'),

    (3, 'Xem trọn một phiên HTTP, cả yêu cầu lẫn phản hồi', '', 'command', '',
     E'curl -v http://127.0.0.1:8080/\ncurl --verbose http://127.0.0.1:8080/\ncurl -v 127.0.0.1:8080', '[]'),

    (4, 'Cái gì báo cho server biết phần header đã hết?', '', 'choice', '', '',
     '[{"text": "Một dòng trống — tức hai lần CRLF liên tiếp", "correct": true},
       {"text": "Header cuối cùng phải tên là End", "correct": false},
       {"text": "Số byte khai trong Content-Length", "correct": false},
       {"text": "Server tự đoán khi hết dữ liệu gửi tới", "correct": false}]'),

    (5, 'HTTP/1.1 khác HTTP/1.0 ở điểm nào?', '', 'choice', '', '',
     '[{"text": "1.1 bắt buộc header Host và mặc định giữ kết nối cho nhiều yêu cầu liên tiếp", "correct": true},
       {"text": "1.1 mã hoá dữ liệu, 1.0 thì không", "correct": false},
       {"text": "1.1 chạy trên UDP, 1.0 chạy trên TCP", "correct": false},
       {"text": "1.1 bỏ hẳn khái niệm mã trạng thái", "correct": false}]')
) AS v(idx, title, hint, kind, script, cmds, opts) ON true
WHERE l.slug = 'net-lab-8'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- Hai câu bổ sung cho lab 1: IPv6 và cách xem trạng thái giao diện.
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, '', 10, v.kind, '', v.cmds, v.opts::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (6, 'Địa chỉ loopback của IPv6 là gì?', 'choice', '',
     '[{"text": "::1", "correct": true},
       {"text": "127.0.0.1", "correct": false},
       {"text": "0.0.0.0", "correct": false},
       {"text": "fe80::1", "correct": false}]'),

    (7, 'Xem trạng thái lên/xuống của mọi giao diện mạng', 'command',
     E'ip link\nip link show\nip l', '[]')
) AS v(idx, title, kind, cmds, opts) ON true
WHERE l.slug = 'net-lab-1'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- --- Ôn tập ----------------------------------------------------------------
INSERT INTO reviews (course_id, title, content_md, order_idx)
SELECT c.id, v.title, v.body, v.idx
FROM courses c
JOIN (VALUES
    (0, 'Bộ lệnh chẩn đoán mạng', $md$| Câu hỏi | Lệnh |
| --- | --- |
| Máy có những giao diện nào, địa chỉ gì? | `ip -o -4 addr` |
| Đi ra ngoài theo đường nào? | `ip route` |
| Đang có gì lắng nghe? | `ss -ltnp` |
| Cổng kia có ai nghe không? | `nc -z 127.0.0.1 8080` |
| Máy chủ web trả về gì? | `curl -I http://...` |
| Toàn bộ một phiên HTTP? | `curl -v http://...` |

Thứ tự chẩn đoán khi “không vào được”: có địa chỉ chưa → có tuyến chưa → tên
miền có phân giải không → cổng có mở không → ứng dụng trả lời gì.

> Trong sandbox này `ping` không chạy được: ICMP cần `CAP_NET_RAW`, mà container
> bỏ toàn bộ capability. Đó là chủ ý, không phải hỏng.$md$),

    (1, 'TCP, UDP và cổng', $md$## Địa chỉ đưa tới máy, cổng đưa tới tiến trình

```
        192.168.1.10  :  8080
        └── máy nào ──┘   └─ tiến trình nào ─┘
```

## TCP hay UDP

| | TCP | UDP |
| --- | --- | --- |
| Bắt tay | có (SYN/SYN-ACK/ACK) | không |
| Đảm bảo tới nơi | có, phát lại khi mất | không |
| Đúng thứ tự | có | không |
| Hợp cho | HTTP, SSH, cơ sở dữ liệu | DNS, thoại, game, streaming |

## Cổng

- `0 – 1023` — đặc quyền, cần root
- `1024 – 49151` — đăng ký (8080, 5432, 3306…)
- `49152 – 65535` — cổng tạm, hệ điều hành cấp cho phía client$md$),

    (2, 'Tính subnet trong đầu', $md$Với tiền tố `/n`:

```
số bit host      = 32 - n
tổng địa chỉ     = 2^(32-n)
host dùng được   = 2^(32-n) - 2
```

| CIDR | Mặt nạ | Tổng | Dùng được |
| --- | --- | --- | --- |
| `/30` | 255.255.255.252 | 4 | 2 |
| `/29` | 255.255.255.248 | 8 | 6 |
| `/28` | 255.255.255.240 | 16 | 14 |
| `/26` | 255.255.255.192 | 64 | 62 |
| `/24` | 255.255.255.0 | 256 | 254 |
| `/22` | 255.255.252.0 | 1024 | 1022 |

**Mẹo tìm địa chỉ mạng:** lấy bước nhảy = 256 − số cuối của mặt nạ. Với `/26`,
mặt nạ cuối là 192, bước nhảy 64 → các mạng con là `.0`, `.64`, `.128`, `.192`.
Địa chỉ quảng bá luôn là địa chỉ ngay trước mạng con kế tiếp.$md$),

    (3, 'Bốn tầng, đọc từ dưới lên', $md$```
┌─────────────┬──────────┬───────────┬─────────────────┐
│ Ứng dụng    │ dữ liệu  │ URL/tên   │ HTTP, DNS, SSH  │
│ Giao vận    │ segment  │ cổng      │ TCP, UDP        │
│ Internet    │ packet   │ địa chỉ IP│ IP, ICMP        │
│ Liên kết    │ frame    │ MAC       │ Ethernet, Wi-Fi │
└─────────────┴──────────┴───────────┴─────────────────┘
```

Đóng gói khi gửi, bóc lớp khi nhận:

```
[ Ethernet [ IP [ TCP [ HTTP: GET / ] ] ] ]
```

Ích lợi thật của việc chia tầng: đổi Wi-Fi sang cáp thì tầng liên kết đổi, ba
tầng trên không phải sửa gì.

**Ai đọc tới đâu**

| Thiết bị | Đọc tới tầng | Quyết định theo |
| --- | --- | --- |
| Switch | liên kết | địa chỉ MAC |
| Router | internet | địa chỉ IP |
| Firewall thường | giao vận | IP + cổng |
| Proxy / WAF | ứng dụng | nội dung HTTP |$md$),

    (4, 'Từ tên miền tới byte đầu tiên', $md$Gõ `example.com` vào trình duyệt, theo thứ tự xảy ra:

1. **Phân giải tên** — `/etc/hosts` trước, rồi DNS. Ra một địa chỉ IP.
2. **Định tuyến** — máy so địa chỉ đó với bảng định tuyến để biết gửi ra đâu.
3. **Bắt tay TCP** — SYN → SYN/ACK → ACK với cổng 80 (hoặc 443).
4. **Yêu cầu HTTP** — `GET / HTTP/1.1`, header `Host`, một dòng trống.
5. **Phản hồi** — dòng trạng thái, header, dòng trống, rồi nội dung.

Hỏng ở bước nào thì triệu chứng khác nhau, và đó chính là cách chẩn đoán:

| Triệu chứng | Hỏng ở bước |
| --- | --- |
| “Không phân giải được tên miền” | 1 |
| Treo rất lâu rồi hết giờ | 2 hoặc 3 (gói không tới, hoặc bị firewall nuốt) |
| “Connection refused” ngay lập tức | 3 — máy tới được, nhưng không ai nghe cổng đó |
| Có mã trạng thái 4xx/5xx | 4–5 — mạng đã thông, lỗi nằm ở ứng dụng |

Đi từ dưới lên: có địa chỉ chưa → có tuyến chưa → tên có phân giải không → cổng
có ai nghe không → ứng dụng trả lời gì.$md$)
) AS v(idx, title, body) ON true
WHERE c.slug = 'mang-may-tinh'
AND NOT EXISTS (SELECT 1 FROM reviews r WHERE r.course_id = c.id AND r.order_idx = v.idx);


-- ===========================================================================
-- CI/CD CƠ BẢN
--
-- Khoá duy nhất không chạy container. Không có gì để `docker exec` vào: học
-- viên viết pipeline, server mô phỏng lịch chạy, và điều kiện đạt nói về lịch
-- chạy đó chứ không về văn bản họ gõ. Xem SIM-CICD.md.
--
-- Mọi con số trong catalog là do tác giả gõ ra, không đo từ CI thật. Bài học
-- nằm ở TỈ LỆ — hai job song song xong nhanh gấp đôi nối tiếp, cache hit rẻ hơn
-- cài lại một bậc — chứ không ở giá trị tuyệt đối.
--
-- `make check-sim` chạy mỗi nhiệm vụ với một pipeline sai (phải trượt) và một
-- pipeline đúng (phải đậu), lấy từ scripts/sim-pipelines/.
-- ===========================================================================

INSERT INTO labs (course_id, slug, title, description_md, duration_minutes, sim_scenario, order_idx)
SELECT c.id, v.slug, v.title, v.body, v.minutes, v.scenario::jsonb, v.idx
FROM courses c
JOIN (VALUES
    ('cicd-lab-1', 'Pipeline Đầu Tiên', 60, 0, $scenario${
      "version": 1,
      "runner_count": 2,
      "cache_restore_seconds": 10,
      "catalog": {
        "checkout":     { "seconds": 5 },
        "npm-ci":       { "seconds": 90, "cacheable": "node_modules" },
        "lint":         { "seconds": 25 },
        "npm-test":     { "seconds": 120 },
        "npm-build":    { "seconds": 60, "produces": "dist" },
        "docker-build": { "seconds": 180, "consumes": "dist" }
      }
    }$scenario$, $md$# Pipeline đầu tiên

Bài này **không có terminal**. Bạn viết một file pipeline, bấm *Chạy pipeline*,
và server mô phỏng lịch chạy rồi vẽ ra: job nào chạy lúc nào, trên runner nào,
mất bao lâu.

> Số giây ở đây là **thời gian mô phỏng**, không đo từ CI thật. Thứ đáng học là
> tỉ lệ giữa các cách xếp job, không phải con số tuyệt đối.

## Hình dạng file

```yaml
jobs:
  build:                       # tên job, bạn tự đặt
    steps: [checkout, npm-ci]  # chạy lần lượt từ trái sang phải
  test:
    needs: [build]             # đợi build xong mới bắt đầu
    steps: [checkout, npm-test]
    cache: [node_modules]      # xin dùng lại cache của khoá này
```

Chỉ có ba khoá: `steps`, `needs`, `cache`. Gõ sai tên khoá thì server báo lỗi
kèm số dòng — nó **không** bỏ qua im lặng.

## Bốn luật quyết định mọi thứ

**1. Job không có `needs` thì chạy ngay.** Lab này có **2 runner**, nên tối đa
hai job chạy cùng lúc. Job thứ ba phải đợi một runner rảnh.

**2. `needs` là lời hứa "tôi cần thứ job kia để lại".** Thêm một dòng `needs`
không cần thiết là tự bắt mình xếp hàng: hai job đáng ra chạy song song thì nay
nối tiếp, và pipeline dài gấp đôi mà không an toàn hơn chút nào.

**3. Artifact phải tới được.** `docker-build` cần thư mục `dist` do `npm-build`
tạo ra. Nó tìm `dist` ở hai chỗ: một step trước đó **trong cùng job** (cùng một
workspace), hoặc một job nằm trong chuỗi `needs` của nó. Job chạy trước ở nhánh
khác **không tính** — đĩa của runner đó không phải đĩa này.

**4. Cache chỉ ấm sang lượt sau.** Khai `cache: [node_modules]` thì lượt chạy
**kế tiếp** mới được giảm giá, lượt đang chạy thì không. Khai xong nhớ bấm Chạy
thêm một lượt nữa.

## Step có sẵn

| Step | Giây | Ghi chú |
| --- | --- | --- |
| `checkout` | 5 | lấy code |
| `npm-ci` | 90 | cài thư viện — cache được bằng khoá `node_modules` |
| `lint` | 25 | soi code |
| `npm-test` | 120 | chạy test |
| `npm-build` | 60 | build, **tạo ra** `dist` |
| `docker-build` | 180 | đóng image, **cần** `dist` |

Bảng này cũng nằm ngay trên ô soạn thảo — không cần nhớ.$md$)
) AS v(slug, title, minutes, idx, scenario, body) ON true
WHERE c.slug = 'ci-cd-co-ban'
ON CONFLICT (slug) DO NOTHING;

-- --- cicd-lab-1 ------------------------------------------------------------
-- Bốn nhiệm vụ mô phỏng theo đúng thứ tự bài học muốn dạy, và một câu lý thuyết
-- xen giữa. Nhiệm vụ cuối đòi cả ba thứ cùng lúc: 260 giây không đạt được nếu
-- thiếu song song hay thiếu cache (xem con số trong SIM-CICD.md).
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, sim_goal, order_idx)
SELECT l.id, v.title, v.hint, 10, v.kind, '', '', v.opts::jsonb, v.goal::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Viết một job tên `ci` chạy trọn vẹn: lấy code, cài, test, build, đóng image. Pipeline phải xanh.',
     'Năm step trong một job, thứ tự quan trọng: `docker-build` cần `dist`, mà `dist` do `npm-build` tạo ra.',
     'sim', '[]',
     '{"all": [{"run_status": "success"}, {"job_present": "ci"}]}'),

    (1, 'Tách thành hai job `test` và `build` chạy **cùng lúc**. Pipeline vẫn phải xanh.',
     'Bỏ `needs` đi thì hai job cùng sẵn sàng từ giây 0, và lab này có 2 runner. Cả hai đều cần `checkout` và `npm-ci` của riêng mình.',
     'sim', '[]',
     '{"all": [{"run_status": "success"}, {"jobs_parallel": ["test", "build"]}]}'),

    (2, 'Job `test` không dùng gì do `build` tạo ra. Thêm `needs: [build]` vào `test` thì điều gì xảy ra?',
     '', 'choice',
     '[{"text": "test phải đợi build xong mới chạy, pipeline dài thêm mà không an toàn hơn", "correct": true},
       {"text": "test chạy nhanh hơn vì build đã cài sẵn thư viện cho nó", "correct": false},
       {"text": "Không có gì đổi, needs chỉ để người đọc biết thứ tự", "correct": false},
       {"text": "build và test vẫn chạy song song, needs chỉ tính khi có artifact", "correct": false}]',
     '{}'),

    (3, 'Khai cache `node_modules` cho cả hai job, rồi chạy lại để `npm-ci` được dùng cache.',
     'Thêm `cache: [node_modules]` vào từng job. Lượt đầu vẫn cài đủ 90 giây — cache chỉ ấm sang lượt sau, nên bấm Chạy thêm một lượt nữa.',
     'sim', '[]',
     '{"all": [{"run_status": "success"}, {"cache_hit": "node_modules"}]}'),

    (4, 'Thêm job `ship` đóng image, và đưa cả pipeline xuống dưới 260 giây.',
     '`ship` cần `dist`, nên nó phải `needs` job đã tạo ra `dist` — chỉ job đó thôi, thêm nữa là tự bắt mình xếp hàng. Giữ nguyên cache của bài trước.',
     'sim', '[]',
     '{"all": [{"run_status": "success"}, {"job_present": "ship"}, {"total_seconds_lte": 260}]}')
) AS v(idx, title, hint, kind, opts, goal) ON true
WHERE l.slug = 'cicd-lab-1'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- ===========================================================================
-- TRỰC SỰ CỐ
--
-- Lab ở đây ngược chiều mọi lab khác: container mở ra là đã hỏng sẵn, và việc
-- của học viên là tìm ra vì sao rồi cứu nó. Một lab mang nhiều kịch bản, mỗi
-- phiên bốc ngẫu nhiên một cái — nên chơi lại là một ca trực khác, không phải
-- một bài đã thuộc.
--
-- Ba kịch bản dưới đây phá **cùng một dịch vụ** theo ba cách. Đó là điều kiện
-- để chúng dùng chung một câu hỏi và một check_script: thứ học viên thấy luôn
-- giống nhau — /healthz không trả `ok` — chỉ nguyên nhân là khác.
-- ===========================================================================

-- Khoá này là **chỗ neo, không phải nội dung**: `labs.course_id` là NOT NULL nên
-- một lab phải thuộc về một khoá nào đó. Để `draft` để nó không hiện ở /courses
-- — War Room vào từ thanh nav, không đi qua khoá học nào, và người dùng không
-- phải đăng ký gì. `SpecBySlug` và `Start` đều miễn trừ lab sự cố khỏi hai rào
-- đó, nên `draft` ở đây không chặn ai bắt đầu.
INSERT INTO courses (slug, title, description, level, status, image_url)
VALUES
    ('truc-su-co', 'Trực Sự Cố',
     'Chỗ neo dữ liệu cho các thử thách War Room. Không phải khoá học, không hiện ở danh sách khoá.',
     'intermediate', 'draft',
     'https://images.unsplash.com/photo-1451187580459-43490279c0fa?w=800')
ON CONFLICT (slug) DO NOTHING;

-- incident_setup dựng dịch vụ; kịch bản phá nó ngay sau đó, trong cùng một
-- shell dưới `set -e`. Ở lab chứ không ở từng kịch bản: cả ba phá chung một
-- dịch vụ, chép setup ba lần là ba chỗ để trôi khác nhau.
INSERT INTO labs (course_id, slug, title, description_md, duration_minutes,
                  lab_image_id, incident_setup, order_idx)
SELECT c.id, v.slug, v.title, v.body, v.minutes,
       (SELECT id FROM lab_images WHERE name = 'devforge/net' AND tag = 'latest'),
       v.setup, v.idx
FROM courses c
JOIN (VALUES
    ('incident-lab-1', 'Ca Trực Đầu Tiên', 15, 0, $md$**23:41.** Điện thoại rung. Trang chủ trả lỗi, khách đang kêu trên mạng xã hội.

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
> viên đọc được, và nó mất cùng lúc với phiên.$md$,
     $sh$mkdir -p "$HOME/web"
printf 'chao devforge\n' > "$HOME/web/index.html"
printf 'ok\n' > "$HOME/web/healthz"
httpd -p 127.0.0.1:8080 -h "$HOME/web"$sh$)
) AS v(slug, title, minutes, idx, body, setup) ON true
WHERE c.slug = 'truc-su-co'
AND NOT EXISTS (SELECT 1 FROM labs l WHERE l.slug = v.slug);

-- Một câu hỏi duy nhất, và nó là câu hỏi của cả ba kịch bản: dịch vụ sống lại
-- chưa. Không hỏi "nguyên nhân là gì" — cái đó hiện ở trang kết quả sau khi
-- xong, chứ hỏi trong lúc làm thì nó thành đáp án trắc nghiệm cho chính bài.
INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, expected_commands, options, order_idx)
SELECT l.id, v.title, v.hint, 20, 'script', v.script, '', '[]'::jsonb, v.idx
FROM labs l
JOIN (VALUES
    (0, 'Khôi phục dịch vụ: /healthz ở cổng 8080 trả về ok',
     'Ba câu hỏi theo thứ tự đó: có ai nghe ở cổng 8080 không (ss -ltn), tiến trình đang nghe là cái gì và trỏ vào đâu (ps aux), thư mục nó phục vụ còn đọc được không (ls -l ~/web).',
     'curl -fsS --max-time 5 http://127.0.0.1:8080/healthz | grep -q ok')
) AS v(idx, title, hint, script) ON true
WHERE l.slug = 'incident-lab-1'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = v.idx);

-- Ba kịch bản. `rps` cố tình giống nhau ở cả ba: ba con số khác nhau biến thanh
-- đếm request hỏng thành vân tay nhận diện kịch bản ngay giây đầu tiên.
--
-- Mỗi break_script chạy sau setup, nên dịch vụ lúc đó đang chạy tốt.
INSERT INTO lab_incidents (lab_id, title, break_script, reveal_md, rps)
SELECT l.id, v.title, v.script, v.reveal, 20
FROM labs l
JOIN (VALUES
    ('Tiến trình web đã chết',
     $sh$pkill httpd$sh$,
     $md$### Tiến trình `httpd` không còn chạy

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
nó chết**, và câu trả lời gần như luôn nằm trong log hoặc trong OOM killer.$md$),

    ('Tiến trình khác đang giữ cổng 8080',
     $sh$pkill httpd
mkdir -p "$HOME/old-release"
printf 'ban cu, khong co healthz\n' > "$HOME/old-release/index.html"
httpd -p 127.0.0.1:8080 -h "$HOME/old-release"$sh$,
     $md$### Cổng 8080 bị một `httpd` khác chiếm, và nó phục vụ nhầm thư mục

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
thay. Bài học: **cổng có người nghe không có nghĩa là đúng người đang nghe.**$md$),

    ('Thư mục web mất quyền đọc',
     $sh$chmod 000 "$HOME/web"$sh$,
     $md$### `~/web` bị `chmod 000`

`httpd` vẫn chạy, cổng vẫn LISTEN, nhưng nó không mở nổi file trong thư mục nên
mọi đường dẫn đều ra **404** — kể cả `/index.html` vốn vẫn nằm nguyên đó.

Đường tìm ra: `ls -ld ~/web` cho ra `d---------`. Sửa:

```sh
chmod 755 ~/web
```

Chỗ dễ mất thì giờ nhất ở kịch bản này là tin vào mã lỗi: 404 đọc ra là "file
không tồn tại", nên người ta đi tìm file trước khi nhìn quyền — mà file vẫn ở
đó. Ngoài đời cảnh này hay tới sau một lệnh `chmod`/`chown` chạy nhầm thư mục,
hoặc một tiến trình deploy chạy dưới user khác.$md$)
) AS v(title, script, reveal) ON true
WHERE l.slug = 'incident-lab-1'
AND NOT EXISTS (
    SELECT 1 FROM lab_incidents i WHERE i.lab_id = l.id AND i.title = v.title
);

-- Không seed tài khoản nào. Hai tài khoản một máy mới cần — một admin, một học
-- viên — do migration 000020 tạo, nên chúng tồn tại kể cả khi file này không
-- được chạy. Bảng xếp hạng vì thế trống cho tới khi có người thật kiếm được
-- điểm, và đó là con số đúng.
