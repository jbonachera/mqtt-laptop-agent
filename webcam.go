package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"log"
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

type webcamProvider struct{}

func (w *webcamProvider) RegisterNode(device homie.Device) {
	node := device.NewNode("webcam", "camera")
	trigger := make(chan struct{}, 1)
	frame := node.NewProperty("frame", "jpeg")
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
}
