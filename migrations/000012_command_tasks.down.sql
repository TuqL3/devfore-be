DELETE FROM lab_tasks WHERE kind = 'command';

ALTER TABLE lab_tasks DROP CONSTRAINT lab_tasks_kind_check;
ALTER TABLE lab_tasks
    ADD CONSTRAINT lab_tasks_kind_check CHECK (kind IN ('script', 'choice'));

ALTER TABLE lab_tasks DROP COLUMN expected_commands;
