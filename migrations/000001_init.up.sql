-- P0: chỉ users + roles. Các bảng còn lại thêm ở P1-P7 theo lộ trình.

CREATE TABLE roles (
    id   SMALLSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

INSERT INTO roles (name) VALUES ('student'), ('admin');

CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT        NOT NULL UNIQUE,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT,
    google_id     TEXT UNIQUE,
    avatar_url    TEXT,
    status        TEXT        NOT NULL DEFAULT 'active',
    banned_reason TEXT,
    banned_at     TIMESTAMPTZ,
    banned_by     BIGINT REFERENCES users (id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT users_status_check CHECK (status IN ('active', 'banned')),
    -- Tài khoản phải đăng nhập được bằng ít nhất một cách.
    CONSTRAINT users_auth_method_check CHECK (password_hash IS NOT NULL OR google_id IS NOT NULL)
);

CREATE TABLE user_roles (
    user_id BIGINT   NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id SMALLINT NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);
