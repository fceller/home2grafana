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
	Metric   string
	Name     string
	Room     string
	Interval float64
	Address  string

	TemperatureUrl string
}

func (t IoBrokerDevice) FullName() string {
	return fmt.Sprintf(
		"%s[provider:iobroker,endpoint:%s,name:%s,room:%s,interval:%v]",
		t.Metric,
		t.Address,
		t.Name,
		t.Room,
		t.Interval,
	)
}

func (t IoBrokerDevice) LogName() string {
	return fmt.Sprintf("IoBroker(%s)", t.Name)
}

func (t IoBrokerDevice) Labels() []string {
	return []string{"iobroker", t.Name, t.Room}
}

func (t IoBrokerDevice) IntervalSec() uint64 {
	return uint64(t.Interval)
}

func (t IoBrokerDevice) MetricName() string {
	return t.Metric
}

func (t IoBrokerDevice) CategoryName() string {
	return "temperature"
}

func (t IoBrokerDevice) CurrentValue(ctx Context) (float64, error) {
	response, err1 := ctx.NetClient.Get(t.TemperatureUrl)

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

	return temp, nil
}

func LoadIoBrokerDevices(ctx Context, deviceList *[]DeviceInterface, device Device) error {
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
				Metric:         device.Source.TemperatureMetric,
				Name:           d.Name,
				Room:           d.Room,
				TemperatureUrl: temperatureUrl,
				Address:        d.Address,
				Interval:       duration.Seconds(),
			}

			ctx.PushFields(logrus.Fields{"name": iobroker.Name, "room": iobroker.Room, "address": iobroker.Address})
			ctx.Info("found device")
			ctx.Pop()

			*deviceList = append(*deviceList, iobroker)
		}
	}

	return nil
}
