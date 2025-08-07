package main

import (
	"fmt"
	"log"
	"net/http"
	"context"

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

var driver neo4j.DriverWithContext

func main() {
	var err error

	driver, err = neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		log.Fatal("Failed to create driver:", err)
	}
	defer driver.Close(nil)

	router := gin.Default()

	router.GET("/persons", getAllPersons)
	router.POST("/person", createPerson)
	router.GET("/person/:name", getPerson)
	router.PUT("/person/:name", updatePerson)
	router.DELETE("/person/:name", deletePerson)
	router.GET("/persons/city/:city", getPersonsByCity)
	router.GET("/persons/filter", getPersonsByAgeAndPlace)
	router.DELETE("/reset", deleteAllData) // This is api is to delete all data from the database

	log.Println("Server running on http://localhost:8080")
	router.Run(":8080")
}

// GET /persons
func getAllPersons(c *gin.Context) {
	ctx := context.Background()

	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (p:Person)
		OPTIONAL MATCH (p)-[:VISITED]->(pl:Place)
		RETURN p.name, p.age, p.gender, collect(pl.name) as places
	`

	result, err := session.Run(ctx, query, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query database"})
		return
	}

	var people []Person
	for result.Next(ctx) {
		record := result.Record()
		name := record.Values[0].(string)
		var age int
			switch v := record.Values[1].(type) {
			case int64:
				age = int(v)
			case float64:
				age = int(v)
			default:
				age = 0
			}
		age = age
		gender := record.Values[2].(string)
		places := toStringSlice(record.Values[3])

		people = append(people, Person{
			Name:          name,
			Age:           age,
			Gender:        gender,
			PlacesVisited: places,
		})
	}

	c.JSON(http.StatusOK, people)
}

// CREATE Person
func createPerson(c *gin.Context) {
	var person Person
	if err := c.ShouldBindJSON(&person); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "person": person})
		return
	}

	session := driver.NewSession(c, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(c)

	_, err := session.ExecuteWrite(c, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// Create person node
		_, err := tx.Run(c,
			`MERGE (p:Person {name: $name})
			 SET p.age = $age, p.gender = $gender`,
			map[string]interface{}{
				"name":   person.Name,
				"age":    person.Age,
				"gender": person.Gender,
			})
		if err != nil {
			return nil, err
		}

		// Create place nodes and VISITED relationships
		for _, place := range person.PlacesVisited {
			_, err := tx.Run(c,
				`MERGE (pl:Place {name: $place})
				 WITH pl
				 MATCH (p:Person {name: $name})
				 MERGE (p)-[:VISITED]->(pl)`,
				map[string]interface{}{
					"name":  person.Name,
					"place": place,
				})
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
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
		query := `
			MATCH (p:Person {name: $name})
			OPTIONAL MATCH (p)-[:VISITED]->(pl:Place)
			RETURN p.name, p.age, p.gender, collect(pl.name) as places
		`
		res, err := tx.Run(c, query, map[string]interface{}{"name": name})
		if err != nil {
			return nil, err
		}

		if res.Next(c) {
			values := res.Record()
			return Person{
				Name:           values.Values[0].(string),
				Age:            int(values.Values[1].(int64)),
				Gender:         values.Values[2].(string),
				PlacesVisited:  toStringSlice(values.Values[3]),
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
			 SET p.age = $age, p.gender = $gender`,
			map[string]interface{}{
				"name":   name,
				"age":    person.Age,
				"gender": person.Gender,
			})
		if err != nil {
			return nil, err
		}

		// Delete existing VISITED relationships
		_, err = tx.Run(c,
			`MATCH (p:Person {name: $name})-[v:VISITED]->() DELETE v`,
			map[string]interface{}{"name": name})
		if err != nil {
			return nil, err
		}

		// Create new VISITED relationships
		for _, place := range person.PlacesVisited {
			_, err := tx.Run(c,
				`MERGE (pl:Place {name: $place})
				 WITH pl
				 MATCH (p:Person {name: $name})
				 MERGE (p)-[:VISITED]->(pl)`,
				map[string]interface{}{
					"name":  name,
					"place": place,
				})
			if err != nil {
				return nil, err
			}
		}

		return nil, nil
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
		_, err := tx.Run(c,
			`MATCH (p:Person {name: $name}) DETACH DELETE p`,
			map[string]interface{}{"name": name})
		return nil, err
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete person"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Person deleted"})
}

// GET persons who visited a city
func getPersonsByCity(c *gin.Context) {
	city := c.Param("city")
	ctx := context.Background()

	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (p:Person)-[:VISITED]->(pl:Place {name: $city})
		OPTIONAL MATCH (p)-[:VISITED]->(other:Place)
		RETURN p.name, p.age, p.gender, collect(other.name) as places
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
		age := int(record.Values[1].(int64))
		gender := record.Values[2].(string)
		places := toStringSlice(record.Values[3])

		people = append(people, Person{
			Name:          name,
			Age:           age,
			Gender:        gender,
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

// DELETE /reset - Deletes all nodes and relationships
func deleteAllData(c *gin.Context) {
	session := driver.NewSession(c, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(c)

	_, err := session.ExecuteWrite(c, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(c, `MATCH (n) DETACH DELETE n`, nil)
		return nil, err
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete all data"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "All data deleted successfully"})
}


func getPersonsByAgeAndPlace(c *gin.Context) {
	ageParam := c.Query("age")
	place := c.Query("place")

	if ageParam == "" || place == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Both age and place query parameters are required"})
		return
	}

	var age int
	_, err := fmt.Sscanf(ageParam, "%d", &age)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid age parameter"})
		return
	}

	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (p:Person)-[:VISITED]->(:Place {name: $place})
		WHERE p.age >= $age
		OPTIONAL MATCH (p)-[:VISITED]->(pl2:Place)
		RETURN p.name, p.age, p.gender, collect(pl2.name) AS places
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"age":   age,
		"place": place,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query database"})
		return
	}

	var people []Person
	for result.Next(ctx) {
		record := result.Record()
		name := record.Values[0].(string)
		var age int
			switch v := record.Values[1].(type) {
			case int64:
				age = int(v)
			case float64:
				age = int(v)
			default:
				age = 0
			}
		age = age
		gender := record.Values[2].(string)
		places := toStringSlice(record.Values[3])

		people = append(people, Person{
			Name:          name,
			Age:           age,
			Gender:        gender,
			PlacesVisited: places,
		})
	}

	c.JSON(http.StatusOK, people)
}




// {
//     "name": "{{$randomFullName}}",
//     "age": {{$randomInt}},
//     "gender": "male",
//     "places_visited": ["{{$randomCity}}", "{{$randomCity}}", "{{$randomCity}}", "{{$randomCity}}"]
// }