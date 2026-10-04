package main

import (
	"fmt"
	"math/rand"
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

	verbose = true // print every roll; set to false for quiet runs
)

var playerNames = [numPlayers]string{"Red", "Blue", "Green", "Yellow"}

type Game struct {
	tokens [numPlayers][tokensPerPlayer]int
	rolls  int
	rng    *rand.Rand
}

func NewGame(rng *rand.Rand) *Game {
	g := &Game{rng: rng}
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

// chooseMove applies the strategy: with a 6, free a jailed token if possible;
// otherwise move the furthest-along token that has a legal move.
// Returns the token index, or -1 if no legal move exists.
func (g *Game) chooseMove(player, roll int) int {
	if roll == 6 && !g.occupiedByOwn(player, 0) {
		for t, pos := range g.tokens[player] {
			if pos == jail {
				return t
			}
		}
	}

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
					g.tokens[op][ot] = jail
					desc += fmt.Sprintf(" and sends %s's token %d to jail!", playerNames[op], ot+1)
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
		t := g.chooseMove(player, roll)
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

func main() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	game := NewGame(rng)
	winner, rolls := game.Play()
	fmt.Printf("\n%s wins! The game took %d total rolls.\n", playerNames[winner], rolls)
}
