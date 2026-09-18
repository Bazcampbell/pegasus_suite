// platform/blob/blob.go

// interface for filesystem settings and AWS s3 settings

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
