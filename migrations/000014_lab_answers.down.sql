DROP INDEX IF EXISTS lab_sessions_user_history_idx;

-- Rows already submitted would fail the narrower check, so they settle back to
-- the status that meant the same thing before this migration existed.
UPDATE lab_sessions SET status = 'ended' WHERE status = 'submitted';

ALTER TABLE lab_sessions DROP CONSTRAINT lab_sessions_status_check;
ALTER TABLE lab_sessions ADD CONSTRAINT lab_sessions_status_check
    CHECK (status IN ('running', 'ended', 'expired'));

ALTER TABLE lab_sessions DROP COLUMN submitted_at;

DROP TABLE lab_answers;
