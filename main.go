package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/qri-io/jsonschema"
	"log"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type ResourceType struct {
	ID     int             `json:"id"`
	Name   string          `json:"name" validate:"required"`
	Schema json.RawMessage `json:"schema" validate:"required"` // Raw JSON schema
}

type ResourceData struct {
	ID             int             `json:"id"`
	ResourceTypeID int             `json:"resource_type_id"`
	Data           json.RawMessage `json:"data"`
}

var (
	db       *sql.DB
	validate *validator.Validate
)

func initDB() {
	var err error
	connStr := "host=0.0.0.0 port=5432 user=postgres password=secret dbname=testdb sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS resource_types (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) UNIQUE NOT NULL,
			schema JSONB NOT NULL
		);

		CREATE TABLE IF NOT EXISTS resource_data (
			id SERIAL PRIMARY KEY,
			resource_type_id INT NOT NULL REFERENCES resource_types(id),
			data JSONB NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	log.Println("Database initialized successfully.")
}

func main() {
	initDB()
	defer db.Close()
	validate = validator.New()

	router := mux.NewRouter()
	router.HandleFunc("/resource-types", createResourceType).Methods("POST")
	router.HandleFunc("/resource-data/{resource_type_name}", validateAndStoreResourceData).Methods("POST")

	log.Println("Starting server on :8080...")
	log.Fatal(http.ListenAndServe(":8080", router))
}

// createResourceType handles adding new resource types
func createResourceType(w http.ResponseWriter, r *http.Request) {
	var resourceType ResourceType
	if err := json.NewDecoder(r.Body).Decode(&resourceType); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if err := validate.Struct(resourceType); err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}

	// Insert into the database
	query := `INSERT INTO resource_types (name, schema) VALUES ($1, $2) RETURNING id`
	err := db.QueryRow(query, resourceType.Name, resourceType.Schema).Scan(&resourceType.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to insert resource type: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resourceType)
}

// validateAndStoreResourceData validates incoming data against a stored resource type
func validateAndStoreResourceData(w http.ResponseWriter, r *http.Request) {
	resourceTypeName := mux.Vars(r)["resource_type_name"]

	// Fetch the resource type
	var resourceType ResourceType
	query := `SELECT id, schema FROM resource_types WHERE name = $1`
	err := db.QueryRow(query, resourceTypeName).Scan(&resourceType.ID, &resourceType.Schema)
	if err == sql.ErrNoRows {
		http.Error(w, "Resource type not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Parse incoming data
	var incomingData json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&incomingData); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate data against schema
	if !validateJSONAgainstSchema(resourceType.Schema, incomingData) {
		http.Error(w, "Validation failed: data does not conform to schema", http.StatusBadRequest)
		return
	}

	// Insert validated data
	query = `INSERT INTO resource_data (resource_type_id, data) VALUES ($1, $2) RETURNING id`
	var resourceData ResourceData
	err = db.QueryRow(query, resourceType.ID, incomingData).Scan(&resourceData.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to insert resource data: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resourceData)
}

// validateJSONAgainstSchema validates data against a JSON schema
func validateJSONAgainstSchema(schema, data json.RawMessage) bool {
	// Load the schema
	rs := &jsonschema.Schema{}
	if err := json.Unmarshal(schema, rs); err != nil {
		log.Printf("Failed to parse schema: %v", err)
		return false
	}

	if !checkAdditionalProperties(schema) {
		log.Println("Sanity check failed: 'additionalProperties' is not explicitly set to false.")
		return false
	}

	// Validate the data
	errs, err := rs.ValidateBytes(context.Background(), data)
	if err != nil {
		log.Printf("Validation error: %v", err)
		return false
	}

	// Check for validation errors
	if len(errs) > 0 {
		var buf bytes.Buffer
		for _, err := range errs {
			buf.WriteString(err.Error() + "\n")
		}
		log.Printf("Validation failed:\n%s", buf.String())
		return false
	}

	// Validation successful
	return true
}

func checkAdditionalProperties(schema json.RawMessage) bool {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(schema, &schemaMap); err != nil {
		log.Printf("Failed to parse schema for sanity check: %v", err)
		return false
	}

	// Check the additionalProperties field
	if ap, ok := schemaMap["additionalProperties"]; ok {
		if apBool, isBool := ap.(bool); isBool && !apBool {
			return true // additionalProperties explicitly set to false
		}
	}
	return false
}
