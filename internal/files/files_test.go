package files_test

import (
	"os"
	"testing"

	"github.com/mona-actions/gh-migrate-packages/internal/files"
)

func TestCreateJSON(t *testing.T) {
	data := struct {
		Name string
		Age  int
	}{
		Name: "John Doe",
		Age:  30,
	}

	filename := "test.json"

	err := files.CreateJSON(data, filename)
	if err != nil {
		t.Errorf("CreateJSON returned an error: %v", err)
	}

	// Verify that the file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Errorf("CreateJSON did not create the JSON file")
	}

	// Clean up the test file
	err = os.Remove(filename)
	if err != nil {
		t.Errorf("Failed to remove the test file: %v", err)
	}
}
func TestOpenFile(t *testing.T) {
	fileName := "test.txt"

	// Create a test file
	file, err := os.Create(fileName)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	file.Close()

	// Open the test file
	openedFile, err := files.OpenFile(fileName)
	if err != nil {
		t.Errorf("OpenFile returned an error: %v", err)
	}

	// Verify that the opened file is not nil
	if openedFile == nil {
		t.Errorf("OpenFile returned a nil file")
	}

	// Clean up the test file
	err = os.Remove(fileName)
	if err != nil {
		t.Errorf("Failed to remove the test file: %v", err)
	}
}
func TestRemoveFile(t *testing.T) {
	fileName := "test.txt"

	// Create a test file
	file, err := os.Create(fileName)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	file.Close()

	// Remove the test file
	err = files.RemoveFile(fileName)
	if err != nil {
		t.Errorf("RemoveFile returned an error: %v", err)
	}

	// Verify that the file does not exist
	if _, err := os.Stat(fileName); !os.IsNotExist(err) {
		t.Errorf("RemoveFile did not remove the file")
	}
}
