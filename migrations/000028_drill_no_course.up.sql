-- A War Room challenge does not belong to a course.
--
-- The code has said so for a while — `Start` skips the enrolment check for a
-- drill, and the comment there calls the course row "a place for it to live,
-- not a gate". This is the schema catching up with that: the place to live is
-- now optional, so a challenge can exist without inventing a course to file it
-- under.
--
-- Existing drills keep whatever course they were created in. Nothing is moved:
-- a column becoming nullable does not oblige any row to become null, and
-- rewriting somebody's content is not a migration's business.
ALTER TABLE labs ALTER COLUMN course_id DROP NOT NULL;

-- Every query that scoped a lab to its course still reads `course_id = ?`, so a
-- course-less lab simply never matches one — which is the behaviour wanted, not
-- a gap. The queries that JOIN courses to name one became LEFT JOINs in the same
-- change; without that they would silently drop every course-less drill from
-- reports and admin stats.
CREATE INDEX idx_labs_no_course ON labs (id) WHERE course_id IS NULL;
