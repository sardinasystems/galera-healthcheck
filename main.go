package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/go-sql-driver/mysql"
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

	time.Sleep(1 * time.Second)
	_ = db
	_ = ctx
}
