-- Puts the anchor back and re-attaches the challenges that have no course, so
-- that 000028's own down migration — which needs somewhere to park orphans —
-- finds nothing left to park.
--
-- Scores are not restored: a drill has earned none since 000028, because the
-- grader skips the scoreboard when there is no course. Going back does not
-- invent the rows that were never written.
INSERT INTO courses (slug, title, description, level, status)
SELECT 'truc-su-co', 'Trực Sự Cố',
       'Chỗ neo dữ liệu cho các thử thách War Room. Không phải khoá học, không hiện ở danh sách khoá.',
       'intermediate', 'draft'
 WHERE EXISTS (
     SELECT 1 FROM labs l
      WHERE l.course_id IS NULL
        AND EXISTS (SELECT 1 FROM lab_incidents i WHERE i.lab_id = l.id))
ON CONFLICT (slug) DO NOTHING;

UPDATE labs l
   SET course_id = (SELECT id FROM courses WHERE slug = 'truc-su-co')
 WHERE l.course_id IS NULL
   AND EXISTS (SELECT 1 FROM lab_incidents i WHERE i.lab_id = l.id);
