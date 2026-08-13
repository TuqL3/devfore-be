-- Whether a War Room challenge is offered to students, held apart from whether
-- its scenarios can be drawn.
--
-- Until now the two were the same question: a lab appeared in War Room when any
-- of its scenarios was active, so taking a challenge down meant switching every
-- scenario off — and putting it back meant remembering which ones had been on.
-- The scenario flag answers "can this fault be drawn"; this one answers "is this
-- challenge open at all", and they are not the same decision.
--
-- Same vocabulary as courses.status on purpose. An author already knows what
-- draft and published mean here, and a second pair of words for the same idea is
-- a second thing to explain.
--
-- On labs rather than on a table of its own: a drill IS a lab, exactly as
-- incident_setup is a column of one. A side table would mean a join on the
-- student-facing War Room query to answer a single boolean.
ALTER TABLE labs ADD COLUMN drill_status TEXT NOT NULL DEFAULT 'draft'
    CONSTRAINT labs_drill_status_check CHECK (drill_status IN ('draft', 'published'));

-- Everything that is live right now stays live. Without this every existing
-- challenge would fall to the default and vanish from War Room the moment this
-- migration ran, which is a content outage caused by a schema change.
--
-- The condition is exactly what the student-facing query tested before this
-- column existed, so the set of visible drills is unchanged across the upgrade.
UPDATE labs l
   SET drill_status = 'published'
 WHERE EXISTS (SELECT 1 FROM lab_incidents i WHERE i.lab_id = l.id AND i.active);

-- The one query that reads it is the War Room listing, which already filters on
-- having an active scenario. Partial, because a published drill is the rare row:
-- most labs are ordinary course labs and will sit at the default forever.
CREATE INDEX idx_labs_published_drill ON labs (id) WHERE drill_status = 'published';
