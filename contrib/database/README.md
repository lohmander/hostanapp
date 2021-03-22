# Database Provider

### Running

```sh
go run main.go -driver=pgx -db-url=<connection string>
```

### Required environment variables

| Key                  | Value                                                   |
| -------------------- | ------------------------------------------------------- |
| SECRET_KEY           | A 32 character long string to be used as encryption key |
| DATABASE_PROVIDER_DB | The name a database to use for storing passwords        |
