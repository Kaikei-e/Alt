package output

import (
	"io"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
)

// Table provides table rendering utilities
type Table struct {
	table  *tablewriter.Table
	header []string
	rows   [][]string
	quiet  bool
}

// NewTable creates a new table with default styling
func NewTable(headers []string) *Table {
	return NewTableWithWriter(os.Stdout, headers)
}

// NewQuietTable creates a table that suppresses output when quiet is true
func NewQuietTable(headers []string, quiet bool) *Table {
	t := NewTableWithWriter(os.Stdout, headers)
	t.quiet = quiet
	return t
}

// NewTableWithWriter creates a new table with a custom writer
func NewTableWithWriter(w io.Writer, headers []string) *Table {
	table := tablewriter.NewTable(w,
		tablewriter.WithConfig(tablewriter.Config{
			Row: tw.CellConfig{
				Formatting: tw.CellFormatting{
					AutoWrap: tw.WrapNone,
				},
				Alignment: tw.CellAlignment{
					Global: tw.AlignLeft,
				},
			},
			Header: tw.CellConfig{
				Formatting: tw.CellFormatting{
					AutoFormat: tw.On,
				},
				Alignment: tw.CellAlignment{
					Global: tw.AlignLeft,
				},
			},
		}),
		tablewriter.WithRendition(tw.Rendition{
			Borders: tw.BorderNone,
			Settings: tw.Settings{
				Separators: tw.Separators{
					ShowHeader: tw.Off,
				},
			},
		}),
	)

	return &Table{table: table, header: headers}
}

// AddRow adds a row to the table
func (t *Table) AddRow(row []string) {
	t.rows = append(t.rows, row)
}

// AddRows adds multiple rows to the table
func (t *Table) AddRows(rows [][]string) {
	t.rows = append(t.rows, rows...)
}

// Render outputs the table
func (t *Table) Render() {
	if t.quiet {
		return
	}
	t.table.Header(t.header)
	t.table.Bulk(t.rows)
	t.table.Render()
}

// SetColumnColors sets colors for specific columns (no-op in v1)
func (t *Table) SetColumnColors(colors ...any) {
	// Colors are handled differently in v1
}

// SetHeaderColors sets colors for header columns (no-op in v1)
func (t *Table) SetHeaderColors(colors ...any) {
	// Colors are handled differently in v1
}
