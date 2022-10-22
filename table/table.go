package table

import (
	"fmt"
	"sort"
	"strings"
	"xdcc-cli/util"
)

type Row []string

type TablePrinter struct {
	Headers   []string
	Rows      []Row
	MaxRows   int
	MaxWidths []int
}

func NewTablePrinter(headers []string) *TablePrinter {
	return &TablePrinter{
		Headers: headers,
		MaxRows: -1,
	}
}

func (printer *TablePrinter) SetMaxWidths(widths []int) {
	printer.MaxWidths = widths
}

func centerString(s string, width int) string {
	padSpace := width - len(s)
	leftPadding := padSpace / 2
	rightPadding := padSpace - leftPadding
	return strings.Repeat(" ", leftPadding) + s + strings.Repeat(" ", rightPadding)
}

func formatStr(s string, maxSize int) string {
	return centerString(util.CutStr(s, maxSize), maxSize)
}

const paddingDefault = 2

func (printer *TablePrinter) NumRows() int {
	if printer.Rows == nil {
		return 0
	}
	return len(printer.Rows)
}

func (printer *TablePrinter) NumCols() int {
	return len(printer.Headers)
}

func (printer *TablePrinter) hasRows() bool {
	return printer.Rows != nil && len(printer.Rows) > 0
}

func (printer *TablePrinter) computeColumnWidthds() []int {
	widths := make([]int, printer.NumCols())
	for i := 0; i < printer.NumCols(); i++ {
		widths[i] = len(printer.Headers[i])
	}

	if printer.hasRows() {
		for col := 0; col < printer.NumCols(); col++ {
			for row := 0; row < len(printer.Rows); row++ {
				if len(printer.Rows[row][col]) > widths[col] {
					widths[col] = len(printer.Rows[row][col])
				}
			}
		}
	}

	for i := 0; i < printer.NumCols(); i++ {
		widths[i] += paddingDefault

		if printer.MaxWidths != nil {
			if printer.MaxWidths[i] > 0 && widths[i] > printer.MaxWidths[i] {
				widths[i] = printer.MaxWidths[i]
			}
		}

	}
	return widths
}

func (printer *TablePrinter) renderRow(s []string, colWidths []int) string {
	content := "|"
	for i := 0; i < len(printer.Headers)-1; i++ {
		content += formatStr(s[i], colWidths[i]) + "|"
	}
	content += formatStr(s[printer.NumCols()-1], colWidths[printer.NumCols()-1]) + "|"
	return content
}

func (printer *TablePrinter) renderLine(colWidths []int) string {
	content := "+"
	for i := 0; i < len(printer.Headers)-1; i++ {
		content += formatStr(strings.Repeat("-", colWidths[i]), colWidths[i]) + "-"
	}
	content += formatStr(strings.Repeat("-", colWidths[printer.NumCols()-1]), colWidths[printer.NumCols()-1]) + "+"
	return content
}

func (printer *TablePrinter) renderHeader(colWidths []int) {
	fmt.Println(printer.renderLine(colWidths))
	fmt.Println(printer.renderRow(printer.Headers, colWidths))
	fmt.Println(printer.renderLine(colWidths))
}

const initialRows = 100

func (printer *TablePrinter) AddRow(r Row) {
	if printer.Rows == nil {
		printer.Rows = make([]Row, 0, initialRows)
	}

	if printer.MaxRows > 0 && printer.NumRows() == printer.MaxRows {
		return
	}
	printer.Rows = append(printer.Rows, r)

}

func (printer *TablePrinter) SortByColumn(col int) {
	if col < printer.NumCols() {
		sort.Slice(printer.Rows, func(i, j int) bool {
			return printer.Rows[i][col] < printer.Rows[j][col]
		})
	}
}

func (printer *TablePrinter) Print() {
	colWidths := printer.computeColumnWidthds()

	printer.renderHeader(colWidths)
	if printer.hasRows() {
		for _, row := range printer.Rows {
			fmt.Println(printer.renderRow(row, colWidths))
		}
		fmt.Println(printer.renderLine(colWidths))
	}
}
