
INSERT INTO lab_images (name, tag, description)
VALUES ('devforge/linux', 'latest', 'Alpine + coreutils cho lab Linux')
ON CONFLICT (name, tag) DO NOTHING;

INSERT INTO courses (slug, title, description, level, status, published_at, image_url)
VALUES
    ('linux-co-ban', 'Linux Cơ Bản',
     'Làm chủ dòng lệnh Linux: filesystem, quyền, process, package.',
     'beginner', 'published', now(),
     'https://images.unsplash.com/photo-1629654297299-c8506221ca97?w=800'),
    ('git-thuc-hanh', 'Git Thực Hành',
     'Quản lý mã nguồn với Git: commit, branch, merge, rebase, remote.',
     'beginner', 'published', now(),
     'https://images.unsplash.com/photo-1618401471353-b98afee0b2eb?w=800'),
    ('docker-nhap-mon', 'Docker Nhập Môn',
     'Container hoá ứng dụng: image, container, volume, network, compose.',
     'intermediate', 'published', now(),
     'https://images.unsplash.com/photo-1605745341112-85968b19335b?w=800')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO labs (course_id, slug, title, description_md, duration_minutes, lab_image_id, order_idx)
SELECT c.id, 'linux-lab-1', 'Điều Hướng Filesystem',
       E'# Điều hướng filesystem\n\nDùng `cd`, `ls`, `pwd` để di chuyển. Tạo thư mục `~/devforge`.',
       60, (SELECT id FROM lab_images WHERE name = 'devforge/linux' LIMIT 1), 0
FROM courses c WHERE c.slug = 'linux-co-ban'
ON CONFLICT (slug) DO NOTHING;

INSERT INTO labs (course_id, slug, title, description_md, duration_minutes, lab_image_id, order_idx)
SELECT c.id, 'linux-lab-2', 'Quyền & Sở Hữu',
       E'# Quyền file\n\nDùng `chmod`, `chown`. Đặt quyền `755` cho script.',
       60, (SELECT id FROM lab_images WHERE name = 'devforge/linux' LIMIT 1), 1
FROM courses c WHERE c.slug = 'linux-co-ban'
ON CONFLICT (slug) DO NOTHING;

INSERT INTO lab_tasks (lab_id, title, points, check_script, order_idx)
SELECT l.id, 'Tạo thư mục ~/devforge', 10, 'test -d /home/student/devforge', 0
FROM labs l WHERE l.slug = 'linux-lab-1'
AND NOT EXISTS (SELECT 1 FROM lab_tasks t WHERE t.lab_id = l.id AND t.order_idx = 0);

INSERT INTO reviews (course_id, title, content_md, order_idx)
SELECT c.id, 'Tổng kết lệnh cơ bản',
       E'## Ôn tập\n\n- `ls -la` liệt kê chi tiết\n- `chmod 755` đặt quyền\n- `man <cmd>` xem hướng dẫn',
       0
FROM courses c WHERE c.slug = 'linux-co-ban'
AND NOT EXISTS (SELECT 1 FROM reviews r WHERE r.course_id = c.id AND r.order_idx = 0);
