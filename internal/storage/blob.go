package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BlobStorage defines the interface for storing large blob artifacts
type BlobStorage interface {
	// SaveBlob stores a blob and returns its key/path
	SaveBlob(name string, content []byte, extension string) (string, error)
}

type DiskBlobStore struct {
	baseDir string
}

func NewDiskBlobStore(baseDir string) (*DiskBlobStore, error) {
	if baseDir == "" {
		baseDir = "artifacts"
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create blob dir: %w", err)
	}
	return &DiskBlobStore{baseDir: baseDir}, nil
}

func (s *DiskBlobStore) SaveBlob(name string, content []byte, extension string) (string, error) {
	timestamp := time.Now().UTC().Format("20060102-150405")
	safeName := safeFilename(name)
	filename := fmt.Sprintf("%s-%s.%s", safeName, timestamp, extension)
	
	fullPath := filepath.Join(s.baseDir, filename)
	
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		return "", fmt.Errorf("write blob: %w", err)
	}
	
	// Return the relative path or key
	return filename, nil
}

func safeFilename(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return "artifact"
	}
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return -1
	}, s)
	if s == "" {
		return "artifact"
	}
	return s
}
