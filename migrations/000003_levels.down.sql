DROP INDEX IF EXISTS courses_level_idx;
ALTER TABLE courses DROP CONSTRAINT IF EXISTS courses_level_fkey;
ALTER TABLE courses ADD CONSTRAINT courses_level_check
    CHECK (level IN ('beginner', 'intermediate', 'advanced'));
DROP TABLE IF EXISTS levels;
