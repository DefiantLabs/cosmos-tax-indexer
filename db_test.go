package main

import (
	"fmt"
	"testing"

	"gorm.io/gorm"
)

//setup does pre-run setup configurations.
//	* Loads the application config from config.tml, cli args and parses/merges
//	* Connects to the database and returns the db object
//	* Returns various values used throughout the application
func db_setup() (*gorm.DB, error) {
	config, err := GetConfig("./config.toml")

	if err != nil {
		fmt.Println("Error opening configuration file", err)
		return nil, err
	}

	db, err := PostgresDbConnectLogInfo(config.Database.Host, config.Database.Port, config.Database.Database, config.Database.User, config.Database.Password)
	if err != nil {
		fmt.Println("Could not establish connection to the database", err)
		return nil, err
	}

	//TODO: create config values for the prefixes here
	//Could potentially check Node info at startup and pass in ourselves?
	setupAddressRegex("juno(valoper)?1[a-z0-9]{38}")
	setupAddressPrefix("juno")

	//run database migrations at every runtime
	MigrateModels(db)

	return db, nil

}

func TestLookupTxForAddresses(t *testing.T) {
	gorm, _ := db_setup()
	taxableEvts, err := GetTaxableEvents([]string{"juno1mt72y3jny20456k247tc5gf2dnat76l4ynvqwl"}, gorm)
	if err != nil || len(taxableEvts) == 0 {
		t.Fatal("FML")
	}
}
