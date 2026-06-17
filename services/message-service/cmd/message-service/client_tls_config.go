package main

import (
	"errors"

	rpcinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/rpc"
)

func policyClientTLSConfigFromEnv() (rpcinfra.PolicyClientTLSConfig, error) {
	config := rpcinfra.PolicyClientTLSConfig{
		CAFile:         envString("NEXUSIM_POLICY_SERVICE_TLS_CA_FILE", ""),
		ServerName:     envString("NEXUSIM_POLICY_SERVICE_TLS_SERVER_NAME", ""),
		ClientCertFile: envString("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE", ""),
		ClientKeyFile:  envString("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_KEY_FILE", ""),
	}
	if !config.Enabled() {
		return config, nil
	}
	if config.CAFile == "" {
		return config, errors.New("NEXUSIM_POLICY_SERVICE_TLS_CA_FILE is required when policy-service TLS is configured")
	}
	if (config.ClientCertFile == "") != (config.ClientKeyFile == "") {
		return config, errors.New("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE and NEXUSIM_POLICY_SERVICE_TLS_CLIENT_KEY_FILE must be configured together")
	}
	return config, nil
}

func conversationClientTLSConfigFromEnv() (rpcinfra.ConversationClientTLSConfig, error) {
	config := rpcinfra.ConversationClientTLSConfig{
		CAFile:         envString("NEXUSIM_CONVERSATION_SERVICE_TLS_CA_FILE", ""),
		ServerName:     envString("NEXUSIM_CONVERSATION_SERVICE_TLS_SERVER_NAME", ""),
		ClientCertFile: envString("NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_CERT_FILE", ""),
		ClientKeyFile:  envString("NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_KEY_FILE", ""),
	}
	if !config.Enabled() {
		return config, nil
	}
	if config.CAFile == "" {
		return config, errors.New("NEXUSIM_CONVERSATION_SERVICE_TLS_CA_FILE is required when conversation-service TLS is configured")
	}
	if (config.ClientCertFile == "") != (config.ClientKeyFile == "") {
		return config, errors.New("NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_CERT_FILE and NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_KEY_FILE must be configured together")
	}
	return config, nil
}
