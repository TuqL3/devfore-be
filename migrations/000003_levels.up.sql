-- Levels were a CHECK constraint plus labels hardcoded in the frontend. They
-- are data, so they live in a table the API can serve.
CREATE TABLE levels (
    slug  TEXT PRIMARY KEY,
    label TEXT     NOT NULL,
    hint  TEXT     NOT NULL DEFAULT '',
    rank  SMALLINT NOT NULL UNIQUE
);

INSERT INTO levels (slug, label, hint, rank) VALUES
    ('beginner',     'Cơ bản',    'Chưa từng gõ lệnh terminal',   1),
    ('intermediate', 'Trung cấp', 'Đã quen Linux, muốn đi sâu',   2),
    ('advanced',     'Nâng cao',  'Vận hành hệ thống chạy thật',  3);

ALTER TABLE courses DROP CONSTRAINT courses_level_check;
ALTER TABLE courses ADD CONSTRAINT courses_level_fkey
    FOREIGN KEY (level) REFERENCES levels (slug);

CREATE INDEX courses_level_idx ON courses (level);
