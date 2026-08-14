-- A drill report the student chose to publish, addressed by a token instead of
-- by the session id.
--
-- NULL until they press Share: this row describes one person's own attempt, and
-- nothing about it goes public because a default said so. Pressing Share again
-- from the report is also how it comes back down, so publishing is reversible.
--
-- A separate token rather than reusing the session id. The id travels in the
-- authenticated URLs of the report and the terminal, and a public link is a
-- thing people paste into places that keep it — the two should not be the same
-- string.
ALTER TABLE lab_sessions ADD COLUMN share_token TEXT;

-- Unique among the rows that have one, and the lookup path for the public page.
-- Partial rather than a UNIQUE constraint: almost no session ever publishes, and
-- an index of NULLs is one the public read would have to walk past.
CREATE UNIQUE INDEX lab_sessions_share_token_idx
    ON lab_sessions (share_token) WHERE share_token IS NOT NULL;

-- The daily board's only query: everyone who drew today's scenario, newest
-- first. Partial on the same column the drill half of the platform already uses
-- to tell a drill session from an ordinary one.
CREATE INDEX lab_sessions_incident_day_idx
    ON lab_sessions (incident_id, started_at DESC)
    WHERE incident_id IS NOT NULL;
