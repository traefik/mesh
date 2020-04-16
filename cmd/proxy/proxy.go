package proxy

import (
	"context"
	"encoding/json"
	stdlog "log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/containous/traefik/v2/cmd"
	"github.com/containous/traefik/v2/cmd/healthcheck"
	"github.com/containous/traefik/v2/pkg/cli"
	"github.com/containous/traefik/v2/pkg/collector"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/config/static"
	"github.com/containous/traefik/v2/pkg/log"
	"github.com/containous/traefik/v2/pkg/metrics"
	"github.com/containous/traefik/v2/pkg/middlewares/accesslog"
	"github.com/containous/traefik/v2/pkg/provider/acme"
	"github.com/containous/traefik/v2/pkg/provider/aggregator"
	"github.com/containous/traefik/v2/pkg/provider/traefik"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/containous/traefik/v2/pkg/server"
	"github.com/containous/traefik/v2/pkg/server/middleware"
	"github.com/containous/traefik/v2/pkg/server/service"
	traefiktls "github.com/containous/traefik/v2/pkg/tls"
	"github.com/containous/traefik/v2/pkg/types"
	"github.com/containous/traefik/v2/pkg/version"
	"github.com/coreos/go-systemd/daemon"
	"github.com/sirupsen/logrus"
	"github.com/vulcand/oxy/roundrobin"
)

// NewCmd builds a new Proxy command.
func NewCmd(loaders []cli.ResourceLoader) *cli.Command {
	tConfig := cmd.NewTraefikConfiguration()

	return &cli.Command{
		Name:          "proxy",
		Description:   `Proxy command.`,
		Configuration: tConfig,
		Run: func(_ []string) error {
			return runCmd(&tConfig.Configuration)
		},
		Resources: loaders,
	}
}

func runCmd(staticConfiguration *static.Configuration) error {
	configureLogging(staticConfiguration)

	http.DefaultTransport.(*http.Transport).Proxy = http.ProxyFromEnvironment

	if err := roundrobin.SetDefaultWeight(0); err != nil {
		log.WithoutContext().Errorf("Could not set round robin default weight: %v", err)
	}

	staticConfiguration.SetEffectiveConfiguration()

	if err := staticConfiguration.ValidateConfiguration(); err != nil {
		return err
	}

	log.WithoutContext().Infof("Traefik version %s built on %s", version.Version, version.BuildDate)

	jsonConf, err := json.Marshal(staticConfiguration)
	if err != nil {
		log.WithoutContext().Errorf("Could not marshal static configuration: %v", err)
		log.WithoutContext().Debugf("Static configuration loaded [struct] %#v", staticConfiguration)
	} else {
		log.WithoutContext().Debugf("Static configuration loaded %s", string(jsonConf))
	}

	stats(staticConfiguration)

	svr, err := setupServer(staticConfiguration)
	if err != nil {
		return err
	}

	ctx := cmd.ContextWithSignal(context.Background())

	if staticConfiguration.Ping != nil {
		staticConfiguration.Ping.WithContext(ctx)
	}

	svr.Start(ctx)
	defer svr.Close()

	sent, err := daemon.SdNotify(false, "READY=1")
	if !sent && err != nil {
		log.WithoutContext().Errorf("Failed to notify: %v", err)
	}

	t, err := daemon.SdWatchdogEnabled(false)
	if err != nil {
		log.WithoutContext().Errorf("Could not enable Watchdog: %v", err)
	} else if t != 0 {
		// Call SdNotify every time / 2 as specified by SdWatchdogEnabled doc.
		t /= 2
		log.WithoutContext().Infof("Watchdog activated with timer duration %s", t)
		safe.Go(func() {
			tick := time.NewTicker(t)
			for range tick.C {
				resp, errHealthCheck := healthcheck.Do(*staticConfiguration)
				if resp != nil {
					_ = resp.Body.Close()
				}

				if staticConfiguration.Ping == nil || errHealthCheck == nil {
					if ok, _ := daemon.SdNotify(false, "WATCHDOG=1"); !ok {
						log.WithoutContext().Error("Fail to tick watchdog")
					}
				} else {
					log.WithoutContext().Error(errHealthCheck)
				}
			}
		})
	}

	svr.Wait()
	log.WithoutContext().Info("Shutting down")

	return nil
}

func setupServer(staticConfiguration *static.Configuration) (*server.Server, error) {
	providerAggregator := aggregator.NewProviderAggregator(*staticConfiguration.Providers)

	// Adds internal provider.
	err := providerAggregator.AddProvider(traefik.New(*staticConfiguration))
	if err != nil {
		return nil, err
	}

	tlsManager := traefiktls.NewManager()

	acmeProviders := initACMEProvider(staticConfiguration, &providerAggregator, tlsManager)

	serverEntryPointsTCP, err := server.NewTCPEntryPoints(staticConfiguration.EntryPoints)
	if err != nil {
		return nil, err
	}

	serverEntryPointsUDP, err := server.NewUDPEntryPoints(staticConfiguration.EntryPoints)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	routinesPool := safe.NewPool(ctx)

	metricsRegistry := registerMetricClients(staticConfiguration.Metrics)
	accessLog := setupAccessLog(staticConfiguration.AccessLog)
	chainBuilder := middleware.NewChainBuilder(*staticConfiguration, metricsRegistry, accessLog)
	managerFactory := service.NewManagerFactory(*staticConfiguration, routinesPool, metricsRegistry)
	routerFactory := server.NewRouterFactory(*staticConfiguration, managerFactory, tlsManager, chainBuilder)

	var defaultEntryPoints []string

	for name, cfg := range staticConfiguration.EntryPoints {
		protocol, _ := cfg.GetProtocol()
		if protocol != "udp" && name != static.DefaultInternalEntryPointName {
			defaultEntryPoints = append(defaultEntryPoints, name)
		}
	}

	sort.Strings(defaultEntryPoints)

	watcher := server.NewConfigurationWatcher(
		routinesPool,
		providerAggregator,
		time.Duration(staticConfiguration.Providers.ProvidersThrottleDuration),
		defaultEntryPoints,
	)

	watcher.AddListener(func(conf dynamic.Configuration) {
		ctx := context.Background()
		tlsManager.UpdateConfigs(ctx, conf.TLS.Stores, conf.TLS.Options, conf.TLS.Certificates)
	})

	watcher.AddListener(func(_ dynamic.Configuration) {
		metricsRegistry.ConfigReloadsCounter().Add(1)
		metricsRegistry.LastConfigReloadSuccessGauge().Set(float64(time.Now().Unix()))
	})

	watcher.AddListener(switchRouter(routerFactory, acmeProviders, serverEntryPointsTCP, serverEntryPointsUDP))

	watcher.AddListener(func(conf dynamic.Configuration) {
		if metricsRegistry.IsEpEnabled() || metricsRegistry.IsSvcEnabled() {
			var eps []string
			for key := range serverEntryPointsTCP {
				eps = append(eps, key)
			}

			metrics.OnConfigurationUpdate(conf, eps)
		}
	})

	resolverNames := map[string]struct{}{}
	for _, p := range acmeProviders {
		resolverNames[p.ResolverName] = struct{}{}

		watcher.AddListener(p.ListenConfiguration)
	}

	watcher.AddListener(func(config dynamic.Configuration) {
		for rtName, rt := range config.HTTP.Routers {
			if rt.TLS == nil || rt.TLS.CertResolver == "" {
				continue
			}

			if _, ok := resolverNames[rt.TLS.CertResolver]; !ok {
				log.WithoutContext().Errorf("the router %s uses an unknown resolver: %s", rtName, rt.TLS.CertResolver)
			}
		}
	})

	return server.NewServer(routinesPool, serverEntryPointsTCP, serverEntryPointsUDP, watcher, chainBuilder, accessLog), nil
}

func switchRouter(routerFactory *server.RouterFactory, acmeProviders []*acme.Provider, serverEntryPointsTCP server.TCPEntryPoints, serverEntryPointsUDP server.UDPEntryPoints) func(conf dynamic.Configuration) {
	return func(conf dynamic.Configuration) {
		routers, udpRouters := routerFactory.CreateRouters(conf)
		for entryPointName, rt := range routers {
			for _, p := range acmeProviders {
				if p != nil && p.HTTPChallenge != nil && p.HTTPChallenge.EntryPoint == entryPointName {
					rt.HTTPHandler(p.CreateHandler(rt.GetHTTPHandler()))
					break
				}
			}
		}

		serverEntryPointsTCP.Switch(routers)
		serverEntryPointsUDP.Switch(udpRouters)
	}
}

// initACMEProvider creates an acme provider from the ACME part of globalConfiguration
func initACMEProvider(c *static.Configuration, providerAggregator *aggregator.ProviderAggregator, tlsManager *traefiktls.Manager) []*acme.Provider {
	challengeStore := acme.NewLocalChallengeStore()
	localStores := map[string]*acme.LocalStore{}

	var resolvers []*acme.Provider

	for name, resolver := range c.CertificatesResolvers {
		if resolver.ACME != nil {
			if localStores[resolver.ACME.Storage] == nil {
				localStores[resolver.ACME.Storage] = acme.NewLocalStore(resolver.ACME.Storage)
			}

			p := &acme.Provider{
				Configuration:  resolver.ACME,
				Store:          localStores[resolver.ACME.Storage],
				ChallengeStore: challengeStore,
				ResolverName:   name,
			}

			if err := providerAggregator.AddProvider(p); err != nil {
				log.WithoutContext().Errorf("Skipping ACME resolver %q: %v", name, err)
				continue
			}

			p.SetTLSManager(tlsManager)

			if p.TLSChallenge != nil {
				tlsManager.TLSAlpnGetter = p.GetTLSALPNCertificate
			}

			p.SetConfigListenerChan(make(chan dynamic.Configuration))

			resolvers = append(resolvers, p)
		}
	}

	return resolvers
}

func registerMetricClients(metricsConfig *types.Metrics) metrics.Registry {
	if metricsConfig == nil {
		return metrics.NewVoidRegistry()
	}

	var registries []metrics.Registry

	if metricsConfig.Prometheus != nil {
		ctx := log.With(context.Background(), log.Str(log.MetricsProviderName, "prometheus"))
		prometheusRegister := metrics.RegisterPrometheus(ctx, metricsConfig.Prometheus)

		if prometheusRegister != nil {
			registries = append(registries, prometheusRegister)

			log.FromContext(ctx).Debug("Configured Prometheus metrics")
		}
	}

	if metricsConfig.Datadog != nil {
		ctx := log.With(context.Background(), log.Str(log.MetricsProviderName, "datadog"))
		registries = append(registries, metrics.RegisterDatadog(ctx, metricsConfig.Datadog))

		log.FromContext(ctx).Debugf("Configured Datadog metrics: pushing to %s once every %s",
			metricsConfig.Datadog.Address, metricsConfig.Datadog.PushInterval)
	}

	if metricsConfig.StatsD != nil {
		ctx := log.With(context.Background(), log.Str(log.MetricsProviderName, "statsd"))
		registries = append(registries, metrics.RegisterStatsd(ctx, metricsConfig.StatsD))
		log.FromContext(ctx).Debugf("Configured StatsD metrics: pushing to %s once every %s",
			metricsConfig.StatsD.Address, metricsConfig.StatsD.PushInterval)
	}

	if metricsConfig.InfluxDB != nil {
		ctx := log.With(context.Background(), log.Str(log.MetricsProviderName, "influxdb"))
		registries = append(registries, metrics.RegisterInfluxDB(ctx, metricsConfig.InfluxDB))
		log.FromContext(ctx).Debugf("Configured InfluxDB metrics: pushing to %s once every %s",
			metricsConfig.InfluxDB.Address, metricsConfig.InfluxDB.PushInterval)
	}

	return metrics.NewMultiRegistry(registries)
}

func setupAccessLog(conf *types.AccessLog) *accesslog.Handler {
	if conf == nil {
		return nil
	}

	accessLoggerMiddleware, err := accesslog.NewHandler(conf)
	if err != nil {
		log.WithoutContext().Warnf("Unable to create access logger: %v", err)
		return nil
	}

	return accessLoggerMiddleware
}

func configureLogging(staticConfiguration *static.Configuration) {
	// Configure default log flags.
	stdlog.SetFlags(stdlog.Lshortfile | stdlog.LstdFlags)

	// Configure log level.
	// An explicitly defined log level always has precedence. If none is
	// given and debug mode is disabled, the default is ERROR, and DEBUG
	// otherwise.
	levelStr := "error"
	if staticConfiguration.Log != nil && staticConfiguration.Log.Level != "" {
		levelStr = strings.ToLower(staticConfiguration.Log.Level)
	}

	level, err := logrus.ParseLevel(levelStr)
	if err != nil {
		log.WithoutContext().Errorf("Error getting log level: %v", err)
	}

	log.SetLevel(level)

	var logFile string
	if staticConfiguration.Log != nil && len(staticConfiguration.Log.FilePath) > 0 {
		logFile = staticConfiguration.Log.FilePath
	}

	// Configure log format.
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
			if closeErr := log.CloseFile(); closeErr != nil {
				log.WithoutContext().Errorf("Error while closing log file: %v", closeErr)
			}
		})

		if err != nil {
			log.WithoutContext().Errorf("Error while opening log file %s: %v", logFile, err)
		}
	}
}

func stats(staticConfiguration *static.Configuration) {
	logger := log.WithoutContext()

	if staticConfiguration.Global.SendAnonymousUsage {
		logger.Info(`Stats collection is enabled.`)
		logger.Info(`Many thanks for contributing to Traefik's improvement by allowing us to receive anonymous information from your configuration.`)
		logger.Info(`Help us improve Traefik by leaving this feature on :)`)
		logger.Info(`More details on: https://docs.traefik.io/contributing/data-collection/`)
		collect(staticConfiguration)
	} else {
		logger.Info(`
Stats collection is disabled.
Help us improve Traefik by turning this feature on :)
More details on: https://docs.traefik.io/contributing/data-collection/
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
