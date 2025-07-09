package filter

import (
	"os"
	"testing"
)

func TestNewWordFilter(t *testing.T) {
	wf := NewWordFilter()
	if wf == nil {
		t.Fatal("NewWordFilter returned nil")
	}
	if wf.GetFilteredCount() != 0 {
		t.Errorf("Expected empty filter, got %d words", wf.GetFilteredCount())
	}
}

func TestAddAndRemoveWord(t *testing.T) {
	wf := NewWordFilter()

	// Test adding words
	wf.AddWord("test")
	wf.AddWord("WORD")
	wf.AddWord("MixedCase")

	if !wf.IsFiltered("test") {
		t.Error("Expected 'test' to be filtered")
	}
	if !wf.IsFiltered("WORD") {
		t.Error("Expected 'WORD' to be filtered")
	}
	if !wf.IsFiltered("MixedCase") {
		t.Error("Expected 'MixedCase' to be filtered")
	}
	if !wf.IsFiltered("TEST") {
		t.Error("Expected 'TEST' to be filtered (case insensitive)")
	}

	// Test removing words
	wf.RemoveWord("test")
	if wf.IsFiltered("test") {
		t.Error("Expected 'test' to not be filtered after removal")
	}
	if !wf.IsFiltered("WORD") {
		t.Error("Expected 'WORD' to still be filtered")
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temporary test file
	content := `# This is a comment
test
WORD
# Another comment
mixedcase

# Empty line above
filtered`

	tmpfile, err := os.CreateTemp("", "filter_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Test loading
	wf := NewWordFilter()
	err = wf.LoadFromFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Check that we loaded the expected words
	expectedCount := 4 // test, WORD, mixedcase, filtered
	if wf.GetFilteredCount() != expectedCount {
		t.Errorf("Expected %d words, got %d", expectedCount, wf.GetFilteredCount())
	}

	// Test that words are filtered (case insensitive)
	testCases := []struct {
		token    string
		shouldBe bool
	}{
		{"test", true},
		{"TEST", true},
		{"word", true},
		{"WORD", true},
		{"mixedcase", true},
		{"MIXEDCASE", true},
		{"filtered", true},
		{"FILTERED", true},
		{"notfound", false},
		{"comment", false},
	}

	for _, tc := range testCases {
		if wf.IsFiltered(tc.token) != tc.shouldBe {
			t.Errorf("IsFiltered('%s') = %v, expected %v", tc.token, wf.IsFiltered(tc.token), tc.shouldBe)
		}
	}
}

func TestLoadFromFileNotFound(t *testing.T) {
	wf := NewWordFilter()
	err := wf.LoadFromFile("nonexistent_file.txt")
	if err == nil {
		t.Error("Expected error when loading nonexistent file")
	}
}

func TestLoadFromFileEmpty(t *testing.T) {
	// Create an empty temporary file
	tmpfile, err := os.CreateTemp("", "empty_filter_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	wf := NewWordFilter()
	err = wf.LoadFromFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadFromFile failed on empty file: %v", err)
	}

	if wf.GetFilteredCount() != 0 {
		t.Errorf("Expected 0 words from empty file, got %d", wf.GetFilteredCount())
	}
}

func TestLoadFromFileOnlyComments(t *testing.T) {
	// Create a file with only comments
	content := `# This is a comment
# Another comment
# Yet another comment`

	tmpfile, err := os.CreateTemp("", "comments_filter_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	wf := NewWordFilter()
	err = wf.LoadFromFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadFromFile failed on comments-only file: %v", err)
	}

	if wf.GetFilteredCount() != 0 {
		t.Errorf("Expected 0 words from comments-only file, got %d", wf.GetFilteredCount())
	}
}

func TestConcurrentAccess(t *testing.T) {
	wf := NewWordFilter()

	// Add some initial words
	wf.AddWord("test1")
	wf.AddWord("test2")

	// Test concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				wf.IsFiltered("test1")
				wf.IsFiltered("test2")
				wf.IsFiltered("notfound")
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify the filter still works correctly
	if !wf.IsFiltered("test1") {
		t.Error("test1 should still be filtered after concurrent access")
	}
	if !wf.IsFiltered("test2") {
		t.Error("test2 should still be filtered after concurrent access")
	}
	if wf.IsFiltered("notfound") {
		t.Error("notfound should not be filtered after concurrent access")
	}
}
