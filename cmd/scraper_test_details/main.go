package main

import (
	"encoding/json"
	"fmt"

	"github.com/d0nedev/newsscore/internal/provider/flashscore"
)

func main() {
	matchID := "KO07rBLH" // Persijap vs Persib
	fmt.Printf("Fetching details for Match %s...\n", matchID)

	events, stats, players, err := flashscore.ScrapeMatchDetails(matchID)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("\n--- Top Stats (Count: %d) ---\n", len(stats))
	for i := 0; i < 5 && i < len(stats); i++ {
		b, _ := json.Marshal(stats[i])
		fmt.Println(string(b))
	}

	fmt.Printf("\n--- Events (Count: %d) ---\n", len(events))
	for i := 0; i < 5 && i < len(events); i++ {
		b, _ := json.Marshal(events[i])
		fmt.Println(string(b))
	}

	fmt.Printf("\n--- Players in Lineups (Count: %d) ---\n", len(players))
	for i := 0; i < 5 && i < len(players); i++ {
		b, _ := json.Marshal(players[i])
		fmt.Println(string(b))
	}
}
