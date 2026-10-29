package secrets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateIsStable(t *testing.T) { // SEC-4
	dir := t.TempDir()
	a, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, keyFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || info.Size() != keySize {
		t.Errorf("key file mode %o size %d", info.Mode().Perm(), info.Size())
	}
	b, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint() != b.Fingerprint() {
		t.Error("reloading the key changed its fingerprint")
	}
	sealed, err := a.Seal([]byte("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := b.Open(sealed); err != nil || string(plain) != "hunter2" {
		t.Errorf("Open = %q, %v", plain, err)
	}
}

func TestSealFormat(t *testing.T) {
	box, err := New(bytes.Repeat([]byte{7}, keySize))
	if err != nil {
		t.Fatal(err)
	}
	s1, _ := box.Seal([]byte("v"))
	s2, _ := box.Seal([]byte("v"))
	if s1[0] != version || len(s1) != headerSize+1+16 {
		t.Errorf("sealed = %x", s1)
	}
	if bytes.Equal(s1, s2) {
		t.Error("two seals of the same value must differ (random nonce)")
	}
}

func TestOpenRejectsTamperingAndOtherKeys(t *testing.T) {
	a, _ := New(bytes.Repeat([]byte{1}, keySize))
	b, _ := New(bytes.Repeat([]byte{2}, keySize))
	sealed, _ := a.Seal([]byte("secret"))
	if _, err := b.Open(sealed); err == nil {
		t.Error("opened with another key")
	}
	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	if _, err := a.Open(tampered); err == nil {
		t.Error("opened a tampered value")
	}
	wrongVersion := bytes.Clone(sealed)
	wrongVersion[0] = 9
	if _, err := a.Open(wrongVersion); err == nil {
		t.Error("opened an unknown version")
	}
	if _, err := a.Open([]byte{1, 2}); err == nil {
		t.Error("opened a short value")
	}
}
