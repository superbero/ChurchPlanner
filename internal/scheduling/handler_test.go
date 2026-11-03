package scheduling

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParsePlanInputWithSeparateDateAndTimes(t *testing.T) {
	request := httptest.NewRequest("POST", "/plans",
		strings.NewReader("service_date=2026-11-02&service_start=23%3A30&service_end=00%3A45&title=Evening+service"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	input, err := parsePlanInput(request)
	if err != nil {
		t.Fatalf("parsePlanInput() error = %v", err)
	}
	if got, want := input.ServiceAt.Format(time.RFC3339),
		"2026-11-02T23:30:00Z"; got != want {
		t.Errorf("ServiceAt = %s, want %s", got, want)
	}
	if got, want := input.ServiceEndsAt.Format(time.RFC3339),
		"2026-11-03T00:45:00Z"; got != want {
		t.Errorf("ServiceEndsAt = %s, want %s", got, want)
	}
}

func TestParsePlanInputSupportsLegacyDatetime(t *testing.T) {
	request := httptest.NewRequest("POST", "/plans",
		strings.NewReader("service_at=2026-11-02T10%3A15&title=Sunday+service"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	input, err := parsePlanInput(request)
	if err != nil {
		t.Fatalf("parsePlanInput() error = %v", err)
	}
	if got := input.ServiceEndsAt.Sub(input.ServiceAt); got != 90*time.Minute {
		t.Errorf("default duration = %s, want 1h30m", got)
	}
}

func TestParsePlanInputRejectsPartialOrInvalidTimes(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing end time", body: "service_date=2026-11-02&service_start=10%3A00"},
		{name: "invalid date", body: "service_date=2026-02-30&service_start=10%3A00&service_end=11%3A00"},
		{name: "invalid time", body: "service_date=2026-11-02&service_start=25%3A00&service_end=26%3A00"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/plans",
				strings.NewReader(test.body))
			request.Header.Set("Content-Type",
				"application/x-www-form-urlencoded")
			if _, err := parsePlanInput(request); err == nil {
				t.Fatal("parsePlanInput() unexpectedly succeeded")
			}
		})
	}
}
