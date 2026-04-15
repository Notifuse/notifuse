package smtp_bridge

import "testing"

func TestValidateMode(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"off", false},
		{"starttls", false},
		{"implicit", false},
		{"", true},
		{"true", true},
		{"false", true},
		{"none", true},
		{"yes", true},
		{"STARTTLS", true}, // case sensitive
		{"Off", true},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			err := ValidateMode(tc.in)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateMode(%q): expected error, got nil", tc.in)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateMode(%q): expected no error, got %v", tc.in, err)
			}
		})
	}
}
