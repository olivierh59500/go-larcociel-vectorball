// Package demo assembles the original vectorball scene with DCK renderers.
package demo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/png"
	"io"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/olivierh59500/democonstructionkit/sound"
	playback "github.com/olivierh59500/democonstructionkit/sound/ebiten"
	"github.com/olivierh59500/democonstructionkit/sprites"
	"github.com/olivierh59500/go-larcociel-vectorball/assets"
	"github.com/olivierh59500/go-larcociel-vectorball/internal/source"
)

const Width, Height, FPS = 320, 200, 50

type Game struct {
	clock                      *source.Clock
	images                     []*ebiten.Image
	balls                      *sprites.ImageSlots
	slots                      [128]sprites.ImageSlot
	stage, background, font    *ebiten.Image
	text                       [2]*ebiten.Image
	front, letter, column, row int
	message                    []byte
	player                     *playback.Player
	visual                     *sound.Stream
	pcm                        [960 * 8]byte
	shader                     *ebiten.Shader
	palette                    [64]float32
	paletteBanks               [2][16]uint16
	gold                       [64]float32
	raster0                    [17 * 4]float32
	raster8                    [67 * 4]float32
	uniforms                   map[string]any
	closed                     bool
}

func resource(name string) ([]byte, error) { return assets.Files.ReadFile("original/" + name) }
func putRGB(dst []float32, w uint16) {
	dst[0] = float32(w>>8&7) * 34 / 255
	dst[1] = float32(w>>4&7) * 34 / 255
	dst[2] = float32(w&7) * 34 / 255
	dst[3] = 1
}
func NewGame(mute bool) (_ *Game, err error) {
	g := &Game{row: 14}
	defer func() {
		if err != nil {
			g.Close()
		}
	}()
	objects, err := resource("objects.json")
	if err != nil {
		return nil, err
	}
	sine, err := resource("sine.bin")
	if err != nil {
		return nil, err
	}
	g.clock, err = source.NewClock(objects, sine)
	if err != nil {
		return nil, err
	}
	load := func(name string) (*ebiten.Image, error) {
		b, e := resource(name)
		if e != nil {
			return nil, e
		}
		im, _, e := image.Decode(bytes.NewReader(b))
		if e != nil {
			return nil, e
		}
		texture := ebiten.NewImageFromImage(im)
		g.images = append(g.images, texture)
		return texture, nil
	}
	for i := 0; i < 16; i++ {
		_, err = load(fmt.Sprintf("ball-%d.png", i))
		if err != nil {
			return nil, err
		}
	}
	g.background, err = load("background.png")
	if err != nil {
		return nil, err
	}
	g.font, err = load("font.png")
	if err != nil {
		return nil, err
	}
	g.message, err = resource("message.txt")
	if err != nil {
		return nil, err
	}
	g.balls, err = sprites.NewImageSlots(sprites.ImageSlotsConfig{Images: g.images[:16], MaxSlots: 128})
	if err != nil {
		return nil, err
	}
	g.stage = ebiten.NewImage(Width, Height)
	for i := range g.text {
		g.text[i] = ebiten.NewImage(Width, Height)
	}
	for _, bank := range []struct {
		name string
		dst  []float32
	}{{"palette.bin", g.palette[:]}, {"raster-0.bin", g.raster0[:]}, {"raster-8.bin", g.raster8[:]}} {
		b, e := resource(bank.name)
		if e != nil {
			return nil, e
		}
		for i := 0; i < len(bank.dst)/4; i++ {
			putRGB(bank.dst[i*4:], binary.BigEndian.Uint16(b[i*2:]))
		}
	}
	for bank, name := range []string{"palette.bin", "palette-b.bin"} {
		b, e := resource(name)
		if e != nil {
			return nil, e
		}
		for i := range g.paletteBanks[bank] {
			g.paletteBanks[bank][i] = binary.BigEndian.Uint16(b[i*2:])
		}
	}
	g.shader, err = ebiten.NewShader([]byte(paletteShader))
	if err != nil {
		return nil, err
	}
	for i, w := range []uint16{0, 0x530, 0x630, 0x640, 0x750, 0x760, 0x770, 0x420, 0x320, 0x220, 0x100, 0x700, 0x700, 0x700, 0x700, 0x777} {
		putRGB(g.gold[i*4:], w)
	}
	g.uniforms = map[string]any{"Palette": g.palette[:], "Gold": g.gold[:], "Raster0": g.raster0[:], "Raster8": g.raster8[:]}
	music, err := resource("music.ym")
	if err != nil {
		return nil, err
	}
	g.visual, err = sound.Open("music.ym", music, sound.Options{SampleRate: 48000, BlockFrames: 960, Loop: true})
	if err != nil {
		return nil, err
	}
	if !mute {
		g.player, err = playback.Open(nil, "music.ym", music, sound.Options{SampleRate: 48000, BlockFrames: 960, Loop: true})
		if err != nil {
			return nil, err
		}
		g.player.Play()
	}
	return g, nil
}

func (g *Game) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		return ebiten.Termination
	}
	g.clock.Step()
	g.updatePalette()
	if _, err := io.ReadFull(g.visual, g.pcm[:]); err != nil {
		return err
	}
	g.stage.Clear()
	count := 0
	for _, p := range g.clock.Points {
		if p.X < 0 || p.X >= 300 || p.Y < 0 || p.Y >= 151 {
			continue
		}
		g.slots[count] = sprites.ImageSlot{Image: p.Material, X: float64(p.X), Y: float64(p.Y)}
		count++
	}
	if err := g.balls.SetSlots(g.slots[:count]); err != nil {
		return err
	}
	g.balls.Draw(g.stage)
	// The native ribbon inserts two font rows into one column each PAL tick.
	target := 1 - g.front
	g.text[target].DrawImage(g.text[g.front], &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy})
	x := g.column * 32
	op := ebiten.DrawImageOptions{Blend: ebiten.BlendCopy}
	op.GeoM.Translate(float64(x), 2)
	g.text[target].DrawImage(g.text[g.front].SubImage(image.Rect(x, 0, x+32, 30)).(*ebiten.Image), &op)
	glyph := int(g.message[g.letter]) - 32
	if glyph >= 0 && glyph < 61 {
		op.GeoM.Reset()
		op.GeoM.Translate(float64(x), 0)
		g.text[target].DrawImage(g.font.SubImage(image.Rect(glyph*32, g.row*2, glyph*32+32, g.row*2+2)).(*ebiten.Image), &op)
	}
	g.row--
	if g.row < 0 {
		g.row = 14
		g.letter = (g.letter + 1) % len(g.message)
		g.column = (g.column + 1) % 10
	}
	g.front = target
	return nil
}
func (g *Game) Draw(dst *ebiten.Image) {
	op := ebiten.DrawRectShaderOptions{Images: [4]*ebiten.Image{g.stage, g.background, g.text[g.front]}, Uniforms: g.uniforms, Blend: ebiten.BlendCopy}
	dst.DrawRectShader(Width, Height, g.shader, &op)
}
func (*Game) Layout(int, int) (int, int) { return Width, Height }
func (g *Game) Tick() int                { return g.clock.Tick }
func (g *Game) Close() {
	if g == nil || g.closed {
		return
	}
	g.closed = true
	if g.player != nil {
		g.player.Close()
	}
	if g.visual != nil {
		g.visual.Close()
	}
	if g.balls != nil {
		g.balls.Close()
	}
	if g.shader != nil {
		g.shader.Deallocate()
	}
	for _, im := range append(g.images, g.stage, g.text[0], g.text[1]) {
		if im != nil {
			im.Deallocate()
		}
	}
}

const paletteShader = `//kage:unit pixels
package main
var Palette [16]vec4
var Gold [16]vec4
var Raster0 [17]vec4
var Raster8 [67]vec4
func Fragment(position vec4,source vec2,color vec4)vec4{
 p:=source-imageSrc0Origin();y:=int(p.y)
 if y>=168 {t:=imageSrc2At(imageSrc0Origin()+vec2(p.x,p.y-168));return Gold[int(clamp(floor(t.r*15+.5),0,15))]*color}
 index:=int(clamp(floor(imageSrc0At(source).r*15+.5),0,7))+int(floor(imageSrc1At(source).r*15+.5))
 if index==0&&y>=150{return Raster0[int(clamp(y-150,0,16))]*color}
 if index==8{return Raster8[int(clamp(floor((p.y-2)/2),0,66))]*color}
 return Palette[index]*color
}
`

// updatePalette keeps the original eight-step fade and alternating color banks.
func (g *Game) updatePalette() {
	bank := g.clock.Scene % 2
	for i, w := range g.paletteBanks[bank] {
		if g.clock.Tick >= 1000 && g.clock.SceneTick <= 16 {
			if g.clock.SceneTick <= 8 {
				w = g.paletteBanks[1-bank][i]
				dark := uint16(g.clock.SceneTick)
				r, green, b := int(w>>8&7), int(w>>4&7), int(w&7)
				w = uint16(max(0, r-int(dark)))<<8 | uint16(max(0, green-int(dark)))<<4 | uint16(max(0, b-int(dark)))
			} else {
				bright := uint16(g.clock.SceneTick - 8)
				w = min(w>>8&7, bright)<<8 | min(w>>4&7, bright)<<4 | min(w&7, bright)
			}
		}
		putRGB(g.palette[i*4:], w)
	}
}
