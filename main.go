package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tsumida/lunaship/infra"
	"github.com/tsumida/lunaship/infra/utils"
	"github.com/tsumida/tex/pkg"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	// pb "github.com/tsumida/tex/pb/connect/api/api"
)

func initDB(env string) func() error {
	switch env {
	case "dev", "test":
		return func() error {
			return infra.InitMySQL(
				mysql.Config{
					DSN: fmt.Sprintf(
						"%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=true",
						utils.StrOrDefault(os.Getenv("MYSQL_USER"), ""),
						utils.StrOrDefault(os.Getenv("MYSQL_PWD"), ""),
						utils.StrOrDefault(os.Getenv("MYSQL_ADDR"), "localhost:3306"),
						utils.StrOrDefault(os.Getenv("MYSQL_DB"), "tex"),
					),
				},
				gorm.Config{},
				func(_ *gorm.DB) error { return nil },
			)
		}
	default:
		panic("invalid env")
	}
}

func main() {
	var (
		ctx = context.Background()
	)
	pkg.RunApp(ctx)
}
