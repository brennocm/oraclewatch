package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/brennocm/oraclewatch/internal/alert"
	"github.com/brennocm/oraclewatch/internal/server"
)

func newRunCommand(info BuildInfo) *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the monitor as a daemon",
		Long: "run monitors every configured feed until SIGINT or SIGTERM. On shutdown it stops\n" +
			"scheduling cycles, lets cycles in progress finish (bounded) and sends pending alerts.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			a, err := loadApp(ctx, path, cmd.ErrOrStderr(), true)
			if err != nil {
				return &exitError{code: ExitError, err: err}
			}
			a.metrics.SetBuildInfo(info.Version, info.Commit)
			al := a.cfg.Alerting
			manager := alert.NewManager(alert.Options{For: al.For.Duration, Cooldown: al.Cooldown.Duration, SendResolved: al.SendResolved, Link: a.link})
			disp := alert.NewDispatcher(a.notifiers(cmd.OutOrStdout()), alert.DispatcherOptions{
				GroupWindow: al.GroupWindow.Duration, StormThreshold: al.StormThreshold, Logger: a.logger, Observer: a.metrics,
			})
			eng, err := a.engine(manager, disp, a.deadman())
			if err != nil {
				return &exitError{code: ExitError, err: err}
			}
			srv := server.New(server.Options{Listen: a.cfg.Server.Listen, Registry: a.metrics.Registry, Source: eng, Alerts: manager, Logger: a.logger, Version: info.Version})

			a.logger.Info("oraclewatch starting", "version", info.Version, "networks", len(a.cfg.Networks), "feeds", len(a.cfg.Feeds))
			// The dispatcher outlives the engine so that alerts from the last
			// cycles are delivered; it gets its own context.
			dctx, dcancel := context.WithCancel(context.WithoutCancel(ctx))
			go disp.Run(dctx)

			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error { return eng.Run(gctx) })
			g.Go(func() error { return srv.ListenAndServe(gctx) })
			err = g.Wait()
			dcancel()
			<-disp.Done()
			a.logger.Info("oraclewatch stopped")
			if err != nil && !errors.Is(err, context.Canceled) {
				return &exitError{code: ExitError, err: a.redactor.Error(err)}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&path, "config", "c", "", "path to the YAML configuration (required)")
	_ = cmd.MarkFlagRequired("config") // only fails if the flag does not exist
	return cmd
}
