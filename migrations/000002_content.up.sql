CREATE TABLE lab_images (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT        NOT NULL,
    tag         TEXT        NOT NULL,
    description TEXT,
    active      BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (name, tag)
);

CREATE TABLE courses (
    id           BIGSERIAL PRIMARY KEY,
    slug         TEXT        NOT NULL UNIQUE,
    title        TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    image_url    TEXT,
    level        TEXT        NOT NULL DEFAULT 'beginner',
    status       TEXT        NOT NULL DEFAULT 'draft',
    published_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT courses_level_check  CHECK (level IN ('beginner', 'intermediate', 'advanced')),
    CONSTRAINT courses_status_check CHECK (status IN ('draft', 'published'))
);

CREATE TABLE labs (
    id               BIGSERIAL PRIMARY KEY,
    course_id        BIGINT      NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    slug             TEXT        NOT NULL UNIQUE,
    title            TEXT        NOT NULL,
    description_md   TEXT        NOT NULL DEFAULT '',
    duration_minutes INT         NOT NULL DEFAULT 60,
    lab_image_id     BIGINT      REFERENCES lab_images (id),
    order_idx        INT         NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_labs_course ON labs (course_id);

CREATE TABLE lab_tasks (
    id           BIGSERIAL PRIMARY KEY,
    lab_id       BIGINT NOT NULL REFERENCES labs (id) ON DELETE CASCADE,
    title        TEXT   NOT NULL,
    points       INT    NOT NULL DEFAULT 10,
    check_script TEXT   NOT NULL DEFAULT '',
    order_idx    INT    NOT NULL DEFAULT 0
);
CREATE INDEX idx_lab_tasks_lab ON lab_tasks (lab_id);

CREATE TABLE reviews (
    id         BIGSERIAL PRIMARY KEY,
    course_id  BIGINT NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    title      TEXT   NOT NULL,
    content_md TEXT   NOT NULL DEFAULT '',
    order_idx  INT    NOT NULL DEFAULT 0
);
CREATE INDEX idx_reviews_course ON reviews (course_id);

CREATE TABLE enrollments (
    user_id    BIGINT      NOT NULL REFERENCES users (id)   ON DELETE CASCADE,
    course_id  BIGINT      NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, course_id)
);
