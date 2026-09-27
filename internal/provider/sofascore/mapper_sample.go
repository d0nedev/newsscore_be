package sofascore

import (
	"encoding/json"
	"fmt"
	"os"
)

// SofaStandingResponse menyesuaikan struktur respons internal SofaScore API
type SofaStandingResponse struct {
	Standings []struct {
		Tournament struct {
			Name string `json:"name"`
		} `json:"tournament"`
		Type string `json:"type"`
		Rows []struct {
			Position int `json:"position"`
			Team     struct {
				ID        int    `json:"id"`
				Name      string `json:"name"`
				ShortName string `json:"shortName"`
			} `json:"team"`
			Matches       int `json:"matches"`
			Wins          int `json:"wins"`
			Draws         int `json:"draws"`
			Losses        int `json:"losses"`
			ScoresFor     int `json:"scoresFor"`
			ScoresAgainst int `json:"scoresAgainst"`
			Points        int `json:"points"`
		} `json:"rows"`
	} `json:"standings"`
}

// MapToDomain mensimulasikan pemetaan data SofaScore ke Domain internal kita
func ParseSampleAndMap() {
	data, err := os.ReadFile("internal/provider/sofascore/sample_sofascore_liga1.json")
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	var res SofaStandingResponse
	if err := json.Unmarshal(data, &res); err != nil {
		fmt.Println("Error unmarshaling:", err)
		return
	}

	fmt.Println("Mengekstraksi Klasemen Liga 1 Indonesia dari JSON SofaScore...")
	fmt.Println("=============================================================")
	for _, st := range res.Standings {
		if st.Type == "total" {
			fmt.Printf("Turnamen: %s\n", st.Tournament.Name)
			fmt.Println("Pos | Tim                  | M | M | S | K | Poin")
			fmt.Println("-----------------------------------------------------")
			for _, row := range st.Rows {
				fmt.Printf("%3d | %-20s | %d | %d | %d | %d | %4d\n",
					row.Position, row.Team.Name,
					row.Matches, row.Wins, row.Draws, row.Losses, row.Points)
			}
		}
	}
}
