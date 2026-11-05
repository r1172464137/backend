-- A verification cannot be un-proved: the rows marked verified are
-- indistinguishable from bindings their owners proved, so rolling back keeps
-- the flag and only has to run.
SELECT 1;
