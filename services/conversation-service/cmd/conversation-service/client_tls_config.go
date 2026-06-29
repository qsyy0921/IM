package main

import (
	"errors"

	rpcinfra "github.com/qsyy0921/IM/services/conversation-service/internal/infrastructure/rpc"
)

func timelineClientTLSConfigFromEnv() (rpcinfra.TimelineClientTLSConfig, error) {
	config := rpcinfra.TimelineClientTLSConfig{
		CAFile:         envString("NEXUSIM_TIMELINE_SERVICE_TLS_CA_FILE", ""),
		ServerName:     envString("NEXUSIM_TIMELINE_SERVICE_TLS_SERVER_NAME", ""),
		ClientCertFile: envString("NEXUSIM_TIMELINE_SERVICE_TLS_CLIENT_CERT_FILE", ""),
		ClientKeyFile:  envString("NEXUSIM_TIMELINE_SERVICE_TLS_CLIENT_KEY_FILE", ""),
	}
	if !config.Enabled() {
		return config, nil
	}
	if config.CAFile == "" {
		return config, errors.New("NEXUSIM_TIMELINE_SERVICE_TLS_CA_FILE is required when timeline-service TLS is configured")
	}
	if (config.ClientCertFile == "") != (config.ClientKeyFile == "") {
		return config, errors.New("NEXUSIM_TIMELINE_SERVICE_TLS_CLIENT_CERT_FILE and NEXUSIM_TIMELINE_SERVICE_TLS_CLIENT_KEY_FILE must be configured together")
	}
	return config, nil
}
