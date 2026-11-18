package xdcc

import (
	"bufio"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	irc "github.com/fluffle/goirc/client"
)

const IRCClientUserName = "xdcc-cli"

type CTCPRequest interface {
	String() string
}

type CTCPResponse interface {
	Parse(args []string) error
	Name() string
}

type XdccSendReq struct {
	Slot int
}

func (send *XdccSendReq) String() string {
	return fmt.Sprintf("xdcc send #%d", send.Slot)
}

type XdccSendRes struct {
	FileName string
	IP       net.IP
	Port     int
	FileSize int64
}

func uint32ToIP(n uint32) net.IP {
	a := byte((n >> 24) & 255)
	b := byte((n >> 16) & 255)
	c := byte((n >> 8) & 255)
	d := byte(n & 255)
	return net.IPv4(a, b, c, d)
}

const XdccSendResMinArgs = 4

func (send *XdccSendRes) Name() string {
	return SEND
}

func (send *XdccSendRes) Parse(args []string) error {
	if len(args) < XdccSendResMinArgs {
		return errors.New("invalid number of arguments")
	}

	rawName := strings.Trim(strings.Join(args[:len(args)-3], " "), "\"")
	cleanName := filepath.Base(strings.ReplaceAll(rawName, "\\", "/"))
	if strings.ContainsAny(rawName, "/\\") || cleanName == "" || cleanName == "." || cleanName == ".." || cleanName == "/" {
		return fmt.Errorf("invalid filename: %q", rawName)
	}
	send.FileName = cleanName

	ipUint32, err := strconv.ParseUint(args[len(args)-3], 10, 32)
	if err != nil {
		return err
	}
	send.IP = uint32ToIP(uint32(ipUint32))

	send.Port, err = strconv.Atoi(args[len(args)-2])
	if err != nil {
		return err
	}

	send.FileSize, err = strconv.ParseInt(args[len(args)-1], 10, 64)
	if err != nil || send.FileSize <= 0 {
		return errors.New("invalid file size")
	}
	return nil
}

const (
	SEND    = "SEND"
	VERSION = "VERSION"
)

func parseCTCPRes(text string) (CTCPResponse, error) {
	fields := strings.Fields(strings.Trim(text, "\x01"))
	if len(fields) == 0 {
		return nil, nil
	}

	var resp CTCPResponse
	switch strings.ToUpper(strings.TrimSpace(fields[0])) {
	case SEND:
		resp = &XdccSendRes{}
	case VERSION:
		return nil, nil
	default:
		return nil, nil
	}

	if err := resp.Parse(fields[1:]); err != nil {
		return nil, err
	}
	return resp, nil
}

const defaultEventChanSize = 1024

func (transfer *XdccTransfer) Start() error {
	return transfer.conn.Connect()
}

type TransferEvent interface{}

type TransferAbortedEvent struct {
	Error string
}

const maxConnAttempts = 5

type Transfer interface {
	Start() error
	PollEvents() chan TransferEvent
}

type retryTransfer struct {
	*XdccTransfer
	conf Config
}

func (t *retryTransfer) Start() error {
	t1 := newXdccTransfer(t.conf, true, false)
	if err := t1.conn.Connect(); err == nil {
		t.XdccTransfer = t1
		return nil
	}

	t2 := newXdccTransfer(t.conf, true, true)
	if err := t2.conn.Connect(); err == nil {
		t.XdccTransfer = t2
		return nil
	}

	t.XdccTransfer = newXdccTransfer(t.conf, false, false)
	return t.XdccTransfer.conn.Connect()
}

func (t *retryTransfer) PollEvents() chan TransferEvent {
	return t.XdccTransfer.PollEvents()
}

type XdccTransfer struct {
	filePath     string
	url          IRCFile
	conn         *irc.Conn
	connAttempts int
	requested    bool
	started      bool
	events       chan TransferEvent
}

type Config struct {
	File    IRCFile
	OutPath string
	SSLOnly bool
}

func NewTransfer(c Config) Transfer {
	if c.SSLOnly {
		return newXdccTransfer(c, true, false)
	}

	return &retryTransfer{
		conf: c,
	}
}

func newXdccTransfer(c Config, enableSSL bool, skipCertificateCheck bool) *XdccTransfer {
	nick := fmt.Sprintf("xdcc%05d", rand.Intn(100000))

	file := c.File

	config := irc.NewConfig(nick)
	config.SSL = enableSSL
	config.SSLConfig = &tls.Config{ServerName: file.Network, InsecureSkipVerify: skipCertificateCheck}
	config.Server = file.Network
	config.NewNick = func(string) string {
		return fmt.Sprintf("xdcc%05d", rand.Intn(100000))
	}

	conn := irc.Client(config)

	t := &XdccTransfer{
		conn:         conn,
		url:          file,
		filePath:     c.OutPath,
		started:      false,
		connAttempts: 0,
		events:       make(chan TransferEvent, defaultEventChanSize),
	}
	t.setupHandlers(file.Channel, file.UserName, file.Slot)
	return t
}

func (transfer *XdccTransfer) send(req CTCPRequest) {
	if transfer.conn != nil {
		transfer.conn.Privmsg(transfer.url.UserName, req.String())
	}
}

func (transfer *XdccTransfer) setupHandlers(channel string, userName string, slot int) {
	conn := transfer.conn

	// e.g. join channel on connect.
	conn.HandleFunc(irc.CONNECTED,
		func(conn *irc.Conn, line *irc.Line) {
			transfer.connAttempts = 0
			conn.Join(channel)
		})

	conn.HandleFunc(irc.ERROR, func(conn *irc.Conn, line *irc.Line) {

	})

	// send xdcc send once NAMES list finishes on channel join
	conn.HandleFunc("366",
		func(conn *irc.Conn, line *irc.Line) {
			if len(line.Args) >= 2 && strings.EqualFold(line.Args[1], channel) && !transfer.requested && !transfer.started {
				transfer.requested = true
				time.AfterFunc(2*time.Second, func() {
					transfer.send(&XdccSendReq{Slot: slot})
				})
			}
		})

	conn.HandleFunc(irc.PRIVMSG, func(conn *irc.Conn, line *irc.Line) {})

	conn.HandleFunc(irc.CTCP,
		func(conn *irc.Conn, line *irc.Line) {
			if len(line.Args) < 2 || line.Args[1] == "" || line.Public() || !strings.EqualFold(line.Nick, userName) {
				return
			}
			res, err := parseCTCPRes(line.Text())
			if err != nil {
				transfer.notifyEvent(&TransferAbortedEvent{Error: err.Error()})
				return
			}
			if res != nil {
				transfer.handleCTCPRes(res)
			}
		})

	conn.HandleFunc(irc.DISCONNECTED,
		func(conn *irc.Conn, line *irc.Line) {
			if transfer.started {
				return
			}

			var err error
			if transfer.connAttempts < maxConnAttempts {
				transfer.requested = false
				time.Sleep(time.Second)

				err = conn.Connect()
			}

			if (err != nil || transfer.connAttempts >= maxConnAttempts) && !transfer.started {
				errMsg := "disconnected from server"
				if err != nil {
					errMsg = err.Error()
				}
				transfer.notifyEvent(&TransferAbortedEvent{Error: errMsg})
			}

			transfer.connAttempts++
		})
}

func (transfer *XdccTransfer) PollEvents() chan TransferEvent {
	return transfer.events
}

type TransferProgessEvent struct {
	TransferBytes uint64
	TransferRate  float32
}

const downloadBufSize = 64 * 1024

type TransferStartedEvent struct {
	FileName string
	FileSize uint64
}

type TransferCompletedEvent struct{}

func (transfer *XdccTransfer) notifyEvent(e TransferEvent) {
	select {
	case transfer.events <- e:
	default:
		break
	}
}

type SpeedMonitorReader struct {
	reader       io.Reader
	lastUpdate   time.Time
	currValue    uint64
	currentSpeed float64
	onUpdate     func(amount int, speed float64)
}

func NewSpeedMonitorReader(reader io.Reader, onUpdate func(int, float64)) *SpeedMonitorReader {
	return &SpeedMonitorReader{
		reader:     reader,
		lastUpdate: time.Now(),
		onUpdate:   onUpdate,
	}
}

func (monitor *SpeedMonitorReader) Read(buf []byte) (int, error) {
	n, err := monitor.reader.Read(buf)
	if n > 0 {
		monitor.currValue += uint64(n)
	}
	elapsed := time.Since(monitor.lastUpdate)
	if elapsed >= 250*time.Millisecond && monitor.currValue > 0 {
		monitor.currentSpeed = float64(monitor.currValue) / elapsed.Seconds()
		monitor.onUpdate(int(monitor.currValue), monitor.currentSpeed)
		monitor.currValue = 0
		monitor.lastUpdate = time.Now()
	}
	return n, err
}

func (monitor *SpeedMonitorReader) Flush() {
	if monitor.currValue > 0 {
		elapsed := time.Since(monitor.lastUpdate)
		if elapsed > 0 {
			monitor.currentSpeed = float64(monitor.currValue) / elapsed.Seconds()
		}
		monitor.onUpdate(int(monitor.currValue), monitor.currentSpeed)
		monitor.currValue = 0
		monitor.lastUpdate = time.Now()
	}
}

func (transfer *XdccTransfer) handleXdccSendRes(send *XdccSendRes) {
	if transfer.started {
		return
	}
	transfer.started = true

	go func() {
		addr := net.JoinHostPort(send.IP.String(), strconv.Itoa(send.Port))
		var conn net.Conn
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			conn, err = net.DialTimeout("tcp", addr, 5*time.Second)
			if err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if err != nil {
			transfer.notifyEvent(&TransferAbortedEvent{Error: fmt.Sprintf("unable to reach host %s: %v", addr, err)})
			return
		}
		defer conn.Close()

		if err := os.MkdirAll(transfer.filePath, 0755); err != nil {
			transfer.notifyEvent(&TransferAbortedEvent{Error: err.Error()})
			return
		}

		file, err := os.OpenFile(filepath.Join(transfer.filePath, send.FileName), os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			transfer.notifyEvent(&TransferAbortedEvent{Error: err.Error()})
			return
		}
		defer file.Close()
		fileWriter := bufio.NewWriter(file)

		transfer.notifyEvent(&TransferStartedEvent{
			FileName: send.FileName,
			FileSize: uint64(send.FileSize),
		})

		reader := NewSpeedMonitorReader(conn, func(dowloadedAmount int, speed float64) {
			transfer.notifyEvent(&TransferProgessEvent{
				TransferRate:  float32(speed),
				TransferBytes: uint64(dowloadedAmount),
			})
		})

		// download loop
		downloadedBytesTotal := int64(0)
		buf := make([]byte, downloadBufSize)
		var ack [4]byte
		for downloadedBytesTotal < send.FileSize {
			n, err := reader.Read(buf)
			if n > 0 {
				if _, werr := fileWriter.Write(buf[:n]); werr != nil {
					transfer.notifyEvent(&TransferAbortedEvent{Error: werr.Error()})
					return
				}

				downloadedBytesTotal += int64(n)
				binary.BigEndian.PutUint32(ack[:], uint32(downloadedBytesTotal))
				_, _ = conn.Write(ack[:])
			}

			if err != nil {
				if err == io.EOF {
					break
				}
				transfer.notifyEvent(&TransferAbortedEvent{Error: err.Error()})
				return
			}
		}

		if downloadedBytesTotal < send.FileSize {
			transfer.notifyEvent(&TransferAbortedEvent{Error: fmt.Sprintf("incomplete transfer: got %d of %d bytes", downloadedBytesTotal, send.FileSize)})
			return
		}

		reader.Flush()
		if err := fileWriter.Flush(); err != nil {
			transfer.notifyEvent(&TransferAbortedEvent{Error: err.Error()})
			return
		}

		if transfer.conn != nil && transfer.conn.Connected() {
			transfer.conn.Quit()
		}
		transfer.notifyEvent(&TransferCompletedEvent{})
	}()
}

func (transfer *XdccTransfer) handleCTCPRes(resp CTCPResponse) {
	switch r := resp.(type) {
	case *XdccSendRes:
		transfer.handleXdccSendRes(r)
	}
}
