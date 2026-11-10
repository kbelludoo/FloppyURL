package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// DefaultCDPURL is used when DEEPSEEK_CDP_URL is not set.
const DefaultCDPURL = "http://127.0.0.1:9222"

// CDPURL returns the configured Chrome DevTools endpoint.
func CDPURL() string {
	if u := strings.TrimSpace(os.Getenv("DEEPSEEK_CDP_URL")); u != "" {
		return u
	}
	return DefaultCDPURL
}

// readTokenJS reads the bearer token the DeepSeek web app stores after login:
//
//	localStorage.userToken = {"value":"<TOKEN>","__version":"0"}
const readTokenJS = `(() => {
  try {
    const raw = window.localStorage.getItem('userToken');
    if (!raw) return null;
    const o = JSON.parse(raw);
    return (o && o.value) ? o.value : null;
  } catch (e) { return null; }
})()`

const readUAJS = `(() => { try { return navigator.userAgent; } catch (e) { return ''; } })()`

type cdpTarget struct {
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpConn struct {
	ws     *websocket.Conn
	nextID int
}

func dialCDP(ctx context.Context, base string) (*cdpConn, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Chrome DevTools at %s: %w\nStart Chrome with: chrome --remote-debugging-port=9222 --user-data-dir=%%LOCALAPPDATA%%\\deepseek-chrome https://chat.deepseek.com/", base, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var targets []cdpTarget
	if err := json.Unmarshal(body, &targets); err != nil {
		return nil, fmt.Errorf("invalid DevTools target list from %s: %w", base, err)
	}

	var chosen *cdpTarget
	for i := range targets {
		if targets[i].Type == "page" && strings.Contains(targets[i].URL, "deepseek.com") {
			chosen = &targets[i]
			break
		}
	}
	if chosen == nil {
		for i := range targets {
			if targets[i].Type == "page" && targets[i].WebSocketDebuggerURL != "" {
				chosen = &targets[i]
				break
			}
		}
	}
	if chosen == nil || chosen.WebSocketDebuggerURL == "" {
		return nil, fmt.Errorf("no debuggable page found at %s — open https://chat.deepseek.com/ in that Chrome window and sign in first", base)
	}

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	ws, _, err := dialer.DialContext(ctx, chosen.WebSocketDebuggerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot open CDP websocket: %w", err)
	}
	return &cdpConn{ws: ws}, nil
}

func (c *cdpConn) close() {
	if c != nil && c.ws != nil {
		_ = c.ws.Close()
	}
}

func (c *cdpConn) call(ctx context.Context, method string, params any, out any) error {
	c.nextID++
	id := c.nextID
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_ = c.ws.SetWriteDeadline(time.Now().Add(20 * time.Second))
	if err := c.ws.WriteMessage(websocket.TextMessage, payload); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = c.ws.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			return err
		}
		var env struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &env) != nil {
			continue
		}
		if env.ID == nil || *env.ID != id {
			continue
		}
		if env.Error != nil {
			return fmt.Errorf("cdp %s: %s", method, env.Error.Message)
		}
		if out != nil && len(env.Result) > 0 {
			return json.Unmarshal(env.Result, out)
		}
		return nil
	}
}

func (c *cdpConn) evaluate(ctx context.Context, expression string) (string, error) {
	var res struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
	}
	params := map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
	}
	if err := c.call(ctx, "Runtime.evaluate", params, &res); err != nil {
		return "", err
	}
	if len(res.Result.Value) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(res.Result.Value, &s); err != nil {
		return "", nil
	}
	return s, nil
}

func (c *cdpConn) cookies(ctx context.Context) (map[string]string, error) {
	var res struct {
		Cookies []struct {
			Name   string `json:"name"`
			Value  string `json:"value"`
			Domain string `json:"domain"`
		} `json:"cookies"`
	}
	if err := c.call(ctx, "Network.getAllCookies", map[string]any{}, &res); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(res.Cookies))
	for _, ck := range res.Cookies {
		out[ck.Name] = ck.Value
	}
	return out, nil
}

// CaptureFromCDP reads the DeepSeek token, cookies and user-agent from a
// running Chrome instance exposed over the DevTools Protocol.
func CaptureFromCDP(ctx context.Context, cdpURL string) (*Session, error) {
	if strings.TrimSpace(cdpURL) == "" {
		cdpURL = CDPURL()
	}
	conn, err := dialCDP(ctx, cdpURL)
	if err != nil {
		return nil, err
	}
	defer conn.close()

	token, err := conn.evaluate(ctx, readTokenJS)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("no userToken found in the browser — make sure you are signed in at https://chat.deepseek.com/")
	}
	ua, _ := conn.evaluate(ctx, readUAJS)
	cookies, err := conn.cookies(ctx)
	if err != nil {
		return nil, err
	}
	return &Session{
		Token:      token,
		Cookies:    cookies,
		UserAgent:  ua,
		CapturedAt: float64(time.Now().UnixNano()) / 1e9,
	}, nil
}

// GetSession returns a usable session: a fresh cached file if present,
// otherwise a capture from the CDP-connected Chrome.
//
// When allowInteractive is false and there is no cached session, it returns a
// *LoginRequired error instead.
func GetSession(ctx context.Context, sessionFile string, maxAge time.Duration, allowInteractive bool) (*Session, error) {
	if sessionFile == "" {
		sessionFile = DefaultSessionFile()
	}
	cached, _ := LoadSession(sessionFile)
	if cached != nil && cached.Token != "" && cached.Age() < maxAge {
		return cached, nil
	}

	if !allowInteractive {
		return nil, &LoginRequired{}
	}

	sess, err := CaptureFromCDP(ctx, CDPURL())
	if err != nil {
		return nil, err
	}
	if err := ProbeToken(ctx, sess); err != nil {
		return nil, err
	}
	_ = sess.Save(sessionFile)
	return sess, nil
}

// ProbeToken verifies a token against the API before saving it. A token can
// exist in localStorage while the server already considers it revoked.
func ProbeToken(ctx context.Context, sess *Session) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		BASE+"/api/v0/chat_session/create", bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	for k, v := range baseHeaders(sess) {
		req.Header.Set(k, v)
	}
	req.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Network blip: warn and let the save proceed.
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 401 || resp.StatusCode == 403 || isAuthFailure(resp.StatusCode, string(body)) {
		return fmt.Errorf("the captured token was rejected by DeepSeek (invalid/expired); do a full logout + login in the browser and retry")
	}

	// Delete the throwaway probe session.
	var data struct {
		Data struct {
			BizData struct {
				ChatSession struct {
					ID string `json:"id"`
				} `json:"chat_session"`
			} `json:"biz_data"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &data) == nil && data.Data.BizData.ChatSession.ID != "" {
		delBody, _ := json.Marshal(map[string]string{"chat_session_id": data.Data.BizData.ChatSession.ID})
		delReq, _ := http.NewRequestWithContext(ctx, http.MethodPost,
			BASE+"/api/v0/chat_session/delete", bytes.NewReader(delBody))
		if delReq != nil {
			for k, v := range baseHeaders(sess) {
				delReq.Header.Set(k, v)
			}
			delReq.Header.Set("content-type", "application/json")
			if r, err := client.Do(delReq); err == nil {
				_ = r.Body.Close()
			}
		}
	}
	return nil
}
