package events

import (
	"testing"
	"time"
)

func TestEventIsOver(t *testing.T) {
	// 2026-09-14T23:59:59+02:00 — the trip ends in CEST while the server runs UTC.
	cest := time.FixedZone("CEST", 2*60*60)
	endsAt := time.Date(2026, 9, 14, 23, 59, 59, 0, cest)
	event := Event{Name: "Italy Trip", EndsAt: endsAt}

	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{
			name: "one second before ends_at",
			now:  endsAt.Add(-time.Second),
			want: false,
		},
		{
			name: "exactly ends_at is over: the boundary is inclusive",
			now:  endsAt,
			want: true,
		},
		{
			name: "one second after ends_at",
			now:  endsAt.Add(time.Second),
			want: true,
		},
		{
			name: "a nanosecond before ends_at is still locked",
			now:  endsAt.Add(-time.Nanosecond),
			want: false,
		},
		{
			name: "same instant expressed in UTC is over",
			now:  time.Date(2026, 9, 14, 21, 59, 59, 0, time.UTC),
			want: true,
		},
		{
			name: "same instant expressed in a third offset is over",
			now:  time.Date(2026, 9, 14, 17, 59, 59, 0, time.FixedZone("EDT", -4*60*60)),
			want: true,
		},
		{
			name: "a UTC wall clock that merely looks past ends_at is not",
			// 23:00 UTC on the 14th is 01:00 CEST on the 15th, genuinely over.
			now:  time.Date(2026, 9, 14, 23, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "a UTC wall clock an hour earlier is still locked",
			// 21:00 UTC is 23:00 CEST, one minute short of the end.
			now:  time.Date(2026, 9, 14, 21, 0, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "long before the trip",
			now:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := event.IsOver(tt.now); got != tt.want {
				t.Fatalf("IsOver(%s) = %v, want %v", tt.now.Format(time.RFC3339Nano), got, tt.want)
			}
		})
	}
}
