package output

import (
	"io"
	"os"

	"github.com/olekukonko/tablewriter"
)

// Table provides table rendering utilities
type Table struct {
	table *tablewriter.Table
}

// NewTable creates a new table with default styling
func NewTable(headers []string) *Table {
	return NewTableWithWriter(os.Stdout, headers)
}

// NewTableWithWriter creates a new table with a custom writer
func NewTableWithWriter(w io.Writer, headers []string) *Table {
	table := tablewriter.NewWriter(w)
	table.SetHeader(headers)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)

	return &Table{table: table}
}

// AddRow adds a row to the table
func (t *Table) AddRow(row []string) {
	t.table.Append(row)
}

// AddRows adds multiple rows to the table
func (t *Table) AddRows(rows [][]string) {
	for _, row := range rows {
		t.table.Append(row)
	}
}

// Render outputs the table
func (t *Table) Render() {
	t.table.Render()
}

// SetColumnColors sets colors for specific columns
func (t *Table) SetColumnColors(colors ...tablewriter.Colors) {
	t.table.SetColumnColor(colors...)
}

// SetHeaderColors sets colors for header columns
func (t *Table) SetHeaderColors(colors ...tablewriter.Colors) {
	t.table.SetHeaderColor(colors...)
}
