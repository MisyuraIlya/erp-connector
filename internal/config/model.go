package config

type ERPType string
type DBDriver string

const (
	ERPSAP         ERPType = "sap"
	ERPHasavshevet ERPType = "hasavshevet"
)

const (
	DBDriverMSSQL DBDriver = "mssql"
)

type DBConfig struct {
	Driver  DBDriver `yaml:"driver"`
	Host    string   `yaml:"host"`
	Port    int      `yaml:"port"`
	User    string   `yaml:"user"`
	Databse string   `yaml:"database"`
}

type Config struct {
	ERP          ERPType  `yaml:"erp"`
	APIListen    string   `yaml:"apiListen"`
	ImageFolders []string `yaml:"imageFolders"`
	sendOrderDir string   `yaml:"sendOrderDir"`
	DB           DBConfig `yaml:"db"`
}

func ErpValues() []ERPType {
	return []ERPType{ERPSAP, ERPHasavshevet}
}

func ErpOption() []string {
	vals := ErpValues()
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, string(v))
	}
	return out
}

func DBDriverValues() []DBDriver {
	return []DBDriver{DBDriverMSSQL}
}

func DBDriverOptions() []string {
	vals := DBDriverValues()
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, string(v))
	}
	return out
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
