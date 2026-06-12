package credential

import "testing"

func TestPBKDF2HasherVerifiesPassword(t *testing.T) {
	hasher := NewPBKDF2Hasher(10_000)
	encoded, err := hasher.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if !hasher.VerifyPassword("correct horse battery staple", encoded) {
		t.Fatal("expected password to verify")
	}
	if hasher.VerifyPassword("wrong", encoded) {
		t.Fatal("expected wrong password to fail")
	}
}

func TestPBKDF2HasherRejectsMalformedHash(t *testing.T) {
	hasher := NewPBKDF2Hasher(10_000)
	if hasher.VerifyPassword("password", "not-a-hash") {
		t.Fatal("expected malformed hash to fail")
	}
}
