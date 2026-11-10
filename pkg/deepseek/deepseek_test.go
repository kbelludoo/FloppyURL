package deepseek

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func TestPowSolverLoadsAndSolves(t *testing.T) {
	ctx := context.Background()
	p, err := NewPow(ctx)
	if err != nil {
		t.Fatalf("NewPow: %v", err)
	}
	defer p.Close(ctx)

	// Real test vector: challenge = SHA3-256("testsalt_1700000000_42"),
	// solvable with answer 42 within difficulty 1000.
	ch := Challenge{
		Algorithm:  "DeepSeekHashV1",
		Challenge:  "d4a2ea58c89e40887c933484868380c6f803eaa8dc53a3b9df8e431b921a4f09",
		Salt:       "testsalt",
		Difficulty: 1000,
		ExpireAt:   1700000000,
	}
	prefix := "testsalt_1700000000_"
	answer, ok, err := p.Solve(ctx, ch.Challenge, prefix, ch.Difficulty)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if !ok {
		t.Fatalf("expected a PoW answer, got none")
	}
	if answer != 42 {
		t.Fatalf("answer = %d, want 42", answer)
	}
}

func TestMakeHeaderRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, err := NewPow(ctx)
	if err != nil {
		t.Fatalf("NewPow: %v", err)
	}
	defer p.Close(ctx)

	ch := Challenge{
		Algorithm:  "DeepSeekHashV1",
		Challenge:  "74b3b7452745b70e85eb32ee7f0a9ec0381d42dd5137b695da915e104fc390e1",
		Salt:       "abc123salt",
		Difficulty: 20000,
		ExpireAt:   1700000000,
		Signature:  "sig",
		TargetPath: completionPath,
	}
	hdr, err := p.MakeHeader(ctx, ch)
	if err != nil {
		t.Fatalf("MakeHeader: %v", err)
	}
	if hdr == "" {
		t.Fatalf("empty header")
	}
	// The header must be base64 of JSON containing the exact fields.
	raw, err := base64.StdEncoding.DecodeString(hdr)
	if err != nil {
		t.Fatalf("header is not valid base64: %v", err)
	}
	obj, err := decodeMap(raw)
	if err != nil {
		t.Fatalf("header payload is not JSON: %v", err)
	}
	if obj["algorithm"] != "DeepSeekHashV1" || obj["salt"] != "abc123salt" {
		t.Fatalf("unexpected payload: %v", obj)
	}
	if ans, _ := asInt(obj["answer"]); ans != 12345 {
		t.Fatalf("answer = %v, want 12345", obj["answer"])
	}
}

func TestEncodeDecodeCID(t *testing.T) {
	if got := encodeCID("abc", nil); got != "abc" {
		t.Fatalf("encodeCID(nil) = %q", got)
	}
	mid := int64(42)
	if got := encodeCID("abc", &mid); got != "abc:42" {
		t.Fatalf("encodeCID = %q", got)
	}
	sid, parent, err := decodeCID("abc:42")
	if err != nil || sid != "abc" || parent == nil || *parent != 42 {
		t.Fatalf("decodeCID = %q %v %v", sid, parent, err)
	}
	sid, parent, _ = decodeCID("abc")
	if sid != "abc" || parent != nil {
		t.Fatalf("decodeCID bare = %q %v", sid, parent)
	}
}

func TestParseSSE(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"v":{"response":{"message_id":42,"fragments":[{"type":"RESPONSE","content":"Hello"}]}}}`,
		`data: {"p":"response/fragments/-1/content","o":"APPEND","v":" world"}`,
		`data: {"v":"!"}`,
		`data: [DONE]`,
	}, "\n")

	var sb strings.Builder
	var messageID int64
	var hasID bool
	if err := parseSSE(strings.NewReader(stream), func(s string) error {
		sb.WriteString(s)
		return nil
	}, &messageID, &hasID); err != nil {
		t.Fatalf("parseSSE: %v", err)
	}
	if sb.String() != "Hello world!" {
		t.Fatalf("text = %q, want %q", sb.String(), "Hello world!")
	}
	if !hasID || messageID != 42 {
		t.Fatalf("messageID = %d has=%v", messageID, hasID)
	}
}

func TestParseSSEErrorFrame(t *testing.T) {
	stream := `data: {"type":"error","content":"backend overloaded","finish_reason":"error"}`
	var id int64
	var has bool
	err := parseSSE(strings.NewReader(stream), func(string) error { return nil }, &id, &has)
	if err == nil {
		t.Fatalf("expected an error")
	}
	if _, ok := err.(*BackendError); !ok {
		t.Fatalf("expected *BackendError, got %T: %v", err, err)
	}
}

func TestIsAccountProtectionResponse(t *testing.T) {
	if !IsAccountProtectionResponse(429, "") {
		t.Fatal("429 should trip protection")
	}
	if !IsAccountProtectionResponse(200, "please complete the CAPTCHA") {
		t.Fatal("captcha text should trip protection")
	}
	if IsAccountProtectionResponse(200, "all good") {
		t.Fatal("normal response should not trip")
	}
}
