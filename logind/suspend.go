package logind

import (
	"fmt"

	dbus "github.com/godbus/dbus"
	homie "github.com/jbonachera/homie-go/homie"
)

func suspendProperty(node homie.Node, conn *dbus.Conn) {
	obj := conn.Object("org.freedesktop.login1", "/org/freedesktop/login1")

	suspend := node.NewProperty("suspend", "bool")
	suspend.SetValue("false")

	suspend.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
		if string(payload) == "true" {
			obj.Call("org.freedesktop.login1.Manager.Suspend", 0, false).Store(nil)
		}
		return true, nil
	})

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
