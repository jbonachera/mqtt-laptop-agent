package main

import (
	"fmt"
	"log"
	"os"

	dbus "github.com/godbus/dbus"
	homie "github.com/jbonachera/homie-go/homie"
)

func notify(conn *dbus.Conn, message string) {
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	obj.Call(
		"org.freedesktop.Notifications.Notify", 0, "MQTT Agent",
		uint32(0), "", "MQTT Agent", message, []string{}, map[string]interface{}{}, 6000)
}

type notificationsProvider struct {
	conn *dbus.Conn
}

func NewNotificationsProvider() *notificationsProvider {
	sessionBus, err := dbus.ConnectSessionBus()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to connect to session bus:", err)
		return &notificationsProvider{}
	}
	return &notificationsProvider{conn: sessionBus}
}

func (n *notificationsProvider) Register(device homie.Device) {
	if n.conn == nil {
		return
	}
	notifications := device.NewNode("notifications", "Notifications")
	message := notifications.NewProperty("message", "string")
	message.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
		notify(n.conn, string(payload))
		return true, nil
	})
}
func (n *notificationsProvider) Notify(msg string) {
	log.Println(msg)
	if n.conn == nil {
		return
	}
	notify(n.conn, msg)
}
