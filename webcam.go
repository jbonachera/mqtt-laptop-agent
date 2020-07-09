package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/blackjack/webcam"
	homie "github.com/jbonachera/homie-go/homie"
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

func (provider *webcamProvider) Capturer(limit int, cb func([]byte)) error {
	cam, err := webcam.Open(provider.path) // Open webcam
	if err != nil {
		if _, err := os.Stat("/system/sdcard/bin/getimage"); err == nil {
			cmd := exec.Command("/system/sdcard/bin/getimage")
			out := bytes.NewBuffer(nil)
			cmd.Stdout = out
			if err := cmd.Run(); err == nil {
				cb(out.Bytes())
				return nil
			}
			log.Println(err)
		} else {
			log.Println(err)
		}
		return fmt.Errorf("failed to open webcam: %v", err)
	}
	defer cam.Close()
	frames := FrameSizes(cam.GetSupportedFrameSizes(V4L2_PIX_FMT_YUYV))
	sort.Sort(frames)
	var size *webcam.FrameSize
	size = &frames[len(frames)-1]
	if size == nil {
		return errors.New("No matching frame size, exiting")
	}

	_, w, h, err := cam.SetImageFormat(V4L2_PIX_FMT_YUYV, uint32(size.MaxWidth), uint32(size.MaxHeight))
	if err != nil {
		return errors.New("SetImageFormat return error")
	}
	err = cam.StartStreaming()
	count := 0
	for {
		count++
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
			if count < 3 {
				continue
			}
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
				return err
			}
			cb(buf.Bytes())
		} else if err != nil {
			return err
		}
		if limit > 0 && limit+3 >= count {
			return nil
		}
	}
}

type webcamProvider struct {
	path string
}

func (w *webcamProvider) RegisterNode(device homie.Device) {
	var v string
	err := w.Capturer(1, func(b []byte) {
		v = string(b)
	})
	if err != nil {
		log.Print(err)
		return
	}
	node := device.NewNode("webcam", "camera")
	trigger := make(chan struct{}, 1)
	frame := node.NewProperty("frame", "jpeg")
	frame.SetValue(v)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for {
			select {
			case <-ticker.C:
			case <-trigger:
			}
			w.Capturer(1, func(b []byte) {
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
}
