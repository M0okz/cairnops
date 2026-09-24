ALTER TABLE cairnops_targets DROP CONSTRAINT cairnops_targets_category_check;
ALTER TABLE cairnops_targets ADD CONSTRAINT cairnops_targets_category_check
 CHECK (category IN (
  'service', 'infrastructure', 'virtual_machine', 'container',
  'virtualization_host', 'storage', 'host', 'network', 'application',
  'scheduled_task', 'software', 'unclassified'
 ));
