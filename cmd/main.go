package main

import (
	"bufio"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"xdcc-cli/pb"
	"xdcc-cli/search"
	table "xdcc-cli/table"
	xdcc "xdcc-cli/xdcc"
)

var searchEngine *search.ProviderAggregator

func init() {
	searchEngine = search.NewProviderAggregator(
		&search.XdccEuProvider{},
		&search.SunXdccProvider{},
	)
}

var defaultColWidths []int = []int{100, 10, -1}

func FloatToString(value float64) string {
	if value-float64(int64(value)) > 0 {
		return strconv.FormatFloat(value, 'f', 2, 32)
	}
	return strconv.FormatFloat(value, 'f', 0, 32)
}

func formatSize(size int64) string {
	if size < 0 {
		return "--"
	}

	if size >= search.GigaByte {
		return FloatToString(float64(size)/float64(search.GigaByte)) + "GB"
	} else if size >= search.MegaByte {
		return FloatToString(float64(size)/float64(search.MegaByte)) + "MB"
	} else if size >= search.KiloByte {
		return FloatToString(float64(size)/float64(search.KiloByte)) + "KB"
	}
	return FloatToString(float64(size)) + "B"
}

func execSearch(args []string) {
	searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
	sortByFilename := searchCmd.Bool("s", false, "sort results by filename")

	args = parseFlags(searchCmd, args)

	printer := table.NewTablePrinter([]string{"File Name", "Size", "URL"})
	printer.SetMaxWidths(defaultColWidths)

	if len(args) < 1 {
		fmt.Println("search: no keyword provided.")
		os.Exit(1)
	}

	res, _ := searchEngine.Search(args)
	for _, fileInfo := range res {
		printer.AddRow(table.Row{fileInfo.Name, formatSize(fileInfo.Size), fileInfo.URL.String()})
	}

	sortColumn := 2
	if *sortByFilename {
		sortColumn = 0
	}
	printer.SortByColumn(sortColumn)

	printer.Print()
}

func transferLoop(transfer xdcc.Transfer) {
	bar := pb.NewProgressBar()

	evts := transfer.PollEvents()
	quit := false
	for !quit {
		e := <-evts
		switch evtType := e.(type) {
		case *xdcc.TransferStartedEvent:
			bar.SetTotal(int(evtType.FileSize))
			bar.SetFileName(evtType.FileName)
			bar.SetState(pb.ProgressStateDownloading)
		case *xdcc.TransferProgessEvent:
			bar.Increment(int(evtType.TransferBytes))
		case *xdcc.TransferCompletedEvent:
			bar.SetState(pb.ProgressStateCompleted)
			quit = true
		}
	}
	// TODO: do clean-up operations here
}

func suggestUnknownAuthoritySwitch(err error) {
	if err.Error() == (x509.UnknownAuthorityError{}.Error()) {
		fmt.Println("use the --allow-unknown-authority flag to skip certificate verification")
	}
}

func doTransfer(transfer xdcc.Transfer) {
	err := transfer.Start()
	if err != nil {
		fmt.Println(err)
		suggestUnknownAuthoritySwitch(err)
		return
	}

	transferLoop(transfer)
}

func parseFlags(flagSet *flag.FlagSet, args []string) []string {
	flagIdx := findFirstFlag(args)
	if flagIdx < 0 {
		return args
	}
	flagSet.Parse(args[flagIdx:])
	return args[:flagIdx]
}

func findFirstFlag(args []string) int {
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
			return i
		}
	}
	return -1
}

func loadUrlListFile(filePath string) []string {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	urlList := make([]string, 0)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		urlList = append(urlList, line)
	}

	if err := scanner.Err(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return urlList
}

func printGetUsageAndExit(flagSet *flag.FlagSet) {
	fmt.Printf("usage: get url1 url2 ... [-o path] [-i file] [--ssl-only]\n\nFlag set:\n")
	flagSet.PrintDefaults()
	os.Exit(0)
}

func execGet(args []string) {
	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	path := getCmd.String("o", ".", "output folder of dowloaded file")
	inputFile := getCmd.String("i", "", "input file containing a list of urls")

	sslOnly := getCmd.Bool("ssl-only", false, "force the client to use TSL connection")

	urlList := parseFlags(getCmd, args)

	if *inputFile != "" {
		urlList = append(urlList, loadUrlListFile(*inputFile)...)
	}

	if len(urlList) == 0 {
		printGetUsageAndExit(getCmd)
	}

	wg := sync.WaitGroup{}
	for _, urlStr := range urlList {
		url, err := xdcc.ParseURL(urlStr)
		if errors.Is(err, xdcc.ErrInvalidURL) {
			fmt.Printf("no valid irc url: %s\n", urlStr)
			continue
		}

		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		transfer := xdcc.NewTransfer(xdcc.Config{
			File:    *url,
			OutPath: *path,
			SSLOnly: *sslOnly,
		})

		wg.Add(1)
		go func(transfer xdcc.Transfer) {
			doTransfer(transfer)
			wg.Done()
		}(transfer)
	}
	wg.Wait()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("one of the following subcommands is expected: [search, get]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "search":
		execSearch(os.Args[2:])
	case "get":
		execGet(os.Args[2:])
	default:
		fmt.Println("no such command: ", os.Args[1])
		os.Exit(1)
	}
}
