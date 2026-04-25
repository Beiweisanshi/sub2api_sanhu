ALTER TABLE model_pricings
    ADD COLUMN IF NOT EXISTS fast_price_multiplier DOUBLE PRECISION;
