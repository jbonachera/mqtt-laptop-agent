package main

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"time"

	homie "github.com/jbonachera/homie-go/homie"
	"github.com/jbonachera/mqtt-laptop-agent/dafang"
	"github.com/jbonachera/mqtt-laptop-agent/logind"
	"github.com/jbonachera/mqtt-laptop-agent/ota"
	"github.com/jbonachera/mqtt-laptop-agent/upower"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Broadcast struct {
	Level   string
	Payload []byte
}

func main() {
	config := viper.New()
	config.AddConfigPath(configDir())
	config.SetConfigType("yaml")
	config.SetConfigName("config")
	cmd := cobra.Command{
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			config.BindEnv()
			config.BindPFlags(cmd.Flags())
			config.BindPFlags(cmd.PersistentFlags())
			if err := config.ReadInConfig(); err != nil {
				log.Printf("failed to load config: %v", err)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			log.SetPrefix(config.GetString("homie.name"))
			rebootCh := make(chan struct{})

			notificationsProvider := NewNotificationsProvider()
			broadcastCh := make(chan Broadcast, 5)
			device := homie.NewDevice(config.GetString("homie.name"), &homie.Config{
				Mqtt: homie.MqttConfig{
					URL:      config.GetString("mqtt.broker"),
					Username: config.GetString("mqtt.username"),
					Password: config.GetString("mqtt.password"),
					OnConnect: func(device homie.Device) {
						notificationsProvider.Notify("connected")
						ota.NewProvider(device.Topic(""), device.Client(), rebootCh)
						device.SendMessage("$implementation/ota/enabled", "true")
					},
					OnConnectionLost: func(device homie.Device, err error) {
						notificationsProvider.Notify(fmt.Sprintf("connection lost: %v", err))
					},
					OnBroadcast: func(device homie.Device, level string, message []byte) {
						log.Printf("broadcast received: %s <- %s", level, string(message))
						select {
						case broadcastCh <- Broadcast{
							Level:   level,
							Payload: message,
						}:
						default:
						}
					},
				},
				BaseTopic:           "devices/",
				StatsReportInterval: 60,
			})

			notificationsProvider.Register(device)
			logind.NewLogindProvider().Serve(device.NewNode("logind", "logind"))
			upower.NewUpowerProvider().Serve(device.NewNode("upower", "upower"))
			dafangProvider := dafang.NewProvider()
			if dafangProvider.Available() {
				dafangProvider.Serve(device.NewNode("dafang", "dafang"))
			}

			webcam := &webcamProvider{path: config.GetString("webcam-path")}
			webcam.RegisterNode(device)
			for {
				log.Printf("attempting to connect to %s", config.GetString("mqtt.broker"))
				err := device.Connect()
				if err == nil {
					break
				}
				msg := fmt.Sprintf("connection failed: %v", err)
				notificationsProvider.Notify(msg)
				<-time.After(3 * time.Second)
			}
			go func() {
				for broadcast := range broadcastCh {
					if dafangProvider.Available() {
						dafangProvider.Broadcast(broadcast.Level, broadcast.Payload)
					}
				}
			}()
			select {
			case <-rebootCh:
				log.Printf("rebooting")
				err := device.Disconnect()
				if err != nil {
					log.Printf("failed to cleanly disconnect from MQTT - rebooting anyway")
				}
				dafangProvider.Stop()
				log.Printf("calling execve on new firmware")
				syscall.Exec(os.Args[0], os.Args, os.Environ())
			}
		},
	}
	cmd.Flags().String("webcam-path", "/dev/video0", "")
	cmd.Execute()
}
