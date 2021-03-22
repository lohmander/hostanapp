package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/url"

	"github.com/iancoleman/strcase"
	_ "github.com/jackc/pgx/v4/stdlib"
	"google.golang.org/grpc"

	"github.com/lohmander/hostanapp/contrib/database/utils"
	provider "github.com/lohmander/hostanapp/provider"
)

// DatabaseServer implements the ProviderServer interface
type DatabaseServer struct {
	provider.UnimplementedProviderServer

	ProviderDatabase string
	SecretKey        []byte
	Driver           string
	DBURL            string
}

func (srv *DatabaseServer) getDB(dbName ...string) (*sql.DB, error) {
	URL, err := url.Parse(srv.DBURL)

	if err != nil {
		log.Printf("error: failed db connection string %v", err)
		return nil, err
	}

	if len(dbName) > 0 {
		URL.Path = dbName[0]
	}

	return sql.Open(srv.Driver, URL.String())
}

func (srv *DatabaseServer) checkExists(tableName, columnName, value string) (bool, error) {
	db, err := srv.getDB()

	if err != nil {
		log.Printf("error: failed to get db %v", err)
		return false, err
	}

	defer db.Close()

	var exists string
	row := db.QueryRow(fmt.Sprintf("SELECT 1 FROM %s WHERE %s = $1", tableName, columnName), value)

	if err := row.Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}

		log.Printf("error: failed %v", err)
		return false, err
	}

	return exists == "1", nil
}

func (srv *DatabaseServer) initPasswordDB() error {
	db, err := srv.getDB(srv.ProviderDatabase)

	if err != nil {
		log.Printf("error: failed to get db %v", err)
		return err
	}

	defer db.Close()

	query := `
		CREATE TABLE IF NOT EXISTS "public"."hostanapp_passwords" (
			"id" serial,
			"name" varchar(250) NOT NULL,
			"password" varchar(250) NOT NULL,
			PRIMARY KEY ("id")
		);
	`
	_, err = db.Exec(query)

	if err != nil {
		log.Printf("error: failed to create password table %v", err)
		return err
	}

	log.Printf("initiated provider database %s\n", srv.ProviderDatabase)

	return nil
}

func (srv *DatabaseServer) getPassword(name string) (*string, error) {
	db, err := srv.getDB(srv.ProviderDatabase)

	if err != nil {
		log.Printf("error: failed to get db %v", err)
		return nil, err
	}

	defer db.Close()

	row := db.QueryRow("SELECT password FROM hostanapp_passwords WHERE name = $1", name)

	var password string

	if err := row.Scan(&password); err != nil {
		if err == sql.ErrNoRows {
			plainPassword := utils.GeneratePassword(30, 5, 5, 5)
			encryptedPassword := utils.Encrypt(srv.SecretKey, plainPassword)

			if _, err := db.Exec("INSERT INTO hostanapp_passwords (name, password) VALUES ($1, $2)", name, encryptedPassword); err != nil {
				log.Printf("error: failed to save password %v", err)
				return nil, err
			}

			password = encryptedPassword
		} else {
			log.Printf("error: failed to get password %v", err)
			return nil, err
		}
	}

	password = utils.Decrypt(srv.SecretKey, password)

	return &password, nil
}

func (srv *DatabaseServer) dropPassword(name string) error {
	db, err := srv.getDB(srv.ProviderDatabase)

	if err != nil {
		log.Printf("error: failed to get db %v", err)
		return err
	}

	defer db.Close()

	_, err = db.Exec("DELETE FROM hostanapp_passwords WHERE name = $1", name)
	return err
}

func (srv *DatabaseServer) ProvisionAppConfig(ctx context.Context, req *provider.ProvisionAppConfigRequest) (*provider.ProvisionAppConfigResponse, error) {
	log.Printf("provisioning database for %s", req.AppName)

	db, err := srv.getDB()

	if err != nil {
		log.Printf("error: failed to get db %v", err)
		return nil, err
	}

	defer db.Close()

	dbName := strcase.ToSnake(req.AppName)
	dbUser := dbName
	dbExists, err := srv.checkExists("pg_database", "datname", dbName)

	if err != nil {
		log.Printf("error: failed %v", err)
		return nil, err
	}

	if !dbExists {
		_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))

		if err != nil {
			log.Printf("error: failed to create db %v", err)
			return nil, err
		}

		log.Printf("created new database %s\n", dbName)
	} else {
		log.Printf("database %s already exists\n", dbName)
	}

	roleExists, err := srv.checkExists("pg_roles", "rolname", dbName)

	if err != nil {
		log.Printf("error: failed %v", err)
		return nil, err
	}

	var password *string

	if !roleExists {
		password, err = srv.getPassword(req.AppName)

		if err != nil {
			return nil, err
		}

		_, err = db.Exec(fmt.Sprintf("CREATE ROLE %s WITH ENCRYPTED PASSWORD '%s'", dbUser, *password))

		if err != nil {
			log.Printf("error: failed to create role %v", err)
			return nil, err
		}

		log.Printf("created new role %s\n", dbUser)
	} else {
		log.Printf("role %s already exists\n", dbUser)

		password, err = srv.getPassword(req.AppName)

		if err != nil {
			return nil, err
		}
	}

	_, err = db.Exec(fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s", dbName, dbUser))

	if err != nil {
		log.Printf("error: failed to grant privileges to role %v", err)
		return nil, err
	}

	log.Printf("granted all privileges to %s on %s", dbUser, dbName)

	parsedURL, err := url.Parse(srv.DBURL)

	if err != nil {
		log.Printf("error: failed %v", err)
		return nil, err
	}

	var sslmode string
	connectionQS := parsedURL.Query()

	if connectionQS.Get("ssl") == "true" {
		sslmode = "require"
	} else if connectionQS.Get("sslmode") == "" {
		sslmode = "prefer"
	} else {
		sslmode = connectionQS.Get("sslmode")
	}

	configVars := []*provider.ConfigVariable{
		{Name: "DATABASE_HOST", Value: parsedURL.Hostname()},
		{Name: "DATABASE_PORT", Value: parsedURL.Port()},
		{Name: "DATABASE_NAME", Value: dbName},
		{Name: "DATABASE_USER", Value: dbName},
		{Name: "DATABASE_SSLMODE", Value: sslmode},
		{Name: "DATABASE_PASSWORD", Value: *password, Secret: true},
	}

	return &provider.ProvisionAppConfigResponse{
		ConfigVariables: configVars,
	}, nil
}

func (srv *DatabaseServer) DeprovisionAppConfig(ctx context.Context, req *provider.DeprovisionAppConfigRequest) (*provider.DeprovisionAppConfigResponse, error) {
	var err error

	dbName := strcase.ToSnake(req.AppName)
	dbUser := dbName

	log.Printf("deprovisioning database for %s", req.AppName)

	db, err := srv.getDB()

	if err != nil {
		log.Printf("error: failed to get db %v", err)
		return nil, err
	}

	defer db.Close()

	// Revoke all privileges from role
	_, err = db.Exec(fmt.Sprintf("REVOKE ALL PRIVILEGES ON DATABASE %s FROM %s", dbName, dbUser))

	if err != nil {
		log.Printf("error: failed to revoke privileges to role %v", err)
		return nil, err
	}

	log.Printf("revoked all privileges from %s on %s", dbUser, dbName)

	// Drop role
	_, err = db.Exec(fmt.Sprintf("DROP ROLE %s", dbUser))

	if err != nil {
		log.Printf("error: failed to drop role %v", err)
		return nil, err
	}

	log.Printf("dropped role %s", dbUser)

	// Drop database
	_, err = db.Exec(fmt.Sprintf("DROP DATABASE %s", dbName))

	if err != nil {
		log.Printf("error: failed to drop db %v", err)
		return nil, err
	}

	log.Printf("dropped db %s", dbName)

	// Delete password
	if err := srv.dropPassword(req.AppName); err != nil {
		log.Printf("error: failed to drop password %v", err)
		return nil, err
	}

	log.Printf("dropped password record for %s", dbName)

	return &provider.DeprovisionAppConfigResponse{}, nil
}

func (srv *DatabaseServer) Serve(port int) {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	log.Printf("started with driver %s", srv.Driver)
	log.Printf("listening on %d", port)

	if err := srv.initPasswordDB(); err != nil {
		log.Fatalf("error: %v", err)
	}

	var opts []grpc.ServerOption

	server := grpc.NewServer(opts...)
	provider.RegisterProviderServer(server, srv)
	server.Serve(lis)
}
