package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"
)

const (
	numPlayers      = 4
	tokensPerPlayer = 4
	trackLen        = 28 // spaces on the shared outer track (7 per player)
	spacing         = trackLen / numPlayers

	// A token's position is stored relative to its owner's start space:
	//   -1      : in jail
	//   0..27   : on the track (0 = own start space, 27 = last space before home)
	//   28..31  : in the 4 finish slots
	jail      = -1
	firstHome = trackLen
	lastHome  = trackLen + tokensPerPlayer - 1

	numGames = 5000
)

// verbose prints every roll when true. Turned off for batch runs.
var verbose = false

var playerNames = [numPlayers]string{"Red", "Blue", "Green", "Yellow"}

// A Strategy picks which token to move for a given roll, returning the token
// index, or -1 if there is no legal move. All four players use the same one.
type Strategy func(g *Game, player, roll int) int

// Rules determines what happens to a token that gets landed on.
type Rules int

const (
	// StandardRules: a captured token goes back to jail and needs a 6 to leave.
	StandardRules Rules = iota
	// ParoleRules: once free from jail, a token never returns. A captured token
	// goes back to its own start space, or if that is occupied (by any token),
	// to the next unoccupied space toward its finish.
	ParoleRules
)

type Game struct {
	tokens   [numPlayers][tokensPerPlayer]int
	rolls    int
	rng      *rand.Rand
	strategy Strategy
	rules    Rules
}

func NewGame(rng *rand.Rand, strategy Strategy, rules Rules) *Game {
	g := &Game{rng: rng, strategy: strategy, rules: rules}
	for p := range g.tokens {
		for t := range g.tokens[p] {
			g.tokens[p][t] = jail
		}
	}
	return g
}

// absPos converts a player-relative track position into a shared board position.
func absPos(player, rel int) int {
	return (player*spacing + rel) % trackLen
}

func (g *Game) roll() int {
	g.rolls++
	return g.rng.Intn(6) + 1
}

func (g *Game) occupiedByOwn(player, rel int) bool {
	for _, pos := range g.tokens[player] {
		if pos == rel {
			return true
		}
	}
	return false
}

// trackOccupied reports whether any token (of any player) other than the given
// one is sitting on the shared board position a.
func (g *Game) trackOccupied(a, exceptPlayer, exceptToken int) bool {
	for p := 0; p < numPlayers; p++ {
		for t, pos := range g.tokens[p] {
			if p == exceptPlayer && t == exceptToken {
				continue
			}
			if pos >= 0 && pos < firstHome && absPos(p, pos) == a {
				return true
			}
		}
	}
	return false
}

// paroleSpot finds where a captured token lands under parole rules: its own
// start space, or the next unoccupied track space heading toward its finish.
func (g *Game) paroleSpot(player, token int) int {
	for rel := 0; rel < firstHome; rel++ {
		if !g.trackOccupied(absPos(player, rel), player, token) {
			return rel
		}
	}
	return jail // unreachable in practice: 28 spaces vs. at most 15 other tokens
}

func (g *Game) allInJail(player int) bool {
	for _, pos := range g.tokens[player] {
		if pos != jail {
			return false
		}
	}
	return true
}

func (g *Game) hasWon(player int) bool {
	for _, pos := range g.tokens[player] {
		if pos < firstHome {
			return false
		}
	}
	return true
}

// furthestFirst (Strategy A): with a 6, free a jailed token if possible;
// otherwise move the furthest-along token that has a legal move.
func furthestFirst(g *Game, player, roll int) int {
	if roll == 6 && !g.occupiedByOwn(player, 0) {
		for t, pos := range g.tokens[player] {
			if pos == jail {
				return t
			}
		}
	}
	return furthestMovable(g, player, roll)
}

// onePieceAtATime (Strategy B): never have more than one token on the track.
// While a token is out on the track, every roll advances it, and 6s are NOT
// used to free another token. Only once nothing is on the track (the previous
// token is safe in the finish area) does a 6 free the next token.
//
// If the lone track token can't legally use a roll (overshoots the finish or
// is blocked), the roll goes to the furthest finish-area token that can move
// deeper; it never frees a token from jail.
func onePieceAtATime(g *Game, player, roll int) int {
	onTrack := -1
	for t, pos := range g.tokens[player] {
		if pos >= 0 && pos < firstHome {
			onTrack = t
		}
	}

	if onTrack == -1 {
		if roll == 6 {
			for t, pos := range g.tokens[player] {
				if pos == jail {
					return t
				}
			}
		}
		return furthestMovable(g, player, roll)
	}

	target := g.tokens[player][onTrack] + roll
	if target <= lastHome && !g.occupiedByOwn(player, target) {
		return onTrack
	}
	return furthestMovable(g, player, roll)
}

// furthestMovable returns the furthest-along non-jailed token that can legally
// move by roll, or -1 if none can.
func furthestMovable(g *Game, player, roll int) int {
	best, bestPos := -1, jail
	for t, pos := range g.tokens[player] {
		if pos == jail {
			continue
		}
		target := pos + roll
		if target > lastHome || g.occupiedByOwn(player, target) {
			continue // must land exactly in finish area; can't land on own token
		}
		if pos > bestPos {
			best, bestPos = t, pos
		}
	}
	return best
}

// applyMove moves the token and handles captures. Returns a description.
func (g *Game) applyMove(player, t, roll int) string {
	from := g.tokens[player][t]
	var to int
	if from == jail {
		to = 0
	} else {
		to = from + roll
	}
	g.tokens[player][t] = to

	desc := ""
	switch {
	case from == jail:
		desc = fmt.Sprintf("token %d leaves jail", t+1)
	case to >= firstHome:
		desc = fmt.Sprintf("token %d moves %d -> home slot %d", t+1, from, to-firstHome+1)
	default:
		desc = fmt.Sprintf("token %d moves %d -> %d", t+1, from, to)
	}

	// Landing on the track bumps any opponent token on that space back to jail.
	if to < firstHome {
		a := absPos(player, to)
		for op := 0; op < numPlayers; op++ {
			if op == player {
				continue
			}
			for ot, opos := range g.tokens[op] {
				if opos >= 0 && opos < firstHome && absPos(op, opos) == a {
					if g.rules == ParoleRules {
						newPos := g.paroleSpot(op, ot)
						g.tokens[op][ot] = newPos
						if newPos == 0 {
							desc += fmt.Sprintf(" and sends %s's token %d back to its start!", playerNames[op], ot+1)
						} else {
							desc += fmt.Sprintf(" and sends %s's token %d back to %d spaces past its start!", playerNames[op], ot+1, newPos)
						}
					} else {
						g.tokens[op][ot] = jail
						desc += fmt.Sprintf(" and sends %s's token %d to jail!", playerNames[op], ot+1)
					}
				}
			}
		}
	}
	return desc
}

// takeTurn plays one player's full turn (including bonus rolls on 6s and the
// three-tries rule when all tokens are in jail). Returns true if they won.
func (g *Game) takeTurn(player int) bool {
	tries := 0
	for {
		limit := 1
		if g.allInJail(player) {
			limit = 3
		}
		if tries >= limit {
			return false
		}
		tries++

		roll := g.roll()
		t := g.strategy(g, player, roll)
		if verbose {
			if t >= 0 {
				fmt.Printf("Roll %3d: %-6s rolled %d, %s\n", g.rolls, playerNames[player], roll, g.applyMove(player, t, roll))
			} else {
				fmt.Printf("Roll %3d: %-6s rolled %d, no legal move\n", g.rolls, playerNames[player], roll)
			}
		} else if t >= 0 {
			g.applyMove(player, t, roll)
		}

		if g.hasWon(player) {
			return true
		}
		if roll == 6 {
			tries = 0 // rolling a 6 earns another roll
		}
	}
}

// Play runs the game to completion, returning the winner and total rolls.
func (g *Game) Play() (winner int, totalRolls int) {
	for player := 0; ; player = (player + 1) % numPlayers {
		if g.takeTurn(player) {
			return player, g.rolls
		}
	}
}

func mean(data []int) float64 {
	sum := 0
	for _, v := range data {
		sum += v
	}
	return float64(sum) / float64(len(data))
}

// median expects data to already be sorted.
func median(sorted []int) float64 {
	n := len(sorted)
	if n%2 == 1 {
		return float64(sorted[n/2])
	}
	return float64(sorted[n/2-1]+sorted[n/2]) / 2
}

// stdDev returns the sample standard deviation (n-1 denominator).
func stdDev(data []int, avg float64) float64 {
	if len(data) < 2 {
		return 0
	}
	sumSq := 0.0
	for _, v := range data {
		d := float64(v) - avg
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(data)-1))
}

// simulate runs numGames games with the given strategy and prints statistics.
func simulate(name string, strategy Strategy, rules Rules, rng *rand.Rand) {
	results := make([]int, numGames)
	for i := range results {
		_, rolls := NewGame(rng, strategy, rules).Play()
		results[i] = rolls
	}

	sort.Ints(results)
	avg := mean(results)

	fmt.Printf("=== %s ===\n", name)
	fmt.Printf("Games simulated:    %d\n", numGames)
	fmt.Printf("Average rolls:      %.2f\n", avg)
	fmt.Printf("Median rolls:       %.1f\n", median(results))
	fmt.Printf("Std deviation:      %.2f\n\n", stdDev(results, avg))
}

func main() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	rulesets := []struct {
		name  string
		rules Rules
	}{
		{"Standard rules", StandardRules},
		{"Parole rules", ParoleRules},
	}
	strategies := []struct {
		name     string
		strategy Strategy
	}{
		{"Strategy A (free on every 6, advance furthest)", furthestFirst},
		{"Strategy B (one token at a time)", onePieceAtATime},
	}

	for _, r := range rulesets {
		for _, s := range strategies {
			simulate(r.name+" / "+s.name, s.strategy, r.rules, rng)
		}
	}
}
