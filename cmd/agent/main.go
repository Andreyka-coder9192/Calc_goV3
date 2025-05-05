package main

import (
	"log"

	"github.com/Andreyka-coder9192/calc_go/internal/application"
)

func main() {
	agent := application.NewAgent()
	log.Println("Starting Agent...")
	agent.Run()
}
