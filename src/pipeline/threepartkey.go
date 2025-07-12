package pipeline

import (
	"crypto/md5"
	"cursor-twitter/src/tweets"
	"encoding/binary"
	"sync"
)

var TokenTo3PK = make(map[string]tweets.ThreePartKey)
var ThreePKToToken = make(map[tweets.ThreePartKey]string)
var Token3PKMutex sync.RWMutex

// Global array length for 3PK generation (set from main.go)
var GlobalArrayLen int = 1000 // Default, will be set from config

func GenerateThreePartKey(token string) tweets.ThreePartKey {
	a := hashWithSuffix(token, "__0NE__", GlobalArrayLen)
	b := hashWithSuffix(token, "__TW0__", GlobalArrayLen)
	c := hashWithSuffix(token, "__THR33__", GlobalArrayLen)
	key := tweets.ThreePartKey{Part1: a, Part2: b, Part3: c}
	// Store in global maps if not already present
	Token3PKMutex.RLock()
	_, exists := TokenTo3PK[token]
	Token3PKMutex.RUnlock()
	if !exists {
		Token3PKMutex.Lock()
		TokenTo3PK[token] = key
		ThreePKToToken[key] = token
		Token3PKMutex.Unlock()
	}
	return key
}

func hashWithSuffix(token, suffix string, modulo int) int {
	h := md5.Sum([]byte(token + suffix))
	return int(binary.BigEndian.Uint32(h[:4])) % modulo
}

// SetGlobalArrayLen sets the global array length for 3PK generation
func SetGlobalArrayLen(arrayLen int) {
	GlobalArrayLen = arrayLen
}

// GetWordFrom3PK retrieves a word from the global 3PK mapping
func GetWordFrom3PK(threePK tweets.ThreePartKey) (string, bool) {
	Token3PKMutex.RLock()
	defer Token3PKMutex.RUnlock()
	word, exists := ThreePKToToken[threePK]
	return word, exists
}

// AddWordTo3PKMapping adds a word to the global 3PK mapping
func AddWordTo3PKMapping(word string, threePK tweets.ThreePartKey) {
	Token3PKMutex.Lock()
	defer Token3PKMutex.Unlock()
	ThreePKToToken[threePK] = word
	TokenTo3PK[word] = threePK
}
