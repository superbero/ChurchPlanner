package scheduling

import (
	"errors"
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

	if got, want := input.ServiceAt.Format("2006-01-02 15:04"),
		"2026-11-02 23:30"; got != want {
		t.Errorf("ServiceAt = %s, want %s", got, want)
	}
	if input.ServiceAt.Location() != time.Local {
		t.Errorf("ServiceAt location = %v, want local time", input.ServiceAt.Location())
	}
	if got, want := input.ServiceEndsAt.Format("2006-01-02 15:04"),
		"2026-11-03 00:45"; got != want {
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
		{name: "zero date", body: "service_date=0001-01-01&service_start=00%3A00&service_end=01%3A00"},
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

func TestNextServiceWindowUsesFollowingWeekAndKeepsDuration(t *testing.T) {
	lastStart := time.Date(2026, time.November, 1, 9, 30, 0, 0, time.UTC)
	lastEnd := lastStart.Add(2 * time.Hour)
	now := time.Date(2026, time.November, 3, 12, 0, 0, 0, time.UTC)

	latest := &Plan{ServiceAt: lastStart, ServiceEndsAt: lastEnd}
	nextStart, nextEnd := suggestedServiceWindow("Cohesion", latest, now)
	if got, want := nextStart.Format(time.RFC3339), "2026-11-08T09:30:00Z"; got != want {
		t.Errorf("next start = %s, want %s", got, want)
	}
	if got, want := nextEnd.Sub(nextStart), 2*time.Hour; got != want {
		t.Errorf("next service duration = %s, want %s", got, want)
	}
}

func TestNextServiceWindowFallsBackToNinetyMinutes(t *testing.T) {
	lastStart := time.Date(2026, time.November, 1, 9, 30, 0, 0, time.UTC)
	now := time.Date(2026, time.November, 3, 12, 0, 0, 0, time.UTC)

	latest := &Plan{ServiceAt: lastStart, ServiceEndsAt: lastStart}
	nextStart, nextEnd := suggestedServiceWindow("Cohesion", latest, now)
	if got, want := nextEnd.Sub(nextStart), 90*time.Minute; got != want {
		t.Errorf("fallback service duration = %s, want %s", got, want)
	}
}

func TestNextServiceWindowAdvancesOldPlanToNextFutureOccurrence(t *testing.T) {
	lastStart := time.Date(2026, time.January, 4, 9, 30, 0, 0, time.UTC)
	lastEnd := lastStart.Add(90 * time.Minute)
	now := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)

	latest := &Plan{ServiceAt: lastStart, ServiceEndsAt: lastEnd}
	nextStart, _ := suggestedServiceWindow("Sunday Morning", latest, now)
	if got, want := nextStart.Format("2006-01-02"), "2026-11-08"; got != want {
		t.Errorf("next occurrence date = %s, want %s", got, want)
	}
}

func TestMidweekSuggestionUsesWednesday(t *testing.T) {
	now := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)
	start, end := suggestedServiceWindow("Midweek", nil, now)
	if got, want := start.Format("2006-01-02"), "2026-11-11"; got != want {
		t.Errorf("midweek date = %s, want Wednesday %s", got, want)
	}
	if got, want := start.Format("15:04"), "10:00"; got != want {
		t.Errorf("midweek default time = %s, want %s", got, want)
	}
	if got, want := end.Sub(start), 90*time.Minute; got != want {
		t.Errorf("midweek duration = %s, want %s", got, want)
	}
}

func TestMidweekLatestPlanStillAlignsToWednesday(t *testing.T) {
	now := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)
	lastStart := time.Date(2026, time.November, 2, 18, 30, 0, 0, time.UTC)
	latest := &Plan{ServiceAt: lastStart, ServiceEndsAt: lastStart.Add(time.Hour)}
	start, _ := suggestedServiceWindow("Midweek", latest, now)
	if got, want := start.Format("2006-01-02 15:04"), "2026-11-11 18:30"; got != want {
		t.Errorf("next midweek plan = %s, want %s", got, want)
	}
}

func TestZeroLatestPlanFallsBackToServiceTypeSuggestion(t *testing.T) {
	now := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)
	latest := &Plan{}
	start, _ := suggestedServiceWindow("Midweek", latest, now)
	if got, want := start.Format("2006-01-02"), "2026-11-11"; got != want {
		t.Errorf("midweek date = %s, want %s", got, want)
	}
}

func TestValidateTemplateTimeConflicts(t *testing.T) {
	serviceStart := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	serviceEnd := serviceStart.Add(90 * time.Minute)
	tests := []struct {
		name    string
		times   []ServiceTemplateTime
		wantErr error
	}{
		{
			name: "adjacent sections do not conflict",
			times: []ServiceTemplateTime{
				{Kind: "service"},
				{Kind: "rehearsal", StartOffsetMinutes: -60, DurationMinutes: 30},
				{Kind: "other", StartOffsetMinutes: -30, DurationMinutes: 30},
			},
		},
		{
			name: "section overlaps primary service",
			times: []ServiceTemplateTime{
				{Kind: "service"},
				{Kind: "other", StartOffsetMinutes: 0, DurationMinutes: 30},
			},
			wantErr: ErrTimeConflict,
		},
		{
			name: "sections overlap each other",
			times: []ServiceTemplateTime{
				{Kind: "service"},
				{Kind: "rehearsal", StartOffsetMinutes: -60, DurationMinutes: 45},
				{Kind: "other", StartOffsetMinutes: -30, DurationMinutes: 30},
			},
			wantErr: ErrTimeConflict,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTemplateTimeConflicts(test.times, serviceStart, serviceEnd)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("validateTemplateTimeConflicts() error = %v, want %v", err, test.wantErr)
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
