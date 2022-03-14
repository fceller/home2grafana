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
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"os"
	"regexp"
	"time"

	"io/ioutil"
	"net/http"
	"path/filepath"
)

type Device struct {
	Source struct {
		Provider          string `yaml:"provider"`
		EnergyMetric      string `yaml:"energy_metric"`
		PowerMetric       string `yaml:"power_metric"`
		TemperatureMetric string `yaml:"temperature_metric"`
		LightMetric       string `yaml:"light_metric"`
		Address           string `yaml:"address"`
		UserName          string `yaml:"user_name,omitempty"`
		Password          string `yaml:"password,omitempty"`
		useSSL            bool   `yaml:"ssl,omitempty"`
		Interval          string `yaml:"interval"`
		Devices           []struct {
			Name    string `yaml:"name"`
			Room    string `yaml:"room"`
			Address string `yaml:"address"`
			HmName  string `yaml:"hm_name"`
		} `yaml:"devices"`
	} `yaml:"source"`
}

type DeviceInterface interface {
	FullName() string
	LogName() string
	Labels() []string
	CategoryName() string
	IntervalSec() uint64

	MetricName() string
	CurrentValue(Context) (float64, error)
}

func LoadDevices(setup string) []DeviceInterface {
	yamlRE := regexp.MustCompile(`\.yaml$`)

	ctx := Context{
		Root:      setup,
		NetClient: &http.Client{Timeout: time.Second * 10},
		Clog:      logrus.WithField("task", "load devices"),
	}

	ctx.PushField("root", ctx.Root)
	defer ctx.Pop()

	deviceList := make([]DeviceInterface, 0)

	err := filepath.Walk(ctx.Root,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() && yamlRE.MatchString(info.Name()) {
				ctx.PushField("filepath", path)
				defer ctx.Pop()

				ctx.Info("loading device file")

				yfile, err := ioutil.ReadFile(path)

				if err != nil {
					ctx.Warn(err, "cannot read file")
					return nil
				}

				device := Device{}
				err = yaml.Unmarshal(yfile, &device)

				if err != nil {
					ctx.Warn(err, "cannot parse file")
					return err
				}

				provider := device.Source.Provider
				ctx.PushField("provider", provider)
				defer ctx.Pop()

				switch {
				case provider == "tasmota":
					return LoadTasmotaDevices(ctx, &deviceList, device)
				case provider == "homematic":
					return LoadHomematicDevices(ctx, &deviceList, device)
				case provider == "iobroker":
					return LoadIoBrokerDevices(ctx, &deviceList, device)
				default:
					ctx.Clog.Warn("unkown provider")
					return nil
				}
			}

			return nil
		})

	if err != nil {
		ctx.Fatal(err, "cannot walk device directory")
	}

	return deviceList
}
