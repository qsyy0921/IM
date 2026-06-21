package com.nexusim.android

import android.webkit.JavascriptInterface
import org.json.JSONObject

class NexusIMBridge {
    @JavascriptInterface
    fun runtimeMetadata(): String {
        return JSONObject()
            .put("target", RUNTIME_TARGET)
            .put("nativeBridgeVersion", NATIVE_BRIDGE_VERSION)
            .put("runtimeLabel", RUNTIME_LABEL)
            .put(
                "capabilities",
                JSONObject()
                    .put(
                        "localStore",
                        JSONObject()
                            .put("currentDefault", LOCAL_STORE_CURRENT)
                            .put("productionTarget", LOCAL_STORE_TARGET)
                            .put("nativeStoreReady", NATIVE_STORE_READY)
                            .put("nativeStoreReason", NATIVE_STORE_REASON)
                            .put("nativeStoreBridge", NATIVE_STORE_BRIDGE)
                    )
            )
            .toString()
    }

    companion object {
        const val RUNTIME_TARGET: String = "android"
        const val NATIVE_BRIDGE_VERSION: String = "0.1.0"
        const val RUNTIME_LABEL: String = "NexusIM Android shell"
        const val LOCAL_STORE_CURRENT: String = "local-storage"
        const val LOCAL_STORE_TARGET: String = "sqlite"
        const val NATIVE_STORE_READY: Boolean = false
        const val NATIVE_STORE_REASON: String = "sqlite-native-bridge-unavailable"
        const val NATIVE_STORE_BRIDGE: String = "android-sqlite"
    }
}
