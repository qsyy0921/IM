CREATE INDEX IF NOT EXISTS idx_identity_challenges_request_window
    ON identity_challenges (tenant_id, user_id, challenge_type, channel, destination, issued_at DESC);
