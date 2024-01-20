package main

import (
	"fmt"
	"math"
	"time"

	"github.com/A7mad-2000as/GoFish/chessEngine"
	"golang.org/x/exp/constraints"
)

const (
	NumOfKillerMoves                                   = 2
	MaximumNumberOfPlies                               = 1024
	EssentialMovesOffset                        uint16 = math.MaxUint16 - 256
	PvMoveScore                                 uint16 = 65
	KillerMoveFirstSlotScore                    uint16 = 10
	KillerMoveSecondSlotScore                   uint16 = 20
	CounterMoveScore                            uint16 = 5
	HistoryHeuristicScoreUpperBound                    = int32(EssentialMovesOffset - 30)
	AspirationWindowOffset                      int16  = 35
	AspirationWindowMissTimeExtensionLowerBound        = 6
	StaticNullMovePruningPenalty                int16  = 85
	NullMovePruningDepthLimit                   int8   = 2
	RazoringDepthUpperBound                     int8   = 2
	FutilityPruningDepthUpperBound              int8   = 8
	InternalIterativeDeepeningDepthLowerBound   int8   = 4
	InternalIterativeDeepeningReductionAmount   int8   = 2
	LateMovePruningDepthUpperBound              int8   = 5
	FutilityPruningLegalMovesLowerBound         int    = 1
	SingularExtensionDepthLowerBound            int8   = 4
	SingularMoveExtensionPenalty                int16  = 125
	SignularMoveExtensionAmount                 int8   = 1
	LateMoveReductionLegalMoveLowerBound        int    = 4
	LateMoveReductionDepthLowerBound            int8   = 3
	drawScore                                   int16  = 0
)

var FutilityBoosts = [9]int16{0, 100, 160, 220, 280, 340, 400, 460, 520}
var LateMovePruningLegalMoveLowerBounds = [6]int{0, 8, 12, 16, 20, 24}
var LateMoveReductions = [chessEngine.MaxDepth + 1][100]int8{}

var MvvLvaScores [7][6]uint16 = [7][6]uint16{
	{15, 14, 13, 12, 11, 10},
	{25, 24, 23, 22, 21, 20},
	{35, 34, 33, 32, 31, 30},
	{45, 44, 43, 42, 41, 40},
	{55, 54, 53, 52, 51, 50},
	{0, 0, 0, 0, 0, 0},
	{0, 0, 0, 0, 0, 0},
}

type CustomSearcher struct {
	timeManager                CustomTimeManager
	position                   chessEngine.Position
	searchedNodes              uint64
	positionHashHistory        [MaximumNumberOfPlies]uint64
	positionHashHistoryCounter uint16
	sideToPlay                 uint8
	ageState                   uint8
	killerMoves                [chessEngine.MaxDepth + 1][NumOfKillerMoves]chessEngine.Move
	counterMoves               [2][64][64]chessEngine.Move
	historyHeuristicStats      [2][64][64]int32
}

func InitializeLateMoveReductions() {
	for depth := int8(3); depth < 100; depth++ {
		for legalMovesCounted := int8(3); legalMovesCounted < 100; legalMovesCounted++ {
			LateMoveReductions[depth][legalMovesCounted] = max(2, depth/4) + legalMovesCounted/12
		}
	}
}

func (searcher *CustomSearcher) GetOptions() map[string]chessEngine.EngineOption {
	options := make(map[string]chessEngine.EngineOption)

	return options
}

func (searcher *CustomSearcher) Reset(evaluator chessEngine.Evaluator) {
	*searcher = CustomSearcher{}
	searcher.InitializeSearchInfo(chessEngine.FENStartPosition, evaluator)
}

func (searcher *CustomSearcher) InitializeSearchInfo(fenString string, evaluator chessEngine.Evaluator) {
	searcher.position.LoadFEN(fenString, evaluator)
	searcher.positionHashHistoryCounter = 0
	searcher.positionHashHistory[searcher.positionHashHistoryCounter] = searcher.position.PositionHash
	searcher.ageState = 0
}

func (searcher *CustomSearcher) ResetToNewGame() {
	searcher.ClearKillerMoves()
	searcher.ClearCounterMoves()
	searcher.ClearHistoryHeuristicStats()
}

func (searcher *CustomSearcher) Position() *chessEngine.Position {
	return &searcher.position
}

func (searcher *CustomSearcher) RecordPositionHash(positionHash uint64) {
	searcher.positionHashHistoryCounter++
	searcher.positionHashHistory[searcher.positionHashHistoryCounter] = positionHash
}

func (searcher *CustomSearcher) EraseLatestPositionHash() {
	searcher.positionHashHistoryCounter--
}

func (searcher *CustomSearcher) ChangeKillerMoveSlot(ply uint8, killerMove chessEngine.Move) {
	nonCapture := (searcher.position.SquareContent[killerMove.GetToSquare()].PieceType == chessEngine.NoneType)
	if nonCapture {
		if !killerMove.IsSameMove(searcher.killerMoves[ply][0]) {
			searcher.killerMoves[ply][1] = searcher.killerMoves[ply][0]
			searcher.killerMoves[ply][0] = killerMove
		}
	}
}

func (searcher *CustomSearcher) ClearKillerMoves() {
	for depth := 0; depth < chessEngine.MaxDepth+1; depth++ {
		searcher.killerMoves[depth][0] = chessEngine.NullMove
		searcher.killerMoves[depth][1] = chessEngine.NullMove
	}
}

func (searcher *CustomSearcher) ClearCounterMoves() {
	for sourceSquare := 0; sourceSquare < 64; sourceSquare++ {
		for destinationSquare := 0; destinationSquare < 64; destinationSquare++ {
			searcher.counterMoves[chessEngine.White][sourceSquare][destinationSquare] = chessEngine.NullMove
			searcher.counterMoves[chessEngine.Black][sourceSquare][destinationSquare] = chessEngine.NullMove
		}
	}
}

func (searcher *CustomSearcher) ChangeCounterMoveSlot(previousMove chessEngine.Move, counterMove chessEngine.Move) {
	nonCapture := (searcher.position.SquareContent[counterMove.GetToSquare()].PieceType == chessEngine.NoneType)
	if nonCapture {
		searcher.counterMoves[searcher.position.SideToMove][previousMove.GetFromSquare()][previousMove.GetToSquare()] = counterMove
	}
}

func (searcher *CustomSearcher) ClearHistoryHeuristicStats() {
	for sourceSquare := 0; sourceSquare < 64; sourceSquare++ {
		for destinationSquare := 0; destinationSquare < 64; destinationSquare++ {
			searcher.historyHeuristicStats[searcher.position.SideToMove][sourceSquare][destinationSquare] = 0
		}
	}
}

func (searcher *CustomSearcher) IncreaseMoveHistoryStrength(move chessEngine.Move, depth int8) {
	nonCapture := (searcher.position.SquareContent[move.GetToSquare()].PieceType == chessEngine.NoneType)

	if nonCapture {
		searcher.historyHeuristicStats[searcher.position.SideToMove][move.GetFromSquare()][move.GetToSquare()] += int32(depth) * int32(depth)
	}

	if searcher.historyHeuristicStats[searcher.position.SideToMove][move.GetFromSquare()][move.GetToSquare()] >= HistoryHeuristicScoreUpperBound {
		searcher.ReduceHistoryHeuristicScores()
	}

}

func (searcher *CustomSearcher) DecreaseMoveHistoryStrength(move chessEngine.Move) {
	nonCapture := (searcher.position.SquareContent[move.GetToSquare()].PieceType == chessEngine.NoneType)

	if nonCapture && searcher.historyHeuristicStats[searcher.position.SideToMove][move.GetFromSquare()][move.GetToSquare()] > 0 {
		searcher.historyHeuristicStats[searcher.position.SideToMove][move.GetFromSquare()][move.GetToSquare()]--
	}
}

func (searcher *CustomSearcher) ReduceHistoryHeuristicScores() {
	for sourceSquare := 0; sourceSquare < 64; sourceSquare++ {
		for destinationSquare := 0; destinationSquare < 64; destinationSquare++ {
			searcher.historyHeuristicStats[searcher.position.SideToMove][sourceSquare][destinationSquare] /= 2
		}
	}
}

func (searcher *CustomSearcher) isThreeFoldRepetition() bool {
	for ply := uint16(0); ply < searcher.positionHashHistoryCounter; ply++ {
		if searcher.positionHashHistory[ply] == searcher.position.PositionHash {
			return true
		}
	}
	return false
}

func (searcher *CustomSearcher) AssignScoresToMoves(moves *chessEngine.MoveList, depth uint8, previousMove chessEngine.Move) {
	for moveIndex := uint8(0); moveIndex < moves.Size; moveIndex++ {
		move := &moves.Moves[moveIndex]
		pieceToBeMoved := searcher.position.SquareContent[move.GetFromSquare()].PieceType
		pieceToBeCaptured := searcher.position.SquareContent[move.GetToSquare()].PieceType

		if pieceToBeCaptured != chessEngine.NoneType {
			move.ModifyMoveScore(EssentialMovesOffset + MvvLvaScores[pieceToBeCaptured][pieceToBeMoved])
		} else if move.IsSameMove(searcher.killerMoves[depth][0]) {
			move.ModifyMoveScore(EssentialMovesOffset - KillerMoveFirstSlotScore)
		} else if move.IsSameMove(searcher.killerMoves[depth][1]) {
			move.ModifyMoveScore(EssentialMovesOffset - KillerMoveSecondSlotScore)
		} else {
			moveScore := uint16(0)
			moveHistoryStrength := uint16(searcher.historyHeuristicStats[searcher.position.SideToMove][move.GetFromSquare()][move.GetToSquare()])
			moveScore += moveHistoryStrength

			if move.IsSameMove(searcher.counterMoves[searcher.position.SideToMove][previousMove.GetFromSquare()][previousMove.GetToSquare()]) {
				moveScore += CounterMoveScore
			}

			move.ModifyMoveScore(moveScore)
		}
	}
}

func OrderHighestScoredMove(destinationIndex uint8, moves *chessEngine.MoveList) {
	highestScore := moves.Moves[destinationIndex].GetScore()
	highestScoreIndex := destinationIndex

	for i := destinationIndex; i < moves.Size; i++ {
		if moves.Moves[i].GetScore() > highestScore {
			highestScore = moves.Moves[i].GetScore()
			highestScoreIndex = i
		}
	}

	temp := moves.Moves[destinationIndex]
	moves.Moves[destinationIndex] = moves.Moves[highestScoreIndex]
	moves.Moves[highestScoreIndex] = temp
}

func (searcher *CustomSearcher) InitializeTimeManager(remainingTime int64, increment int64, moveTime int64, movesToGo int16, depth uint8, nodeCount uint64) {
	searcher.timeManager.Initialize(remainingTime, increment, moveTime, movesToGo, depth, nodeCount)
}

func getPresentableScore(nodeScore int16) string {
	if nodeScore > chessEngine.MateThreshold || nodeScore < -chessEngine.MateThreshold {
		halfMovesToMate := chessEngine.CheckmateScore - abs(nodeScore)
		fullMovesToMate := (halfMovesToMate / 2) + (halfMovesToMate % 2)
		return fmt.Sprintf("mate %d", fullMovesToMate*(nodeScore/abs(nodeScore)))
	} else {
		return fmt.Sprintf("cp %d", nodeScore)
	}
}

func (searcher *CustomSearcher) StartSearch(evaluator chessEngine.Evaluator) chessEngine.Move {
	bestMove := chessEngine.NullMove
	pv := chessEngine.PV{}
	searcher.ageState ^= 1
	searcher.sideToPlay = searcher.position.SideToMove
	searcher.searchedNodes = 0
	searchTime := int64(0)
	aspirationWindowMissTimeExtension := false
	alpha := -chessEngine.CheckmateScore
	beta := chessEngine.CheckmateScore

	searcher.ReduceHistoryHeuristicScores()
	searcher.timeManager.StartMoveTimeAllocation(searcher.position.CurrentPly)

	for depth := uint8(1); searcher.timeManager.nodeCount > 0 && depth <= chessEngine.MaxDepth && depth <= searcher.timeManager.depth; depth++ {
		pv.DeleteVariation()

		searchStartInstant := time.Now()
		nodeScore := searcher.Negamax(evaluator, int8(depth), 0, alpha, beta, &pv, true, chessEngine.NullMove, chessEngine.NullMove, false)
		searchDuration := time.Since(searchStartInstant)

		if searcher.timeManager.endSearch {
			if bestMove == chessEngine.NullMove && depth == 1 {
				bestMove = pv.GetVariationFirstMove()
			}
			break
		}

		if nodeScore >= beta || nodeScore <= alpha { // Outside aspiration window
			alpha = -chessEngine.CheckmateScore
			beta = chessEngine.CheckmateScore
			depth--

			if depth >= AspirationWindowMissTimeExtensionLowerBound && !aspirationWindowMissTimeExtension {
				searcher.timeManager.ChangeMoveAllocatedTime(searcher.timeManager.moveAllocatedTime * 13 / 10)
				aspirationWindowMissTimeExtension = true
			}
			continue
		}

		alpha = nodeScore - AspirationWindowOffset
		beta = nodeScore + AspirationWindowOffset

		searchTime += searchDuration.Milliseconds()
		bestMove = pv.GetVariationFirstMove()

		fmt.Printf("info depth %d score %s nodes %d nps %d time %d pv %s\n", depth, getPresentableScore(nodeScore), searcher.searchedNodes, uint64(float64(1000*searcher.searchedNodes)/float64(searchTime)), searchTime, pv)
	}

	return bestMove
}

func (searcher *CustomSearcher) StopSearch() {
	searcher.timeManager.endSearch = true
}

func (searcher *CustomSearcher) Negamax(evaluator chessEngine.Evaluator, depth int8, ply uint8, alpha int16, beta int16, pv *chessEngine.PV, nullMovePruningRequired bool, previousMove chessEngine.Move, singularMoveExtensionMove chessEngine.Move, singularMoveExtendedSearch bool) int16 {
	searcher.searchedNodes++

	if ply >= chessEngine.MaxDepth {
		return evaluator.EvaluatePosition(&searcher.position)
	}

	if searcher.searchedNodes >= searcher.timeManager.nodeCount {
		searcher.timeManager.endSearch = true
	}

	if searcher.searchedNodes&2047 == 0 {
		searcher.timeManager.SetMoveTimeIsUp()
	}

	if searcher.timeManager.endSearch {
		return 0
	}

	onTreeRoot := (ply == 0)
	inCheck := searcher.position.IsCurrentSideInCheck()
	isCurrentNodePv := beta-alpha != 1
	continuationPv := chessEngine.PV{}
	futilityPruningPossibility := false

	if inCheck {
		depth++
	}

	if depth <= 0 {
		searcher.searchedNodes--
		return evaluator.EvaluatePosition(&searcher.position)
	}

	if !onTreeRoot && ((searcher.position.Rule50 >= 100 && !(inCheck && ply == 1)) || searcher.isThreeFoldRepetition()) {
		return drawScore
	}

	if abs(beta) < chessEngine.MateThreshold && !inCheck && !isCurrentNodePv {
		currentPositionStaticEvaluation := evaluator.EvaluatePosition(&searcher.position)
		penalizedEvaluation := currentPositionStaticEvaluation - StaticNullMovePruningPenalty*int16(depth)
		if penalizedEvaluation >= beta {
			return penalizedEvaluation
		}
	}

	if depth <= RazoringDepthUpperBound && !inCheck && !isCurrentNodePv {
		currentPositionStaticEvaluation := evaluator.EvaluatePosition(&searcher.position)
		boostedScore := currentPositionStaticEvaluation + FutilityBoosts[depth]*3

		if boostedScore < alpha {
			razoredScore := evaluator.EvaluatePosition(&searcher.position)
			if razoredScore < alpha {
				return alpha
			}
		}
	}

	if depth <= FutilityPruningDepthUpperBound && alpha < chessEngine.MateThreshold && beta < chessEngine.MateThreshold && !inCheck && !isCurrentNodePv {
		currentPositionStaticEvaluation := evaluator.EvaluatePosition(&searcher.position)
		boost := FutilityBoosts[depth]
		futilityPruningPossibility = currentPositionStaticEvaluation+boost <= alpha
	}

	pseudoLegalMoves := chessEngine.GeneratePseudoLegalMoves(&searcher.position)
	searcher.AssignScoresToMoves(&pseudoLegalMoves, ply, previousMove)

	legalMoveCount := 0
	highestScore := -chessEngine.CheckmateScore

	for i := uint8(0); i < pseudoLegalMoves.Size; i++ {
		OrderHighestScoredMove(i, &pseudoLegalMoves)
		currentMove := pseudoLegalMoves.Moves[i]
		if currentMove.IsSameMove(singularMoveExtensionMove) {
			continue
		}

		if !searcher.position.DoMove(currentMove, evaluator) {
			searcher.position.UnDoPreviousMove(currentMove, evaluator)
			continue
		}

		legalMoveCount++

		if depth <= LateMovePruningDepthUpperBound && legalMoveCount > LateMovePruningLegalMoveLowerBounds[depth] && !inCheck && !isCurrentNodePv {
			if !(searcher.position.IsCurrentSideInCheck() || currentMove.GetMoveType() == chessEngine.PromotionMoveType) {
				searcher.position.UnDoPreviousMove(currentMove, evaluator)
				continue
			}
		}

		if futilityPruningPossibility && legalMoveCount > FutilityPruningLegalMovesLowerBound && currentMove.GetMoveType() != chessEngine.CaptureMoveType && currentMove.GetMoveType() != chessEngine.PromotionMoveType && !searcher.position.IsCurrentSideInCheck() {
			searcher.position.UnDoPreviousMove(currentMove, evaluator)
			continue
		}

		searcher.RecordPositionHash(searcher.position.PositionHash)

		score := int16(0)
		if legalMoveCount == 1 {
			effectiveDepth := depth - 1

			score = -searcher.Negamax(evaluator, effectiveDepth, ply+1, -beta, -alpha, &continuationPv, true, currentMove, chessEngine.NullMove, singularMoveExtendedSearch)
		} else {
			reductionAmount := int8(0)
			if legalMoveCount >= LateMoveReductionLegalMoveLowerBound && depth >= LateMoveReductionDepthLowerBound && !(inCheck || currentMove.GetMoveType() == chessEngine.CaptureMoveType) && !isCurrentNodePv {
				reductionAmount = LateMoveReductions[depth][legalMoveCount]
			}

			score = -searcher.Negamax(evaluator, depth-1-reductionAmount, ply+1, -(alpha + 1), -alpha, &continuationPv, true, currentMove, chessEngine.NullMove, singularMoveExtendedSearch)

			if score > alpha && reductionAmount > 0 {
				score = -searcher.Negamax(evaluator, depth-1, ply+1, -(alpha + 1), -alpha, &continuationPv, true, currentMove, chessEngine.NullMove, singularMoveExtendedSearch)
				if score > alpha {
					score = -searcher.Negamax(evaluator, depth-1, ply+1, -beta, -alpha, &continuationPv, true, currentMove, chessEngine.NullMove, singularMoveExtendedSearch)
				}
			} else if score > alpha && score < beta {
				score = -searcher.Negamax(evaluator, depth-1, ply+1, -beta, -alpha, &continuationPv, true, currentMove, chessEngine.NullMove, singularMoveExtendedSearch)
			}
		}

		searcher.position.UnDoPreviousMove(currentMove, evaluator)
		searcher.EraseLatestPositionHash()

		if score > highestScore {
			highestScore = score
		}

		if score >= beta {
			searcher.ChangeKillerMoveSlot(ply, currentMove)
			searcher.ChangeCounterMoveSlot(previousMove, currentMove)
			searcher.IncreaseMoveHistoryStrength(currentMove, depth)
			break
		} else {
			searcher.DecreaseMoveHistoryStrength(currentMove)
		}

		if score > alpha {
			alpha = score
			pv.SetNewVariation(currentMove, continuationPv)
			searcher.IncreaseMoveHistoryStrength(currentMove, depth)
		} else {
			searcher.DecreaseMoveHistoryStrength(currentMove)
		}
		continuationPv.DeleteVariation()
	}

	if legalMoveCount == 0 {
		if inCheck {
			return -chessEngine.CheckmateScore + int16(ply)
		}
		return drawScore
	}

	return highestScore
}

func (searcher *CustomSearcher) CleanUp() {
	return
}

func abs[Int constraints.Integer](n Int) Int {
	if n < 0 {
		return -n
	}
	return n
}

func max[Int constraints.Integer](a, b Int) Int {
	if a > b {
		return a
	}
	return b
}
