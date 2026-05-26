package backup

import (
	"testing"
)

func TestParseBackupMode(t *testing.T) {
	tests := []struct {
		input   string
		want    BackupMode
		wantErr bool
	}{
		{"full", BackupModeFull, false},
		{"incremental", BackupModeIncremental, false},
		{"differential", BackupModeDifferential, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseBackupMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBackupMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseBackupMode(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseBackupType(t *testing.T) {
	tests := []struct {
		input   string
		want    BackupType
		wantErr bool
	}{
		{"logical", BackupTypeLogical, false},
		{"physical", BackupTypePhysical, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseBackupType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBackupType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseBackupType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseOutputFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    OutputFormat
		wantErr bool
	}{
		{"text", OutputFormatText, false},
		{"json", OutputFormatJSON, false},
		{"", OutputFormatText, false},
		{"xml", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseOutputFormat(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseOutputFormat(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseOutputFormat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
