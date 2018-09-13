//go:generate retool do packr

package main

import (
	"fmt"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/emed-appts/emed-mailer/pkg/collector"
	"github.com/emed-appts/emed-mailer/pkg/config"
	"github.com/emed-appts/emed-mailer/pkg/job"
	"github.com/emed-appts/emed-mailer/pkg/mailer"
	"github.com/emed-appts/emed-mailer/pkg/version"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/robfig/cron.v2"
	"gopkg.in/urfave/cli.v2"
)

func main() {
	app := &cli.App{
		Name:        "emed-mailer",
		Version:     version.Version.String(),
		Usage:       "eMedical Appointments Mailer Service",
		Description: "Runs the emed-mailer service, sending notifications about booked/cancelled appointments",
		Compiled:    time.Now(),

		Authors: []*cli.Author{
			{
				Name:  "David Schneiderbauer",
				Email: "david.schneiderbauer@dschneiderbauer.me",
			},
		},

		Before: func(c *cli.Context) error {
			return nil
		},

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Value:       "conf/app.ini",
				Usage:       "set config path",
				Destination: &config.Path,
			},
		},

		Action: func(ctx *cli.Context) error {
			// load config
			err := config.Load()
			if err != nil {
				fmt.Fprintf(ctx.App.Writer, "\nCould not load configuration file.\n%v\n\n", errors.Cause(err))

				cli.ShowAppHelp(ctx)
				return cli.Exit("", 128)
			}

			// open logfile
			logFile, err := os.OpenFile(path.Join(config.General.Root, "emed-mailer.log"), os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
			if err != nil {
				fmt.Fprintf(ctx.App.Writer, "\nCould not open log file.\n%v\n\n", errors.Cause(err))

				cli.ShowAppHelp(ctx)
				return cli.Exit("", 128)
			}
			defer logFile.Close()

			// configure logger
			if config.Log.Pretty {
				log.Logger = log.Output(
					zerolog.ConsoleWriter{
						Out:     logFile,
						NoColor: !config.Log.Colored,
					},
				)
			} else {
				log.Logger = log.Output(logFile)
			}

			// set configured log level
			logLvl, err := zerolog.ParseLevel(config.Log.Level)
			if err != nil {
				fmt.Fprintf(ctx.App.Writer, "\nCould not parse Log Level.\n%v\n\n", errors.Cause(err))

				cli.ShowAppHelp(ctx)
				return cli.Exit("", 128)
			}
			zerolog.SetGlobalLevel(logLvl)

			stop := make(chan struct{}, 1)

			// open database connection
			db, err := collector.OpenSQL(collector.DBConfig{
				Server:   config.DB.Server,
				Port:     config.DB.Port,
				User:     config.DB.User,
				Password: config.DB.Password,
				Database: config.DB.Database,
			})
			if err != nil {
				log.Fatal().
					Msgf("%+v\n", errors.Wrap(err, "could not connect to db"))

				return err
			}
			defer db.Close()

			// instantiate collector
			c := collector.New(db)

			// instantiate emed-mailer
			m := mailer.New(mailer.Config{
				Server:   config.Mail.Server,
				Port:     config.Mail.Port,
				User:     config.Mail.User,
				Password: config.Mail.Password,

				From:    config.Mail.From,
				To:      config.Mail.To,
				Subject: config.Mail.Subject,
			})
			// run emed-mailer daemon
			m.Run(stop)

			// instantiate job
			changedApptsJob := job.New(c, m)

			cj := cron.New()
			cj.AddFunc(config.General.Schedule, changedApptsJob.Run)
			cj.Start()

			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
			<-sigs

			cj.Stop()
			close(sigs)
			close(stop)

			return nil
		},
	}

	cli.HelpFlag = &cli.BoolFlag{
		Name:    "help",
		Aliases: []string{"h"},
		Usage:   "show the help, so what you see now",
	}

	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "print the current version of that tool",
	}

	if err := app.Run(os.Args); err != nil {
		os.Exit(1)
	}
}
