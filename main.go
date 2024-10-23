package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/go-sql-driver/mysql"
	sloghttp "github.com/samber/slog-http"

	"github.com/sardinasystems/galera-healthcheck/healthcheck"
)

var cli struct {
	Listen   string     `name:"listen" short:"l" default:":9200" help:"listen host:port for http server"`
	DSN      string     `name:"dsn" help:"connection dsn (alternative to options below)"`
	Username string     `name:"username" short:"u" help:"mysql user name"`
	Password string     `name:"password" short:"p" help:"mysql user password"`
	Host     string     `name:"host" short:"H" default:"localhost" help:"mysql tcp host"`
	Port     int        `name:"port" short:"P" default:"3306" help:"mysql tcp port"`
	PidFile  string     `name:"pid-file" help:"Write pid file"`
	LogLevel slog.Level `name:"log-level" default:"info" choices:"debug,info,warn,error" help:"Log level"`

	Version kong.VersionFlag `name:"version" help:"Show version"`
}

func main() {
	runtime.GOMAXPROCS(1)

	// stop on ctrl+c or TERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	kctx := kong.Parse(&cli,
		kong.Description("Galera Healthchecker"),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: false,
		}),
		kong.DefaultEnvars("GALERA_HEALTH"),
	)
	kctx.Command()

	var err error

	// setup logging
	lgh := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cli.LogLevel})
	lg := slog.New(lgh).With("process", os.Args[0])
	slog.SetDefault(lg)

	var cfg *mysql.Config
	if cli.DSN != "" {
		cfg, err = mysql.ParseDSN(cli.DSN)
		kctx.FatalIfErrorf(err)
	} else {
		cfg = mysql.NewConfig()
		cfg.Addr = fmt.Sprintf("%s:%d", cli.Host, cli.Port)
		cfg.User = cli.Username
		cfg.Passwd = cli.Password
	}

	conn, err := mysql.NewConnector(cfg)
	kctx.FatalIfErrorf(err)

	db := sql.OpenDB(conn)

	if cli.PidFile != "" {
		pid := os.Getpid()
		err = os.WriteFile(cli.PidFile, []byte(fmt.Sprintf("%d", pid)), 0644)
		kctx.FatalIfErrorf(err)

		lg2 := lg.With("pid", pid, "file", cli.PidFile)
		lg2.Debug("Wrote pid-file")

		defer func() {
			err := os.Remove(cli.PidFile)
			if err != nil {
				lg2.Error("Failed to remove pid-file", "error", err)
			} else {
				lg2.Info("Removed pid-file")
			}
		}()
	}

	mux := http.NewServeMux()
	srv := http.Server{
		Addr:    cli.Listen,
		Handler: sloghttp.New(lg)(mux),
	}

	hc := healthcheck.New(db)

	queryBool := func(r *http.Request, key string, def bool) (bool, error) {
		qv := r.URL.Query()
		if !qv.Has(key) {
			return def, nil
		}

		v := qv.Get(key)
		if v == "" {
			// ?donor_ok == ?donor_ok=1
			return true, nil
		}

		return strconv.ParseBool(v)
	}

	makeHandler := func(defDonorOk, defReadOnlyOk bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			w.Header().Add("Content-Type", "text/plain")

			donorOk, err1 := queryBool(r, "donor_ok", defDonorOk)
			readOnlyOk, err2 := queryBool(r, "readonly_ok", defReadOnlyOk)
			err := errors.Join(err1, err2)

			if err != nil {
				lg.ErrorContext(ctx, "Failed to parse query opts", "error", err)
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "Failed to parse query opts:\n\n%v\n", err)
				return
			}

			healthy, msg, err := hc.Check(ctx, donorOk, readOnlyOk)
			if err != nil {
				lg.ErrorContext(ctx, "Query failed", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "Query failed: %v", err)
				return
			}

			code := http.StatusServiceUnavailable
			if msg == "syncing" {
				code = http.StatusContinue
			} else if healthy {
				code = http.StatusOK
			}

			w.WriteHeader(code)
			fmt.Fprintf(w, "Galera Cluster Node status: %s", msg)
		}
	}

	mux.HandleFunc("GET /ready", makeHandler(false, false)) // for haproxy
	mux.HandleFunc("GET /boot", makeHandler(true, true))    // for boot health checks
	mux.HandleFunc("GET /", makeHandler(false, false))      // default

	go func() {
		err := srv.ListenAndServe()
		if err != nil {
			lg.Error("Serve failed")
		}
	}()

	// Wait for termination signal
	<-ctx.Done()

	// Graceful shutdown
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel2()

	err = srv.Shutdown(ctx2)
	kctx.FatalIfErrorf(err)
}
