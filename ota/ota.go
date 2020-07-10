package ota

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MqttClient interface {
	Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token
	Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token
}

var (
	errChecksumMismatch = errors.New("checksum mismatch")
)

type otaState int

const (
	initializing otaState = iota
	readyState
	writingState
	rebootingState
)

type provider struct {
	mtx      sync.Mutex
	state    otaState
	checksum string
}

func fingerprintFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)[:16]), nil

}
func (p *provider) runUpdate(recvChecksum string, message mqtt.Message) error {
	p.state = writingState
	file, err := ioutil.TempFile("", "ota.*.homie")
	if err != nil {
		return fmt.Errorf("failed to create temp file to receive ota update: %v", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()

	reader, err := gzip.NewReader(bytes.NewReader(message.Payload()))
	if err != nil {
		return fmt.Errorf("failed to start gzip reader: %v", err)
	}
	defer reader.Close()
	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to write ota update: %v", err)
	}
	hash, err := fingerprintFile(file.Name())
	if err != nil {
		return fmt.Errorf("failed to fingerprint ota update: %v", err)
	}
	if hash != recvChecksum {
		log.Printf("recv checksum: %s", recvChecksum)
		return errChecksumMismatch
	}
	selfPath := os.Args[0]
	seflStat, err := os.Stat(selfPath)
	if err != nil {
		return fmt.Errorf("failed to stat current firmware: %v", err)
	}
	err = os.Remove(selfPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete current firmware: %v", err)
	}
	dest, err := os.Create(selfPath)
	defer dest.Close()
	if err != nil {
		return fmt.Errorf("failed to create new firmware: %v", err)
	}
	err = os.Chmod(dest.Name(), seflStat.Mode())
	if err != nil {
		return fmt.Errorf("failed to change new firmware permissions: %v", err)
	}
	file.Seek(0, io.SeekStart)
	_, err = io.Copy(dest, file)
	if err != nil {
		return fmt.Errorf("failed to move new firmware: %v", err)
	}
	return nil
}

func NewProvider(baseTopic string, client MqttClient, rebootCh chan struct{}) *provider {
	p := &provider{}

	selfHash, err := fingerprintFile(os.Args[0])
	if err == nil {
		log.Printf("starting OTA receiver with version %s", selfHash)
		p.checksum = selfHash
	}
	prefix := fmt.Sprintf("%s$implementation/ota/", baseTopic)
	publishStatus := func(status string) {
		client.Publish(fmt.Sprintf("%sstatus", prefix), 1, true, status).Wait()
	}
	client.Subscribe(fmt.Sprintf("%sfirmware/+", prefix), 1, func(client mqtt.Client, message mqtt.Message) {
		p.mtx.Lock()
		defer p.mtx.Unlock()
		if message.Retained() || p.state != readyState {
			log.Print("refusing to treat OTA update: state is not ready or message is retained")
			return
		}
		checksum := strings.TrimPrefix(message.Topic(), fmt.Sprintf("%sfirmware/", prefix))
		if checksum == p.checksum {
			log.Print("refusing to treat OTA update: firmware is up to date with request")
			return
		}
		log.Print("starting OTA Update")
		err := p.runUpdate(checksum, message)
		if err != nil {
			log.Printf("OTA Update failed: %v", err)
			p.state = readyState
			if err == errChecksumMismatch {
				publishStatus("400 BAD_CHECKSUM")
			} else {
				publishStatus(fmt.Sprintf("500 %v", err))
			}
			return
		}
		log.Print("OTA Update succeeded")
		p.state = rebootingState
		publishStatus("200")
		close(rebootCh)
	})
	p.state = readyState
	return p
}
