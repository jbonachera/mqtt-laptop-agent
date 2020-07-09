package logind

import (
	dbus "github.com/godbus/dbus"
	homie "github.com/jbonachera/homie-go/homie"
)

func poweroffProperty(node homie.Node, conn *dbus.Conn) {
	obj := conn.Object("org.freedesktop.login1", "/org/freedesktop/login1")

	poweroff := node.NewProperty("poweroff", "bool")
	poweroff.SetValue("false")

	poweroff.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
		if string(payload) == "true" {
			obj.Call("org.freedesktop.login1.Manager.PowerOff", 0, false).Store(nil)
		}
		return true, nil
	})
}
