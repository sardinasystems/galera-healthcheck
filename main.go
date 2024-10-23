package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/go-sql-driver/mysql"
)

var cli struct {
	Listen   string `name:"listen" short:"l" default:":9200" help:"listen host:port for http server"`
	DSN      string `name:"dsn" help:"connection dsn (alternative to options below)"`
	Username string `name:"username" short:"u" help:"mysql user name"`
	Password string `name:"password" short:"p" help:"mysql user password"`
	Host     string `name:"host" short:"H" default:"localhost" help:"mysql tcp host"`
	Port     int    `name:"port" short:"P" default:"3306" help:"mysql tcp port"`
	Pid      string `name:"pid-file" help:"Write pid file"`

	Version kong.VersionFlag `name:"version" help:"Show version"`
}

func main() {
	runtime.GOMAXPROCS(1)

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

	if cli.Pid != "" {
		err = os.WriteFile(cli.Pid, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
		kctx.FatalIfErrorf(err)

		defer func() {
			<-ctx.Done()
			os.Remove(cli.Pid)
		}()
	}

}
