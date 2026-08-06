-- Dựng lại khung, không dựng lại nội dung: nội dung đã chuyển sang code và
-- không còn ở đâu trong database để lấy về.
CREATE TABLE sim_scenarios (
    id             BIGSERIAL   PRIMARY KEY,
    slug           TEXT        NOT NULL UNIQUE,
    title          TEXT        NOT NULL,
    description_md TEXT        NOT NULL DEFAULT '',
    guide_md       TEXT        NOT NULL DEFAULT '',
    scenario       JSONB       NOT NULL,
    published      BOOLEAN     NOT NULL DEFAULT false,
    order_idx      INT         NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sim_scenarios_published ON sim_scenarios (published, order_idx, id);

CREATE TABLE app_settings (
    key        TEXT        PRIMARY KEY,
    value      TEXT        NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
