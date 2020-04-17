package main

import (
	"github.com/caddyserver/caddy/caddy/caddymain"

	_ "github.com/hacdias/caddy-webdav"
)

func main() {
	caddymain.Run()
}
