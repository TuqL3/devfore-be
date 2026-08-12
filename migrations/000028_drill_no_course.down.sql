-- Restoring NOT NULL needs somewhere to put the challenges that have no course.
--
-- Deleting them would be the short way and it throws away content, along with
-- every session and report that points at them. Instead they are parked in one
-- clearly-named draft course, which is visible to an admin and to nobody else.
-- Whoever rolls forward again can detach them, or leave them there.
WITH parked AS (
    INSERT INTO courses (slug, title, description, level, status)
    SELECT 'war-room-khoi-phuc',
           'War Room (khôi phục)',
           'Giữ chỗ cho các thử thách không thuộc khoá nào, tạo khi lùi migration 000028.',
           'advanced',
           'draft'
     WHERE EXISTS (SELECT 1 FROM labs WHERE course_id IS NULL)
    ON CONFLICT (slug) DO NOTHING
    RETURNING id
)
UPDATE labs
   SET course_id = COALESCE(
         (SELECT id FROM parked),
         (SELECT id FROM courses WHERE slug = 'war-room-khoi-phuc'))
 WHERE course_id IS NULL;

DROP INDEX IF EXISTS idx_labs_no_course;
ALTER TABLE labs ALTER COLUMN course_id SET NOT NULL;
