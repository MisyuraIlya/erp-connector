package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"erp-connector/internal/config"
	"erp-connector/internal/db"
	"erp-connector/internal/erp/hasavshevet"
	"erp-connector/internal/secrets"
)

type optionalString struct {
	set   bool
	value string
}

func (o *optionalString) String() string {
	return o.value
}

func (o *optionalString) Set(v string) error {
	o.set = true
	o.value = v
	return nil
}

type optionalInt struct {
	set   bool
	value int
}

func (o *optionalInt) String() string {
	if !o.set {
		return ""
	}
	return strconv.Itoa(o.value)
}

func (o *optionalInt) Set(v string) error {
	if v == "" {
		return errors.New("value required")
	}
	val, err := strconv.Atoi(v)
	if err != nil {
		return err
	}
	o.set = true
	o.value = val
	return nil
}

type optionalBool struct {
	set   bool
	value bool
}

func (o *optionalBool) String() string {
	if !o.set {
		return ""
	}
	if o.value {
		return "true"
	}
	return "false"
}

func (o *optionalBool) Set(v string) error {
	if v == "" {
		v = "true"
	}
	val, err := strconv.ParseBool(v)
	if err != nil {
		return err
	}
	o.set = true
	o.value = val
	return nil
}

func (o *optionalBool) IsBoolFlag() bool {
	return true
}

type stringSliceFlag struct {
	set    bool
	values []string
}

func (s *stringSliceFlag) String() string {
	return strings.Join(s.values, ",")
}

func (s *stringSliceFlag) Set(v string) error {
	s.set = true
	s.values = append(s.values, v)
	return nil
}

func hasHeadlessFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--headless" || arg == "--cli" {
			return true
		}
	}
	return false
}

func runHeadless(uiLog *uiLogger) (bool, error) {
	if !hasHeadlessFlag(os.Args[1:]) {
		return false, nil
	}

	fs := flag.NewFlagSet("erp-connector", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)

	var (
		headless    = fs.Bool("headless", false, "Run without GUI (CLI mode)")
		cli         = fs.Bool("cli", false, "Alias for --headless")
		show        = fs.Bool("show", false, "Print current config summary and exit")
		generateTok = fs.Bool("generate-token", false, "Generate a new bearer token and print it")
		testConn    = fs.Bool("test-connection", false, "Test DB connection using current config")
		initProc    = fs.Bool("init-hasavshevet-proc", false, "Initialize GPRICE_Bulk for Hasavshevet")
		clearImages = fs.Bool("clear-image-folders", false, "Clear configured image folders")
	)

	var (
		erp          optionalString
		apiListen    optionalString
		debug        optionalBool
		bearerToken  optionalString
		dbDriver     optionalString
		dbHost       optionalString
		dbPort       optionalInt
		dbUser       optionalString
		dbName       optionalString
		dbPassword   optionalString
		sendOrderDir optionalString
		imageFolders stringSliceFlag
	)

	fs.Var(&erp, "erp", "ERP type: sap or hasavshevet")
	fs.Var(&apiListen, "api-listen", "API listen address (host:port)")
	fs.Var(&debug, "debug", "Enable debug logging (true/false)")
	fs.Var(&bearerToken, "bearer-token", "Bearer token to store in config")
	fs.Var(&dbDriver, "db-driver", "DB driver (mssql)")
	fs.Var(&dbHost, "db-host", "DB host")
	fs.Var(&dbPort, "db-port", "DB port (1-65535)")
	fs.Var(&dbUser, "db-user", "DB user")
	fs.Var(&dbName, "db-name", "DB database name")
	fs.Var(&dbPassword, "db-password", "DB password (stored securely)")
	fs.Var(&sendOrderDir, "send-order-dir", "Hasavshevet send order folder")
	fs.Var(&imageFolders, "image-folder", "Image folder path (repeatable)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return true, err
	}

	if !*headless && !*cli {
		return false, nil
	}

	uiLog.Printf("headless start")

	cfg, err := config.LoadOrDefault()
	if err != nil {
		return true, err
	}

	if *show {
		printConfigSummary(cfg)
	}

	changed := false

	if erp.set {
		val := strings.ToLower(strings.TrimSpace(erp.value))
		if !isAllowedERP(val) {
			return true, fmt.Errorf("invalid erp: %q", erp.value)
		}
		cfg.ERP = config.ERPType(val)
		changed = true
	}
	if apiListen.set {
		cfg.APIListen = strings.TrimSpace(apiListen.value)
		changed = true
	}
	if debug.set {
		cfg.Debug = debug.value
		changed = true
	}
	if bearerToken.set {
		cfg.BearerToken = strings.TrimSpace(bearerToken.value)
		changed = true
	}
	if dbDriver.set {
		val := strings.ToLower(strings.TrimSpace(dbDriver.value))
		if !isAllowedDriver(val) {
			return true, fmt.Errorf("invalid db-driver: %q", dbDriver.value)
		}
		cfg.DB.Driver = config.DBDriver(val)
		changed = true
	}
	if dbHost.set {
		cfg.DB.Host = strings.TrimSpace(dbHost.value)
		changed = true
	}
	if dbPort.set {
		if dbPort.value <= 0 || dbPort.value > 65535 {
			return true, fmt.Errorf("invalid db-port: %d", dbPort.value)
		}
		cfg.DB.Port = dbPort.value
		changed = true
	}
	if dbUser.set {
		cfg.DB.User = strings.TrimSpace(dbUser.value)
		changed = true
	}
	if dbName.set {
		cfg.DB.Database = strings.TrimSpace(dbName.value)
		changed = true
	}
	if sendOrderDir.set {
		cfg.SendOrderDir = strings.TrimSpace(sendOrderDir.value)
		changed = true
	}

	if *clearImages {
		cfg.ImageFolders = []string{}
		changed = true
	}
	if imageFolders.set {
		folders := make([]string, 0, len(imageFolders.values))
		for _, raw := range imageFolders.values {
			val := strings.TrimSpace(raw)
			if val == "" {
				continue
			}
			folders = append(folders, val)
		}
		cfg.ImageFolders = folders
		changed = true
	}

	if *generateTok {
		token, err := newBearerToken()
		if err != nil {
			return true, err
		}
		cfg.BearerToken = token
		changed = true
		fmt.Fprintf(os.Stdout, "Bearer token: %s\n", token)
	}

	if dbPassword.set {
		if err := secrets.Set(dbPasswordKey(cfg.ERP), []byte(dbPassword.value)); err != nil {
			return true, fmt.Errorf("failed to save db password: %w", err)
		}
		changed = true
		fmt.Fprintln(os.Stdout, "DB password saved.")
	}

	if changed {
		if err := config.Save(cfg); err != nil {
			return true, err
		}
		fmt.Fprintln(os.Stdout, "Config saved.")
	}

	if *testConn || *initProc {
		password, passErr := resolveDBPassword(cfg.ERP, dbPassword.value, false)
		if passErr != nil {
			return true, passErr
		}
		if *testConn {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			if err := db.TestConnection(ctx, cfg, password); err != nil {
				return true, err
			}
			fmt.Fprintln(os.Stdout, "Connection OK.")
		}
		if *initProc {
			if cfg.ERP != config.ERPHasavshevet {
				return true, errors.New("init-hasavshevet-proc requires erp=hasavshevet")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			dbConn, err := db.Open(cfg, password, db.DefaultOptions())
			if err != nil {
				return true, err
			}
			defer dbConn.Close()
			created, err := hasavshevet.EnsureGPriceBulkProcedure(ctx, dbConn)
			if err != nil {
				return true, err
			}
			if created {
				fmt.Fprintln(os.Stdout, "GPRICE_Bulk created.")
			} else {
				fmt.Fprintln(os.Stdout, "GPRICE_Bulk already exists.")
			}
		}
	}

	if !*show && !changed && !*testConn && !*initProc {
		fmt.Fprintln(os.Stdout, "No changes requested. Use --show or set flags. Example:")
		fmt.Fprintln(os.Stdout, "  erp-connector.exe --headless --generate-token --api-listen 127.0.0.1:8080")
	}

	return true, nil
}

func isAllowedERP(val string) bool {
	for _, v := range config.ErpValues() {
		if string(v) == val {
			return true
		}
	}
	return false
}

func isAllowedDriver(val string) bool {
	for _, v := range config.DBDriverValues() {
		if string(v) == val {
			return true
		}
	}
	return false
}

func printConfigSummary(cfg config.Config) {
	fmt.Fprintln(os.Stdout, "Config summary:")
	fmt.Fprintf(os.Stdout, "  ERP: %s\n", cfg.ERP)
	fmt.Fprintf(os.Stdout, "  API Listen: %s\n", cfg.APIListen)
	fmt.Fprintf(os.Stdout, "  Debug: %v\n", cfg.Debug)
	if cfg.BearerToken == "" {
		fmt.Fprintln(os.Stdout, "  Bearer Token: (empty)")
	} else {
		fmt.Fprintf(os.Stdout, "  Bearer Token: (set, len=%d)\n", len(cfg.BearerToken))
	}
	fmt.Fprintf(os.Stdout, "  DB Driver: %s\n", cfg.DB.Driver)
	fmt.Fprintf(os.Stdout, "  DB Host: %s\n", cfg.DB.Host)
	fmt.Fprintf(os.Stdout, "  DB Port: %d\n", cfg.DB.Port)
	fmt.Fprintf(os.Stdout, "  DB User: %s\n", cfg.DB.User)
	fmt.Fprintf(os.Stdout, "  DB Database: %s\n", cfg.DB.Database)
	if len(cfg.ImageFolders) == 0 {
		fmt.Fprintln(os.Stdout, "  Image Folders: (none)")
	} else {
		fmt.Fprintln(os.Stdout, "  Image Folders:")
		for _, p := range cfg.ImageFolders {
			fmt.Fprintf(os.Stdout, "    - %s\n", p)
		}
	}
	if cfg.SendOrderDir == "" {
		fmt.Fprintln(os.Stdout, "  Send Order Dir: (empty)")
	} else {
		fmt.Fprintf(os.Stdout, "  Send Order Dir: %s\n", cfg.SendOrderDir)
	}
}
