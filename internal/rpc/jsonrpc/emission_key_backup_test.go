// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"strings"
	"testing"

	"github.com/monetarium/monetarium-node/dcrec/secp256k1"
)

// TestEmissionKeyBackupSaltNonDeterminism exercises the CRITICAL code-review
// fix: encrypting the same private key under the same passphrase twice must
// produce two different ciphertexts (because the salt and nonce are fresh
// random per call). If the old sha256(passphrase) KDF is reintroduced this
// test fails immediately.
func TestEmissionKeyBackupSaltNonDeterminism(t *testing.T) {
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	const passphrase = "correct horse battery staple"

	blob1, err := encryptPrivateKeyWithPassphrase(priv, passphrase)
	if err != nil {
		t.Fatalf("encrypt #1: %v", err)
	}
	blob2, err := encryptPrivateKeyWithPassphrase(priv, passphrase)
	if err != nil {
		t.Fatalf("encrypt #2: %v", err)
	}
	if blob1 == blob2 {
		t.Fatalf("two encryptions of the same key+passphrase produced identical ciphertext %q — missing salt/nonce", blob1)
	}
	if !strings.HasPrefix(blob1, "aes256gcm:v2:") || !strings.HasPrefix(blob2, "aes256gcm:v2:") {
		t.Fatalf("new blobs must carry the v2 prefix; got %q / %q", blob1, blob2)
	}
}

// TestEmissionKeyBackupRoundTrip covers the round trip for v2 blobs across a
// variety of passphrase shapes including long unicode. Decrypting must yield
// the same private key bytes the caller encrypted.
func TestEmissionKeyBackupRoundTrip(t *testing.T) {
	passphrases := []string{
		"s",
		"short",
		"a somewhat longer passphrase with spaces and 7 digits 1234567",
		"パスワードつきUnicodeテスト — correct horse battery staple 🙂",
	}
	for _, pass := range passphrases {
		t.Run(pass, func(t *testing.T) {
			priv, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Fatalf("GeneratePrivateKey: %v", err)
			}
			blob, err := encryptPrivateKeyWithPassphrase(priv, pass)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			roundTripped, err := decryptPrivateKeyWithPassphrase(blob, pass)
			if err != nil {
				t.Fatalf("decrypt with correct passphrase: %v", err)
			}
			if got, want := roundTripped.Serialize(), priv.Serialize(); !equalBytes(got, want) {
				t.Fatalf("round-tripped key differs from original")
			}
		})
	}
}

// TestEmissionKeyBackupWrongPassphraseFails confirms GCM authentication trips
// when a caller provides the wrong passphrase. It is NOT a timing-attack
// check, just a functional guard.
func TestEmissionKeyBackupWrongPassphraseFails(t *testing.T) {
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	blob, err := encryptPrivateKeyWithPassphrase(priv, "right")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := decryptPrivateKeyWithPassphrase(blob, "wrong"); err == nil {
		t.Fatal("decrypt with wrong passphrase must fail")
	}
}

// TestEmissionKeyBackupV1Rejected locks in the chosen migration policy: old
// sha256(passphrase) v1 blobs are refused with an actionable error, not
// silently decrypted. A user with a v1 backup is forced to re-export from the
// canonical wallet DB under the new KDF.
func TestEmissionKeyBackupV1Rejected(t *testing.T) {
	// Shape of a v1 blob: "aes256gcm:<hex-iv>:<hex-ct>" — 3 parts.
	const v1Blob = "aes256gcm:0123456789abcdef01234567:deadbeefcafebabe"
	_, err := decryptPrivateKeyWithPassphrase(v1Blob, "whatever")
	if err == nil {
		t.Fatal("v1 blob must be rejected")
	}
	if !strings.Contains(err.Error(), "v1") || !strings.Contains(err.Error(), "insecure") {
		t.Fatalf("rejection error must mention v1 insecurity; got %v", err)
	}
}

// TestEmissionKeyBackupMalformedRejected guards the parser against a handful
// of obviously invalid shapes. We do not enumerate every failure mode — the
// goal is a smoke test that malformed input never reaches the cipher.
func TestEmissionKeyBackupMalformedRejected(t *testing.T) {
	cases := []string{
		"",                                                         // empty
		"aes128gcm:v2:00:32768:8:1:000000000000000000000000:aa",    // wrong alg prefix
		"aes256gcm:v2:zz:32768:8:1:000000000000000000000000:aa",    // non-hex salt
		"aes256gcm:v2:00:abc:8:1:000000000000000000000000:aa",      // non-integer N
		"aes256gcm:v2:00:32768:8:1:00:aa",                          // nonce wrong length
		"aes256gcm:v2:00:32768:8:1:000000000000000000000000",       // truncated (7 parts, not 8)
	}
	for _, blob := range cases {
		t.Run(blob, func(t *testing.T) {
			if _, err := decryptPrivateKeyWithPassphrase(blob, "x"); err == nil {
				t.Fatalf("malformed blob %q must be rejected", blob)
			}
		})
	}
}

// TestEmissionKeyBackupScryptNUpperBound regression test for the DoS vector
// where a malicious blob carrying N = 1<<25 (a power of two, but absurdly
// large) would force a multi-GiB scrypt allocation. The decryptor must reject
// it before invoking scrypt.
func TestEmissionKeyBackupScryptNUpperBound(t *testing.T) {
	// 1<<25 = 33_554_432; well above the 1<<20 hard cap and far above the
	// 1<<15 used by encryption. Other fields are syntactically valid; only
	// the N parameter is hostile.
	const blob = "aes256gcm:v2:00:33554432:8:1:000000000000000000000000:aa"
	_, err := decryptPrivateKeyWithPassphrase(blob, "anything")
	if err == nil {
		t.Fatal("scrypt N above the hard cap must be rejected before scrypt is called")
	}
	if !strings.Contains(err.Error(), "scrypt N") {
		t.Fatalf("rejection should mention scrypt N; got %v", err)
	}
}

// TestEmissionKeyBackupScryptRPUpperBound parallels the N-cap test for the r
// and p parameters: scrypt's contract honours arbitrarily large r and p, so a
// malicious blob with (e.g.) p = 1<<10 would burn CPU on every decrypt
// attempt.  The decryptor must reject out-of-range values before invoking
// scrypt.
func TestEmissionKeyBackupScryptRPUpperBound(t *testing.T) {
	cases := []struct {
		name     string
		blob     string
		mustSay  string
	}{
		{
			name:    "r over 16 rejected",
			blob:    "aes256gcm:v2:00:32768:17:1:000000000000000000000000:aa",
			mustSay: "scrypt r",
		},
		{
			name:    "p over 16 rejected",
			blob:    "aes256gcm:v2:00:32768:8:17:000000000000000000000000:aa",
			mustSay: "scrypt p",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decryptPrivateKeyWithPassphrase(tc.blob, "anything")
			if err == nil {
				t.Fatal("out-of-range scrypt parameter must be rejected before scrypt is called")
			}
			if !strings.Contains(err.Error(), tc.mustSay) {
				t.Fatalf("rejection should mention %q; got %v", tc.mustSay, err)
			}
		})
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
