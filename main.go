package main

import (
	"context"

	"github.com/tsumida/tex/pkg"
)

func main() {
	var (
		ctx = context.Background()
	)
	pkg.RunApp(ctx)
}
