-- One row per attempt at a lab. The reaper reads expires_at from here rather
-- than from a timer in the process, so a restarted server still cleans up the
-- containers it forgot about and a closed browser tab still gets reaped.
CREATE TABLE lab_sessions (
    id           TEXT        PRIMARY KEY,
    user_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    lab_id       BIGINT      NOT NULL REFERENCES labs (id)  ON DELETE CASCADE,
    container_id TEXT        NOT NULL DEFAULT '',
    status       TEXT        NOT NULL DEFAULT 'running',
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    ended_at     TIMESTAMPTZ,

    CONSTRAINT lab_sessions_status_check
        CHECK (status IN ('running', 'ended', 'expired'))
);

-- One live container per student, enforced where two requests racing each other
-- cannot both win. A check in Go would let a double-clicked Start button leak a
-- container that nothing owns.
CREATE UNIQUE INDEX lab_sessions_one_running_per_user
    ON lab_sessions (user_id) WHERE status = 'running';

-- The reaper's only query: running rows already past their deadline.
CREATE INDEX lab_sessions_reaper_idx
    ON lab_sessions (expires_at) WHERE status = 'running';
