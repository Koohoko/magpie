package provider

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

const claudeAuthLapse = "Claude Code could not authenticate this account; sign in again in magpie"

// ClaudeSignInRequired recognizes a CLI refusal that needs a new login,
// not a network failure or another process temporarily holding the refresh lock.
func ClaudeSignInRequired(message string) bool {
	for _, text := range []string{
		"OAuth session expired and could not be refreshed",
		"OAuth access token has been revoked",
		"OAuth token revoked",
		claudeAuthLapse,
		claudeLogoutLapse,
		legacyClaudeLogoutLapse,
	} {
		if strings.Contains(message, text) {
			return true
		}
	}
	return false
}

func claudeLoginVersion(l savedLogin) string {
	c, ok := parseClaudeCredentials(l.Auth)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(c.OAuth.AccessToken+"\x00"+c.OAuth.RefreshToken)))
}

// ClaudeLoginVersion identifies the saved login a CLI run starts with.
// An error from an older run must not invalidate a newly authorized login.
func ClaudeLoginVersion(user string) string {
	loginsMu.Lock()
	defer loginsMu.Unlock()
	for _, l := range readLogins() {
		if l.Agent == "claude" && strings.EqualFold(l.User, user) {
			return claudeLoginVersion(l)
		}
	}
	return ""
}

// NoteClaudeSignInFailure records a terminal authentication refusal for
// this login only. The original CLI error remains in the request trace.
func NoteClaudeSignInFailure(user, version, message string) {
	if version == "" || !ClaudeSignInRequired(message) {
		return
	}
	loginsMu.Lock()
	defer loginsMu.Unlock()
	ls := readLogins()
	for i := range ls {
		if ls[i].Agent == "claude" && strings.EqualFold(ls[i].User, user) &&
			claudeLoginVersion(ls[i]) == version && ls[i].Lapsed == "" {
			ls[i].Lapsed = claudeAuthLapse
			_ = writeLogins(ls)
			return
		}
	}
}

// SignInError is a saved Claude account's terminal login state.
func (a *Account) SignInError() string {
	if a == nil || a.Agent != "claude" {
		return ""
	}
	loginsMu.Lock()
	defer loginsMu.Unlock()
	for _, l := range readLogins() {
		if l.Agent == "claude" && strings.EqualFold(l.User, a.User) && l.Lapsed != "" {
			return claudeSignedOut(l)
		}
	}
	return ""
}
