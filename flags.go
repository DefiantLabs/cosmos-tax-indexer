package main

import (
	"flag"
	"io"
)

func ParseArgs(w io.Writer, args []string) (Config, error) {

	c := Config{}
	fs := flag.NewFlagSet("config", flag.ContinueOnError)

	fs.SetOutput(w)

	fs.StringVar(&c.Database.Host, "db-host", "", "The PostgreSQL hostname for the indexer db")
	fs.StringVar(&c.Database.Database, "db", "", "The PostgreSQL database for the indexer db")
	fs.StringVar(&c.Database.Port, "db-port", "", "The PostgreSQL port for the indexer db")
	fs.StringVar(&c.Database.Password, "db-password", "", "The PostgreSQL user password for the indexer db")
	fs.StringVar(&c.Database.User, "db-user", "", "The PostgreSQL user for the indexer db")
	fs.StringVar(&c.ConfigFileLocation, "config", "", "The file to load for configuration variables")

	err := fs.Parse(args)
	if err != nil {
		return c, err
	}

	return c, nil

}
