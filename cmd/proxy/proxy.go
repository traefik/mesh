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

	"github.com/containous/maesh/cmd"
	httpprovider "github.com/containous/maesh/pkg/proxy/provider/http"
	"github.com/containous/maesh/pkg/version"
	"github.com/containous/traefik/v2/pkg/cli"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/config/static"
	"github.com/containous/traefik/v2/pkg/log"
	"github.com/containous/traefik/v2/pkg/metrics"
	"github.com/containous/traefik/v2/pkg/middlewares/accesslog"
	"github.com/containous/traefik/v2/pkg/provider/aggregator"
	"github.com/containous/traefik/v2/pkg/provider/traefik"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/containous/traefik/v2/pkg/server"
	"github.com/containous/traefik/v2/pkg/server/middleware"
	"github.com/containous/traefik/v2/pkg/server/service"
	traefiktls "github.com/containous/traefik/v2/pkg/tls"
	"github.com/containous/traefik/v2/pkg/types"
	"github.com/sirupsen/logrus"
	"github.com/vulcand/oxy/roundrobin"
)

// NewCmd builds a new Proxy command.
func NewCmd(loaders []cli.ResourceLoader) *cli.Command {
	proxyConfig := cmd.NewProxyConfiguration()

	return &cli.Command{
		Name:          "proxy",
		Description:   `Proxy command.`,
		Configuration: proxyConfig,
		Run: func(_ []string) error {
			return runCmd(proxyConfig)
		},
		Resources: loaders,
	}
}

func runCmd(proxyConfiguration *cmd.ProxyConfiguration) error {
	proxyConfiguration.Configuration.SetEffectiveConfiguration()

	if err := proxyConfiguration.Configuration.ValidateConfiguration(); err != nil {
		return err
	}

	configureLogging(&proxyConfiguration.Configuration)

	http.DefaultTransport.(*http.Transport).Proxy = http.ProxyFromEnvironment

	if err := roundrobin.SetDefaultWeight(0); err != nil {
		log.WithoutContext().Errorf("Could not set round robin default weight: %v", err)
	}

	log.WithoutContext().Infof("Maesh proxy version %s built on %s", version.Version, version.Date)

	if proxyConfiguration.Configuration.Log != nil && proxyConfiguration.Configuration.Log.Level == "DEBUG" {
		jsonConf, err := json.Marshal(proxyConfiguration.Configuration)
		if err != nil {
			log.WithoutContext().Errorf("Could not marshal static configuration: %v", err)
			log.WithoutContext().Debugf("Static configuration loaded [struct] %#v", proxyConfiguration.Configuration)

			return err
		}

		log.WithoutContext().Debugf("Static configuration loaded %s", string(jsonConf))
	}

	svr, err := setupServer(proxyConfiguration)
	if err != nil {
		return err
	}

	ctx := cmd.ContextWithSignal(context.Background())

	if proxyConfiguration.Configuration.Ping != nil {
		proxyConfiguration.Configuration.Ping.WithContext(ctx)
	}

	svr.Start(ctx)
	defer svr.Close()

	svr.Wait()
	log.WithoutContext().Info("Shutting down")

	return nil
}

func setupServer(proxyConfiguration *cmd.ProxyConfiguration) (*server.Server, error) {
	providerAggregator := aggregator.NewProviderAggregator(static.Providers{})

	// Adds internal provider.
	err := providerAggregator.AddProvider(traefik.New(proxyConfiguration.Configuration))
	if err != nil {
		return nil, err
	}

	// Require the HTTP Provider to be configured.
	err = providerAggregator.AddProvider(httpprovider.New(proxyConfiguration.Endpoint,
		proxyConfiguration.PollInterval, proxyConfiguration.PollTimeout))
	if err != nil {
		return nil, err
	}

	tlsManager := traefiktls.NewManager()

	serverEntryPointsTCP, err := server.NewTCPEntryPoints(proxyConfiguration.Configuration.EntryPoints)
	if err != nil {
		return nil, err
	}

	serverEntryPointsUDP, err := server.NewUDPEntryPoints(proxyConfiguration.Configuration.EntryPoints)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	routinesPool := safe.NewPool(ctx)

	metricsRegistry := registerMetricClients(proxyConfiguration.Configuration.Metrics)
	accessLog := setupAccessLog(proxyConfiguration.Configuration.AccessLog)
	chainBuilder := middleware.NewChainBuilder(proxyConfiguration.Configuration, metricsRegistry, accessLog)
	managerFactory := service.NewManagerFactory(proxyConfiguration.Configuration, routinesPool, metricsRegistry)
	routerFactory := server.NewRouterFactory(proxyConfiguration.Configuration, managerFactory, tlsManager, chainBuilder)

	var defaultEntryPoints []string

	for name, cfg := range proxyConfiguration.Configuration.EntryPoints {
		protocol, _ := cfg.GetProtocol()
		if protocol != "udp" && name != static.DefaultInternalEntryPointName {
			defaultEntryPoints = append(defaultEntryPoints, name)
		}
	}

	sort.Strings(defaultEntryPoints)

	watcher := server.NewConfigurationWatcher(
		routinesPool,
		providerAggregator,
		time.Duration(proxyConfiguration.Configuration.Providers.ProvidersThrottleDuration),
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

	watcher.AddListener(switchRouter(routerFactory, serverEntryPointsTCP, serverEntryPointsUDP))

	watcher.AddListener(func(conf dynamic.Configuration) {
		if metricsRegistry.IsEpEnabled() || metricsRegistry.IsSvcEnabled() {
			var eps []string
			for key := range serverEntryPointsTCP {
				eps = append(eps, key)
			}

			metrics.OnConfigurationUpdate(conf, eps)
		}
	})

	return server.NewServer(routinesPool, serverEntryPointsTCP, serverEntryPointsUDP, watcher, chainBuilder, accessLog), nil
}

func switchRouter(routerFactory *server.RouterFactory, serverEntryPointsTCP server.TCPEntryPoints, serverEntryPointsUDP server.UDPEntryPoints) func(conf dynamic.Configuration) {
	return func(conf dynamic.Configuration) {
		routers, udpRouters := routerFactory.CreateRouters(conf)

		serverEntryPointsTCP.Switch(routers)
		serverEntryPointsUDP.Switch(udpRouters)
	}
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
