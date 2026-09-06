package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestCanonicalJSONSortsKeys(t *testing.T) {
	got, err := CanonicalJSON(map[string]any{"z": 2, "a": map[string]any{"b": true, "a": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":{"a":"x","b":true},"z":2}` {
		t.Fatalf("got %s", got)
	}
}
func TestSignVerifies(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Unix(100, 0).UTC()
	payload := map[string]any{"answer": 42}
	env, err := Sign("device", 1, "event", payload, private, now)
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := CanonicalJSON(payload)
	digest := sha256.Sum256(canonical)
	message := []byte(fmt.Sprintf("%s\n%d\n%s\n%s\n%s", env.DeviceID, env.Sequence, env.Timestamp, env.EventID, hex.EncodeToString(digest[:])))
	sig, _ := base64.StdEncoding.DecodeString(env.Signature)
	if !ed25519.Verify(public, message, sig) {
		t.Fatal("signature invalid")
	}
}
