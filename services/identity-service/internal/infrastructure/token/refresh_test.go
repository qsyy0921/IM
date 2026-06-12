package token

import "testing"

func TestRefreshTokenCodecRoundTrip(t *testing.T) {
	codec := NewRefreshTokenCodec()
	plain, record, err := codec.NewRefreshToken()
	if err != nil {
		t.Fatalf("new refresh token: %v", err)
	}
	parsed, err := codec.ParseRefreshToken(plain)
	if err != nil {
		t.Fatalf("parse refresh token: %v", err)
	}
	if parsed.TokenID != record.TokenID {
		t.Fatalf("expected token id %s, got %s", record.TokenID, parsed.TokenID)
	}
	if got := codec.HashRefreshTokenSecret(parsed.Secret); got != record.TokenHash {
		t.Fatalf("expected hash %s, got %s", record.TokenHash, got)
	}
}

func TestRefreshTokenCodecRejectsMalformedToken(t *testing.T) {
	codec := NewRefreshTokenCodec()
	if _, err := codec.ParseRefreshToken("bad-token"); err == nil {
		t.Fatal("expected malformed token to fail")
	}
}
