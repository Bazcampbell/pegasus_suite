// platform/blob/fs.go

// filesystem process settings storage

package blob

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type fsBucket struct {
	root string
}

func OpenFS(root string) (Bucket, error) {
	if root == "" {
		return nil, fmt.Errorf("blob: file url needs a path")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &fsBucket{root: root}, nil
}

func (b *fsBucket) path(key string) (string, error) {
	clean := filepath.Clean("/" + key)
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("blob: bad key %q", key)
	}
	return filepath.Join(b.root, filepath.FromSlash(clean)), nil
}

func (b *fsBucket) Get(_ context.Context, key string) ([]byte, error) {
	p, err := b.path(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return data, err
}

// writes to a temp file, then renames
// reader can never see an in-progress write
func (b *fsBucket) Put(_ context.Context, key string, body []byte) error {
	p, err := b.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func (b *fsBucket) Delete(_ context.Context, key string) error {
	p, err := b.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (b *fsBucket) List(_ context.Context, prefix string) ([]string, error) {
	dir, err := b.path(prefix)
	if err != nil {
		return nil, err
	}

	var keys []string
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".tmp-") {
			return nil
		}
		rel, err := filepath.Rel(b.root, p)
		if err != nil {
			return err
		}
		keys = append(keys, filepath.ToSlash(rel))
		return nil
	})
	return keys, err
}
