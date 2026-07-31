-- A third kind of task: one graded by what the student typed rather than by what
-- the container looks like afterwards. Read-only commands — uname, which, cat
-- /proc/… — leave no trace to inspect, so the shell history is the only evidence
-- they were run.
ALTER TABLE lab_tasks
    -- One accepted command per line. A list rather than a single string because
    -- the same question usually has more than one right answer (`uname -a` and
    -- `uname --all`), and an author should not need a regex to say so.
    ADD COLUMN expected_commands TEXT NOT NULL DEFAULT '';

ALTER TABLE lab_tasks DROP CONSTRAINT lab_tasks_kind_check;
ALTER TABLE lab_tasks
    ADD CONSTRAINT lab_tasks_kind_check CHECK (kind IN ('script', 'choice', 'command'));
