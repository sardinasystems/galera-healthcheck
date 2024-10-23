package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"strconv"

	_ "github.com/go-sql-driver/mysql"
	"github.com/sttts/galera-healthcheck/healthcheck"
	. "github.com/sttts/galera-healthcheck/logger"
)

var serverPort = flag.Int(
	"port",
	8080,
	"Specifies the port of the healthcheck server",
)

var mysqlUser = flag.String(
	"user",
	"root",
	"Specifies the MySQL user to connect as",
)

var mysqlPassword = flag.String(
	"password",
	"",
	"Specifies the MySQL password to connect with",
)

var availableWhenDonor = flag.Bool(
	"availWhenDonor",
	true,
	"Specifies if the healthcheck allows availability when in donor state",
)

var availableWhenReadOnly = flag.Bool(
	"availWhenReadOnly",
	false,
	"Specifies if the healthcheck allows availability when in read only mode",
)

var pidfile = flag.String(
	"pidfile",
	"",
	"Location for the pidfile",
)

var connectionCutterPath = flag.String(
	"connectionCutterPath",
	"",
	"Location for the script which cuts mysql connections",
)

var healthchecker *healthcheck.Healthchecker

func handler(w http.ResponseWriter, r *http.Request) {
	result, msg := healthchecker.Check()
	if result != nil && result.Healthy {
		w.WriteHeader(http.StatusOK)
	} else if result != nil && !result.Healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusContinue)
	}

	fmt.Fprintf(w, "Galera Cluster Node status: %s", msg)
	LogWithTimestamp(msg)
}

func main() {
	runtime.GOMAXPROCS(1)

	flag.Parse()

	err := ioutil.WriteFile(*pidfile, []byte(strconv.Itoa(os.Getpid())), 0644)
	if err != nil {
		panic(err)
	}

	db, _ := sql.Open("mysql", fmt.Sprintf("%s:%s@/", *mysqlUser, *mysqlPassword))
	config := healthcheck.HealthcheckerConfig{
		*availableWhenDonor,
		*availableWhenReadOnly,
	}

	healthchecker = healthcheck.New(db, config)

	http.HandleFunc("/", handler)
	http.ListenAndServe(fmt.Sprintf(":%d", *serverPort), nil)
}
