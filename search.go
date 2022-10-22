package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

type XdccFileInfo struct {
	URL  IRCFile
	Name string
	Size int64
	Slot int
}

type XdccSearchProvider interface {
	Search(keywords []string) ([]XdccFileInfo, error)
}

type XdccProviderRegistry struct {
	providerList []XdccSearchProvider
}

const MaxProviders = 100

func NewProviderRegistry() *XdccProviderRegistry {
	return &XdccProviderRegistry{
		providerList: make([]XdccSearchProvider, 0, MaxProviders),
	}
}

func (registry *XdccProviderRegistry) AddProvider(provider XdccSearchProvider) {
	registry.providerList = append(registry.providerList, provider)
}

const MaxResults = 1024

func (registry *XdccProviderRegistry) Search(keywords []string) ([]XdccFileInfo, error) {
	allResults := make(map[IRCFile]XdccFileInfo)

	mtx := sync.Mutex{}

	wg := sync.WaitGroup{}
	wg.Add(len(registry.providerList))
	for _, p := range registry.providerList {
		go func(p XdccSearchProvider) {
			resList, err := p.Search(keywords)
			if err != nil {
				return
			}

			mtx.Lock()
			for _, res := range resList {
				allResults[res.URL] = res
			}
			mtx.Unlock()

			wg.Done()
		}(p)
	}
	wg.Wait()

	results := make([]XdccFileInfo, 0, MaxResults)
	for _, res := range allResults {
		results = append(results, res)
	}
	return results, nil
}

func parseFileSize(sizeStr string) (int64, error) {
	if len(sizeStr) == 0 {
		return -1, errors.New("empty string")
	}
	lastChar := sizeStr[len(sizeStr)-1]
	sizePart := sizeStr[:len(sizeStr)-1]

	size, err := strconv.ParseFloat(sizePart, 32)

	if err != nil {
		return -1, err
	}
	switch lastChar {
	case 'G':
		return int64(size * GigaByte), nil
	case 'M':
		return int64(size * MegaByte), nil
	case 'K':
		return int64(size * KiloByte), nil
	}
	return -1, errors.New("unable to parse: " + sizeStr)
}

type XdccEuProvider struct{}

const XdccEuURL = "https://www.xdcc.eu/search.php"

const xdccEuNumberOfEntries = 7

func (p *XdccEuProvider) parseFields(fields []string) (*XdccFileInfo, error) {
	if len(fields) != xdccEuNumberOfEntries {
		return nil, errors.New("unexpected number of search entry fields")
	}

	fInfo := &XdccFileInfo{}
	fInfo.URL.Network = fields[0]
	fInfo.URL.Channel = fields[1]
	fInfo.URL.UserName = fields[2]
	slot, err := strconv.Atoi(fields[3][1:])
	if err != nil {
		return nil, err
	}

	fInfo.Size, _ = parseFileSize(fields[5]) // ignoring error

	fInfo.Name = fields[6]

	if err != nil {
		return nil, err
	}

	fInfo.Slot = slot
	return fInfo, nil
}

func (p *XdccEuProvider) Search(keywords []string) ([]XdccFileInfo, error) {
	keywordString := strings.Join(keywords, " ")
	searchkey := strings.Join(strings.Fields(keywordString), "+")
	res, err := http.Get(XdccEuURL + "?searchkey=" + searchkey)

	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	fileInfos := make([]XdccFileInfo, 0)
	doc.Find("tr").Each(func(_ int, s *goquery.Selection) {
		fields := make([]string, 0)

		var urlStr string
		s.Children().Each(func(i int, si *goquery.Selection) {
			if i == 1 {
				value, exists := si.Find("a").First().Attr("href")
				if exists {
					urlStr = value
				}
			}
			fields = append(fields, strings.TrimSpace(si.Text()))
		})

		info, err := p.parseFields(fields)
		if err == nil {
			url, err := parseURL(urlStr + "/" + info.URL.UserName + "/" + strconv.Itoa(info.Slot))
			if err == nil {
				info.URL = *url
				fileInfos = append(fileInfos, *info)
			}
		}
	})
	return fileInfos, nil
}

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
		return nil, fmt.Errorf("Parse Error, not all fields have the same size")
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
