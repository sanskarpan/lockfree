package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	password := flag.String("password", "", "Password to hash with bcrypt")
	cost := flag.Int("cost", bcrypt.DefaultCost, "bcrypt cost")
	flag.Parse()

	if *password == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/hashpassword -password <value>")
		os.Exit(2)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(*password), *cost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash password: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(hash))
}
