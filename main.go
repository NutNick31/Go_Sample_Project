package main

import (
	"fmt"
	"log"
	"net/http"
	"context"
	// "os"
	// "strings"

	"github.com/gin-gonic/gin"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Person represents a person node
type Person struct {
	Name           string   `json:"name"`
	Age            int      `json:"age"`
	Gender         string   `json:"gender"`
	PlacesVisited  []string `json:"places_visited"`
}

// Neo4j config (Set your actual credentials here)
var (
	uri      = "neo4j+s://95ae6c1e.databases.neo4j.io"
	username = "neo4j"
	password = "PFsVDeiLmHW2fgUEFZEnXg-gPMthhT1rIGOvsCn-1QA"
	database = "neo4j"
)

// Neo4j driver
var driver neo4j.DriverWithContext

func main() {
	var err error

	// Connect to Neo4j
	driver, err = neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		log.Fatal("Failed to create driver:", err)
	}
	defer driver.Close(nil)

	// Gin router
	router := gin.Default()

	// Routes
	router.GET("/persons", getAllPersons)
	router.POST("/person", createPerson)
	router.GET("/person/:name", getPerson)
	router.PUT("/person/:name", updatePerson)
	router.DELETE("/person/:name", deletePerson)
	router.GET("/persons/city/:city", getPersonsByCity)

	// Start server
	log.Println("Server running on http://localhost:8080")
	router.Run(":8080")
}

// GET /persons
func getAllPersons(c *gin.Context) {
	ctx := context.Background()

	// Create a read session
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Cypher query to get all Person fields
	query := `MATCH (p:Person) RETURN p.name, p.age, p.gender, p.places_visited`

	// Run the query
	result, err := session.Run(ctx, query, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query database"})
		return
	}

	// Collect the results into a slice of Person
	var people []Person
	for result.Next(ctx) {
		record := result.Record()
		var age int
			switch v := record.Values[1].(type) {
			case int64:
				age = int(v)
			case float64:
				age = int(v)
			default:
				age = 0
			}
		name := record.Values[0].(string)
		age = age // Neo4j returns integers as int64
		gender := record.Values[2].(string)

		var places []string
		if record.Values[3] != nil {
			for _, place := range record.Values[3].([]interface{}) {
				places = append(places, place.(string))
			}
		}

		people = append(people, Person{
			Name:          name,
			Age:           age,
			Gender:        gender,
			PlacesVisited: places,
		})
	}

	if err = result.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error reading result"})
		return
	}

	// Send the result as JSON
	c.JSON(http.StatusOK, people)
}



// CREATE Person
func createPerson(c *gin.Context) {
	var person Person
	if err := c.ShouldBindJSON(&person); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "person": person})
		return
	}

	fmt.Println("Error creating person:", person)

	session := driver.NewSession(c, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(c)

	_, err := session.ExecuteWrite(c, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(c,
			`CREATE (p:Person {name: $name, age: $age, gender: $gender, places_visited: $places_visited})`,
			map[string]interface{}{
				"name":            person.Name,
				"age":             person.Age,
				"gender":          person.Gender,
				"places_visited":  person.PlacesVisited,
			})
		return nil, err
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create person"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Person created"})
}

// READ Person
func getPerson(c *gin.Context) {
	name := c.Param("name")

	session := driver.NewSession(c, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(c)

	result, err := session.ExecuteRead(c, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		record, err := tx.Run(c,
			`MATCH (p:Person {name: $name}) RETURN p.name AS name, p.age AS age, p.gender AS gender, p.places_visited AS places_visited`,
			map[string]interface{}{"name": name})
		if err != nil {
			return nil, err
		}

		if record.Next(c) {
			values := record.Record()
			var age int
			switch v := values.Values[1].(type) {
			case int64:
				age = int(v)
			case float64:
				age = int(v)
			default:
				age = 0
			}
			return Person{
				Name:          values.Values[0].(string),
				Age:           age,
				Gender:        values.Values[2].(string),
				PlacesVisited: toStringSlice(values.Values[3]),
			}, nil
		}
		return nil, nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get person"})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "Person not found"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// UPDATE Person
func updatePerson(c *gin.Context) {
	name := c.Param("name")
	var person Person
	if err := c.ShouldBindJSON(&person); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session := driver.NewSession(c, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(c)

	_, err := session.ExecuteWrite(c, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(c,
			`MATCH (p:Person {name: $name})
			 SET p.age = $age, p.gender = $gender, p.places_visited = $places_visited`,
			map[string]interface{}{
				"name":           name,
				"age":            person.Age,
				"gender":         person.Gender,
				"places_visited": person.PlacesVisited,
			})
		return nil, err
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update person"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Person updated"})
}

// DELETE Person
func deletePerson(c *gin.Context) {
	name := c.Param("name")

	session := driver.NewSession(c, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(c)

	_, err := session.ExecuteWrite(c, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(c, `MATCH (p:Person {name: $name}) DELETE p`, map[string]interface{}{"name": name})
		return nil, err
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete person"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Person deleted"})
}

func getPersonsByCity(c *gin.Context) {
	city := c.Param("city")
	ctx := context.Background()

	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (p:Person)
		WHERE $city IN p.places_visited
		RETURN p.name, p.age, p.gender, p.places_visited
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"city": city,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query database"})
		return
	}

	var people []Person
	for result.Next(ctx) {
		record := result.Record()
		name := record.Values[0].(string)
		gender := record.Values[2].(string)
		// values := record.Record()
		var age int
		switch v := record.Values[1].(type) {
		case int64:
			age = int(v)
		case float64:
			age = int(v)
		default:
				age = 80
			}

		places := []string{}
		if record.Values[3] != nil {
			for _, place := range record.Values[3].([]interface{}) {
				places = append(places, place.(string))
			}
		}

		people = append(people, Person{
			Name:          name,
			Gender:        gender,
			Age: 		 age,
			PlacesVisited: places,
		})
	}

	c.JSON(http.StatusOK, people)
}


// Helper: convert interface{} to []string
func toStringSlice(value interface{}) []string {
	raw, ok := value.([]interface{})
	if !ok {
		return []string{}
	}
	strs := make([]string, len(raw))
	for i, v := range raw {
		strs[i] = v.(string)
	}
	return strs
}

// {
//     "name": "{{$randomFullName}}",
//     "age": {{$randomInt}},
//     "gender": "male",
//     "places_visited": ["{{$randomCity}}", "{{$randomCity}}", "{{$randomCity}}", "{{$randomCity}}"]
// }