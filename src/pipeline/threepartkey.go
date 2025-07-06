package pipeline

import (
	"crypto/md5"
	"cursor-twitter/src/tweets"
	"encoding/binary"
)

const (
	ThreePKModulo = 1000 // Can be adjusted as needed
)

var TokenTo3PK = make(map[string]tweets.ThreePartKey)
var ThreePKToToken = make(map[tweets.ThreePartKey]string)

func GenerateThreePartKey(token string) tweets.ThreePartKey {
	a := hashWithSuffix(token, "__0NE__", ThreePKModulo)
	b := hashWithSuffix(token, "__TW0__", ThreePKModulo)
	c := hashWithSuffix(token, "__THR33__", ThreePKModulo)
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
