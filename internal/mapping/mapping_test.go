package mapping

import (
	"encoding/csv"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadHandleMap(t *testing.T) {
	filePath := "test.csv"

	// Create a test CSV file
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer file.Close()

	// Write test data to the CSV file
	data := [][]string{
		{"key1", "value1"},
		{"key2", "value2"},
	}
	writer := csv.NewWriter(file)
	for _, record := range data {
		err := writer.Write(record)
		if err != nil {
			t.Fatalf("Failed to write to test file: %v", err)
		}
	}
	writer.Flush()

	// Load handle map from the test file
	handleMap, err := loadHandleMap(filePath)
	if err != nil {
		t.Errorf("loadHandleMap returned an error: %v", err)
	}

	// Verify the loaded handle map
	expectedHandleMap := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	if !reflect.DeepEqual(handleMap, expectedHandleMap) {
		t.Errorf("Loaded handle map does not match the expected handle map")
	}

	// Clean up the test file
	err = os.Remove(filePath)
	if err != nil {
		t.Errorf("Failed to remove the test file: %v", err)
	}
}
func TestModifyReleaseBody(t *testing.T) {
	releaseBody := "This is a test release body made by @naruto on https://example.com/source-org/repo/"
	filePath := "test.csv"

	// Create a test CSV file
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer file.Close()

	// Write test data to the CSV file
	data := [][]string{
		{"source", "target"},
		{"naruto", "naruto.uzumaki"},
	}
	writer := csv.NewWriter(file)
	for _, record := range data {
		err := writer.Write(record)
		if err != nil {
			t.Fatalf("Failed to write to test file: %v", err)
		}
	}
	writer.Flush()

	// Set up viper configuration
	viper.Set("SOURCE_HOSTNAME", "example.com")
	viper.Set("SOURCE_ORGANIZATION", "source-org")
	viper.Set("TARGET_ORGANIZATION", "target-org")

	// Modify the release body
	updatedReleaseBody, err := ModifyReleaseBody(&releaseBody, filePath)

	if err != nil {
		t.Errorf("ModifyReleaseBody returned an error: %v", err)
	}
	expectedReleaseBody := releaseBody
	expectedReleaseBody = strings.ReplaceAll(expectedReleaseBody, "example.com", "github.com")
	expectedReleaseBody = strings.ReplaceAll(expectedReleaseBody, "source-org", "target-org")
	expectedReleaseBody = strings.ReplaceAll(expectedReleaseBody, "naruto", "naruto.uzumaki")
	expectedReleaseBody = strings.ReplaceAll(expectedReleaseBody, "source", "target")

	if *updatedReleaseBody != expectedReleaseBody {
		t.Errorf("Modified release body does not match the expected release body")
	}

	// Clean up the test file
	err = os.Remove(filePath)
	if err != nil {
		t.Errorf("Failed to remove the test file: %v", err)
	}
}

func TestModifyReleaseBodyWithNilBody(t *testing.T) {
	var releaseBody *string
	filePath := "test.csv"

	// Create a test CSV file
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer file.Close()

	// Write test data to the CSV file
	data := [][]string{
		{"source", "target"},
		{"naruto", "naruto.uzumaki"},
	}
	writer := csv.NewWriter(file)
	for _, record := range data {
		err := writer.Write(record)
		if err != nil {
			t.Fatalf("Failed to write to test file: %v", err)
		}
	}
	writer.Flush()

	// Set up viper configuration
	viper.Set("SOURCE_HOSTNAME", "example.com")
	viper.Set("SOURCE_ORGANIZATION", "source-org")
	viper.Set("TARGET_ORGANIZATION", "target-org")

	// Modify the release body
	updatedReleaseBody, err := ModifyReleaseBody(releaseBody, filePath)

	if err != nil {
		t.Errorf("ModifyReleaseBody returned an error: %v", err)
	}

	if updatedReleaseBody != nil && *updatedReleaseBody != "" {
		t.Errorf("Modified release body is not nil")
	}

	// Clean up the test file
	err = os.Remove(filePath)
	if err != nil {
		t.Errorf("Failed to remove the test file: %v", err)
	}
}

// func TestAddSourceTimeStampsWithNilBody(t *testing.T) {
// 	release := &github.RepositoryRelease{}
// 	updatedRelease, err := AddSourceTimeStamps(release)
// 	if err != nil {
// 		t.Errorf("AddSourceTimeStamps returned an error: %v", err)
// 	}
// 	if updatedRelease.Body == nil {
// 		t.Errorf("Updated release body is nil")
// 	}
// }

// func TestAddSourceTimeStampsWithNilCreatedAt(t *testing.T) {
// 	release := &github.RepositoryRelease{
// 		Body: github.String("Test release body"),
// 	}
// 	updatedRelease, err := AddSourceTimeStamps(release)
// 	if err != nil {
// 		t.Errorf("AddSourceTimeStamps returned an error: %v", err)
// 	}
// 	if !strings.Contains(*updatedRelease.Body, "Release Originally Created on:") {
// 		t.Errorf("Updated release body does not contain the expected created at timestamp")
// 	}
// }

// func TestAddSourceTimeStampsWithNilPublishedAt(t *testing.T) {
// 	release := &github.RepositoryRelease{
// 		Body:      github.String("Test release body"),
// 		CreatedAt: &github.Timestamp{Time: time.Now()},
// 	}
// 	updatedRelease, err := AddSourceTimeStamps(release)
// 	if err != nil {
// 		t.Errorf("AddSourceTimeStamps returned an error: %v", err)
// 	}
// 	if !strings.Contains(*updatedRelease.Body, "Release Originally Published on:") {
// 		t.Errorf("Updated release body does not contain the expected published at timestamp")
// 	}
// }
