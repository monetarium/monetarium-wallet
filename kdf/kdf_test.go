// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package kdf

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"runtime"
	"testing"
)

// TestNewArgon2idParamsPinsOWASPDefaults locks in the parameter choices that
// back wallet passphrase derivation. A regression that drops Time below 3
// (OWASP argon2id minimum) or shrinks Memory below 256 MiB silently weakens
// every encrypted wallet without surfacing in higher-level tests.
//
// Pin from the 2026-05-04 review: Time=3, Memory=256*1024 KiB (256 MiB),
// Threads = min(NumCPU, 256). Salt must be filled from the supplied reader.
func TestNewArgon2idParamsPinsOWASPDefaults(t *testing.T) {
	p, err := NewArgon2idParams(rand.Reader)
	if err != nil {
		t.Fatalf("NewArgon2idParams: %v", err)
	}
	if p.Time != 3 {
		t.Errorf("Time = %d, want 3 (OWASP argon2id minimum)", p.Time)
	}
	if p.Memory != 256*1024 {
		t.Errorf("Memory = %d KiB, want %d KiB (256 MiB)", p.Memory, 256*1024)
	}
	wantThreads := runtime.NumCPU()
	if wantThreads > 256 {
		wantThreads = 256
	}
	if int(p.Threads) != wantThreads {
		t.Errorf("Threads = %d, want min(NumCPU, 256) = %d", p.Threads, wantThreads)
	}
	if p.Salt == ([16]byte{}) {
		t.Errorf("Salt was not populated from rand reader")
	}
}

// TestNewArgon2idParamsSaltSourceIsRandReader confirms the salt is read from
// the supplied reader rather than a hardcoded source. A regression that
// shadowed the reader argument would silently reuse the same salt across
// wallets and turn argon2id into a trivial offline rainbow target.
func TestNewArgon2idParamsSaltSourceIsRandReader(t *testing.T) {
	src := bytes.Repeat([]byte{0xAB}, 16)
	r := bytes.NewReader(src)
	p, err := NewArgon2idParams(r)
	if err != nil {
		t.Fatalf("NewArgon2idParams: %v", err)
	}
	if !bytes.Equal(p.Salt[:], src) {
		t.Errorf("Salt = %x, want %x (must be read from rand argument)",
			p.Salt[:], src)
	}
}

// TestNewArgon2idParamsPropagatesReaderError surfaces a Read failure rather
// than silently returning a zero-salt params struct. Pin: a salt-randomness
// error must reach the caller so the wallet open path can refuse rather than
// proceed with a deterministic salt.
func TestNewArgon2idParamsPropagatesReaderError(t *testing.T) {
	want := errors.New("synthetic")
	_, err := NewArgon2idParams(errReader{want})
	if !errors.Is(err, want) {
		t.Errorf("got %v, want %v propagated from rand reader", err, want)
	}
}

// TestArgon2idParamsRoundTrip locks the binary marshal/unmarshal contract
// against silent layout changes. The wallet stores these parameters next to
// the ciphertext, so a layout shift would render existing wallets unopenable.
func TestArgon2idParamsRoundTrip(t *testing.T) {
	p := &Argon2idParams{
		Time:    3,
		Memory:  256 * 1024,
		Threads: 8,
	}
	for i := range p.Salt {
		p.Salt[i] = byte(i)
	}
	buf, err := p.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if len(buf) != MarshaledLen {
		t.Fatalf("len(buf) = %d, want %d", len(buf), MarshaledLen)
	}
	var got Argon2idParams
	if err := got.UnmarshalBinary(buf); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	if got != *p {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, *p)
	}
}

// TestArgon2idParamsUnmarshalRejectsWrongLength locks the input-length check
// so a future caller passing a truncated/extended buffer cannot silently
// produce a partly-zeroed params struct.
func TestArgon2idParamsUnmarshalRejectsWrongLength(t *testing.T) {
	for _, n := range []int{0, MarshaledLen - 1, MarshaledLen + 1, 1024} {
		var p Argon2idParams
		err := p.UnmarshalBinary(make([]byte, n))
		if err == nil {
			t.Errorf("UnmarshalBinary(len=%d) returned nil; want error", n)
		}
	}
}

type errReader struct{ err error }

func (r errReader) Read(_ []byte) (int, error) { return 0, r.err }

// Compile-time assertion that errReader implements io.Reader.
var _ io.Reader = errReader{}
