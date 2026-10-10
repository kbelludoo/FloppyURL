package router

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sync"
	"time"
)

// ArenaClient encapsulates session management and API requests for arena.ai.
type ArenaClient struct {
	client  *http.Client
	baseURL string
	email   string
	pass    string
	user    *ArenaUser
	mu      sync.Mutex
}

type ArenaUser struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	Username       string `json:"username"`
	EmailConfirmed bool   `json:"emailConfirmed"`
}

type ArenaModel struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	PublicName   string `json:"publicName"`
	Organization string `json:"organization"`
}

var (
	defaultArenaClient *ArenaClient
	arenaOnce          sync.Once
)

// GetArenaClient returns a singleton ArenaClient initialized from environment or defaults.
func GetArenaClient() *ArenaClient {
	arenaOnce.Do(func() {
		email := os.Getenv("ARENA_EMAIL")
		if email == "" {
			email = "brunofonsecadesouza89@gmail.com"
		}
		pass := os.Getenv("ARENA_PASSWORD")
		if pass == "" {
			pass = "ECTbb,,0209"
		}
		jar, _ := cookiejar.New(nil)
		defaultArenaClient = &ArenaClient{
			client: &http.Client{
				Jar:     jar,
				Timeout: 45 * time.Second,
			},
			baseURL: "https://arena.ai",
			email:   email,
			pass:    pass,
		}
	})
	return defaultArenaClient
}

// Login authenticates with arena.ai using email and password.
func (a *ArenaClient) Login() (*ArenaUser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	payload := map[string]any{
		"email":             a.email,
		"password":          a.pass,
		"shouldLinkHistory": false,
	}
	bodyJSON, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", a.baseURL+"/nextjs-api/sign-in/email", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", a.baseURL)
	req.Header.Set("Referer", a.baseURL+"/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		Success bool       `json:"success"`
		Message string     `json:"message"`
		Error   string     `json:"error"`
		User    *ArenaUser `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}

	if resp.StatusCode != http.StatusOK || !res.Success {
		return nil, fmt.Errorf("login failed (status %d): %s", resp.StatusCode, res.Error)
	}

	a.user = res.User
	return a.user, nil
}

// GetCatalog retrieves the available model list from arena.ai.
func (a *ArenaClient) GetCatalog() ([]ArenaModel, error) {
	req, err := http.NewRequest("GET", a.baseURL+"/nextjs-api/model-catalog", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var arenas []struct {
		Arena  string       `json:"arena"`
		Models []ArenaModel `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&arenas); err != nil {
		return nil, err
	}

	for _, ar := range arenas {
		if ar.Arena == "text" || ar.Arena == "chat" {
			return ar.Models, nil
		}
	}
	if len(arenas) > 0 {
		return arenas[0].Models, nil
	}
	return nil, errors.New("no models found in catalog")
}

// NewUUIDv7 generates an RFC 9562 compliant UUIDv7.
func NewUUIDv7() string {
	var value [16]byte
	ms := uint64(time.Now().UnixMilli())
	value[0] = byte(ms >> 40)
	value[1] = byte(ms >> 32)
	value[2] = byte(ms >> 24)
	value[3] = byte(ms >> 16)
	value[4] = byte(ms >> 8)
	value[5] = byte(ms)
	_, _ = rand.Read(value[6:])
	value[6] = (value[6] & 0x0f) | 0x70
	value[8] = (value[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(value[0:4]),
		binary.BigEndian.Uint16(value[4:6]),
		binary.BigEndian.Uint16(value[6:8]),
		binary.BigEndian.Uint16(value[8:10]),
		value[10:16],
	)
}
