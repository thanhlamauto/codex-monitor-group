package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"
)

type Envelope struct {
	DeviceID  string `json:"device_id"`
	Sequence  uint64 `json:"sequence"`
	Timestamp string `json:"timestamp"`
	EventID   string `json:"event_id"`
	Payload   any    `json:"payload"`
	Signature string `json:"signature"`
}

func CanonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := writeCanonical(&out, normalized); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeCanonical(out *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if v {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case string:
		b, _ := json.Marshal(v)
		out.Write(b)
	case float64:
		out.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
	case []any:
		out.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeCanonical(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			kb, _ := json.Marshal(key)
			out.Write(kb)
			out.WriteByte(':')
			if err := writeCanonical(out, v[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return fmt.Errorf("unsupported canonical JSON type %T", value)
	}
	return nil
}

func Sign(deviceID string, sequence uint64, eventID string, payload any, privateKey ed25519.PrivateKey, now time.Time) (Envelope, error) {
	timestamp := now.UTC().Format(time.RFC3339Nano)
	canonical, err := CanonicalJSON(payload)
	if err != nil {
		return Envelope{}, err
	}
	digest := sha256.Sum256(canonical)
	message := []byte(fmt.Sprintf("%s\n%d\n%s\n%s\n%s", deviceID, sequence, timestamp, eventID, hex.EncodeToString(digest[:])))
	signature := ed25519.Sign(privateKey, message)
	return Envelope{DeviceID: deviceID, Sequence: sequence, Timestamp: timestamp, EventID: eventID, Payload: payload, Signature: base64.StdEncoding.EncodeToString(signature)}, nil
}
