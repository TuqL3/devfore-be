-- Login folds the whole input to lower case before matching, so without this
-- index "DunTT" and "duntt" could both exist and the lookup would pick one of
-- them by row order. The plain UNIQUE on username cannot express that.
--
-- This fails if such a pair is already in the table. That is the point — the
-- rows have to be reconciled by hand first. Find them with:
--   SELECT lower(username), count(*) FROM users GROUP BY 1 HAVING count(*) > 1;
CREATE UNIQUE INDEX users_username_lower_key ON users (lower(username));
