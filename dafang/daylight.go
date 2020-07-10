package dafang

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jbonachera/homie-go/homie"
)

const (
	DAYLIGHT_SENSOR = "/dev/jz_adc_aux_0"
)

func readDaylight() (uint16, error) {
	fd, err := os.Open(DAYLIGHT_SENSOR)
	if err != nil {
		return 0, errors.New("failed to open light sensor at " + DAYLIGHT_SENSOR)
	}
	defer fd.Close()
	buf := make([]byte, 2)
	_, err = fd.Read(buf)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(buf), nil

}

func toPercent(value uint16) int {
	return int(value) * 100 / 3057
}

func daylightProperty(node homie.Node) {
	v, err := readDaylight()
	if err != nil {
		log.Printf("failed to read dafang daylight: %v", err)
		return
	}
	daylightPercent := node.NewProperty("daylightPercent", "number")
	daylight := node.NewProperty("daylight", "number")
	daylight.SetValue(fmt.Sprintf("%d", v))
	daylightPercent.SetValue(fmt.Sprintf("%d", toPercent(v)))

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			v, err := readDaylight()
			if err != nil {
				log.Printf("failed to read dafang daylight: %v", err)
			} else {
				daylight.SetValue(fmt.Sprintf("%d", v))
				daylightPercent.SetValue(fmt.Sprintf("%d", toPercent(v)))
				if node.Device().Client().IsConnected() {
					daylight.Publish()
					daylightPercent.Publish()
				}
			}
		}
	}()

}
