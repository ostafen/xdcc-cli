package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
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

type SunXdccProvider struct{}

const SunXdccURL = "http://sunxdcc.com/deliver.php"

const SunXdccNumberOfEntries = 8

func (p *SunXdccProvider) parseFields(fields []string) (*XdccFileInfo, error) {
	if len(fields) != SunXdccNumberOfEntries {
		return nil, errors.New("unexpected number of search entry fields")
	}

	fInfo := &XdccFileInfo{}
	fInfo.Network = fields[1]
	fInfo.BotName = fields[2]
	fInfo.Channel = fields[3]

	slot, err := strconv.Atoi(fields[4][1:])

	if err != nil {
		return nil, err
	}

	sizeString := strings.TrimLeft(strings.TrimRight(fields[6], "]"), "[")

	fInfo.Size, _ = parseFileSize(sizeString) // ignoring error

	fInfo.Name = fields[7]

	if err != nil {
		return nil, err
	}

	fInfo.Slot = slot

	fInfo.Url = "irc://" + fInfo.Network + "/" + strings.TrimLeft(fInfo.Channel, "#") + "/" + fInfo.BotName + "/" + strconv.Itoa(fInfo.Slot)

	return fInfo, nil
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
	// for API definition use https://sunxdcc.com/#api
	res, err := http.Get(SunXdccURL + "?sterm=" + searchkey)

	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode != 200 {
		log.Fatalf("status code error: %d %s", res.StatusCode, res.Status)
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	// Load the JSON Response document

	var entry SunXdccEntry
	json.Unmarshal([]byte(body), &entry)

	var sizes = [8]int{
		len(entry.Botrec),
		len(entry.Network),
		len(entry.Bot),
		len(entry.Channel),
		len(entry.Packnum),
		len(entry.Gets),
		len(entry.Fsize),
		len(entry.Fname)}

	var length = sizes[0]
	for _, l := range sizes {
		if length != l {
			log.Fatalf("Parse Error, not all fields have the same size")
			return nil, errors.New("Parse Error, not all fields have the same size")
		}
	}

	fileInfos := make([]XdccFileInfo, 0)

	type XdccFileInfo struct {
		Network string
		Channel string
		BotName string
		Name    string
		Url     string
		Size    int64
		Slot    int
	}

	for i := 0; i < len(entry.Botrec); i++ {

		var fields = []string{
			entry.Botrec[i],
			entry.Network[i],
			entry.Bot[i],
			entry.Channel[i],
			entry.Packnum[i],
			entry.Gets[i],
			entry.Fsize[i],
			entry.Fname[i]}

		info, err := p.parseFields(fields)
		if err == nil {
			fileInfos = append(fileInfos, *info)
		}
	}

	return fileInfos, nil
}
