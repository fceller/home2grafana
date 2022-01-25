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
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"time"

	"encoding/json"
	"net/http"
)

type TasmotaDevice struct {
	Metric    string
	Name      string
	Room      string
	Interval  float64
	Category  string
	EnergyUrl string
	StatusUrl string
}

type TasmotaStatus struct {
	Status struct {
		Module       int      `json:"Module"`
		DeviceName   string   `json:"DeviceName"`
		FriendlyName []string `json:"FriendlyName"`
		Topic        string   `json:"Topic"`
		ButtonTopic  string   `json:"ButtonTopic"`
		Power        int      `json:"Power"`
		PowerOnState int      `json:"PowerOnState"`
		LedState     int      `json:"LedState"`
		LedMask      string   `json:"LedMask"`
		SaveData     int      `json:"SaveData"`
		SaveState    int      `json:"SaveState"`
		SwitchTopic  string   `json:"SwitchTopic"`
		SwitchMode   []int    `json:"SwitchMode"`
		ButtonRetain int      `json:"ButtonRetain"`
		SwitchRetain int      `json:"SwitchRetain"`
		SensorRetain int      `json:"SensorRetain"`
		PowerRetain  int      `json:"PowerRetain"`
		InfoRetain   int      `json:"InfoRetain"`
		StateRetain  int      `json:"StateRetain"`
	} `json:"Status"`
}

type TasmotaEnergy struct {
	StatusSNS struct {
		Time   string `json:"Time"`
		ENERGY struct {
			TotalStartTime string  `json:"TotalStartTime"`
			Total          float64 `json:"Total"`
			Yesterday      float64 `json:"Yesterday"`
			Today          float64 `json:"Today"`
			Power          int     `json:"Power"`
			ApparentPower  int     `json:"ApparentPower"`
			ReactivePower  int     `json:"ReactivePower"`
			Factor         float64 `json:"Factor"`
			Voltage        int     `json:"Voltage"`
			Current        float64 `json:"Current"`
		} `json:"ENERGY"`
	} `json:"StatusSNS"`
}

func (t TasmotaDevice) FullName() string {
	return fmt.Sprintf(
		"%s[provider:tasmota,name:%s,room:%s,interval:%v]",
		t.Metric,
		t.Name,
		t.Room,
		t.Interval,
	)
}

func (t TasmotaDevice) LogName() string {
	return fmt.Sprintf("Tasmota(%s)", t.Name)
}

func (t TasmotaDevice) Labels() []string {
	return []string{"tasmota", t.Name, t.Room}
}

func (t TasmotaDevice) IntervalSec() uint64 {
	return uint64(t.Interval)
}

func (t TasmotaDevice) MetricName() string {
	return t.Metric
}

func (t TasmotaDevice) CategoryName() string {
	return t.Category
}

func (t TasmotaDevice) CurrentValue(ctx Context) (float64, error) {
	response, err1 := ctx.NetClient.Get(t.EnergyUrl)

	if err1 != nil {
		return 0, err1
	}

	defer response.Body.Close()

	body, err2 := io.ReadAll(response.Body)

	if err2 != nil {
		return 0, err2
	}

	tasmota := TasmotaEnergy{}
	err3 := json.Unmarshal([]byte(body), &tasmota)

	if err3 != nil {
		return 0, err3
	}

	if t.Category == "energy" {
		return tasmota.StatusSNS.ENERGY.Total * 1000, nil
	} else if t.Category == "power" {
		return float64(tasmota.StatusSNS.ENERGY.Power), nil
	} else {
		return 0, errors.New(fmt.Sprintf("unknown category %s", t.Category))
	}
}

func readTasmotaName(netClient *http.Client, tasmota *TasmotaDevice) error {
	response, err1 := netClient.Get(tasmota.StatusUrl)

	if err1 != nil {
		return err1
	}

	defer response.Body.Close()
	body, err2 := io.ReadAll(response.Body)

	if err2 != nil {
		return err2
	}

	status := TasmotaStatus{}
	err3 := json.Unmarshal(body, &status)

	if err3 != nil {
		return err3
	}

	if len(status.Status.FriendlyName) > 0 {
		tasmota.Name = status.Status.FriendlyName[0]
	} else if len(status.Status.DeviceName) > 0 {
		tasmota.Name = status.Status.DeviceName
	}

	return nil
}

func LoadTasmotaDevices(ctx Context, deviceList *[]DeviceInterface, device Device) error {
	duration, err := time.ParseDuration(device.Source.Interval)

	if err != nil {
		ctx.Warn(err, "cannot parse duration")
		return nil
	}

	if duration < 0 {
		duration = 60
	}

	for _, d := range device.Source.Devices {
		energyUrl := fmt.Sprintf("http://%s/cm?cmnd=Status%%2010", d.Address)
		statusUrl := fmt.Sprintf("http://%s/cm?cmnd=Status", d.Address)

		energy := TasmotaDevice{
			Metric:    device.Source.EnergyMetric,
			Category:  "energy",
			Name:      d.Name,
			Room:      d.Room,
			EnergyUrl: energyUrl,
			StatusUrl: statusUrl,
			Interval:  duration.Seconds(),
		}

		if len(energy.Name) == 0 {
			err := readTasmotaName(ctx.NetClient, &energy)

			if err == nil {
				ctx.PushFields(logrus.Fields{"name": energy.Name, "room": energy.Room})
				ctx.Info("found device")
				ctx.Pop()
			} else {
				ctx.Warn(err, "cannot read name")
				continue
			}
		}

		if device.Source.EnergyMetric != "" {
			*deviceList = append(*deviceList, energy)
		}

		if device.Source.PowerMetric != "" {
			power := TasmotaDevice{
				Metric:    device.Source.PowerMetric,
				Category:  "power",
				Name:      energy.Name,
				Room:      energy.Room,
				EnergyUrl: energy.EnergyUrl,
				StatusUrl: energy.StatusUrl,
				Interval:  energy.Interval,
			}

			*deviceList = append(*deviceList, power)
		}
	}

	return nil
}
