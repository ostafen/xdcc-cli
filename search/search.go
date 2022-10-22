package search

import (
	"errors"
	"strconv"
	"sync"
	"xdcc-cli/xdcc"
)

type XdccFileInfo struct {
	URL  xdcc.IRCFile
	Name string
	Size int64
	Slot int
}

type XdccSearchProvider interface {
	Search(keywords []string) ([]XdccFileInfo, error)
}

type ProviderAggregator struct {
	providerList []XdccSearchProvider
}

const MaxProviders = 100

func NewProviderAggregator(providers ...XdccSearchProvider) *ProviderAggregator {
	return &ProviderAggregator{
		providerList: providers,
	}
}

func (registry *ProviderAggregator) AddProvider(provider XdccSearchProvider) {
	registry.providerList = append(registry.providerList, provider)
}

const MaxResults = 1024

func (registry *ProviderAggregator) Search(keywords []string) ([]XdccFileInfo, error) {
	allResults := make(map[xdcc.IRCFile]XdccFileInfo)

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

const (
	KiloByte = 1024
	MegaByte = KiloByte * 1024
	GigaByte = MegaByte * 1024
)

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
