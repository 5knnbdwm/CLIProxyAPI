package auth

import (
	"context"
	"strconv"
	"strings"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

// weeklyWindowMinutes is the length of the Codex weekly quota window as reported
// in X-Codex-<window>-Window-Minutes.
const weeklyWindowMinutes = 7 * 24 * 60

// ResetFirstSelector picks the available credential whose weekly quota window
// resets soonest, so quota that would otherwise expire unused is spent first.
// Credentials with no known upcoming weekly reset (never observed, or the
// observed reset already passed) are picked first so their window gets observed.
// Ties keep the deterministic candidate order, like FillFirstSelector.
type ResetFirstSelector struct{}

// Pick selects the available auth with the soonest weekly quota reset.
func (s *ResetFirstSelector) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	_ = opts
	now := time.Now()
	available, err := getSelectorAvailableAuths(ctx, auths, provider, model, now)
	if err != nil {
		return nil, err
	}
	available = preferCodexWebsocketAuths(ctx, provider, available)
	best := available[0]
	bestReset, bestKnown := weeklyQuotaReset(best, now)
	for _, candidate := range available[1:] {
		if !bestKnown {
			break
		}
		reset, known := weeklyQuotaReset(candidate, now)
		if !known || reset.Before(bestReset) {
			best, bestReset, bestKnown = candidate, reset, known
		}
	}
	return best, nil
}

// weeklyQuotaReset returns the next weekly quota reset observed for auth.
// Claude reports it in Anthropic-Ratelimit-Unified-7d-Reset. Codex reports
// rolling windows as primary/secondary, so the weekly one is found by its
// window length rather than its name.
func weeklyQuotaReset(auth *Auth, now time.Time) (time.Time, bool) {
	if auth == nil || len(auth.Quota.Signals) == 0 {
		return time.Time{}, false
	}
	signals := auth.Quota.Signals
	reset, ok := unixSignal(signals, "Anthropic-Ratelimit-Unified-7d-Reset")
	if !ok {
		for _, window := range []string{"Primary", "Secondary"} {
			prefix := "X-Codex-" + window + "-"
			if minutes, okMinutes := intSignal(signals, prefix+"Window-Minutes"); !okMinutes || minutes != weeklyWindowMinutes {
				continue
			}
			if reset, ok = unixSignal(signals, prefix+"Reset-At"); ok {
				break
			}
			if after, okAfter := intSignal(signals, prefix+"Reset-After-Seconds"); okAfter && !auth.Quota.ObservedAt.IsZero() {
				reset, ok = auth.Quota.ObservedAt.Add(time.Duration(after)*time.Second), true
				break
			}
		}
	}
	if !ok || !reset.After(now) {
		return time.Time{}, false
	}
	return reset, true
}

func intSignal(signals map[string]string, name string) (int64, bool) {
	value, err := strconv.ParseInt(strings.TrimSpace(signals[name]), 10, 64)
	return value, err == nil
}

func unixSignal(signals map[string]string, name string) (time.Time, bool) {
	seconds, ok := intSignal(signals, name)
	if !ok || seconds <= 0 {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0), true
}
