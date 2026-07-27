
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

-- ---------------------------------------------------------------------------
-- Demo leaderboard. These are seed accounts, not real users: passwords are a
-- bcrypt hash of a throwaway string so nobody can log in as them by guessing.
-- ---------------------------------------------------------------------------
INSERT INTO users (username, email, password_hash, status)
VALUES
    ('minhtran',  'minhtran@example.com',  '$2a$10$ZHqZm4jSgAAqEV.9hEV0AeKZzZ3M0Y6hb2wLZ3E1eDkZq3lC4kV1O', 'active'),
    ('halinh',    'halinh@example.com',    '$2a$10$ZHqZm4jSgAAqEV.9hEV0AeKZzZ3M0Y6hb2wLZ3E1eDkZq3lC4kV1O', 'active'),
    ('ducanh',    'ducanh@example.com',    '$2a$10$ZHqZm4jSgAAqEV.9hEV0AeKZzZ3M0Y6hb2wLZ3E1eDkZq3lC4kV1O', 'active'),
    ('thaonguyen','thaonguyen@example.com','$2a$10$ZHqZm4jSgAAqEV.9hEV0AeKZzZ3M0Y6hb2wLZ3E1eDkZq3lC4kV1O', 'active'),
    ('quangvu',   'quangvu@example.com',   '$2a$10$ZHqZm4jSgAAqEV.9hEV0AeKZzZ3M0Y6hb2wLZ3E1eDkZq3lC4kV1O', 'active'),
    ('phuongmai', 'phuongmai@example.com', '$2a$10$ZHqZm4jSgAAqEV.9hEV0AeKZzZ3M0Y6hb2wLZ3E1eDkZq3lC4kV1O', 'active'),
    ('baoson',    'baoson@example.com',    '$2a$10$ZHqZm4jSgAAqEV.9hEV0AeKZzZ3M0Y6hb2wLZ3E1eDkZq3lC4kV1O', 'active')
ON CONFLICT (email) DO NOTHING;

INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u CROSS JOIN roles r
WHERE u.email LIKE '%@example.com' AND r.name = 'student'
ON CONFLICT DO NOTHING;

-- Enrol the demo users, then give them scores. Every course gets a board.
INSERT INTO enrollments (user_id, course_id)
SELECT u.id, c.id FROM users u CROSS JOIN courses c
WHERE u.email LIKE '%@example.com'
ON CONFLICT DO NOTHING;

INSERT INTO course_scores (user_id, course_id, score, labs_completed, attempts, updated_at)
SELECT u.id, c.id, s.score, s.labs, s.attempts, now() - (s.hours_ago || ' hours')::interval
FROM courses c
JOIN (VALUES
    ('minhtran@example.com',  480, 2, 3,  2),
    ('halinh@example.com',    455, 2, 5,  6),
    ('ducanh@example.com',    390, 2, 4, 20),
    ('thaonguyen@example.com',330, 1, 2, 26),
    ('quangvu@example.com',   275, 1, 6, 50),
    ('phuongmai@example.com', 210, 1, 3, 74),
    ('baoson@example.com',     95, 0, 1, 96)
) AS s(email, score, labs, attempts, hours_ago) ON true
JOIN users u ON u.email = s.email
ON CONFLICT (user_id, course_id) DO UPDATE SET
    score          = EXCLUDED.score,
    labs_completed = EXCLUDED.labs_completed,
    attempts       = EXCLUDED.attempts,
    updated_at     = EXCLUDED.updated_at;
