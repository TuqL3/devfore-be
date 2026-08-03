-- What the student answered, kept per session. lab_task_completions says a task
-- was ever passed and is keyed by user, which is right for scoring and useless
-- for a report: doing a lab twice leaves one row there and has to leave two
-- histories here.
CREATE TABLE lab_answers (
    session_id  TEXT        NOT NULL REFERENCES lab_sessions (id) ON DELETE CASCADE,
    task_id     BIGINT      NOT NULL REFERENCES lab_tasks (id)    ON DELETE CASCADE,
    -- Zero-based option indexes, matching the order the student was shown.
    -- Empty for a script or command task, which has nothing to tick.
    selected    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    passed      BOOLEAN     NOT NULL,
    answered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- The last answer is the answer: pressing check again replaces this row
    -- rather than adding one, so the report shows what was handed in.
    PRIMARY KEY (session_id, task_id)
);

-- Handing the lab in ends the session and freezes its report. Separate from
-- ended_at because the two mean different things to a student: one is "I am
-- done with this", the other is "the container is gone".
ALTER TABLE lab_sessions ADD COLUMN submitted_at TIMESTAMPTZ;

ALTER TABLE lab_sessions DROP CONSTRAINT lab_sessions_status_check;
ALTER TABLE lab_sessions ADD CONSTRAINT lab_sessions_status_check
    CHECK (status IN ('running', 'ended', 'expired', 'submitted'));

-- The history list reads a student's sessions newest first.
CREATE INDEX lab_sessions_user_history_idx
    ON lab_sessions (user_id, started_at DESC);
