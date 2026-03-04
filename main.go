package main

import (
	"aVNE/engine"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	eng, err := engine.NewEngine()
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
	}

	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("aVNE - air Visual Novel Engine - Ver 0.0.1")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(eng); err != nil {
		log.Fatalf("Game error: %v", err)
	}
}
