DROP TABLE sim_runs;

-- Tasks of the kind this migration introduced would fail the narrower check, and
-- no older kind means the same thing: a sim goal cannot be rewritten as a script
-- or a list of options. They go with the column that gave them meaning, and the
-- answers recorded against them go too, by the cascade already on lab_answers.
--
-- This direction is lossy. Rolling back after students have used a sim lab
-- discards their attempts at it.
DELETE FROM lab_tasks WHERE kind = 'sim';

ALTER TABLE lab_tasks DROP CONSTRAINT lab_tasks_kind_check;
ALTER TABLE lab_tasks ADD CONSTRAINT lab_tasks_kind_check
    CHECK (kind IN ('script', 'choice', 'command'));

ALTER TABLE lab_tasks DROP COLUMN sim_goal;

ALTER TABLE labs DROP CONSTRAINT labs_runtime_check;

-- Labs that were sim labs are left behind with neither an image nor a scenario.
-- They are not deleted, because a lab row carries a title, a description and an
-- order a course was built around; what is lost is the scenario, and the lab
-- stops being startable until an author gives it an image.
ALTER TABLE labs DROP COLUMN sim_scenario;
