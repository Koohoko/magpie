package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func claudeAuthFixture(t *testing.T, lapse string) (Provider, savedLogin) {
	t.Helper()
	testHome := claudeHome(t)
	noAnthropic(t)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	claudeSignIn(t, testHome, time.Now().Add(time.Hour))
	writeFile(t, filepath.Join(testHome, ".claude.json"), map[string]any{
		"oauthAccount": map[string]any{"emailAddress": "own@example.com"},
	})
	side := savedLogin{Agent: "claude", User: "side@example.com", On: true, Plan: "max", Seen: time.Now().Add(-time.Hour), Lapsed: lapse,
		Auth: mustJSONRaw(t, map[string]any{"claudeAiOauth": map[string]any{
			"accessToken": "old-access", "refreshToken": "old-refresh",
			"expiresAt": time.Now().Add(time.Hour).UnixMilli(), "subscriptionType": "max",
		}})}
	writeFile(t, loginsPath(), []savedLogin{side})
	forgetAccountCaches()
	p, ok := find(All(), "claude")
	if !ok {
		t.Fatal("own Claude account missing")
	}
	return p, side
}

func TestClaudeLapsedLoginIsNotRoutedOrRestored(t *testing.T) {
	p, side := claudeAuthFixture(t, legacyClaudeLogoutLapse)
	if also := p.AlsoOn(); len(also) != 0 {
		t.Fatalf("lapsed account still routed: %+v", also)
	}
	if _, err := claudeSavedDir(side.User); err == nil || !strings.Contains(err.Error(), "sign in again") {
		t.Fatalf("lapsed login restored: %v", err)
	}
	if _, err := os.Stat(claudeAccountDir(side.User)); !os.IsNotExist(err) {
		t.Fatalf("a directory was created for the lapsed login: %v", err)
	}
	for _, l := range Logins("claude") {
		if l.User == side.User && (l.Lapsed != claudeLogoutLapse || strings.Contains(l.Lapsed, "/logout")) {
			t.Fatalf("absence blamed on logout: %+v", l)
		}
	}
}

func TestClaudeAuthFailureRequiresNewLoginAndIgnoresOldRuns(t *testing.T) {
	p, side := claudeAuthFixture(t, "")
	oldVersion := ClaudeLoginVersion(side.User)
	NoteClaudeSignInFailure(side.User, oldVersion, "Failed to authenticate: OAuth session expired and could not be refreshed")
	if len(p.AlsoOn()) != 0 {
		t.Fatal("authentication failure was not removed from routing")
	}
	if err := SwitchLogin("claude", side.User); err == nil {
		t.Fatal("switching restored a rejected login")
	}
	c, _ := parseClaudeCredentials(side.Auth)
	c.OAuth.AccessToken, c.OAuth.RefreshToken = "new-access", "new-refresh"
	side.Auth, _ = c.marshal()
	if _, err := addLogin(side); err != nil {
		t.Fatal(err)
	}
	NoteClaudeSignInFailure(side.User, oldVersion, "OAuth token revoked")
	also := p.AlsoOn()
	if len(also) != 1 {
		t.Fatalf("an old run invalidated the new login: %+v", also)
	}
	if _, _, err := also[0].Account.Token(context.Background()); err != nil {
		t.Fatalf("new login could not be used: %v", err)
	}
}

func TestClaudeClearedKeychainDoesNotFallBackToStaleFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands in for the keychain")
	}
	_, side := claudeAuthFixture(t, "")
	dir, err := claudeSavedDir(side.User)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	tombstone := filepath.Join(bin, "cleared.json")
	writeFile(t, tombstone, map[string]any{"claudeAiOauth": map[string]any{
		"accessToken": "", "refreshToken": "", "expiresAt": 0, "subscriptionType": "max",
	}})
	script := "#!/bin/sh\ncat '" + tombstone + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "security"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	claudeKeychain = true
	if _, err := claudeSavedDir(side.User); err == nil {
		t.Fatal("cleared keychain fell back to the old credentials file")
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".credentials.json"))
	if !strings.Contains(string(b), "old-refresh") {
		t.Fatal("the stale file was rewritten rather than refusing it")
	}
	for _, l := range Logins("claude") {
		if l.User == side.User && l.Lapsed == "" {
			t.Fatal("cleared saved login was not marked as needing sign-in")
		}
	}
}

func TestClaudeSignInRequiredDoesNotBlameTemporaryFailures(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    bool
	}{
		{"Failed to authenticate: OAuth session expired and could not be refreshed", true},
		{"OAuth access token has been revoked.", true},
		{"Failed to refresh OAuth token: another Claude Code process is refreshing it or exited mid-refresh", false},
		{"OAuth token refresh failed (HTTP 503)", false},
		{"Failed to authenticate: network connection timed out", false},
		{"You've hit your session limit", false},
	} {
		if got := ClaudeSignInRequired(tc.message); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.message, got, tc.want)
		}
	}
}

func TestClaudeProbeReportsTheAccountAndMarksItsFailedLogin(t *testing.T) {
	p, side := claudeAuthFixture(t, "")
	old := claudeCLIProbe
	t.Cleanup(func() { claudeCLIProbe = old })
	claudeCLIProbe = func(context.Context, string, string) error {
		return errors.New("Failed to authenticate: OAuth session expired and could not be refreshed")
	}
	saved := p.AlsoOn()[0]
	result := saved.testClaude(context.Background(), "claude-sonnet-5")
	if result.OK || result.Account != side.User || !strings.Contains(result.Error, "OAuth session expired") {
		t.Fatalf("failed test lost the account or original error: %+v", result)
	}
	if len(p.AlsoOn()) != 0 {
		t.Fatal("the login that failed its connection test is still routed")
	}
}
