package xdcc

import "testing"

func TestParseCTCPRes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantNil  bool
		wantErr  bool
		wantFile string
		wantIP   string
		wantPort int
		wantSize int64
	}{
		{
			name:     "quoted filename with spaces and uint32 ip",
			input:    "SEND \"[SubsPlease] Kyouran Reijou Nia Liston - 01 (1080p) [7F8BCA0B].mkv\" 2163345637 30013 985924527",
			wantFile: "[SubsPlease] Kyouran Reijou Nia Liston - 01 (1080p) [7F8BCA0B].mkv",
			wantIP:   "128.242.8.229",
			wantPort: 30013,
			wantSize: 985924527,
		},
		{
			name:     "unquoted filename",
			input:    "SEND ubuntu.iso 2130706433 1024 5368709120",
			wantFile: "ubuntu.iso",
			wantIP:   "127.0.0.1",
			wantPort: 1024,
			wantSize: 5368709120,
		},
		{
			name:    "posix path traversal rejected",
			input:   "SEND \"../../etc/passwd\" 2130706433 5555 100",
			wantErr: true,
		},
		{
			name:    "windows path traversal rejected",
			input:   "SEND \"..\\..\\evil.exe\" 2130706433 5555 100",
			wantErr: true,
		},
		{
			name:    "version ignored",
			input:   "\x01VERSION\x01",
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := parseCTCPRes(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseCTCPRes(%q) = %+v, want error", tc.input, resp)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCTCPRes(%q) unexpected error: %v", tc.input, err)
			}
			if tc.wantNil {
				if resp != nil {
					t.Fatalf("parseCTCPRes(%q) = %+v, want nil", tc.input, resp)
				}
				return
			}
			res := resp.(*XdccSendRes)
			if res.FileName != tc.wantFile || res.IP.String() != tc.wantIP || res.Port != tc.wantPort || res.FileSize != tc.wantSize {
				t.Fatalf("got %+v, want file=%q ip=%s port=%d size=%d", res, tc.wantFile, tc.wantIP, tc.wantPort, tc.wantSize)
			}
		})
	}
}
