-- How many times the student pressed check on this task in this session. The row
-- itself only ever holds the last answer, so without this a question fixed on the
-- fifth try and one right on the first are indistinguishable in the report — and
-- the report claimed everyone got everything right on the first go.
--
-- Nullable on purpose, with no backfill: sessions recorded before this column
-- existed have no attempt count, and writing 1 into them would be inventing the
-- very number the column exists to stop guessing. The report shows those as
-- unknown rather than as first-try passes.
ALTER TABLE lab_answers ADD COLUMN attempts INT;

ALTER TABLE lab_answers ADD CONSTRAINT lab_answers_attempts_check
    CHECK (attempts IS NULL OR attempts >= 1);
