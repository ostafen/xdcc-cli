package main

import (
	"flag"
	"fmt"
	"os"
)

var registry *XdccProviderRegistry = nil

func init() {
	registry = NewProviderRegistry()
	registry.AddProvider(&XdccEuProvider{})
}

func search(fileName string) {
	printer := NewTablePrinter([]string{"File Name", "Network", "Channel"})

	res, _ := registry.Search(fileName)
	for _, fileInfo := range res {
		printer.AddRow(Row{fileInfo.Name, fileInfo.Network, fileInfo.Channel})
	}

	printer.Print()
}

func main() {
	searchCmd := flag.NewFlagSet("foo", flag.ExitOnError)
	fileName := searchCmd.String("f", "", "name of the file to search")

	if len(os.Args) < 2 {
		fmt.Println("one of the following subcommands is expected: [search, get]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "search":
		searchCmd.Parse(os.Args[2:])
		search(*fileName)
	case "get":
		break
	default:
		fmt.Println("no such command: ", os.Args[1])
		os.Exit(1)
	}
}
