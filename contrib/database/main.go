package main

import (
	"flag"
	"log"
	"os"
	"strconv"

	srv "github.com/lohmander/hostanapp/contrib/database/server"
)

var PORT = os.Getenv("PORT")

func main() {
	port, err := strconv.Atoi(PORT)

	if err != nil {
		port = 5000
	}

	driver := flag.String("driver", "", "which database driver to use, eg. 'pgx'")
	dbURL := flag.String("db-url", "", "a connection string to the db")
	providerDB := os.Getenv("DATABASE_PROVIDER_DB")
	secretKey := os.Getenv("SECRET_KEY")

	flag.Parse()

	if *driver == "" {
		log.Fatalln("you must specify a database driver")
	}

	if *dbURL == "" {
		log.Fatalln("you must specify a database connection string")
	}

	if providerDB == "" {
		log.Fatalln("you must specify a provider db")
	}

	if len(secretKey) != 32 {
		log.Fatalln("you must specify a secret key of 32 characters, got", len(secretKey))
	}

	postgresqlServer := &srv.DatabaseServer{ProviderDatabase: providerDB, SecretKey: []byte(secretKey), Driver: *driver, DBURL: *dbURL}
	postgresqlServer.Serve(port)
}
