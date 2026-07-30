-- A task saved with no options stored JSON `null`, and every read of its lab —
-- including the public one — runs jsonb_array_elements over that column, which
-- errors on a scalar. One such row was enough to 500 the whole lab page.
UPDATE lab_tasks SET options = '[]'::jsonb WHERE jsonb_typeof(options) <> 'array';

-- The application no longer writes anything else, and this makes sure nothing
-- else ever does: the failure it causes shows up far from the write that caused
-- it, which is the worst kind to debug.
ALTER TABLE lab_tasks
    ADD CONSTRAINT lab_tasks_options_array_check CHECK (jsonb_typeof(options) = 'array');
