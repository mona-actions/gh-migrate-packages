package utils

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"	
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pterm/pterm"
)

const (
	maxRequestsPerMinute = 5000  // Define a safe threshold
	maxRequestsPerHour   = 10000 // Define a safe threshold
	cachePath            = "./cache"
)

var (
	mu           sync.Mutex
	requestCount int
	minuteStart  time.Time
	hourStart    time.Time
)

func ResetRequestCounters() {
	mu.Lock()
	defer mu.Unlock()

	requestCount = 0
	minuteStart = time.Now()
	hourStart = time.Now()
}

func ParseUrl(urlStr string) *url.URL {
	parsedUrl, err := url.Parse(urlStr)
	if err != nil {
		panic(err)
	}
	return parsedUrl
}

func RenameFileOccurances(filename, oldScope, newScope string, occurances int) error {

	// Read the file
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	newContent := strings.Replace(string(content), oldScope, newScope, occurances)

	// Write back to file
	err = os.WriteFile(filename, []byte(newContent), 0644)
	if err != nil {
		return err
	}

	return nil
}

func CacheFile(path, content string, overwrite bool) (string, error) {
	path = filepath.Join(cachePath, path)
	// Check if the file exists
	if FileExists(path) && !overwrite {
		return path, nil
	}

	// Create the directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directories: %v", err)
	}

	// Create the file
	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	// Write the content to the file
	_, err = file.WriteString(content)
	if err != nil {
		return "", fmt.Errorf("failed to write to file: %v", err)
	}

	return path, nil
}

func LoadCacheFile(path string) (string, error) {
	path = filepath.Join(cachePath, path)
	// Check if the file exists
	if !FileExists(path) {
		return "", fmt.Errorf("file not found: %s", path)
	}

	// Read the content of the file
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	return string(content), nil
}

// GetListOfUniqueEntries returns unique combinations of specified columns
func GetListOfUniqueEntries(data [][]string, columns []int) [][]string {
	seen := make(map[string][]string)
	var result [][]string

	for _, record := range data {
		// Build subset of columns we want
		subset := make([]string, len(columns))
		for i, col := range columns {
			subset[i] = record[col]
		}

		// Create key for uniqueness check
		key := strings.Join(subset, ":")

		if _, exists := seen[key]; !exists {
			seen[key] = subset
			result = append(result, subset)
		}
	}

	return result
}

// GetFlatListOfColumn returns unique values from a specified column that match all provided filters
func GetFlatListOfColumn(data [][]string, filters map[string]string, targetCol int) []string {
	seen := make(map[string]bool)
	var result []string

	for _, record := range data {
		// Check if record matches all filters
		matches := true
		for col, value := range filters {
			colIdx, _ := strconv.Atoi(col)
			if record[colIdx] != value {
				matches = false
				break
			}
		}

		if matches {
			if !seen[record[targetCol]] {
				seen[record[targetCol]] = true
				result = append(result, record[targetCol])
			}
		}
	}

	return result
}

func EnsureDirExists(path string) error {
	// Create the directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}

	return nil
}

func DownloadFile(url, outputPath, token string) error {
	// Create the directory if it doesn't exist
	if err := EnsureDirExists(outputPath); err != nil {
		pterm.Error.Println("Failed to create directories:", err)
		return err
	}

	client := &http.Client{}

	for {
		// Check and update request count
		if !CanMakeRequest() {
			pterm.Warning.Println("Approaching rate limit. Sleeping for 1 minute...")
			time.Sleep(time.Minute)
			continue
		}

		// Create a new HTTP request
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %v", err)
		}

		if token != "" {
			// Add the authorization header
			req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
		}

		// Perform the HTTP request
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to perform request: %v", err)
		}
		defer resp.Body.Close()
		time.Sleep(500 * time.Millisecond)

		// Check if the response status is OK
		if resp.StatusCode == http.StatusOK {
			// Create the file
			out, err := os.Create(outputPath)
			if err != nil {
				return fmt.Errorf("failed to create file: %v", err)
			}
			defer out.Close()

			// Write the response body to the file
			_, err = io.Copy(out, resp.Body)
			if err != nil {
				return fmt.Errorf("failed to write to file: %v", err)
			}

			return nil
		}

		return fmt.Errorf("failed to download file %s, status: %d, message: %s", url, resp.StatusCode, resp.Status)
	}
}

func UploadFile(url, inputPath, token string) (*http.Response, error) {
	// Open the file
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file stats: %v", err)
	}

	client := &http.Client{}

	for {
		// Check and update request count
		if !CanMakeRequest() {
			pterm.Warning.Println("Approaching rate limit. Sleeping for 1 minute...")
			time.Sleep(time.Minute)
			continue
		}

		// Create a new HTTP request
		req, err := http.NewRequest("PUT", url, file)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %v", err)
		}

		// Add the authorization header
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		req.Header.Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
		if strings.HasSuffix(file.Name(), ".jar") {
			req.Header.Set("Content-Type", "application/java-archive")
		} else if strings.HasSuffix(file.Name(), ".pom") {
			req.Header.Set("Content-Type", "application/xml")
		} else {
			req.Header.Set("Content-Type", "application/octet-stream")
		}

		// Perform the HTTP request
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to perform request: %v", err)
		}
		defer resp.Body.Close()
		time.Sleep(500 * time.Millisecond)
		return resp, nil
	}
}

func CanMakeRequest() bool {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()

	// Reset counts if time windows have passed
	if now.Sub(minuteStart) >= time.Minute {
		minuteStart = now
		requestCount = 0
	}
	if now.Sub(hourStart) >= time.Hour {
		hourStart = now
		requestCount = 0
	}

	// Check against thresholds
	if requestCount >= maxRequestsPerMinute || requestCount >= maxRequestsPerHour {
		return false
	}

	requestCount++
	return true
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func Contains(slice []string, item string) bool {
	for _, s := range slice {
			if s == item {
					return true
			}
	}
	return false
}

func FindMostRecentFile(pattern string) (string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
			return "", err
	}
	
	if len(matches) == 0 {
			return "", fmt.Errorf("no files found matching pattern: %s", pattern)
	}

	// Sort files by modification time, most recent first
	sort.Slice(matches, func(i, j int) bool {
			iInfo, err := os.Stat(matches[i])
			if err != nil {
					return false
			}
			jInfo, err := os.Stat(matches[j])
			if err != nil {
					return false
			}
			return iInfo.ModTime().After(jInfo.ModTime())
	})

	return matches[0], nil
}
