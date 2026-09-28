package storage

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupInfo describes a backup archive.
type BackupInfo struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
}

// CreateArchive creates a timestamped backup of all QIX data.
func (s *Storage) CreateArchive() (string, error) {
	if err := s.FlushAll(); err != nil {
		return "", err
	}
	path := filepath.Join(s.config.BackupDir, "qix_backup_"+time.Now().Format("20060102_150405.000000000")+".tar.gz")
	if err := s.ExportBackup(path); err != nil {
		return "", err
	}
	_, _ = s.CleanupBackups()
	return path, nil
}

// ListBackups returns QIX backups ordered newest first.
func (s *Storage) ListBackups() ([]BackupInfo, error) {
	files, err := filepath.Glob(filepath.Join(s.config.BackupDir, "qix_backup_*.tar.gz"))
	if err != nil {
		return nil, err
	}
	result := make([]BackupInfo, 0, len(files))
	for _, path := range files {
		info, err := os.Stat(path)
		if err == nil {
			result = append(result, BackupInfo{Path: path, Name: filepath.Base(path), Size: info.Size(), ModTime: info.ModTime()})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ModTime.After(result[j].ModTime) })
	return result, nil
}

// ExportBackup writes a complete QIX data archive to targetPath.
func (s *Storage) ExportBackup(targetPath string) error {
	if !strings.HasSuffix(targetPath, ".tar.gz") {
		targetPath += ".tar.gz"
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
		return err
	}
	targetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}
	out, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	succeeded := false
	defer func() {
		_ = out.Close()
		if !succeeded {
			_ = os.Remove(targetPath)
		}
	}()
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	err = filepath.Walk(s.config.QixDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if path == s.config.BackupDir {
			return filepath.SkipDir
		}
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if absolutePath == targetPath {
			return nil
		}
		if strings.HasSuffix(path, ".tmp") {
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name, err = filepath.Rel(filepath.Dir(s.config.QixDir), path)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err == nil {
		err = tw.Close()
	} else {
		_ = tw.Close()
	}
	if err == nil {
		err = gz.Close()
	} else {
		_ = gz.Close()
	}
	if err == nil {
		err = out.Close()
	}
	if err != nil {
		return err
	}
	succeeded = true
	return nil
}

// RestoreBackup restores a QIX archive after first creating a safety backup.
func (s *Storage) RestoreBackup(sourcePath string) (string, error) {
	safetyPath, err := s.CreateArchive()
	if err != nil {
		return "", fmt.Errorf("create safety backup: %w", err)
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return safetyPath, err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return safetyPath, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	qixRoot := filepath.Clean(s.config.QixDir)
	rootPrefix := qixRoot + string(os.PathSeparator)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return safetyPath, err
		}
		target := filepath.Clean(filepath.Join(filepath.Dir(s.config.QixDir), header.Name))
		if target != qixRoot && !strings.HasPrefix(target, rootPrefix) {
			return safetyPath, fmt.Errorf("archive contains unsafe path %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0700); err != nil {
				return safetyPath, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return safetyPath, err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode)&0777)
			if err != nil {
				return safetyPath, err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return safetyPath, copyErr
			}
			if closeErr != nil {
				return safetyPath, closeErr
			}
		}
	}
	s.ClearCache()
	return safetyPath, s.RebuildIndex()
}

// CleanupBackups removes archives older than the configured retention period.
func (s *Storage) CleanupBackups() (int, error) {
	backups, err := s.ListBackups()
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().AddDate(0, 0, -s.config.BackupRetentionDays)
	removed := 0
	for _, backup := range backups {
		if backup.ModTime.Before(cutoff) {
			if err := os.Remove(backup.Path); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}
