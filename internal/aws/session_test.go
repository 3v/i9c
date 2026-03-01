package aws

import (
	"context"
	"errors"
	"testing"
)

func TestClassifySessionErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want SessionStatus
	}{
		{name: "expired", err: errors.New("ExpiredToken"), want: StatusExpired},
		{name: "no-session", err: errors.New("SSO login required"), want: StatusNoSession},
		{name: "denied", err: errors.New("AccessDenied"), want: StatusDenied},
		{name: "deadline", err: context.DeadlineExceeded, want: StatusDenied},
		{name: "other", err: errors.New("boom"), want: StatusError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifySessionErr(tc.err); got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}
