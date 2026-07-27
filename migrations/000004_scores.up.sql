-- One row per (user, course): the running total the leaderboard reads. Task
-- grading will write here; until then it is filled by scripts/seed.sql.
CREATE TABLE course_scores (
    user_id    BIGINT      NOT NULL REFERENCES users (id)   ON DELETE CASCADE,
    course_id  BIGINT      NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    score      INT         NOT NULL DEFAULT 0 CHECK (score >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, course_id)
);

-- The leaderboard reads one course at a time, ordered by score.
CREATE INDEX course_scores_board_idx ON course_scores (course_id, score DESC);
