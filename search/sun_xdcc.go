package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	sunXdccURL             = "http://sunxdcc.com/deliver.php"
	sunXdccNumberOfEntries = 8
)

type SunXdccProvider struct{}

func (p *SunXdccProvider) parseResponseEntry(entry *SunXdccResponse, index int) (*XdccFileInfo, error) {
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

type SunXdccResponse struct {
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
	httpResp, err := http.Get(sunXdccURL + "?sterm=" + searchkey)
	if err != nil {
		return nil, err
	}

	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code error: %d %s", httpResp.StatusCode, httpResp.Status)
	}

	resp, err := p.parseResponse(httpResp)
	if err != nil {
		return nil, err
	}

	if !p.validateResult(resp) {
		return nil, fmt.Errorf("parse Error, not all fields have the same size")
	}
	return p.parseResults(resp)
}

func (p *SunXdccProvider) parseResults(resp *SunXdccResponse) ([]XdccFileInfo, error) {
	fileInfos := make([]XdccFileInfo, 0)
	for i := 0; i < len(resp.Botrec); i++ {
		info, err := p.parseResponseEntry(resp, i)
		if err == nil {
			fileInfos = append(fileInfos, *info)
		}
	}
	return fileInfos, nil
}

func (*SunXdccProvider) validateResult(entry *SunXdccResponse) bool {
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

func (*SunXdccProvider) parseResponse(res *http.Response) (*SunXdccResponse, error) {
	entry := &SunXdccResponse{}
	decoder := json.NewDecoder(res.Body)
	err := decoder.Decode(entry)
	return entry, err
}
