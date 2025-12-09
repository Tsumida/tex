package database

import (
	"fmt"
	"os"

	"github.com/tsumida/lunaship/infra"
	"github.com/tsumida/lunaship/infra/utils"
	"gorm.io/driver/mysql"
	"gorm.io/gen"
	"gorm.io/gorm"
)

func main() {
	err := infra.InitMySQL(
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
	if err != nil {
		panic(err)
	}

	db := infra.GlobalMySQL()
	g := gen.NewGenerator(gen.Config{
		OutPath: "./query", // 生成的目录
		Mode:    gen.WithDefaultQuery | gen.WithQueryInterface,
	})

	g.UseDB(db)

	// 自动为所有表生成 struct
	g.GenerateAllTable()

	g.Execute()
}
