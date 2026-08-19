package identity

import (
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	encoded, err := hashPassword("a-long-and-correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encoded, "a-long-and-correct-password") {
		t.Fatal("password leaked into encoded hash")
	}
	valid, err := verifyPassword("a-long-and-correct-password", encoded)
	if err != nil || !valid {
		t.Fatalf("expected password to verify, valid=%v err=%v", valid, err)
	}
	valid, err = verifyPassword("definitely-not-the-password", encoded)
	if err != nil || valid {
		t.Fatalf("expected wrong password to fail, valid=%v err=%v", valid, err)
	}
}

func TestVerifyPasswordRejectsHostileParameters(t *testing.T) {
	encoded := "$argon2id$v=19$m=4294967295,t=3,p=4$c2FsdHNhbHRzYWx0c2FsdA$AAAAAAAAAAAAAAAAAAAAAA"
	if _, err := verifyPassword("password", encoded); err == nil {
		t.Fatal("expected oversized memory cost to be rejected")
	}
}
