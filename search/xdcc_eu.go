package search

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"xdcc-cli/xdcc"

	"github.com/PuerkitoBio/goquery"
)

type XdccEuProvider struct{}

const (
	xdccEuURL             = "https://www.xdcc.eu/search.php"
	xdccEuNumberOfEntries = 7
)

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
	res, err := http.Get(xdccEuURL + "?searchkey=" + searchkey)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
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
			url, err := xdcc.ParseURL(urlStr + "/" + info.URL.UserName + "/" + strconv.Itoa(info.Slot))
			if err == nil {
				info.URL = *url
				fileInfos = append(fileInfos, *info)
			}
		}
	})
	return fileInfos, nil
}
