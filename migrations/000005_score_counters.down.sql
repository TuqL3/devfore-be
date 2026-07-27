ALTER TABLE course_scores
    DROP COLUMN IF EXISTS labs_completed,
    DROP COLUMN IF EXISTS attempts;
