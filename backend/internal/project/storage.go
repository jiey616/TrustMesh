package project

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FileStorage abstracts file storage for project files.
type FileStorage interface {
	Save(ctx context.Context, projectID, fileID, filename string, r io.Reader) (uri string, err error)
	CopyFromPath(ctx context.Context, srcPath, projectID, taskID, agentID, filename string) (uri string, err error)
	Get(ctx context.Context, uri string) (io.ReadCloser, error)
	Delete(ctx context.Context, uri string) error
}

// LocalFileStorage stores files on the local filesystem.
type LocalFileStorage struct {
	basePath string
}

// NewLocalFileStorage creates a new LocalFileStorage rooted at basePath.
func NewLocalFileStorage(basePath string) *LocalFileStorage {
	return &LocalFileStorage{basePath: basePath}
}

// Save stores a user-uploaded file. The file is written to:
//
//	{basePath}/{projectID}/uploads/{fileID}_{filename}
func (s *LocalFileStorage) Save(_ context.Context, projectID, fileID, filename string, r io.Reader) (string, error) {
	dir := filepath.Join(s.basePath, projectID, "uploads")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	// Use fileID prefix to avoid collision on same filename.
	diskName := fileID + "_" + filename
	path := filepath.Join(dir, diskName)
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return path, nil
}

// CopyFromPath copies a file from srcPath into the project files directory under:
//
//	{basePath}/{projectID}/tasks/{taskID}/artifacts/{agentID}/{filename}
//
// This is used to persist agent artifacts from the transfer volume into the
// project file volume.
func (s *LocalFileStorage) CopyFromPath(_ context.Context, srcPath, projectID, taskID, agentID, filename string) (string, error) {
	if _, err := os.Stat(srcPath); err != nil {
		return "", fmt.Errorf("source file not accessible: %w", err)
	}

	dir := filepath.Join(s.basePath, projectID, "tasks", taskID, "artifacts", agentID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	dstPath := filepath.Join(dir, filename)

	src, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return "", fmt.Errorf("create destination: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("copy file: %w", err)
	}

	return dstPath, nil
}

// Get opens a file by its absolute URI path for reading.
func (s *LocalFileStorage) Get(_ context.Context, uri string) (io.ReadCloser, error) {
	f, err := os.Open(uri)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return f, nil
}

// Delete removes the physical file from disk.
// Uses RemoveAll to also clean up empty parent directories if desired,
// but here we target the specific file path.
func (s *LocalFileStorage) Delete(_ context.Context, uri string) error {
	// Remove the specific file first.
	if err := os.Remove(uri); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove file: %w", err)
	}
	// Clean up empty parent directories up to the project root.
	dir := filepath.Dir(uri)
	for dir != s.basePath && dir != filepath.Dir(dir) {
		if err := os.Remove(dir); err != nil {
			break // directory not empty or other error, stop cleaning
		}
		dir = filepath.Dir(dir)
	}
	return nil
}
