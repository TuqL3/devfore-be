DROP TRIGGER IF EXISTS labs_with_incident_keep_no_course ON labs;
DROP FUNCTION IF EXISTS lab_with_incident_keeps_no_course();

DROP TRIGGER IF EXISTS lab_incidents_lab_has_no_course ON lab_incidents;
DROP FUNCTION IF EXISTS drill_lab_has_no_course();
