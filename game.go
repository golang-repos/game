// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin linux

package main

import (
	"image"
	"log"
	"math/rand"

	_ "image/png"

	"golang.org/x/mobile/asset"
	"golang.org/x/mobile/exp/f32"
	"golang.org/x/mobile/exp/sprite"
	"golang.org/x/mobile/exp/sprite/clock"
)

const (
	tileWidth, tileHeight = 16, 16 // width and height of each tile
	tilesX, tilesY        = 16, 16 // number of horizontal tiles

	gopherTile = 1 // which tile the gopher is standing on (0-indexed)

	initScrollV = 1     // initial scroll velocity
	scrollA     = 0.001 // scroll accelleration
	gravity     = 0.1   // gravity
	jumpV       = -5    // jump velocity
	flapV       = -1.5  // flap velocity

	deadScrollA         = -0.01 // scroll decelleration after the gopher dies
	deadTimeBeforeReset = 240   // how long to wait before restarting the game

	groundChangeProb = 5 // 1/probability of ground height change
	groundWobbleProb = 3 // 1/probability of minor ground height change
	groundMin        = tileHeight * (tilesY - 2*tilesY/5)
	groundMax        = tileHeight * tilesY
	initGroundY      = tileHeight * (tilesY - 1)

	climbGrace = tileHeight / 3 // gopher won't die if it hits a cliff this high
)

type Game struct {
	gopher struct {
		y        float32    // y-offset
		v        float32    // velocity
		atRest   bool       // is the gopher on the ground?
		flapped  bool       // has the gopher flapped since it became airborne?
		dead     bool       // is the gopher dead?
		deadTime clock.Time // when the gopher died
	}
	scroll struct {
		x float32 // x-offset
		v float32 // velocity
	}
	groundY  [tilesX + 3]float32 // ground y-offsets
	lastCalc clock.Time          // when we last calculated a frame
}

func NewGame() *Game {
	var g Game
	g.reset()
	return &g
}

func (g *Game) reset() {
	g.gopher.y = 0
	g.gopher.v = 0
	g.scroll.x = 0
	g.scroll.v = initScrollV
	for i := range g.groundY {
		g.groundY[i] = initGroundY
	}
	g.gopher.atRest = false
	g.gopher.flapped = false
	g.gopher.dead = false
	g.gopher.deadTime = 0
}

func (g *Game) Scene(eng sprite.Engine) *sprite.Node {
	texs := loadTextures(eng)

	scene := &sprite.Node{}
	eng.Register(scene)
	eng.SetTransform(scene, f32.Affine{
		{1, 0, 0},
		{0, 1, 0},
	})

	newNode := func(fn arrangerFunc) {
		n := &sprite.Node{Arranger: arrangerFunc(fn)}
		eng.Register(n)
		scene.AppendChild(n)
	}

	// The ground.
	for i := range g.groundY {
		i := i
		// The top of the ground.
		newNode(func(eng sprite.Engine, n *sprite.Node, t clock.Time) {
			eng.SetSubTex(n, texs[texGround])
			eng.SetTransform(n, f32.Affine{
				{tileWidth, 0, float32(i)*tileWidth - g.scroll.x},
				{0, tileHeight, g.groundY[i]},
			})
		})
		// The earth beneath.
		newNode(func(eng sprite.Engine, n *sprite.Node, t clock.Time) {
			eng.SetSubTex(n, texs[texEarth])
			eng.SetTransform(n, f32.Affine{
				{tileWidth, 0, float32(i)*tileWidth - g.scroll.x},
				{0, tileHeight * tilesY, g.groundY[i] + tileHeight},
			})
		})
	}

	// The gopher.
	newNode(func(eng sprite.Engine, n *sprite.Node, t clock.Time) {
		switch {
		case g.gopher.dead:
			eng.SetSubTex(n, texs[texGopherDead])
		case g.gopher.v < 0:
			eng.SetSubTex(n, texs[texGopherFlap])
		default:
			eng.SetSubTex(n, texs[texGopher])
		}
		eng.SetTransform(n, f32.Affine{
			{tileWidth * 2, 0, tileWidth*(gopherTile-1) + tileWidth/8},
			{0, tileHeight * 2, g.gopher.y - tileHeight + tileHeight/4},
		})
	})

	return scene
}

type arrangerFunc func(e sprite.Engine, n *sprite.Node, t clock.Time)

func (a arrangerFunc) Arrange(e sprite.Engine, n *sprite.Node, t clock.Time) { a(e, n, t) }

const (
	texGopher = iota
	texGopherDead
	texGopherFlap
	texGround
	texEarth
)

func loadTextures(eng sprite.Engine) []sprite.SubTex {
	a, err := asset.Open("sprite.png")
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()

	m, _, err := image.Decode(a)
	if err != nil {
		log.Fatal(err)
	}
	t, err := eng.LoadTexture(m)
	if err != nil {
		log.Fatal(err)
	}

	const n = 128
	return []sprite.SubTex{
		texGopher:     sprite.SubTex{t, image.Rect(n*0, 0, n*1, n)},
		texGopherFlap: sprite.SubTex{t, image.Rect(n*2, 0, n*3, n)},
		texGopherDead: sprite.SubTex{t, image.Rect(n*4, 0, n*5, n)},
		texGround:     sprite.SubTex{t, image.Rect(n*6+1, 0, n*7-1, n)},
		texEarth:      sprite.SubTex{t, image.Rect(n*10+1, 0, n*11-1, n)},
	}
}

func (g *Game) Press(down bool) {
	if g.gopher.dead {
		// Player can't control a dead gopher.
		return
	}

	if down {
		switch {
		case g.gopher.atRest:
			// Gopher may jump from the ground.
			g.gopher.v = jumpV
		case !g.gopher.flapped:
			// Gopher may flap once in mid-air.
			g.gopher.flapped = true
			g.gopher.v = flapV
		}
	} else {
		// Stop gopher rising on button release.
		if g.gopher.v < 0 {
			g.gopher.v = 0
		}
	}
}

func (g *Game) Update(now clock.Time) {
	if g.gopher.dead && now-g.gopher.deadTime > deadTimeBeforeReset {
		// Restart if the gopher has been dead for a while.
		g.reset()
	}

	// Compute game states up to now.
	for ; g.lastCalc < now; g.lastCalc++ {
		g.calcFrame()
	}
}

func (g *Game) calcFrame() {
	g.calcScroll()
	g.calcGopher()
}

func (g *Game) calcScroll() {
	// Compute velocity.
	if g.gopher.dead {
		// Decrease scroll speed when the gopher dies.
		g.scroll.v += deadScrollA
		if g.scroll.v < 0 {
			g.scroll.v = 0
		}
	} else {
		// Increase scroll speed.
		g.scroll.v += scrollA
	}

	// Compute offset.
	g.scroll.x += g.scroll.v

	// Create new ground tiles if we need to.
	for g.scroll.x > tileWidth {
		g.newGroundTile()

		// Check whether the gopher has crashed.
		// Do this for each new ground tile so that when the scroll
		// velocity is >tileWidth/frame it can't pass through the ground.
		if !g.gopher.dead && g.gopherCrashed() {
			g.killGopher()
		}
	}
}

func (g *Game) calcGopher() {
	// Compute velocity.
	g.gopher.v += gravity

	// Compute offset.
	g.gopher.y += g.gopher.v

	g.clampToGround()
}

func (g *Game) newGroundTile() {
	// Compute next ground y-offset.
	next := g.nextGroundY()

	// Shift ground tiles to the left.
	g.scroll.x -= tileWidth
	copy(g.groundY[:], g.groundY[1:])
	g.groundY[len(g.groundY)-1] = next
}

func (g *Game) nextGroundY() float32 {
	prev := g.groundY[len(g.groundY)-1]
	if change := rand.Intn(groundChangeProb) == 0; change {
		return (groundMax-groundMin)*rand.Float32() + groundMin
	}
	if wobble := rand.Intn(groundWobbleProb) == 0; wobble {
		return prev + (rand.Float32()-0.5)*climbGrace
	}
	return prev
}

func (g *Game) gopherCrashed() bool {
	return g.gopher.y+tileHeight-climbGrace > g.groundY[gopherTile+1]
}

func (g *Game) killGopher() {
	g.gopher.dead = true
	g.gopher.deadTime = g.lastCalc
	g.gopher.v = jumpV // Bounce off screen.
}

func (g *Game) clampToGround() {
	if g.gopher.dead {
		// Allow the gopher to fall through ground when dead.
		return
	}

	// Compute the minimum offset of the ground beneath the gopher.
	minY := g.groundY[gopherTile]
	if y := g.groundY[gopherTile+1]; y < minY {
		minY = y
	}

	// Prevent the gopher from falling through the ground.
	maxGopherY := minY - tileHeight
	g.gopher.atRest = false
	if g.gopher.y >= maxGopherY {
		g.gopher.v = 0
		g.gopher.y = maxGopherY
		g.gopher.atRest = true
		g.gopher.flapped = false
	}
}
