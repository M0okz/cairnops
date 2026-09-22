-- Retry oversized catalogues with adaptive, bounded pagination.
UPDATE cairnops_software_services SET next_check_at=now()
 WHERE state='retry' AND last_error='response too large';
