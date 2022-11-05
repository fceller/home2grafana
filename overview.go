/*
 * MIT License
 *
 * Copyright (c) 2022 Frank Celler
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 *
 */

package main

import (
	"github.com/fceller/home2grafana/devices"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"html/template"
	"io"
	"io/ioutil"
	"sort"
)

type OverviewTable struct {
	Title   string `yaml:"title"`
	Metrics []struct {
		Name   string `yaml:"name"`
		Header string `yaml:"header"`
	} `yaml:"metrics"`
	Group []struct {
		Name   string `yaml:"name,omitempty"`
		Header string `yaml:"header,omitempty"`
	} `yaml:"group"`
}

type Overview struct {
	Tables []OverviewTable `yaml:"tables"`
	html   *template.Template
}

func LoadOverviewDesc(setup string) (*Overview, error) {
	ctx := devices.Context{
		Root: setup,
		Clog: logrus.WithField("task", "load overview description"),
	}

	ctx.PushField("root", ctx.Root)
	defer ctx.Pop()
	ctx.Info("loading overview file")

	yfile, err := ioutil.ReadFile(setup + "/overview.yaml")

	if err != nil {
		ctx.Warn(err, "cannot read file")
		return nil, err
	}

	overview := Overview{}
	err = yaml.Unmarshal(yfile, &overview)

	if err != nil {
		ctx.Warn(err, "cannot parse file 'overview.yaml'")
		return nil, err
	}

	overview.html, err = template.ParseFiles(setup + "/overview.html")

	if err != nil {
		ctx.Warn(err, "cannot parse file 'overview.html'")
		return nil, err
	}

	return &overview, nil
}

type TableEntry struct {
	Entry   string
	IsRight bool
}

type Table struct {
	Title   string
	Headers []TableEntry
	Rows    [][]TableEntry
}

func Max(x, y int) int {
	if x < y {
		return y
	}
	return x
}

func generateTable(overviewTable *OverviewTable, devs *devices.Devices, details bool) Table {
	table := Table{}
	table.Title = overviewTable.Title

	if details {
		table.Headers = []TableEntry{TableEntry{"Device", false}}
	} else {
		table.Headers = []TableEntry{}
	}

	for _, group := range overviewTable.Group {
		table.Headers = append(table.Headers, TableEntry{group.Header, false})
	}

	for _, metric := range overviewTable.Metrics {
		table.Headers = append(table.Headers, TableEntry{metric.Header, true})
	}

	start := 0

	if details {
		start = 1
	}

	stop := start + len(overviewTable.Group)

	for k, d := range devs.ByDID {
		row := []TableEntry{}
		use := false

		if details {
			row = append(row, TableEntry{k, false})
		}

		for _, r := range overviewTable.Metrics {
			found := false

			for _, w := range *d.Devices {
				if r.Name == w.MetricName() {
					if !use {
						for _, group := range overviewTable.Group {
							switch group.Name {
							case "room":
								row = append(row, TableEntry{w.Room(), false})
							case "name":
								row = append(row, TableEntry{w.Name(), false})
							default:
								row = append(row, TableEntry{"", false})
							}
						}
					}
					row = append(row, TableEntry{w.LastValue(), true})
					found = true
					use = true
					break
				}
			}

			if !found {
				row = append(row, TableEntry{"-", true})
			}
		}
		if use {
			table.Rows = append(table.Rows, row)
		}
	}

	sort.SliceStable(table.Rows, func(i, j int) bool {
		for d := start; d < stop; d++ {
			if table.Rows[i][d].Entry < table.Rows[j][d].Entry {
				return true
			}
			if table.Rows[i][d].Entry > table.Rows[j][d].Entry {
				return false
			}
		}
		return false
	})

	return table
}

func GenerateOverview(writer io.Writer, overview *Overview, devs *devices.Devices, details bool) {
	tables := make([]Table, 0)

	for _, overviewTable := range overview.Tables {
		table := generateTable(&overviewTable, devs, details)
		widths := make([]int, len(table.Headers))

		for k, v := range table.Headers {
			widths[k] = len(v.Entry)
		}

		for _, w := range table.Rows {
			for k, v := range w {
				widths[k] = Max(widths[k], len(v.Entry))
			}
		}

		/*
			for k, v := range table.Headers {
				table.Headers[k] = fmt.Sprintf("%-"+strconv.Itoa(widths[k])+"s", v)
			}

			for _, w := range table.Rows {
				for k, v := range w {
					w[k] = fmt.Sprintf("%-"+strconv.Itoa(widths[k])+"s", v)
				}
			}
		*/

		tables = append(tables, table)
	}

	overview.html.Execute(writer, tables)
}
