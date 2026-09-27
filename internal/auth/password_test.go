package auth

import (
	"strings"
	"testing"
)

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash = %q", hash)
	}

	if ok, err := VerifyPassword("correct horse", hash); !ok || err != nil {
		t.Errorf("right password: %v %v", ok, err)
	}
	if ok, err := VerifyPassword("wrong", hash); ok || err != nil {
		t.Errorf("wrong password: %v %v", ok, err)
	}
	if again, _ := HashPassword("correct horse"); again == hash {
		t.Error("salt not random")
	}
	if _, err := VerifyPassword("x", "$argon2id$garbage"); err == nil {
		t.Error("malformed hash accepted")
	}
}
