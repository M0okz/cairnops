-- Only aggregate metadata from provider responses; prompts and credentials are never retained.
CREATE TABLE cairnops_software_ai_usage (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    endpoint text NOT NULL,
    model text NOT NULL,
    prompt_tokens bigint NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens bigint NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0),
    total_tokens bigint NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    reported boolean NOT NULL
);
CREATE INDEX ON cairnops_software_ai_usage(endpoint, created_at DESC);
