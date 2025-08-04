package main

import (
	"fmt"
	"log"

	"github.com/muesli/clustering/distance"
	"github.com/muesli/kmeans"
)

// Data: sentences and their key words
var data = map[string][]string{
	"I love pizza and pasta":        {"pizza", "pasta"},
	"She likes ramen and sushi":     {"ramen", "sushi"},
	"Cat sleeps all day":            {"cat", "sleeps"},
	"Dogs bark in the park":         {"dog", "bark", "park"},
	"I enjoy Italian food":          {"pizza", "italian"},
	"Japanese food is delicious":    {"sushi", "ramen"},
	"Dog plays fetch":               {"dog", "plays"},
	"Cat chases laser":              {"cat", "chases"},
	"He walks in the dog park":      {"dog", "walks", "park"},
	"She naps beside the cat":       {"cat", "naps"},
}

func main() {
	// Step 1: Build vocabulary
	wordToIndex := make(map[string]int)
	idx := 0
	for _, words := range data {
		for _, word := range words {
			if _, exists := wordToIndex[word]; !exists {
				wordToIndex[word] = idx
				idx++
			}
		}
	}
	vocabSize := len(wordToIndex)

	// Step 2: Build vectors
	sentences := make([]string, 0, len(data))
	vectors := make([]kmeans.Observation, 0, len(data))

	for sentence, words := range data {
		vec := make([]float64, vocabSize)
		for _, word := range words {
			if i, ok := wordToIndex[word]; ok {
				vec[i] = 1.0
			}
		}
		sentences = append(sentences, sentence)
		vectors = append(vectors, vec)
	}

	// Step 3: Cluster
	model := kmeans.New()
	clusters, err := model.Partition(vectors, 3, distance.Euclidean)
	if err != nil {
		log.Fatal(err)
	}

	// Step 4: Output
	for i, cluster := range clusters {
		fmt.Printf("\nCluster %d:\n", i)
		for _, obs := range cluster.Observations {
			// Find sentence corresponding to this observation
			for j, v := range vectors {
				if sameVector(v, obs) {
					fmt.Println("-", sentences[j])
				}
			}
		}
	}
}

// Helper: compare two vectors
func sameVector(a, b kmeans.Observation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

🛠 To Run This:

    Install the dependency:

// go get github.com/muesli/kmeans

//     Save the program to main.go

//     Run it:

// go run main.go
