-- Lossy, and safely so: nothing else points at this table and no run was ever
-- stored against it. What is lost is the scenarios an author wrote, which the
-- seed puts back for the ones it shipped.
DROP TABLE sim_scenarios;
