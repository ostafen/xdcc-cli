package xdcc

import (
	"errors"
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		input   string
		want    IRCFile
		wantErr error
	}{
		{
			input: "irc://irc.rizon.net/#subsplease/Gattsu/#15326",
			want:  IRCFile{Network: "irc.rizon.net", Channel: "#subsplease", UserName: "Gattsu", Slot: 15326},
		},
		{
			input: "irc://irc.rizon.net/#subsplease/Gattsu/15326",
			want:  IRCFile{Network: "irc.rizon.net", Channel: "#subsplease", UserName: "Gattsu", Slot: 15326},
		},
		{
			input: "irc://irc.rizon.net/subsplease/Gattsu/#42",
			want:  IRCFile{Network: "irc.rizon.net", Channel: "#subsplease", UserName: "Gattsu", Slot: 42},
		},
		{input: "irc.rizon.net/#subsplease/Gattsu/1", wantErr: ErrInvalidURL},
		{input: "irc://irc.rizon.net/#subsplease/Gattsu", wantErr: ErrInvalidURL},
	}

	for _, tc := range tests {
		got, err := ParseURL(tc.input)
		if tc.wantErr != nil {
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ParseURL(%q) err = %v, want %v", tc.input, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseURL(%q) unexpected error: %v", tc.input, err)
		}
		if *got != tc.want {
			t.Fatalf("ParseURL(%q) = %+v, want %+v", tc.input, *got, tc.want)
		}
	}

	if _, err := ParseURL("irc://irc.rizon.net/#subsplease/Gattsu/#abc"); err == nil {
		t.Fatal("expected error for non-numeric slot")
	}
}
