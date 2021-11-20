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
	Slot    int
}

type XdccSearchProvider interface {
	Search(fileName string) ([]XdccFileInfo, error)
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

func (registry *XdccProviderRegistry) Search(fileName string) ([]XdccFileInfo, error) {
	allResults := make([]XdccFileInfo, 0, MaxResults)

	wg := sync.WaitGroup{}
	wg.Add(len(registry.providerList))
	for _, p := range registry.providerList {
		go func(p XdccSearchProvider) {
			res, err := p.Search(fileName)

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

func (p *XdccEuProvider) parseFields(fields []string) (*XdccFileInfo, error) {
	if len(fields) != 7 {
		return nil, errors.New("unespected number of search entry fields")
	}

	fInfo := &XdccFileInfo{}
	fInfo.Network = fields[0]
	fInfo.Channel = fields[1]
	fInfo.BotName = fields[2]
	slot, err := strconv.Atoi(fields[3][1:])
	fInfo.Name = fields[6]

	if err != nil {
		return nil, err
	}

	fInfo.Slot = slot
	return fInfo, nil
}

func (p *XdccEuProvider) Search(fileName string) ([]XdccFileInfo, error) {
	searchkey := strings.Join(strings.Fields(fileName), "+")
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
