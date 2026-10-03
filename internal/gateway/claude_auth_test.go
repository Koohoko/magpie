package gateway

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeAuthFailureFallsBackWithoutRetryingTheLogin(t *testing.T) {
	claudeMadeFirst(t, false)
	s := New()
	t.Cleanup(s.subscription.abortAll)
	logfile := filepath.Join(t.TempDir(), "calls")
	script := `#!/bin/sh
case "$1" in auth) exit 1;; esac
creds="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/.credentials.json"
while read -r line; do
  tok=$(grep -o 'tok-[a-z]*' "$creds" | head -1)
  echo "$tok" >> '` + logfile + `'
  if [ "$tok" = "tok-a" ]; then
    echo '{"type":"result","is_error":true,"result":"Failed to authenticate: OAuth session expired and could not be refreshed"}'
  else
    echo '{"type":"stream_event","event":{"type":"message_start","message":{"id":"m","model":"claude-sonnet-5","usage":{"input_tokens":1}}}}'
    echo '{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"healthy account"}}}'
    echo '{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}}'
    echo '{"type":"stream_event","event":{"type":"message_stop"}}'
    echo '{"type":"result","is_error":false,"result":""}'
  fi
done
`
	binary := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	if err := os.WriteFile(filepath.Join(binary, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"first", "second"} {
		body := `{"model":"claude/claude-sonnet-5","max_tokens":100,"messages":[{"role":"user","content":"` + text + `"}]}`
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body)))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "healthy account") {
			t.Fatalf("%s: %d %s", text, rec.Code, rec.Body.String())
		}
	}
	calls, _ := os.ReadFile(logfile)
	if strings.Count(string(calls), "tok-a\n") != 1 {
		t.Fatalf("rejected login retried: %s", calls)
	}
	var first *Route
	for _, route := range s.Trace(context.Background(), 0, 0).Routes {
		if len(route.Tries) == 2 {
			r := route
			first = &r
		}
	}
	if first == nil || first.Tries[0].Fail != failAuth || first.Tries[0].Rest != nil ||
		!strings.Contains(first.Tries[0].Error, "OAuth session expired") {
		t.Fatalf("authentication reason lost or shown as a timed rest: %+v", first)
	}
}
