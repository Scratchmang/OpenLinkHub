package keyboards

// Package: keyboards
// Feature: Reactive Fade
//
// Reactive Fade is a keyboard-agnostic RGB effect. It reuses each keyboard
// driver's existing HID state listener and only asks the driver to render a
// complete per-key RGB frame when a key is actively fading.

import (
	"OpenLinkHub/src/rgb"
	"math/big"
	"sync"
	"time"
)

const reactiveFadeDefaultDuration uint32 = 500

// ReactiveFadeConfig contains the persistent Reactive Fade settings for one
// keyboard profile.
type ReactiveFadeConfig struct {
	PressColor rgb.Color
	Duration   uint32
	Keys       []int
}

// ReactiveFadeRenderer receives a complete per-key RGB frame. The key index
// is the same index used by the keyboard WebUI.
type ReactiveFadeRenderer func(map[int]rgb.Color)

type reactiveFadeController struct {
	mu       sync.Mutex
	keyboard *Keyboard
	config   ReactiveFadeConfig
	active   map[int]time.Time
	stop     chan struct{}
	done     chan struct{}
	renderer ReactiveFadeRenderer
}

var reactiveFadeControllers = struct {
	sync.Mutex
	items map[string]*reactiveFadeController
}{
	items: make(map[string]*reactiveFadeController),
}

// StartReactiveFade starts Reactive Fade for one physical keyboard. The base
// keyboard colors are rendered once immediately; no USB writes occur while
// the effect is idle.
func StartReactiveFade(serial string, keyboard *Keyboard, config ReactiveFadeConfig, brightness float64, renderer ReactiveFadeRenderer) {
	if serial == "" || keyboard == nil || renderer == nil {
		return
	}

	StopReactiveFade(serial)

	if config.Duration == 0 {
		config.Duration = reactiveFadeDefaultDuration
	}
	if config.PressColor == (rgb.Color{}) {
		config.PressColor = rgb.Color{Red: 255, Green: 255, Blue: 255}
	}
	config.Keys = append([]int(nil), config.Keys...)

	controller := &reactiveFadeController{
		keyboard: keyboard,
		config:   config,
		active:   make(map[int]time.Time),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		renderer: renderer,
	}

	reactiveFadeControllers.Lock()
	reactiveFadeControllers.items[serial] = controller
	reactiveFadeControllers.Unlock()

	renderer(controller.baseColors(brightness))
	go controller.loop(brightness)
}

// StopReactiveFade stops the renderer and clears all transient key state.
func StopReactiveFade(serial string) {
	reactiveFadeControllers.Lock()
	controller, ok := reactiveFadeControllers.items[serial]
	if ok {
		delete(reactiveFadeControllers.items, serial)
	}
	reactiveFadeControllers.Unlock()

	if !ok {
		return
	}

	controller.mu.Lock()
	select {
	case <-controller.stop:
		controller.mu.Unlock()
		<-controller.done
		return
	default:
		close(controller.stop)
		controller.active = make(map[int]time.Time)
	}
	controller.mu.Unlock()

	<-controller.done
}

// UpdateReactiveFade feeds the normalized HID key bitmap into the controller.
// Only newly pressed bits start a fade, so holding a key does not restart it.
func UpdateReactiveFade(serial string, previous, current *big.Int) {
	reactiveFadeControllers.Lock()
	controller := reactiveFadeControllers.items[serial]
	reactiveFadeControllers.Unlock()

	if controller == nil || current == nil {
		return
	}

	controller.mu.Lock()
	defer controller.mu.Unlock()

	if previous == nil {
		previous = new(big.Int)
	}

	pressed := new(big.Int).Xor(previous, current)
	pressed.And(pressed, current)
	if pressed.Sign() == 0 {
		return
	}

	now := time.Now()
	for _, row := range controller.keyboard.Row {
		for keyIndex, key := range row.Keys {
			if !reactiveFadeKeySelected(controller.config.Keys, keyIndex) {
				continue
			}

			for _, hash := range key.KeyHash {
				mask := new(big.Int)
				if _, ok := mask.SetString(hash, 10); !ok || mask.Sign() == 0 {
					continue
				}

				matched := new(big.Int).And(new(big.Int).Set(pressed), mask)
				if matched.Cmp(mask) == 0 {
					controller.active[keyIndex] = now
					break
				}
			}
		}
	}
}

func (c *reactiveFadeController) loop(brightness float64) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	defer close(c.done)

	for {
		select {
		case <-c.stop:
			return
		case now := <-ticker.C:
			c.mu.Lock()
			if len(c.active) == 0 {
				c.mu.Unlock()
				continue
			}

			colors := c.colors(now, brightness)
			duration := time.Duration(c.config.Duration) * time.Millisecond
			for keyIndex, started := range c.active {
				if now.Sub(started) >= duration {
					delete(c.active, keyIndex)
				}
			}
			c.mu.Unlock()

			c.renderer(colors)
		}
	}
}

func (c *reactiveFadeController) baseColors(brightness float64) map[int]rgb.Color {
	colors := make(map[int]rgb.Color)
	for _, row := range c.keyboard.Row {
		for keyIndex, key := range row.Keys {
			colors[keyIndex] = applyReactiveFadeBrightness(key.Color, brightness)
		}
	}
	return colors
}

func (c *reactiveFadeController) colors(now time.Time, brightness float64) map[int]rgb.Color {
	colors := c.baseColors(brightness)
	duration := time.Duration(c.config.Duration) * time.Millisecond

	for _, row := range c.keyboard.Row {
		for keyIndex, key := range row.Keys {
			started, ok := c.active[keyIndex]
			if !ok {
				continue
			}

			progress := float64(now.Sub(started)) / float64(duration)
			if progress < 0 {
				progress = 0
			}
			if progress > 1 {
				progress = 1
			}

			colors[keyIndex] = applyReactiveFadeBrightness(rgb.Color{
				Red:   lerpReactiveFade(c.config.PressColor.Red, key.Color.Red, progress),
				Green: lerpReactiveFade(c.config.PressColor.Green, key.Color.Green, progress),
				Blue:  lerpReactiveFade(c.config.PressColor.Blue, key.Color.Blue, progress),
			}, brightness)
		}
	}

	return colors
}

func reactiveFadeKeySelected(keys []int, keyIndex int) bool {
	if len(keys) == 0 {
		return true
	}
	for _, selected := range keys {
		if selected == keyIndex {
			return true
		}
	}
	return false
}

func applyReactiveFadeBrightness(color rgb.Color, brightness float64) rgb.Color {
	color.Brightness = brightness
	return *rgb.ModifyBrightness(color)
}

func lerpReactiveFade(from, to, progress float64) float64 {
	return from + ((to - from) * progress)
}

// BuildReactiveFadeInterleavedFrame builds the RGB packet used by the majority
// of Corsair per-key keyboard drivers. offset=1 is normal RGBRGB ordering;
// offset=colorPacketLength/3 is used by drivers with planar channel offsets.
func BuildReactiveFadeInterleavedFrame(keyboard *Keyboard, colors map[int]rgb.Color, size, offset int) []byte {
	buffer := make([]byte, size)
	if keyboard == nil || offset <= 0 {
		return buffer
	}

	for _, row := range keyboard.Row {
		for keyIndex, key := range row.Keys {
			if key.NoColor {
				continue
			}
			color, ok := colors[keyIndex]
			if !ok {
				color = key.Color
			}
			for _, packetIndex := range key.PacketIndex {
				if packetIndex < 0 || packetIndex+2*offset >= len(buffer) {
					continue
				}
				buffer[packetIndex] = byte(color.Red)
				buffer[packetIndex+offset] = byte(color.Green)
				buffer[packetIndex+(2*offset)] = byte(color.Blue)
			}
		}
	}
	return buffer
}

// BuildReactiveFadePlanarFrame builds three independent R/G/B buffers.
func BuildReactiveFadePlanarFrame(keyboard *Keyboard, colors map[int]rgb.Color, size int) ([]byte, []byte, []byte) {
	red := make([]byte, size)
	green := make([]byte, size)
	blue := make([]byte, size)
	if keyboard == nil {
		return red, green, blue
	}

	for _, row := range keyboard.Row {
		for keyIndex, key := range row.Keys {
			if key.NoColor {
				continue
			}
			color, ok := colors[keyIndex]
			if !ok {
				color = key.Color
			}
			for _, packetIndex := range key.PacketIndex {
				if packetIndex < 0 || packetIndex >= size {
					continue
				}
				red[packetIndex] = byte(color.Red)
				green[packetIndex] = byte(color.Green)
				blue[packetIndex] = byte(color.Blue)
			}
		}
	}
	return red, green, blue
}

// BuildReactiveFadeMapFrame builds the three-channel map used by K95-class
// devices.
func BuildReactiveFadeMapFrame(keyboard *Keyboard, colors map[int]rgb.Color, size int) map[int][]byte {
	frame := map[int][]byte{
		0: make([]byte, size),
		1: make([]byte, size),
		2: make([]byte, size),
	}
	if keyboard == nil {
		return frame
	}

	for _, row := range keyboard.Row {
		for keyIndex, key := range row.Keys {
			if key.NoColor {
				continue
			}
			color, ok := colors[keyIndex]
			if !ok {
				color = key.Color
			}
			for _, packetIndex := range key.PacketIndex {
				if packetIndex < 0 || packetIndex >= size {
					continue
				}
				frame[0][packetIndex] = byte(color.Red)
				frame[1][packetIndex] = byte(color.Green)
				frame[2][packetIndex] = byte(color.Blue)
			}
		}
	}
	return frame
}
