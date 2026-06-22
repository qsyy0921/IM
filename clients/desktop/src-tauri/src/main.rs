#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use rusqlite::{params, Connection, OptionalExtension};
use std::fs;
use std::path::PathBuf;
use std::time::{SystemTime, UNIX_EPOCH};
use tauri::Manager;

const RUNTIME_TARGET: &str = "windows-desktop";
const NATIVE_BRIDGE_VERSION: &str = "0.2.0";
const RUNTIME_LABEL: &str = "NexusIM desktop shell";
const LOCAL_STORE_CURRENT: &str = "local-storage";
const LOCAL_STORE_TARGET: &str = "sqlite";
const NATIVE_STORE_READY: &str = "true";
const NATIVE_STORE_REASON: &str = "";
const NATIVE_STORE_BRIDGE: &str = "tauri-sqlite";
const LOCAL_STORE_ERROR: &str = "local store unavailable";
const LOCAL_STORE_KEY_PREFIX: &str = "nexusim:client-message-store:v1:";
const LOCAL_STORE_MAX_KEY_BYTES: usize = 512;
const LOCAL_STORE_MAX_VALUE_BYTES: usize = 4 * 1024 * 1024;

#[tauri::command]
fn runtime_metadata() -> String {
    format!(
        "{{\"target\":\"{}\",\"nativeBridgeVersion\":\"{}\",\"runtimeLabel\":\"{}\",\"capabilities\":{{\"localStore\":{{\"currentDefault\":\"{}\",\"productionTarget\":\"{}\",\"nativeStoreReady\":{},\"nativeStoreReason\":\"{}\",\"nativeStoreBridge\":\"{}\"}}}}}}",
        RUNTIME_TARGET,
        NATIVE_BRIDGE_VERSION,
        RUNTIME_LABEL,
        LOCAL_STORE_CURRENT,
        LOCAL_STORE_TARGET,
        NATIVE_STORE_READY,
        NATIVE_STORE_REASON,
        NATIVE_STORE_BRIDGE
    )
}

#[tauri::command]
fn local_store_get_item(app: tauri::AppHandle, key: String) -> Result<Option<String>, String> {
    validate_store_key(&key)?;
    let conn = open_local_store(&app)?;
    conn.query_row(
        "SELECT value FROM local_store WHERE key = ?1",
        params![key],
        |row| row.get(0),
    )
    .optional()
    .map_err(|_| LOCAL_STORE_ERROR.to_string())
}

#[tauri::command]
fn local_store_set_item(app: tauri::AppHandle, key: String, value: String) -> Result<(), String> {
    validate_store_key(&key)?;
    validate_store_value(&value)?;
    let conn = open_local_store(&app)?;
    conn.execute(
        "INSERT INTO local_store (key, value, updated_at)
         VALUES (?1, ?2, ?3)
         ON CONFLICT(key) DO UPDATE SET
           value = excluded.value,
           updated_at = excluded.updated_at",
        params![key, value, unix_timestamp()],
    )
    .map(|_| ())
    .map_err(|_| LOCAL_STORE_ERROR.to_string())
}

#[tauri::command]
fn local_store_remove_item(app: tauri::AppHandle, key: String) -> Result<(), String> {
    validate_store_key(&key)?;
    let conn = open_local_store(&app)?;
    conn.execute("DELETE FROM local_store WHERE key = ?1", params![key])
        .map(|_| ())
        .map_err(|_| LOCAL_STORE_ERROR.to_string())
}

fn main() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![
            runtime_metadata,
            local_store_get_item,
            local_store_set_item,
            local_store_remove_item
        ])
        .run(tauri::generate_context!())
        .expect("failed to run NexusIM desktop shell");
}

fn open_local_store(app: &tauri::AppHandle) -> Result<Connection, String> {
    let path = local_store_path(app)?;
    let conn = Connection::open(path).map_err(|_| LOCAL_STORE_ERROR.to_string())?;
    conn.execute_batch(
        "CREATE TABLE IF NOT EXISTS local_store (
           key TEXT PRIMARY KEY NOT NULL,
           value TEXT NOT NULL,
           updated_at INTEGER NOT NULL
         );",
    )
    .map_err(|_| LOCAL_STORE_ERROR.to_string())?;
    Ok(conn)
}

fn local_store_path(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    let dir = app
        .path()
        .app_local_data_dir()
        .map_err(|_| LOCAL_STORE_ERROR.to_string())?;
    fs::create_dir_all(&dir).map_err(|_| LOCAL_STORE_ERROR.to_string())?;
    Ok(dir.join("local-store.sqlite3"))
}

fn validate_store_key(key: &str) -> Result<(), String> {
    if key.is_empty()
        || key.len() > LOCAL_STORE_MAX_KEY_BYTES
        || !key.starts_with(LOCAL_STORE_KEY_PREFIX)
        || key.chars().any(char::is_control)
    {
        return Err(LOCAL_STORE_ERROR.to_string());
    }
    Ok(())
}

fn validate_store_value(value: &str) -> Result<(), String> {
    if value.len() > LOCAL_STORE_MAX_VALUE_BYTES {
        return Err(LOCAL_STORE_ERROR.to_string());
    }
    Ok(())
}

fn unix_timestamp() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_secs() as i64)
        .unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use super::{validate_store_key, validate_store_value};

    #[test]
    fn validates_local_store_keys() {
        assert!(validate_store_key("nexusim:client-message-store:v1:default").is_ok());
        assert!(validate_store_key("").is_err());
        assert!(validate_store_key("nexusim:desktop:session").is_err());
        assert!(validate_store_key("other:client-message-store").is_err());
        assert!(validate_store_key("nexusim:bad\nkey").is_err());
    }

    #[test]
    fn validates_local_store_value_size() {
        assert!(validate_store_value("ok").is_ok());
        assert!(validate_store_value(&"x".repeat(super::LOCAL_STORE_MAX_VALUE_BYTES + 1)).is_err());
    }
}
