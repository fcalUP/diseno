package main

import (
	_ "github.com/aws/aws-sdk-go-v2/config"
	_ "github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/jackc/pgx/v5/pgxpool"
	_ "golang.org/x/crypto/bcrypt"
	_ "golang.org/x/oauth2/google"
	_ "golang.org/x/text/unicode/norm"
)

func main() {}
