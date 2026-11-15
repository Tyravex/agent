package protocol

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

type EnvironmentBinding struct {
	NodeID    string `json:"nodeId"`
	ServiceID string `json:"serviceId"`
	JobID     string `json:"jobId"`
	CommandID string `json:"commandId"`
	SessionID string `json:"sessionId"`
}
type serviceEnvironmentEnvelope struct {
	FormatVersion int `json:"formatVersion"`
	EnvironmentBinding
	ExpiresAt  string `json:"expiresAt"`
	IV         string `json:"iv"`
	Tag        string `json:"tag"`
	Ciphertext string `json:"ciphertext"`
}

// OpenServiceEnvironment never includes token, ciphertext, or plaintext in errors.
func OpenServiceEnvironment(data []byte, token string, expected EnvironmentBinding, allowed map[string]bool, now time.Time) (map[string]string, error) {
	fail := func() (map[string]string, error) { return nil, errors.New("service environment authentication failed") }
	if len(data) > 131072 || len(token) != 64 {
		return fail()
	}
	if err := RejectDuplicateJSONKeys(data); err != nil {
		return fail()
	}
	var envelope serviceEnvironmentEnvelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return fail()
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fail()
	}
	if envelope.FormatVersion != 1 || envelope.EnvironmentBinding != expected {
		return fail()
	}
	expires, err := time.Parse(time.RFC3339Nano, envelope.ExpiresAt)
	if err != nil || !expires.After(now) || expires.After(now.Add(35*time.Second)) {
		return fail()
	}
	iv, err := base64.StdEncoding.DecodeString(envelope.IV)
	if err != nil || len(iv) != 12 {
		return fail()
	}
	tag, err := base64.StdEncoding.DecodeString(envelope.Tag)
	if err != nil || len(tag) != 16 {
		return fail()
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) > 65536 {
		return fail()
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte("tyravex:agent-environment:v1:" + expected.SessionID))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return fail()
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fail()
	}
	aad := strings.Join([]string{expected.NodeID, expected.ServiceID, expected.JobID, expected.CommandID, expected.SessionID, envelope.ExpiresAt}, "\n")
	plain, err := aead.Open(nil, iv, append(ciphertext, tag...), []byte(aad))
	if err != nil {
		return fail()
	}
	defer func() {
		for index := range plain {
			plain[index] = 0
		}
	}()
	if err := RejectDuplicateJSONKeys(plain); err != nil {
		return fail()
	}
	var values map[string]string
	if err := json.Unmarshal(plain, &values); err != nil || values == nil || len(values) > 64 {
		return fail()
	}
	for key, value := range values {
		if _, exists := allowed[key]; !exists || len(value) > 8192 || strings.ContainsRune(value, '\x00') {
			return fail()
		}
	}
	for key, required := range allowed {
		if required && values[key] == "" {
			return fail()
		}
	}
	return values, nil
}
