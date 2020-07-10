package dafang

import (
	"io"
	"os"

	homie "github.com/jbonachera/homie-go/homie"
)

type dafangProvider struct {
	armCh       chan struct{}
	disarmCh    chan struct{}
	motorCloser io.Closer
}

func NewProvider() *dafangProvider {
	return &dafangProvider{
		armCh:    make(chan struct{}),
		disarmCh: make(chan struct{}),
	}
}
func (l *dafangProvider) Stop() {
	if l.motorCloser != nil {
		l.motorCloser.Close()
	}
}
func (l *dafangProvider) Broadcast(level string, payload []byte) {
	switch level {
	case "alarm":
		switch string(payload) {
		case "arm":
			select {
			case l.armCh <- struct{}{}:
			default:
			}
		case "disarm":
			select {
			case l.disarmCh <- struct{}{}:
			default:
			}
		}
	}
}
func (l *dafangProvider) Available() bool {
	if _, err := os.Stat("/system/sdcard"); err != nil {
		return false
	}
	return true
}
func (l *dafangProvider) Serve(node homie.Node) {
	trigger := make(chan struct{}, 1)
	daylightProperty(node)
	cameraProperty(trigger, node)
	l.motorCloser = motorProperty(l.armCh, l.disarmCh, trigger, node)
}
