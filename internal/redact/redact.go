// Package redact removes secrets from strings and errors before they reach
// logs, metrics, HTTP responses or alerts.
//
// RPC URLs embed API keys, and webhook URLs and bot tokens are credentials.
// The rule in oraclewatch is that such values are registered here as soon as
// they are read from the environment, and every string that leaves the process
// passes through a Redactor. Code that refers to an endpoint uses its
// configured name, never its URL.
package redact

import (
	"errors"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// Placeholder replaces every secret occurrence.
const Placeholder = "[REDACTED]"

// minSecretLen avoids replacing short, common substrings (an empty chat ID or
// a one-character value would otherwise mangle every message).
const minSecretLen = 6

// minPartLen is the minimum length of a URL path segment or query value that
// is treated as a secret on its own. API keys and tokens are far longer.
const minPartLen = 16

// Redactor holds the set of known secrets. The zero value is ready to use and
// it is safe for concurrent use.
type Redactor struct {
	mu      sync.RWMutex
	secrets []string // sorted longest first so that overlapping secrets are fully removed
}

// Add registers a secret. URLs are also registered in their normalised form
// and by their sensitive parts (userinfo, path, query), because net/http and
// other libraries may print a URL differently from how it was configured.
func (r *Redactor) Add(secret string) {
	secret = strings.TrimSpace(secret)
	if len(secret) < minSecretLen {
		return
	}
	candidates := []string{secret}
	if u, err := url.Parse(secret); err == nil && u.Host != "" {
		candidates = append(candidates, u.String(), u.Redacted())
		if u.User != nil {
			candidates = append(candidates, u.User.String())
		}
		if p := strings.Trim(u.EscapedPath(), "/"); p != "" {
			candidates = append(candidates, p)
			// Providers put the key in a path segment (".../v2/<key>"). Only
			// long segments are registered so that words like "webhooks" are
			// not redacted everywhere.
			for _, seg := range strings.Split(p, "/") {
				if len(seg) >= minPartLen {
					candidates = append(candidates, seg)
				}
			}
		}
		if u.RawQuery != "" {
			candidates = append(candidates, u.RawQuery)
			for _, values := range u.Query() {
				for _, v := range values {
					if len(v) >= minPartLen {
						candidates = append(candidates, v)
					}
				}
			}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range candidates {
		if len(c) < minSecretLen || contains(r.secrets, c) {
			continue
		}
		r.secrets = append(r.secrets, c)
	}
	sort.Slice(r.secrets, func(i, j int) bool { return len(r.secrets[i]) > len(r.secrets[j]) })
}

// String returns s with every registered secret replaced by Placeholder.
func (r *Redactor) String(s string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, secret := range r.secrets {
		s = strings.ReplaceAll(s, secret, Placeholder)
	}
	return s
}

// Error returns an error that is safe to log.
//
// A *url.Error anywhere in the chain is always rewritten: its message embeds
// the full request URL, which for an RPC endpoint contains the API key. Its
// URL is removed even if it was never registered, so an unregistered URL
// cannot leak either. Other errors are returned unchanged when their message
// contains no registered secret, which keeps errors.Is/As working for callers
// (for example to detect context.DeadlineExceeded).
//
// When an error is rewritten, the original is not reachable through Unwrap,
// except for the inner error of a *url.Error when that one is itself clean.
func (r *Redactor) Error(err error) error {
	if err == nil {
		return nil
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		msg := strings.ReplaceAll(err.Error(), ue.URL, Placeholder)
		return &redactedError{msg: r.String(msg), inner: r.Error(ue.Err)}
	}
	msg := err.Error()
	if clean := r.String(msg); clean != msg {
		return &redactedError{msg: clean}
	}
	return err
}

type redactedError struct {
	msg   string
	inner error
}

func (e *redactedError) Error() string { return e.msg }

// Unwrap exposes only an inner error that has itself been through Error.
func (e *redactedError) Unwrap() error { return e.inner }

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
