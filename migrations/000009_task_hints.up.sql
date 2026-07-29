-- The lab runner shows a hint tab next to each task. Empty by default so every
-- existing task keeps working; the tab hides itself when there is nothing here.
ALTER TABLE lab_tasks ADD COLUMN hint TEXT NOT NULL DEFAULT '';
