-- Dropping the column takes every published link down with it. That is the
-- honest rollback: the public pages those tokens address stop existing, and
-- there is no half-state where a link resolves to a row that cannot say whether
-- it was meant to be public.
DROP INDEX IF EXISTS lab_sessions_incident_day_idx;
DROP INDEX IF EXISTS lab_sessions_share_token_idx;
ALTER TABLE lab_sessions DROP COLUMN IF EXISTS share_token;
