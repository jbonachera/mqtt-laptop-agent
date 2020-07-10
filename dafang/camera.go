package dafang

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"time"

	homie "github.com/jbonachera/homie-go/homie"
)

func capture() ([]byte, error) {
	for {
		if _, err := os.Stat("/system/sdcard/bin/getimage"); err != nil {
			log.Println(err)
			return nil, err
		}
		cmd := exec.Command("/system/sdcard/bin/getimage")
		out := bytes.NewBuffer(nil)
		cmd.Stdout = out
		err := cmd.Run()
		if err != nil {
			return nil, err
		}
		if len(out.Bytes()) == 0 {
			<-time.After(500 * time.Millisecond)
			continue
		}
		return out.Bytes(), err
	}
}

type webcamProvider struct {
	path string
}

func cameraProperty(externalTrigger chan struct{}, node homie.Node) {
	v, err := capture()
	if err != nil {
		log.Print(err)
		return
	}
	trigger := make(chan struct{}, 1)
	frame := node.NewProperty("frame", "jpeg")
	frame.SetValue(string(v))
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for {
			select {
			case <-ticker.C:
			case <-trigger:
			case <-externalTrigger:
			}
			v, err := capture()
			if err != nil {
				log.Print(err)
				return
			}
			frame.SetValue(string(v)).Publish()
		}
	}()

	frame.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
		select {
		case trigger <- struct{}{}:
		default:
		}
		return true, nil
	})
}
