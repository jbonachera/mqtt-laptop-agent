package dafang

import (
	"io"
	"os"
	"sync"

	homie "github.com/jbonachera/homie-go/homie"
)

type dafangProvider struct {
	armCh          chan struct{}
	disarmCh       chan struct{}
	cameraArmCh    chan struct{}
	cameraDisarmCh chan struct{}
	motorCloser    io.Closer
	mtx            sync.Mutex
}

func NewProvider() *dafangProvider {
	return &dafangProvider{
		armCh:          make(chan struct{}, 1),
		disarmCh:       make(chan struct{}, 1),
		cameraArmCh:    make(chan struct{}, 1),
		cameraDisarmCh: make(chan struct{}, 1),
	}
}
func (l *dafangProvider) Stop() {
	if l.motorCloser != nil {
		l.motorCloser.Close()
	}
}
func (l *dafangProvider) Broadcast(level string, payload []byte) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	switch level {
	case "alarm":
		switch string(payload) {
		case "arm":
			select {
			case l.armCh <- struct{}{}:
			default:
			}
			select {
			case l.cameraArmCh <- struct{}{}:
			default:
			}
		case "disarm":
			select {
			case l.disarmCh <- struct{}{}:
			default:
			}
			select {
			case l.cameraDisarmCh <- struct{}{}:
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
	cameraProperty(l.cameraArmCh, l.cameraDisarmCh, trigger, node)
	l.motorCloser = motorProperty(l.armCh, l.disarmCh, trigger, node)
}
