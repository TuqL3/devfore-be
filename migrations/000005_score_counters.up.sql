-- Per-(user, course) counters shown on the leaderboard. Kept next to score
-- because they are updated by the same grading write.
ALTER TABLE course_scores
    ADD COLUMN labs_completed INT NOT NULL DEFAULT 0 CHECK (labs_completed >= 0),
    ADD COLUMN attempts       INT NOT NULL DEFAULT 0 CHECK (attempts >= 0);
