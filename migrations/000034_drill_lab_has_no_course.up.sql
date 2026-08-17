-- A lab carrying a row in lab_incidents is a War Room drill, and Start exempts
-- drills from the enrolment gate (internal/labs/usecase/labs.go: the enrolment
-- check sits behind `if !spec.IsIncident`). Since migration 000028 a drill lab
-- is exactly a lab with no course, and that is the whole reason the exemption
-- is safe: there is no course to enrol in.
--
-- Nothing enforced the pairing. Attach an incident to a lab of a real course
-- and that lab quietly stops asking for enrolment while every sibling lab of
-- the same course still asks — a hole that looks like working software from
-- every screen. The rule lived only in the habit of whoever wrote the seed.
--
-- In the database rather than in Go because incidents arrive by migration and
-- by psql as well as through the admin API. A rule that only one of those three
-- paths honours is not a rule.

CREATE FUNCTION drill_lab_has_no_course() RETURNS trigger AS $$
BEGIN
    IF (SELECT course_id FROM labs WHERE id = NEW.lab_id) IS NOT NULL THEN
        RAISE EXCEPTION
            'lab % belongs to a course, so it must not carry an incident: a lab with an incident skips the enrolment gate',
            NEW.lab_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER lab_incidents_lab_has_no_course
    BEFORE INSERT OR UPDATE OF lab_id ON lab_incidents
    FOR EACH ROW EXECUTE FUNCTION drill_lab_has_no_course();

-- The same hole opens from the other side: moving an existing drill lab into a
-- course never touches lab_incidents, so the trigger above would never see it.
CREATE FUNCTION lab_with_incident_keeps_no_course() RETURNS trigger AS $$
BEGIN
    IF NEW.course_id IS NOT NULL
       AND EXISTS (SELECT 1 FROM lab_incidents WHERE lab_id = NEW.id) THEN
        RAISE EXCEPTION
            'lab % carries an incident, so it must not be given a course: a lab with an incident skips the enrolment gate',
            NEW.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER labs_with_incident_keep_no_course
    BEFORE UPDATE OF course_id ON labs
    FOR EACH ROW EXECUTE FUNCTION lab_with_incident_keeps_no_course();
