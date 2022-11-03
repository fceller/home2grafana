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
	"fmt"
	"github.com/fceller/home2grafana/devices"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"sort"
	"strconv"
	"text/template"
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
		ctx.Warn(err, "cannot parse file")
		return nil, err
	}

	return &overview, nil
}

type Table struct {
	Title   string
	Headers []string
	Rows    [][]string
}

func Max(x, y int) int {
	if x < y {
		return y
	}
	return x
}

func generateTable(overviewTable *OverviewTable, devs *devices.Devices) Table {
	table := Table{}
	table.Title = overviewTable.Title
	table.Headers = []string{"Device"}

	for _, group := range overviewTable.Group {
		table.Headers = append(table.Headers, group.Header)
	}

	for _, metric := range overviewTable.Metrics {
		table.Headers = append(table.Headers, metric.Header)
	}

	start := 1
	stop := start + len(overviewTable.Group)

	for k, d := range devs.ByDID {
		row := []string{k}
		use := false

		for _, r := range overviewTable.Metrics {
			found := false

			for _, w := range *d.Devices {
				if r.Name == w.MetricName() {
					if !use {
						for _, group := range overviewTable.Group {
							switch group.Name {
							case "room":
								row = append(row, w.Room())
							case "name":
								row = append(row, w.Name())
							default:
								row = append(row, "")
							}
						}
					}
					row = append(row, w.LastValue())
					use = true
					found = true
					break
				}
			}

			if !found {
				row = append(row, "-")
			}
		}

		if use {
			table.Rows = append(table.Rows, row)
		}
	}

	sort.SliceStable(table.Rows, func(i, j int) bool {
		for d := start; d < stop; d++ {
			if table.Rows[i][d] < table.Rows[j][d] {
				return true
			}
			if table.Rows[i][d] > table.Rows[j][d] {
				return false
			}
		}
		return false
	})

	return table
}

func GenerateOverview(writer io.Writer, overview *Overview, devs *devices.Devices) {
	for _, overviewTable := range overview.Tables {
		table := generateTable(&overviewTable, devs)
		widths := make([]int, len(table.Headers))

		for k, v := range table.Headers {
			widths[k] = len(v)
		}

		for _, w := range table.Rows {
			for k, v := range w {
				widths[k] = Max(widths[k], len(v))
			}
		}

		for k, v := range table.Headers {
			table.Headers[k] = fmt.Sprintf("%-"+strconv.Itoa(widths[k])+"s", v)
		}

		for _, w := range table.Rows {
			for k, v := range w {
				w[k] = fmt.Sprintf("%-"+strconv.Itoa(widths[k])+"s", v)
			}
		}

		text := template.New("Table")
		text = template.Must(text.Parse(`
-- {{ .Title }} --

{{ range .Headers }}{{ . }}  {{ end }}{{ range .Rows }}
{{ range . }}{{ . }}  {{ end }}{{ end }}
        `))
		text.Execute(writer, table)
	}
}
