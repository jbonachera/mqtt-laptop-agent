package dafang

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
	"unsafe"

	homie "github.com/jbonachera/homie-go/homie"
	"golang.org/x/sys/unix"
)

// https://github.com/Dafang-Hacks/Main/blob/master/motor_app/motor.c
type Command int
type Direction int
type Axis int

const (
	xMax int = 1250
	yMax int = 400
	xMin int = 0
	yMin int = 0
)

type State int

type Status struct {
	X     int
	Y     int
	State State
	Speed int
}

const (
	StopCommand Command = 1 + iota
	ResetCommand
	MoveCommand
	GetStatusCommand
	SpeedCommand
)
const (
	UpDirection Direction = iota
	DownDirection
	LeftDirection
	RightDirection
)

const (
	HorizontalAxis Axis = 1 + iota
	VerticalAxis
)

const InitSpeed = 1000

type Controller struct {
	movement    sync.Mutex
	calibration sync.Mutex
	calibrated  time.Time
	fd          int
	disabled    bool
}

type Movement struct {
	x int
	y int
}
type ResetData struct {
	x_max_steps uint
	y_max_steps uint
	x_cur_step  uint
	y_cur_step  uint
}

func motorProperty(arm, disarm chan struct{}, screenshotTrigger chan struct{}, node homie.Node) io.Closer {
	controller, err := NewController()
	if err != nil {
		log.Printf("failed to start dafang motor: %v", err)
		return nil
	}
	err = controller.Calibrate()
	if err != nil {
		log.Printf("failed to calibrate dafang motor: %v", err)
		return nil
	}
	status, err := controller.Status()
	if err != nil {
		log.Printf("failed to read dafang motor status: %v", err)
		return nil
	}
	xAxis := node.NewProperty("x_axis", "number").SetValue(fmt.Sprintf("%d", status.X))
	yAxis := node.NewProperty("y_axis", "number").SetValue(fmt.Sprintf("%d", status.Y))
	xAxisIncr := node.NewProperty("x_axis_incr", "number").SetValue(fmt.Sprintf("%d", status.X))
	yAxisIncr := node.NewProperty("y_axis_incr", "number").SetValue(fmt.Sprintf("%d", status.Y))
	node.NewProperty("x_axis_max", "number").SetValue(fmt.Sprintf("%d", xMax))
	node.NewProperty("y_axis_max", "number").SetValue(fmt.Sprintf("%d", yMax))
	node.NewProperty("x_axis_min", "number").SetValue(fmt.Sprintf("%d", xMin))
	node.NewProperty("y_axis_min", "number").SetValue(fmt.Sprintf("%d", yMin))

	xAxis.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
		parsed, err := strconv.ParseInt(string(payload), 10, 32)
		if err != nil {
			return false, err
		}
		step := int(parsed)
		err = controller.SetX(step)
		if err != nil {
			log.Printf("failed to move dafang motor: %v", err)
			return false, err
		}
		status, err := controller.Status()
		if err != nil {
			log.Printf("failed to read dafang motor status: %v", err)
			return false, err
		}
		xAxis.SetValue(fmt.Sprintf("%d", status.X)).Publish()
		xAxisIncr.SetValue(fmt.Sprintf("%d", status.X)).Publish()
		select {
		case screenshotTrigger <- struct{}{}:
		default:
		}
		return true, nil
	})

	xAxisIncr.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
		parsed, err := strconv.ParseInt(string(payload), 10, 32)
		if err != nil {
			return false, err
		}
		step := int(parsed)
		err = controller.IncrX(step)
		if err != nil {
			log.Printf("failed to move dafang motor: %v", err)
			return false, err
		}
		status, err := controller.Status()
		if err != nil {
			log.Printf("failed to read dafang motor status: %v", err)
			return false, err
		}
		xAxis.SetValue(fmt.Sprintf("%d", status.X)).Publish()
		xAxisIncr.SetValue(fmt.Sprintf("%d", status.X)).Publish()
		select {
		case screenshotTrigger <- struct{}{}:
		default:
		}
		return true, nil
	})

	yAxis.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
		parsed, err := strconv.ParseInt(string(payload), 10, 32)
		if err != nil {
			return false, err
		}
		step := int(parsed)
		err = controller.SetY(step)
		if err != nil {
			log.Printf("failed to move dafang motor: %v", err)
			return false, err
		}
		status, err := controller.Status()
		if err != nil {
			log.Printf("failed to read dafang motor status: %v", err)
			return false, err
		}
		yAxis.SetValue(fmt.Sprintf("%d", status.Y)).Publish()
		select {
		case screenshotTrigger <- struct{}{}:
		default:
		}
		return true, nil
	})
	yAxisIncr.SetHandler(func(p homie.Property, payload []byte, topic string) (bool, error) {
		parsed, err := strconv.ParseInt(string(payload), 10, 32)
		if err != nil {
			return false, err
		}
		step := int(parsed)
		err = controller.IncrY(step)
		if err != nil {
			log.Printf("failed to move dafang motor: %v", err)
			return false, err
		}
		status, err := controller.Status()
		if err != nil {
			log.Printf("failed to read dafang motor status: %v", err)
			return false, err
		}
		yAxis.SetValue(fmt.Sprintf("%d", status.Y)).Publish()
		yAxisIncr.SetValue(fmt.Sprintf("%d", status.Y)).Publish()
		select {
		case screenshotTrigger <- struct{}{}:
		default:
		}
		return true, nil
	})
	go func() {
		for {
			select {
			case <-disarm:
				controller.goTo(0, 0)
				controller.disabled = true
			case <-arm:
				controller.disabled = false
				controller.Calibrate()
			}
			select {
			case screenshotTrigger <- struct{}{}:
			default:
			}
		}
	}()
	log.Printf("dafang motor initialized")
	return controller
}

func NewController() (*Controller, error) {
	fd, err := unix.Open("/dev/motor", os.O_WRONLY, 0660)
	if err != nil {
		return nil, fmt.Errorf("failed to open motor control: %v", err)
	}
	c := &Controller{fd: fd}

	return c, c.Speed(1200)
}
func (controller *Controller) Close() error {
	return unix.Close(controller.fd)
}
func (controller *Controller) move(direction Direction, steps int) error {
	x := 0
	y := 0
	switch direction {
	case LeftDirection:
		x = -steps
	case RightDirection:
		x = steps
	case DownDirection:
		y = -steps
	case UpDirection:
		y = steps
	}

	cmd := Movement{
		x: x,
		y: y,
	}
	err := controller.sendCommand(MoveCommand, unsafe.Pointer(&cmd))
	if err != nil {
		return err
	}
	return controller.wait()
}
func (controller *Controller) goTo(x, y int) error {
	status, err := controller.status()
	if err != nil {
		return err
	}
	movement := Movement{
		x: x - status.X,
		y: y - status.Y,
	}
	controller.sendCommand(MoveCommand, unsafe.Pointer(&movement))
	return controller.wait()
}

func (controller *Controller) goToAxis(target int, current int, incFunc, decFunc func(int) error) error {

	steps := target - current
	switch {
	case steps > 0:
		return incFunc(steps)
	case steps < 0:
		return decFunc(-steps)
	default:
		return nil
	}
}

func (controller *Controller) Center() error {
	return controller.goTo(xMax/2, yMax/2)
}
func (controller *Controller) Stop() error {
	return controller.sendCommand(StopCommand, unsafe.Pointer(nil))
}
func (controller *Controller) Reset() error {
	reset := &ResetData{}
	return controller.sendCommand(ResetCommand, unsafe.Pointer(reset))
}
func (controller *Controller) up(steps int) error {
	log.Printf("motor: %d steps up", steps)
	return controller.move(UpDirection, steps)
}

func (controller *Controller) down(steps int) error {
	log.Printf("motor: %d steps down", steps)
	return controller.move(DownDirection, steps)
}

func (controller *Controller) right(steps int) error {
	log.Printf("motor: %d steps right", steps)
	return controller.move(RightDirection, steps)
}
func (controller *Controller) left(steps int) error {
	log.Printf("motor: %d steps left", steps)
	return controller.move(LeftDirection, steps)
}

func (controller *Controller) wait() error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := controller.status()
		if err != nil {
			return err
		}
		if current.State == 0 {
			return nil
		}
		<-ticker.C
	}
}
func (controller *Controller) Status() (Status, error) {
	controller.calibration.Lock()
	defer controller.calibration.Unlock()
	return controller.status()
}
func (controller *Controller) status() (Status, error) {
	status := Status{}
	err := controller.sendCommand(GetStatusCommand, unsafe.Pointer(&status))
	if err != nil {
		return status, err
	}
	return status, nil
}
func (controller *Controller) SetX(target int) error {
	controller.movement.Lock()
	defer controller.movement.Unlock()

	status, err := controller.status()
	if err != nil {
		return err
	}
	return controller.goToAxis(target, status.X, controller.right, controller.left)
}
func (controller *Controller) IncrX(steps int) error {
	controller.movement.Lock()
	defer controller.movement.Unlock()
	switch {
	case steps < 0:
		return controller.left(-steps)
	case steps > 0:
		return controller.right(steps)
	default:
		return nil
	}
}
func (controller *Controller) IncrY(steps int) error {
	controller.movement.Lock()
	defer controller.movement.Unlock()
	switch {
	case steps < 0:
		return controller.down(-steps)
	case steps > 0:
		return controller.up(steps)
	default:
		return nil
	}
}
func (controller *Controller) SetY(target int) error {
	controller.movement.Lock()
	defer controller.movement.Unlock()

	status, err := controller.status()
	if err != nil {
		return err
	}
	return controller.goToAxis(target, status.Y, controller.up, controller.down)
}

func (controller *Controller) Speed(speed int) error {
	_speed := speed
	return controller.sendCommand(SpeedCommand, unsafe.Pointer(&_speed))
}
func (controller *Controller) Calibrate() error {
	controller.calibration.Lock()
	defer controller.calibration.Unlock()
	controller.Reset()
	controller.calibrated = time.Now()
	return nil
}

func (controller *Controller) sendCommand(cmd Command, payload unsafe.Pointer) error {
	if controller.disabled {
		return errors.New("controller disabled")
	}
	_, _, errorp := unix.Syscall(unix.SYS_IOCTL,
		uintptr(controller.fd),
		uintptr(int(cmd)),
		uintptr(payload))

	if errorp.Error() != "errno 0" {
		return fmt.Errorf("ioctl returned: %v", errorp)
	}
	return nil
}
