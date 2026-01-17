package config

type ERPType string

const (
	ERPSAP         ERPType = "sap"
	ERPHasavshevet ERPType = "hasavshevet"
)

type DBConfig struct {
	Driver  string `yaml: "driver"`
	Host    string `yaml: "host"`
	Port    int    `yaml: "port"`
	User    string `yaml: "user"`
	Databse string `yaml: "database"`
}

type Config struct {
	ERP          ERPType  `yaml: "erp"`
	APIListen    string   `yaml: "apiListen"`
	ImageFolders []string `yaml: "imageFolders"`
	sendOrderDir string   `yaml: "sendOrderDir"`
	DB           DBConfig `yaml: "db"`
}

func Default() Config {
	return Config{
		ERP:          ERPHasavshevet,
		APIListen:    "127.0.0.1:8080",
		ImageFolders: []string{},
		sendOrderDir: "",
		DB: DBConfig{
			Driver:  "mssql",
			Host:    "localhost",
			Port:    1433,
			Databse: "",
			User:    "",
		},
	}
}
