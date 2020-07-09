package upower

import (
	"fmt"
	"os"

	"github.com/go-acme/lego/log"
	dbus "github.com/godbus/dbus"
	homie "github.com/jbonachera/homie-go/homie"
)

type upowerProvider struct {
}

func NewUpowerProvider() *upowerProvider {
	return &upowerProvider{}
}

func (l *upowerProvider) Serve(node homie.Node) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to connect to system bus:", err)
		return
	}
	obj := conn.Object("org.freedesktop.UPower", "/org/freedesktop/UPower/devices/battery_BAT0")
	result, err := obj.GetProperty("org.freedesktop.UPower.Device.Percentage")
	if err != nil {
		log.Print(err)
	}
	suspend := node.NewProperty("batteryPercentage", "float64")
	suspend.SetValue(fmt.Sprintf("%.2f", result.Value().(float64)))

	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
	); err != nil {
		panic(err)
	}
	c := make(chan *dbus.Signal, 0)

	conn.Signal(c)

	go func() {
		for event := range c {
			if len(event.Body) >= 2 {
				if v, ok := event.Body[0].(string); ok && v == "org.freedesktop.UPower.Device" {
					value := event.Body[1].(map[string]dbus.Variant)["Percentage"].Value()
					if value != nil {
						suspend.SetValue(fmt.Sprintf("%.2f", value.(float64))).Publish()
					}
				}
			}
		}
	}()

}
