package main

import (
	"log"

	"github.com/Andreyka-coder9192/calc_go/internal/application"
)

func main() {
	app := application.NewOrchestrator()
	log.Println("Starting Orchestrator on port", app.Config.Addr)
	if err := app.RunServer(); err != nil {
		log.Fatal(err)
	}
}
