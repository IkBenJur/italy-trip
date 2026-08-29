package env

import (
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
		wantUTC string
	}{
		{
			name:    "rfc3339 with positive offset",
			value:   "2026-09-14T23:59:59+02:00",
			wantUTC: "2026-09-14T21:59:59Z",
		},
		{
			name:    "rfc3339 in UTC",
			value:   "2026-09-14T21:59:59Z",
			wantUTC: "2026-09-14T21:59:59Z",
		},
		{
			name:    "rfc3339 with negative offset",
			value:   "2026-09-14T17:59:59-04:00",
			wantUTC: "2026-09-14T21:59:59Z",
		},
		{
			name:    "surrounding whitespace is tolerated",
			value:   "  2026-09-14T23:59:59+02:00\n",
			wantUTC: "2026-09-14T21:59:59Z",
		},
		{name: "bare date", value: "2026-09-14", wantErr: true},
		{name: "date and time without offset", value: "2026-09-14T23:59:59", wantErr: true},
		{name: "empty string", value: "", wantErr: true},
		{name: "whitespace only", value: "   ", wantErr: true},
		{name: "garbage", value: "garbage", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTime(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseTime(%q) = %v, want error", tt.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTime(%q) returned unexpected error: %v", tt.value, err)
			}
			if gotUTC := got.UTC().Format(time.RFC3339); gotUTC != tt.wantUTC {
				t.Fatalf("ParseTime(%q).UTC() = %s, want %s", tt.value, gotUTC, tt.wantUTC)
			}
		})
	}
}

// captureFatal swaps the package's fatal hook for one that panics, so a test can
// observe an abort without killing the test binary.
func captureFatal(t *testing.T) {
	t.Helper()
	original := fatal
	fatal = func(message string) { panic(message) }
	t.Cleanup(func() { fatal = original })
}

func expectFatal(t *testing.T, label string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s: expected boot to abort, but it returned normally", label)
		}
	}()
	fn()
}

func TestMustGet(t *testing.T) {
	captureFatal(t)

	t.Setenv("PRESENT_KEY", "value")
	if got := MustGet("PRESENT_KEY"); got != "value" {
		t.Fatalf("MustGet = %q, want %q", got, "value")
	}

	t.Setenv("BLANK_KEY", "   ")
	expectFatal(t, "blank value", func() { MustGet("BLANK_KEY") })
	expectFatal(t, "unset key", func() { MustGet("DEFINITELY_UNSET_KEY") })
}

func TestMustTime(t *testing.T) {
	captureFatal(t)

	t.Setenv("EVENT_ENDS_AT", "2026-09-14T23:59:59+02:00")
	got := MustTime("EVENT_ENDS_AT")
	if want := time.Date(2026, 9, 14, 21, 59, 59, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("MustTime = %s, want %s", got, want)
	}

	t.Setenv("EVENT_ENDS_AT", "garbage")
	expectFatal(t, "malformed timestamp", func() { MustTime("EVENT_ENDS_AT") })

	t.Setenv("EVENT_ENDS_AT", "2026-09-14")
	expectFatal(t, "bare date", func() { MustTime("EVENT_ENDS_AT") })
}

func TestGetEnvInt64(t *testing.T) {
	t.Setenv("MAX_UPLOAD_BYTES", "15728640")
	if got := GetEnvInt64("MAX_UPLOAD_BYTES", 1); got != 15728640 {
		t.Fatalf("GetEnvInt64 = %d, want 15728640", got)
	}

	t.Setenv("MAX_UPLOAD_BYTES", "not-a-number")
	if got := GetEnvInt64("MAX_UPLOAD_BYTES", 42); got != 42 {
		t.Fatalf("GetEnvInt64 on garbage = %d, want fallback 42", got)
	}

	if got := GetEnvInt64("UNSET_INT64_KEY", 7); got != 7 {
		t.Fatalf("GetEnvInt64 on unset = %d, want fallback 7", got)
	}
}
