// triple-s/crypto.go

package triples

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// SHA-256 of an empty body
// Required by SigV4
var emptyPayloadHash = func() string {
	h := sha256.Sum256(nil)
	return hex.EncodeToString(h[:])
}()

func signedWebSocketURL(ctx context.Context, endpoint, region string, creds aws.Credentials) (string, error) {
	rawURL := fmt.Sprintf("wss://%s/mqtt", endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	// AWS IoT requires X-Amz-Expires in the presigned URL — add before signing
	// so the value is part of the canonical query string.
	q := req.URL.Query()
	q.Set("X-Amz-Expires", "86400")
	req.URL.RawQuery = q.Encode()

	signer := v4.NewSigner()
	signedURI, _, err := signer.PresignHTTP(
		ctx,
		creds,
		req,
		emptyPayloadHash,
		"iotdevicegateway",
		region,
		time.Now().UTC(),
	)
	if err != nil {
		return "", fmt.Errorf("presign: %w", err)
	}

	return signedURI, nil
}
