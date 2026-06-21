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
            .toString()
    }

    @JavascriptInterface
    fun target(): String {
        return RUNTIME_TARGET
    }

    companion object {
        const val RUNTIME_TARGET: String = "android"
        const val NATIVE_BRIDGE_VERSION: String = "0.1.0"
        const val RUNTIME_LABEL: String = "NexusIM Android shell"
    }
}
