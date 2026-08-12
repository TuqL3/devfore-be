-- Lossy, and the loss is the content: the scenarios themselves and the command
-- log of every incident session go with these drops. No session, grade or point
-- is touched — the sessions stay, they only stop remembering which fault they
-- were handed.
ALTER TABLE lab_sessions DROP COLUMN command_log;
ALTER TABLE lab_sessions DROP COLUMN incident_id;
ALTER TABLE labs DROP COLUMN incident_setup;
DROP TABLE lab_incidents;
