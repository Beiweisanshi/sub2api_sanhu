CREATE TABLE IF NOT EXISTS model_pricings (
    id BIGSERIAL PRIMARY KEY,
    model_name TEXT NOT NULL UNIQUE,
    input_cost_per_token DOUBLE PRECISION,
    output_cost_per_token DOUBLE PRECISION,
    cache_read_input_token_cost DOUBLE PRECISION,
    cache_creation_input_token_cost DOUBLE PRECISION,
    is_custom BOOLEAN NOT NULL DEFAULT FALSE,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_model_pricings_updated_at ON model_pricings(updated_at DESC);
