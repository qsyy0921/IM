#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

const RUNTIME_TARGET: &str = "windows-desktop";
const NATIVE_BRIDGE_VERSION: &str = "0.1.0";
const RUNTIME_LABEL: &str = "NexusIM desktop shell";

#[tauri::command]
fn runtime_metadata() -> String {
    format!(
        "{{\"target\":\"{}\",\"nativeBridgeVersion\":\"{}\",\"runtimeLabel\":\"{}\"}}",
        RUNTIME_TARGET, NATIVE_BRIDGE_VERSION, RUNTIME_LABEL
    )
}

fn main() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![runtime_metadata])
        .run(tauri::generate_context!())
        .expect("failed to run NexusIM desktop shell");
}
