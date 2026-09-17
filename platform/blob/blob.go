// platform/blob/blob.go
//
// One small object-store interface with two backends: a directory on disk for
// development and S3 for real. Settings documents and the log archive both
// live behind it, so the layout is written once.

package blob

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var ErrNotFound = errors.New("blob: not found")

type Bucket interface {
	// Get returns the object, or ErrNotFound.
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, body []byte) error
	Delete(ctx context.Context, key string) error

	// List returns every key under prefix.
	List(ctx context.Context, prefix string) ([]string, error)
}

// Open dials a bucket from a URL:
//
//	file:///var/lib/wagering/data
//	s3://my-bucket/optional/prefix
func Open(ctx context.Context, raw string) (Bucket, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("blob: bad url %q: %w", raw, err)
	}

	switch u.Scheme {
	case "file":
		return OpenFS(u.Path)
	case "s3":
		return OpenS3(ctx, u.Host, strings.Trim(u.Path, "/"))
	}
	return nil, fmt.Errorf("blob: unsupported scheme %q (want file:// or s3://)", u.Scheme)
}
