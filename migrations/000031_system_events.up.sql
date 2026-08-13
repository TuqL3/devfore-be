-- Things that went wrong, kept where somebody can look at them.
--
-- Until now every one of these went to slog and left with the container: a start
-- that failed, a check script that timed out, an incident script that exited
-- non-zero, a host that ran out of room. The person who most needs to know is
-- the admin, and the admin has no shell on the box.
--
-- Not a log replacement. slog still gets everything, with the stack and the
-- request id; this table gets the handful of events an admin would act on, in a
-- shape a screen can list. Anything that needs grepping belongs in Loki.
CREATE TABLE system_events (
    id       BIGSERIAL   PRIMARY KEY,
    at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Dotted name of what happened: `lab.start_failed`, `check.timeout`,
    -- `incident.script_failed`, `capacity.refused`, `reaper.orphan`.
    -- A string rather than an enum: a new kind must not need a migration, and
    -- the screen groups by whatever it finds.
    kind     TEXT        NOT NULL,
    severity TEXT        NOT NULL DEFAULT 'error',
    -- Who it happened to, when that is known. NULL for events about the host
    -- rather than about a person — the reaper's sweep belongs to nobody.
    -- ON DELETE SET NULL: deleting an account must not take the record of a
    -- system fault with it.
    actor_id BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    -- What it happened to: a session id, a lab slug, a container id. Free text
    -- because the answer is a different kind of thing per event, and a column
    -- per kind would be a table nobody could add a row to.
    subject  TEXT        NOT NULL DEFAULT '',
    -- One sentence, already written for a person to read.
    detail   TEXT        NOT NULL DEFAULT '',

    CONSTRAINT system_events_severity_check
        CHECK (severity IN ('info', 'warn', 'error'))
);

-- The only two queries: the feed, newest first, and the feed filtered to one
-- kind. Both read the recent end, which is why `at` leads.
CREATE INDEX system_events_at_idx   ON system_events (at DESC);
CREATE INDEX system_events_kind_idx ON system_events (kind, at DESC);
