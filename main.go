package main

import (
	"github.com/A7mad-2000as/GoFish/chessEngine"
)

func main() {
	InitializeLateMoveReductions()
	chessEngine.InitEvaluationRelatedMasks()
	engineInterface := chessEngine.NewCustomEngineInterface(&CustomSearcher{}, &chessEngine.DefaultEvaluator{})
	engineInterface.StartEngine()
}
