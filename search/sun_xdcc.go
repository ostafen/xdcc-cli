package search

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

const (
	SunXdccURL             = "http://sunxdcc.com/deliver.php"
	SunXdccNumberOfEntries = 8
)

type SunXdccProvider struct{}

func (p *SunXdccProvider) parseFields(entry *SunXdccEntry, index int) (*XdccFileInfo, error) {
	info := &XdccFileInfo{}
	info.URL.Network = entry.Network[index]
	info.URL.UserName = entry.Bot[index]
	info.URL.Channel = entry.Channel[index]

	slot, err := strconv.Atoi(entry.Packnum[index][1:])

	if err != nil {
		return nil, err
	}

	sizeString := strings.TrimLeft(strings.TrimRight(entry.Fsize[index], "]"), "[")

	info.Size, _ = parseFileSize(sizeString) // ignoring error
	info.Name = entry.Fname[index]
	if err != nil {
		return nil, err
	}

	info.Slot = slot
	return info, nil
}

type SunXdccEntry struct {
	Botrec  []string
	Network []string
	Bot     []string
	Channel []string
	Packnum []string
	Gets    []string
	Fsize   []string
	Fname   []string
}

func (p *SunXdccProvider) Search(keywords []string) ([]XdccFileInfo, error) {
	keywordString := strings.Join(keywords, " ")
	searchkey := strings.Join(strings.Fields(keywordString), "+")
	// see https://sunxdcc.com/#api for API definition
	res, err := http.Get(SunXdccURL + "?sterm=" + searchkey)

	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	entry, err := p.parseResponse(res)
	if err != nil {
		return nil, err
	}

	if !p.validateResult(entry) {
		return nil, fmt.Errorf("parse Error, not all fields have the same size")
	}

	fileInfos := make([]XdccFileInfo, 0)
	for i := 0; i < len(entry.Botrec); i++ {
		info, err := p.parseFields(entry, i)
		if err == nil {
			fileInfos = append(fileInfos, *info)
		}
	}
	return fileInfos, nil
}

func (*SunXdccProvider) validateResult(entry *SunXdccEntry) bool {
	sizes := [8]int{
		len(entry.Botrec),
		len(entry.Network),
		len(entry.Bot),
		len(entry.Channel),
		len(entry.Packnum),
		len(entry.Gets),
		len(entry.Fsize),
		len(entry.Fname),
	}

	length := sizes[0]
	for _, l := range sizes {
		if length != l {
			return false
		}
	}
	return true
}

func (*SunXdccProvider) parseResponse(res *http.Response) (*SunXdccEntry, error) {
	entry := &SunXdccEntry{}
	decoder := json.NewDecoder(res.Body)
	err := decoder.Decode(entry)
	return entry, err
}
