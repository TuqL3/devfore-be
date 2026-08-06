-- Scenarios for the playground: the simulator with no lab attached, no tasks and
-- no marks. A separate table rather than a flag on labs, because these two are
-- different things that happen to share an engine — a lab scenario exists to be
-- graded against, and one of these exists to be played with.
--
-- Nothing here references labs or sessions. A run in the playground is not
-- stored at all: it is computed, drawn, and forgotten, which is the whole reason
-- this can be opened without enrolling in anything.
CREATE TABLE sim_scenarios (
    id             BIGSERIAL   PRIMARY KEY,
    slug           TEXT        NOT NULL UNIQUE,
    title          TEXT        NOT NULL,
    description_md TEXT        NOT NULL DEFAULT '',
    scenario       JSONB       NOT NULL,
    -- Drafts are unfinished, not hidden: an author writing a catalogue should
    -- not have half of it live on the playground while they think.
    published  BOOLEAN     NOT NULL DEFAULT false,
    order_idx  INT         NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The listing is "published, in the author's order", which is this index read
-- forwards. No second index for the admin listing: it reads every row anyway.
CREATE INDEX idx_sim_scenarios_published ON sim_scenarios (published, order_idx, id);
