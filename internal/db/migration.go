package db

import (
	"errors"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(migrationsDir string, dbUrl string) error {
	migrator, err := migrate.New("file://"+migrationsDir, dbUrl)
	if err != nil {
		return err
	}
	defer migrator.Close()
	err = migrator.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}
