package thingprinter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type PrintableThing interface {
	AsTableRow(columns []string) []string
	AsJson(columns []string) (json.RawMessage, error)
}

func PrintTable[a PrintableThing](writer io.Writer, columns []string, things []a) error {
	columnWidths := make([]int, len(columns))
	for i, column := range columns {
		columnWidths[i] = len(column)
	}
	thingRows := make([][]string, 1+len(things))
	thingRows[0] = columns
	for i, thing := range things {
		r := thing.AsTableRow(columns)
		if lr, lc := len(r), len(columnWidths); lr > lc {
			r = r[:lc]
		} else if lr < lc {
			r2 := make([]string, len(columnWidths))
			copy(r2, r)
			r = r2
		}
		for ci, cv := range r {
			columnWidths[ci] = max(columnWidths[ci], len(cv))
		}
		thingRows[1+i] = r
	}
	for _, row := range thingRows {
		sb := new(bytes.Buffer)
		for i, column := range row {
			sb.WriteString(column)
			sb.WriteString(strings.Repeat(" ", columnWidths[i]-len(column)+1))
		}
		sb.WriteRune('\n')
		if _, err := writer.Write(sb.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func PrintJson[a PrintableThing](writer io.Writer, columns []string, things []a) error {
	output := make([]json.RawMessage, len(things))
	for ri, thing := range things {
		rm, err := thing.AsJson(columns)
		if err != nil {
			return err
		}
		output[ri] = rm
	}
	enc := json.NewEncoder(writer)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

type PrintableMap map[string]interface{}

func (p PrintableMap) AsTableRow(columns []string) []string {
	out := make([]string, len(columns))
	for i, column := range columns {
		out[i] = fmt.Sprintf("%v", p[column])
	}
	return out
}

func (p PrintableMap) AsJson(columns []string) (json.RawMessage, error) {
	if columns == nil {
		return json.Marshal(p)
	}
	out := make(map[string]interface{}, len(columns))
	for _, column := range columns {
		out[column] = p[column]
	}
	return json.Marshal(p)
}
