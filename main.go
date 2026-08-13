package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

var conn *sql.DB

type Auto struct {
	Id    uuid.UUID `json:"id"`
	Model string    `json:"model"`
	Color string    `json:"color"`
}

type AutoFilter struct {
	Model string `json:"model"`
	Color string `json:"color"`
}

func NewAuto(model string, color string) *Auto {
	newUUID := uuid.New()
	return &Auto{
		Id:    newUUID,
		Model: model,
		Color: color,
	}
}

func isRunInDocker() bool {
	file, err := os.Open("/.dockerenv")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	defer file.Close()
	return true
}

func getEvn() {
	if isRunInDocker() {
		return
	}
	file, err := os.Open("./.env")
	if err != nil {
		log.Fatal(err)
	}
	reader := bufio.NewReader(file)
	for true {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		envVar := strings.Split(line, "=")
		key := strings.Trim(envVar[0], " \n\t")
		value := strings.Trim(envVar[1], " \n\t")
		os.Setenv(key, value)
	}
}

func delEnv() {
	os.Unsetenv("POSTGRES_PORT")
	os.Unsetenv("POSTGRES_USER")
	os.Unsetenv("POSTGRES_HOST")
	os.Unsetenv("POSTGRES_PASSWORD")
	os.Unsetenv("POSTGRES_DB")

}

func getConnectDB() *sql.DB {
	getEvn()
	pg_port, err := strconv.Atoi(os.Getenv("POSTGRES_PORT"))
	if err != nil {
		log.Fatal(err)
	}
	pg_config := pq.Config{
		Host:     os.Getenv("POSTGRES_HOST"),
		Port:     uint16(pg_port),
		User:     os.Getenv("POSTGRES_USER"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
		Database: os.Getenv("POSTGRES_DB"),
		SSLMode:  "disable",
	}
	if conn, err := pq.NewConnectorConfig(pg_config); err != nil {
		log.Fatal(err)
	} else {
		return sql.OpenDB(conn)
	}
	delEnv()
	return nil
}

func createTableIfNotExist(tableName string) {
	var exists bool
	query := "SELECT EXISTS (SELECT FROM pg_tables WHERE tablename = $1);"
	err := conn.QueryRow(query, tableName).Scan(&exists)
	if err != nil {
		log.Fatal(err)
	}
	if !exists {
		query = fmt.Sprintf("CREATE TABLE %s (data json);", tableName)
		if _, err = conn.Exec(query); err != nil {
			log.Fatal(err)
		}
	}
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	answer := []byte{'O', 'K', '!'}
	w.WriteHeader(http.StatusOK)
	w.Write(answer)
}

func CreateAuto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method error", 409)
		return
	}
	var filter AutoFilter
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&filter); err != nil {
		http.Error(w, "Invalid json", http.StatusBadRequest)
		return
	}
	if filter.Model == "" || filter.Color == "" {
		http.Error(w, "Invalid json", http.StatusBadRequest)
		return
	}

	newAuto := NewAuto(filter.Model, filter.Color)
	autoJson, err := json.Marshal(newAuto)
	if err != nil {
		log.Fatal(err)
		return
	}
	query := `INSERT INTO auto (data) VALUES ($1)`
	_, err = conn.Exec(query, autoJson)
	if err != nil {
		log.Fatal(err)
	}
}

func GetAuto(w http.ResponseWriter, r *http.Request) {
	model := r.URL.Query().Get("model")
	if model == "" {
		http.Error(w, "Missing model parametr", http.StatusBadRequest)
		return
	}
	var auto []byte
	query := `SELECT data FROM auto WHERE data->>'model' = $1 LIMIT 1;`
	if err := conn.QueryRow(query, model).Scan(&auto); err == sql.ErrNoRows {
		http.Error(w, "Now rows", http.StatusBadRequest)
		return
	}
	w.Write(auto)
}

func GetAutos(w http.ResponseWriter, r *http.Request) {
	query := `SELECT data FROM auto;`
	rows, err := conn.Query(query)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var autos []*Auto = []*Auto{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		var auto Auto
		if err := json.Unmarshal(data, &auto); err != nil {
			http.Error(w, "Unmarshal error", http.StatusInternalServerError)
			return
		}
		autos = append(autos, &auto)
	}
	if len(autos) == 0 {
		w.WriteHeader(http.StatusNotFound)
		http.Error(w, "List auto is blank", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(autos)
}

func main() {
	conn = getConnectDB()
	defer conn.Close()

	createTableIfNotExist("auto")

	http.HandleFunc("/new", CreateAuto)
	http.HandleFunc("/get", GetAutos)
	http.HandleFunc("/get_auto", GetAuto)
	http.HandleFunc("/healthcheck", healthCheck)

	http.ListenAndServe(":8888", nil)
}
