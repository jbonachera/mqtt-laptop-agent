package main

import (
	"fmt"
	"os"

	dbus "github.com/godbus/dbus"
	homie "github.com/jbonachera/homie-go/homie"
)

type lockProvider struct {
	conn *dbus.Conn
}

func (l *lockProvider) Register(device homie.Device) {
	systemBus, err := dbus.ConnectSystemBus()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to connect to system bus:", err)
		return
	}
	c := make(chan *dbus.Signal, 0)
	if err = systemBus.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
	); err != nil {
		panic(err)
	}
	obj := systemBus.Object("org.freedesktop.login1", "/org/freedesktop/login1")
	lockscreen := device.NewNode("lockscreen", "Lock")
	lock := lockscreen.NewProperty("lock", "bool")
	lock.SetValue("false")

	lock.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
		if string(payload) == "true" {
			obj.Call("org.freedesktop.login1.Manager.LockSessions", 0).Store(nil)
		} else {
			obj.Call("org.freedesktop.login1.Manager.UnlockSessions", 0).Store(nil)
		}
		return true, nil
	})

	systemBus.Signal(c)

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
