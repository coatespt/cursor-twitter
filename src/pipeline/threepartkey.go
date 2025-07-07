package pipeline

import (
	"crypto/md5"
	"cursor-twitter/src/tweets"
	"encoding/binary"
)

var TokenTo3PK = make(map[string]tweets.ThreePartKey)
var ThreePKToToken = make(map[tweets.ThreePartKey]string)

// Global array length for 3PK generation (set from main.go)
var GlobalArrayLen int = 1000 // Default, will be set from config

func GenerateThreePartKey(token string) tweets.ThreePartKey {
	a := hashWithSuffix(token, "__0NE__", GlobalArrayLen)
	b := hashWithSuffix(token, "__TW0__", GlobalArrayLen)
	c := hashWithSuffix(token, "__THR33__", GlobalArrayLen)
	key := tweets.ThreePartKey{Part1: a, Part2: b, Part3: c}
	// Store in global maps if not already present
	if _, exists := TokenTo3PK[token]; !exists {
		TokenTo3PK[token] = key
		ThreePKToToken[key] = token
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
