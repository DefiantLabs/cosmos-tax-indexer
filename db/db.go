package db

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DefiantLabs/cosmos-tax-cli-private/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

func GetAddresses(addressList []string, db *gorm.DB) ([]Address, error) {
	// Look up all DB Addresses that match the search
	var addresses []Address
	result := db.Where("address IN ?", addressList).Find(&addresses)
	fmt.Printf("Found %d addresses in the db\n", result.RowsAffected)
	if result.Error != nil {
		config.Log.Error("Error searching DB for addresses.", result.Error)
	}

	return addresses, result.Error
}

// PostgresDbConnect connects to the database according to the passed in parameters
func PostgresDbConnect(host string, port string, database string, user string, password string, level string) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable", host, port, database, user, password)
	gormLogLevel := logger.Silent

	if level == "info" {
		gormLogLevel = logger.Info
	}
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(gormLogLevel)})
}

// PostgresDbConnect connects to the database according to the passed in parameters
func PostgresDbConnectLogInfo(host string, port string, database string, user string, password string) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable", host, port, database, user, password)
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Info)})
}

// MigrateModels runs the gorm automigrations with all the db models. This will migrate as needed and do nothing if nothing has changed.
func MigrateModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&Block{},
		&FailedBlock{},
		&Chain{},
		&Tx{},
		&Fee{},
		&Address{},
		&MessageType{},
		&Message{},
		&TaxableTransaction{},
		&TaxableEvent{},
		&Denom{},
		&DenomUnit{},
		&DenomUnitAlias{},
	)
}

func GetFailedBlocks(db *gorm.DB, chainID uint) []FailedBlock {
	var failedBlocks []FailedBlock
	db.Table("failed_blocks").Where("blockchain_id = ?::int", chainID).Order("height asc").Scan(&failedBlocks)
	return failedBlocks
}

func GetFirstMissingBlockInRange(db *gorm.DB, start, end int64, chainID uint) int64 {
	// Find the highest block we have indexed so far
	currMax := GetHighestIndexedBlock(db, chainID)

	// If this is after the start date, fine the first missing block between the desired start, and the highest we have indexed +1
	if currMax.Height > start {
		end = currMax.Height + 1
	}

	var firstMissingBlock int64
	err := db.Raw(`SELECT s.i AS missing_blocks
						FROM generate_series($1::int,$2::int) s(i)
						WHERE NOT EXISTS (SELECT 1 FROM blocks WHERE height = s.i AND blockchain_id = $3::int AND indexed = true)
						ORDER BY s.i ASC LIMIT 1;`, start, end, chainID).Row().Scan(&firstMissingBlock)
	if err != nil {
		if !strings.Contains(err.Error(), "no rows in result set") {
			config.Log.Fatalf("Unable to find start block. Err: %v", err)
		}
		firstMissingBlock = start
	}

	return firstMissingBlock
}

func GetDBChainID(db *gorm.DB, chain Chain) (uint, error) {
	if err := db.Where(&chain).FirstOrCreate(&chain).Error; err != nil {
		config.Log.Error("Error getting/creating chain DB object.", err)
		return chain.ID, err
	}
	return chain.ID, nil
}

func GetHighestIndexedBlock(db *gorm.DB, chainID uint) Block {
	var block Block
	// this can potentially be optimized by getting max first and selecting it (this gets translated into a select * limit 1)
	db.Table("blocks").Where("blockchain_id = ?::int AND indexed = true", chainID).Order("height desc").First(&block)
	return block
}

func UpsertFailedBlock(db *gorm.DB, blockHeight int64, chainID string, chainName string) error {
	return db.Transaction(func(dbTransaction *gorm.DB) error {
		failedBlock := FailedBlock{Height: blockHeight, Chain: Chain{ChainID: chainID, Name: chainName}}

		if err := dbTransaction.Where(&failedBlock.Chain).FirstOrCreate(&failedBlock.Chain).Error; err != nil {
			config.Log.Error("Error creating chain DB object.", err)
			return err
		}

		if err := dbTransaction.Where(&failedBlock).FirstOrCreate(&failedBlock).Error; err != nil {
			config.Log.Error("Error creating failed block DB object.", err)
			return err
		}
		return nil
	})
}

func IndexNewBlock(db *gorm.DB, blockHeight int64, blockTime time.Time, txs []TxDBWrapper, dbChainID uint) error {
	// consider optimizing the transaction, but how? Ordering matters due to foreign key constraints
	// Order required: Block -> (For each Tx: Signer Address -> Tx -> (For each Message: Message -> Taxable Events))
	// Also, foreign key relations are struct value based so create needs to be called first to get right foreign key ID
	return db.Transaction(func(dbTransaction *gorm.DB) error {
		// remove from failed blocks if exists
		if err := dbTransaction.
			Exec("DELETE FROM failed_blocks WHERE height = ? AND blockchain_id = ?", blockHeight, dbChainID).
			Error; err != nil {
			config.Log.Error("Error updating failed block.", err)
			return err
		}

		// create block if it doesn't exist
		block := BlockOnly{Height: blockHeight, TimeStamp: blockTime, Indexed: true, BlockchainID: dbChainID}
		if err := dbTransaction.
			Where(Block{Height: block.Height, BlockchainID: block.BlockchainID}).
			Assign(Block{Indexed: true, TimeStamp: blockTime}).
			FirstOrCreate(&block).Error; err != nil {
			config.Log.Error("Error getting/creating block DB object.", err)
			return err
		}

		for _, transaction := range txs {
			txOnly := TxOnly{
				Hash:            transaction.Tx.Hash,
				Code:            transaction.Tx.Code,
				BlockID:         block.ID,
				SignerAddressID: nil,
			}

			// store the signer address if there is one
			if transaction.SignerAddress.Address != "" {
				// viewing gorm logs shows this gets translated into a single ON CONFLICT DO NOTHING RETURNING "id"
				if err := dbTransaction.Where(Address{Address: transaction.SignerAddress.Address}).
					FirstOrCreate(&transaction.SignerAddress).
					Error; err != nil {
					config.Log.Error("Error getting/creating signer address for tx.", err)
					return err
				}
				// store created db model in signer address, creates foreign key relation
				txOnly.SignerAddressID = &transaction.SignerAddress.ID
			}

			// store the TX
			if err := dbTransaction.Where(Tx{Hash: txOnly.Hash}).FirstOrCreate(&txOnly).Error; err != nil {
				config.Log.Error("Error creating tx.", err)
				return err
			}

			for _, fee := range transaction.Tx.Fees {
				thisFee := FeeOnly{
					TxID:           txOnly.ID,
					Amount:         fee.Amount,
					DenominationID: fee.Denomination.ID,
				}
				if fee.PayerAddress.Address != "" {
					if err := dbTransaction.Where(Address{Address: fee.PayerAddress.Address}).
						FirstOrCreate(&fee.PayerAddress).
						Error; err != nil {
						config.Log.Error("Error getting/creating fee payer address.", err)
						return err
					}

					// creates foreign key relation.
					thisFee.PayerAddressID = fee.PayerAddress.ID
				} else if fee.PayerAddress.Address == "" {
					return errors.New("fee cannot have empty payer address")
				}

				if fee.Denomination.Base == "" || fee.Denomination.Symbol == "" {
					return fmt.Errorf("denom not cached for base %s and symbol %s", fee.Denomination.Base, fee.Denomination.Symbol)
				}

				// store the Fee //TODO: make sure the denom ID is correct on this....
				if err := dbTransaction.Where(Fee{TxID: thisFee.TxID, DenominationID: thisFee.DenominationID}).FirstOrCreate(&thisFee).Error; err != nil {
					config.Log.Error("Error creating fee.", err)
					return err
				}
			}

			for _, message := range transaction.Messages {
				if message.Message.MessageType.MessageType == "" {
					config.Log.Fatal("Message type not getting to DB")
				}
				if err := dbTransaction.Where(&message.Message.MessageType).FirstOrCreate(&message.Message.MessageType).Error; err != nil {
					config.Log.Error("Error getting/creating message_type.", err)
					return err
				}

				msg := MessageOnly{
					TxID:          txOnly.ID,
					MessageTypeID: message.Message.MessageType.ID,
					MessageIndex:  message.Message.MessageIndex,
				}

				// Store the msg
				if err := dbTransaction.Where(Message{TxID: msg.TxID, MessageTypeID: msg.MessageTypeID, MessageIndex: msg.MessageIndex}).FirstOrCreate(&msg).Error; err != nil {
					config.Log.Error("Error creating message.", err)
					return err
				}

				for _, taxableTx := range message.TaxableTxs {
					thisTaxableTx := TaxableTransactionOnly{
						MessageID:              msg.ID,
						AmountSent:             taxableTx.TaxableTx.AmountSent,
						AmountReceived:         taxableTx.TaxableTx.AmountReceived,
						DenominationSentID:     taxableTx.TaxableTx.DenominationSentID,     // TODO: make sure this is correct
						DenominationReceivedID: taxableTx.TaxableTx.DenominationReceivedID, // TODO: make sure this is correct
					}
					if taxableTx.SenderAddress.Address != "" {
						if err := dbTransaction.Where(&taxableTx.SenderAddress).FirstOrCreate(&taxableTx.SenderAddress).Error; err != nil {
							config.Log.Error("Error getting/creating sender address.", err)
							return err
						}
						// store created db model in sender address, creates foreign key relation
						thisTaxableTx.SenderAddressID = &taxableTx.SenderAddress.ID
					}

					if taxableTx.ReceiverAddress.Address != "" {
						if err := dbTransaction.Where(&taxableTx.ReceiverAddress).FirstOrCreate(&taxableTx.ReceiverAddress).Error; err != nil {
							config.Log.Error("Error getting/creating receiver address.", err)
							return err
						}
						// store created db model in receiver address, creates foreign key relation
						thisTaxableTx.ReceiverAddressID = &taxableTx.ReceiverAddress.ID
					}

					// It is possible to have more than 1 taxable TX for a single msg. In most cases it should only be 1 or 2, but
					// more is possible. Keying off of msg ID and amount may be sufficient....
					if err := dbTransaction.
						Where(TaxableTransaction{
							MessageID:      thisTaxableTx.MessageID,
							AmountSent:     thisTaxableTx.AmountSent,
							AmountReceived: thisTaxableTx.AmountReceived,
						}).
						FirstOrCreate(&thisTaxableTx).Error; err != nil {
						config.Log.Error("Error creating taxable transaction.", err)
						return err
					}
				}
			}
		}

		return nil
	})
}

func UpsertDenoms(db *gorm.DB, denoms []DenomDBWrapper) error {
	return db.Transaction(func(dbTransaction *gorm.DB) error {
		for _, denom := range denoms {
			if err := dbTransaction.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "base"}},
				DoUpdates: clause.AssignmentColumns([]string{"symbol", "name"}),
			}).Create(&denom.Denom).Error; err != nil {
				return err
			}

			for _, denomUnit := range denom.DenomUnits {
				denomUnit.DenomUnit.Denom = denom.Denom

				if err := dbTransaction.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "name"}},
					DoUpdates: clause.AssignmentColumns([]string{"exponent"}),
				}).Create(&denomUnit.DenomUnit).Error; err != nil {
					return err
				}

				for _, denomAlias := range denomUnit.Aliases {
					thisDenomAlias := denomAlias // This is redundant but required for the picky gosec linter
					thisDenomAlias.DenomUnit = denomUnit.DenomUnit
					if err := dbTransaction.Clauses(clause.OnConflict{
						DoNothing: true,
					}).Create(&thisDenomAlias).Error; err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}
