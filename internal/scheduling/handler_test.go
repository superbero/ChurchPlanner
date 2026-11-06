package scheduling

import (
	"net/http/httptest"
	"net/url"
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

func TestParseServiceTemplateInput(t *testing.T) {
	form := url.Values{
		"service_type_id":  {"type-id"},
		"name":             {"Sunday morning"},
		"section_kind":     {"service", "rehearsal", "other"},
		"section_name":     {"Service", "Rehearsal", "Prayer"},
		"section_offset":   {"0", "-60", "15"},
		"section_duration": {"0", "45", "20"},
	}
	request := httptest.NewRequest("POST", "/plans/templates", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	template, err := parseServiceTemplateInput(request)
	if err != nil {
		t.Fatalf("parseServiceTemplateInput() error = %v", err)
	}
	if template.ServiceTypeID != "type-id" || template.Name != "Sunday morning" {
		t.Fatalf("template identity = (%q, %q)", template.ServiceTypeID, template.Name)
	}
	if len(template.Times) != 3 || template.Times[1].StartOffsetMinutes != -60 ||
		template.Times[2].Position != 2 || template.Times[2].DurationMinutes != 20 {
		t.Fatalf("unexpected template times: %+v", template.Times)
	}
}

func TestParseServiceTemplateInputRejectsInvalidSections(t *testing.T) {
	tests := []struct {
		name string
		form url.Values
	}{
		{
			name: "missing service section",
			form: url.Values{
				"service_type_id":  {"type-id"},
				"name":             {"Template"},
				"section_kind":     {"rehearsal"},
				"section_name":     {"Rehearsal"},
				"section_offset":   {"0"},
				"section_duration": {"30"},
			},
		},
		{
			name: "service section offset",
			form: url.Values{
				"service_type_id":  {"type-id"},
				"name":             {"Template"},
				"section_kind":     {"service"},
				"section_name":     {"Service"},
				"section_offset":   {"10"},
				"section_duration": {"0"},
			},
		},
		{
			name: "duration out of range",
			form: url.Values{
				"service_type_id":  {"type-id"},
				"name":             {"Template"},
				"section_kind":     {"service", "other"},
				"section_name":     {"Service", "Prayer"},
				"section_offset":   {"0", "0"},
				"section_duration": {"0", "721"},
			},
		},
		{
			name: "incomplete sections",
			form: url.Values{
				"service_type_id": {"type-id"},
				"name":            {"Template"},
				"section_kind":    {"service"},
				"section_name":    {"Service"},
				"section_offset":  {"0"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/plans/templates", strings.NewReader(test.form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if _, err := parseServiceTemplateInput(request); err == nil {
				t.Fatal("parseServiceTemplateInput() unexpectedly succeeded")
			}
		})
	}
}
