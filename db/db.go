package db

import (
	"log"
	"os"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/joho/godotenv"
)

var Driver neo4j.DriverWithContext
var DbName string

func InitNeo4j() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("❌ Error loading .env file")
	}

	uri := os.Getenv("NEO4J_URI")
	username := os.Getenv("NEO4J_USERNAME")
	password := os.Getenv("NEO4J_PASSWORD")
	DbName = os.Getenv("NEO4J_DATABASE")

	Driver, err = neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		log.Fatalf("❌ Failed to create Neo4j driver: %v", err)
	}

	log.Println("✅ Successfully connected to Neo4j Aura")
}
