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
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"golang.org/x/text/encoding/charmap"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

type HomematicDesc struct {
	HmName             string
	Name               string
	Room               string
	Interval           float64
	ScriptUrl          string
	UserName           string
	Password           string
	EnergyMetric       string
	EnergyChannel      int
	PowerMetric        string
	PowerChannel       int
	TemperatureMetric  string
	TemperatureChannel int
	TemperatureName    string
	LightMetric        string
	LightChannel       int
	LightName          string
}

type HomematicDevice struct {
	metric    string
	name      string
	room      string
	hmName    string
	interval  float64
	scriptUrl string
	userName  string
	password  string
	category  string
	dpChannel int
	dpName    string
	lastValue string
}

func (t *HomematicDevice) DeviceID() string {
	return fmt.Sprintf("homematic|%s", t.hmName)
}

func (t *HomematicDevice) Name() string {
	return t.name
}

func (t *HomematicDevice) Room() string {
	return t.room
}

func (t *HomematicDevice) FullName() string {
	return fmt.Sprintf(
		"%s[provider:homematic,hm:%s,name:%s,room:%s,interval:%v]",
		t.metric,
		t.hmName,
		t.name,
		t.room,
		t.interval,
	)
}

func (t *HomematicDevice) LogName() string {
	return fmt.Sprintf("Homematic(%s/%s)", t.hmName, t.name)
}

func (t *HomematicDevice) Labels() []string {
	return []string{"homematic", t.name, t.room}
}

func (t *HomematicDevice) IntervalSec() uint64 {
	return uint64(t.interval)
}

func (t *HomematicDevice) MetricName() string {
	return t.metric
}

func (t *HomematicDevice) CategoryName() string {
	return t.category
}

func (t *HomematicDevice) CurrentValue(ctx Context) (float64, error) {
	return getValue(ctx, t)
}

func (t *HomematicDevice) LastValue() string {
	return t.lastValue
}

type homematicXml struct {
	XmlName       xml.Name `xml:"xml"`
	Text          string   `xml:",chardata"`
	Exec          string   `xml:"exec"`
	SessionId     string   `xml:"sessionId"`
	HttpUserAgent string   `xml:"httpUserAgent"`
	Channel       string   `xml:"channel"`
	Device        string   `xml:"device"`
	Interface     string   `xml:"interface"`
	HssType       string   `xml:"hssType"`
	Name          string   `xml:"name"`
	Room          string   `xml:"room"`
	Value         string   `xml:"value"`
}

func readHomematicXml(
	ctx Context, scriptUrl string, userName string, password string, cmd string, result *homematicXml) error {

	req, err := http.NewRequest("POST", scriptUrl, bytes.NewBufferString(cmd))
	if err != nil {
		return err
	}

	req.SetBasicAuth(userName, password)
	cli := &http.Client{}
	response, err := cli.Do(req)

	if err != nil {
		return err
	}

	body, err := io.ReadAll(charmap.ISO8859_1.NewDecoder().Reader(response.Body))
	defer response.Body.Close()

	if response.StatusCode != 200 {
		ctx.Clog.Fatalf("request failed: Response code %d ", string(body))
	}

	if err = parseXml(ctx, body, &result); err != nil {
		return err
	}

	return nil
}

func getValue(ctx Context, t *HomematicDevice) (float64, error) {
	valueCmd := fmt.Sprintf(
		`var value = dom.GetObject('%s:%d.%s').State();`, t.hmName, t.dpChannel, t.dpName)

	info := homematicXml{}
	err1 := readHomematicXml(ctx, t.scriptUrl, t.userName, t.password, valueCmd, &info)

	if err1 != nil {
		return 0, err1
	}

	value, err2 := strconv.ParseFloat(info.Value, 64)

	if err2 != nil {
		return 0, err2
	}

	switch t.category {
	case "temperature":
		t.lastValue = fmt.Sprintf("%.2f °C", value)
	case "power":
		t.lastValue = fmt.Sprintf("%.2f kW/h", value/1000)
	case "energy":
		t.lastValue = fmt.Sprintf("%.2f kW/h", value/1000)
	case "light":
		t.lastValue = fmt.Sprintf("%.2f", value)
	default:
		t.lastValue = fmt.Sprintf("%.2f %s", value, t.category)
	}

	return value, nil
}

func readHmDevice(ctx Context, channel int, name string, homematic *HomematicDesc) error {
	roomCmd := fmt.Sprintf(
		`var channelId = dom.GetObject('%s:%d.%s').Channel();
		var channel = dom.GetObject(channelId);
		var name = channel.Name();
		var roomId = channel.ChnRoom();
		var room = dom.GetObject(roomId);
	`, homematic.HmName, channel, name)

	info := homematicXml{}
	err1 := readHomematicXml(ctx, homematic.ScriptUrl, homematic.UserName, homematic.Password, roomCmd, &info)

	if err1 != nil {
		return err1
	}

	if len(homematic.Name) == 0 && len(info.Name) > 0 {
		homematic.Name = info.Name
	}

	if len(homematic.Room) == 0 && len(info.Room) > 0 {
		homematic.Room = info.Room
	}

	ctx.PushFields(logrus.Fields{"name": homematic.Name, "room": homematic.Room})
	ctx.Info("found device")
	ctx.Pop()

	return nil
}

func generateHmDevice(ctx Context, devices *Devices, desc *HomematicDesc) error {
	var err error

	if desc.EnergyMetric != "" {
		err = readHmDevice(ctx, desc.EnergyChannel, "ENERGY_COUNTER", desc)
	} else if desc.TemperatureMetric != "" {
		err = readHmDevice(ctx, desc.TemperatureChannel, desc.TemperatureName, desc)
	} else if desc.LightMetric != "" {
		err = readHmDevice(ctx, desc.LightChannel, desc.LightName, desc)
	}

	if err != nil {
		return err
	}

	if desc.EnergyMetric != "" {
		energy := HomematicDevice{
			name:      desc.Name,
			room:      desc.Room,
			hmName:    desc.HmName,
			interval:  desc.Interval,
			scriptUrl: desc.ScriptUrl,
			metric:    desc.EnergyMetric,
			category:  "energy",
			dpChannel: desc.EnergyChannel,
			dpName:    "ENERGY_COUNTER",
		}

		devices.addDevice(&energy)
	}

	if desc.PowerMetric != "" {
		power := HomematicDevice{
			name:      desc.Name,
			room:      desc.Room,
			hmName:    desc.HmName,
			interval:  desc.Interval,
			scriptUrl: desc.ScriptUrl,
			metric:    desc.PowerMetric,
			category:  "power",
			dpChannel: desc.EnergyChannel,
			dpName:    "POWER",
		}

		devices.addDevice(&power)
	}

	if desc.TemperatureMetric != "" {
		temperature := HomematicDevice{
			name:      desc.Name,
			room:      desc.Room,
			hmName:    desc.HmName,
			interval:  desc.Interval,
			scriptUrl: desc.ScriptUrl,
			metric:    desc.TemperatureMetric,
			category:  "temperature",
			dpChannel: desc.TemperatureChannel,
			dpName:    desc.TemperatureName,
		}

		devices.addDevice(&temperature)
	}

	if desc.LightMetric != "" {
		light := HomematicDevice{
			name:      desc.Name,
			room:      desc.Room,
			hmName:    desc.HmName,
			interval:  desc.Interval,
			scriptUrl: desc.ScriptUrl,
			metric:    desc.LightMetric,
			category:  "light",
			dpChannel: desc.LightChannel,
			dpName:    desc.LightName,
		}

		devices.addDevice(&light)
	}

	return nil
}

func generateHomematic(ctx Context, devices *Devices, desc *HomematicDesc) error {
	typeCmd := fmt.Sprintf(
		`var channel = dom.GetObject('%s:0.UNREACH').Channel();
		var device = dom.GetObject(dom.GetObject(channel).Device());
		var hssType = device.HssType();
		var interface = dom.GetObject(device.Interface());`, desc.HmName)

	info := homematicXml{}
	err1 := readHomematicXml(ctx, desc.ScriptUrl, desc.UserName, desc.Password, typeCmd, &info)

	if err1 != nil {
		return err1
	}

	if info.Channel == "null" {
		return errors.New("unknown desc device: " + desc.HmName)
	}

	hssType := info.HssType
	ctx.PushFields(logrus.Fields{"HssType": hssType, "HmName": desc.HmName})
	defer ctx.Pop()

	switch {
	case hssType == "HMIP-PSM":
		desc.EnergyChannel = 6
		desc.LightMetric = ""
		desc.PowerChannel = 6
		desc.TemperatureChannel = 0
		desc.TemperatureName = "ACTUAL_TEMPERATURE"
		return generateHmDevice(ctx, devices, desc)
	case hssType == "HM-ES-PMSw1-Pl":
		desc.EnergyChannel = 2
		desc.LightMetric = ""
		desc.PowerChannel = 2
		desc.TemperatureMetric = ""
		return generateHmDevice(ctx, devices, desc)
	case hssType == "HM-ES-TX-WM":
		desc.EnergyChannel = 1
		desc.LightMetric = ""
		desc.PowerChannel = 1
		desc.TemperatureMetric = ""
		return generateHmDevice(ctx, devices, desc)
	case hssType == "HmIP-WTH-2" || hssType == "HmIP-eTRV-B":
		desc.EnergyMetric = ""
		desc.LightMetric = ""
		desc.PowerMetric = ""
		desc.TemperatureChannel = 1
		desc.TemperatureName = "ACTUAL_TEMPERATURE"
		return generateHmDevice(ctx, devices, desc)
	case hssType == "HM-CC-RT-DN":
		desc.EnergyMetric = ""
		desc.LightMetric = ""
		desc.PowerMetric = ""
		desc.TemperatureChannel = 4
		desc.TemperatureName = "ACTUAL_TEMPERATURE"
		return generateHmDevice(ctx, devices, desc)
	case hssType == "HM-WDS10-TH-O" || hssType == "HM-WDS40-TH-I":
		desc.EnergyMetric = ""
		desc.LightMetric = ""
		desc.PowerMetric = ""
		desc.TemperatureChannel = 1
		desc.TemperatureName = "TEMPERATURE"
		return generateHmDevice(ctx, devices, desc)
	case hssType == "HmIP-SMI55":
		desc.EnergyMetric = ""
		desc.LightChannel = 3
		desc.LightName = "CURRENT_ILLUMINATION"
		desc.PowerMetric = ""
		desc.TemperatureMetric = ""
		return generateHmDevice(ctx, devices, desc)
	case hssType == "HmIP-SMI":
		desc.EnergyMetric = ""
		desc.LightChannel = 1
		desc.LightName = "CURRENT_ILLUMINATION"
		desc.PowerMetric = ""
		desc.TemperatureMetric = ""
		return generateHmDevice(ctx, devices, desc)
	case hssType == "HM-Sec-MDIR-2":
		desc.EnergyMetric = ""
		desc.LightChannel = 1
		desc.LightName = "BRIGHTNESS"
		desc.PowerMetric = ""
		desc.TemperatureMetric = ""
		return generateHmDevice(ctx, devices, desc)
	case hssType == "HM-WDS100-C6-O":
		desc.EnergyMetric = ""
		desc.LightChannel = 1
		desc.LightName = "BRIGHTNESS"
		desc.PowerMetric = ""
		desc.TemperatureChannel = 1
		desc.TemperatureName = "TEMPERATURE"
		return generateHmDevice(ctx, devices, desc)
	default:
		ctx.Clog.Warn("unknown HssType: ", hssType)
	}

	return nil
}

func LoadHomematicDevices(ctx Context, devices *Devices, device Device) error {
	duration, err1 := time.ParseDuration(device.Source.Interval)

	if err1 != nil {
		ctx.Warn(err1, "cannot parse duration")
		return nil
	}

	if duration < 0 {
		duration = 60
	}

	var port string
	var protocol = "http"
	if device.Source.useSSL {
		port = "48181"
		protocol += "s"
	} else {
		port = "8181"
	}

	scriptUrl := fmt.Sprintf("%s://%s:%s/Test.exe", protocol, device.Source.Address, port)

	for _, d := range device.Source.Devices {
		homematic := HomematicDesc{
			EnergyMetric:      device.Source.EnergyMetric,
			TemperatureMetric: device.Source.TemperatureMetric,
			LightMetric:       device.Source.LightMetric,
			PowerMetric:       device.Source.PowerMetric,
			HmName:            d.HmName,
			Name:              d.Name,
			Room:              d.Room,
			ScriptUrl:         scriptUrl,
			UserName:          device.Source.UserName,
			Password:          device.Source.Password,
			Interval:          duration.Seconds(),
		}

		err := generateHomematic(ctx, devices, &homematic)

		if err != nil {
			ctx.Warn(err, "cannot load homematic device data")
		}
	}

	return nil
}
