package main

import (
	"math/rand"
	"time"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"gitlab.com/acnodal/epic/resource-model/cmd"
)

func init() {
	// Seed the RNG so we can generate pseudo-random tunnel authn keys
	rand.Seed(time.Now().UTC().UnixNano())
}

func main() {
	cmd.Execute()
}
