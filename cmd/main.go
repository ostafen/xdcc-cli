package main

import (
	"bufio"
	"crypto/x509"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	xdcc "xdcc-cli"
	"xdcc-cli/search"
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

func searchCommand(args []string) {
	searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
	sortByFilename := searchCmd.Bool("s", false, "sort results by filename")

	args = parseFlags(searchCmd, args)

	printer := xdcc.NewTablePrinter([]string{"File Name", "Size", "URL"})
	printer.SetMaxWidths(defaultColWidths)

	if len(args) < 1 {
		fmt.Println("search: no keyword provided.")
		os.Exit(1)
	}

	res, _ := searchEngine.Search(args)
	for _, fileInfo := range res {
		printer.AddRow(xdcc.Row{fileInfo.Name, formatSize(fileInfo.Size), fileInfo.URL.String()})
	}

	sortColumn := 2
	if *sortByFilename {
		sortColumn = 0
	}
	printer.SortByColumn(sortColumn)

	printer.Print()
}

func transferLoop(transfer *xdcc.XdccTransfer) {
	pb := xdcc.NewProgressBar()

	evts := transfer.PollEvents()
	quit := false
	for !quit {
		e := <-evts
		switch evtType := e.(type) {
		case *xdcc.TransferStartedEvent:
			pb.SetTotal(int(evtType.FileSize))
			pb.SetFileName(evtType.FileName)
			pb.SetState(xdcc.ProgressStateDownloading)
		case *xdcc.TransferProgessEvent:
			pb.Increment(int(evtType.TransferBytes))
		case *xdcc.TransferCompletedEvent:
			pb.SetState(xdcc.ProgressStateCompleted)
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

func doTransfer(transfer *xdcc.XdccTransfer) {
	err := transfer.Start()
	if err != nil {
		fmt.Println(err)
		suggestUnknownAuthoritySwitch(err)
		return
	}

	transferLoop(transfer)
}

func parseFlags(flagSet *flag.FlagSet, args []string) []string {
	findFirstFlag := func(args []string) int {
		for i, arg := range args {
			if strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
				return i
			}
		}
		return -1
	}
	flagIdx := findFirstFlag(args)
	if flagIdx >= 0 {
		flagSet.Parse(args[flagIdx:])
		return args[:flagIdx]
	}
	return args
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
	fmt.Printf("usage: get url1 url2 ... [-o path] [-i file] [--allow-unknown-authority]\n\nFlag set:\n")
	flagSet.PrintDefaults()
	os.Exit(0)
}

func getCommand(args []string) {
	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	path := getCmd.String("o", ".", "output folder of dowloaded file")
	inputFile := getCmd.String("i", "", "input file containing a list of urls")
	skipCertificateCheck := getCmd.Bool("allow-unknown-authority", false, "skip x509 certificate check during tls connection")
	noSSL := getCmd.Bool("no-ssl", false, "disable SSL.")

	urlList := parseFlags(getCmd, args)

	if *inputFile != "" {
		urlList = append(urlList, loadUrlListFile(*inputFile)...)
	}

	if len(urlList) == 0 {
		printGetUsageAndExit(getCmd)
	}

	wg := sync.WaitGroup{}
	for _, urlStr := range urlList {
		if strings.HasPrefix(urlStr, "irc://") {
			url, err := xdcc.ParseURL(urlStr)

			if err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}

			wg.Add(1)
			transfer := xdcc.NewTransfer(*url, *path, !*noSSL, *skipCertificateCheck)
			go func(transfer *xdcc.XdccTransfer) {
				doTransfer(transfer)
				wg.Done()
			}(transfer)
		} else {
			fmt.Printf("no valid irc url %s\n", urlStr)
		}
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
		searchCommand(os.Args[2:])
	case "get":
		getCommand(os.Args[2:])
	default:
		fmt.Println("no such command: ", os.Args[1])
		os.Exit(1)
	}
}
