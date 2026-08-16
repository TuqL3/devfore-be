-- Level labels are served straight to the UI, which is now English-only. The
-- slugs were already English and are referenced by courses.level, so only the
-- display text moves.
UPDATE levels SET label = 'Beginner',     hint = 'Never typed a terminal command' WHERE slug = 'beginner';
UPDATE levels SET label = 'Intermediate', hint = 'Comfortable with Linux, wants to go deeper' WHERE slug = 'intermediate';
UPDATE levels SET label = 'Advanced',     hint = 'Operates systems that run for real' WHERE slug = 'advanced';
