package auth

import "testing"

func TestHashPasswordAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("123456")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	if hash == "123456" {
		t.Fatal("hash must not equal plain password")
	}
	if !CheckPassword("123456", hash) {
		t.Fatal("expected correct password to match")
	}
	if CheckPassword("wrong", hash) {
		t.Fatal("expected wrong password to be rejected")
	}
}
