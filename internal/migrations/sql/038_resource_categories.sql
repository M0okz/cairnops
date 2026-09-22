ALTER TABLE cairnops_targets ADD COLUMN category text
 CHECK (category IN ('service', 'infrastructure', 'scheduled_task', 'software', 'unclassified'));
COMMENT ON COLUMN cairnops_targets.category IS 'Manual resource category; NULL follows the category suggested from structured controls and inventory.';
