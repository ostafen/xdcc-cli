package xdcc

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

func TestHandleXdccSendResSuccess(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789abcdef"), 12*1024)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer ln.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		ackDone := make(chan error, 1)
		go func() {
			var lastAck uint32
			var buf [4]byte
			for lastAck < uint32(len(payload)) {
				if _, err := io.ReadFull(conn, buf[:]); err != nil {
					ackDone <- err
					return
				}
				lastAck = binary.BigEndian.Uint32(buf[:])
			}
			ackDone <- nil
		}()

		if _, err := conn.Write(payload); err != nil {
			serverErr <- err
			return
		}
		serverErr <- <-ackDone
	}()

	outDir := filepath.Join(t.TempDir(), "sub")
	transfer := &XdccTransfer{
		filePath: outDir,
		url:      IRCFile{Network: "127.0.0.1", Channel: "#test", UserName: "bot", Slot: 1},
		events:   make(chan TransferEvent, defaultEventChanSize),
	}
	transfer.handleXdccSendRes(&XdccSendRes{
		FileName: "video test.mkv",
		IP:       net.ParseIP("127.0.0.1"),
		Port:     ln.Addr().(*net.TCPAddr).Port,
		FileSize: int64(len(payload)),
	})

	timeout := time.After(5 * time.Second)
	for done := false; !done; {
		select {
		case evt := <-transfer.PollEvents():
			switch e := evt.(type) {
			case *TransferCompletedEvent:
				done = true
			case *TransferAbortedEvent:
				t.Fatalf("unexpected abort: %s", e.Error)
			}
		case <-timeout:
			t.Fatal("timed out waiting for completion")
		}
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("mock DCC server error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(outDir, "video test.mkv"))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("file content mismatch (err=%v, len=%d)", err, len(got))
	}
}

func TestHandleXdccSendResTruncatedEOF(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = conn.Write(bytes.Repeat([]byte("A"), 5000))
		_ = conn.Close()
	}()

	transfer := &XdccTransfer{
		filePath: t.TempDir(),
		url:      IRCFile{Network: "127.0.0.1", Channel: "#test", UserName: "bot", Slot: 1},
		events:   make(chan TransferEvent, defaultEventChanSize),
	}
	transfer.handleXdccSendRes(&XdccSendRes{
		FileName: "truncated.mkv",
		IP:       net.ParseIP("127.0.0.1"),
		Port:     ln.Addr().(*net.TCPAddr).Port,
		FileSize: 10000,
	})

	timeout := time.After(5 * time.Second)
	for {
		select {
		case evt := <-transfer.PollEvents():
			switch evt.(type) {
			case *TransferCompletedEvent:
				t.Fatal("expected abort on truncated EOF, got completion")
			case *TransferAbortedEvent:
				return
			}
		case <-timeout:
			t.Fatal("timed out waiting for abort")
		}
	}
}
