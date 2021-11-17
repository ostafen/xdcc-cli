package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type IRCFileURL struct {
	Network  string
	Channel  string
	UserName string
	Slot     int
}

const ircFileURLFields = 4

func parseSlot(slotStr string) (int, error) {
	if !strings.HasPrefix(slotStr, "#") {
		return -1, errors.New("invalid slot")
	}
	return strconv.Atoi(strings.TrimPrefix(slotStr, "#"))
}

// url has the following format: irc://network/channel/bot/#slot
func parseIRCFileURl(url string) (*IRCFileURL, error) {
	if !strings.HasPrefix(url, "irc://") {
		return nil, errors.New("not an IRC url")
	}

	fields := strings.Split(strings.TrimPrefix(url, "irc://"), "/")
	if len(fields) != ircFileURLFields {
		return nil, errors.New("invalid IRC url")
	}

	slot, err := parseSlot(fields[3])
	if err != nil {
		return nil, err
	}

	return &IRCFileURL{
		Network:  fields[0],
		Channel:  fields[1],
		UserName: fields[2],
		Slot:     slot,
	}, nil
}

func (url *IRCFileURL) String() string {
	return fmt.Sprintf("irc://%s/%s/%s/#%d", url.Network, url.Channel, url.UserName, url.Slot)
}
