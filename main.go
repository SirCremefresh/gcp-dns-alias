package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
	"log"
	"os"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file", err)
	}

	ctx := context.Background()

	projectId := os.Getenv("PROJECT_ID")
	credentialsJSON := os.Getenv("CREDENTIALS_JSON")

	service, err := dns.NewService(ctx, option.WithCredentialsJSON([]byte(credentialsJSON)))

	if err != nil {
		log.Fatal("could not create dns service", err)
	}

	do, err := service.ManagedZones.List(projectId).Do()
	if err != nil {
		fmt.Println("could not get managed zones")
		fmt.Println(err)
		os.Exit(1)
	}
	for _, p := range do.ManagedZones {
		fmt.Printf("%+v\n", p.DnsName)
	}
}
