package main

import (
	"fmt"
	"log"
	"time"

	homie "github.com/jbonachera/homie-go/homie"
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
				if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
					log.Printf("failed to load config: %v", err)
				}
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

			webcam := &webcamProvider{}
			webcam.RegisterNode(device)
			for {
				err := device.Connect()
				if err == nil {
					break
				}
				notificationsProvider.Notify(fmt.Sprintf("connection failed: %v", err))
				<-time.After(3 * time.Second)
			}
			select {}
		},
	}
	cmd.Execute()
}
