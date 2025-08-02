package main

import (
	"github.com/flybasist/bmft/internal/postgresql"
	_ "github.com/lib/pq"
)

func main() {
	postgresql.Run()
}
