// util/env.go

package util

import (
	"log"
	"os"
	"strconv"
)

func MustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing env var: %s", k)
	}
	return v
}

func MustEnvInt64(k string) int64 {
	v := MustEnv(k)
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Fatalf("env var %s must be an integer, got %q", k, v)
	}
	return n
}
