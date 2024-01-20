package main

import (
	"github.com/A7mad-2000as/GoFish/chessEngine"
)

func main() {
	engineInterface := chessEngine.NewCustomEngineInterface(&CustomSearcher{}, &chessEngine.DefaultEvaluator{})
	engineInterface.StartEngine()
}
