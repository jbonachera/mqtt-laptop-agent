package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"
	"sort"
	"time"

	"github.com/blackjack/webcam"
	dbus "github.com/godbus/dbus"
	homie "github.com/jbonachera/homie-go/homie"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	V4L2_PIX_FMT_PJPG = 0x47504A50
	V4L2_PIX_FMT_YUYV = 0x56595559
)

type FrameSizes []webcam.FrameSize

func (slice FrameSizes) Len() int {
	return len(slice)
}

//For sorting purposes
func (slice FrameSizes) Less(i, j int) bool {
	ls := slice[i].MaxWidth * slice[i].MaxHeight
	rs := slice[j].MaxWidth * slice[j].MaxHeight
	return ls < rs
}

//For sorting purposes
func (slice FrameSizes) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

var supportedFormats = map[webcam.PixelFormat]bool{
	V4L2_PIX_FMT_PJPG: true,
	V4L2_PIX_FMT_YUYV: true,
}

func Capturer(cb func([]byte)) {
	cam, err := webcam.Open("/dev/video0") // Open webcam
	if err != nil {
		log.Fatal("failed to open webcam", err)
	}
	defer cam.Close()
	frames := FrameSizes(cam.GetSupportedFrameSizes(V4L2_PIX_FMT_YUYV))
	sort.Sort(frames)
	var size *webcam.FrameSize
	size = &frames[len(frames)-1]
	if size == nil {
		log.Fatal("No matching frame size, exiting")
		return
	}

	_, w, h, err := cam.SetImageFormat(V4L2_PIX_FMT_YUYV, uint32(size.MaxWidth), uint32(size.MaxHeight))
	if err != nil {
		log.Fatal("SetImageFormat return error, ", err)
		return

	}
	err = cam.StartStreaming()
	for {
		err = cam.WaitForFrame(5000)

		switch err.(type) {
		case nil:
		case *webcam.Timeout:
			continue
		default:
			panic(err.Error())
		}

		frame, err := cam.ReadFrame()
		if len(frame) != 0 {
			yuyv := image.NewYCbCr(image.Rect(0, 0, int(w), int(h)), image.YCbCrSubsampleRatio422)
			for i := range yuyv.Cb {
				ii := i * 4
				yuyv.Y[i*2] = frame[ii]
				yuyv.Y[i*2+1] = frame[ii+2]
				yuyv.Cb[i] = frame[ii+1]
				yuyv.Cr[i] = frame[ii+3]

			}
			buf := &bytes.Buffer{}
			if err := jpeg.Encode(buf, yuyv, &jpeg.Options{Quality: 80}); err != nil {
				log.Fatal(err)
				return
			}
			cb(buf.Bytes())
		} else if err != nil {
			panic(err.Error())
		}
		return
	}
}

func notify(conn *dbus.Conn, message string) {
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	obj.Call(
		"org.freedesktop.Notifications.Notify", 0, "MQTT Agent",
		uint32(0), "", "MQTT Agent", message, []string{}, map[string]interface{}{}, 6000)
}

func main() {
	config := viper.New()
	config.AddConfigPath(configDir())
	config.SetConfigType("yaml")
	config.SetConfigName("config")
	cmd := cobra.Command{
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			config.BindPFlags(cmd.Flags())
			config.BindPFlags(cmd.PersistentFlags())
			if err := config.ReadInConfig(); err != nil {
				if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
					log.Fatal(err)
				}
			}
		},
		Run: func(cmd *cobra.Command, args []string) {

			sessionBus, err := dbus.ConnectSessionBus()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to connect to session bus:", err)
				os.Exit(1)
			}
			defer sessionBus.Close()
			systemBus, err := dbus.ConnectSystemBus()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to connect to session bus:", err)
				os.Exit(1)
			}
			defer systemBus.Close()

			if err = systemBus.AddMatchSignal(
				dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
			); err != nil {
				panic(err)
			}

			c := make(chan *dbus.Signal, 0)

			device := homie.NewDevice(config.GetString("homie.name"), &homie.Config{
				Mqtt: homie.MqttConfig{
					URL:      config.GetString("mqtt.broker"),
					Username: config.GetString("mqtt.username"),
					Password: config.GetString("mqtt.password"),
				},
				BaseTopic:           "devices/",
				StatsReportInterval: 60,
			})

			lockscreen := device.NewNode("lockscreen", "Lock")
			lock := lockscreen.NewProperty("lock", "bool")
			lock.SetValue("false")

			obj := systemBus.Object("org.freedesktop.login1", "/org/freedesktop/login1")

			lock.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
				if string(payload) == "true" {
					notify(sessionBus, "Screen locking requested via MQTT")
					obj.Call("org.freedesktop.login1.Manager.LockSessions", 0).Store(nil)
				} else {
					notify(sessionBus, "Screen unlocking requested via MQTT")
					obj.Call("org.freedesktop.login1.Manager.UnlockSessions", 0).Store(nil)
				}
				return true, nil
			})

			notifications := device.NewNode("notifications", "Notifications")
			message := notifications.NewProperty("message", "string")
			message.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
				notify(sessionBus, string(payload))
				return true, nil
			})

			systemBus.Signal(c)

			webcam := device.NewNode("webcam", "camera")
			trigger := make(chan struct{}, 1)
			frame := webcam.NewProperty("frame", "jpeg")
			Capturer(func(b []byte) {
				frame.SetValue(string(b))
			})
			go func() {
				ticker := time.NewTicker(1 * time.Hour)
				for {
					select {
					case <-ticker.C:
					case <-trigger:
					}
					Capturer(func(b []byte) {
						frame.SetValue(string(b)).Publish()
					})
				}
			}()

			frame.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
				select {
				case trigger <- struct{}{}:
				default:
				}
				return true, nil
			})
			notify(sessionBus, "mqtt agent started")
			device.Run(false)
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
			select {}
		},
	}
	cmd.Execute()
}
