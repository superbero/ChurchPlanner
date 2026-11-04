package people

import "testing"

func TestNormalizePhoneRequiresAndNormalizesCountryCode(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "", want: ""},
		{input: "+1 555-0100", want: "+15550100"},
		{input: "+44 (20) 1234 5678", want: "+442012345678"},
		{input: "555-0100", wantErr: true},
		{input: "+1 123", wantErr: true},
		{input: "+01234567", wantErr: true},
		{input: "+1+5550100", wantErr: true},
		{input: "+1234567890123456", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := normalizePhone(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizePhone() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("normalizePhone() = %q, want %q", got, tt.want)
			}
		})
	}
}
