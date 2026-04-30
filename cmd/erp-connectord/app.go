package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"erp-connector/internal/api"
	"erp-connector/internal/config"
	"erp-connector/internal/db"
	"erp-connector/internal/email"
	"erp-connector/internal/erp/hasavshevet"
	"erp-connector/internal/logger"
	"erp-connector/internal/pdf"
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
	// Bootstrap logger writes directly to server.log so we capture pre-config
	// failures (config.Load errors, permissions, missing dirs) even when
	// running as a Windows service where stderr is unavailable.
	bootstrapLog := logger.NewBootstrap()
	bootstrapLog.Info(fmt.Sprintf("daemon Start() called: pid=%d goos=%s", os.Getpid(), runtime.GOOS))

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
	bootstrapLog.Info(fmt.Sprintf("config loaded: erp=%s apiListen=%s sendOrderDir=%q", cfg.ERP, cfg.APIListen, cfg.SendOrderDir))

	bootstrapLog.Info("calling logger.New(cfg)")
	logSvc, err := logger.New(cfg)
	if err != nil {
		bootstrapLog.Error("logger init failed; using bootstrap logger", err)
		logSvc = bootstrapLog
	}
	a.logSvc = logSvc
	bootstrapLog.Info("logger.New(cfg) returned")

	logSvc.Info(fmt.Sprintf("calling secrets.Get for db password (key=%s)", dbPasswordKey(cfg.ERP)))
	dbPassword, dbPassErr := secrets.Get(dbPasswordKey(cfg.ERP))
	if dbPassErr != nil {
		logSvc.Error("failed to load db password", dbPassErr)
	}
	if dbPassErr == nil {
		a.dbPassStr = string(dbPassword)
		logSvc.Info(fmt.Sprintf("db password loaded (length=%d)", len(a.dbPassStr)))
	}

	logSvc.Info(fmt.Sprintf(
		"calling db.Open: driver=%s host=%s port=%d database=%s user=%s",
		cfg.DB.Driver, cfg.DB.Host, cfg.DB.Port, cfg.DB.Database, cfg.DB.User,
	))
	dbConn, err := db.Open(cfg, a.dbPassStr, db.DefaultOptions())
	if err != nil {
		logSvc.Error("db connection failed", err)
		a.Stop(context.Background())
		return err
	}
	a.dbConn = dbConn
	logSvc.Info("db.Open returned successfully")

	// Build the send-order queue for Hasavshevet.
	// Order number file lives next to IMOVEIN files for self-contained directory.
	numStorePath := filepath.Join(cfg.SendOrderDir, "lastOrderNumber.json")
	numStore := hasavshevet.NewOrderNumberStore(numStorePath)
	sender := hasavshevet.NewSender(dbConn, cfg, numStore, logSvc)

	// Set up post-order hooks (PDF generation, printing, email).
	logSvc.Info(fmt.Sprintf(
		"PDF config snapshot at startup: PrintAfterOrder=%v EmailAfterOrder=%v UseRemoteTemplate=%v RemoteTemplateBaseURL=%q tokenCount=%d ChromePath=%q SumatraPDFPath=%q PrinterName=%q",
		cfg.PDF.PrintAfterOrder, cfg.PDF.EmailAfterOrder, cfg.PDF.UseRemoteTemplate,
		cfg.PDF.RemoteTemplateBaseURL, len(cfg.PDF.RemoteTokens),
		cfg.PDF.ChromePath, cfg.PDF.SumatraPDFPath, cfg.PDF.PrinterName,
	))
	var postHooks []hasavshevet.PostOrderHook
	if cfg.PDF.PrintAfterOrder || cfg.PDF.EmailAfterOrder {
		chromePath := cfg.PDF.ChromePath
		if chromePath == "" {
			chromePath = pdf.DetectChrome()
			logSvc.Info(fmt.Sprintf("ChromePath empty in config; auto-detect resolved=%q", chromePath))
		}
		if chromePath == "" {
			logSvc.Warn("Chrome not found; PDF generation after order will be skipped (no PDF post-order hook will be registered)")
		} else {
			pdfGen := pdf.NewGenerator(chromePath)

			var emailSender *email.Sender
			if cfg.PDF.EmailAfterOrder && cfg.SMTP.Host != "" {
				smtpPass, _ := secrets.Get("smtp_password")
				emailSender = email.NewSender(cfg.SMTP, string(smtpPass))
				logSvc.Info("email after order enabled")
			}

			postHooks = append(postHooks, hasavshevet.NewPDFPostOrderHook(
				cfg, pdfGen, emailSender, logSvc,
			))
			logSvc.Info(fmt.Sprintf("PDF post-order hook enabled (print=%v, email=%v, chrome=%s)",
				cfg.PDF.PrintAfterOrder, cfg.PDF.EmailAfterOrder, chromePath))
		}
	} else {
		logSvc.Warn("no PDF post-order hook registered: both PrintAfterOrder and EmailAfterOrder are false in config — toggle them in the GUI Settings → PDF & Email Settings, click Save, then RESTART erp-connectord for changes to take effect")
	}

	queue := hasavshevet.NewOrderQueue(sender, logSvc, postHooks...)
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
	logSvc.Info(fmt.Sprintf("HTTP server goroutine launched, will listen on %s", srv.Addr))

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
