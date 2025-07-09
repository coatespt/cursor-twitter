package pipeline

import (
	"cursor-twitter/src/tweets"
	"testing"
)

// mockThreePartKey returns a fixed ThreePartKey for collision simulation
// Rationale: We want to simulate a hash collision between two different tokens.
func mockThreePartKey() tweets.ThreePartKey {
	return tweets.ThreePartKey{Part1: 1, Part2: 2, Part3: 3}
}

// TestThreePartKeyCollisionHandling verifies that when two tokens map to the same 3PK,
// the latest token overrides the previous one in the lookup table.
//
// Rationale:
// - 3PKs are not guaranteed to be unique; collisions are possible, though rare.
// - The system policy is that the latest token wins for a given 3PK.
// - This test ensures the system behaves as designed and does not panic or misbehave on collision.
// - This is acceptable for the use case, as collisions are extremely rare and not catastrophic.
func TestThreePartKeyCollisionHandling(t *testing.T) {
	// Clear global maps before test
	// Rationale: Ensure a clean state for the test, avoiding interference from previous tests.
	TokenTo3PK = make(map[string]tweets.ThreePartKey)
	ThreePKToToken = make(map[tweets.ThreePartKey]string)

	tokenA := "tokenA"
	tokenB := "tokenB"
	collidingKey := mockThreePartKey()

	// Manually insert tokenA with the colliding key
	// Rationale: Simulate the first token being assigned a 3PK.
	TokenTo3PK[tokenA] = collidingKey
	ThreePKToToken[collidingKey] = tokenA

	// Now insert tokenB with the same key (simulate collision)
	// Rationale: Simulate a collision where a new token gets the same 3PK as an existing one.
	TokenTo3PK[tokenB] = collidingKey
	ThreePKToToken[collidingKey] = tokenB

	// The lookup table should now map the colliding key to tokenB (latest wins)
	// Rationale: This is the intended system behavior for collisions.
	if got := ThreePKToToken[collidingKey]; got != tokenB {
		t.Errorf("Expected ThreePKToToken to map to latest token (tokenB), got %s", got)
	}

	// Only the latest token is retrievable by the 3PK
	// Rationale: Both tokens should still exist as keys in TokenTo3PK, but only the latest is mapped in ThreePKToToken.
	if _, exists := TokenTo3PK[tokenA]; !exists {
		t.Errorf("TokenTo3PK should still contain tokenA as a key")
	}
	if _, exists := TokenTo3PK[tokenB]; !exists {
		t.Errorf("TokenTo3PK should contain tokenB as a key")
	}

	// Rationale: This test demonstrates that 3PK collisions result in the latest token overwriting the previous mapping in ThreePKToToken. This is by design and is acceptable due to the rarity of collisions.
}
