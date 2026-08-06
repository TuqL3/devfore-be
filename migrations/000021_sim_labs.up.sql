-- A lab graded by a simulated CI/CD pipeline instead of by a container. NULL on
-- every existing row, and the column being NULL is what makes a lab a container
-- lab: there is no second flag that could fall out of step with this one.
--
-- The scenario is data rather than code so that a new pipeline lab is a row, not
-- a deploy — the same rule the rest of the course content already follows.
ALTER TABLE labs ADD COLUMN sim_scenario JSONB;

-- A lab is one kind or the other. Written as "not both" rather than "exactly
-- one" because lab_image_id has always been nullable and rows already exist with
-- neither: a stricter check would fail on data that predates this migration and
-- has nothing to do with it.
ALTER TABLE labs ADD CONSTRAINT labs_runtime_check
    CHECK (NOT (lab_image_id IS NOT NULL AND sim_scenario IS NOT NULL));

-- What passing a sim task means, read against the final state of the student's
-- most recent run. Defaulted to an empty object and rejected at grading time
-- rather than here: an author saving a half-written task should not be stopped by
-- the database, but an empty goal must never be allowed to pass a student.
ALTER TABLE lab_tasks ADD COLUMN sim_goal JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE lab_tasks DROP CONSTRAINT lab_tasks_kind_check;
ALTER TABLE lab_tasks ADD CONSTRAINT lab_tasks_kind_check
    CHECK (kind IN ('script', 'choice', 'command', 'sim'));

-- Every press of Run. The pipeline is stored alongside the result because the
-- result on its own cannot be checked afterwards: grading reads the stored
-- result, and without the input that produced it there is no way to tell a
-- faithful replay from a rewrite.
CREATE TABLE sim_runs (
    id         BIGSERIAL   PRIMARY KEY,
    session_id TEXT        NOT NULL REFERENCES lab_sessions (id) ON DELETE CASCADE,
    -- Counts from 1 within a session and feeds the failure seed, so re-running an
    -- unchanged pipeline is a different run rather than the same one twice. That
    -- is deliberate: a flaky step has to be able to pass on a retry, or the lab
    -- teaches the opposite of what flakiness is.
    run_index  INT         NOT NULL,
    pipeline   TEXT        NOT NULL,
    result     JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Grading reads the newest run of a session, which is this index scanned
    -- backwards. No separate index for that: it is the same one.
    UNIQUE (session_id, run_index)
);
