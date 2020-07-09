package logind

import (
	"fmt"
	"os"

	dbus "github.com/godbus/dbus"
	homie "github.com/jbonachera/homie-go/homie"
)

type logindProvider struct {
}

func NewLogindProvider() *logindProvider {
	return &logindProvider{}
}

func (l *logindProvider) Serve(node homie.Node) {
	systemBus, err := dbus.ConnectSystemBus()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to connect to system bus:", err)
		return
	}

	lockProperty(node, systemBus)
	suspendProperty(node, systemBus)
	poweroffProperty(node, systemBus)
}
