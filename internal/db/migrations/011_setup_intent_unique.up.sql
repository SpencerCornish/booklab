CREATE UNIQUE INDEX idx_bookings_setup_intent
    ON bookings (stripe_setup_intent_id)
    WHERE stripe_setup_intent_id IS NOT NULL;
