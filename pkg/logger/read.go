package logger

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ReadLastNLogLines returns the last n entries of each log file in path, the
// directory of the file output, for the administrators' log view. A missing
// file is skipped; an error is returned only when no log file is readable,
// as when the logs go to the console.
func ReadLastNLogLines(path string, n int) ([]string, error) {
	if n <= 0 {
		return []string{}, nil
	}
	filenames := []string{accessFilename, errorFilename, slowFilename}
	lines := make([]string, 0, n*len(filenames))
	var readErrs []error
	readable := false
	for _, filename := range filenames {
		fileLines, err := readLastNLinesFromFile(filepath.Join(path, filename), n)
		if err != nil {
			if !os.IsNotExist(err) {
				readErrs = append(readErrs, fmt.Errorf("%s: %w", filename, err))
			}
			continue
		}
		readable = true
		lines = append(lines, fileLines...)
	}
	if !readable {
		if len(readErrs) > 0 {
			return nil, errors.Join(readErrs...)
		}
		return nil, os.ErrNotExist
	}
	return lines, nil
}

func readLastNLinesFromFile(filename string, n int) ([]string, error) {
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	// The file is only read; closing it cannot lose data.
	defer func() { _ = file.Close() }()

	// Get file size
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := fileInfo.Size()

	// If file is empty, return empty slice
	if fileSize == 0 {
		return []string{}, nil
	}

	// Buffer for reading
	bufferSize := int64(4096)
	if bufferSize > fileSize {
		bufferSize = fileSize
	}
	buffer := make([]byte, bufferSize)

	// Start reading from the end
	position := fileSize
	lines := make([]string, 0, n)
	lineCount := 0

	for lineCount < n && position > 0 {
		// How much to read
		readSize := bufferSize
		if position < bufferSize {
			readSize = position
		}
		position -= readSize

		// Read the chunk at position; ReadAt fills the buffer or fails, where
		// a plain Read may return a short chunk.
		if _, err := file.ReadAt(buffer[:readSize], position); err != nil {
			return nil, err
		}

		// Count newlines in reverse
		for i := readSize - 1; i >= 0; i-- {
			if buffer[i] == '\n' {
				lineCount++
				if lineCount > n {
					// We found more than n lines
					// Need to adjust position to read only last n lines
					position += i + 1
					break
				}
			}
		}
	}

	// If we couldn't find n lines, start from beginning
	if position < 0 {
		position = 0
	}

	// Seek to the position where we want to start reading
	_, err = file.Seek(position, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// Read lines from position to end
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Check if we need to trim
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return lines, scanner.Err()
}
