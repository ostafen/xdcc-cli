package xdcc

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type IRCFile struct {
	Network  string
	Channel  string
	UserName string
	Slot     int
}

type IRCBot struct {
	Network string
	Channel string
	Name    string
}

const ircFileURLFields = 4

func parseSlot(slotStr string) (int, error) {
	if strings.HasPrefix(slotStr, "#") {
		strconv.Atoi(strings.TrimPrefix(slotStr, "#"))
	}
	return strconv.Atoi(slotStr)
}

var ErrInvalidURL = errors.New("invalid IRC url")

// url has the following format: irc://network/channel/bot/slot
func ParseURL(url string) (*IRCFile, error) {
	if !strings.HasPrefix(url, "irc://") {
		return nil, ErrInvalidURL
	}

	fields := strings.Split(strings.TrimPrefix(url, "irc://"), "/")
	if len(fields) != ircFileURLFields {
		return nil, ErrInvalidURL
	}

	slot, err := parseSlot(fields[3])
	if err != nil {
		return nil, err
	}

	fileUrl := &IRCFile{
		Network:  fields[0],
		Channel:  fields[1],
		UserName: fields[2],
		Slot:     slot,
	}

	if !strings.HasPrefix(fileUrl.Channel, "#") {
		fileUrl.Channel = "#" + fileUrl.Channel
	}
	return fileUrl, nil
}

func (url *IRCFile) GetBot() IRCBot {
	return IRCBot{Network: url.Network, Channel: url.Channel, Name: url.UserName}
}

func (url *IRCFile) String() string {
	return fmt.Sprintf("irc://%s/%s/%s/%d", url.Network, url.Channel, url.UserName, url.Slot)
}
