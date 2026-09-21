// platform/store/store.go

// interface for filesystem settings and AWS s3 settings

package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"
)

var ErrNotFound = errors.New("store: not found")

type Bucket interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, body []byte) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Open dials a bucket from a URL:
//
//	file:///var/lib/wagering/data
//	s3://my-bucket/optional/prefix
func Open(ctx context.Context, raw string) (Bucket, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("store: bad url %q: %w", raw, err)
	}

	switch u.Scheme {
	case "file":
		return OpenFS(fsPath(u.Path))
	case "s3":
		return OpenS3(ctx, u.Host, strings.Trim(u.Path, "/"))
	}
	return nil, fmt.Errorf("store: unsupported scheme %q (want file:// or s3://)", u.Scheme)
}

// turns a file URL's path into an OS path. file:///C:/data parses to
// "/C:/data", which Windows cannot open; the drive letter has to lead.
func fsPath(p string) string {
	if runtime.GOOS == "windows" && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		return p[1:]
	}
	return p
}
