package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// BASE is the DeepSeek web origin.
const BASE = "https://chat.deepseek.com"

const completionPath = "/api/v0/chat/completion"

// Model_type wire values: "default" is Instant (fast), "expert" is the
// stronger/slower model.
const (
	ModelDefault = "default"
	ModelExpert  = "expert"
)

const cidSep = ":"

// Reply is a completed chat reply plus the id to resume the conversation.
type Reply struct {
	Text           string
	ConversationID string
}

// BackendError means DeepSeek accepted the request (HTTP 200) but its own
// generation backend failed (surfaced on the SSE stream as an error frame).
type BackendError struct {
	Detail       string
	FinishReason string
}

func (e *BackendError) Error() string {
	suffix := ""
	if e.FinishReason != "" {
		suffix = " (finish_reason=" + e.FinishReason + ")"
	}
	return "DeepSeek backend error: " + e.Detail + suffix
}

// Client speaks chat.deepseek.com's internal API using a captured session.
type Client struct {
	session     *Session
	http        *http.Client
	pow         *Pow
	guard       *RequestGuard
	sessionFile string
}

// NewClient resolves a session (cached file or CDP capture) and prepares the
// PoW wasm runtime.
func NewClient(ctx context.Context, sessionFile string, allowInteractive bool) (*Client, error) {
	sess, err := GetSession(ctx, sessionFile, SessionMaxAge, allowInteractive)
	if err != nil {
		return nil, err
	}
	return NewClientWithSession(ctx, sess, sessionFile)
}

// NewClientWithSession builds a client from an already-captured session.
func NewClientWithSession(ctx context.Context, sess *Session, sessionFile string) (*Client, error) {
	pow, err := NewPow(ctx)
	if err != nil {
		return nil, err
	}
	if sessionFile == "" {
		sessionFile = DefaultSessionFile()
	}
	return &Client{
		session:     sess,
		pow:         pow,
		guard:       NewRequestGuard(minIntervalFromEnv()),
		sessionFile: sessionFile,
		http: &http.Client{
			Timeout: 0, // streaming: no global timeout
		},
	}, nil
}

func minIntervalFromEnv() time.Duration {
	v := strings.TrimSpace(os.Getenv("DEEPSEEK_MIN_REQUEST_INTERVAL"))
	if v == "" {
		return 3 * time.Second
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return 3 * time.Second
	}
	return time.Duration(f * float64(time.Second))
}

// Close releases the wasm runtime.
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.pow == nil {
		return nil
	}
	return c.pow.Close(ctx)
}

// Session returns the underlying session.
func (c *Client) Session() *Session { return c.session }

// SetSession swaps the session used for subsequent requests.
func (c *Client) SetSession(s *Session) { c.session = s }

func baseHeaders(s *Session) map[string]string {
	return map[string]string{
		"authorization":            "Bearer " + s.Token,
		"accept":                   "*/*",
		"content-type":             "application/json",
		"user-agent":               s.UserAgent,
		"origin":                   BASE,
		"referer":                  BASE + "/",
		"x-app-version":            "2.0.0",
		"x-client-version":         "2.0.0",
		"x-client-platform":        "web",
		"x-client-locale":          "en_US",
		"x-client-bundle-id":       "com.deepseek.chat",
		"x-client-timezone-offset": "19800",
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, body []byte, extra map[string]string) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, BASE+path, rdr)
	if err != nil {
		return nil, err
	}
	for k, v := range baseHeaders(c.session) {
		req.Header.Set(k, v)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	return req, nil
}

// authFailed drops a stale cached session and returns a *LoginRequired.
func (c *Client) authFailed() error {
	if cached, _ := LoadSession(c.sessionFile); cached != nil && cached.Token == c.session.Token {
		_ = os.Remove(c.sessionFile)
	}
	return &LoginRequired{Message: "DeepSeek rejected the saved session (invalid/expired token). Log in again with: agent-go -deepseek-login"}
}

func decodeMap(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func asInt(v any) (int64, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			f, ferr := n.Float64()
			if ferr != nil {
				return 0, false
			}
			return int64(f), true
		}
		return i, true
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

func isZeroCode(v any) bool {
	i, ok := asInt(v)
	return ok && i == 0
}

// biz unwraps the data.biz_data envelope, raising on API-level errors.
func (c *Client) biz(data map[string]any) (map[string]any, error) {
	if !isZeroCode(data["code"]) {
		msg := fmt.Sprintf("%v", data["msg"])
		if msg == "<nil>" {
			msg = fmt.Sprintf("%v", data)
		}
		if isAuthFailure(0, msg) {
			return nil, c.authFailed()
		}
		return nil, fmt.Errorf("DeepSeek API error: %s", msg)
	}
	inner, _ := data["data"].(map[string]any)
	if inner == nil {
		return nil, fmt.Errorf("unexpected response shape: %v", data)
	}
	biz, _ := inner["biz_data"].(map[string]any)
	if biz == nil {
		return nil, fmt.Errorf("unexpected response shape: %v", data)
	}
	return biz, nil
}

// CreateChatSession creates a new chat thread and returns its id.
func (c *Client) CreateChatSession(ctx context.Context) (string, error) {
	if err := c.guard.Wait(); err != nil {
		return "", err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v0/chat_session/create", []byte("{}"), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if rerr := c.checkResponse(resp.StatusCode, string(body), "session creation"); rerr != nil {
		return "", rerr
	}
	data, err := decodeMap(body)
	if err != nil {
		return "", fmt.Errorf("invalid JSON from create session: %w", err)
	}
	biz, err := c.biz(data)
	if err != nil {
		return "", err
	}
	cs, _ := biz["chat_session"].(map[string]any)
	id, _ := cs["id"].(string)
	if id == "" {
		return "", fmt.Errorf("create session returned no id")
	}
	return id, nil
}

// DeleteChatSession deletes a chat thread (best effort).
func (c *Client) DeleteChatSession(ctx context.Context, id string) bool {
	if id == "" {
		return false
	}
	body, _ := json.Marshal(map[string]string{"chat_session_id": id})
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v0/chat_session/delete", body, nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode < 300
}

// checkResponse maps HTTP status/body to an auth or safety error, or nil.
func (c *Client) checkResponse(status int, body, phase string) error {
	if isAuthFailure(status, body) {
		return c.authFailed()
	}
	if IsAccountProtectionResponse(status, body) {
		reason := fmt.Sprintf("DeepSeek returned HTTP %d during %s; stopped to avoid repeated account-security requests.", status, phase)
		c.guard.Trip(reason)
		return &SafetyTrip{Reason: reason}
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("DeepSeek HTTP %d during %s: %s", status, phase, strings.TrimSpace(body))
	}
	return nil
}

func (c *Client) powHeader(ctx context.Context, targetPath string) (string, error) {
	if err := c.guard.Wait(); err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{"target_path": targetPath})
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v0/chat/create_pow_challenge", body, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if rerr := c.checkResponse(resp.StatusCode, string(raw), "PoW challenge issuance"); rerr != nil {
		return "", rerr
	}
	data, err := decodeMap(raw)
	if err != nil {
		return "", fmt.Errorf("invalid JSON from pow challenge: %w", err)
	}
	biz, err := c.biz(data)
	if err != nil {
		return "", err
	}
	chRaw, _ := json.Marshal(biz["challenge"])
	var ch Challenge
	if err := json.Unmarshal(chRaw, &ch); err != nil {
		return "", fmt.Errorf("invalid pow challenge shape: %w", err)
	}
	return c.pow.MakeHeader(ctx, ch)
}

// StreamOptions controls a single completion request.
type StreamOptions struct {
	ConversationID string
	Model          string // ModelDefault / ModelExpert; only for a new thread
	Thinking       bool
	Search         bool
}

// Stream runs a completion, invoking onDelta for each text chunk, and returns
// the full reply (with the conversation id to resume).
func (c *Client) Stream(ctx context.Context, prompt string, opts StreamOptions, onDelta func(string) error) (Reply, error) {
	sessionID, parentID, err := decodeCID(opts.ConversationID)
	if err != nil {
		return Reply{}, err
	}
	var modelType any
	if sessionID == "" {
		sessionID, err = c.CreateChatSession(ctx)
		if err != nil {
			return Reply{}, err
		}
		model := opts.Model
		if model == "" {
			model = ModelDefault
		}
		modelType = model
	} else {
		if opts.Model != "" {
			return Reply{}, fmt.Errorf("Model cannot be set together with ConversationID; a thread's model is fixed when created")
		}
		modelType = nil
	}

	body := map[string]any{
		"chat_session_id":   sessionID,
		"parent_message_id": nil,
		"prompt":            prompt,
		"ref_file_ids":      []any{},
		"thinking_enabled":  opts.Thinking,
		"search_enabled":    opts.Search,
		"action":            nil,
		"preempt":           false,
	}
	if parentID != nil {
		body["parent_message_id"] = *parentID
	}
	if modelType != nil {
		body["model_type"] = modelType
	}

	// PoW challenges are short-lived: solve right before the request.
	powHdr, err := c.powHeader(ctx, completionPath)
	if err != nil {
		return Reply{}, err
	}

	if err := c.guard.Wait(); err != nil {
		return Reply{}, err
	}
	reqBody, _ := json.Marshal(body)
	req, err := c.newRequest(ctx, http.MethodPost, completionPath, reqBody,
		map[string]string{"x-ds-pow-response": powHdr})
	if err != nil {
		return Reply{}, err
	}
	req.Header.Set("accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return Reply{}, err
	}
	defer resp.Body.Close()

	if rerr := c.checkResponse(resp.StatusCode, "", "completion"); rerr != nil {
		return Reply{}, rerr
	}

	var sb strings.Builder
	messageID := int64(-1)
	hasMessageID := false
	if err := parseSSE(resp.Body, func(delta string) error {
		sb.WriteString(delta)
		if onDelta != nil {
			return onDelta(delta)
		}
		return nil
	}, &messageID, &hasMessageID); err != nil {
		return Reply{}, err
	}

	cid := sessionID
	if hasMessageID {
		cid = encodeCID(sessionID, &messageID)
	}
	return Reply{Text: sb.String(), ConversationID: cid}, nil
}

// Chat runs a completion and returns the full reply.
func (c *Client) Chat(ctx context.Context, prompt string, opts StreamOptions) (Reply, error) {
	return c.Stream(ctx, prompt, opts, nil)
}

func parseSSE(r io.Reader, emit func(string) error, messageID *int64, hasMessageID *bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	activePath := ""
	emittedInitial := false

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(line[len("data:"):])
		if payload == "" || payload == "[DONE]" {
			continue
		}
		obj, err := decodeMap([]byte(payload))
		if err != nil {
			continue
		}

		if t, _ := obj["type"].(string); t == "error" {
			content, _ := obj["content"].(string)
			if content == "" {
				content = "the server reported an unspecified error"
			}
			finish, _ := obj["finish_reason"].(string)
			if isAuthFailure(0, content+finish) {
				return &LoginRequired{}
			}
			return &BackendError{Detail: content, FinishReason: finish}
		}

		v := obj["v"]

		// Snapshot frame: full response object.
		if vm, ok := v.(map[string]any); ok {
			if resp, ok := vm["response"].(map[string]any); ok {
				captureMessageID(vm, messageID, hasMessageID)
				if frags, ok := resp["fragments"].([]any); ok {
					for _, f := range frags {
						fm, ok := f.(map[string]any)
						if !ok {
							continue
						}
						if ft, _ := fm["type"].(string); ft == "RESPONSE" {
							if content, _ := fm["content"].(string); content != "" {
								activePath = "response/fragments/-1/content"
								if !emittedInitial {
									emittedInitial = true
									if err := emit(content); err != nil {
										return err
									}
								}
							}
						}
					}
				}
				continue
			}
		}

		// Path-setting append frame.
		if p, ok := obj["p"].(string); ok {
			activePath = p
			if strings.HasSuffix(p, "message_id") {
				if id, ok := asInt(v); ok && messageID != nil {
					*messageID = id
					if hasMessageID != nil {
						*hasMessageID = true
					}
				}
			}
			if o, _ := obj["o"].(string); o == "APPEND" {
				if s, ok := v.(string); ok && strings.HasSuffix(p, "content") {
					if err := emit(s); err != nil {
						return err
					}
				}
			}
			continue
		}

		// Bare append to the current path.
		if s, ok := v.(string); ok && strings.HasSuffix(activePath, "content") {
			if err := emit(s); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func captureMessageID(snapshot map[string]any, messageID *int64, hasMessageID *bool) {
	if messageID == nil {
		return
	}
	for _, container := range []map[string]any{asMap(snapshot["response"]), snapshot} {
		if container == nil {
			continue
		}
		if id, ok := asInt(container["message_id"]); ok {
			*messageID = id
			if hasMessageID != nil {
				*hasMessageID = true
			}
			return
		}
		if id, ok := asInt(container["id"]); ok {
			*messageID = id
			if hasMessageID != nil {
				*hasMessageID = true
			}
			return
		}
	}
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func encodeCID(sessionID string, messageID *int64) string {
	if messageID == nil {
		return sessionID
	}
	return sessionID + cidSep + strconv.FormatInt(*messageID, 10)
}

func decodeCID(conversationID string) (string, *int64, error) {
	if conversationID == "" {
		return "", nil, nil
	}
	parts := strings.SplitN(conversationID, cidSep, 2)
	sessionID := parts[0]
	var parent *int64
	if len(parts) == 2 && parts[1] != "" {
		if n, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			parent = &n
		}
	}
	if sessionID == "" {
		return "", nil, nil
	}
	return sessionID, parent, nil
}
