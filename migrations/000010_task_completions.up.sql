-- One row per task a user has ever passed. The primary key is what makes points
-- award exactly once: grading inserts with ON CONFLICT DO NOTHING and only adds
-- to the score when the insert actually wrote a row, so pressing the check
-- button twice is worth the same as pressing it once.
CREATE TABLE lab_task_completions (
    user_id   BIGINT      NOT NULL REFERENCES users (id)     ON DELETE CASCADE,
    task_id   BIGINT      NOT NULL REFERENCES lab_tasks (id) ON DELETE CASCADE,
    -- Which session it was done in. Not part of the key: the credit belongs to
    -- the user, not to a container that is removed an hour later.
    session_id TEXT       NOT NULL,
    passed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, task_id)
);

-- Reading back "which tasks in this lab has the user passed" is the query the
-- lab screen makes on every load, and it goes through task_id.
CREATE INDEX lab_task_completions_task_idx ON lab_task_completions (task_id);
