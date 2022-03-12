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
	"github.com/paulrosania/go-charset/charset"
	_ "github.com/paulrosania/go-charset/data"
	"net/http"
	"io"
	"log"
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
	HmName    string
	Name      string
	Room      string
	Interval  float64
	ScriptUrl string
	Metric    string
	Category  string
	DPChannel int
	DPName    string
}

func (t HomematicDevice) FullName() string {
	return fmt.Sprintf(
		"%s[provider:homematic,hm:%s,name:%s,room:%s,interval:%v]",
		t.Metric,
		t.HmName,
		t.Name,
		t.Room,
		t.Interval,
	)
}

func (t HomematicDevice) LogName() string {
	return fmt.Sprintf("Homematic(%s/%s)", t.HmName, t.Name)
}

func (t HomematicDevice) Labels() []string {
	return []string{"homematic", t.Name, t.Room}
}

func (t HomematicDevice) IntervalSec() uint64 {
	return uint64(t.Interval)
}

func (t HomematicDevice) MetricName() string {
	return t.Metric
}

func (t HomematicDevice) CategoryName() string {
	return t.Category
}

func (t HomematicDevice) CurrentValue(ctx Context) (float64, error) {
	return getValue(ctx, &t)
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

func parseXML(xmlDoc []byte, target interface{}) {
	reader := bytes.NewReader(xmlDoc)
	decoder := xml.NewDecoder(reader)
	decoder.CharsetReader = charset.NewReader
	if err := decoder.Decode(target); err != nil {
		log.Fatalf("unable to parse XML '%s':\n%s", err, xmlDoc)
	}
}

func readHomematicXml(ctx Context, scriptUrl string, cmd string, result *homematicXml) error {

	log.Printf("%s",bytes.NewBufferString(cmd))

	req, err1 := http.NewRequest("POST", scriptUrl, bytes.NewBufferString(cmd))
	req.SetBasicAuth("Admin", "q.97U-n.")
	cli := &http.Client{}
	response, err1 := cli.Do(req)
	
//	response, err1 := ctx.Post(scriptUrl, "application/text", bytes.NewBufferString(cmd))

	if err1 != nil {
		return err1
	}

	defer response.Body.Close()

	body, err2 := io.ReadAll(response.Body)
	log.Printf("body ==========  %s",string(body))
	var parsed []byte
	parseXML(body, &parsed)
	log.Printf("body ==========  %s",string(parsed))

	if err2 != nil {
		return err2
	}

	return nil
	err3 := xml.Unmarshal(parsed, &result)

	if err3 != nil {
		return err3
	}

	return nil

}

func getValue(ctx Context, t *HomematicDevice) (float64, error) {
	valueCmd := fmt.Sprintf(
		`var value = dom.GetObject('%s:%d.%s').State();`, t.HmName, t.DPChannel, t.DPName)

	info := homematicXml{}
	err1 := readHomematicXml(ctx, t.ScriptUrl, valueCmd, &info)

	if err1 != nil {
		return 0, err1
	}

	value, err2 := strconv.ParseFloat(info.Value, 64)

	if err2 != nil {
		return 0, err2
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

	log.Printf(roomCmd)
	info := homematicXml{}
	err1 := readHomematicXml(ctx, homematic.ScriptUrl, roomCmd, &info)

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

func generateHmDevice(ctx Context, deviceList *[]DeviceInterface, desc *HomematicDesc) error {
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
			HmName:    desc.HmName,
			Name:      desc.Name,
			Room:      desc.Room,
			Interval:  desc.Interval,
			ScriptUrl: desc.ScriptUrl,
			Metric:    desc.EnergyMetric,
			Category:  "energy",
			DPChannel: desc.EnergyChannel,
			DPName:    "ENERGY_COUNTER",
		}

		*deviceList = append(*deviceList, energy)
	}

	if desc.PowerMetric != "" {
		power := HomematicDevice{
			HmName:    desc.HmName,
			Name:      desc.Name,
			Room:      desc.Room,
			Interval:  desc.Interval,
			ScriptUrl: desc.ScriptUrl,
			Metric:    desc.PowerMetric,
			Category:  "power",
			DPChannel: desc.EnergyChannel,
			DPName:    "POWER",
		}

		*deviceList = append(*deviceList, power)
	}

	if desc.TemperatureMetric != "" {
		temperature := HomematicDevice{
			HmName:    desc.HmName,
			Name:      desc.Name,
			Room:      desc.Room,
			Interval:  desc.Interval,
			ScriptUrl: desc.ScriptUrl,
			Metric:    desc.TemperatureMetric,
			Category:  "temperature",
			DPChannel: desc.TemperatureChannel,
			DPName:    desc.TemperatureName,
		}

		*deviceList = append(*deviceList, temperature)
	}

	if desc.LightMetric != "" {
		light := HomematicDevice{
			HmName:    desc.HmName,
			Name:      desc.Name,
			Room:      desc.Room,
			Interval:  desc.Interval,
			ScriptUrl: desc.ScriptUrl,
			Metric:    desc.LightMetric,
			Category:  "light",
			DPChannel: desc.LightChannel,
			DPName:    desc.LightName,
		}

		*deviceList = append(*deviceList, light)
	}

	return nil
}

func generateHomematic(ctx Context, deviceList *[]DeviceInterface, desc *HomematicDesc) error {
	typeCmd := fmt.Sprintf(
`
var channel = dom.GetObject('%s:0.UNREACH').Channel();
var device = dom.GetObject(dom.GetObject(channel).Device());
var hssType = device.HssType();
var interface = dom.GetObject(device.Interface());`, desc.HmName)

	info := homematicXml{}
	err1 := readHomematicXml(ctx, desc.ScriptUrl, typeCmd, &info)

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
		return generateHmDevice(ctx, deviceList, desc)
	case hssType == "HM-ES-PMSw1-Pl":
		desc.EnergyChannel = 2
		desc.LightMetric = ""
		desc.PowerChannel = 2
		desc.TemperatureMetric = ""
		return generateHmDevice(ctx, deviceList, desc)
	case hssType == "HM-ES-TX-WM":
		desc.EnergyChannel = 1
		desc.LightMetric = ""
		desc.PowerChannel = 1
		desc.TemperatureMetric = ""
		return generateHmDevice(ctx, deviceList, desc)
	case hssType == "HmIP-WTH-2" || hssType == "HmIP-eTRV-B":
		desc.EnergyMetric = ""
		desc.LightMetric = ""
		desc.PowerMetric = ""
		desc.TemperatureChannel = 1
		desc.TemperatureName = "ACTUAL_TEMPERATURE"
		return generateHmDevice(ctx, deviceList, desc)
	case hssType == "HM-CC-RT-DN":
		desc.EnergyMetric = ""
		desc.LightMetric = ""
		desc.PowerMetric = ""
		desc.TemperatureChannel = 4
		desc.TemperatureName = "ACTUAL_TEMPERATURE"
		return generateHmDevice(ctx, deviceList, desc)
	case hssType == "HM-WDS10-TH-O" || hssType == "HM-WDS40-TH-I":
		desc.EnergyMetric = ""
		desc.LightMetric = ""
		desc.PowerMetric = ""
		desc.TemperatureChannel = 1
		desc.TemperatureName = "TEMPERATURE"
		return generateHmDevice(ctx, deviceList, desc)
	case hssType == "HmIP-SMI55":
		desc.EnergyMetric = ""
		desc.LightChannel = 3
		desc.LightName = "CURRENT_ILLUMINATION"
		desc.PowerMetric = ""
		desc.TemperatureMetric = ""
		return generateHmDevice(ctx, deviceList, desc)
	case hssType == "HmIP-SMI":
		desc.EnergyMetric = ""
		desc.LightChannel = 1
		desc.LightName = "CURRENT_ILLUMINATION"
		desc.PowerMetric = ""
		desc.TemperatureMetric = ""
		return generateHmDevice(ctx, deviceList, desc)
	case hssType == "HM-Sec-MDIR-2":
		desc.EnergyMetric = ""
		desc.LightChannel = 1
		desc.LightName = "BRIGHTNESS"
		desc.PowerMetric = ""
		desc.TemperatureMetric = ""
		return generateHmDevice(ctx, deviceList, desc)
	case hssType == "HM-WDS100-C6-O":
		desc.EnergyMetric = ""
		desc.LightChannel = 1
		desc.LightName = "BRIGHTNESS"
		desc.PowerMetric = ""
		desc.TemperatureChannel = 1
		desc.TemperatureName = "TEMPERATURE"
		return generateHmDevice(ctx, deviceList, desc)
	default:
		ctx.Clog.Warn("unknown HssType: ", hssType)
	}

	return nil
}

func LoadHomematicDevices(ctx Context, deviceList *[]DeviceInterface, device Device) error {
	duration, err1 := time.ParseDuration(device.Source.Interval)

	if err1 != nil {
		ctx.Warn(err1, "cannot parse duration")
		return nil
	}

	if duration < 0 {
		duration = 60
	}

	scriptUrl := fmt.Sprintf("http://%s:8181/Test.exe", device.Source.Address)

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
			Interval:          duration.Seconds(),
		}

		err := generateHomematic(ctx, deviceList, &homematic)

		if err != nil {
			ctx.Warn(err, "cannot load homematic device data")
		}
	}

	return nil
}
