// Package source preserves the intro's authored word arithmetic and scene data.
package source

import (
	"encoding/json"
	"fmt"
	"slices"
)

type Scene struct {
	AngularStep [3]int16     `json:"angular_step"`
	Translation [3]int16     `json:"translation"`
	Pivot       [3]int16     `json:"pivot"`
	Wait        bool         `json:"wait"`
	Frames      [][][4]int16 `json:"frames"`
}

type Point struct {
	X, Y, Z  int16
	Material int
}

type Clock struct {
	Tick, SceneTick, Scene, Frame int
	Angles                        [3]int16
	Scenes                        []Scene
	Sine                          []byte
	Points                        []Point
}

func NewClock(objects, sine []byte) (*Clock, error) {
	c := &Clock{Sine: append([]byte(nil), sine...), Points: make([]Point, 0, 128)}
	if err := json.Unmarshal(objects, &c.Scenes); err != nil {
		return nil, err
	}
	if len(c.Scenes) != 14 || len(sine) != 360 {
		return nil, fmt.Errorf("source: incomplete vectorball tables")
	}
	for _, scene := range c.Scenes {
		if len(scene.Frames) == 0 {
			return nil, fmt.Errorf("source: empty vectorball scene")
		}
		for _, frame := range scene.Frames {
			if len(frame) == 0 || len(frame) > cap(c.Points) {
				return nil, fmt.Errorf("source: invalid point population")
			}
			for _, p := range frame {
				if p[3] < 0 || p[3] > 15 {
					return nil, fmt.Errorf("source: invalid sphere material")
				}
			}
		}
	}
	return c, nil
}

func (c *Clock) trig(angle int16) (int16, int16) {
	angle %= 360
	if angle < 0 {
		angle += 360
	}
	other := (90 - int(angle) + 360) % 360
	return int16(int8(c.Sine[angle] - 128)), int16(int8(c.Sine[other] - 128))
}

// rotate retains the 68000's low-word product sum before the seven-bit shift.
func rotate(a, b, cosine, sine int16, subtract bool) (int16, int16) {
	u, v := int32(a)*int32(cosine), int32(b)*int32(sine)
	w, z := int32(b)*int32(cosine), int32(a)*int32(sine)
	if subtract {
		return int16(u-v) >> 7, int16(w+z) >> 7
	}
	return int16(u+v) >> 7, int16(w-z) >> 7
}

func project(value, center, depth int16) int16 {
	n := int32(value - center)
	// LSL.W changes only the low word of the extended numerator.
	n = int32(uint32(n)&0xffff0000 | uint32(uint16(int16(value-center)<<8)))
	if depth == 0 {
		return -32768
	}
	quotient := n / int32(depth)
	if quotient < -32768 || quotient > 32767 {
		return int16(n) + center
	}
	return int16(quotient) + center
}

func (c *Clock) Step() {
	// VBLs continue advancing the music and ribbon during the palette handoff.
	// Geometry stays still for eight darkening and eight brightening frames.
	transition := c.Tick >= 1000 && c.SceneTick < 16
	if transition && c.SceneTick != 8 {
		c.Tick++
		c.SceneTick++
		return
	}
	s := &c.Scenes[c.Scene]
	cy, sy := c.trig(c.Angles[0])
	cx, sx := c.trig(c.Angles[1])
	cz, sz := c.trig(c.Angles[2])
	c.Points = c.Points[:0]
	for _, p := range s.Frames[c.Frame] {
		x, y, z := p[0]+s.Translation[0], p[1]+s.Translation[1], p[2]+s.Translation[2]
		x, z = rotate(x-s.Pivot[0], z-s.Pivot[2], cy, sy, true)
		x += s.Pivot[0]
		z += s.Pivot[2]
		y, z = rotate(y-s.Pivot[1], z-s.Pivot[2], cx, sx, false)
		y += s.Pivot[1]
		z += s.Pivot[2]
		x, y = rotate(x-s.Pivot[0], y-s.Pivot[1], cz, sz, false)
		x += s.Pivot[0]
		y += s.Pivot[1]
		c.Points = append(c.Points, Point{project(x, 160, z+256), project(y, 100, z+256), z, int(p[3])})
	}
	slices.SortStableFunc(c.Points, func(a, b Point) int { return int(b.Z) - int(a.Z) })
	c.Frame = (c.Frame + 1) % len(s.Frames)
	if transition {
		c.Tick++
		c.SceneTick++
		return
	}
	for i := range c.Angles {
		step := i
		if i < 2 {
			step = 1 - i
		}
		c.Angles[i] += s.AngularStep[step]
		if c.Angles[i] >= 359 {
			c.Angles[i] -= 359
		}
		if c.Angles[i] < 0 {
			c.Angles[i] += 359
		}
	}
	c.Tick++
	c.SceneTick++
	if c.SceneTick == 1000 {
		c.Scene = (c.Scene + 1) % len(c.Scenes)
		c.SceneTick = 0
		c.Frame = 0
		c.Angles = [3]int16{}
	}
}
