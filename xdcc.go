package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
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
	FileSize int
}

func uint32ToIP(n int) net.IP {
	a := byte((n >> 24) & 255)
	b := byte((n >> 16) & 255)
	c := byte((n >> 8) & 255)
	d := byte(n & 255)
	return net.IPv4(a, b, c, d)
}

const XdccSendResArgs = 4

func (send *XdccSendRes) Name() string {
	return SEND
}

func (send *XdccSendRes) Parse(args []string) error {
	if len(args) != XdccSendResArgs {
		return errors.New("invalid number of arguments")
	}

	send.FileName = args[0]

	ipUint32, err := strconv.Atoi(args[1])

	if err != nil {
		return err
	}

	send.IP = uint32ToIP(ipUint32)
	send.Port, err = strconv.Atoi(args[2])

	if err != nil {
		return err
	}

	send.FileSize, err = strconv.Atoi(args[3])

	if err != nil {
		return err
	}
	return nil
}

const (
	SEND    = "SEND"
	VERSION = "\x01VERSION\x01"
)

func parseCTCPRes(text string) (CTCPResponse, error) {
	fields := strings.Fields(text)

	var resp CTCPResponse = nil

	switch strings.TrimSpace(fields[0]) {
	case SEND:
		resp = &XdccSendRes{}
	case VERSION:
		return nil, nil
	}

	if resp == nil {
		return nil, errors.New("invalid command: " + fields[0])
	}

	err := resp.Parse(fields[1:])
	if err != nil {
		return nil, err
	}
	return resp, nil
}

const defaultEventChanSize = 1024

func (transfer *XdccTransfer) Start() error {
	return transfer.conn.Connect()
}

type XdccEvent interface{}

type TransferAbortedEvent struct {
	Error string
}

type XdccTransfer struct {
	filePath string
	url      IRCFileURL
	conn     *irc.Conn
	started  bool
	events   chan XdccEvent
}

type TransferManager struct {
	transfers map[IRCBot]*XdccTransfer
}

func (tm *TransferManager) addTransfer(fileUrl *IRCFileURL, filePath string) {
	transfer, ok := tm.transfers[fileUrl.GetBot()]

	if !ok {
		transfer = NewXdccTransfer(*fileUrl, filePath)
		tm.transfers[fileUrl.GetBot()] = transfer
		// add transfer
	}

}

func NewXdccTransfer(url IRCFileURL, filePath string) *XdccTransfer {
	conn := irc.SimpleClient(IRCClientUserName + strconv.Itoa(int(rand.Uint32())))
	conn.Config().Server = url.Network
	conn.Config().NewNick = func(nick string) string {
		return nick + "" + strconv.Itoa(int(rand.Uint32()))
	}

	t := &XdccTransfer{
		conn:     conn,
		url:      url,
		filePath: filePath,
		started:  false,
		events:   make(chan XdccEvent, defaultEventChanSize),
	}
	t.setupHandlers(url.Channel, url.UserName, url.Slot)
	return t
}

func (transfer *XdccTransfer) send(req CTCPRequest) {
	transfer.conn.Privmsg(transfer.url.UserName, req.String())
}

func (transfer *XdccTransfer) setupHandlers(channel string, userName string, slot int) {
	conn := transfer.conn

	// e.g. join channel on connect.
	conn.HandleFunc(irc.CONNECTED,
		func(conn *irc.Conn, line *irc.Line) {
			fmt.Println("connected ", channel)
			conn.Join(channel)
		})

	// send xdcc send on successfull join
	conn.HandleFunc(irc.JOIN,
		func(conn *irc.Conn, line *irc.Line) {
			if line.Args[0] == channel && !transfer.started {
				fmt.Println("contacting ", transfer.url.UserName)
				transfer.send(&XdccSendReq{Slot: slot})
			}
		})

	conn.HandleFunc(irc.CTCP,
		func(conn *irc.Conn, line *irc.Line) {
			fmt.Println(line.Text())
			res, err := parseCTCPRes(line.Text())
			if err != nil {
				fmt.Println(err.Error())
				os.Exit(1) // TODO: correct clean up
			}
			transfer.handleCTCPRes(res)
		})

	conn.HandleFunc(irc.DISCONNECTED,
		func(conn *irc.Conn, line *irc.Line) {
			if !transfer.started {
				transfer.notifyEvent(&TransferAbortedEvent{Error: "disconnected from server"})
			}
		})
}

func (transfer *XdccTransfer) PollEvents() chan XdccEvent {
	return transfer.events
}

type TransferProgessEvent struct {
	transferBytes uint64
	transferRate  float32
}

const downloadBufSize = 1024

type TransferStartedEvent struct {
	FileName string
	FileSize uint64
}

type TransferCompletedEvent struct{}

func (transfer *XdccTransfer) notifyEvent(e XdccEvent) {
	select {
	case transfer.events <- e:
	default:
		break
	}
}

type SpeedMonitorReader struct {
	reader       io.Reader
	elapsedTime  time.Duration
	currValue    uint64
	currentSpeed float64
	onUpdate     func(amount int, speed float64)
}

func NewSpeedMonitorReader(reader io.Reader, onUpdate func(int, float64)) *SpeedMonitorReader {
	return &SpeedMonitorReader{
		reader:       reader,
		elapsedTime:  time.Duration(0),
		currValue:    0,
		currentSpeed: 0,
		onUpdate:     onUpdate,
	}
}

func (monitor *SpeedMonitorReader) Read(buf []byte) (int, error) {
	now := time.Now()
	n, err := monitor.reader.Read(buf)
	elapsedTime := time.Since(now)
	monitor.currValue += uint64(n)
	monitor.elapsedTime += elapsedTime

	if monitor.elapsedTime > time.Second {
		monitor.currentSpeed = float64(monitor.currValue) / monitor.elapsedTime.Seconds()
		monitor.onUpdate(int(monitor.currValue), monitor.currentSpeed)
		monitor.currValue = 0
		monitor.elapsedTime = time.Duration(0)
	}
	return n, err
}

func (transfer *XdccTransfer) handleXdccSendRes(send *XdccSendRes) {
	go func() {
		conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP: send.IP, Port: send.Port})
		if err != nil {
			log.Fatalf("unable to reach host %s:%d", send.IP.String(), send.Port)
			return
		}

		file, err := os.OpenFile(transfer.filePath+"/"+send.FileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		fileWriter := bufio.NewWriter(file)

		if err != nil {
			log.Fatal(err.Error())
			return
		}

		transfer.notifyEvent(&TransferStartedEvent{
			FileName: send.FileName,
			FileSize: uint64(send.FileSize),
		})
		transfer.started = true

		reader := NewSpeedMonitorReader(conn, func(dowloadedAmount int, speed float64) {
			transfer.notifyEvent(&TransferProgessEvent{
				transferRate:  float32(speed),
				transferBytes: uint64(dowloadedAmount),
			})
		})

		// download loop
		downloadedBytesTotal := 0
		buf := make([]byte, downloadBufSize)
		for downloadedBytesTotal < send.FileSize {
			n, err := reader.Read(buf)

			if err != nil {
				log.Fatal(err.Error())
				return
			}

			if _, err := fileWriter.Write(buf[:n]); err != nil {
				log.Fatal(err.Error())
				return
			}

			downloadedBytesTotal += n
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
