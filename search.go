package main

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

type XdccFileInfo struct {
	Network string
	Channel string
	BotName string
	Name    string
	Url     string
	Size    int64
	Slot    int
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
	allResults := make([]XdccFileInfo, 0, MaxResults)

	wg := sync.WaitGroup{}
	wg.Add(len(registry.providerList))
	for _, p := range registry.providerList {
		go func(p XdccSearchProvider) {
			res, err := p.Search(keywords)

			if err != nil {
				return
			}

			allResults = append(allResults, res...)

			wg.Done()
		}(p)
	}
	wg.Wait()
	return allResults, nil
}

type XdccEuProvider struct{}

const XdccEuURL = "https://www.xdcc.eu/search.php"

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

const xdccEuNumberOfEntries = 7

func (p *XdccEuProvider) parseFields(fields []string) (*XdccFileInfo, error) {
	if len(fields) != xdccEuNumberOfEntries {
		return nil, errors.New("unespected number of search entry fields")
	}

	fInfo := &XdccFileInfo{}
	fInfo.Network = fields[0]
	fInfo.Channel = fields[1]
	fInfo.BotName = fields[2]
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
	if res.StatusCode != 200 {
		log.Fatalf("status code error: %d %s", res.StatusCode, res.Status)
		return nil, err
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

		var url string
		s.Children().Each(func(i int, si *goquery.Selection) {
			if i == 1 {
				value, exists := si.Find("a").First().Attr("href")
				if exists {
					url = value
				}
			}
			fields = append(fields, strings.TrimSpace(si.Text()))
		})

		info, err := p.parseFields(fields)
		if err == nil {
			info.Url = url + "/" + info.BotName + "/" + strconv.Itoa(info.Slot)
			fileInfos = append(fileInfos, *info)
		}
	})
	return fileInfos, nil
}
