package source

import (
	"encoding/json"
	"github.com/olivierh59500/go-larcociel-vectorball/assets"
	"os"
	"testing"
)

func nativeClock(t *testing.T) *Clock {
	t.Helper()
	objects, e := assets.Files.ReadFile("original/objects.json")
	if e != nil {
		t.Fatal(e)
	}
	sine, e := assets.Files.ReadFile("original/sine.bin")
	if e != nil {
		t.Fatal(e)
	}
	c, e := NewClock(objects, sine)
	if e != nil {
		t.Fatal(e)
	}
	return c
}

func TestNativePointCheckpoint(t *testing.T) {
	c := nativeClock(t)
	for range 50 {
		c.Step()
	}
	b, e := os.ReadFile("testdata/native-points-50.json")
	if e != nil {
		t.Fatal(e)
	}
	var native [][4]int16
	if e = json.Unmarshal(b, &native); e != nil {
		t.Fatal(e)
	}
	if len(native) != len(c.Points) {
		t.Fatal("native point count differs")
	}
	for i, p := range native {
		want := Point{project(p[0], 160, p[2]+256), project(p[1], 100, p[2]+256), p[2], int(p[3]) / 12}
		if got := c.Points[i]; got != want {
			t.Errorf("point %d: got %+v, native %+v", i, got, want)
		}
	}
	if c.Angles != [3]int16{150, 200, 100} {
		t.Fatalf("native angles differ: %v", c.Angles)
	}
}

func TestCompleteObjectSequence(t *testing.T) {
	c := nativeClock(t)
	for tick := 1; tick <= 28000; tick++ {
		c.Step()
		if c.Tick != tick || c.SceneTick != tick%1000 || c.Scene != (tick/1000)%14 {
			t.Fatalf("transport differs at tick %d", tick)
		}
	}
}
