package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/migration"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	var (
		configPath string
		action     string
		dir        string
		version    int
	)

	flag.StringVar(&configPath, "config", "configs/config.yaml", "path to config file")
	flag.StringVar(&action, "action", "up", "action: up, down, version, force")
	flag.StringVar(&dir, "dir", "migrations", "migrations directory")
	flag.IntVar(&version, "version", 0, "version to force")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	dsn := cfg.MySQL.DSN()

	switch action {
	case "up":
		ver, dirty, err := migration.RunUp(dsn, dir)
		if err != nil {
			log.Fatalf("migrate up failed: %v", err)
		}
		fmt.Printf("Migrate UP successful! Current version: %d, Dirty: %v\n", ver, dirty)

	case "down":
		ver, dirty, err := migration.RunDown(dsn, dir)
		if err != nil {
			log.Fatalf("migrate down failed: %v", err)
		}
		fmt.Printf("Migrate DOWN successful! Current version: %d, Dirty: %v\n", ver, dirty)

	case "version":
		ver, dirty, err := migration.Version(dsn, dir)
		if err != nil {
			log.Fatalf("get version failed: %v", err)
		}
		fmt.Printf("Current version: %d, Dirty: %v\n", ver, dirty)

	case "force":
		if version <= 0 {
			log.Fatalf("version must be > 0 for force action")
		}
		if err := migration.Force(dsn, dir, version); err != nil {
			log.Fatalf("force version failed: %v", err)
		}
		fmt.Printf("Force set to version %d successfully\n", version)

	default:
		log.Fatalf("unknown action: %s (supported: up, down, version, force)", action)
		os.Exit(1)
	}
}
