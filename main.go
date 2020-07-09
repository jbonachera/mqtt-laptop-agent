package main

import (
	"fmt"
	"log"
	"time"

	homie "github.com/jbonachera/homie-go/homie"
	"github.com/jbonachera/mqtt-laptop-agent/logind"
	"github.com/jbonachera/mqtt-laptop-agent/upower"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

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

			notificationsProvider := NewNotificationsProvider()

			device := homie.NewDevice(config.GetString("homie.name"), &homie.Config{
				Mqtt: homie.MqttConfig{
					URL:      config.GetString("mqtt.broker"),
					Username: config.GetString("mqtt.username"),
					Password: config.GetString("mqtt.password"),
					OnConnect: func() {
						notificationsProvider.Notify("connected")
					},
					OnConnectionLost: func(err error) {
						notificationsProvider.Notify(fmt.Sprintf("connection lost: %v", err))
					},
				},
				BaseTopic:           "devices/",
				StatsReportInterval: 60,
			})
			notificationsProvider.Register(device)
			logind.NewLogindProvider().Serve(device.NewNode("logind", "logind"))
			upower.NewUpowerProvider().Serve(device.NewNode("upower", "upower"))

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
			select {}
		},
	}
	cmd.Flags().String("webcam-path", "/dev/video0", "")
	cmd.Execute()
}
