package cmd

import (
	"fmt"
	"log"

	"github.com/komari-monitor/komari/database/models"
	logutil "github.com/komari-monitor/komari/utils/log"
	"github.com/spf13/cobra"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate data from SQLite to MariaDB/MySQL",
	Long: `Migrate all data from a SQLite database file to a MariaDB/MySQL database.

Example:
  komari migrate \
    --src ./data/komari.db \
    --db-host 127.0.0.1 --db-port 3306 \
    --db-user komari --db-pass secret --db-name komari`,
	Run: func(cmd *cobra.Command, args []string) {
		runMigrate(cmd)
	},
}

var migrateSrc string

func init() {
	migrateCmd.Flags().StringVar(&migrateSrc, "src", "./data/komari.db", "Source SQLite database file path")
	migrateCmd.Flags().String("db-host", GetEnv("KOMARI_DB_HOST", "localhost"), "Target MariaDB/MySQL host [env: KOMARI_DB_HOST]")
	migrateCmd.Flags().String("db-port", GetEnv("KOMARI_DB_PORT", "3306"), "Target MariaDB/MySQL port [env: KOMARI_DB_PORT]")
	migrateCmd.Flags().String("db-user", GetEnv("KOMARI_DB_USER", "komari"), "Target MariaDB/MySQL username [env: KOMARI_DB_USER]")
	migrateCmd.Flags().String("db-pass", GetEnv("KOMARI_DB_PASS", ""), "Target MariaDB/MySQL password [env: KOMARI_DB_PASS]")
	migrateCmd.Flags().String("db-name", GetEnv("KOMARI_DB_NAME", "komari"), "Target MariaDB/MySQL database name [env: KOMARI_DB_NAME]")
	RootCmd.AddCommand(migrateCmd)
}

const migrateBatchSize = 500

func runMigrate(cmd *cobra.Command) {
	host, _ := cmd.Flags().GetString("db-host")
	port, _ := cmd.Flags().GetString("db-port")
	user, _ := cmd.Flags().GetString("db-user")
	pass, _ := cmd.Flags().GetString("db-pass")
	name, _ := cmd.Flags().GetString("db-name")

	logCfg := &gorm.Config{Logger: logutil.NewGormLogger()}

	log.Printf("Opening SQLite source: %s", migrateSrc)
	src, err := gorm.Open(sqlite.Open(migrateSrc), logCfg)
	if err != nil {
		log.Fatalf("Failed to open SQLite source: %v", err)
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&collation=utf8mb4_unicode_ci&parseTime=True&loc=Local",
		user, pass, host, port, name)
	log.Printf("Connecting to MariaDB/MySQL: %s@%s:%s/%s", user, host, port, name)
	dst, err := gorm.Open(mysql.Open(dsn), logCfg)
	if err != nil {
		log.Fatalf("Failed to connect to MariaDB/MySQL: %v", err)
	}

	log.Println("Running AutoMigrate on target database...")
	if err := dst.AutoMigrate(
		&models.User{},
		&models.Session{},
		&models.Client{},
		&models.Record{},
		&models.GPURecord{},
		&models.Log{},
		&models.Clipboard{},
		&models.LoadNotification{},
		&models.OfflineNotification{},
		&models.PingRecord{},
		&models.PingTask{},
		&models.OidcProvider{},
		&models.MessageSenderProvider{},
		&models.ThemeConfiguration{},
		&models.Task{},
		&models.TaskResult{},
		&models.Config{},
	); err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}
	if err := dst.Table("records_long_term").AutoMigrate(&models.Record{}); err != nil {
		log.Printf("records_long_term AutoMigrate: %v", err)
	}
	if err := dst.Table("gpu_records_long_term").AutoMigrate(&models.GPURecord{}); err != nil {
		log.Printf("gpu_records_long_term AutoMigrate: %v", err)
	}

	migrateSimple[models.Config](src, dst, "configs")
	migrateSimple[models.User](src, dst, "users")
	migrateSimple[models.Session](src, dst, "sessions")
	migrateSimple[models.Client](src, dst, "clients")
	migrateSimple[models.OidcProvider](src, dst, "oidc_providers")
	migrateSimple[models.MessageSenderProvider](src, dst, "message_sender_providers")
	migrateSimple[models.ThemeConfiguration](src, dst, "theme_configurations")
	migrateSimple[models.Clipboard](src, dst, "clipboards")
	migrateSimple[models.Log](src, dst, "logs")
	migrateSimple[models.LoadNotification](src, dst, "load_notifications")
	migrateSimple[models.OfflineNotification](src, dst, "offline_notifications")
	migrateSimple[models.PingTask](src, dst, "ping_tasks")
	migrateSimple[models.PingRecord](src, dst, "ping_records")
	migrateSimple[models.Task](src, dst, "tasks")
	migrateSimple[models.TaskResult](src, dst, "task_results")

	migrateLargeTable[models.Record](src, dst, "records", "records")
	migrateLargeTable[models.Record](src, dst, "records_long_term", "records_long_term")
	migrateLargeTable[models.GPURecord](src, dst, "gpu_records", "gpu_records")
	migrateLargeTable[models.GPURecord](src, dst, "gpu_records_long_term", "gpu_records_long_term")

	log.Println("Migration completed successfully.")
}

// migrateSimple migrates a table where the model's default table name matches.
func migrateSimple[T any](src, dst *gorm.DB, tableName string) {
	log.Printf("Migrating table: %s", tableName)
	var rows []T
	if err := src.Table(tableName).Find(&rows).Error; err != nil {
		log.Printf("  [%s] read error: %v", tableName, err)
		return
	}
	if len(rows) == 0 {
		log.Printf("  [%s] empty, skipping", tableName)
		return
	}
	if err := dst.Table(tableName).CreateInBatches(&rows, migrateBatchSize).Error; err != nil {
		log.Printf("  [%s] write error: %v", tableName, err)
		return
	}
	log.Printf("  [%s] done: %d rows", tableName, len(rows))
}

// migrateLargeTable migrates potentially large tables in offset batches to avoid OOM.
func migrateLargeTable[T any](src, dst *gorm.DB, srcTable, dstTable string) {
	log.Printf("Migrating large table: %s -> %s", srcTable, dstTable)
	offset := 0
	total := 0
	for {
		var rows []T
		if err := src.Table(srcTable).Offset(offset).Limit(migrateBatchSize).Find(&rows).Error; err != nil {
			log.Printf("  [%s] read error at offset %d: %v", srcTable, offset, err)
			return
		}
		if len(rows) == 0 {
			break
		}
		if err := dst.Table(dstTable).CreateInBatches(&rows, migrateBatchSize).Error; err != nil {
			log.Printf("  [%s] write error at offset %d: %v", srcTable, offset, err)
			return
		}
		total += len(rows)
		offset += len(rows)
		log.Printf("  [%s] %d rows migrated...", srcTable, total)
	}
	log.Printf("  [%s] done: %d total rows", srcTable, total)
}
