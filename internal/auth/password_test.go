package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "pbkdf2_sha256$") {
		t.Fatalf("hash = %q", hash)
	}
	ok, err := VerifyPassword(hash, "correct horse battery")
	if err != nil || !ok {
		t.Fatalf("VerifyPassword correct = %v, %v", ok, err)
	}
	ok, err = VerifyPassword(hash, "wrong password value")
	if err != nil || ok {
		t.Fatalf("VerifyPassword wrong = %v, %v", ok, err)
	}
}

func TestPasswordSaltIsRandom(t *testing.T) {
	a, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("相同密码应产生不同盐与哈希")
	}
}

func TestWeakPasswordRejected(t *testing.T) {
	if _, err := HashPassword("short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"plain",
		"pbkdf2_sha256$not-a-number$c2FsdA$aGFzaA",
		"pbkdf2_sha256$1000$c2FsdA$aGFzaA",
		"pbkdf2_sha256$600000$!!!$aGFzaA",
		"bcrypt$600000$c2FsdA$aGFzaA",
	}
	for _, c := range cases {
		if _, err := VerifyPassword(c, "whatever"); !errors.Is(err, ErrInvalidHashFormat) {
			t.Errorf("VerifyPassword(%q) err = %v", c, err)
		}
	}
}

func TestSessionToken(t *testing.T) {
	a, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) < 40 {
		t.Fatalf("tokens = %q, %q", a, b)
	}
	if HashToken(a) == HashToken(b) {
		t.Fatal("不同令牌不应产生相同摘要")
	}
	if len(HashToken(a)) != 64 {
		t.Fatalf("hash = %q", HashToken(a))
	}
}
