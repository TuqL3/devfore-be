-- Not every question needs a container. Theory questions are answered by
-- picking from a list, and running a shell to grade them would be a container
-- started to compare two strings.
ALTER TABLE lab_tasks
    ADD COLUMN kind    TEXT  NOT NULL DEFAULT 'script',
    -- [{"text": "...", "correct": true}, …]. One column rather than a list of
    -- options plus a list of answers: they are written together, read together,
    -- and two columns could disagree about how many options there are.
    ADD COLUMN options JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE lab_tasks
    ADD CONSTRAINT lab_tasks_kind_check CHECK (kind IN ('script', 'choice'));
