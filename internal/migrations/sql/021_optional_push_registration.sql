-- L'appareil est appairé avant la décision système concernant les
-- notifications. Le destinataire opaque n'existe donc qu'après autorisation
-- et enregistrement auprès du Relais Push.
ALTER TABLE cairnops_devices
    ALTER COLUMN push_recipient_sealed DROP NOT NULL;

DO $$
DECLARE
    pairing_claim_constraint text;
BEGIN
    SELECT constraint_row.conname
    INTO pairing_claim_constraint
    FROM pg_constraint constraint_row
    WHERE constraint_row.conrelid = 'cairnops_device_pairings'::regclass
      AND constraint_row.contype = 'c'
      AND pg_get_constraintdef(constraint_row.oid) LIKE '%claimed_at IS NULL%'
      AND pg_get_constraintdef(constraint_row.oid) LIKE '%claimed_push_recipient_sealed IS NOT NULL%'
    LIMIT 1;

    IF pairing_claim_constraint IS NOT NULL THEN
        EXECUTE format(
            'ALTER TABLE cairnops_device_pairings DROP CONSTRAINT %I',
            pairing_claim_constraint
        );
    END IF;
END
$$;

ALTER TABLE cairnops_device_pairings
    ADD CONSTRAINT cairnops_device_pairings_claim_complete_check
    CHECK (
        claimed_at IS NULL
        OR (
            claimed_name IS NOT NULL
            AND claimed_platform IS NOT NULL
            AND claimed_app_version IS NOT NULL
            AND claimed_locale IS NOT NULL
            AND claimed_notification_content IS NOT NULL
            AND claimed_encryption_public_key IS NOT NULL
        )
    );
