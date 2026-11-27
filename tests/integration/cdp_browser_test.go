//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// cdpBrowser is a headless Chrome driven over the Chrome DevTools Protocol, for tests that
// must run a page's own JavaScript and click it as a person does. A click goes through
// Input.dispatchMouseEvent, so the page receives pointerdown, mousedown and click as from
// a mouse; element.click() would send the click alone.
type cdpBrowser struct {
	cmd     *exec.Cmd
	profile string
	conn    *websocket.Conn

	nextID  atomic.Int64
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[int64]chan cdpMessage
	thrown  []string
	closed  chan struct{}
}

type cdpMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// launchCDPBrowser starts headless Chrome on a blank page and connects to it. Close it
// before the server it talks to stops. Its methods take the *testing.T of the test or
// subtest calling them, so a failure stops that one.
func launchCDPBrowser(t *testing.T) *cdpBrowser {
	t.Helper()
	chrome := findChrome()
	require.NotEmpty(t, chrome, "BROWSER_E2E=true needs Chrome or Chromium installed")

	// Not t.TempDir(): its cleanup fails the test when Chrome is still flushing its
	// profile as the test ends
	profile, err := os.MkdirTemp("", "nc-chrome-profile-")
	require.NoError(t, err)
	b := &cdpBrowser{
		profile: profile,
		pending: map[int64]chan cdpMessage{},
		closed:  make(chan struct{}),
	}
	b.cmd = exec.Command(chrome,
		"--headless=new", "--disable-gpu", "--no-sandbox",
		"--no-first-run", "--no-default-browser-check",
		// Port 0: Chrome picks a free port and writes it to DevToolsActivePort in its
		// profile, so this test never attaches to another process's browser
		"--remote-debugging-port=0",
		"--user-data-dir="+profile,
		"--lang=en-US",
		"--user-agent=Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"about:blank",
	)
	require.NoError(t, b.cmd.Start())
	// A failure before the caller holds the browser must not leave Chrome running
	started := false
	defer func() {
		if !started {
			b.Close()
		}
	}()

	port := b.devToolsPort(t)
	b.conn = b.dialPage(t, port)
	go b.readLoop()
	b.call(t, "Runtime.enable", nil)
	b.call(t, "Page.enable", nil)
	started = true
	return b
}

func (b *cdpBrowser) devToolsPort(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		data, err := os.ReadFile(filepath.Join(b.profile, "DevToolsActivePort"))
		// The port is complete once its line is: Chrome may still be writing the file
		if port, _, complete := strings.Cut(string(data), "\n"); err == nil && complete && strings.TrimSpace(port) != "" {
			return strings.TrimSpace(port)
		}
		if time.Now().After(deadline) {
			t.Fatalf("Chrome did not open its debugging port")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// dialPage connects to the page Chrome opened on about:blank.
func (b *cdpBrowser) dialPage(t *testing.T, port string) *websocket.Conn {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for {
		var targets []struct {
			Type                 string `json:"type"`
			WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
		}
		resp, err := http.Get("http://127.0.0.1:" + port + "/json/list")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			_ = json.Unmarshal(body, &targets)
		}
		for _, target := range targets {
			if target.Type == "page" && target.WebSocketDebuggerURL != "" {
				// The page can be listed before it accepts a connection: try again
				conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
				if err == nil {
					return conn
				}
				lastErr = err
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("Chrome opened no page to drive (last error: %v)", lastErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (b *cdpBrowser) readLoop() {
	defer close(b.closed)
	for {
		var msg cdpMessage
		if err := b.conn.ReadJSON(&msg); err != nil {
			return
		}
		if msg.ID != 0 {
			b.mu.Lock()
			ch := b.pending[msg.ID]
			delete(b.pending, msg.ID)
			b.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
			continue
		}
		if msg.Method == "Runtime.exceptionThrown" {
			var params struct {
				ExceptionDetails struct {
					Text      string `json:"text"`
					Exception *struct {
						Description string `json:"description"`
					} `json:"exception"`
				} `json:"exceptionDetails"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			text := params.ExceptionDetails.Text
			if params.ExceptionDetails.Exception != nil {
				text += " " + params.ExceptionDetails.Exception.Description
			}
			b.mu.Lock()
			b.thrown = append(b.thrown, text)
			b.mu.Unlock()
		}
	}
}

// send runs one CDP method and returns its result, or the error Chrome answered.
func (b *cdpBrowser) send(method string, params interface{}) (json.RawMessage, error) {
	id := b.nextID.Add(1)
	ch := make(chan cdpMessage, 1)
	b.mu.Lock()
	b.pending[id] = ch
	b.mu.Unlock()

	if params == nil {
		params = map[string]interface{}{}
	}
	b.writeMu.Lock()
	err := b.conn.WriteJSON(map[string]interface{}{"id": id, "method": method, "params": params})
	b.writeMu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case msg := <-ch:
		if msg.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, msg.Error.Message)
		}
		return msg.Result, nil
	case <-b.closed:
		return nil, fmt.Errorf("%s: Chrome closed the connection", method)
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("%s: no answer from Chrome", method)
	}
}

func (b *cdpBrowser) call(t *testing.T, method string, params interface{}) json.RawMessage {
	t.Helper()
	result, err := b.send(method, params)
	require.NoError(t, err)
	return result
}

// evaluate runs expression in the page and returns its value. A page that is being
// replaced, during a navigation, answers an error.
func (b *cdpBrowser) evaluate(expression string) (json.RawMessage, error) {
	result, err := b.send("Runtime.evaluate", map[string]interface{}{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		return nil, err
	}
	if out.ExceptionDetails != nil {
		return nil, fmt.Errorf("evaluate %q: %s", expression, out.ExceptionDetails.Text)
	}
	return out.Result.Value, nil
}

// waitFor polls expression until it is true.
func (b *cdpBrowser) waitFor(t *testing.T, expression, what string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if value, err := b.evaluate(expression); err == nil && string(value) == "true" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s; page text: %q", what, b.text())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// navigate opens url, following any redirect, and waits until the new page has loaded.
func (b *cdpBrowser) navigate(t *testing.T, url string) {
	t.Helper()
	// Marks the current page, so the wait below cannot be satisfied by it
	_, err := b.evaluate(`window.__cdpPreviousPage = true`)
	require.NoError(t, err)
	b.call(t, "Page.navigate", map[string]interface{}{"url": url})
	b.waitFor(t, `!window.__cdpPreviousPage && document.readyState === "complete"`, "the page at "+url)
}

// click presses and releases the left mouse button on the middle of the element that
// elementExpression returns.
func (b *cdpBrowser) click(t *testing.T, elementExpression string) {
	t.Helper()
	value, err := b.evaluate(`(() => {
		const el = (` + elementExpression + `);
		if (!el) return null;
		el.scrollIntoView({ block: "center" });
		const r = el.getBoundingClientRect();
		return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
	})()`)
	require.NoError(t, err)
	var point *struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	require.NoError(t, json.Unmarshal(value, &point))
	require.NotNil(t, point, "nothing to click: %s; page text: %q", elementExpression, b.text())
	for _, kind := range []string{"mouseMoved", "mousePressed", "mouseReleased"} {
		params := map[string]interface{}{"type": kind, "x": point.X, "y": point.Y}
		if kind != "mouseMoved" {
			params["button"] = "left"
			params["clickCount"] = 1
		}
		b.call(t, "Input.dispatchMouseEvent", params)
	}
}

// pressShift presses and releases the Shift key, an interaction that clicks nothing on
// the page.
func (b *cdpBrowser) pressShift(t *testing.T) {
	t.Helper()
	for _, kind := range []string{"rawKeyDown", "keyUp"} {
		b.call(t, "Input.dispatchKeyEvent", map[string]interface{}{
			"type": kind, "key": "Shift", "code": "ShiftLeft", "windowsVirtualKeyCode": 16,
		})
	}
}

// text returns the page's visible text, for failure messages.
func (b *cdpBrowser) text() string {
	value, err := b.evaluate(`document.body ? document.body.innerText : ""`)
	if err != nil {
		return "(unreadable: " + err.Error() + ")"
	}
	var s string
	_ = json.Unmarshal(value, &s)
	return s
}

// thrownErrors returns the uncaught exceptions the page raised so far.
func (b *cdpBrowser) thrownErrors() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.thrown...)
}

func (b *cdpBrowser) Close() {
	if b.conn != nil {
		_ = b.conn.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
		_ = b.cmd.Wait() // reap before the profile directory is removed
	}
	_ = os.RemoveAll(b.profile)
}
