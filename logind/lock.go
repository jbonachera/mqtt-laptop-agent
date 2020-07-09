package logind

import (
	"fmt"
	"log"

	dbus "github.com/godbus/dbus"
	homie "github.com/jbonachera/homie-go/homie"
)

func lockProperty(node homie.Node, conn *dbus.Conn) {
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
	); err != nil {
		panic(err)
	}
	obj := conn.Object("org.freedesktop.login1", "/org/freedesktop/login1/session/self")

	lock := node.NewProperty("lock", "bool")

	result, err := obj.GetProperty("org.freedesktop.login1.Session.LockedHint")
	if err != nil {
		log.Print(err)
	} else {
		lock.SetValue(fmt.Sprintf("%v", result.Value().(bool)))
	}

	lock.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
		if string(payload) == "true" {
			obj.Call("org.freedesktop.login1.Session.Lock", 0).Store(nil)
		} else {
			obj.Call("org.freedesktop.login1.Session.Unlock", 0).Store(nil)
		}
		return true, nil
	})
	c := make(chan *dbus.Signal, 0)

	conn.Signal(c)

	go func() {
		for event := range c {
			if len(event.Body) >= 2 {
				if v, ok := event.Body[0].(string); ok && v == "org.freedesktop.login1.Session" {
					data := event.Body[1].(map[string]dbus.Variant)
					locked := data["LockedHint"].Value().(bool)
					lockValue := fmt.Sprintf("%v", locked)
					lock.SetValue(lockValue).Publish()
				}
			}
		}
	}()
}
