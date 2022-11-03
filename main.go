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
	"sync"
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

var prometheusGauges = make(map[string]*prometheus.GaugeVec)
var prometheusCounters = make(map[string]*prometheus.CounterVec)
var registry *prometheus.Registry

const totalSuffix = "total"
const rateSuffix = "rate"

var SyncPoint sync.Mutex

func setPrometheusValue(ctx devices.Context, d *DeviceItem) uint64 {
	SyncPoint.Lock()
	defer SyncPoint.Unlock()

	category := d.methods.CategoryName()
	name := d.methods.MetricName()
	value, err := d.methods.CurrentValue(ctx)

	if err == nil {
		ctx.PushField(category, value)
		ctx.Info(fmt.Sprintf("read %s: %f", category, value))
		ctx.Pop()

		prometheusGauges[name].WithLabelValues(d.methods.Labels()...).Set(value)
		//d.methods.lastStr = fmt.Sprintf("%.2f", value)

		if category == "energy" {
			counter := fmt.Sprintf("%s_%s", name, totalSuffix)
			now := time.Now().UnixMilli()

			if !math.IsNaN(d.last) {
				if value >= d.last {
					prometheusCounters[counter].WithLabelValues(d.methods.Labels()...).Add(value - d.last)
				} else {
					registry.Unregister(prometheusCounters[counter])
					prometheusCounters[counter] = prometheus.NewCounterVec(prometheus.CounterOpts{
						Name: counter,
						Help: name,
					}, []string{"provider", "name", "room"})
				}
			}

			d.last = value
			d.time = now

			if !math.IsNaN(d.lastRate) {
				if value > d.lastRate && now > d.timeRate {
					avg := fmt.Sprintf("%s_%s", name, rateSuffix)
					computed := (value - d.lastRate) / float64(now-d.timeRate) * 1000
					prometheusGauges[avg].WithLabelValues(d.methods.Labels()...).Set(computed)
					d.lastRate = value
					d.timeRate = now
				} else if value < d.lastRate {
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

func readData(d *devices.Devices) {
	if d.IsEmpty() {
		return
	}

	ctx := devices.Context{
		NetClient: &http.Client{Timeout: time.Second * 10},
		Clog:      logrus.WithField("task", "read metrics"),
	}

	deviceHeap := make(PriorityQueue, d.Length())
	now := time.Now().UnixMilli()

	for i, dev := range *d.Devices {
		deviceHeap[i] = &DeviceItem{
			methods:  dev,
			last:     math.NaN(),
			lastRate: math.NaN(),
			time:     now,
			timeRate: now,
			expiry:   now/1000 + (500+rand.Int63n(501))*int64(dev.IntervalSec())/1000,
			index:    i,
		}
	}

	heap.Init(&deviceHeap)

	for deviceHeap.Len() > 0 {
		item := heap.Pop(&deviceHeap).(*DeviceItem)
		sleep := int64(1)

		if now < item.expiry {
			sleep = item.expiry - time.Now().Unix()
		}

		time.Sleep(time.Second * time.Duration(sleep))

		ctx.PushField("device", item.methods.LogName())
		defer ctx.Pop()

		factor := setPrometheusValue(ctx, item)
		item.expiry = time.Now().Unix() + int64(item.methods.IntervalSec()*factor)

		heap.Push(&deviceHeap, item)
	}
}

var GlobalDevices devices.Devices
var GlobalOverview *Overview

func overviewHandler(writer http.ResponseWriter, request *http.Request) {
	SyncPoint.Lock()
	defer SyncPoint.Unlock()

	GenerateOverview(writer, GlobalOverview, &GlobalDevices)
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

	GlobalDevices = devices.LoadDevices(setup)

	if GlobalDevices.IsEmpty() {
		logrus.Panic("no devices have been defined, exiting...")
	}

	registry = prometheus.NewRegistry()

	for _, d := range *GlobalDevices.Devices {
		name := d.MetricName()

		if name != "" {
			cat := d.CategoryName()

			if _, prs := prometheusGauges[name]; !prs {
				prometheusGauges[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name: name,
					Help: name,
				}, []string{"provider", "name", "room"})

				registry.MustRegister(prometheusGauges[name])
			}

			if cat == "energy" {
				counter := fmt.Sprintf("%s_%s", name, totalSuffix)

				if _, prs := prometheusCounters[counter]; !prs {
					prometheusCounters[counter] = prometheus.NewCounterVec(prometheus.CounterOpts{
						Name: counter,
						Help: name,
					}, []string{"provider", "name", "room"})

					registry.MustRegister(prometheusCounters[counter])
				}
			}

			if cat == "energy" {
				avg := fmt.Sprintf("%s_%s", name, rateSuffix)

				if _, prs := prometheusGauges[avg]; !prs {
					prometheusGauges[avg] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
						Name: avg,
						Help: name,
					}, []string{"provider", "name", "room"})

					registry.MustRegister(prometheusGauges[avg])
				}
			}
		}
	}

	var err error
	GlobalOverview, err = LoadOverviewDesc(setup)

	if err != nil {
		logrus.Panic(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/index.html", overviewHandler)
	mux.HandleFunc("/", overviewHandler)

	go readData(&GlobalDevices)

	var srv *http.Server
	if enableH2c {
		srv = &http.Server{Addr: bind, Handler: h2c.NewHandler(mux, &http2.Server{})}
	} else {
		srv = &http.Server{Addr: bind, Handler: mux}
	}

	logrus.Infof("start listing on %s", bind)
	log.Fatal(srv.ListenAndServe())
}
