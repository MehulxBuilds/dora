package main

import (
	"fmt"
	"log"

	dora "github.com/MehulxBuilds/dora/engine"
)

func main() {
	db, err := dora.Open("demo.db")

	if err != nil {
		fmt.Printf("Error: %v", err)
	}
	defer db.Close()

	db.Set("user:1", dora.NewObject(map[string]dora.Value{
		"name": dora.NewString("Mehul"),
		"age":  dora.NewNumber(20),
	}))

	val1, _ := db.Get("user:1")
	
	fmt.Println(val1)

	db.Delete("user:1")

	if _, err = db.Get("user:1"); err != nil {
		log.Printf("Error: %v\n", err)
	}
}
