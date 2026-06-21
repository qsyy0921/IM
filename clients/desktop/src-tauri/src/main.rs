#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

const RUNTIME_TARGET: &str = "windows-desktop";
const NATIVE_BRIDGE_VERSION: &str = "0.1.0";
const RUNTIME_LABEL: &str = "NexusIM desktop shell";
const LOCAL_STORE_CURRENT: &str = "local-storage";
const LOCAL_STORE_TARGET: &str = "sqlite";
const NATIVE_STORE_READY: &str = "false";
const NATIVE_STORE_REASON: &str = "sqlite-native-bridge-unavailable";
const NATIVE_STORE_BRIDGE: &str = "tauri-sqlite";

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

fn main() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![runtime_metadata])
        .run(tauri::generate_context!())
        .expect("failed to run NexusIM desktop shell");
}
