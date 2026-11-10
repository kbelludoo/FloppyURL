package deepseek

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Session is a captured signed-in DeepSeek web session. Its JSON layout is
// compatible with the reference Python implementation so the same
// session/session.json can be reused by both.
type Session struct {
	Token      string            `json:"token"`
	Cookies    map[string]string `json:"cookies"`
	UserAgent  string            `json:"user_agent"`
	CapturedAt float64           `json:"captured_at"`
}

// Age returns how long ago the session was captured.
func (s *Session) Age() time.Duration {
	return time.Duration(float64(time.Second) * (float64(time.Now().UnixNano())/1e9 - s.CapturedAt))
}

// Save writes the session to disk as JSON.
func (s *Session) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadSession reads a session file. It returns (nil, nil) when the file does
// not exist or cannot be parsed.
func LoadSession(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, nil
	}
	return &s, nil
}

// DefaultSessionFile returns the session file path, honouring
// DEEPSEEK_SESSION_FILE and defaulting to session/session.json.
func DefaultSessionFile() string {
	if p := os.Getenv("DEEPSEEK_SESSION_FILE"); p != "" {
		return p
	}
	return filepath.Join("session", "session.json")
}

// SessionMaxAge is how long a captured token is trusted before a fresh
// capture is requested.
const SessionMaxAge = 24 * time.Hour

// LoginRequired is returned when no usable session exists and interactive
// login is not allowed (e.g. while handling a server request).
type LoginRequired struct{ Message string }

func (e *LoginRequired) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "No DeepSeek session found. Log in first by running: agent-go -deepseek-login"
}

// IsLoginRequired reports whether err is (or wraps) a LoginRequired.
func IsLoginRequired(err error) bool {
	var lr *LoginRequired
	return errors.As(err, &lr)
}
