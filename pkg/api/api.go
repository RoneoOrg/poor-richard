package api

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/carlmjohnson/flagext"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/peterbourgon/ff/v2"
	"github.com/piotrkubisa/apigo"
)

const AppName = "spotlightpa-api"

var BuildVersion string = "Development"

func CLI(args []string) error {
	var app appEnv
	if err := app.parseArgs(args); err != nil {
		fmt.Fprintf(os.Stderr, "Startup error: %v\n", err)
		return err
	}
	if err := app.exec(); err != nil {
		fmt.Fprintf(os.Stderr, "Runtime error: %v\n", err)
		return err
	}
	return nil
}

func (app *appEnv) parseArgs(args []string) error {
	fl := flag.NewFlagSet(AppName, flag.ContinueOnError)
	app.Logger = log.New(nil, AppName+" ", log.LstdFlags)
	flagext.LoggerVar(fl, app.Logger, "silent", flagext.LogSilent, "silence logging")

	fl.BoolVar(&app.isLambda, "lambda", false, "use AWS Lambda rather than HTTP")
	fl.StringVar(&app.port, "port", ":12345", "listen on port (HTTP only)")
	sentryDSN := fl.String("sentry-dsn", "", "DSN `pseudo-URL` for Sentry")
	fl.StringVar(&app.googleCreds, "google-creds", "", "JSON credentials for Google")
	fl.StringVar(&app.viewID, "view-id", "", "view ID for Google Analytics")

	fl.Usage = func() {
		fmt.Fprintf(fl.Output(), "spotlightpa-api help\n\n")
		fl.PrintDefaults()
	}
	if err := ff.Parse(fl, args, ff.WithEnvVarPrefix("POOR_RICHARD")); err != nil {
		return err
	}

	if err := app.initSentry(*sentryDSN, app.Logger); err != nil {
		return err
	}

	return nil
}

type appEnv struct {
	port     string
	isLambda bool
	*log.Logger
	googleCreds string
	viewID      string
}

func (app *appEnv) exec() error {
	app.Printf("starting %s (%s)", AppName, BuildVersion)
	routes := sentryhttp.
		New(sentryhttp.Options{
			WaitForDelivery: true,
			Timeout:         5 * time.Second,
		}).
		Handle(app.routes())

	if app.isLambda {
		app.Printf("starting on AWS Lambda")
		apigo.ListenAndServe("", routes)
		panic("unreachable")
	}

	app.Printf("starting on port %s", app.port)

	return http.ListenAndServe(app.port, routes)
}

func (app *appEnv) initSentry(dsn string, l *log.Logger) error {
	var transport sentry.Transport
	if app.isLambda {
		l.Printf("setting sentry sync with timeout")
		transport = &sentry.HTTPSyncTransport{Timeout: 5 * time.Second}
	}
	return sentry.Init(sentry.ClientOptions{
		Dsn:       dsn,
		Release:   BuildVersion,
		Transport: transport,
	})
}

func (app *appEnv) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/most-popular", app.getMostPopular)
	return app.versionMiddleware(mux)
}
