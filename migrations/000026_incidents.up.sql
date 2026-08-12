-- One authored way of breaking a lab's service. A lab carries several of these
-- and Start picks one at random, so replaying the lab is a fresh drill rather
-- than a memory test.
--
-- Having an active row here is the whole of what makes a lab an incident lab.
-- No flag column says so: two things that can disagree eventually do, which is
-- the same rule labs.sim_scenario already follows.
CREATE TABLE lab_incidents (
    id           BIGSERIAL   PRIMARY KEY,
    lab_id       BIGINT      NOT NULL REFERENCES labs (id) ON DELETE CASCADE,
    -- Names the cause, so it is shown to the student only once the attempt is
    -- over. While the drill runs, the cause is the thing being worked out.
    title        TEXT        NOT NULL,
    -- The answer key, in the most literal sense: it runs inside the student's
    -- container and its content says exactly what is wrong. Same trust level as
    -- lab_tasks.check_script — authored by an admin, never sent to a client.
    break_script TEXT        NOT NULL,
    -- Read after the drill: what had happened, and how it is normally found.
    reveal_md    TEXT        NOT NULL DEFAULT '',
    -- Requests per second the outage is assumed to hurt, which is what turns
    -- elapsed time into a cost the student can feel. An authored number, not a
    -- measurement — the screen showing it has to say so, exactly as the
    -- simulator says its seconds are simulated.
    rps          INT         NOT NULL DEFAULT 20 CHECK (rps >= 0),
    -- Retiring a scenario is a flag rather than a delete, because sessions point
    -- at the row that broke them and their reports still have to read it back.
    active       BOOLEAN     NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The only query on this table: pick among a lab's usable scenarios.
CREATE INDEX idx_lab_incidents_lab ON lab_incidents (lab_id) WHERE active;

-- Builds the service the scenarios then break. On the lab rather than on each
-- scenario: every scenario of one lab breaks the same service, and three copies
-- of the setup would be three places for it to drift apart.
ALTER TABLE labs ADD COLUMN incident_setup TEXT NOT NULL DEFAULT '';

-- Which scenario this session drew. NULL for every session of a normal lab, and
-- for every row that predates this migration.
--
-- No ON DELETE clause on purpose: a scenario that has been played cannot be
-- deleted out from under the reports that describe it. Retire it with
-- active = false instead. Deleting the whole lab still works — that cascades to
-- the sessions as well, so nothing is left pointing at a missing row.
ALTER TABLE lab_sessions ADD COLUMN incident_id BIGINT REFERENCES lab_incidents (id);

-- The shell history of an incident session, read once when the session ends.
-- Only incident sessions write here: it is the raw record of what a person
-- typed, so the sessions that have no use for it do not carry it at all.
ALTER TABLE lab_sessions ADD COLUMN command_log TEXT NOT NULL DEFAULT '';
