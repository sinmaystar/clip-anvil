package auth

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSignAndVerifyTokenReturnsAccountID(t *testing.T) {
	accountID := pgtype.UUID{
		Bytes: [16]byte{0x4a, 0x7b, 0x3c, 0x88, 0x90, 0x1d, 0x4c, 0xe7, 0xa1, 0x22, 0x5d, 0x10, 0x64, 0xee, 0xaa, 0x91},
		Valid: true,
	}

	token, err := SignToken(accountID, "test-secret", 1)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	got, err := VerifyToken(token, "test-secret")
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}

	if got != accountID {
		t.Fatalf("account id = %v, want %v", got, accountID)
	}
}

func TestVerifyTokenRejectsWrongSecret(t *testing.T) {
	accountID := pgtype.UUID{
		Bytes: [16]byte{0x4a, 0x7b, 0x3c, 0x88, 0x90, 0x1d, 0x4c, 0xe7, 0xa1, 0x22, 0x5d, 0x10, 0x64, 0xee, 0xaa, 0x91},
		Valid: true,
	}

	token, err := SignToken(accountID, "test-secret", 1)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if _, err := VerifyToken(token, "other-secret"); err == nil {
		t.Fatal("expected wrong secret to be rejected")
	}
}
