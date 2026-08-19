ALTER TABLE cairnops_signal_sources
    ADD COLUMN heartbeat_token_digest bytea;

ALTER TABLE cairnops_signal_sources
    ADD COLUMN last_signal_outcome text;

ALTER TABLE cairnops_signal_sources
    ADD CONSTRAINT cairnops_signal_sources_heartbeat_digest_length
    CHECK (heartbeat_token_digest IS NULL OR octet_length(heartbeat_token_digest) = 32);

CREATE UNIQUE INDEX cairnops_signal_sources_heartbeat_digest_idx
    ON cairnops_signal_sources (heartbeat_token_digest)
    WHERE heartbeat_token_digest IS NOT NULL;

ALTER TABLE cairnops_signal_sources
    ADD CONSTRAINT cairnops_signal_sources_last_signal_outcome
    CHECK (last_signal_outcome IS NULL OR last_signal_outcome IN ('healthy', 'unhealthy'));
