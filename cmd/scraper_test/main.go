package main

import (
	"encoding/json"
	"fmt"

	"github.com/d0nedev/newsscore/internal/provider/flashscore"
)

func main() {
	matches, teams, err := flashscore.ScrapeLeague()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("Berhasil mengambil %d tim dan %d pertandingan!\n", len(teams), len(matches))

	fmt.Println("\n--- Sampel 3 Tim ---")
	for i := 0; i < 3 && i < len(teams); i++ {
		b, _ := json.Marshal(teams[i])
		fmt.Println(string(b))
	}

	fmt.Println("\n--- Sampel 3 Pertandingan ---")
	for i := 0; i < 3 && i < len(matches); i++ {
		b, _ := json.Marshal(matches[i])
		fmt.Println(string(b))
	}
}
