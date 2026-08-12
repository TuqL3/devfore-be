-- Retires the anchor course.
--
-- Its own description said what it was: "Chỗ neo dữ liệu cho các thử thách War
-- Room. Không phải khoá học". It existed only because labs.course_id was NOT
-- NULL, and 000028 removed that reason. A row in `courses` that is not a course
-- is a lie every future reader has to decode, so it goes.
--
-- Order matters and is not stylistic: labs.course_id cascades on delete, so
-- dropping the course first would take its challenges with it. Detach, then
-- delete.
UPDATE labs
   SET course_id = NULL, updated_at = now()
 WHERE course_id = (SELECT id FROM courses WHERE slug = 'truc-su-co');

-- Only when it is empty. If somebody filed real course material under it, this
-- leaves it alone rather than deleting content on a guess.
DELETE FROM courses c
 WHERE c.slug = 'truc-su-co'
   AND NOT EXISTS (SELECT 1 FROM labs l WHERE l.course_id = c.id);
