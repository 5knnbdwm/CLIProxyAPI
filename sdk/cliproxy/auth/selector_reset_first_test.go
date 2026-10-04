package auth

import (
	"context"
	"strconv"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func codexWeeklyAuth(id string, resetAt time.Time) *Auth {
	return &Auth{ID: id, Quota: QuotaState{
		ObservedAt: time.Now(),
		Signals: map[string]string{
			"X-Codex-Primary-Window-Minutes": "10080",
			"X-Codex-Primary-Reset-At":       strconv.FormatInt(resetAt.Unix(), 10),
		},
	}}
}

func TestResetFirstSelectorPick(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name  string
		auths []*Auth
		want  string
	}{
		{
			name:  "soonest weekly reset wins",
			auths: []*Auth{codexWeeklyAuth("a", now.Add(5*24*time.Hour)), codexWeeklyAuth("b", now.Add(24*time.Hour))},
			want:  "b",
		},
		{
			name:  "unobserved credential is probed first",
			auths: []*Auth{codexWeeklyAuth("a", now.Add(time.Hour)), {ID: "b"}},
			want:  "b",
		},
		{
			name:  "passed reset counts as unknown",
			auths: []*Auth{codexWeeklyAuth("a", now.Add(time.Hour)), codexWeeklyAuth("b", now.Add(-time.Hour))},
			want:  "b",
		},
		{
			name: "codex five hour window is ignored",
			auths: []*Auth{
				codexWeeklyAuth("a", now.Add(3*24*time.Hour)),
				{ID: "b", Quota: QuotaState{Signals: map[string]string{
					"X-Codex-Primary-Window-Minutes":   "300",
					"X-Codex-Primary-Reset-At":         strconv.FormatInt(now.Add(time.Hour).Unix(), 10),
					"X-Codex-Secondary-Window-Minutes": "10080",
					"X-Codex-Secondary-Reset-At":       strconv.FormatInt(now.Add(4*24*time.Hour).Unix(), 10),
				}}},
			},
			want: "a",
		},
		{
			name: "claude seven day reset",
			auths: []*Auth{
				{ID: "a", Quota: QuotaState{Signals: map[string]string{"Anthropic-Ratelimit-Unified-7d-Reset": strconv.FormatInt(now.Add(6*24*time.Hour).Unix(), 10)}}},
				{ID: "b", Quota: QuotaState{Signals: map[string]string{"Anthropic-Ratelimit-Unified-7d-Reset": strconv.FormatInt(now.Add(2*24*time.Hour).Unix(), 10)}}},
			},
			want: "b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := (&ResetFirstSelector{}).Pick(context.Background(), "codex", "", cliproxyexecutor.Options{}, tt.auths)
			if err != nil {
				t.Fatalf("Pick() error = %v", err)
			}
			if got.ID != tt.want {
				t.Fatalf("Pick() auth.ID = %q, want %q", got.ID, tt.want)
			}
		})
	}
}
