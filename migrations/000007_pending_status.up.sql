-- A signup now lands as 'pending' and stays there until the emailed code is
-- entered. Existing rows are untouched: they predate verification and are
-- treated as already verified.
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'pending', 'banned'));
