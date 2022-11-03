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

package devices

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"strconv"
	"time"
)

type IoBrokerDevice struct {
	metric         string
	name           string
	room           string
	interval       float64
	address        string
	temperatureUrl string
	lastValue      string
}

func (t *IoBrokerDevice) DeviceID() string {
	return fmt.Sprintf("iobroker|%s", t.address)
}

func (t *IoBrokerDevice) Name() string {
	return t.name
}

func (t *IoBrokerDevice) Room() string {
	return t.room
}

func (t *IoBrokerDevice) FullName() string {
	return fmt.Sprintf(
		"%s[provider:iobroker,endpoint:%s,name:%s,room:%s,interval:%v]",
		t.metric,
		t.address,
		t.name,
		t.room,
		t.interval,
	)
}

func (t *IoBrokerDevice) LogName() string {
	return fmt.Sprintf("IoBroker(%s)", t.name)
}

func (t *IoBrokerDevice) Labels() []string {
	return []string{"iobroker", t.name, t.room}
}

func (t *IoBrokerDevice) IntervalSec() uint64 {
	return uint64(t.interval)
}

func (t *IoBrokerDevice) MetricName() string {
	return t.metric
}

func (t *IoBrokerDevice) CategoryName() string {
	return "temperature"
}

func (t *IoBrokerDevice) CurrentValue(ctx Context) (float64, error) {
	response, err1 := ctx.NetClient.Get(t.temperatureUrl)

	if err1 != nil {
		return 0, err1
	}

	defer response.Body.Close()

	body, err2 := io.ReadAll(response.Body)

	if err2 != nil {
		return 0, err2
	}

	temp, err3 := strconv.ParseFloat(string(body), 64)

	if err3 != nil {
		return 0, err3
	}

	t.lastValue = fmt.Sprintf("%.2f °C", temp)
	return temp, nil
}

func (t *IoBrokerDevice) LastValue() string {
	return t.lastValue
}

func LoadIoBrokerDevices(ctx Context, devices *Devices, device Device) error {
	duration, err := time.ParseDuration(device.Source.Interval)

	if err != nil {
		ctx.Warn(err, "cannot parse duration")
		return nil
	}

	if duration < 0 {
		duration = 60
	}

	for _, d := range device.Source.Devices {
		temperatureUrl := fmt.Sprintf("http://%s/getPlainValue/%s", device.Source.Address, d.Address)

		if device.Source.TemperatureMetric != "" {
			iobroker := IoBrokerDevice{
				name:           d.Name,
				room:           d.Room,
				metric:         device.Source.TemperatureMetric,
				temperatureUrl: temperatureUrl,
				address:        d.Address,
				interval:       duration.Seconds(),
			}

			ctx.PushFields(logrus.Fields{"name": iobroker.name, "room": iobroker.room, "address": iobroker.address})
			ctx.Info("found device")
			ctx.Pop()

			devices.addDevice(&iobroker)
		}
	}

	return nil
}
