function Ensure-KafkaTopic {
    param([string]$Topic)
    docker exec $KafkaExecContainer kafka-topics `
        --bootstrap-server $KafkaAdminBootstrap `
        --create `
        --if-not-exists `
        --topic $Topic `
        --partitions 3 `
        --replication-factor $KafkaTopicReplicationFactor | Out-Null
}

function Reset-ConsumerGroupToLatest {
    param(
        [string]$Group,
        [string]$Topic
    )
    docker exec $KafkaExecContainer kafka-consumer-groups `
        --bootstrap-server $KafkaAdminBootstrap `
        --group $Group `
        --topic $Topic `
        --reset-offsets `
        --to-latest `
        --execute | Out-Null
}

function Clear-LocalMessageOutboxSmokeResiduals {
    $cleanupSQL = @'
DELETE FROM message_outbox
WHERE status <> 'PUBLISHED'
  AND (
    tenant_id LIKE 'tenant-it-%'
    OR tenant_id LIKE 'tenant-outbox-concurrent-%'
    OR tenant_id LIKE 'tenant-policy-context-%'
    OR tenant_id LIKE 'tenant-push-%'
    OR tenant_id LIKE 'tenant-push-gateway-%'
  );
'@
    $cleanupFile = Join-Path $resultDir "cleanup-message-outbox-residuals.sql"
    $cleanupLog = Join-Path $logDir "preflight-cleanup.out.log"
    Set-Content -LiteralPath $cleanupFile -Value $cleanupSQL -Encoding ASCII
    docker cp $cleanupFile "${PostgresExecContainer}:/tmp/cleanup-message-outbox-residuals.sql" | Out-Null
    docker exec $PostgresExecContainer psql `
        -U nexusim `
        -d nexusim `
        -v ON_ERROR_STOP=1 `
        -f /tmp/cleanup-message-outbox-residuals.sql |
        Tee-Object -FilePath $cleanupLog | Out-Null
}

function Wait-Tcp {
    param(
        [string]$HostName,
        [int]$Port,
        [int]$TimeoutSeconds = 20
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $client = [System.Net.Sockets.TcpClient]::new()
        try {
            $connect = $client.BeginConnect($HostName, $Port, $null, $null)
            if ($connect.AsyncWaitHandle.WaitOne(300)) {
                $client.EndConnect($connect)
                return
            }
        } catch {
        } finally {
            $client.Close()
        }
        Start-Sleep -Milliseconds 200
    }
    throw "Timed out waiting for ${HostName}:${Port}"
}

function Start-NexusProcess {
    param(
        [string]$Name,
        [string]$FilePath,
        [hashtable]$Env,
        [int]$Port = 0
    )
    foreach ($key in $Env.Keys) {
        [Environment]::SetEnvironmentVariable($key, [string]$Env[$key], "Process")
    }
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    $proc = Start-Process -FilePath $FilePath `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput $out `
        -RedirectStandardError $err
    if ($Port -gt 0) {
        Wait-Tcp -HostName "127.0.0.1" -Port $Port
    } else {
        Start-Sleep -Milliseconds 800
    }
    return $proc
}

function Add-PushRedisEnv {
    param([hashtable]$Env)
    $Env["NEXUSIM_PUSH_REDIS_MODE"] = $RedisMode
    $Env["NEXUSIM_PUSH_REDIS_KEY_PREFIX"] = $pushRouteKeyPrefix
    if ($RedisMode -eq "sentinel") {
        $Env["NEXUSIM_PUSH_REDIS_SENTINEL_ADDRS"] = $RedisSentinelAddrs
        $Env["NEXUSIM_PUSH_REDIS_SENTINEL_MASTER_NAME"] = $RedisSentinelMasterName
    } elseif ($RedisMode -eq "cluster") {
        $Env["NEXUSIM_PUSH_REDIS_CLUSTER_ADDRS"] = $RedisClusterAddrs
    } else {
        $Env["NEXUSIM_PUSH_REDIS_ADDR"] = $RedisAddr
    }
    return $Env
}

function Add-PushAuthEnv {
    param([hashtable]$Env)
    $Env["NEXUSIM_PUSH_AUTH_MODE"] = $PushAuthMode
    $Env["NEXUSIM_PUSH_AUTH_HMAC_SECRET"] = $PushAuthHmacSecret
    $Env["NEXUSIM_PUSH_AUTH_HMAC_PREVIOUS_SECRETS"] = $PushAuthHmacPreviousSecrets
    if ($rs256SmokeKeyMaterial) {
        $Env["NEXUSIM_PUSH_AUTH_JWKS_URL"] = "http://127.0.0.1:11611/.well-known/jwks.json"
        $Env["NEXUSIM_PUSH_AUTH_JWKS_REFRESH_INTERVAL"] = "1s"
        $Env["NEXUSIM_PUSH_AUTH_TRUSTED_ISSUERS"] = "nexusim-identity"
    }
    return $Env
}

function Add-PushWSTLSEnv {
    param(
        [hashtable]$Env,
        [string]$DebugAddr = ""
    )
    if (-not [string]::IsNullOrWhiteSpace($PushWsTlsCertFile)) {
        $Env["NEXUSIM_PUSH_WS_TLS_CERT_FILE"] = $PushWsTlsCertFile
    }
    if (-not [string]::IsNullOrWhiteSpace($PushWsTlsKeyFile)) {
        $Env["NEXUSIM_PUSH_WS_TLS_KEY_FILE"] = $PushWsTlsKeyFile
    }
    if (-not [string]::IsNullOrWhiteSpace($PushWsTlsClientCaFile)) {
        $Env["NEXUSIM_PUSH_WS_TLS_CLIENT_CA_FILE"] = $PushWsTlsClientCaFile
    }
    if (-not [string]::IsNullOrWhiteSpace($PushWsTlsRequireClientCert)) {
        $Env["NEXUSIM_PUSH_WS_TLS_REQUIRE_CLIENT_CERT"] = $PushWsTlsRequireClientCert
    }
    if (-not [string]::IsNullOrWhiteSpace($PushWsTlsClientAllowedDnsNames)) {
        $Env["NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_DNS_NAMES"] = $PushWsTlsClientAllowedDnsNames
    }
    if (-not [string]::IsNullOrWhiteSpace($PushWsTlsClientAllowedUris)) {
        $Env["NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_URIS"] = $PushWsTlsClientAllowedUris
    }
    if (-not [string]::IsNullOrWhiteSpace($DebugAddr)) {
        $Env["NEXUSIM_PUSH_DEBUG_ADDR"] = $DebugAddr
    }
    return $Env
}

function New-RS256SmokeKeyMaterial {
    param(
        [string]$Directory,
        [string]$KeyID
    )
    $generator = Join-Path $Directory "generate-rs256-smoke-key.go"
    @'
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"math/big"
)

func base64URL(bytes []byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func main() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	jwks, err := json.Marshal(map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"use": "sig",
			"kid": os.Args[1],
			"alg": "RS256",
			"n":   base64URL(key.PublicKey.N.Bytes()),
			"e":   base64URL(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}},
	})
	if err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{
		"private_key_pem": string(privatePEM),
		"jwks_json":       string(jwks),
	}); err != nil {
		panic(err)
	}
}
'@ | Set-Content -LiteralPath $generator -Encoding UTF8
    $raw = & go run $generator $KeyID
    if ($LASTEXITCODE -ne 0) {
        throw "failed to generate RS256 smoke key"
    }
    $material = $raw | ConvertFrom-Json
    $privateKeyFile = Join-Path $Directory "identity-gateway-rs256-private.pem"
    $jwksFile = Join-Path $Directory "push-auth-rs256-jwks.json"
    Set-Content -LiteralPath $privateKeyFile -Value $material.private_key_pem -Encoding ASCII
    Set-Content -LiteralPath $jwksFile -Value $material.jwks_json -Encoding ASCII
    return @{
        KeyID = $KeyID
        PrivateKeyFile = $privateKeyFile
        JwksFile = $jwksFile
    }
}
