package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	"erp-connector/internal/api"
	"erp-connector/internal/config"
	"erp-connector/internal/db"
	"erp-connector/internal/erp/hasavshevet"
	"erp-connector/internal/logger"
	"erp-connector/internal/platform/autostart"
	"erp-connector/internal/secrets"
)

const windowsServiceName = "erp-connectord"

type serverApp struct {
	cfg          config.Config
	logSvc       logger.LoggerService
	dbConn       *sql.DB
	srv          *http.Server
	errCh        chan error
	dbPassStr    string
	orderQueue   *hasavshevet.OrderQueue
	queueCancel  context.CancelFunc
}

func (a *serverApp) Start() error {
	bootstrapLog := logger.NewStderr()

	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			bootstrapLog.Error("config not found; run erp-connector UI to create it", nil)
			return err
		}
		bootstrapLog.Error("failed to load config", err)
		return err
	}
	a.cfg = cfg

	logSvc, err := logger.New(cfg)
	if err != nil {
		bootstrapLog.Error("logger init failed; using stderr", err)
		logSvc = bootstrapLog
	}
	a.logSvc = logSvc

	dbPassword, dbPassErr := secrets.Get(dbPasswordKey(cfg.ERP))
	if dbPassErr != nil {
		logSvc.Error("failed to load db password", dbPassErr)
	}
	if dbPassErr == nil {
		a.dbPassStr = string(dbPassword)
	}

	dbConn, err := db.Open(cfg, a.dbPassStr, db.DefaultOptions())
	if err != nil {
		logSvc.Error("db connection failed", err)
		a.Stop(context.Background())
		return err
	}
	a.dbConn = dbConn

	// Build the send-order queue for Hasavshevet.
	// Order number file lives next to IMOVEIN files for self-contained directory.
	numStorePath := filepath.Join(cfg.SendOrderDir, "lastOrderNumber.json")
	numStore := hasavshevet.NewOrderNumberStore(numStorePath)
	sender := hasavshevet.NewSender(dbConn, cfg, numStore, logSvc)
	queue := hasavshevet.NewOrderQueue(sender, logSvc)
	queueCtx, queueCancel := context.WithCancel(context.Background())
	queue.Start(queueCtx)
	a.orderQueue = queue
	a.queueCancel = queueCancel

	srv, err := api.NewServer(cfg, api.ServerDeps{
		DBPassword:     a.dbPassStr,
		DB:             dbConn,
		Logger:         logSvc,
		SendOrderQueue: queue,
	})
	if err != nil {
		logSvc.Error("config validation error", err)
		a.Stop(context.Background())
		return err
	}
	a.srv = srv

	a.errCh = make(chan error, 1)
	go func() {
		a.errCh <- srv.ListenAndServe()
	}()

	logSvc.Info(fmt.Sprintf("erp-connectord listening on %s", srv.Addr))
	return nil
}

func (a *serverApp) Stop(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	if a.queueCancel != nil {
		a.queueCancel()
	}
	if a.srv != nil {
		_ = a.srv.Shutdown(ctx)
	}
	if a.dbConn != nil {
		_ = a.dbConn.Close()
	}
	if a.logSvc != nil {
		_ = a.logSvc.Close()
	}
}

func (a *serverApp) Errors() <-chan error {
	return a.errCh
}

func (a *serverApp) Logger() autostart.Logger {
	return a.logSvc
}
