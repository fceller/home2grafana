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
	"container/heap"
	"flag"
	"fmt"
	"github.com/sirupsen/logrus"
	"log"
	"math"
	"math/rand"
	"os"
	"time"

	"net/http"

	"github.com/fceller/home2grafana/devices"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type DeviceItem struct {
	methods  devices.DeviceInterface
	last     float64
	lastRate float64
	time     int64
	timeRate int64
	expiry   int64
	index    int
}

type PriorityQueue []*DeviceItem

func (pq PriorityQueue) Len() int {
	return len(pq)
}

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].expiry < pq[j].expiry
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*DeviceItem)
	item.index = n
	*pq = append(*pq, item)
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

var PrometheusGauges = make(map[string]*prometheus.GaugeVec)
var PrometheusCounters = make(map[string]*prometheus.CounterVec)

const totalSuffix = "total"
const rateSuffix = "rate"

var prometheusRegistry *prometheus.Registry

func setPrometheusValue(ctx devices.Context, d *DeviceItem) uint64 {
	category := d.methods.CategoryName()
	name := d.methods.MetricName()
	value, err := d.methods.CurrentValue(ctx)

	if err == nil {
		ctx.PushField(category, value)
		ctx.Info(fmt.Sprintf("read %s", category))
		ctx.Pop()

		PrometheusGauges[name].WithLabelValues(d.methods.Labels()...).Set(value)

		if category == "energy" {
			counter := fmt.Sprintf("%s_%s", name, totalSuffix)
			now := time.Now().UnixMilli()

			if !math.IsNaN(d.last) {
				if value >= d.last {
					PrometheusCounters[counter].WithLabelValues(d.methods.Labels()...).Add(value - d.last)
				} else {
					prometheusRegistry.Unregister(PrometheusCounters[counter])
					PrometheusCounters[counter] = prometheus.NewCounterVec(prometheus.CounterOpts{
						Name: counter,
						Help: name,
					}, []string{"provider", "name", "room"})
					prometheusRegistry.MustRegister(PrometheusCounters[counter])
				}
			}

			d.last = value
			d.time = now

			if !math.IsNaN(d.lastRate) {
				if value > d.lastRate && now > d.timeRate {
					avg := fmt.Sprintf("%s_%s", name, rateSuffix)
					computed := (value - d.lastRate) / float64(now-d.timeRate) * 1000
					PrometheusGauges[avg].WithLabelValues(d.methods.Labels()...).Set(computed)
					d.lastRate = value
					d.timeRate = now
				}
			} else {
				d.lastRate = value
				d.timeRate = now
			}
		}

		return 1
	} else {
		ctx.Warn(err, fmt.Sprintf("cannot read %s total", category))
		return 5
	}
}

func readData(deviceList []devices.DeviceInterface) {
	if len(deviceList) == 0 {
		return
	}

	ctx := devices.Context{
		NetClient: &http.Client{Timeout: time.Second * 10},
		Clog:      logrus.WithField("task", "read metrics"),
	}

	deviceHeap := make(PriorityQueue, len(deviceList))
	now := time.Now().UnixMilli()

	for i, d := range deviceList {
		deviceHeap[i] = &DeviceItem{
			methods:  d,
			last:     math.NaN(),
			lastRate: math.NaN(),
			time:     now,
			timeRate: now,
			expiry:   now/1000 + (500+rand.Int63n(501))*int64(d.IntervalSec())/1000,
			index:    i,
		}
	}

	heap.Init(&deviceHeap)

	for deviceHeap.Len() > 0 {
		now = time.Now().Unix()

		item := heap.Pop(&deviceHeap).(*DeviceItem)
		sleep := int64(1)

		if now < item.expiry {
			sleep = item.expiry - now
		}

		time.Sleep(time.Second * time.Duration(sleep))

		ctx.PushField("device", item.methods.LogName())
		defer ctx.Pop()

		factor := setPrometheusValue(ctx, item)
		item.expiry = now + int64(item.methods.IntervalSec()*factor)

		heap.Push(&deviceHeap, item)
	}
}

func main() {
	bind := ""
	setup := ""
	enableH2c := false

	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagset.StringVar(&bind, "bind", ":9876", "The socket to bind to.")
	flagset.StringVar(&setup, "setup", "./setup", "The directory holding the device definitions.")
	flagset.BoolVar(&enableH2c, "h2c", false, "Enable h2c (http/2 over tcp) protocol.")
	flagset.Parse(os.Args[1:])

	deviceList := devices.LoadDevices(setup)

	if len(deviceList) == 0 {
		logrus.Panic("no devices have been defined, exiting...")
	}

	prometheusRegistry := prometheus.NewRegistry()

	for _, d := range deviceList {
		name := d.MetricName()

		if name != "" {
			cat := d.CategoryName()

			if _, prs := PrometheusGauges[name]; !prs {
				PrometheusGauges[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name: name,
					Help: name,
				}, []string{"provider", "name", "room"})

				prometheusRegistry.MustRegister(PrometheusGauges[name])
			}

			if cat == "energy" {
				counter := fmt.Sprintf("%s_%s", name, totalSuffix)

				if _, prs := PrometheusCounters[counter]; !prs {
					PrometheusCounters[counter] = prometheus.NewCounterVec(prometheus.CounterOpts{
						Name: counter,
						Help: name,
					}, []string{"provider", "name", "room"})

					prometheusRegistry.MustRegister(PrometheusCounters[counter])
				}
			}

			if cat == "energy" {
				avg := fmt.Sprintf("%s_%s", name, rateSuffix)

				if _, prs := PrometheusGauges[avg]; !prs {
					PrometheusGauges[avg] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
						Name: avg,
						Help: name,
					}, []string{"provider", "name", "room"})

					prometheusRegistry.MustRegister(PrometheusGauges[avg])
				}
			}
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(prometheusRegistry, promhttp.HandlerOpts{}))

	go readData(deviceList)

	var srv *http.Server
	if enableH2c {
		srv = &http.Server{Addr: bind, Handler: h2c.NewHandler(mux, &http2.Server{})}
	} else {
		srv = &http.Server{Addr: bind, Handler: mux}
	}

	logrus.Infof("start listing on %s", bind)
	log.Fatal(srv.ListenAndServe())
}
