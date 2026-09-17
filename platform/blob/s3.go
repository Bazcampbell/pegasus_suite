// platform/blob/s3.go
//
// Credentials and region come from the default chain: env, shared config, or
// the instance role. Bucket encryption and IAM are what protect secrets here;
// nothing is field-encrypted.

package blob

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type s3Bucket struct {
	client *s3.Client
	bucket string
	prefix string
}

func OpenS3(ctx context.Context, bucket, prefix string) (Bucket, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &s3Bucket{client: s3.NewFromConfig(cfg), bucket: bucket, prefix: prefix}, nil
}

func (b *s3Bucket) key(k string) string {
	k = strings.TrimPrefix(k, "/")
	if b.prefix == "" {
		return k
	}
	return b.prefix + "/" + k
}

func (b *s3Bucket) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(b.bucket), Key: aws.String(b.key(key))})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (b *s3Bucket) Put(ctx context.Context, key string, body []byte) error {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.key(key)),
		Body:   bytes.NewReader(body),
	})
	return err
}

func (b *s3Bucket) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(b.bucket), Key: aws.String(b.key(key))})
	return err
}

func (b *s3Bucket) List(ctx context.Context, prefix string) ([]string, error) {
	full := b.key(prefix)
	strip := ""
	if b.prefix != "" {
		strip = b.prefix + "/"
	}

	var keys []string
	pages := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{Bucket: aws.String(b.bucket), Prefix: aws.String(full)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range page.Contents {
			keys = append(keys, strings.TrimPrefix(aws.ToString(o.Key), strip))
		}
	}
	return keys, nil
}
