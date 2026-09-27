-- Les notes officielles restent consultables après une nouvelle version installée,
-- même lorsqu'aucun fournisseur d'analyse IA n'est configuré.
CREATE TABLE cairnops_software_note_archives (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    binding_id uuid NOT NULL REFERENCES cairnops_software_services(binding_id),
    installed_version text NOT NULL,
    target_version text NOT NULL,
    source jsonb NOT NULL,
    content_hash text NOT NULL,
    notes jsonb NOT NULL,
    incomplete boolean NOT NULL DEFAULT false,
    captured_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (binding_id, installed_version, target_version, source, content_hash)
);
CREATE INDEX ON cairnops_software_note_archives(binding_id, id DESC);

INSERT INTO cairnops_software_note_archives
    (binding_id, installed_version, target_version, source, content_hash, notes, captured_at)
SELECT binding_id, installed_version, target_version, source, content_hash, notes, created_at
FROM cairnops_software_analyses
WHERE jsonb_array_length(notes) > 0
ON CONFLICT DO NOTHING;

INSERT INTO cairnops_software_note_archives
    (binding_id, installed_version, target_version, source, content_hash, notes, incomplete)
SELECT binding_id, collection->>'installed_version', collection->>'target_version', source,
       content_hash, collection->'notes', coalesce((collection->>'incomplete')::boolean, false)
FROM cairnops_software_services
WHERE collection IS NOT NULL AND jsonb_array_length(collection->'notes') > 0
ON CONFLICT DO NOTHING;
