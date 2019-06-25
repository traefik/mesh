package traefik

import (
	"context"
	"encoding/json"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containous/i3o/internal/providers/smi"
	traefikcmd "github.com/containous/traefik/cmd"
	"github.com/containous/traefik/pkg/cli"
	"github.com/containous/traefik/pkg/collector"
	"github.com/containous/traefik/pkg/config/static"
	"github.com/containous/traefik/pkg/log"
	"github.com/containous/traefik/pkg/provider/aggregator"
	"github.com/containous/traefik/pkg/safe"
	"github.com/containous/traefik/pkg/server"
	"github.com/containous/traefik/pkg/server/router"
	traefiktls "github.com/containous/traefik/pkg/tls"
	"github.com/sirupsen/logrus"
	"github.com/vulcand/oxy/roundrobin"
)

// NewCmd builds a new Version command
func NewCmd(tConfig *traefikcmd.TraefikCmdConfiguration, loaders []cli.ResourceLoader) *cli.Command {
	return &cli.Command{
		Name: "traefik",
		Description: `Traefik is a modern HTTP reverse proxy and load balancer made to deploy microservices with ease.
Complete documentation is available at https://traefik.io`,
		Configuration: tConfig,
		Run: func(_ []string) error {
			return runCmd(&tConfig.Configuration, cli.GetConfigFile(loaders))
		},
		Resources: loaders,
	}
}

func runCmd(staticConfiguration *static.Configuration, configFile string) error {
	configureLogging(staticConfiguration)

	http.DefaultTransport.(*http.Transport).Proxy = http.ProxyFromEnvironment

	if err := roundrobin.SetDefaultWeight(0); err != nil {
		log.WithoutContext().Errorf("Could not set roundrobin default weight: %v", err)
	}

	staticConfiguration.SetEffectiveConfiguration(configFile)
	staticConfiguration.ValidateConfiguration()

	jsonConf, err := json.Marshal(staticConfiguration)
	if err != nil {
		log.WithoutContext().Errorf("Could not marshal static configuration: %v", err)
		log.WithoutContext().Debugf("Static configuration loaded [struct] %#v", staticConfiguration)
	} else {
		log.WithoutContext().Debugf("Static configuration loaded %s", string(jsonConf))
	}

	stats(staticConfiguration)

	providerAggregator := aggregator.NewProviderAggregator(*staticConfiguration.Providers)

	if err = providerAggregator.AddProvider(&smi.Provider{}); err != nil {
		log.WithoutContext().Errorf("Unable to add kubernetessmi provider to the providers list: %v", err)
	}

	serverEntryPointsTCP := make(server.TCPEntryPoints)
	for entryPointName, config := range staticConfiguration.EntryPoints {
		ctx := log.With(context.Background(), log.Str(log.EntryPointName, entryPointName))
		serverEntryPointsTCP[entryPointName], err = server.NewTCPEntryPoint(ctx, config)
		if err != nil {
			return fmt.Errorf("error while building entryPoint %s: %v", entryPointName, err)
		}
		serverEntryPointsTCP[entryPointName].RouteAppenderFactory = router.NewRouteAppenderFactory(*staticConfiguration, entryPointName, nil)
	}

	tlsManager := traefiktls.NewManager()

	svr := server.NewServer(*staticConfiguration, providerAggregator, serverEntryPointsTCP, tlsManager)
	ctx := traefikcmd.ContextWithSignal(context.Background())

	if staticConfiguration.Ping != nil {
		staticConfiguration.Ping.WithContext(ctx)
	}

	svr.Start(ctx)
	defer svr.Close()

	svr.Wait()
	log.WithoutContext().Info("Shutting down")
	logrus.Exit(0)
	return nil
}

func configureLogging(staticConfiguration *static.Configuration) {
	// configure default log flags
	stdlog.SetFlags(stdlog.Lshortfile | stdlog.LstdFlags)

	// configure log level
	// an explicitly defined log level always has precedence. if none is
	// given and debug mode is disabled, the default is ERROR, and DEBUG
	// otherwise.
	levelStr := "error"
	if staticConfiguration.Log != nil && staticConfiguration.Log.Level != "" {
		levelStr = strings.ToLower(staticConfiguration.Log.Level)
	}

	level, err := logrus.ParseLevel(levelStr)
	if err != nil {
		log.WithoutContext().Errorf("Error getting level: %v", err)
	}
	log.SetLevel(level)

	var logFile string
	if staticConfiguration.Log != nil && len(staticConfiguration.Log.FilePath) > 0 {
		logFile = staticConfiguration.Log.FilePath
	}

	// configure log format
	var formatter logrus.Formatter
	if staticConfiguration.Log != nil && staticConfiguration.Log.Format == "json" {
		formatter = &logrus.JSONFormatter{}
	} else {
		disableColors := len(logFile) > 0
		formatter = &logrus.TextFormatter{DisableColors: disableColors, FullTimestamp: true, DisableSorting: true}
	}
	log.SetFormatter(formatter)

	if len(logFile) > 0 {
		dir := filepath.Dir(logFile)

		if err = os.MkdirAll(dir, 0755); err != nil {
			log.WithoutContext().Errorf("Failed to create log path %s: %s", dir, err)
		}

		err = log.OpenFile(logFile)
		logrus.RegisterExitHandler(func() {
			if err = log.CloseFile(); err != nil {
				log.WithoutContext().Errorf("Error while closing log: %v", err)
			}
		})
		if err != nil {
			log.WithoutContext().Errorf("Error while opening log file %s: %v", logFile, err)
		}
	}
}

func stats(staticConfiguration *static.Configuration) {
	if staticConfiguration.Global.SendAnonymousUsage == nil {
		log.WithoutContext().Error(`
You haven't specified the sendAnonymousUsage option, it will be enabled by default.
`)
		sendAnonymousUsage := true
		staticConfiguration.Global.SendAnonymousUsage = &sendAnonymousUsage
	}

	if *staticConfiguration.Global.SendAnonymousUsage {
		log.WithoutContext().Info(`
Stats collection is enabled.
Many thanks for contributing to Traefik's improvement by allowing us to receive anonymous information from your configuration.
Help us improve Traefik by leaving this feature on :)
More details on: https://docs.traefik.io/basics/#collected-data
`)
		collect(staticConfiguration)
	} else {
		log.WithoutContext().Info(`
Stats collection is disabled.
Help us improve Traefik by turning this feature on :)
More details on: https://docs.traefik.io/basics/#collected-data
`)
	}
}

func collect(staticConfiguration *static.Configuration) {
	ticker := time.NewTicker(24 * time.Hour)
	safe.Go(func() {
		for time.Sleep(10 * time.Minute); ; <-ticker.C {
			if err := collector.Collect(staticConfiguration); err != nil {
				log.WithoutContext().Debug(err)
			}
		}
	})
}
