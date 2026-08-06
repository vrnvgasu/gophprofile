package main

import (
	"log"

	"github.com/vrnvgasu/gophprofile/internal/config"
)

func parseConfig() *config.Config {
	cfg, err := config.Parse()
	if err != nil {
		log.Fatalf("config.Parse: %v", err)
	}

	return cfg
}
