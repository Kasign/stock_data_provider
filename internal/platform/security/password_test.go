package security

import (
	"strings"
	"testing"
)

func testPasswordHasher(t *testing.T, memory uint32) *Argon2idHasher {
	t.Helper()
	hasher, err := NewArgon2idHasher(Argon2idParams{
		Memory: memory, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	return hasher
}

func TestArgon2idHashVerifyAndSaltUniqueness(t *testing.T) {
	hasher := testPasswordHasher(t, 8*1024)
	password := []byte("a long test password")
	first, err := hasher.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := hasher.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("independent hashes must use independent salts")
	}
	if ok, err := hasher.Verify(first, password); err != nil || !ok {
		t.Fatalf("correct password rejected: ok=%v err=%v", ok, err)
	}
	if ok, err := hasher.Verify(first, []byte("incorrect password")); err != nil || ok {
		t.Fatalf("incorrect password accepted: ok=%v err=%v", ok, err)
	}
}

func TestArgon2idNeedsRehash(t *testing.T) {
	oldHasher := testPasswordHasher(t, 8*1024)
	newHasher := testPasswordHasher(t, 16*1024)
	encoded, err := oldHasher.Hash([]byte("test password"))
	if err != nil {
		t.Fatal(err)
	}
	needs, err := newHasher.NeedsRehash(encoded)
	if err != nil || !needs {
		t.Fatalf("needs rehash=%v err=%v", needs, err)
	}
	needs, err = oldHasher.NeedsRehash(encoded)
	if err != nil || needs {
		t.Fatalf("same parameters should not need rehash: %v %v", needs, err)
	}
}

func TestArgon2idRejectsInvalidInputsAndParameters(t *testing.T) {
	invalid := []Argon2idParams{
		{Memory: 8 * 1024, Iterations: 1, Parallelism: 0, SaltLength: 16, KeyLength: 32},
		{Memory: 8 * 1024, Iterations: 0, Parallelism: 1, SaltLength: 16, KeyLength: 32},
		{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 8, KeyLength: 32},
		{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 8},
		{Memory: maxArgonMemoryKiB + 1, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32},
	}
	for _, params := range invalid {
		if _, err := NewArgon2idHasher(params); err == nil {
			t.Errorf("accepted invalid parameters: %+v", params)
		}
	}
	hasher := testPasswordHasher(t, 8*1024)
	for _, password := range [][]byte{nil, {}, []byte(strings.Repeat("x", maxPasswordBytes+1))} {
		if _, err := hasher.Hash(password); err == nil {
			t.Error("accepted invalid password length")
		}
	}
}

func TestArgon2idRejectsMalformedAndExpensiveEncodedHashes(t *testing.T) {
	hasher := testPasswordHasher(t, 8*1024)
	for _, encoded := range []string{
		"", "$argon2i$v=19$m=8192,t=1,p=1$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU2",
		"$argon2id$v=18$m=8192,t=1,p=1$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU2",
		"$argon2id$19$m=8192,t=1,p=1$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU2",
		"$argon2id$v=19$m=8192,m=8192,t=1,p=1$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU2",
		"$argon2id$v=19$m=4294967295,t=1,p=1$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU2",
		strings.Repeat("x", maxEncodedHashBytes+1),
	} {
		if _, err := hasher.Verify(encoded, []byte("password")); err == nil {
			t.Errorf("accepted malformed hash %q", encoded)
		}
	}
}
