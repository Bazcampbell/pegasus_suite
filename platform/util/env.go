// util/env.go

package util

import (
	"log"
	"os"
)

func MustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing env var: %s", k)
	}
	return v
}
