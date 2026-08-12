-- Loses only the flag. Every scenario, session, grade and point survives.
--
-- Going back makes War Room visible again for any lab with an active scenario,
-- which is what it did before this migration — a drill that had been taken down
-- by drafting it comes back. Switch its scenarios off first if that matters.
DROP INDEX IF EXISTS idx_labs_published_drill;
ALTER TABLE labs DROP COLUMN drill_status;
